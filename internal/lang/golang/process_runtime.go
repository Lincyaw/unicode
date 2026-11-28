package golang

import (
	"context"
	uccrand "crypto/rand"
	ucbinary "encoding/binary"
	"encoding/json"
	ucflag "flag"
	"fmt"
	"os"
	ucexec "os/exec"
	ucfilepath "path/filepath"
	ucregexp "regexp"
	ucruntime "runtime"
	"slices"
	"strconv"
	"strings"
	ucsync "sync"
	"testing"
	"time"
)

// Under plain go test each property runs in child processes of the test
// binary (docs/design.md §6.3): processes share no package-level state, so
// properties run at the same time, and a property's random cases are split
// among ucProcessShards children that each draw their share from a seed of
// their own. The number of cases stays rapid's configured count. Every
// child gets a temporary directory of its own (TMPDIR, and
// UNICODE_CONTRACT_TEMP to tell it so), removed when it ends however it
// ends; results are reported once all children have ended, sorted by
// property and child.

// ucProcessShards is how many children split one property's random cases.
const ucProcessShards = 8

// ucProcessUnit is a property to run in children: one child when it has a
// time budget, whose percentile is taken over all its cases.
type ucProcessUnit struct {
	ID     string
	Shards int
}

// ucProcessShard is one child and what it gave.
type ucProcessShard struct {
	count  int
	seed   uint64
	output []byte
	err    error
}

// ucProcesses tells whether this test binary runs its properties in child
// processes: not when it is such a child, measures coverage or a profile,
// or replays a rapid seed or fail file, which run in this process as before.
func ucProcesses() bool {
	if os.Getenv("UNICODE_CONTRACT_CHILD") != "" || testing.CoverMode() != "" {
		return false
	}
	inProcess := false
	ucflag.Visit(func(f *ucflag.Flag) {
		switch f.Name {
		case "rapid.seed", "rapid.failfile", "test.cpuprofile", "test.memprofile", "test.blockprofile", "test.mutexprofile", "test.trace":
			inProcess = true
		}
	})
	return !inProcess
}

// ucProcessJobs is how many children run at once: GOMAXPROCS, or fewer when
// UNICODE_TEST_JOBS asks for fewer.
func ucProcessJobs() int {
	jobs := max(1, ucruntime.GOMAXPROCS(0))
	if n, err := strconv.Atoi(os.Getenv("UNICODE_TEST_JOBS")); err == nil && n >= 1 {
		jobs = min(jobs, n)
	}
	return jobs
}

// ucProcessPlan is what every child of this run shares.
type ucProcessPlan struct {
	checks, effective int // rapid's count, and what it runs (a fifth under -short)
	short             bool
	forwarded         []string
	logged            bool // whether go test keeps a test log of this process
	slots             chan struct{}
	ctx               context.Context
}

func ucNewProcessPlan(t *testing.T) (*ucProcessPlan, context.CancelFunc) {
	p := &ucProcessPlan{checks: 100, short: testing.Short(), slots: make(chan struct{}, ucProcessJobs())}
	if f := ucflag.Lookup("rapid.checks"); f != nil {
		if n, err := strconv.Atoi(f.Value.String()); err == nil {
			p.checks = n
		}
	}
	p.effective = p.checks
	if p.short {
		p.effective = p.checks / 5
	}
	ucflag.Visit(func(f *ucflag.Flag) {
		if strings.HasPrefix(f.Name, "rapid.") && f.Name != "rapid.checks" {
			p.forwarded = append(p.forwarded, "-"+f.Name+"="+f.Value.String())
		}
	})
	if testing.Verbose() {
		p.forwarded = append(p.forwarded, "-test.v=true")
	}
	if p.short {
		p.forwarded = append(p.forwarded, "-test.short=true")
	}
	if f := ucflag.Lookup("test.testlogfile"); f != nil && f.Value.String() != "" {
		p.logged = true
	}
	// Children are stopped before go test's own deadline, so that this
	// process still removes their directories and reports them.
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if deadline, ok := t.Deadline(); ok {
		margin := min(10*time.Second, time.Until(deadline)/10)
		ctx, cancel = context.WithDeadline(ctx, deadline.Add(-margin))
		p.forwarded = append(p.forwarded, "-test.timeout="+time.Until(deadline.Add(-margin)).String())
	}
	p.ctx = ctx
	return p, cancel
}

// ucProcessRunAll runs every selected property in children at once, then
// reports each property in a subtest of its own, in the order of their
// identities. A property the -run pattern selects but the early guess
// (ucProcessMayRun) did not is run when its subtest comes.
func ucProcessRunAll(t *testing.T, selected []string, units []ucProcessUnit) {
	plan, cancel := ucNewProcessPlan(t)
	defer cancel()
	units = slices.Clone(units)
	slices.SortFunc(units, func(a, b ucProcessUnit) int { return strings.Compare(a.ID, b.ID) })
	var wait ucsync.WaitGroup
	results := map[string][]ucProcessShard{}
	for _, u := range units {
		if (len(selected) == 0 || slices.Contains(selected, u.ID)) && ucProcessMayRun(u.ID) {
			results[u.ID] = plan.start(&wait, u)
		}
	}
	wait.Wait()
	for _, u := range units {
		if len(selected) > 0 && !slices.Contains(selected, u.ID) {
			continue
		}
		t.Run(u.ID, func(t *testing.T) {
			shards, ok := results[u.ID]
			if !ok {
				var late ucsync.WaitGroup
				shards = plan.start(&late, u)
				late.Wait()
			}
			plan.report(t, u.ID, shards)
		})
	}
}

// start launches the children of u and returns their slots, which they fill
// before wait is done.
func (p *ucProcessPlan) start(wait *ucsync.WaitGroup, u ucProcessUnit) []ucProcessShard {
	shards := make([]ucProcessShard, max(1, min(u.Shards, p.effective)))
	ids, err := json.Marshal([]string{u.ID})
	for i := range shards {
		s := &shards[i]
		s.count = p.effective / len(shards)
		if i < p.effective%len(shards) {
			s.count++
		}
		s.seed = ucProcessSeed()
		if err != nil {
			s.err = err
			continue
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			p.slots <- struct{}{}
			defer func() { <-p.slots }()
			s.output, s.err = p.run(u.ID, string(ids), s)
		}()
	}
	return shards
}

// run runs one child with a temporary directory of its own and removes the
// directory once the child has ended, killed or not; the child's test log is
// replayed here first.
func (p *ucProcessPlan) run(id, ids string, s *ucProcessShard) ([]byte, error) {
	dir, err := os.MkdirTemp("", "unicode-process-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	temp := ucfilepath.Join(dir, "tmp")
	if err := os.Mkdir(temp, 0o700); err != nil {
		return nil, err
	}
	requested := s.count
	if p.short {
		requested = s.count * 5
	}
	args := append([]string{"-test.run=^TestUnicodeContracts$/^" + ucregexp.QuoteMeta(id) + "$", "-test.count=1", "-rapid.checks=" + strconv.Itoa(requested), "-rapid.seed=" + strconv.FormatUint(s.seed, 10)}, p.forwarded...)
	log := ucfilepath.Join(dir, "test.log")
	if p.logged {
		args = append(args, "-test.testlogfile="+log)
	}
	cmd := ucexec.CommandContext(p.ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), "UNICODE_CONTRACT_CHILD=1", "UNICODE_CONTRACT_IDS="+ids, "UNICODE_CONTRACT_TEMP="+temp, "TMPDIR="+temp, "TMP="+temp, "TEMP="+temp)
	cmd.WaitDelay = 10 * time.Second
	output, err := cmd.CombinedOutput()
	if p.logged {
		data, readErr := os.ReadFile(log)
		if readErr != nil && err == nil {
			err = fmt.Errorf("the child wrote no test log: %w", readErr)
		}
		defer ucReplayTestLog(string(data))
	}
	return output, err
}

// report fails t with every child that failed, in their order, each named
// by the property, its place and its seed; under -v it logs every child.
func (p *ucProcessPlan) report(t *testing.T, id string, shards []ucProcessShard) {
	for i, s := range shards {
		if s.err != nil {
			t.Errorf("%s: process %d of %d (%d cases, -rapid.seed=%d) failed: %v\n%s", id, i+1, len(shards), s.count, s.seed, s.err, s.output)
		} else if testing.Verbose() {
			t.Logf("process %d of %d (%d cases, -rapid.seed=%d):\n%s", i+1, len(shards), s.count, s.seed, s.output)
		}
	}
}

// ucProcessMayRun guesses whether -test.run and -test.skip let the subtest
// id run, so that its children start with the others. It reads a pattern only
// when it has no brackets, parentheses, alternatives or escaped slashes, which
// change how go test splits it into levels, and answers yes when it cannot
// tell: go test still selects the subtest itself.
func ucProcessMayRun(id string) bool {
	levels := func(name string) []*ucregexp.Regexp {
		f := ucflag.Lookup(name)
		if f == nil || f.Value.String() == "" || strings.ContainsAny(f.Value.String(), "[(|") || strings.Contains(f.Value.String(), "\\/") {
			return nil
		}
		var out []*ucregexp.Regexp
		for _, part := range strings.Split(f.Value.String(), "/") {
			re, err := ucregexp.Compile(part)
			if err != nil {
				return nil
			}
			out = append(out, re)
		}
		return out
	}
	if run := levels("test.run"); len(run) >= 2 && !run[1].MatchString(id) {
		return false
	}
	if skip := levels("test.skip"); len(skip) == 2 && skip[0].MatchString("TestUnicodeContracts") && skip[1].MatchString(id) {
		return false
	}
	return true
}

// ucReplayTestLog repeats in this process the reads a child's test log
// records, so that this process's own test log, by which go test caches the
// package's result, holds them: each opened or examined path, made absolute
// against the child's working directory, and each environment variable.
func ucReplayTestLog(data string) {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for _, line := range strings.Split(data, "\n") {
		op, name, ok := strings.Cut(line, " ")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		if op != "getenv" && !ucfilepath.IsAbs(name) {
			name = ucfilepath.Join(dir, name)
		}
		switch op {
		case "getenv":
			_ = os.Getenv(name)
		case "chdir":
			dir = name
		case "stat":
			_, _ = os.Stat(name)
		case "open":
			// Only a regular file or a directory is opened again: opening a
			// pipe or a device could block.
			if info, err := os.Stat(name); err != nil || info.Mode().IsRegular() || info.IsDir() {
				if f, err := os.Open(name); err == nil {
					_ = f.Close()
				}
			}
		}
	}
}

// ucProcessSeed is a random nonzero seed; rapid draws one itself only for zero.
func ucProcessSeed() uint64 {
	for {
		var b [8]byte
		if _, err := uccrand.Read(b[:]); err != nil {
			panic(fmt.Sprintf("unicode: no random seed for a property process: %v", err))
		}
		if seed := ucbinary.LittleEndian.Uint64(b[:]); seed != 0 {
			return seed
		}
	}
}

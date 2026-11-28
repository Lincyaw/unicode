package golang

import (
	ucbytes "bytes"
	"context"
	uccrand "crypto/rand"
	ucbinary "encoding/binary"
	"encoding/json"
	"errors"
	ucflag "flag"
	"fmt"
	ucfs "io/fs"
	"os"
	ucexec "os/exec"
	ucfilepath "path/filepath"
	"reflect"
	ucregexp "regexp"
	ucruntime "runtime"
	"slices"
	"strconv"
	"strings"
	ucsync "sync"
	ucsyscall "syscall"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Under plain go test each property runs in child processes of the test
// binary (docs/design.md §6.3): processes share no package-level state, so
// properties run at the same time, and a property's random cases are split
// among ucProcessShards children that each draw their share from a seed of
// their own. The number of cases stays rapid's configured count.
//
// A child holds a token of a pool shared by every test binary of the user
// on this machine, so at most UNICODE_TEST_JOBS (by default GOMAXPROCS)
// children run at once. Each child has a temporary directory of its own
// (TMPDIR, and UNICODE_CONTRACT_TEMP to tell it so) and a process group of
// its own; once it ends, however it ends, its group is killed and its
// directory removed. The children of one run share a content cache made for
// the run (UNICODE_CACHE), removed with it, unless UNICODE_CACHE is set.
// Results are reported once every child has ended, sorted by property and
// child.

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
// or replays a rapid seed or fail file, given on the command line or by
// rapid's environment variables, which run in this process as before.
func ucProcesses() bool {
	if os.Getenv("UNICODE_CONTRACT_CHILD") != "" || testing.CoverMode() != "" {
		return false
	}
	// rapid takes the defaults of its flags from RAPID_SEED and the like, so
	// the values decide, not whether a flag was given.
	for _, name := range []string{"rapid.seed", "rapid.failfile", "test.cpuprofile", "test.memprofile", "test.blockprofile", "test.mutexprofile", "test.trace"} {
		if f := ucflag.Lookup(name); f != nil && f.Value.String() != "" && f.Value.String() != "0" {
			return false
		}
	}
	return true
}

// ucDeclaredElsewhere tells whether a declared case reaching tb is replayed
// by another process: the first child of a property replays its declared
// cases, the others only draw random ones.
func ucDeclaredElsewhere(tb rapid.TB) bool {
	if _, declared := tb.(*testing.T); !declared {
		return false
	}
	shard := os.Getenv("UNICODE_CONTRACT_SHARD")
	return shard != "" && shard != "0"
}

// ucProcessPlan is what every child of this run shares.
type ucProcessPlan struct {
	effective int // the cases rapid runs: its count, or a fifth of it under -short
	short     bool
	forwarded []string
	logged    bool // whether go test keeps a test log of this process
	cache     string
	pool      ucProcessPool
	ctx       context.Context
	deadline  time.Time
}

func ucNewProcessPlan(t *testing.T) (*ucProcessPlan, func()) {
	p := &ucProcessPlan{short: testing.Short(), pool: ucNewProcessPool()}
	f := ucflag.Lookup("rapid.checks")
	if f == nil {
		t.Fatal("rapid registers no -rapid.checks flag")
	}
	checks, err := strconv.Atoi(f.Value.String())
	if err != nil {
		t.Fatalf("-rapid.checks is not a count: %v", err)
	}
	p.effective = checks
	if p.short {
		p.effective = checks / 5
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
	cleanups := []func(){}
	if os.Getenv("UNICODE_CACHE") == "" {
		if p.cache, err = os.MkdirTemp("", "unicode-cache-run-"); err != nil {
			t.Fatal(err)
		}
		cleanups = append(cleanups, func() { _ = os.RemoveAll(p.cache) })
	}
	// Children are stopped before go test's own deadline, so that this
	// process still removes their directories and reports them.
	p.ctx = context.Background()
	if deadline, ok := t.Deadline(); ok {
		p.deadline = deadline.Add(-min(10*time.Second, time.Until(deadline)/10))
		var cancel context.CancelFunc
		p.ctx, cancel = context.WithDeadline(p.ctx, p.deadline)
		cleanups = append(cleanups, cancel)
	}
	return p, func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
}

// ucProcessRunAll runs every selected property in children, then reports
// each property in a subtest of its own, in the order of their identities.
// A property whose selection by -run and -skip ucProcessMayRun can read
// starts with the others; any other starts when go test reaches its subtest.
func ucProcessRunAll(t *testing.T, selected []string, units []ucProcessUnit) {
	plan, done := ucNewProcessPlan(t)
	defer done()
	units = slices.Clone(units)
	slices.SortFunc(units, func(a, b ucProcessUnit) int { return strings.Compare(a.ID, b.ID) })
	var wait ucsync.WaitGroup
	results := map[string][]ucProcessShard{}
	for _, u := range units {
		if len(selected) > 0 && !slices.Contains(selected, u.ID) {
			continue
		}
		if known, runs := ucProcessMayRun(u.ID); known && runs {
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
		go func(index int) {
			defer wait.Done()
			release, err := p.pool.acquire(p.ctx)
			if err != nil {
				s.err = fmt.Errorf("no process token before the deadline: %w", err)
				return
			}
			defer release()
			s.output, s.err = p.run(u.ID, string(ids), index, s)
		}(i)
	}
	return shards
}

// run runs one child in a temporary directory and a process group of its
// own. When the run's deadline comes first, the group gets SIGQUIT, so that
// the child prints its goroutines, and is killed ten seconds later; once
// the child has ended the group is killed and the directory removed. The
// child's test log is replayed here first.
func (p *ucProcessPlan) run(id, ids string, index int, s *ucProcessShard) ([]byte, error) {
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
	if !p.deadline.IsZero() {
		// The child's own timeout comes first and prints its goroutines.
		left := time.Until(p.deadline)
		args = append(args, "-test.timeout="+(left-min(5*time.Second, left/20)).String())
	}
	log := ucfilepath.Join(dir, "test.log")
	if p.logged {
		args = append(args, "-test.testlogfile="+log)
	}
	cmd := ucexec.CommandContext(p.ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), "UNICODE_CONTRACT_CHILD=1", "UNICODE_CONTRACT_IDS="+ids, "UNICODE_CONTRACT_SHARD="+strconv.Itoa(index),
		"UNICODE_CONTRACT_TEMP="+temp, "TMPDIR="+temp, "TMP="+temp, "TEMP="+temp)
	if p.cache != "" {
		cmd.Env = append(cmd.Env, "UNICODE_CACHE="+p.cache)
	}
	grouped := ucOwnProcessGroup(cmd)
	cmd.Cancel = func() error { return ucSignalGroup(cmd.Process, grouped, ucsyscall.SIGQUIT) }
	cmd.WaitDelay = 10 * time.Second
	var output ucbytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err = cmd.Run()
	if cmd.Process != nil {
		_ = ucSignalGroup(cmd.Process, grouped, os.Kill)
	}
	if p.logged {
		data, readErr := os.ReadFile(log)
		if readErr != nil && err == nil {
			err = fmt.Errorf("the child wrote no test log: %w", readErr)
		}
		defer ucReplayTestLog(string(data))
	}
	return output.Bytes(), err
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

// ucOwnProcessGroup asks for cmd to start in a process group of its own,
// where the platform has them, and tells whether it did.
func ucOwnProcessGroup(cmd *ucexec.Cmd) bool {
	attr := &ucsyscall.SysProcAttr{}
	field := reflect.ValueOf(attr).Elem().FieldByName("Setpgid")
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false
	}
	field.SetBool(true)
	cmd.SysProcAttr = attr
	return true
}

// ucSignalGroup sends sig to the process group a child leads, or to the
// child alone where it has no group of its own.
func ucSignalGroup(child *os.Process, grouped bool, sig os.Signal) error {
	if grouped {
		if group, err := os.FindProcess(-child.Pid); err == nil {
			if err := group.Signal(sig); err == nil {
				return nil
			}
		}
	}
	if err := child.Signal(sig); err != nil && sig != os.Kill {
		return child.Kill()
	}
	return nil
}

// ucProcessPool is a pool of tokens shared by every test binary of the user
// on this machine: a token is a file slot-<i> created exclusively in dir,
// holding its owner's process identifier, and removed on release. A slot
// whose owner no longer runs is taken back. Within one process at most size
// goroutines look for a token, and a goroutine that finds none waits a
// little longer each time, up to a fifth of a second.
type ucProcessPool struct {
	dir   string
	size  int
	local chan struct{}
}

func ucNewProcessPool() ucProcessPool {
	size := max(1, ucruntime.GOMAXPROCS(0))
	if n, err := strconv.Atoi(os.Getenv("UNICODE_TEST_JOBS")); err == nil && n >= 1 {
		size = n
	}
	return ucProcessPool{dir: ucfilepath.Join(os.TempDir(), "unicode-test-slots-"+strconv.Itoa(os.Getuid())), size: size, local: make(chan struct{}, size)}
}

// acquire waits for a free token until ctx ends, and returns its release.
func (pool ucProcessPool) acquire(ctx context.Context) (func(), error) {
	select {
	case pool.local <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release, err := pool.acquireSlot(ctx)
	if err != nil {
		<-pool.local
		return nil, err
	}
	return func() { release(); <-pool.local }, nil
}

func (pool ucProcessPool) acquireSlot(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(pool.dir, 0o700); err != nil {
		return nil, err
	}
	wait, lastCheck := 5*time.Millisecond, time.Time{}
	for {
		reclaim := time.Since(lastCheck) > time.Second
		if reclaim {
			lastCheck = time.Now()
		}
		for i := 0; i < pool.size; i++ {
			name := ucfilepath.Join(pool.dir, "slot-"+strconv.Itoa(i))
			f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err == nil {
				_, writeErr := fmt.Fprintf(f, "%d\n", os.Getpid())
				if closeErr := f.Close(); writeErr != nil || closeErr != nil {
					_ = os.Remove(name)
					return nil, errors.Join(writeErr, closeErr)
				}
				return func() { _ = os.Remove(name) }, nil
			}
			if !errors.Is(err, ucfs.ErrExist) {
				return nil, err
			}
			if reclaim && ucSlotAbandoned(name) {
				_ = os.Remove(name)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		wait = min(2*wait, 200*time.Millisecond)
	}
}

// ucSlotAbandoned tells whether the process that holds the slot has ended.
func ucSlotAbandoned(name string) bool {
	data, err := os.ReadFile(name)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		// A slot being written has no identifier yet.
		info, statErr := os.Stat(name)
		return statErr == nil && time.Since(info.ModTime()) > time.Minute
	}
	owner, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	if ucruntime.GOOS == "windows" {
		return false
	}
	err = owner.Signal(ucsyscall.Signal(0))
	return err != nil && !errors.Is(err, ucsyscall.EPERM)
}

// ucProcessMayRun tells whether -test.run and -test.skip let the subtest id
// run, when it can read them: patterns without brackets, parentheses,
// alternatives or escaped slashes split into levels at every slash, as go
// test splits them. known is false for any other pattern.
func ucProcessMayRun(id string) (known, runs bool) {
	levels := func(name string) ([]*ucregexp.Regexp, bool) {
		f := ucflag.Lookup(name)
		if f == nil || f.Value.String() == "" {
			return nil, true
		}
		if strings.ContainsAny(f.Value.String(), "[(|") || strings.Contains(f.Value.String(), "\\/") {
			return nil, false
		}
		var out []*ucregexp.Regexp
		for _, part := range strings.Split(f.Value.String(), "/") {
			re, err := ucregexp.Compile(part)
			if err != nil {
				return nil, false
			}
			out = append(out, re)
		}
		return out, true
	}
	run, runKnown := levels("test.run")
	skip, skipKnown := levels("test.skip")
	if !runKnown || !skipKnown {
		return false, false
	}
	if len(run) >= 2 && !run[1].MatchString(id) {
		return true, false
	}
	if len(skip) == 2 && skip[0].MatchString("TestUnicodeContracts") && skip[1].MatchString(id) {
		return true, false
	}
	return true, true
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

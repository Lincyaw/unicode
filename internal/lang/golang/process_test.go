package golang

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// The split properties run the generated driver of a one-property module:
// domain.sign calls classify(n) for a generated int n and ensures its sign.
// With UNICODE_SPLIT_FIXTURE_FAIL set, classify answers 2 for every odd n,
// so about half the cases fail and rapid shrinks each failure to n = 1 or
// n = -1; a replay from a wrong seed draws other inputs.

const (
	splitProperty = "domain.sign"
	splitFailEnv  = "UNICODE_SPLIT_FIXTURE_FAIL"
)

var (
	splitProcessLine = regexp.MustCompile(`process (\d+) of (\d+) \((\d+) cases, -rapid\.seed=(\d+)\)`)
	splitFailedLine  = regexp.MustCompile(regexp.QuoteMeta(splitProperty) + `: process (\d+) of (\d+) \((\d+) cases, -rapid\.seed=(\d+)\) failed`)
	splitPassedLine  = regexp.MustCompile(`\[rapid\] OK, passed (\d+) tests`)
	splitReproLine   = regexp.MustCompile(`To reproduce, specify -run=\S+ -rapid\.seed=(\d+)`)
	splitShrunkLine  = regexp.MustCompile(`\[rapid\] draw n: (-?\d+)`)
)

//uc:checks lang.golang.split-runs-cover-the-same-cases
func checkSplitRunsCoverTheSameCases(t rapid.TB, checks int) {
	binary, remove := splitFixture(t)
	defer remove()
	split, err := runSplitFixture(binary, checks, false, false)
	if err != nil {
		t.Fatalf("the split run of %d passing cases failed: %v\n%s", checks, err, split)
	}
	processes := splitProcessLine.FindAllStringSubmatch(split, -1)
	want := min(ucProcessShards, checks)
	if len(processes) != want {
		t.Fatalf("%d cases ran in %d processes, not %d:\n%s", checks, len(processes), want, split)
	}
	seeds := map[string]bool{}
	given := 0
	for i, p := range processes {
		if p[1] != strconv.Itoa(i+1) || p[2] != strconv.Itoa(want) {
			t.Fatalf("process %s of %s is reported as the %d-th of %d:\n%s", p[1], p[2], i+1, want, split)
		}
		n, _ := strconv.Atoi(p[3])
		given += n
		seeds[p[4]] = true
	}
	if len(seeds) != want {
		t.Fatalf("the %d processes share seeds: %v\n%s", want, seeds, split)
	}
	if passed := passedCases(split); given != checks || passed != checks {
		t.Fatalf("the processes were given %d cases and passed %d, not the %d configured:\n%s", given, passed, checks, split)
	}
	whole, err := runSplitFixture(binary, checks, false, true)
	if err != nil {
		t.Fatalf("the unsplit run of %d passing cases failed: %v\n%s", checks, err, whole)
	}
	if processes := splitProcessLine.FindAllString(whole, -1); len(processes) != 0 || passedCases(whole) != checks {
		t.Fatalf("the unsplit run used %d child processes or did not pass all %d cases:\n%s", len(processes), checks, whole)
	}
}

//uc:checks lang.golang.split-runs-shrink-alike
func checkSplitRunsShrinkAlike(t rapid.TB, checks int) {
	binary, remove := splitFixture(t)
	defer remove()
	for _, whole := range []bool{false, true} {
		output, err := runSplitFixture(binary, checks, true, whole)
		if err == nil {
			t.Fatalf("a run of %d cases, about half of them failing, passed (unsplit: %t):\n%s", checks, whole, output)
		}
		shrunk := shrunkInputs(output)
		if len(shrunk) == 0 || slices.ContainsFunc(shrunk, func(n string) bool { return n != "1" && n != "-1" }) {
			t.Fatalf("the run (unsplit: %t) did not shrink every failure to n = 1 or n = -1: %v\n%s", whole, shrunk, output)
		}
		if failed := failedProcesses(output); !whole && len(failed) != len(shrunk) {
			t.Fatalf("the split run reported %d failed processes and %d shrunk failures:\n%s", len(failed), len(shrunk), output)
		}
	}
}

//uc:checks lang.golang.split-failures-name-their-process
func checkSplitFailuresNameTheirProcess(t rapid.TB, checks int) {
	binary, remove := splitFixture(t)
	defer remove()
	split, err := runSplitFixture(binary, checks, true, false)
	failed := failedProcesses(split)
	if err == nil || len(failed) == 0 {
		t.Fatalf("a split run of %d cases, about half of them failing, reported no failed process naming %s, its place and its seed (error %v):\n%s", checks, splitProperty, err, split)
	}
	want, places := strconv.Itoa(min(ucProcessShards, checks)), map[string]bool{}
	for _, f := range failed {
		if f.of != want || places[f.place] {
			t.Fatalf("failed process %s of %s is not one of %s distinct places:\n%s", f.place, f.of, want, split)
		}
		places[f.place] = true
		shrunk := shrunkInputs(f.text)
		if len(shrunk) != 1 {
			t.Fatalf("failed process %s reports %d shrunk failures:\n%s", f.place, len(shrunk), f.text)
		}
		replayed, err := replaySplitFixture(binary, f.seed, f.count)
		if err == nil || !slices.Equal(shrunkInputs(replayed), shrunk) {
			t.Fatalf("running %d cases from the seed %s of process %s did not fail with n = %s (error %v):\n%s", f.count, f.seed, f.place, shrunk[0], err, replayed)
		}
	}
}

//uc:checks lang.golang.split-failures-replay-by-seed
func checkSplitFailuresReplayBySeed(t rapid.TB, checks int) {
	binary, remove := splitFixture(t)
	defer remove()
	split, err := runSplitFixture(binary, checks, true, false)
	failed := failedProcesses(split)
	if err == nil || len(failed) == 0 {
		t.Fatalf("a split run of %d cases, about half of them failing, reported no failed process (error %v):\n%s", checks, err, split)
	}
	for _, f := range failed {
		repro := splitReproLine.FindStringSubmatch(f.text)
		shrunk := shrunkInputs(f.text)
		if repro == nil || len(shrunk) != 1 {
			t.Fatalf("failed process %s gives no seed to reproduce its one failure with:\n%s", f.place, f.text)
		}
		replayed, err := replaySplitFixture(binary, repro[1], 1)
		if err == nil || splitProcessLine.MatchString(replayed) || !slices.Equal(shrunkInputs(replayed), shrunk) {
			t.Fatalf("replaying -rapid.seed=%s for one case did not fail in one process with n = %s (error %v):\n%s", repro[1], shrunk[0], err, replayed)
		}
	}
}

// failedProcess is one failed child as the split run reports it, with its
// output up to the next report.
type failedProcess struct {
	place, of, seed string
	count           int
	text            string
}

func failedProcesses(output string) []failedProcess {
	var out []failedProcess
	matches := splitFailedLine.FindAllStringSubmatchIndex(output, -1)
	for i, m := range matches {
		end := len(output)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		count, _ := strconv.Atoi(output[m[6]:m[7]])
		out = append(out, failedProcess{place: output[m[2]:m[3]], of: output[m[4]:m[5]], count: count, seed: output[m[8]:m[9]], text: output[m[0]:end]})
	}
	return out
}

// replaySplitFixture runs the failing property in one process from seed for
// count cases.
func replaySplitFixture(binary, seed string, count int) (string, error) {
	cmd := exec.Command(binary, "-test.run=^TestUnicodeContracts$/^"+regexp.QuoteMeta(splitProperty)+"$", "-test.count=1", "-rapid.nofailfile", "-rapid.seed="+seed, "-rapid.checks="+strconv.Itoa(count))
	cmd.Env = splitFixtureEnv(true, false)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func passedCases(output string) int {
	total := 0
	for _, p := range splitPassedLine.FindAllStringSubmatch(output, -1) {
		n, _ := strconv.Atoi(p[1])
		total += n
	}
	return total
}

func shrunkInputs(output string) []string {
	var out []string
	for _, m := range splitShrunkLine.FindAllStringSubmatch(output, -1) {
		out = append(out, m[1])
	}
	return out
}

// runSplitFixture runs the fixture's driver with rapid's count set to checks,
// failing every case or not, in child processes or, when whole, in one.
func runSplitFixture(binary string, checks int, failing, whole bool) (string, error) {
	cmd := exec.Command(binary, "-test.run=^TestUnicodeContracts$", "-test.v", "-test.count=1", "-rapid.nofailfile", "-rapid.checks="+strconv.Itoa(checks))
	cmd.Env = splitFixtureEnv(failing, whole)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// splitFixtureEnv is this process's environment without the driver's own
// settings, so that the fixture decides for itself whether to split.
func splitFixtureEnv(failing, whole bool) []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "UNICODE_CONTRACT_") && !strings.HasPrefix(entry, splitFailEnv+"=") {
			env = append(env, entry)
		}
	}
	if failing {
		env = append(env, splitFailEnv+"=1")
	}
	if whole {
		env = append(env, "UNICODE_CONTRACT_CHILD=1")
	}
	return env
}

// A child process of this package's driver has a temporary directory of its
// own that its parent removes (UNICODE_CONTRACT_TEMP), so it builds the
// fixture there once; any other process builds it for each case and removes
// it after.
var sharedSplitFixture struct {
	sync.Once
	binary string
	err    error
}

func splitFixture(t rapid.TB) (string, func()) {
	if private := os.Getenv("UNICODE_CONTRACT_TEMP"); private == "" || private != os.TempDir() {
		dir, err := os.MkdirTemp("", "unicode-split-fixture-")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		binary, err := buildSplitFixture(dir)
		if err != nil {
			_ = os.RemoveAll(dir)
			t.Fatalf("setup failed: cannot build the split fixture: %v", err)
		}
		return binary, func() { _ = os.RemoveAll(dir) }
	}
	sharedSplitFixture.Do(func() {
		var dir string
		if dir, sharedSplitFixture.err = os.MkdirTemp("", "unicode-split-fixture-"); sharedSplitFixture.err == nil {
			sharedSplitFixture.binary, sharedSplitFixture.err = buildSplitFixture(dir)
		}
	})
	if sharedSplitFixture.err != nil {
		t.Fatalf("setup failed: cannot build the split fixture: %v", sharedSplitFixture.err)
	}
	return sharedSplitFixture.binary, func() {}
}

// buildSplitFixture writes the module under root, generates its driver with
// this backend and compiles its test binary.
func buildSplitFixture(root string) (string, error) {
	sum, err := os.ReadFile(filepath.Join("..", "..", "..", "go.sum"))
	if err != nil {
		return "", err
	}
	files := map[string]string{
		"go.mod":           "module example.test/split\n\ngo 1.26.2\n\nrequire pgregory.net/rapid v1.3.0\n",
		"go.sum":           string(sum),
		"design/domain.uc": "component domain \"Split fixture\" {\n  paths \"src/**\"\n}\n",
		"src/domain.go": "package split\n\nimport \"os\"\n\n//uc:realizes domain.classify\nfunc Classify(n int) int {\n" +
			"\tif os.Getenv(\"" + splitFailEnv + "\") != \"\" && n%2 != 0 {\n\t\treturn 2\n\t}\n" +
			"\tif n < 0 {\n\t\treturn -1\n\t}\n\tif n == 0 {\n\t\treturn 0\n\t}\n\treturn 1\n}\n",
	}
	for name, content := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	g := graph.NewGraph()
	at := diag.Position{Path: "design/domain.uc", Line: 1, Column: 1}
	for _, n := range []graph.Node{
		{Id: "domain", Kind: graph.NodeKindContainer, Form: "component", Attrs: map[string][]string{"paths": {"src/**"}}},
		{Id: "domain.classify", Kind: graph.NodeKindOp, Form: "op", Attrs: map[string][]string{"signature": {"(n: int) -> int"}, "pure": {"true"}}},
		{Id: splitProperty, Kind: graph.NodeKindProperty, Form: "postcondition", Attrs: map[string][]string{"symbols": {"n: int"}, "call": {"classify(n)"}, "ensure": {"n < 0 implies result == -1", "n == 0 implies result == 0", "n > 0 implies result == 1"}}},
	} {
		n.Layer, n.Source, n.Origin = graph.LayerDesign, graph.SourceDeclared, at
		if err := g.AddNode(n); err != nil {
			return "", err
		}
	}
	for _, e := range []graph.Edge{
		{Kind: graph.EdgeKindContains, From: "domain", To: "domain.classify"},
		{Kind: graph.EdgeKindContains, From: "domain", To: splitProperty},
		{Kind: graph.EdgeKindRefers, From: splitProperty, To: "domain.classify"},
	} {
		e.Source = graph.SourceDeclared
		if err := g.AddEdge(e); err != nil {
			return "", err
		}
	}
	extracted, err := NewBackend().ExtractForGeneration(root, []string{"src"})
	if err != nil {
		return "", err
	}
	if err := g.AddFragment(extracted.Fragment); err != nil {
		return "", err
	}
	if ds := g.Derive(); len(ds) > 0 {
		return "", diagnosticsError(ds)
	}
	generated := NewBackend().Generate(lang.GenPlan{Root: root, Roots: []string{"src"}, Graph: &g})
	if len(generated.Diagnostics) > 0 {
		return "", diagnosticsError(generated.Diagnostics)
	}
	for _, edit := range generated.Edits {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(edit.Path)), edit.Content, 0o644); err != nil {
			return "", err
		}
	}
	binary := filepath.Join(root, "split.test")
	build := exec.Command("go", "test", "-c", "-o", binary, "./src")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return "", &buildError{err, output}
	}
	return binary, nil
}

type buildError struct {
	err    error
	output []byte
}

func (e *buildError) Error() string { return e.err.Error() + ": " + string(e.output) }

type diagnosticsError []diag.Diagnostic

func (ds diagnosticsError) Error() string {
	var lines []string
	for _, d := range ds {
		lines = append(lines, d.Code+": "+d.Message)
	}
	return strings.Join(lines, "; ")
}

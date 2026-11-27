package evidence

import (
	"bytes"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/syntax"
)

// The predicate observes actual filesystem bytes and a stateful execution
// boundary. Pure selection and attachment relations are generated from ensure.
//
//uc:checks evidence.recordings-are-replayable
func checkRecordingsAreReplayable(t rapid.TB, sample Sample) {
	root, err := os.MkdirTemp("", "unicode-evidence-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
	}()
	g := recordingGraph(t, sample)
	sentinel := []byte("designer-owned " + strconv.Itoa(sample.Limit))
	writeFixture(t, root, "design/domain.uc", sentinel)
	before := fileTree(t, root)
	cases := sample.Cases
	unsupported := []string{}
	unsupportedCases := []int{}
	var printable func(lang.Value, string) lang.Value
	printable = func(value lang.Value, path string) lang.Value {
		switch value.Kind {
		case "stringBytes":
			unsupported = append(unsupported, path)
			return lang.Value{Kind: "string", Text: value.Text}
		case "bytes":
			raw, err := base64.StdEncoding.DecodeString(value.Text)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range raw {
				if b < 32 && b != '\n' && b != '\t' || b >= 127 {
					unsupported = append(unsupported, path)
					return lang.Value{Kind: "bytes", Text: base64.StdEncoding.EncodeToString([]byte(value.Text))}
				}
			}
		}
		value.Items = slices.Clone(value.Items)
		for i := range value.Items {
			value.Items[i] = printable(value.Items[i], path+".item")
		}
		value.Entries = slices.Clone(value.Entries)
		for i := range value.Entries {
			value.Entries[i].Key = printable(value.Entries[i].Key, path+".key")
			value.Entries[i].Value = printable(value.Entries[i].Value, path+".value")
		}
		return value
	}
	companions := slices.Clone(cases)
	for i, source := range cases {
		beforeUnsupported := len(unsupported)
		input, err := NormalizeInput(source.Input)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range input.Values {
			input.Values[name] = printable(value, name)
		}
		if len(unsupported) > beforeUnsupported {
			unsupportedCases = append(unsupportedCases, i)
		}
		companions[i].Input, err = NormalizeInput(input)
		if err != nil {
			t.Fatal(err)
		}
		companions[i].InputHash = hash(companions[i].Input.Values)
		if companions[i].Form != "declared" {
			companions[i].Id = companions[i].Check + ".r-" + companions[i].InputHash[:8]
		}
	}
	plan := Recordings(root, g, cases)
	if len(unsupportedCases) > 0 {
		// Declared and redundant coverage cases need not be written. Make one
		// raw input a mandatory counterexample independently of selection, so
		// the encoding boundary is exercised even when the original is omitted.
		required := cases[unsupportedCases[0]]
		required.Form, required.Reason, required.Passed = "recorded", "counterexample", false
		required.Id = required.Check + ".r-" + required.InputHash[:8]
		mandatory := Recordings(root, g, []lang.CaseResult{required})
		requireProblems(t, mandatory.Diagnostics)
		for _, rejected := range []Recording{mandatory, plan} {
			if len(rejected.Diagnostics) == 0 {
				continue
			}
			if len(rejected.Edits) != 0 || len(rejected.Removals) != 0 {
				t.Fatal("unrepresentable bytes produced a writable plan")
			}
			for _, problem := range rejected.Diagnostics {
				if problem.Code != "evidence.unreplayable-input" {
					t.Fatalf("unexpected recording classification: %+v", problem)
				}
			}
		}
		if !reflect.DeepEqual(before, fileTree(t, root)) {
			t.Fatal("unrepresentable input changed files")
		}
		if len(plan.Diagnostics) > 0 {
			cases = companions
			plan = Recordings(root, g, cases)
		}
	}
	if len(plan.Diagnostics) != 0 {
		t.Fatalf("recording rejected complete model inputs: %+v", plan.Diagnostics)
	}
	if after := fileTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("planning changed files: before=%v after=%v", before, after)
	}
	// Each written case must recover one complete original input, including raw
	// non-UTF8 string bytes and user-defined codec representations.
	original := map[string][]byte{}
	for _, source := range cases {
		original[source.InputHash] = canonical(source.Input.Values)
	}
	for _, edit := range plan.Edits {
		parsed := syntax.Parse(edit.Path, syntax.FileKindCorpus, edit.Content)
		if len(parsed.Diagnostics) != 0 {
			t.Fatal(parsed.Diagnostics)
		}
		for _, declaration := range parsed.File.Decls {
			candidate, ok := declaration.(*syntax.RecordedCase)
			if !ok {
				continue
			}
			recovered, err := inputFromAssignments(g, "domain.rule", candidate.Lines)
			if err != nil {
				t.Fatal(err)
			}
			expected, found := original[hash(recovered.Values)]
			if !found || !bytes.Equal(expected, canonical(recovered.Values)) {
				t.Fatalf("corpus lost input state: %s", canonical(recovered.Values))
			}
		}
	}
	if problems := ApplyRecordings(root, plan); len(problems) != 0 {
		t.Fatal(problems)
	}
	applied := fileTree(t, root)
	again := Recordings(root, g, cases)
	if len(again.Diagnostics) != 0 || len(again.Edits) != 0 || len(again.Removals) != 0 {
		t.Fatalf("recording is not idempotent: %+v", again)
	}
	if data := readFixture(t, root, "design/domain.uc"); !bytes.Equal(data, sentinel) {
		t.Fatal("recording changed designer-owned bytes")
	}
	// An invalid later action must prevent publication of an earlier valid edit.
	bad := Recording{Edits: []lang.Edit{{Path: "corpus/a.uc", Content: []byte("corpus a\n")}, {Path: "../outside.uc", Content: []byte("corpus outside\n")}}}
	requireProblems(t, ApplyRecordings(root, bad))
	if after := fileTree(t, root); !reflect.DeepEqual(after, applied) {
		t.Fatal("invalid plan partially changed the filesystem")
	}
	// Valid orphan namespaces are removed; malformed ownership never is.
	writeFixture(t, root, "corpus/removed.uc", []byte("corpus removed\n"))
	orphan := Recordings(root, g, cases)
	if len(orphan.Diagnostics) != 0 || !slices.Contains(orphan.Removals, "corpus/removed.uc") {
		t.Fatalf("orphan not planned: %+v", orphan)
	}
	if problems := ApplyRecordings(root, orphan); len(problems) != 0 {
		t.Fatal(problems)
	}
	if _, err := os.Lstat(filepath.Join(root, "corpus/removed.uc")); !os.IsNotExist(err) {
		t.Fatalf("orphan remained: %v", err)
	}
	// A former check can remain in the graph as another kind or layer. Its
	// canonical namespace is still orphaned; ownership checks remain strict.
	obsolete := graph.Node{Id: "removed", Kind: graph.NodeKindOp, Layer: graph.LayerDesign}
	switch sample.Limit % 3 {
	case 1:
		obsolete.Kind = graph.NodeKindType
	case 2:
		obsolete.Kind, obsolete.Layer = graph.NodeKindProperty, graph.LayerCode
	}
	if err := g.AddNode(obsolete); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "corpus/removed.uc", []byte("corpus removed\n"))
	retyped := Recordings(root, g, cases)
	if len(retyped.Diagnostics) != 0 || !slices.Contains(retyped.Removals, "corpus/removed.uc") {
		t.Fatalf("former check namespace not removed: %+v", retyped)
	}
	if problems := ApplyRecordings(root, retyped); len(problems) != 0 {
		t.Fatal(problems)
	}
	if _, err := os.Lstat(filepath.Join(root, "corpus/removed.uc")); !os.IsNotExist(err) {
		t.Fatalf("former check namespace remained: %v", err)
	}
	writeFixture(t, root, "corpus/broken.uc", []byte("corpus other\n"))
	invalidBefore := fileTree(t, root)
	invalid := Recordings(root, g, cases)
	requireProblems(t, invalid.Diagnostics)
	if len(invalid.Edits) != 0 || len(invalid.Removals) != 0 {
		t.Fatal("invalid namespace produced a writable plan")
	}
	requireProblems(t, ApplyRecordings(root, Recording{Removals: []string{"corpus/broken.uc"}}))
	if !reflect.DeepEqual(invalidBefore, fileTree(t, root)) {
		t.Fatal("invalid ownership changed bytes")
	}
	removeFixture(t, root, "corpus/broken.uc")
	// Symlink destinations are never followed by either the planner or publisher.
	link := filepath.Join(root, "corpus/link.uc")
	if err := os.Symlink(filepath.Join(root, "design/domain.uc"), link); err != nil {
		t.Fatal(err)
	}
	requireProblems(t, Recordings(root, g, cases).Diagnostics)
	requireProblems(t, ApplyRecordings(root, Recording{Removals: []string{"corpus/link.uc"}}))
	if data := readFixture(t, root, "design/domain.uc"); !bytes.Equal(data, sentinel) {
		t.Fatal("symlink target changed")
	}
	removeFixture(t, root, "corpus/link.uc")

	// Compare the full persisted report against the frozen source, not a success
	// Boolean. Repeat-save must preserve immutable history and latest bytes.
	result := lang.RunResult{Id: "domain.rule", Passed: true, Cases: sample.Cases}
	for _, candidate := range sample.Cases {
		if candidate.Rejected {
			result.Rejected++
		} else {
			result.Accepted++
		}
		if !candidate.Passed {
			result.Passed = false
		}
	}
	report := lang.RunReport{Version: 1, Snapshot: graph.InputDigest(g), At: time.Unix(int64(1000+sample.Limit), 0).UTC().Format(time.RFC3339Nano), Mode: "explore", Results: []lang.RunResult{result}}
	expected, err := normalizeReport(report, false)
	if err != nil {
		t.Fatal(err)
	}
	if problems := Save(root, report); len(problems) != 0 {
		t.Fatal(problems)
	}
	storedBefore := fileTree(t, root)
	if problems := Save(root, report); len(problems) != 0 {
		t.Fatal(problems)
	}
	if !reflect.DeepEqual(storedBefore, fileTree(t, root)) {
		t.Fatal("repeat save changed report bytes or modes")
	}
	loaded := Read(root)
	if len(loaded.Diagnostics) != 0 || len(loaded.Reports) != 1 || !bytes.Equal(canonical(loaded.Reports[0]), canonical(expected)) {
		t.Fatalf("stored report differs from actual inputs: %+v", loaded)
	}
	second := report
	second.At = time.Unix(int64(1001+sample.Limit), 0).UTC().Format(time.RFC3339Nano)
	if problems := Save(root, second); len(problems) != 0 {
		t.Fatal(problems)
	}
	loaded = Read(root)
	if len(loaded.Diagnostics) != 0 || len(loaded.Reports) != 2 {
		t.Fatalf("save lost immutable history: %+v", loaded)
	}
	writeFixture(t, root, ".unicode/runs/broken.json", []byte("{\"version\":1,\"version\":1}"))
	corrupted := fileTree(t, root)
	requireProblems(t, Read(root).Diagnostics)
	requireProblems(t, Save(root, report))
	if !reflect.DeepEqual(corrupted, fileTree(t, root)) {
		t.Fatal("corrupt history was overwritten")
	}
	removeFixture(t, root, ".unicode/runs/broken.json")
	removeFixture(t, root, ".unicode/results.json")
	if err := os.Symlink(filepath.Join(root, "design/domain.uc"), filepath.Join(root, ".unicode/results.json")); err != nil {
		t.Fatal(err)
	}
	requireProblems(t, Read(root).Diagnostics)
	requireProblems(t, Save(root, report))
	if !bytes.Equal(readFixture(t, root, "design/domain.uc"), sentinel) {
		t.Fatal("run storage followed a symlink")
	}

	// A real execution boundary computes its outcome from a generated integer.
	// Rejected values and environment failures cannot replace the known failure.
	threshold := 1 + sample.Limit%5
	start := threshold + len(sample.Cases) + len(sample.Required)
	input := lang.Input{Values: map[string]lang.Value{"n": intValue(start)}}
	frozen, err := NormalizeInput(input)
	if err != nil {
		t.Fatal(err)
	}
	initial := lang.CaseResult{Check: "domain.rule", Form: "recorded", Reason: "counterexample", Input: frozen, InputHash: hash(frozen.Values), Passed: false}
	initial.Id = initial.Check + ".r-" + initial.InputHash[:8]
	runner := &thresholdRunner{threshold: threshold}
	shrunk := Shrink(initial, runner, time.Second)
	if len(shrunk.Diagnostics) != 0 {
		t.Fatal(shrunk.Diagnostics)
	}
	if got := shrunk.Result.Input.Values["n"]; got.Kind != "int" || got.Text != strconv.Itoa(threshold) || shrunk.Result.Passed || shrunk.Result.Rejected {
		t.Fatalf("minimized input no longer reproduces boundary failure: %+v", shrunk)
	}
	if runner.evaluations == 0 || runner.rejected == 0 {
		t.Fatal("shrinker did not exercise the execution boundary")
	}
	if !bytes.Equal(canonical(initial.Input), canonical(frozen)) {
		t.Fatal("candidate execution mutated the original input")
	}
	for _, mode := range []string{"exhausted", "environment"} {
		runner := &thresholdRunner{threshold: threshold, mode: mode}
		stopped := Shrink(initial, runner, time.Second)
		if !bytes.Equal(canonical(stopped.Result.Input), canonical(initial.Input)) {
			t.Fatal("execution interruption replaced known failure")
		}
		if mode == "environment" {
			requireProblems(t, stopped.Diagnostics)
		} else if len(stopped.Diagnostics) != 0 {
			t.Fatal(stopped.Diagnostics)
		}
	}
}

type thresholdRunner struct {
	threshold             int
	mode                  string
	evaluations, rejected int
}

func (r *thresholdRunner) Evaluate(input lang.Input) lang.CandidateEvaluation {
	r.evaluations++
	if r.mode == "exhausted" {
		return lang.CandidateEvaluation{Exhausted: true}
	}
	if r.mode == "environment" {
		return lang.CandidateEvaluation{Diagnostics: []diag.Diagnostic{sampleDiagnostic("lang.golang.observation-failed")}}
	}
	result := lang.CaseResult{Check: "domain.rule", Form: "recorded", Input: input}
	value := input.Values["n"]
	if value.Kind != "int" {
		result.Rejected = true
		r.rejected++
		return lang.CandidateEvaluation{Result: &result}
	}
	n, err := strconv.Atoi(value.Text)
	if err != nil {
		result.Rejected = true
		r.rejected++
	} else {
		result.Passed = n < r.threshold
	}
	// Mutating this runner-owned copy after observation must not change the
	// initial frozen input or any previously retained execution.
	result.Input, _ = NormalizeInput(input)
	input.Values["n"] = intValue(-r.threshold)
	return lang.CandidateEvaluation{Result: &result}
}

type fixtureFile struct {
	Data string
	Mode fs.FileMode
	Link string
}

func fileTree(t rapid.TB, root string) map[string]fixtureFile {
	out := map[string]fixtureFile{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := fixtureFile{Mode: info.Mode()}
		if info.Mode()&os.ModeSymlink != 0 {
			item.Link, err = os.Readlink(path)
		} else if !entry.IsDir() {
			var data []byte
			data, err = os.ReadFile(path)
			item.Data = string(data)
		}
		out[filepath.ToSlash(relative)] = item
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func writeFixture(t rapid.TB, root, path string, data []byte) {
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, data, 0o640); err != nil {
		t.Fatal(err)
	}
}
func readFixture(t rapid.TB, root, path string) []byte {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func removeFixture(t rapid.TB, root, path string) {
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(path))); err != nil {
		t.Fatal(err)
	}
}
func requireProblems(t rapid.TB, problems []diag.Diagnostic) {
	if len(problems) == 0 {
		t.Fatal("unsafe boundary unexpectedly succeeded")
	}
	for _, problem := range problems {
		if problem.Code == "" || problem.Position.Path == "" || problem.Position.Line < 1 || problem.Position.Column < 1 || problem.Message == "" || problem.Fix == "" {
			t.Fatalf("incomplete diagnostic: %+v", problem)
		}
	}
}

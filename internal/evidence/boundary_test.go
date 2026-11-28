package evidence

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/repo"
)

// CorpusSample is a graph, the results of cases of its check domain.rule,
// whether every input can be written to a recording file, a repository
// tree, and the state of its corpus.
//
//uc:realizes evidence.CorpusSample
type CorpusSample struct {
	Graph     *graph.Graph
	Cases     []lang.CaseResult
	Printable bool
	Tree      repo.Tree
	Corpus    string
}

// StoreSample is the reports saved before, the report to save, and how the
// store is damaged.
//
//uc:realizes evidence.StoreSample
type StoreSample struct {
	Earlier []lang.RunReport
	Report  lang.RunReport
	Damage  string
}

// ShrinkSample is a threshold, the counterexample to shrink, the mode of the
// candidate runner, the budget, and the smallest failing input value.
//
//uc:realizes evidence.ShrinkSample
type ShrinkSample struct {
	Threshold int
	Initial   lang.CaseResult
	Mode      string
	Budget    time.Duration
	Minimal   lang.Value
}

// RunnerLog is how often the candidate runner ran and rejected a candidate.
//
//uc:realizes evidence.RunnerLog
type RunnerLog struct {
	Evaluations int
	Rejected    int
}

// The shapes of sampleValue whose inputs a recording file cannot hold:
// encoded records and maps with non-UTF-8 strings, bytes with control bytes.
var unprintableShapes = []int{4, 5, 8}

// drawCases draws one to eight results of domain.rule with inputs of one shape.
func drawCases(t *rapid.T, shape int) []lang.CaseResult {
	var out []lang.CaseResult
	for index := range rapid.IntRange(1, 8).Draw(t, "cases") {
		input, err := NormalizeInput(lang.Input{Values: map[string]lang.Value{"payload": sampleValue(shape, rapid.IntRange(-100, 100).Draw(t, "value"))}})
		if err != nil {
			t.Fatal(err)
		}
		result := lang.CaseResult{Check: "domain.rule", Form: "recorded", Reason: "coverage", Input: input, InputHash: hash(input.Values), Passed: true,
			Coverage: []lang.Coverage{}, Trace: []lang.TraceEvent{}, Observations: []string{}, Differences: []string{}, Diagnostics: []diag.Diagnostic{}}
		result.Id = result.Check + ".r-" + result.InputHash[:8]
		if rapid.Bool().Draw(t, "declared") {
			result.Form, result.Reason, result.Id = "declared", "", fmt.Sprintf("domain.rule.declared%d", index)
		} else if rapid.Bool().Draw(t, "counterexample") {
			result.Passed, result.Reason = false, "counterexample"
		}
		out = append(out, result)
	}
	return out
}

// printableValue replaces a non-UTF-8 string by its Base64 text as a string,
// and bytes by the bytes of their Base64 text, which a recording file holds.
func printableValue(value lang.Value) lang.Value {
	switch value.Kind {
	case "stringBytes":
		return lang.Value{Kind: "string", Text: value.Text}
	case "bytes":
		return lang.Value{Kind: "bytes", Text: base64.StdEncoding.EncodeToString([]byte(value.Text))}
	}
	value.Items = slices.Clone(value.Items)
	for i := range value.Items {
		value.Items[i] = printableValue(value.Items[i])
	}
	value.Entries = slices.Clone(value.Entries)
	for i := range value.Entries {
		value.Entries[i] = lang.ValueEntry{Key: printableValue(value.Entries[i].Key), Value: printableValue(value.Entries[i].Value)}
	}
	return value
}

// printableCase is the case with printable values, its hash and recorded
// name following them.
func printableCase(t *rapid.T, c lang.CaseResult) lang.CaseResult {
	input := lang.Input{Values: map[string]lang.Value{}}
	for name, value := range c.Input.Values {
		input.Values[name] = printableValue(value)
	}
	normalized, err := NormalizeInput(input)
	if err != nil {
		t.Fatal(err)
	}
	c.Input, c.InputHash = normalized, hash(normalized.Values)
	if c.Form != "declared" {
		c.Id = c.Check + ".r-" + c.InputHash[:8]
	}
	return c
}

func treeFile(content string) repo.Entry {
	return repo.Entry{Kind: repo.EntryKindFile, Content: []byte(content), Mode: 0o644}
}

func treeDir() repo.Entry { return repo.Entry{Kind: repo.EntryKindDir, Mode: 0o755} }

//uc:generates evidence.recordings-roundtrip-inputs.sample
//uc:generates evidence.recordings-refuse-unreplayable-inputs.sample
//uc:generates evidence.recordings-retire-orphans.sample
//uc:generates evidence.recordings-refuse-foreign-corpus.sample
//uc:generates evidence.recordings-diagnostics-are-described.sample
//uc:generates evidence.recordings-are-idempotent.sample
//uc:generates evidence.apply-writes-the-plan.sample
//uc:generates evidence.apply-keeps-other-entries.sample
//uc:generates evidence.apply-is-all-or-nothing.sample
//uc:generates evidence.apply-diagnostics-are-described.sample
//uc:generates evidence.apply-refuses-foreign-removals.sample
func genCorpusSample() *rapid.Generator[CorpusSample] {
	return rapid.Custom(func(t *rapid.T) CorpusSample {
		shape := rapid.IntRange(0, 13).Draw(t, "shape")
		sample := CorpusSample{Cases: drawCases(t, shape), Printable: !slices.Contains(unprintableShapes, shape)}
		if !sample.Printable && rapid.Bool().Draw(t, "companions") {
			for i := range sample.Cases {
				sample.Cases[i] = printableCase(t, sample.Cases[i])
			}
			sample.Printable = true
		}
		if !sample.Printable {
			first := &sample.Cases[0]
			first.Form, first.Reason, first.Passed = "recorded", "counterexample", false
			first.Id = first.Check + ".r-" + first.InputHash[:8]
		}
		sample.Graph = corpusGraph(t, sample.Cases)
		sample.Tree = repo.Tree{Entries: map[string]repo.Entry{"design": treeDir(), "design/domain.uc": treeFile("designer-owned\n")}}
		sample.Corpus = rapid.SampledFrom([]string{"none", "orphan", "foreign", "link"}).Draw(t, "corpus")
		switch sample.Corpus {
		case "orphan":
			sample.Tree.Entries["corpus"] = treeDir()
			sample.Tree.Entries["corpus/removed.uc"] = treeFile("corpus removed\n")
			former := graph.Node{Id: "removed", Kind: graph.NodeKindOp, Layer: graph.LayerDesign}
			switch rapid.IntRange(0, 3).Draw(t, "former") {
			case 1:
				former.Kind = graph.NodeKindType
			case 2:
				former.Kind, former.Layer = graph.NodeKindProperty, graph.LayerCode
			case 3:
				former.Id = ""
			}
			if former.Id != "" {
				if err := sample.Graph.AddNode(former); err != nil {
					t.Fatal(err)
				}
			}
		case "foreign":
			sample.Tree.Entries["corpus"] = treeDir()
			sample.Tree.Entries["corpus/broken.uc"] = treeFile("corpus other\n")
		case "link":
			sample.Tree.Entries["corpus"] = treeDir()
			sample.Tree.Entries["corpus/link.uc"] = repo.Entry{Kind: repo.EntryKindLink, Target: "../design/domain.uc"}
		}
		return sample
	})
}

// corpusGraph is a graph whose property domain.rule has the symbol payload
// of the type of the cases' input.
func corpusGraph(t rapid.TB, cases []lang.CaseResult) *graph.Graph {
	g := attachmentGraph(t, 0, 3)
	nodes := slices.DeleteFunc(g.Nodes(), func(n graph.Node) bool { return n.Id == "domain.rule" })
	edges := slices.DeleteFunc(g.Edges(), func(e graph.Edge) bool { return e.From == "domain.rule" || e.To == "domain.rule" })
	g = fixtureBuild(t, nodes, edges)
	counter := 0
	typ := fixtureType(t, g, cases[0].Input.Values["payload"], &counter)
	if err := g.AddNode(graph.Node{Id: "domain.rule", Kind: graph.NodeKindProperty, Form: "postcondition", Layer: graph.LayerDesign, Attrs: map[string][]string{"symbols": {"payload: " + typ}}}); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindContains, From: "domain", To: "domain.rule"}); err != nil {
		t.Fatal(err)
	}
	return g
}

//uc:generates evidence.recordings-roundtrip-inputs.dir
//uc:generates evidence.recordings-refuse-unreplayable-inputs.dir
//uc:generates evidence.recordings-retire-orphans.dir
//uc:generates evidence.recordings-refuse-foreign-corpus.dir
//uc:generates evidence.recordings-diagnostics-are-described.dir
//uc:generates evidence.apply-writes-the-plan.dir
//uc:generates evidence.apply-keeps-other-entries.dir
//uc:generates evidence.apply-is-all-or-nothing.dir
//uc:generates evidence.apply-diagnostics-are-described.dir
//uc:generates evidence.apply-refuses-foreign-removals.dir
func deriveCorpusDir(sample CorpusSample) (string, error) { return repo.Materialize(sample.Tree) }

//uc:generates evidence.apply-writes-the-plan.plan
//uc:generates evidence.apply-keeps-other-entries.plan
func derivePlan(sample CorpusSample, dir string) Recording {
	return Recordings(dir, sample.Graph, sample.Cases)
}

// deriveWritten writes the sample's tree, then the plan for it.
//
//uc:generates evidence.recordings-are-idempotent.written
func deriveWritten(sample CorpusSample) (string, error) {
	dir, err := repo.Materialize(sample.Tree)
	if err != nil {
		return "", err
	}
	ApplyRecordings(dir, Recordings(dir, sample.Graph, sample.Cases))
	return dir, nil
}

// deriveBad is the plan without its diagnostics and with one unsafe action after it.
//
//uc:generates evidence.apply-is-all-or-nothing.bad
//uc:generates evidence.apply-diagnostics-are-described.bad
func deriveBad(sample CorpusSample, action int, dir string) Recording {
	plan := Recordings(dir, sample.Graph, sample.Cases)
	plan.Diagnostics = []diag.Diagnostic{}
	switch {
	case action == 1:
		plan.Removals = append(plan.Removals, "design/domain.uc")
	case action == 2:
		plan.Edits = append(plan.Edits, lang.Edit{Path: "corpus/x.uc", Content: []byte("not a recording file")})
	case action == 3 && len(plan.Edits) > 0:
		plan.Edits = append(plan.Edits, plan.Edits[0])
	default:
		plan.Edits = append(plan.Edits, lang.Edit{Path: "../outside.uc", Content: []byte("corpus outside\n")})
	}
	return plan
}

//uc:generates evidence.apply-refuses-foreign-removals.removal
func deriveForeignRemoval() Recording {
	return Recording{Edits: []lang.Edit{}, Removals: []string{"corpus/broken.uc", "corpus/link.uc"}, Diagnostics: []diag.Diagnostic{}}
}

// The observation adapters read the tree under dir, each for one named
// observation (docs/model.md §10 rule 15).

//uc:observes evidence.recordings-roundtrip-inputs.files
func observeRoundtripTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.recordings-refuse-unreplayable-inputs.files
func observeUnreplayableTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.recordings-retire-orphans.files
func observeOrphanTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.recordings-refuse-foreign-corpus.files
func observeForeignTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.apply-writes-the-plan.files
func observeAppliedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.apply-keeps-other-entries.files
func observeKeptTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.apply-is-all-or-nothing.files
func observeRefusedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.apply-refuses-foreign-removals.files
func observeRemovalTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.save-is-idempotent.files
func observeResavedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.save-refuses-damaged-store.files
func observeDamagedSaveTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.read-reports-damaged-store.files
func observeDamagedReadTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes evidence.save-keeps-history.stored
func observeStored(dir string) (Loaded, error) { return Read(dir), nil }

// drawReport draws a report of domain.rule whose result follows its cases.
func drawReport(t *rapid.T, second int) lang.RunReport {
	cases := drawCases(t, rapid.IntRange(0, 13).Draw(t, "reportShape"))
	result := lang.RunResult{Id: "domain.rule", Passed: true, Cases: cases, Observations: []string{}, Differences: []string{}, Diagnostics: []diag.Diagnostic{}}
	for _, c := range cases {
		if c.Rejected {
			result.Rejected++
		} else {
			result.Accepted++
		}
		result.Passed = result.Passed && c.Passed
	}
	return lang.RunReport{Version: 1, Snapshot: rapid.StringMatching(`[0-9a-f]{64}`).Draw(t, "snapshot"),
		At: time.Date(2026, 11, 28, 0, 0, second, 0, time.UTC).Format(time.RFC3339Nano), Mode: "explore",
		Results: []lang.RunResult{result}, Diagnostics: []diag.Diagnostic{}}
}

func drawStoreSample(t *rapid.T, damages []string) StoreSample {
	seconds := rapid.SliceOfNDistinct(rapid.IntRange(0, 86399), 3, 3, rapid.ID[int]).Draw(t, "seconds")
	sample := StoreSample{Damage: rapid.SampledFrom(damages).Draw(t, "damage")}
	for i := range rapid.IntRange(0, 2).Draw(t, "earlier") {
		sample.Earlier = append(sample.Earlier, drawReport(t, seconds[i]))
	}
	sample.Report = drawReport(t, seconds[2])
	if len(sample.Earlier) > 0 && rapid.IntRange(0, 2).Draw(t, "again") == 0 {
		sample.Report = rapid.SampledFrom(sample.Earlier).Draw(t, "repeated")
	}
	return sample
}

//uc:generates evidence.save-keeps-history.sample
//uc:generates evidence.save-is-idempotent.sample
func genIntactStore() *rapid.Generator[StoreSample] {
	return rapid.Custom(func(t *rapid.T) StoreSample { return drawStoreSample(t, []string{"none"}) })
}

//uc:generates evidence.save-refuses-damaged-store.sample
//uc:generates evidence.read-reports-damaged-store.sample
func genDamagedStore() *rapid.Generator[StoreSample] {
	return rapid.Custom(func(t *rapid.T) StoreSample { return drawStoreSample(t, []string{"record", "link"}) })
}

// savedDir writes a repository with only design/domain.uc and saves the
// reports into it in order.
func savedDir(reports []lang.RunReport) (string, error) {
	dir, err := repo.Materialize(repo.Tree{Entries: map[string]repo.Entry{"design": treeDir(), "design/domain.uc": treeFile("designer-owned\n")}})
	if err != nil {
		return "", err
	}
	for _, report := range reports {
		if problems := Save(dir, report); len(problems) > 0 {
			return "", fmt.Errorf("cannot save an earlier report: %+v", problems)
		}
	}
	return dir, nil
}

//uc:generates evidence.save-keeps-history.dir
func deriveEarlierStore(sample StoreSample) (string, error) { return savedDir(sample.Earlier) }

//uc:generates evidence.save-is-idempotent.dir
func deriveSavedStore(sample StoreSample) (string, error) {
	return savedDir(append(slices.Clone(sample.Earlier), sample.Report))
}

// deriveDamagedStore saves the earlier reports, then adds a run record with
// a repeated key, or turns results.json into a link to design/domain.uc.
//
//uc:generates evidence.save-refuses-damaged-store.dir
//uc:generates evidence.read-reports-damaged-store.dir
func deriveDamagedStore(sample StoreSample) (string, error) {
	dir, err := savedDir(sample.Earlier)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, ".unicode", "runs"), 0o755); err != nil {
		return "", err
	}
	if sample.Damage == "record" {
		return dir, os.WriteFile(filepath.Join(dir, ".unicode", "runs", "broken.json"), []byte(`{"version":1,"version":1}`), 0o644)
	}
	latest := filepath.Join(dir, ".unicode", "results.json")
	if err := os.Remove(latest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return dir, os.Symlink(filepath.Join("..", "design", "domain.uc"), latest)
}

func drawShrinkSample(t *rapid.T, modes []string) ShrinkSample {
	threshold := rapid.IntRange(1, 5).Draw(t, "threshold")
	start := threshold + rapid.IntRange(0, 10).Draw(t, "above")
	input, err := NormalizeInput(lang.Input{Values: map[string]lang.Value{"n": intValue(start)}})
	if err != nil {
		t.Fatal(err)
	}
	initial := lang.CaseResult{Check: "domain.rule", Form: "recorded", Reason: "counterexample", Input: input, InputHash: hash(input.Values), Passed: false}
	initial.Id = initial.Check + ".r-" + initial.InputHash[:8]
	return ShrinkSample{Threshold: threshold, Initial: initial, Mode: rapid.SampledFrom(modes).Draw(t, "mode"), Budget: time.Second, Minimal: intValue(threshold)}
}

//uc:generates evidence.shrink-finds-the-smallest-failure.sample
func genNormalShrink() *rapid.Generator[ShrinkSample] {
	return rapid.Custom(func(t *rapid.T) ShrinkSample { return drawShrinkSample(t, []string{"normal"}) })
}

//uc:generates evidence.shrink-keeps-the-failure-when-interrupted.sample
func genStoppedShrink() *rapid.Generator[ShrinkSample] {
	return rapid.Custom(func(t *rapid.T) ShrinkSample { return drawShrinkSample(t, []string{"exhausted", "environment"}) })
}

//uc:generates evidence.shrink-finds-the-smallest-failure.runner
//uc:generates evidence.shrink-keeps-the-failure-when-interrupted.runner
func deriveRunner(sample ShrinkSample) lang.CandidateRunner {
	return &thresholdRunner{threshold: sample.Threshold, mode: sample.Mode}
}

func runnerLog(runner lang.CandidateRunner) (RunnerLog, error) {
	r, ok := runner.(*thresholdRunner)
	if !ok {
		return RunnerLog{}, errors.New("the runner of the sample does not count its candidates")
	}
	return RunnerLog{Evaluations: r.evaluations, Rejected: r.rejected}, nil
}

//uc:observes evidence.shrink-finds-the-smallest-failure.log
func observeShrinkLog(runner lang.CandidateRunner) (RunnerLog, error) { return runnerLog(runner) }

//uc:observes evidence.shrink-keeps-the-failure-when-interrupted.log
func observeStoppedLog(runner lang.CandidateRunner) (RunnerLog, error) { return runnerLog(runner) }

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

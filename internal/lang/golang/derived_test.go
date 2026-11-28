package golang

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// DerivedOutcome is how a case of the derived-input module ends.
//
//uc:realizes lang.golang.DerivedOutcome
type DerivedOutcome string

const (
	DerivedOutcomePasses      DerivedOutcome = "passes"
	DerivedOutcomeFails       DerivedOutcome = "fails"
	DerivedOutcomeRejected    DerivedOutcome = "rejected"
	DerivedOutcomePanics      DerivedOutcome = "panics"
	DerivedOutcomeUnderivable DerivedOutcome = "underivable"
)

// BuiltState is what the derived-input module's derivation recorded: the
// directories it made, and how many of them still exist.
//
//uc:realizes lang.golang.BuiltState
type BuiltState struct {
	Recorded  int
	Remaining int
}

// derivedModes gives each outcome the mode its case passes to the module:
// Probe returns true for 0 and false for 1, the precondition refuses 2,
// Probe panics on 3, and the derivation fails on 4.
var derivedModes = map[DerivedOutcome]int{
	DerivedOutcomePasses: 0, DerivedOutcomeFails: 1, DerivedOutcomeRejected: 2, DerivedOutcomePanics: 3, DerivedOutcomeUnderivable: 4,
}

// deriveCleanupPlan writes a Go module into a new directory: a function
// Probe, and a property demo.cleans on it whose derived input dir is made by
// a derivation that creates a directory and records its path in built.txt
// at the module's root, the working directory of the drivers Backend.run
// starts, so that the module names no directory of its own and its state
// template can move. Each outcome is one declared case. It generates the
// module's drivers and gives the plan that replays demo.cleans.
//
//uc:generates lang.golang.derived-state-is-removed.plan
//uc:generates lang.golang.underivable-input-is-check-error.plan
func deriveCleanupPlan(outcomes []DerivedOutcome) (lang.RunPlan, error) {
	root, err := os.MkdirTemp("", "derived-module-")
	if err != nil {
		return lang.RunPlan{}, err
	}
	sum, err := os.ReadFile(filepath.Join("..", "..", "..", "go.sum"))
	if err != nil {
		return lang.RunPlan{}, err
	}
	files := map[string]string{
		"go.mod":       "module example.test/derived\n\ngo 1.26.2\n\nrequire pgregory.net/rapid v1.3.0\n",
		"go.sum":       string(sum),
		"src/probe.go": "package probe\n\n//uc:realizes demo.probe\nfunc Probe(mode int) bool {\n\tif mode == 3 {\n\t\tpanic(\"probe panics\")\n\t}\n\treturn mode != 1\n}\n",
		"src/derive_test.go": "package probe\n\nimport (\n\t\"errors\"\n\t\"os\"\n)\n\n" +
			"//uc:generates demo.cleans.dir\nfunc deriveDir(mode int) (string, error) {\n" +
			"\tdir, err := os.MkdirTemp(\"\", \"built-\")\n\tif err != nil {\n\t\treturn \"\", err\n\t}\n" +
			"\tf, err := os.OpenFile(\"built.txt\", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)\n" +
			"\tif err != nil {\n\t\treturn \"\", err\n\t}\n\tdefer f.Close()\n" +
			"\tif _, err := f.WriteString(dir + \"\\n\"); err != nil {\n\t\treturn \"\", err\n\t}\n" +
			"\tif mode == 4 {\n\t\treturn \"\", errors.New(\"the derivation refuses mode 4\")\n\t}\n\treturn dir, nil\n}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return lang.RunPlan{}, err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return lang.RunPlan{}, err
		}
	}
	g, err := derivedGraph(root, outcomes)
	if err != nil {
		return lang.RunPlan{}, err
	}
	generated := NewBackend().Generate(lang.GenPlan{Root: root, Roots: []string{"src"}, Graph: &g})
	if len(generated.Diagnostics) > 0 {
		return lang.RunPlan{}, fmt.Errorf("the derived-input module does not generate: %+v", generated.Diagnostics)
	}
	for _, edit := range generated.Edits {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(edit.Path)), edit.Content, 0o644); err != nil {
			return lang.RunPlan{}, err
		}
	}
	return lang.RunPlan{Root: root, Roots: []string{"src"}, Graph: &g, Ids: []string{"demo.cleans"}}, nil
}

// derivedGraph is the derived-input module's design, its declared cases
// one for each outcome, linked with the code under root.
func derivedGraph(root string, outcomes []DerivedOutcome) (graph.Graph, error) {
	g := graph.NewGraph()
	var problems []error
	add := func(id string, kind graph.NodeKind, form, parent string, attrs map[string][]string) {
		problems = append(problems, g.AddNode(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerDesign, Source: graph.SourceDeclared,
			Origin: diag.Position{Path: "design/demo.uc", Line: 1, Column: 1}, Attrs: attrs}))
		if parent != "" {
			problems = append(problems, g.AddEdge(graph.Edge{Kind: graph.EdgeKindContains, From: parent, To: id, Source: graph.SourceDeclared}))
		}
	}
	add("demo", graph.NodeKindContainer, "component", "", map[string][]string{"paths": {"src/**"}})
	add("demo.probe", graph.NodeKindOp, "op", "demo", map[string][]string{"signature": {"(mode: int) -> bool"}})
	add("demo.cleans", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{
		"symbols": {"mode: int", "dir: string generated"}, "requires": {"mode != 2"}, "call": {"probe(mode)"}, "ensure": {"result"},
	})
	problems = append(problems, g.AddEdge(graph.Edge{Kind: graph.EdgeKindRefers, From: "demo.cleans", To: "demo.probe", Source: graph.SourceDeclared}))
	for i, o := range outcomes {
		add(fmt.Sprintf("demo.cleans.c%d", i), graph.NodeKindCase, "declared", "demo.cleans", map[string][]string{"values": {fmt.Sprintf("mode = %d", derivedModes[o])}})
	}
	if err := errors.Join(problems...); err != nil {
		return g, err
	}
	extracted, err := NewBackend().ExtractForGeneration(root, []string{"src"})
	if err != nil {
		return g, err
	}
	if err := g.AddFragment(extracted.Fragment); err != nil {
		return g, err
	}
	if ds := g.Derive(); len(ds) > 0 {
		return g, fmt.Errorf("the derived-input module does not link: %+v", ds)
	}
	return g, nil
}

// observeBuilt reads the directories the module's derivation recorded in
// built.txt, and counts those that still exist.
//
//uc:observes lang.golang.derived-state-is-removed.built
func observeBuilt(plan lang.RunPlan) (BuiltState, error) {
	f, err := os.Open(filepath.Join(plan.Root, "built.txt"))
	if errors.Is(err, fs.ErrNotExist) {
		return BuiltState{}, nil
	}
	if err != nil {
		return BuiltState{}, err
	}
	defer func() { _ = f.Close() }()
	var state BuiltState
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		dir := strings.TrimSpace(lines.Text())
		if dir == "" {
			continue
		}
		state.Recorded++
		if _, err := os.Stat(dir); err == nil {
			state.Remaining++
		} else if !errors.Is(err, fs.ErrNotExist) {
			return BuiltState{}, err
		}
	}
	return state, lines.Err()
}

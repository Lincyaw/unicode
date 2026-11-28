package gen

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/repo"
)

// WriteSample is a directory tree, with the repository under repo and a
// directory outside it under outside, a generation plan for the repository,
// the path in the tree of each path of the plan, whether the plan is safe
// by the way it was drawn, and the tree the plan writes when it is safe.
//
//uc:realizes gen.WriteSample
type WriteSample struct {
	Tree    repo.Tree
	Edits   []lang.Edit
	Keys    map[string]string
	Safe    bool
	Applied repo.Tree
}

// PlanSample supplies a system graph, each backend's generation, and the
// generations of the configured languages.
//
//uc:realizes gen.PlanSample
type PlanSample struct {
	Graph       *graph.Graph
	Generations map[string]lang.Generation
	Configured  []lang.Generation
}

// Received is one generation input a recording backend was given.
//
//uc:realizes gen.Received
type Received struct {
	Root     string
	Roots    []string
	Snapshot []byte
}

func file(content string, mode int) repo.Entry {
	return repo.Entry{Kind: repo.EntryKindFile, Content: []byte(content), Mode: mode}
}

func link(target string) repo.Entry { return repo.Entry{Kind: repo.EntryKindLink, Target: target} }

var (
	allVariants  = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	safeVariants = []int{0, 9, 10}
)

// drawWriteSample draws the tree and a plan that creates stem/new.go, then
// changes the plan by one of the variants: 0 also rewrites owned.go, 9
// leaves it, 10 makes the new file createOnly, and the others make it
// unsafe.
func drawWriteSample(t *rapid.T, variants []int) WriteSample {
	stem := rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "stem")
	text := rapid.String().Draw(t, "content")
	owned := "// " + generatedMarker + "\n" + text
	tree := repo.Tree{Entries: map[string]repo.Entry{
		"repo": {Kind: repo.EntryKindDir, Mode: 0o755}, "repo/owned.go": file(owned, 0o640), "repo/user.go": file(text, 0o640),
		"outside": {Kind: repo.EntryKindDir, Mode: 0o755}, "outside/user.go": file("outside", 0o600),
	}}
	edits := []lang.Edit{{Path: stem + "/new.go", Content: []byte(owned + "\n")}}
	variant := rapid.SampledFrom(variants).Draw(t, "variant")
	switch variant {
	case 0:
		edits = append(edits, lang.Edit{Path: "owned.go", Content: []byte(owned + "new")})
	case 1:
		edits = append(edits, lang.Edit{Path: "user.go", Content: []byte(owned)})
	case 2:
		edits = append(edits, lang.Edit{Path: "../outside/user.go", Content: []byte(owned)})
	case 3:
		edits = append(edits, edits[0])
	case 4:
		tree.Entries["repo/link.go"] = link("user.go")
		edits = append(edits, lang.Edit{Path: "link.go", Content: []byte(owned)})
	case 5:
		tree.Entries["repo/redirect"] = link("../outside")
		edits = append(edits, lang.Edit{Path: "redirect/user.go", Content: []byte(owned)})
	case 6:
		edits = append(edits, lang.Edit{Path: stem + "/./other.go", Content: []byte(owned)})
	case 7:
		edits = append(edits, lang.Edit{Path: stem, Content: []byte(owned)})
	case 8:
		edits = append(edits, lang.Edit{Path: "", Content: []byte(owned)})
	case 10:
		edits[0].CreateOnly = true
	case 11:
		edits = append(edits, lang.Edit{Path: "owned.go", Content: []byte(owned), CreateOnly: true})
	case 12:
		edits = append(edits, lang.Edit{Path: "user.go", Content: []byte(text), CreateOnly: true})
	}
	sample := WriteSample{Tree: tree, Edits: edits, Keys: map[string]string{}, Safe: slices.Contains(safeVariants, variant), Applied: tree}
	for _, e := range edits {
		sample.Keys[e.Path] = "repo/" + e.Path
	}
	if sample.Safe {
		sample.Applied = written(tree, edits)
	}
	return sample
}

// written is tree after the safe plan edits is written into its repo: new
// directories 755, new files 644, existing files keeping their bits.
func written(tree repo.Tree, edits []lang.Edit) repo.Tree {
	out := repo.Tree{Entries: maps.Clone(tree.Entries)}
	for _, e := range edits {
		target := "repo/" + e.Path
		for dir := path.Dir(target); dir != "repo"; dir = path.Dir(dir) {
			if _, ok := out.Entries[dir]; !ok {
				out.Entries[dir] = repo.Entry{Kind: repo.EntryKindDir, Mode: 0o755}
			}
		}
		mode := 0o644
		if old, ok := out.Entries[target]; ok {
			mode = old.Mode
		}
		out.Entries[target] = repo.Entry{Kind: repo.EntryKindFile, Content: e.Content, Mode: mode}
	}
	return out
}

//uc:generates gen.write-applies-safe-plans.sample
//uc:generates gen.write-rejects-unsafe-plans.sample
//uc:generates gen.write-keeps-other-entries.sample
//uc:generates gen.write-keeps-modes.sample
//uc:generates gen.write-diagnostics-are-described.sample
//uc:generates gen.check-files-agrees-with-write.sample
//uc:generates gen.check-files-diagnostics-are-described.sample
func genWriteSample() *rapid.Generator[WriteSample] {
	return rapid.Custom(func(t *rapid.T) WriteSample { return drawWriteSample(t, allVariants) })
}

//uc:generates gen.write-is-idempotent.sample
//uc:generates gen.check-files-reports-stale-targets.sample
//uc:generates gen.check-files-rejects-written-create-only.sample
func genSafeWriteSample() *rapid.Generator[WriteSample] {
	return rapid.Custom(func(t *rapid.T) WriteSample { return drawWriteSample(t, safeVariants) })
}

// genStaleState draws, with even chances, the tree before the plan is
// written, the tree after, or the tree after with one target's content
// changed (a ! appended) or with one target removed.
//
//uc:generates gen.check-files-reports-stale-targets.state
func genStaleState(sample WriteSample) *rapid.Generator[repo.Tree] {
	return rapid.Custom(func(t *rapid.T) repo.Tree {
		choice := rapid.IntRange(0, 3).Draw(t, "state")
		switch choice {
		case 0:
			return sample.Tree
		case 1:
			return sample.Applied
		}
		tree := repo.Tree{Entries: maps.Clone(sample.Applied.Entries)}
		target := "repo/" + rapid.SampledFrom(sample.Edits).Draw(t, "target").Path
		if choice == 2 {
			e := tree.Entries[target]
			e.Content = append(slices.Clone(e.Content), '!')
			tree.Entries[target] = e
		} else {
			delete(tree.Entries, target)
		}
		return tree
	})
}

//uc:generates gen.write-applies-safe-plans.dir
//uc:generates gen.write-rejects-unsafe-plans.dir
//uc:generates gen.write-keeps-other-entries.dir
//uc:generates gen.write-keeps-modes.dir
//uc:generates gen.write-diagnostics-are-described.dir
//uc:generates gen.check-files-agrees-with-write.dir
//uc:generates gen.check-files-diagnostics-are-described.dir
func deriveTreeDir(sample WriteSample) (string, error) { return repo.Materialize(sample.Tree) }

//uc:generates gen.write-is-idempotent.dir
//uc:generates gen.check-files-rejects-written-create-only.dir
func deriveAppliedDir(sample WriteSample) (string, error) { return repo.Materialize(sample.Applied) }

//uc:generates gen.check-files-reports-stale-targets.dir
func deriveStateDir(state repo.Tree) (string, error) { return repo.Materialize(state) }

//uc:generates gen.write-applies-safe-plans.root
//uc:generates gen.write-rejects-unsafe-plans.root
//uc:generates gen.write-keeps-other-entries.root
//uc:generates gen.write-keeps-modes.root
//uc:generates gen.write-diagnostics-are-described.root
//uc:generates gen.write-is-idempotent.root
//uc:generates gen.check-files-agrees-with-write.root
//uc:generates gen.check-files-reports-stale-targets.root
//uc:generates gen.check-files-rejects-written-create-only.root
//uc:generates gen.check-files-diagnostics-are-described.root
func deriveRepoRoot(dir string) string { return filepath.Join(dir, "repo") }

// deriveRejection writes the plan into a copy of the sample's tree and
// gives what Write reports.
//
//uc:generates gen.check-files-agrees-with-write.rejection
func deriveRejection(sample WriteSample) ([]diag.Diagnostic, error) {
	dir, err := repo.Materialize(sample.Tree)
	if err != nil {
		return nil, err
	}
	return Write(filepath.Join(dir, "repo"), sample.Edits), nil
}

// The observation adapters read the tree under dir, each for one named
// observation (docs/model.md §10 rule 15).

//uc:observes gen.write-applies-safe-plans.files
func observeAppliedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes gen.write-rejects-unsafe-plans.files
func observeRejectedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes gen.write-keeps-other-entries.files
func observeKeptTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes gen.write-keeps-modes.files
func observeModeTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes gen.write-is-idempotent.files
func observeRewrittenTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes gen.check-files-reports-stale-targets.files
func observeStaleTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes gen.check-files-rejects-written-create-only.files
func observeWrittenTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes gen.plan-dispatches-configured-languages.files
func observePlanTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:generates gen.plan-dispatches-configured-languages.sample
//uc:generates gen.plan-rejects-duplicate-backends.sample
//uc:generates gen.plan-rejects-backends-without-generation.sample
func genPlanSample() *rapid.Generator[PlanSample] {
	return rapid.Custom(func(t *rapid.T) PlanSample {
		g := graph.NewGraph()
		roots := []string{}
		sample := PlanSample{Graph: &g, Generations: map[string]lang.Generation{}}
		for _, name := range []string{"go", "python", "unused"} {
			configured := name != "unused" && rapid.Bool().Draw(t, name+"configured")
			if configured {
				for range rapid.IntRange(1, 3).Draw(t, name+"roots") {
					roots = append(roots, name+": "+rapid.StringMatching(`[a-z]{1,6}`).Draw(t, "root"))
				}
			}
			var generation lang.Generation
			for i := range rapid.IntRange(0, 3).Draw(t, name+"edits") {
				generation.Edits = append(generation.Edits, lang.Edit{Path: fmt.Sprintf("%s/%d.go", name, i), Content: []byte(rapid.String().Draw(t, "content"))})
			}
			if rapid.Bool().Draw(t, name+"diagnostic") {
				generation.Diagnostics = []diag.Diagnostic{diagnostic("test.generation", name+".uc", "a generated diagnostic", "change the input")}
			}
			sample.Generations[name] = generation
			if configured {
				sample.Configured = append(sample.Configured, generation)
			}
		}
		if err := g.AddNode(graph.Node{Id: "system", Kind: graph.NodeKindContainer, Form: "system", Layer: graph.LayerDesign, Origin: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}, Attrs: map[string][]string{"roots": roots}}); err != nil {
			t.Fatal(err)
		}
		return sample
	})
}

//uc:generates gen.plan-dispatches-configured-languages.dir
//uc:generates gen.plan-rejects-duplicate-backends.dir
//uc:generates gen.plan-rejects-backends-without-generation.dir
func deriveEmptyDir() (string, error) { return repo.Materialize(repo.Tree{}) }

// generationBackend records each generation input it is given and returns
// its generation.
type generationBackend struct {
	name       string
	generation lang.Generation
	plans      []lang.GenPlan
}

func (b *generationBackend) Language() string { return b.name }
func (*generationBackend) Extract(string, []string) (lang.Extracted, error) {
	panic("generation must not extract code")
}
func (*generationBackend) MapName(lang.NameRole, string) string {
	panic("generation orchestration must not map names")
}
func (b *generationBackend) Generate(p lang.GenPlan) lang.Generation {
	b.plans = append(b.plans, p)
	return b.generation
}

// readOnlyBackend is a backend without generation.
type readOnlyBackend struct{ name string }

func (b readOnlyBackend) Language() string { return b.name }
func (readOnlyBackend) Extract(string, []string) (lang.Extracted, error) {
	panic("orchestration must not extract code")
}
func (readOnlyBackend) MapName(lang.NameRole, string) string {
	panic("orchestration must not map names")
}

//uc:generates gen.plan-dispatches-configured-languages.backends
func deriveRecordingBackends(sample PlanSample) []lang.Backend {
	var out []lang.Backend
	for _, name := range []string{"go", "python", "unused"} {
		out = append(out, &generationBackend{name: name, generation: sample.Generations[name]})
	}
	return out
}

//uc:generates gen.plan-rejects-duplicate-backends.backends
func deriveDuplicateBackends(sample PlanSample) []lang.Backend {
	backends := deriveRecordingBackends(sample)
	return append(backends, backends[0])
}

//uc:generates gen.plan-rejects-backends-without-generation.backends
func deriveReadOnlyBackends() []lang.Backend {
	return []lang.Backend{readOnlyBackend{name: "go"}, readOnlyBackend{name: "python"}}
}

// observeReceived reads the generation inputs each recording backend was
// given, the graph as its snapshot.
//
//uc:observes gen.plan-dispatches-configured-languages.received
func observeReceived(backends []lang.Backend) (map[string][]Received, error) {
	out := map[string][]Received{}
	for _, b := range backends {
		recording, ok := b.(*generationBackend)
		if !ok {
			return nil, errors.New("a backend of the sample does not record its inputs")
		}
		out[recording.name] = []Received{}
		for _, p := range recording.plans {
			out[recording.name] = append(out[recording.name], Received{Root: p.Root, Roots: slices.Clone(p.Roots), Snapshot: p.Graph.Encode()})
		}
	}
	return out, nil
}

// genGenerations draws generation plans whose files often share a path.
//
//uc:generates gen.merge-keeps-every-file.generations
//uc:generates gen.merge-reports-duplicate-paths.generations
//uc:generates gen.merge-ignores-order.generations
func genGenerations() *rapid.Generator[[]lang.Generation] {
	return rapid.Custom(func(t *rapid.T) []lang.Generation {
		var out []lang.Generation
		for range rapid.IntRange(1, 4).Draw(t, "generations") {
			var g lang.Generation
			for range rapid.IntRange(0, 3).Draw(t, "edits") {
				g.Edits = append(g.Edits, lang.Edit{
					Path:       rapid.SampledFrom([]string{"a.go", "b.go", "c/d.go"}).Draw(t, "path"),
					Content:    []byte(rapid.SampledFrom([]string{"x", "y"}).Draw(t, "editContent")),
					CreateOnly: rapid.Bool().Draw(t, "createOnly"),
				})
			}
			if rapid.Bool().Draw(t, "diagnostic") {
				g.Diagnostics = []diag.Diagnostic{diagnostic(rapid.SampledFrom([]string{"gen.stale-file", "gen.write-failed"}).Draw(t, "code"),
					rapid.SampledFrom([]string{"a.go", "design/system.uc"}).Draw(t, "at"),
					rapid.SampledFrom([]string{"one cause", "another cause"}).Draw(t, "cause"), "change the input")}
			}
			out = append(out, g)
		}
		return out
	})
}

//uc:generates gen.merge-ignores-order.rotated
func deriveRotatedGenerations(generations []lang.Generation, rotation int) []lang.Generation {
	by := rotation % len(generations)
	return append(slices.Clone(generations[by:]), generations[:by]...)
}

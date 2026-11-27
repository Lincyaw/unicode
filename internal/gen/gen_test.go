package gen

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// WriteSample is a file tree and the edits proposed for it.
//
//uc:realizes gen.WriteSample
type WriteSample struct {
	Files map[string]string
	Links map[string]string
	Edits []lang.Edit
}

// PlanSample supplies a system graph and each backend's generation.
//
//uc:realizes gen.PlanSample
type PlanSample struct {
	Graph       *graph.Graph
	Generations map[string]lang.Generation
}

//uc:generates gen.write-preserves-unowned-files.sample
func genWriteSample() *rapid.Generator[WriteSample] {
	return rapid.Custom(func(t *rapid.T) WriteSample {
		stem := rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "stem")
		text := rapid.String().Draw(t, "content")
		owned := "// " + generatedMarker + "\n" + text
		s := WriteSample{Files: map[string]string{"owned.go": owned, "user.go": text}, Links: map[string]string{}, Edits: []lang.Edit{{Path: stem + "/new.go", Content: []byte(owned + "\n")}}}
		switch rapid.IntRange(0, 8).Draw(t, "conflict") {
		case 0:
			s.Edits = append(s.Edits, lang.Edit{Path: "owned.go", Content: []byte(owned + "new")})
		case 1:
			s.Edits = append(s.Edits, lang.Edit{Path: "user.go", Content: []byte(owned)})
		case 2:
			s.Edits = append(s.Edits, lang.Edit{Path: "../outside/user.go", Content: []byte(owned)})
		case 3:
			s.Edits = append(s.Edits, s.Edits[0])
		case 4:
			s.Links["link.go"] = "user.go"
			s.Edits = append(s.Edits, lang.Edit{Path: "link.go", Content: []byte(owned)})
		case 5:
			s.Links["redirect"] = "../outside"
			s.Edits = append(s.Edits, lang.Edit{Path: "redirect/user.go", Content: []byte(owned)})
		case 7:
			s.Edits = append(s.Edits, lang.Edit{Path: stem, Content: []byte(owned)})
		case 6:
			s.Edits = append(s.Edits, lang.Edit{Path: stem + "/./other.go", Content: []byte(owned)})
		}
		return s
	})
}

//uc:checks gen.write-preserves-unowned-files
func checkWritePreservesUnownedFiles(t rapid.TB, sample WriteSample) {
	base, err := os.MkdirTemp("", "unicode-write-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(base) }()
	root := filepath.Join(base, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "outside"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside", "user.go"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	for p, src := range sample.Files {
		if err := os.WriteFile(filepath.Join(root, p), []byte(src), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	for p, target := range sample.Links {
		if err := os.Symlink(target, filepath.Join(root, p)); err != nil {
			t.Fatal(err)
		}
	}
	before := readTree(t, base)
	want := map[string][]byte{}
	valid := true
	for _, edit := range sample.Edits {
		if !filepath.IsLocal(edit.Path) || edit.Path == "." || strings.ContainsAny(edit.Path, "\\\x00") || path.Clean(edit.Path) != edit.Path {
			valid = false
			continue
		}
		if _, ok := want[edit.Path]; ok {
			valid = false
		}
		want[edit.Path] = edit.Content
		for link := range sample.Links {
			if edit.Path == link || strings.HasPrefix(edit.Path, link+"/") {
				valid = false
			}
		}
		if src, ok := sample.Files[edit.Path]; ok && !strings.HasPrefix(src, "// "+generatedMarker+"\n") {
			valid = false
		}
	}
	for file := range want {
		for parent := path.Dir(file); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if _, exists := want[parent]; exists {
				valid = false
			}
		}
	}
	ds := Write(root, sample.Edits)
	if !valid {
		if len(ds) == 0 {
			t.Fatal("Write accepted a conflicting plan")
		}
		if after := readTree(t, base); !reflect.DeepEqual(before, after) {
			t.Fatalf("Write changed a file before rejecting the plan: before %v, after %v", before, after)
		}
		return
	}
	if len(ds) > 0 {
		t.Fatalf("Write rejected a safe plan: %+v", ds)
	}
	for p, content := range want {
		got, err := os.ReadFile(filepath.Join(root, p))
		if err != nil || !bytes.Equal(got, content) {
			t.Fatalf("Write(%s) = %q, %v; want %q", p, got, err, content)
		}
	}
	for p, src := range sample.Files {
		if _, changed := want[p]; !changed {
			got, err := os.ReadFile(filepath.Join(root, p))
			if err != nil || string(got) != src {
				t.Fatalf("Write changed the unplanned file %s", p)
			}
		}
	}
	first := readTree(t, base)
	if ds := Write(root, sample.Edits); len(ds) > 0 {
		t.Fatalf("reapplying a generated plan failed: %+v", ds)
	}
	if after := readTree(t, base); !reflect.DeepEqual(first, after) {
		t.Fatal("reapplying the plan changed file contents")
	}
	if _, changed := want["owned.go"]; changed {
		info, err := os.Stat(filepath.Join(root, "owned.go"))
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("Write did not preserve existing permissions: %v, %v", info, err)
		}
	}
}

func readTree(t rapid.TB, root string) map[string]string {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if e.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			out[rel] = "link:" + target
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = "file:" + string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

//uc:generates gen.plan-follows-language-roots.sample
func genPlanSample() *rapid.Generator[PlanSample] {
	return rapid.Custom(func(t *rapid.T) PlanSample {
		g := graph.NewGraph()
		roots := []string{}
		generations := map[string]lang.Generation{}
		for _, name := range []string{"go", "python", "unused"} {
			if name != "unused" && rapid.Bool().Draw(t, name+"configured") {
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
			generations[name] = generation
		}
		if err := g.AddNode(graph.Node{Id: "system", Kind: graph.NodeKindContainer, Form: "system", Layer: graph.LayerDesign, Origin: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}, Attrs: map[string][]string{"roots": roots}}); err != nil {
			t.Fatal(err)
		}
		return PlanSample{Graph: &g, Generations: generations}
	})
}

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

type readOnlyBackend struct{ name string }

func (b readOnlyBackend) Language() string { return b.name }
func (readOnlyBackend) Extract(string, []string) (lang.Extracted, error) {
	panic("orchestration must not extract code")
}
func (readOnlyBackend) MapName(lang.NameRole, string) string {
	panic("orchestration must not map names")
}

//uc:checks gen.plan-follows-language-roots
func checkPlanFollowsLanguageRoots(t rapid.TB, sample PlanSample) {
	root, err := os.MkdirTemp("", "unicode-plan-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	backends := []lang.Backend{}
	byName := map[string]*generationBackend{}
	for _, name := range []string{"go", "python", "unused"} {
		b := &generationBackend{name: name, generation: sample.Generations[name]}
		byName[name] = b
		backends = append(backends, b)
	}
	got := Plan(root, sample.Graph, backends)
	var edits []lang.Edit
	var ds []diag.Diagnostic
	configured := map[string][]string{}
	for _, entry := range sample.Graph.Node("system").Attrs["roots"] {
		name, dir, _ := strings.Cut(entry, ": ")
		configured[name] = append(configured[name], dir)
	}
	for name, b := range byName {
		roots := configured[name]
		if len(roots) == 0 {
			if len(b.plans) != 0 {
				t.Fatalf("Plan called unconfigured %s", name)
			}
			continue
		}
		if len(b.plans) != 1 || b.plans[0].Root != root || b.plans[0].Graph != sample.Graph || !slices.Equal(b.plans[0].Roots, roots) {
			t.Fatalf("Plan passed wrong inputs to %s: %+v", name, b.plans)
		}
		edits = append(edits, b.generation.Edits...)
		ds = append(ds, b.generation.Diagnostics...)
	}
	slices.SortFunc(edits, func(a, b lang.Edit) int { return strings.Compare(a.Path, b.Path) })
	sortDiagnostics(ds)
	if !reflect.DeepEqual(got.Edits, edits) || !reflect.DeepEqual(got.Diagnostics, ds) {
		t.Fatalf("Plan did not merge backend outputs: %+v, want edits %+v diagnostics %+v", got, edits, ds)
	}
	if tree := readTree(t, root); len(tree) != 0 {
		t.Fatal("Plan wrote repository files")
	}
	duplicate := Plan(root, sample.Graph, append(backends, backends[0]))
	if !slices.ContainsFunc(duplicate.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "gen.duplicate-backend" }) {
		t.Fatal("Plan accepted duplicate backends")
	}
	if len(configured) > 0 {
		unsupported := Plan(root, sample.Graph, []lang.Backend{readOnlyBackend{name: "go"}, readOnlyBackend{name: "python"}})
		if !slices.ContainsFunc(unsupported.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "gen.unsupported-language" }) {
			t.Fatal("Plan accepted a backend without generation capability")
		}
	}
}

func TestWritePreservesUnownedFiles(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkWritePreservesUnownedFiles(t, genWriteSample().Draw(t, "sample")) })
}
func FuzzWritePreservesUnownedFiles(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) { checkWritePreservesUnownedFiles(t, genWriteSample().Draw(t, "sample")) }))
}
func TestPlanFollowsLanguageRoots(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkPlanFollowsLanguageRoots(t, genPlanSample().Draw(t, "sample")) })
}
func FuzzPlanFollowsLanguageRoots(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) { checkPlanFollowsLanguageRoots(t, genPlanSample().Draw(t, "sample")) }))
}

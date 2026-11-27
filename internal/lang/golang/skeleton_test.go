package golang

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// SkeletonSample varies a package's existing declarations and client boundary.
//
//uc:realizes lang.golang.SkeletonSample
type SkeletonSample struct {
	Name                                   string
	ExistingType, ExistingMethod, External bool
}

//uc:generates lang.golang.skeletons-preserve-source.sample
func genSkeletonSample() *rapid.Generator[SkeletonSample] {
	return rapid.Custom(func(t *rapid.T) SkeletonSample {
		return SkeletonSample{
			Name:         rapid.StringMatching(`[a-z][a-z0-9]{0,5}`).Draw(t, "name"),
			ExistingType: rapid.Bool().Draw(t, "existingType"), ExistingMethod: rapid.Bool().Draw(t, "existingMethod"), External: rapid.Bool().Draw(t, "external"),
		}
	})
}

//uc:checks lang.golang.skeletons-preserve-source
func checkSkeletonsPreserveSource(t rapid.TB, sample SkeletonSample) {
	root, err := os.MkdirTemp("", "unicode-skeleton-contract-")
	if err != nil {
		t.Fatal(err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	files := map[string]string{"docs/example/go.mod": "this is an unrelated documentation asset\n", "go.mod": "module example.test/skeleton\n\ngo 1.26.2\n\nrequire pgregory.net/rapid v1.3.0\n", "design/component.uc": "component demo \"sample\" {\n  paths \"src/**\"\n}\n"}
	sum, err := os.ReadFile(filepath.Join("..", "..", "..", "go.sum"))
	if err != nil {
		t.Fatal(err)
		return
	}
	files["go.sum"] = string(sum)
	nativeName := "Counter" + upperFirst(sample.Name)
	if sample.ExistingType {
		source := fmt.Sprintf("package app\n//uc:realizes demo.%s\ntype %s struct { Value int }\nfunc (*%s) Read() int { return 13 }\n", nativeName, nativeName, nativeName)
		// Names vary across samples while the binding remains exact.
		if sample.ExistingMethod {
			source += fmt.Sprintf("func (receiver *%s) Bump(delta int) int { receiver.Value += delta; return receiver.Value }\n", nativeName)
		}
		files["src/existing.go"] = source
		probe := "package main\nimport pkg \"example.test/skeleton/src\"\nfunc main(){if new(pkg.Child).Read()!=13 {panic(\"inherited implementation changed\")};"
		if sample.ExistingMethod {
			probe += "func(){defer func(){if recover()!=\"unicode: unimplemented\" {panic(\"explicit child operation is not a stub\")}}();new(pkg.ExplicitChild).Read()}();"
		}
		probe += "}\n"
		files["inheritance-probe/main.go"] = probe
	}
	for name, content := range files {
		writeSkeletonFixture(t, root, name, []byte(content))
	}
	outside, err := os.MkdirTemp("", "unicode-skeleton-outside-")
	if err != nil {
		t.Fatal(err)
		return
	}
	defer func() { _ = os.RemoveAll(outside) }()
	writeSkeletonFixture(t, outside, "untouched.txt", []byte("outside data"))
	if err = os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
		return
	}
	outsideBefore := skeletonTree(t, outside, true)
	if _, err = loadPlannedGo(lang.GenPlan{Root: root, Roots: []string{"src"}}, []lang.Edit{{Path: "escape/missing/stub.go", Content: []byte("package escaped\n"), CreateOnly: true}}, false); err == nil {
		t.Fatal("planning followed an outside directory symlink")
	}
	if after := skeletonTree(t, outside, true); !reflect.DeepEqual(outsideBefore, after) {
		t.Fatal("planning through a directory symlink changed outside entries")
	}
	if sample.External && !sample.ExistingType {
		writeSkeletonFixture(t, outside, "go.mod", []byte(files["go.mod"]))
		if err = os.Remove(filepath.Join(root, "go.mod")); err != nil {
			t.Fatal(err)
			return
		}
		if err = os.Symlink(filepath.Join(outside, "go.mod"), filepath.Join(root, "go.mod")); err != nil {
			t.Fatal(err)
			return
		}
		writeSkeletonFixture(t, outside, "workspace-sentinel", []byte("outside workspace sentinel"))
		if err = os.Symlink(filepath.Join(outside, "workspace-sentinel"), filepath.Join(root, "unicode-skeleton.go.work")); err != nil {
			t.Fatal(err)
			return
		}
		writeSkeletonFixture(t, root, "go.work", []byte("go 1.26.2\nuse .\n"))
	}
	outsideBefore = skeletonTree(t, outside, true)
	checkMalformedSkeletonModule(t, sample)
	g := skeletonFixtureGraph(t, sample)
	attachSkeletonCode(t, root, &g)
	plan := lang.GenPlan{Root: root, Roots: []string{"src"}, Graph: &g}
	before := skeletonTree(t, root)
	planningBefore := skeletonTree(t, root, true)
	generated := NewBackend().Generate(plan)
	if len(generated.Diagnostics) > 0 {
		t.Fatalf("generation failed: %+v", generated.Diagnostics)
		return
	}
	if after := skeletonTree(t, root, true); !reflect.DeepEqual(planningBefore, after) {
		t.Fatal("planning skeletons changed repository entries")
	}
	if after := skeletonTree(t, outside, true); !reflect.DeepEqual(outsideBefore, after) {
		t.Fatal("module or workspace planning changed outside entries")
	}
	count := 0
	for _, edit := range generated.Edits {
		if edit.CreateOnly {
			count++
			if bytes.Contains(edit.Content, []byte(unicodeGeneratedHeader)) {
				t.Fatalf("human-owned skeleton %s carries generation ownership", edit.Path)
			}
			if !strings.Contains(string(edit.Content), "//uc:") && !strings.Contains(string(edit.Content), "func (") {
				t.Fatalf("skeleton %s has no binding", edit.Path)
			}
		}
		writeSkeletonFixture(t, root, edit.Path, edit.Content)
	}
	if count == 0 {
		t.Fatal("missing declarations produced no create-only skeletons")
	}
	applied := skeletonTree(t, root)
	for name, content := range before {
		if current, ok := applied[name]; !ok || !reflect.DeepEqual(content, current) {
			t.Fatalf("generation changed existing entry %s", name)
		}
	}
	strict, err := NewBackend().Extract(root, []string{"src"})
	if err != nil {
		t.Fatalf("one-pass generated output does not load: %v", err)
		return
	}
	if len(strict.Diagnostics) > 0 {
		t.Fatalf("generated bindings are invalid: %+v", strict.Diagnostics)
	}
	realized := map[string]bool{}
	methods := map[string]bool{}
	for _, edge := range strict.Fragment.Edges {
		if edge.Kind == graph.EdgeKindRealizes {
			realized[edge.To] = true
		}
	}
	for _, node := range strict.Fragment.Nodes {
		if node.Kind == graph.NodeKindOp && node.Form == "method" {
			methods[lastName(node.Id)] = true
		}
	}
	for _, id := range []string{("demo.Counter" + upperFirst(sample.Name)), "demo.Port", "demo.Input", "demo.State", "demo.Failure", "demo.work"} {
		if !realized[id] {
			t.Fatalf("planned declaration %s is not realized", id)
		}
	}
	if sample.External && !realized["gateway.charge"] {
		t.Fatal("external client was not realized")
	}
	if sample.External {
		installed := false
		for _, edge := range strict.Fragment.Edges {
			if edge.Kind == graph.EdgeKindSubstitutes && edge.To == "gateway.charge" {
				installed = true
				if len(edge.Attrs["nativeSignature"]) != 1 || edge.Attrs["nativeSignature"][0] != "true" {
					t.Fatal("installer skeleton does not preserve the native client signature")
				}
			}
		}
		if !installed {
			t.Fatal("external replies have no installer skeleton")
		}
	}
	factory := false
	for _, edge := range strict.Fragment.Edges {
		if edge.Kind == graph.EdgeKindGenerates && edge.To == "demo.manual" {
			for _, name := range edge.Attrs["symbols"] {
				if name == "n" {
					factory = true
				}
			}
		}
	}
	if !factory {
		t.Fatal("missing generated symbol factory was not bootstrapped")
	}
	if sample.ExistingType {
		command := exec.Command("go", "run", "./inheritance-probe")
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("inherited or explicit child behavior is wrong: %v\n%s", err, output)
		}
	}
	if !methods["Bump"] || !methods["Read"] {
		t.Fatalf("type methods are incomplete: %v", methods)
	}
	next := skeletonFixtureGraph(t, sample)
	if err = next.AddFragment(strict.Fragment); err != nil {
		t.Fatal(err)
		return
	}
	if ds := next.Derive(); len(ds) > 0 {
		t.Fatalf("re-extracted graph has unresolved bindings: %+v", ds)
		return
	}
	replanned := NewBackend().Generate(lang.GenPlan{Root: root, Roots: []string{"src"}, Graph: &next})
	if len(replanned.Diagnostics) > 0 {
		t.Fatalf("replanning failed: %+v", replanned.Diagnostics)
		return
	}
	if after := skeletonTree(t, outside, true); !reflect.DeepEqual(outsideBefore, after) {
		t.Fatal("replanning changed outside module or workspace data")
	}
	firstGenerated := map[string][]byte{}
	for _, edit := range generated.Edits {
		if !edit.CreateOnly {
			firstGenerated[edit.Path] = edit.Content
		}
	}
	for _, edit := range replanned.Edits {
		if edit.CreateOnly {
			t.Fatalf("replanning repeats human-owned skeleton %s", edit.Path)
		}
		if !bytes.Equal(firstGenerated[edit.Path], edit.Content) {
			t.Fatalf("replanning changed generated bytes %s", edit.Path)
		}
	}
}

type skeletonEntry struct {
	Mode             os.FileMode
	Contents, Target string
	Modified         int64
}

func skeletonTree(t rapid.TB, root string, directories ...bool) map[string]skeletonEntry {
	out := map[string]skeletonEntry{}
	err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		// Directory mtimes change when the plan is applied; existing file and
		// link contents/modes are the preservation oracle across application.
		if entry.IsDir() && (len(directories) == 0 || !directories[0]) {
			return nil
		}
		item := skeletonEntry{Mode: info.Mode(), Modified: info.ModTime().UnixNano()}
		if entry.Type()&os.ModeSymlink != 0 {
			item.Target, err = os.Readlink(name)
		} else if !entry.IsDir() {
			var content []byte
			content, err = os.ReadFile(name)
			item.Contents = string(content)
		}
		out[filepath.ToSlash(rel)] = item
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func writeSkeletonFixture(t rapid.TB, root, name string, content []byte) {
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
		return
	}
	if err := os.WriteFile(target, content, 0o640); err != nil {
		t.Fatal(err)
	}
}
func attachSkeletonCode(t rapid.TB, root string, g *graph.Graph) {
	extracted, err := NewBackend().ExtractForGeneration(root, []string{"src"})
	if err != nil {
		t.Fatal(err)
		return
	}
	if err = g.AddFragment(extracted.Fragment); err != nil {
		t.Fatal(err)
		return
	}
	if ds := g.Derive(); len(ds) > 0 {
		t.Fatalf("fixture graph has unresolved bindings: %+v", ds)
	}
}
func skeletonFixtureGraph(t rapid.TB, sample SkeletonSample) graph.Graph {
	g := graph.NewGraph()
	add := func(id string, kind graph.NodeKind, form, parent string, attrs map[string][]string) {
		n := graph.Node{Id: id, Layer: graph.LayerDesign, Kind: kind, Form: form, Source: graph.SourceDeclared, Origin: diag.Position{Path: "design/component.uc", Line: 1, Column: 1}, Attrs: attrs}
		if err := g.AddNode(n); err != nil {
			t.Fatal(err)
			return
		}
		if parent != "" {
			if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindContains, From: parent, To: id, Source: graph.SourceDeclared}); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("demo", graph.NodeKindContainer, "component", "", map[string][]string{"paths": {"src/**"}})
	add(("demo.Counter" + upperFirst(sample.Name)), graph.NodeKindType, "object", "demo", map[string][]string{"fields": {"value: int"}})
	add(("demo.Counter" + upperFirst(sample.Name) + ".bump"), graph.NodeKindOp, "op", ("demo.Counter" + upperFirst(sample.Name)), map[string][]string{"signature": {"(delta: int) -> int"}})
	add("demo.Port", graph.NodeKindType, "interface", "demo", nil)
	add("demo.Port.read", graph.NodeKindOp, "op", "demo.Port", map[string][]string{"signature": {"() -> int"}, "pure": {"true"}})
	if sample.ExistingType {
		add("demo.Child", graph.NodeKindType, "object", "demo", nil)
		if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindExtends, From: "demo.Child", To: "demo.Counter" + upperFirst(sample.Name), Source: graph.SourceDeclared}); err != nil {
			t.Fatal(err)
		}
		if sample.ExistingMethod {
			if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindImplements, From: "demo.Child", To: "demo.Port", Source: graph.SourceDeclared}); err != nil {
				t.Fatal(err)
			}
			add("demo.ExplicitChild", graph.NodeKindType, "object", "demo", nil)
			add("demo.ExplicitChild.read", graph.NodeKindOp, "op", "demo.ExplicitChild", map[string][]string{"signature": {"() -> int"}, "pure": {"true"}})
			if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindExtends, From: "demo.ExplicitChild", To: "demo.Counter" + upperFirst(sample.Name), Source: graph.SourceDeclared}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindImplements, From: ("demo.Counter" + upperFirst(sample.Name)), To: "demo.Port", Source: graph.SourceDeclared}); err != nil {
		t.Fatal(err)
	}
	add("demo.Input", graph.NodeKindType, "record", "demo", map[string][]string{"fields": {"value: int"}})
	add("demo.State", graph.NodeKindType, "enum", "demo", map[string][]string{"values": {"ready", "done"}})
	add("demo.Failure", graph.NodeKindType, "error", "demo", map[string][]string{"fields": {"reason: string"}})
	add("demo.work", graph.NodeKindOp, "op", "demo", map[string][]string{"signature": {"(n: int) -> int raises demo.Failure"}})
	add("demo.manual", graph.NodeKindProperty, "invariant", "demo", map[string][]string{"symbols": {"n: int generated"}, "manual": {"Independent environment observation"}})
	add("demo.positive", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{"symbols": {"n: int"}, "call": {"work(n)"}, "ensure": {"result >= 0"}})
	if sample.External {
		add("gateway", graph.NodeKindActor, "external", "", nil)
		add("gateway.charge", graph.NodeKindOp, "op", "gateway", map[string][]string{"signature": {"(n: int) -> int"}})
		add("demo.gateway-works", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{"symbols": {"n: int", "answers: replies<gateway.charge>"}, "call": {"gateway.charge(n)"}, "ensure": {"result >= 0"}})
		if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindUses, From: "demo", To: "gateway", Source: graph.SourceDeclared}); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func checkMalformedSkeletonModule(t rapid.TB, sample SkeletonSample) {
	root, err := os.MkdirTemp("", "unicode-skeleton-invalid-module-")
	if err != nil {
		t.Fatal(err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	writeSkeletonFixture(t, root, "go.mod", []byte("this is not a module declaration\n"))
	before := skeletonTree(t, root, true)
	g := skeletonFixtureGraph(t, SkeletonSample{Name: sample.Name})
	generated := NewBackend().Generate(lang.GenPlan{Root: root, Roots: []string{"src"}, Graph: &g})
	if len(generated.Diagnostics) == 0 {
		t.Fatal("a malformed main go.mod was ignored during skeleton generation")
	}
	if after := skeletonTree(t, root, true); !reflect.DeepEqual(before, after) {
		t.Fatal("a failed malformed-module plan changed source entries")
	}
}

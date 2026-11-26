package main

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/design"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang/golang"
	"github.com/lincyaw/unicode/internal/syntax"
)

// This file draws small repositories whose design files and Go code agree,
// and builds their graph by calling the components directly, as the oracle
// of the flows check-code, build-snapshot and query-graph.

// modulePath is the Go module of every drawn repository. The module requires
// a stand-in for rapid, so that it loads offline; it is a nested module, which
// the source root "." does not reach.
const modulePath = "example.com/m"

var moduleFiles = map[string]string{
	"go.mod": "module " + modulePath + "\n\ngo 1.26\n\nrequire pgregory.net/rapid v0.0.0\n\n" +
		"replace pgregory.net/rapid => ./rapid\n",
	"rapid/go.mod": "module pgregory.net/rapid\n\ngo 1.26\n",
	"rapid/rapid.go": "package rapid\n\ntype TB interface{ Fatalf(format string, args ...any) }\n\ntype T struct{}\n\n" +
		"type Generator[V any] struct{ draw func(*T) V }\n\n" +
		"func Custom[V any](draw func(*T) V) *Generator[V] { return &Generator[V]{draw: draw} }\n",
}

// componentNames are valid as component names and as Go package names.
var componentNames = []string{"order", "stock", "bill", "mail", "audit"}

// repoComponent is one component of a drawn repository. It declares a record
// Item, an operation make that builds one, a flow makes in which the person
// calls make, and a property keeps about make; its Go package realizes Item
// and make, and its test file holds the two predicates. uses names an
// earlier component, whose Item the field other refers to.
type repoComponent struct {
	name      string
	focus     string
	uses      string
	generated bool // the property's symbol is an Item that a generator draws
	cased     bool // the property declares a case
	recorded  int  // the value of a recorded case, or -1 for no corpus file
}

// repo is the model of a drawn repository.
type repo struct {
	ddd        bool
	components []repoComponent
}

func drawRepo(t *rapid.T) repo {
	r := repo{ddd: rapid.Bool().Draw(t, "ddd")}
	names := rapid.Permutation(componentNames).Draw(t, "names")
	for i, name := range names[:rapid.IntRange(1, 3).Draw(t, "components")] {
		c := repoComponent{name: name, focus: rapid.SampledFrom([]string{"core", "support", "utility"}).Draw(t, "focus"), recorded: -1}
		if i > 0 && rapid.Bool().Draw(t, "uses?") {
			c.uses = r.components[rapid.IntRange(0, i-1).Draw(t, "used")].name
		}
		c.generated = rapid.IntRange(0, 2).Draw(t, "generated?") == 0
		if !c.generated {
			c.cased = rapid.Bool().Draw(t, "cased")
			if rapid.Bool().Draw(t, "recorded?") {
				c.recorded = rapid.IntRange(0, 9).Draw(t, "recorded")
			}
		}
		r.components = append(r.components, c)
	}
	return r
}

// texts writes the repository's files: design/, corpus/, the module and the code.
func (r repo) texts(t *rapid.T) map[string]string {
	files := maps.Clone(moduleFiles)
	system := "system sys \"the system\"\n\n"
	if r.ddd {
		system += "notation ddd\n\n"
	}
	files["design/system.uc"] = canonical(t, "design/system.uc", system+"language go {\nroot \".\"\n}\n\nactor who person \"a person\"\n")
	for _, c := range r.components {
		path := "design/" + c.name + ".uc"
		files[path] = canonical(t, path, c.design())
		files["internal/"+c.name+"/"+c.name+".go"] = c.code()
		files["internal/"+c.name+"/"+c.name+"_test.go"] = c.predicates()
		if c.recorded >= 0 {
			path := "corpus/" + c.name + "/keeps.uc"
			files[path] = canonical(t, path, fmt.Sprintf("corpus %s.keeps\n\ncase r-%08x coverage {\nn = %d\n}\n", c.name, 0xc0de+c.recorded, c.recorded))
		}
	}
	return files
}

func (c repoComponent) design() string {
	var b strings.Builder
	fmt.Fprintf(&b, "component %s \"the %s component\" {\nfocus %s\npaths \"internal/%s/**\"\n", c.name, c.name, c.focus, c.name)
	if c.uses != "" {
		fmt.Fprintf(&b, "uses %s\n", c.uses)
	}
	b.WriteString("}\n\ntype Item record \"an item\" {\nfield n: int\n")
	if c.uses != "" {
		fmt.Fprintf(&b, "field other: optional<%s.Item>\n", c.uses)
	}
	b.WriteString("}\n\nop make(n: int) -> Item \"makes an item\"\n\n")
	b.WriteString("flow makes \"the person makes an item\" {\nforall n: int\nwho -> make(n)\n}\n\n")
	b.WriteString("property keeps invariant \"an item keeps its number\" {\nabout make\n")
	if c.generated {
		b.WriteString("forall item: Item generated \"an item\"\n}\n")
		return b.String()
	}
	b.WriteString("forall n: int\nwhere n in 0..9\n")
	if c.cased {
		b.WriteString("case three {\nn = 3\n}\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func (c repoComponent) code() string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Package %s is the %s component.\npackage %s\n\n", c.name, c.name, c.name)
	if c.uses != "" {
		fmt.Fprintf(&b, "import \"%s/internal/%s\"\n\n", modulePath, c.uses)
	}
	fmt.Fprintf(&b, "// Item is an item.\n//\n//uc:realizes %s.Item\ntype Item struct {\n\tN int\n", c.name)
	if c.uses != "" {
		fmt.Fprintf(&b, "\tOther *%s.Item\n", c.uses)
	}
	fmt.Fprintf(&b, "}\n\n// Make makes an item.\n//\n//uc:realizes %s.make\nfunc Make(n int) Item {\n\treturn Item{N: n}\n}\n", c.name)
	return b.String()
}

func (c repoComponent) predicates() string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport \"pgregory.net/rapid\"\n\n", c.name)
	fmt.Fprintf(&b, "//uc:checks %s.makes\nfunc checkFlowMakes(t rapid.TB, n int) {\n\t_ = Make(n)\n}\n\n", c.name)
	if c.generated {
		fmt.Fprintf(&b, "//uc:checks %s.keeps\nfunc checkKeeps(t rapid.TB, item Item) {\n\tif Make(item.N).N != item.N {\n\t\tt.Fatalf(\"Make does not keep %%d\", item.N)\n\t}\n}\n\n", c.name)
		fmt.Fprintf(&b, "//uc:generates %s.keeps.item\nfunc genItem() *rapid.Generator[Item] {\n\treturn rapid.Custom(func(*rapid.T) Item { return Item{} })\n}\n", c.name)
		return b.String()
	}
	fmt.Fprintf(&b, "//uc:checks %s.keeps\nfunc checkKeeps(t rapid.TB, n int) {\n\tif Make(n).N != n {\n\t\tt.Fatalf(\"Make does not keep %%d\", n)\n\t}\n}\n", c.name)
	return b.String()
}

func repoFiles(texts map[string]string) map[string]RepoFile {
	files := map[string]RepoFile{}
	for p, text := range texts {
		files[p] = RepoFile{Content: text, Mode: 0o644}
	}
	return files
}

// genRepo draws a small repository whose design, corpus and Go code agree:
// building it gives no diagnostic.
//
//uc:generates cli.build-snapshot.files
//uc:generates cli.query-graph.files
func genRepo() *rapid.Generator[map[string]RepoFile] {
	return rapid.Custom(func(t *rapid.T) map[string]RepoFile {
		return repoFiles(drawRepo(t).texts(t))
	})
}

// built is what calling the components directly gives for a repository:
// the graph, and the diagnostics of building it as the CLI's JSON writes them.
type built struct {
	graph graph.Graph
	diags []diagnosticJSON
}

// buildDirectly builds the graph of the repository at root, whose files are
// files, by calling syntax.KindOf, syntax.Parse, design.Load, the Go
// backend's Extract, AddFragment and Derive directly. The diagnostics that
// the CLI itself makes (cli.*) carry only their code and position: those of
// reading the files, cli.unsupported-language at a configured language that
// has no backend, and cli.extract-failed at one whose code Extract refuses.
func buildDirectly(t rapid.TB, root string, files map[string]RepoFile) built {
	want, trees := readExpected(files)
	loaded := design.Load(trees)
	g := loaded.Graph
	want = append(want, diagnosticsJSON(loaded.Diagnostics)...)
	cli := func(code string, at diag.Position) {
		want = append(want, diagnosticJSON{Code: code, Subject: at.Path, Path: at.Path, Line: at.Line, Column: at.Column})
	}
	// Each language is declared where its first language block is, with the roots of all of them.
	var languages []string
	declared := map[string]diag.Position{}
	roots := map[string][]string{}
	for _, f := range trees {
		for _, d := range f.Decls {
			if d, ok := d.(*syntax.LanguageDecl); ok && f.Path == "design/system.uc" {
				if _, seen := declared[d.Name]; !seen {
					languages = append(languages, d.Name)
					declared[d.Name] = diag.Position{Path: f.Path, Line: d.Pos.Line, Column: d.Pos.Column}
				}
				for _, line := range d.Lines {
					if r, ok := line.(*syntax.Root); ok {
						roots[d.Name] = append(roots[d.Name], string(r.Path))
					}
				}
			}
		}
	}
	for _, language := range languages {
		if language != "go" {
			cli("cli.unsupported-language", declared[language])
			continue
		}
		extracted, err := golang.NewBackend().Extract(root, roots[language])
		if err != nil {
			cli("cli.extract-failed", declared[language])
			continue
		}
		if err := g.AddFragment(extracted.Fragment); err != nil {
			t.Fatalf("AddFragment refused the code that the Go backend extracted: %v", err)
		}
		want = append(want, diagnosticsJSON(extracted.Diagnostics)...)
	}
	return built{graph: g, diags: append(want, diagnosticsJSON(g.Derive())...)}
}

// checkDirectly adds to buildDirectly what check.Conform gives: its
// diagnostics, and the statuses.
func checkDirectly(t rapid.TB, root string, files map[string]RepoFile) ([]diagnosticJSON, map[string]graph.Status) {
	b := buildDirectly(t, root, files)
	checked := check.Conform(&b.graph)
	return sortJSON(append(b.diags, diagnosticsJSON(checked.Diagnostics)...)), checked.Statuses
}

// sameDiagnostics reports the first difference between the diagnostics the
// CLI gave and those calling the components directly gives, both sorted; a
// diagnostic of the CLI's own is compared by its code and position, and must
// carry its subject, cause and fix. It returns "" when they agree.
func sameDiagnostics(got, want []diagnosticJSON) string {
	if len(got) != len(want) {
		return fmt.Sprintf("the CLI reported %d diagnostics, calling the components directly gives %d\ngot: %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i, g := range got {
		w := want[i]
		switch {
		case g.Code != w.Code || g.Path != w.Path || g.Line != w.Line || g.Column != w.Column:
			return fmt.Sprintf("diagnostic %d is %s at %s:%d:%d, yet it should be %s at %s:%d:%d", i+1, g.Code, g.Path, g.Line, g.Column, w.Code, w.Path, w.Line, w.Column)
		case strings.HasPrefix(g.Code, "cli.") && (g.Subject != g.Path || g.Message == "" || g.Fix == ""):
			return fmt.Sprintf("the diagnostic %s of %s lacks its subject, cause or fix: %+v", g.Code, g.Path, g)
		case !strings.HasPrefix(g.Code, "cli.") && g != w:
			return fmt.Sprintf("the diagnostic %s is reported as %+v, yet the component gives %+v", g.Code, g, w)
		}
	}
	return ""
}

// gitCommit commits every file of the repository at root into a new git
// repository there.
func gitCommit(t rapid.TB, root string) {
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=unicode", "-c", "user.email=unicode@example.com", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null",
			"commit", "-q", "-m", "the drawn repository"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup failed: git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// sortedKeys lists a map's keys in order.
func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// BodySample varies a closure's value independently of source layout.
//
//uc:realizes lang.golang.BodySample
type BodySample struct {
	Value   int
	Changed bool
	Spaced  bool
}

//uc:generates lang.golang.implementation-content-is-normalized.sample
func genBodySample() *rapid.Generator[BodySample] {
	return rapid.Custom(func(t *rapid.T) BodySample {
		return BodySample{
			Value:   rapid.IntRange(-1000, 1000).Draw(t, "value"),
			Changed: rapid.Bool().Draw(t, "changed"),
			Spaced:  rapid.Bool().Draw(t, "spaced"),
		}
	})
}

// sourceContent independently reads the fixture's declarations without
// comments. The expectation is paired with model nodes by source position.
func sourceContent(t rapid.TB, name, src string) map[diag.Position]map[string]string {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, name, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("the content fixture is not valid Go: %v", err)
	}
	out := map[diag.Position]map[string]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		var at token.Pos
		contents := map[string]ast.Node{}
		switch node := node.(type) {
		case *ast.FuncDecl:
			at = node.Name.Pos()
			header := *node
			header.Body = nil
			contents["declaration"] = &header
			if node.Body != nil {
				contents["body"] = node.Body
			}
		case *ast.TypeSpec:
			at, contents["declaration"] = node.Name.Pos(), node
		default:
			return true
		}
		where := positions.Position(at)
		values := map[string]string{}
		for key, content := range contents {
			var canonical bytes.Buffer
			if err := format.Node(&canonical, token.NewFileSet(), content); err != nil {
				t.Fatalf("cannot format the fixture declaration: %v", err)
			}
			values[key] = canonical.String()
		}
		out[diag.Position{Path: name, Line: where.Line, Column: where.Column}] = values
		return true
	})
	return out
}

func (x *expectation) expectContent(files map[string]string) {
	for _, file := range x.w.files() {
		contents := sourceContent(x.w.t, file.path, files[file.path])
		for id, node := range x.nodes {
			for key, value := range contents[node.Origin] {
				node.Attrs[key] = []string{value}
			}
			x.nodes[id] = node
		}
	}
}

func bodyFixture(value, method int, constraint string, spaced bool) string {
	source := fmt.Sprintf(`package subject

//uc:realizes model.Box
type Box[T %s] struct { Value T; Slots [2]int }
type Reader interface { Read() int }

//uc:realizes model.eval
func Eval() int { read := func() int { return %d }; return read() }
func (box Box[T]) Count() int { return %d }
func Generic[T %s](value T) T { return value }
`, constraint, value, method, constraint)
	if spaced {
		source = strings.Replace(source, "func (box Box[T]) Count() int", "func (box /* receiver */ Box[T]) Count( ) int", 1)
		source = strings.Replace(source, "func Generic[T "+constraint+"](value T) T", "func Generic[\nT /* constraint */ "+constraint+",\n](value /* parameter */ T) T", 1)
		source = strings.Replace(source, "package subject", "\n\n// A layout-only package comment.\npackage subject", 1)
		source = strings.Replace(source, "{ Value T; Slots [2]int }", "{\n// A field comment.\nValue T\n\nSlots [2]int // A line comment.\n}", 1)
		source = strings.Replace(source, "{ read := func() int { return ", "{\n// A body comment.\nread := func() int {\nreturn ", 1)
		source = strings.Replace(source, "; return read() }", "\n// A closure comment.\nreturn read()\n}", 1)
		source = strings.Replace(source, " };", "\n};", 1)
	}
	return source
}

//uc:checks lang.golang.implementation-content-is-normalized
func checkImplementationContentIsNormalized(t rapid.TB, sample BodySample) {
	dir, err := os.MkdirTemp("", "unicode-content-")
	if err != nil {
		t.Fatalf("cannot make a content module: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	changed := sample.Value
	if sample.Changed {
		changed++
	}
	files := map[string]string{
		"go.mod":             "module example.com/content\n\ngo 1.26\n",
		"base/source.go":     bodyFixture(sample.Value, sample.Value, "~int", false),
		"layout/source.go":   bodyFixture(sample.Value, sample.Value, "~int", sample.Spaced),
		"changed/source.go":  bodyFixture(changed, sample.Value, "~int", sample.Spaced),
		"method/source.go":   bodyFixture(sample.Value, sample.Value+1, "~int", sample.Spaced),
		"typed/source.go":    bodyFixture(sample.Value, sample.Value, "~int64", sample.Spaced),
		"receiver/source.go": strings.Replace(bodyFixture(sample.Value, sample.Value, "~int", sample.Spaced), "Box[T]) Count", "*Box[T]) Count", 1),
	}
	expected := map[string]map[diag.Position]map[string]string{}
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, ".go") {
			expected[name] = sourceContent(t, name, src)
		}
	}
	extracted, err := NewBackend().Extract(dir, []string{"."})
	if err != nil {
		t.Fatalf("cannot extract the content fixture: %v", err)
	}
	if len(extracted.Diagnostics) != 0 {
		t.Fatalf("the content fixture has binding diagnostics: %+v", extracted.Diagnostics)
	}
	g := graph.NewGraph()
	for _, node := range extracted.Fragment.Nodes {
		if err := g.AddNode(node); err != nil {
			t.Fatal(err)
		}
		for key, want := range expected[node.Origin.Path][node.Origin] {
			if !slices.Equal(node.Attrs[key], []string{want}) {
				t.Fatalf("%s %s is %q, want the independently formatted AST %q", node.Id, key, node.Attrs[key], want)
			}
		}
		if node.Form == "abstract" {
			for _, key := range []string{"body", "declaration"} {
				if _, exists := node.Attrs[key]; exists {
					t.Fatalf("the abstract operation %s has native %s content", node.Id, key)
				}
			}
		}
	}
	get := func(pkg, name string) *graph.Node {
		id := "go:example.com/content/" + pkg + "." + name
		node := g.Node(id)
		if node == nil {
			t.Fatalf("extraction omitted %s", id)
		}
		return node
	}
	body := func(pkg, name string) string {
		values := get(pkg, name).Attrs["body"]
		if len(values) != 1 || values[0] == "" {
			t.Fatalf("%s.%s has no body content", pkg, name)
		}
		return values[0]
	}
	declaration := func(pkg, name string) string {
		values := get(pkg, name).Attrs["declaration"]
		if len(values) != 1 || values[0] == "" {
			t.Fatalf("%s.%s has no declaration content", pkg, name)
		}
		return values[0]
	}
	for _, name := range []string{"Eval", "Box.Count", "Generic"} {
		if declaration("base", name) != declaration("layout", name) {
			t.Fatalf("layout, comments or positions changed the native header of %s", name)
		}
	}
	if body("base", "Box.Count") != body("receiver", "Box.Count") || declaration("base", "Box.Count") == declaration("receiver", "Box.Count") {
		t.Fatal("a pointer receiver change was not isolated in the method declaration")
	}
	if g.Digest(get("base", "Box.Count").Id) == g.Digest(get("receiver", "Box.Count").Id) {
		t.Fatal("the fixture's pointer receiver change did not alter its operation digest")
	}
	if body("base", "Generic") != body("typed", "Generic") || declaration("base", "Generic") == declaration("typed", "Generic") {
		t.Fatal("a generic function constraint change was not isolated in its declaration")
	}
	if g.Digest(get("base", "Generic").Id) == g.Digest(get("typed", "Generic").Id) {
		t.Fatal("the fixture's generic function constraint change did not alter its operation digest")
	}
	if body("base", "Eval") != body("layout", "Eval") || g.Digest(get("base", "Eval").Id) != g.Digest(get("layout", "Eval").Id) {
		t.Fatal("layout, comments or source positions changed the normalized operation")
	}
	if equal := body("base", "Eval") == body("changed", "Eval"); equal == sample.Changed {
		t.Fatal("the closure's return expression did not determine its body's content")
	}
	if sample.Changed && g.Digest(get("base", "Eval").Id) == g.Digest(get("changed", "Eval").Id) {
		t.Fatalf("the content digest did not distinguish the fixture constants %s and %s", strconv.Itoa(sample.Value), strconv.Itoa(changed))
	}
	if body("base", "Box.Count") == body("method", "Box.Count") {
		t.Fatal("a changed method body has unchanged content")
	}
	if declaration("base", "Box.Count") != declaration("method", "Box.Count") || declaration("base", "Eval") != declaration("changed", "Eval") {
		t.Fatal("a function body changed its native declaration header")
	}
	if !slices.Equal(get("base", "Box").Attrs["declaration"], get("method", "Box").Attrs["declaration"]) || g.Digest(get("base", "Box").Id) != g.Digest(get("method", "Box").Id) {
		t.Fatal("a method body changed its containing type's own content")
	}
	if !slices.Equal(get("base", "Box").Attrs["declaration"], get("layout", "Box").Attrs["declaration"]) {
		t.Fatal("type comments, layout or source positions changed the normalized declaration")
	}
	if slices.Equal(get("base", "Box").Attrs["declaration"], get("typed", "Box").Attrs["declaration"]) {
		t.Fatal("a generic constraint change was lost from the type declaration")
	}
	for name, before := range files {
		after, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || string(after) != before {
			t.Fatalf("content extraction changed the source file %s: %v", name, err)
		}
	}
}

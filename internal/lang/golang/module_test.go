package golang

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"pgregory.net/rapid"
)

// This file holds the model of a random Go module: its packages, files and
// declarations, and which declarations a file can name. draw_test.go draws
// the declarations, write_test.go writes them as source, and expect_test.go
// computes the nodes and edges that extraction should give.

// Module is a random Go module: its files, and the nodes, edges and
// diagnostics that extraction should give, written by describeNode,
// describeEdge and describeDiagnostic.
//
//uc:realizes lang.golang.Module
type Module struct {
	Files       map[string]string
	Nodes       []string
	Edges       []string
	Diagnostics []string
}

const modulePath = "example.com/m"

// marker is the unimplemented marker of docs/design.md §6.1.
const marker = "unicode: unimplemented"

// The module requires a stand-in for rapid, so that it loads offline and a
// test can call rapid.Check.
var rapidFiles = map[string]string{
	"rapid/go.mod": "module pgregory.net/rapid\n\ngo 1.26\n",
	"rapid/rapid.go": "package rapid\n\ntype TB interface{ Helper() }\n\ntype T struct{}\n\n" +
		"func Check(t TB, prop func(*T)) {}\n",
}

const goMod = "module " + modulePath + "\n\ngo 1.26\n\nrequire pgregory.net/rapid v0.0.0\n\n" +
	"replace pgregory.net/rapid => ./rapid\n"

// genModule draws a random module of up to three packages, some with test
// files and an external test package, holding types, functions, methods,
// calls, imports, embedding, interfaces and their implementations, and
// //uc: bindings, some of them not well-formed or misplaced.
//
//uc:generates lang.golang.extracts-generated-module.module
func genModule() *rapid.Generator[Module] {
	return rapid.Custom(func(t *rapid.T) Module {
		w := &world{t: t}
		w.drawPackages()
		for range rapid.IntRange(0, 6).Draw(t, "types") {
			w.drawType()
		}
		for _, ty := range slices.Clone(w.types) {
			w.drawMethods(ty)
			w.drawConsts(ty)
		}
		for range rapid.IntRange(0, 5).Draw(t, "funcs") {
			w.drawFunc(w.anyFile("funcFile"), "")
		}
		for _, f := range w.files() {
			// A function named like a test outside a test file is none.
			most := 0
			if f.test {
				most = 2
			} else if rapid.IntRange(0, 4).Draw(t, "testOutside") == 0 {
				most = 1
			}
			for range rapid.IntRange(0, most).Draw(t, "tests") {
				w.drawFunc(f, rapid.SampledFrom([]string{"Test", "Fuzz", "Testlower"}).Draw(t, "testName"))
			}
		}
		for _, fn := range w.funcs {
			w.drawBody(fn)
		}
		w.drawLoose()
		return w.module()
	})
}

// The model.

type world struct {
	t        *rapid.T
	count    int
	packages []*gpkg
	types    []*gtype
	funcs    []*gfunc // functions and methods
}

type gpkg struct {
	name, dir, path string
	index           int   // a package imports only packages of a smaller index
	tested          *gpkg // for an external test package, the package it tests
	files           []*gfile
}

type gfile struct {
	pkg                *gpkg
	path               string // relative to the module's root
	test               bool
	doc                []binding // the package's doc comment, its sentence first
	decls              []any     // *gtype, *ggroup, *gfunc, *gconst or *gloose, in the order written
	header             string
	headerAfterPackage bool
}

type gtype struct {
	file     *gfile
	name     string
	form     string // struct, interface or defined
	fields   []gfield
	embed    *gtype // a struct's embedded struct
	embedPtr bool
	embeds   []*gtype // an interface's embedded interfaces
	sigs     []*gsig  // an interface's explicit methods
	under    *gexpr   // a defined type's underlying type
	methods  []*gfunc
	doc      []binding
}

// ggroup is a parenthesized type declaration; its own doc comment binds nothing.
type ggroup struct {
	types []*gtype
	doc   []binding
}

type gfield struct {
	name string
	typ  *gexpr
	doc  []binding
}

type gsig struct {
	pkg     *gpkg // where the method or function is declared
	name    string
	params  []gparam
	results []*gexpr
	doc     []binding // an interface's method's
}

type gparam struct {
	name string // "" when the parameters are unnamed
	typ  *gexpr
}

type gfunc struct {
	file     *gfile
	recv     *gtype
	ptrRecv  bool
	sig      *gsig
	testName string // Test, Fuzz or Testlower for a function named like a test
	body     []gstmt
	terminal string // the panic's message that ends the body, or ""
	doc      []binding
}

// gstmt is one statement of a body, or a comment line in it.
type gstmt struct {
	kind    string // call, assign, iface, closure, builtin, ref, rapid or comment
	target  *gfunc // what a call calls
	via     *gtype // the type whose value a method is called on
	method  *gsig  // an interface's method
	inner   *gstmt // a closure's statement
	name    string // an assignment's variable
	comment binding
}

// gconst is a constant of a defined type.
type gconst struct {
	typ  *gtype
	name string
}

// gloose is a declaration that is no node: a constant, a variable, an
// init function, or a comment that belongs to no declaration.
type gloose struct {
	kind string // const, var, trailing, init or floating
	name string
	doc  []binding
}

// gexpr is a type expression.
type gexpr struct {
	kind  string // basic, raw, named, ext, time, duration, error, pointer, slice, bytes, map or set
	basic string // a basic type's or a raw type's Go text; a map's or set's key
	named *gtype
	ext   [3]string // an external type's package path, package name and type name
	elem  *gexpr
}

// Names.

// fresh draws a Go name that no other declaration of the module has.
func (w *world) fresh(label string, exported bool) string {
	w.count++
	first := "[a-z]"
	if exported {
		first = "[A-Z]"
	}
	name := rapid.StringMatching(first+"[a-z]{0,2}").Draw(w.t, label) + strconv.Itoa(w.count)
	if token.IsKeyword(name) || types.Universe.Lookup(name) != nil {
		name += "x"
	}
	return name
}

func (w *world) exported(label string) bool { return rapid.IntRange(0, 3).Draw(w.t, label) > 0 }

// Packages and files.

func (w *world) drawPackages() {
	names := []string{"pa", "pb", "pc"}
	var regular []*gpkg
	for i := range rapid.IntRange(1, 3).Draw(w.t, "packages") {
		dir := names[i]
		switch {
		case i == 0 && rapid.Bool().Draw(w.t, "atRoot"):
			dir = ""
		case i > 0 && rapid.Bool().Draw(w.t, "nested"):
			parent := rapid.SampledFrom(regular).Draw(w.t, "parent")
			dir = strings.TrimPrefix(parent.dir+"/"+names[i], "/")
		}
		p := &gpkg{name: names[i], dir: dir, path: strings.TrimSuffix(modulePath+"/"+dir, "/"), index: i}
		for k := range rapid.IntRange(1, 2).Draw(w.t, "files") {
			p.files = append(p.files, &gfile{pkg: p, path: join(dir, fmt.Sprintf("%s_%d.go", p.name, k))})
		}
		if rapid.Bool().Draw(w.t, "internalTest") {
			p.files = append(p.files, &gfile{pkg: p, path: join(dir, p.name+"_0_test.go"), test: true})
		}
		regular = append(regular, p)
		w.packages = append(w.packages, p)
	}
	for _, p := range regular {
		if rapid.IntRange(0, 2).Draw(w.t, "externalTest") == 0 {
			q := &gpkg{name: p.name + "_test", dir: p.dir, path: p.path + "_test", index: len(names), tested: p}
			q.files = []*gfile{{pkg: q, path: join(p.dir, p.name+"_x_test.go"), test: true}}
			w.packages = append(w.packages, q)
		}
	}
	for _, p := range w.packages {
		for _, f := range p.files {
			f.header = rapid.SampledFrom([]string{"", "// Code generated by unicode. DO NOT EDIT.", "// Code generated by another tool. DO NOT EDIT.", "// Code generated by unicode. DO NOT EDIT"}).Draw(w.t, "generationHeader")
			f.headerAfterPackage = rapid.Bool().Draw(w.t, "headerAfterPackage")
		}
		if rapid.Bool().Draw(w.t, "packageDoc") {
			f := rapid.SampledFrom(p.files).Draw(w.t, "docFile")
			f.doc = append([]binding{{text: "// Package " + p.name + w.words("packageWords") + "."}}, w.misplaced("packageDocBindings")...)
		}
	}
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func (w *world) files() []*gfile {
	var files []*gfile
	for _, p := range w.packages {
		files = append(files, p.files...)
	}
	return files
}

func (w *world) anyFile(label string) *gfile { return rapid.SampledFrom(w.files()).Draw(w.t, label) }

// canImport tells whether a file of from may import p: a package imports
// packages of smaller index, an external test package any other package.
func canImport(from *gfile, p *gpkg) bool {
	return p.tested == nil && (from.pkg.tested != nil || p.index < from.pkg.index)
}

// visible tells whether a declaration in decl's file, with the given name,
// can be named from a file: in its own package unless it is in a test file
// and the file is not; in another package that the file may import when it
// is exported and not in a test file.
func visible(from, decl *gfile, name string) bool {
	if decl.pkg == from.pkg {
		return !decl.test || from.test
	}
	return canImport(from, decl.pkg) && token.IsExported(name) && !decl.test
}

func typeVisible(from *gfile, ty *gtype) bool { return visible(from, ty.file, ty.name) }

func exprVisible(from *gfile, e *gexpr) bool {
	switch e.kind {
	case "named":
		return typeVisible(from, e.named)
	case "pointer", "slice", "map":
		return exprVisible(from, e.elem)
	}
	return true
}

func sigVisible(from *gfile, sig *gsig) bool {
	for _, p := range sig.params {
		if !exprVisible(from, p.typ) {
			return false
		}
	}
	for _, r := range sig.results {
		if !exprVisible(from, r) {
			return false
		}
	}
	return true
}

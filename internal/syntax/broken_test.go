package syntax

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
)

// Broken is a valid file with one edit that breaks a rule of docs/format.md,
// and the code of that rule's diagnostic.
//
//uc:realizes syntax.Broken
type Broken struct {
	Kind FileKind
	Text string
	Code string
}

// breakage makes one rule-breaking edit to a valid sample.
type breakage struct {
	code string
	make func(t *rapid.T) (FileKind, string)
}

// inserted puts a character no file may contain at any byte of a valid sample.
func inserted(code, char string) breakage {
	return breakage{code, func(t *rapid.T) (FileKind, string) {
		s := genSample().Draw(t, "sample")
		at := rapid.IntRange(0, len(s.Text)).Draw(t, "at")
		return s.Kind, s.Text[:at] + char + s.Text[at:]
	}}
}

// appended puts a broken top-level declaration after the last one of a valid
// sample. Every kind of file parses every declaration, so the broken text is
// where the grammar or the lexis fails.
func appended(code, text string) breakage {
	return breakage{code, func(t *rapid.T) (FileKind, string) {
		s := genSample().Draw(t, "sample")
		return s.Kind, strings.TrimRight(s.Text, "\n ") + "\n\n" + text
	}}
}

// edited breaks a rule that the checks after parsing enforce: it changes the
// declarations of a valid component file and writes them with random layout.
func edited(code string, edit func(t *rapid.T, ds []Decl) []Decl) breakage {
	return breakage{code, func(t *rapid.T) (FileKind, string) {
		f := &File{Kind: FileKindComponent, Decls: genComponentDecls(t)}
		f.Decls = edit(t, f.Decls)
		w := &writer{t: t}
		w.file(f)
		return f.Kind, w.b.String()
	}}
}

// withType inserts a type among the types of a component file, so that only its own content breaks a rule.
func withType(d *TypeDecl) func(*rapid.T, []Decl) []Decl {
	return func(_ *rapid.T, ds []Decl) []Decl { return slices.Insert(ds, 1, Decl(d)) }
}

func record(lines ...TypeLine) *TypeDecl {
	return &TypeDecl{Name: "Broken", Form: "record", Summary: "一个类型", Lines: lines}
}

func intField(name Lower) *Field {
	return &Field{Name: name, Type: &BuiltinType{Name: "int"}}
}

// componentLines replaces the lines of the component declaration.
func componentLines(edit func([]ComponentLine) []ComponentLine) func(*rapid.T, []Decl) []Decl {
	return func(_ *rapid.T, ds []Decl) []Decl {
		c := ds[0].(*ComponentDecl)
		c.Lines = edit(c.Lines)
		return ds
	}
}

var breakages = []breakage{
	inserted("syntax.tab", "\t"),
	inserted("syntax.carriage-return", "\r"),
	inserted("syntax.control-character", "\x01"),
	inserted("syntax.invalid-utf8", "\xff"),
	appended("syntax.invalid-escape", `type Broken record "a\qb"`+"\n"),
	appended("syntax.unterminated-string", `type Broken record "abc`+"\n"),
	appended("syntax.unexpected-character", `type Broken record "s" @`+"\n"),
	appended("syntax.trailing-comment", `type Broken record "s" // 注释`+"\n"),
	appended("syntax.comment-in-value", "property p invariant \"p\" {\n  about x\n  case c {\n    x = [\n    // 注释\n    ]\n  }\n}\n"),
	appended("syntax.brace-position", "type Broken record \"s\" { label a\n}\n"),
	appended("syntax.brace-position", "type Broken record \"s\" {\n  label a }\n"),
	appended("syntax.misplaced-else", "flow f \"f\" {\n  alt \"a\" {\n  }\n  else \"b\" {\n  }\n}\n"),
	appended("syntax.unexpected-end", "type Broken record \"s\" {\n  label a\n"),
	appended("syntax.unexpected-token", `type Broken recor "s"`+"\n"),
	appended("syntax.unexpected-token", "op f(a int)\n"),
	appended("syntax.unexpected-token", "actor p person \"人\" {\n}\n"),
	appended("syntax.unexpected-token", "flow f \"f\" {\n  field a: int\n}\n"),
	edited("syntax.declaration-order", func(t *rapid.T, ds []Decl) []Decl { return append(ds, genOp(t), genType(t)) }),
	edited("syntax.member-order", withType(record(intField("a"), &Label{Name: "value"}))),
	edited("syntax.duplicate-member", componentLines(func(ls []ComponentLine) []ComponentLine {
		return append(ls, &Paths{Patterns: []Text{"x/**"}})
	})),
	edited("syntax.missing-member", componentLines(func(ls []ComponentLine) []ComponentLine {
		return slices.DeleteFunc(ls, func(l ComponentLine) bool { _, paths := l.(*Paths); return paths })
	})),
	edited("syntax.missing-header", func(_ *rapid.T, ds []Decl) []Decl { return ds[1:] }),
	edited("syntax.unknown-declaration", func(_ *rapid.T, ds []Decl) []Decl {
		return append(ds, &SystemDecl{Name: "broken", Summary: "一个系统"})
	}),
	edited("syntax.invalid-name", withType(&TypeDecl{Name: "broken", Form: "record", Summary: "一个类型"})),
	edited("syntax.invalid-reference", withType(record(&Extends{Type: TypeRef{Segments: []string{"a", "b"}}}))),
	edited("syntax.reserved-name", withType(record(intField("none")))),
	edited("syntax.empty-summary", withType(&TypeDecl{Name: "Broken", Form: "record", Summary: ""})),
	edited("syntax.summary-surrounding-space", withType(&TypeDecl{Name: "Broken", Form: "record", Summary: " 前面有空格"})),
	edited("syntax.summary-tab-or-newline", withType(&TypeDecl{Name: "Broken", Form: "record", Summary: "两\n行"})),
	edited("syntax.field-not-allowed", withType(&TypeDecl{Name: "Broken", Form: "enum", Summary: "一个枚举", Lines: []TypeLine{intField("a")}})),
	edited("syntax.value-outside-enum", withType(record(&EnumValue{Name: "a"}))),
	edited("syntax.on-without-call", func(_ *rapid.T, ds []Decl) []Decl {
		on := &On{Error: TypeRef{Segments: []string{"Failed"}}}
		return append(ds, &FlowDecl{Name: "broken", Summary: "一个流程", Lines: []FlowLine{on}})
	}),
	edited("syntax.not-an-integer", func(_ *rapid.T, ds []Decl) []Decl {
		about := &About{Refs: []Ref{{Segments: []string{"x"}}}}
		size := &IntervalLimit{Path: []Lower{"x"}, Op: "size", Low: "1.5", High: "2"}
		return append(ds, &PropertyDecl{Name: "broken", Form: "invariant", Summary: "一条性质", Lines: []PropertyLine{about, size}})
	}),
}

// genBroken draws a valid sample with one edit that breaks a rule, one kind of
// edit for each diagnostic code of syntax.
//
//uc:generates syntax.rejects-broken-text.broken
func genBroken() *rapid.Generator[Broken] {
	return rapid.Custom(func(t *rapid.T) Broken {
		b := rapid.SampledFrom(breakages).Draw(t, "breakage")
		kind, text := b.make(t)
		return Broken{Kind: kind, Text: text, Code: b.code}
	})
}

//uc:checks syntax.rejects-broken-text
func checkRejectsBrokenText(t rapid.TB, broken Broken) {
	parsed := Parse(samplePaths[broken.Kind], broken.Kind, []byte(broken.Text))
	if parsed.File != nil {
		t.Fatalf("text that breaks a rule should get %s, yet it parsed into a tree\ntext:\n%s", broken.Code, broken.Text)
	}
	if !slices.ContainsFunc(parsed.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == broken.Code }) {
		t.Fatalf("text that breaks a rule should get %s, yet it got:%s\ntext:\n%s", broken.Code, describeDiagnostics(parsed), broken.Text)
	}
}

func TestRejectsBrokenText(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkRejectsBrokenText(t, genBroken().Draw(t, "broken"))
	})
}

func FuzzRejectsBrokenText(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkRejectsBrokenText(t, genBroken().Draw(t, "broken"))
	}))
}

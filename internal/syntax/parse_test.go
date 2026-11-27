package syntax

import (
	"bytes"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
)

var syntaxCode = regexp.MustCompile(`^syntax\.[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

//uc:checks syntax.parse-is-total
func checkParseIsTotal(t rapid.TB, path string, kind FileKind, src []byte) {
	var parsed Parsed
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Parse panicked: %v\n%s", r, debug.Stack())
			}
		}()
		parsed = Parse(path, kind, src)
	}()
	if len(parsed.Diagnostics) == 0 {
		if parsed.File == nil || parsed.File.Path != path || parsed.File.Kind != kind {
			t.Fatalf("Parse gave no diagnostic, yet no tree of the file (path %q, kind %s)", path, kind)
		}
		return
	}
	lines := bytes.Split(src, []byte("\n"))
	for i, d := range parsed.Diagnostics {
		if i > 0 {
			if p := parsed.Diagnostics[i-1]; !inOrder(p, d) {
				t.Fatalf("the diagnostics are not ordered by line, column and code: %s at %d:%d comes before %s at %d:%d",
					p.Code, p.Position.Line, p.Position.Column, d.Code, d.Position.Line, d.Position.Column)
			}
		}
		at := d.Position
		switch {
		case !syntaxCode.MatchString(d.Code):
			t.Fatalf("the code %q is not of the form syntax.<kebab-name>", d.Code)
		case d.Message == "" || d.Fix == "":
			t.Fatalf("the diagnostic %s lacks its cause or its fix: cause %q, fix %q", d.Code, d.Message, d.Fix)
		case at.Path != path:
			t.Fatalf("the diagnostic %s names the path %q, not the parsed file %q", d.Code, at.Path, path)
		case at.Line < 1 || at.Line > len(lines) || at.Column < 1 || at.Column > len(lines[at.Line-1])+1:
			t.Fatalf("the diagnostic %s stands at %d:%d, outside the file of %d lines", d.Code, at.Line, at.Column, len(lines))
		}
	}
}

// inOrder tells whether a may come before b: an earlier line, an earlier
// column on the same line, or a code not after b's at the same place.
func inOrder(a, b diag.Diagnostic) bool {
	x, y := a.Position, b.Position
	switch {
	case x.Line != y.Line:
		return x.Line < y.Line
	case x.Column != y.Column:
		return x.Column < y.Column
	}
	return a.Code <= b.Code
}

// genPath draws a path of 1 to 64 bytes.
//
//uc:generates syntax.parse-is-total.path
func genPath() *rapid.Generator[string] {
	return rapid.StringN(1, 64, 64)
}

//uc:generates syntax.parse-is-total.kind
func genKind() *rapid.Generator[FileKind] {
	return rapid.SampledFrom([]FileKind{FileKindSystem, FileKindComponent, FileKindCorpus})
}

// fragments are pieces of the format and of what breaks it, for building
// sources that reach deep into the parser.
var fragments = []string{
	"system", "notation", "language", "root", "actor", "person", "external", "component", "label", "focus",
	"paths", "uses", "type", "extends", "implements", "field", "value", "op", "raises", "handles", "flow",
	"on", "async", "alt", "else", "opt", "loop", "emits", "property", "about", "forall", "generated",
	"where", "in", "size", "case", "corpus", "coverage", "record", "enum", "list", "map", "call", "int",
	"true", "none", "core", "ddd", "go", "Order", "order", "a-b", "x", "A.b", "a.B.c", "-1", "1.5", "0",
	"{", "}", "(", ")", "[", "]", "<", ">", ",", ":", ".", "..", "=", "->", "->>", "\"s\"", "\"\"",
	"\"a\\nb\"", "\"\\q\"", "\"", "\\", " ", "  ", "\n", "\n", "\n", "//c\n", "// x", "\t", "\r\n",
	"\x00", "\xff", "\xe4\xb8", "中", "@", "_", "#", "\u00a0",
}

// lineFragments are whole lines that, put into a valid file, break one rule
// of the grammar, or two.
var lineFragments = []string{
	"value v\n", "field f: int\n", "focus core\n", "paths \"p\"\n", "uses a\n", "about x\n", "label x\n",
	"label Bad\n", "root \"r\"\n", "where x size 1.5..2\n", "where x in 1..2\n", "where x in {a, b}\n",
	"forall x: int\n", "forall X: integer generated \"d\"\n", "else \"x\" {\n", "} else \"y\" {\n", "on E {\n",
	"}\n", "x = [1,\n", "x = [\n// c\n1]\n", "x = {a: 1, \"b\": [], 2: c(d: none)}\n", "x = f(a: 1,)\n",
	"type t record \"\"\n", "type T record \" x\"\n", "type T record \"a\\nb\"\n", "type T enum \"e\" {\n",
	"op Bad()\n", "op f(a: int) -> T raises E handles F \"s\"\n", "component a \"c\" {\n", "system s \"s\"\n",
	"notation uml\n", "language go {\n", "corpus a\n", "actor p person \"p\" {\n", "case c {\n", "case r-1 coverage {\n",
	"a -> b()\n", "a -> b() {\n", "a ->> b(c)\n", "A emits b.E(c)\n", "async a {\n", "alt \"a\" {\n", "opt \"o\" {\n",
	"flow f \"f\" {\n", "property p invariant \"p\" {\n", "// comment\n", "\n", "field a: int }\n",
	"type T record \"t\" { label x\n", "uses a.B-c\n", "extends a.b\n", "x = ,\n", "x = {: 1}\n", "where x\n",
	"field f: ,\n",
	"type E enum \"e\" {\n  field f: int\n}\n", "type R record \"r\" {\n  field f: ,\n  field a: int }\n",
	"property p invariant \"p\" {\n  about x\n  case c {\n    x = ,\n    y = {: 1}\n  }\n}\n",
	"property p invariant \"p\" {\n  about a.B-c\n  where x size 1.5..2\n}\n", "flow f \"f\" {\n  on E {\n  }\n}\n",
	"actor p person \"p\" {\n  op f()\n}\n", "op true(false: int)\n", "flow f \"f\" {\n  alt \"\" {\n  }\n}\n",
	"where x in 007..-0.50\n",
}

// splices are what a byte edit puts in place of the bytes it removes.
var splices = append(slices.Clone(fragments), "")

// genSrc draws 0 to 4096 bytes: raw bytes, a soup of fragments, a valid
// sample with random line and byte edits, or a valid sample cut short.
//
//uc:generates syntax.parse-is-total.src
func genSrc() *rapid.Generator[[]byte] {
	soup := rapid.Custom(func(t *rapid.T) []byte {
		parts := rapid.SliceOfN(rapid.SampledFrom(fragments), 0, 300).Draw(t, "fragments")
		var b bytes.Buffer
		for _, p := range parts {
			b.WriteString(p)
		}
		return b.Bytes()
	})
	edited := rapid.Custom(func(t *rapid.T) []byte {
		lines := strings.SplitAfter(genSample().Draw(t, "sample").Text, "\n")
		for range rapid.IntRange(1, 4).Draw(t, "edits") {
			i := rapid.IntRange(0, len(lines)-1).Draw(t, "line")
			j := rapid.IntRange(0, len(lines)-1).Draw(t, "other")
			switch rapid.IntRange(0, 4).Draw(t, "edit") {
			case 0:
				lines = slices.Insert(lines, i, lines[j])
			case 1:
				lines = slices.Delete(lines, i, i+1)
			case 2:
				lines[i], lines[j] = lines[j], lines[i]
			case 3:
				lines = slices.Insert(lines, i, rapid.SampledFrom(lineFragments).Draw(t, "insert"))
			default:
				l := lines[i]
				at := rapid.IntRange(0, len(l)).Draw(t, "at")
				end := rapid.IntRange(at, min(len(l), at+8)).Draw(t, "end")
				lines[i] = l[:at] + rapid.SampledFrom(splices).Draw(t, "splice") + l[end:]
			}
			if len(lines) == 0 {
				lines = []string{""}
			}
		}
		return []byte(strings.Join(lines, ""))
	})
	truncated := rapid.Custom(func(t *rapid.T) []byte {
		text := genSample().Draw(t, "sample").Text
		return []byte(text[:rapid.IntRange(0, len(text)).Draw(t, "cut")])
	})
	return rapid.Map(rapid.OneOf(rapid.SliceOfN(rapid.Byte(), 0, 4096), soup, edited, edited, truncated), func(b []byte) []byte {
		return b[:min(len(b), 4096)]
	})
}

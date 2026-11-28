package syntax

import (
	"bytes"
	"slices"
	"strings"

	"pgregory.net/rapid"
)

// RawText is a source and the byte widths of its lines, by line number from 1.
//
//uc:realizes syntax.RawText
type RawText struct {
	Src    []byte
	Widths map[int]int
}

// genRawText draws a source with genSrc and gives the widths of its lines,
// split at each LF, as the lines' byte counts.
//
//uc:generates syntax.parse-is-total.raw
func genRawText() *rapid.Generator[RawText] {
	return rapid.Map(genSrc(), func(src []byte) RawText {
		widths := map[int]int{}
		for i, line := range bytes.Split(src, []byte("\n")) {
			widths[i+1] = len(line)
		}
		return RawText{Src: src, Widths: widths}
	})
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
	"where", "in", "size", "codec", "encoded", "case", "corpus", "coverage", "record", "enum", "list", "map", "call", "int",
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
		lines := strings.SplitAfter(string(genSample().Draw(t, "sample").Text), "\n")
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

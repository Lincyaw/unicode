package syntax

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"pgregory.net/rapid"
)

// Sample is a valid file: its kind, its path, its text with random layout,
// and the canonical text of the same tree.
//
//uc:realizes syntax.Sample
type Sample struct {
	Kind      FileKind
	Path      string
	Text      []byte
	Canonical []byte
}

// genPathOf draws a path that fits kind: design/system.uc for the system
// file, and a .uc file in zero to two directories under design/ or corpus/
// for a component or corpus file.
func genPathOf(t *rapid.T, kind FileKind) string {
	top := map[FileKind]string{FileKindSystem: "design", FileKindComponent: "design", FileKindCorpus: "corpus"}[kind]
	if kind == FileKindSystem {
		return systemFilePath
	}
	return rapid.Custom(func(t *rapid.T) string {
		segs := append([]string{top}, rapid.SliceOfN(kebabNames, 0, 2).Draw(t, "dirs")...)
		return strings.Join(append(segs, kebabNames.Draw(t, "name")+".uc"), "/")
	}).Filter(func(p string) bool { return p != systemFilePath }).Draw(t, "path")
}

// genSample draws a tree and writes it twice: with random legal layout, and
// with every layout choice canonical. A third of the trees are a component
// whose property's claim is a random expression.
//
//uc:generates syntax.format-is-stable.sample
func genSample() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample {
		if rapid.IntRange(0, 2).Draw(t, "expression") == 0 {
			e := genExprTree(t, 3)
			return Sample{Kind: FileKindComponent, Path: genPathOf(t, FileKindComponent),
				Text: []byte(expressionFile(bracketed(e))), Canonical: []byte(expressionFile(canonicalExpr(e, 0)))}
		}
		f := genFile(t)
		random, canonical := &writer{t: t}, &writer{t: t, canonical: true}
		random.file(f)
		canonical.file(f)
		return Sample{Kind: f.Kind, Path: genPathOf(t, f.Kind), Text: []byte(random.b.String()), Canonical: []byte(canonical.b.String())}
	})
}

// expressionFile is a canonical component file whose one property claims expr.
func expressionFile(expr string) string {
	return "component sample \"Expression fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty expression invariant \"Expression normalization\" {\n  about compute\n  ensure " + expr + "\n}\n"
}

// valueColumns measures display width as docs/language.md §3 P6 counts it:
// East Asian wide characters take two columns.
var valueColumns = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: true}

// valueWidth is the width P6 lets a value line take.
const valueWidth = 100

// canonicalGap is the space P3 puts between two tokens on one line: none
// before a comma, a colon, a closing bracket, an opening parenthesis or angle
// bracket, and around a dot or a range; none after an opening bracket; one
// everywhere else, around ->, ->>, =, emits and after a comma or a colon included.
func canonicalGap(a, b string) string {
	switch b {
	case ",", ":", ")", "]", ">", "}", ".", "..", "(", "<":
		return ""
	}
	switch a {
	case "(", "[", "<", "{", ".", "..":
		return ""
	}
	return " "
}

// canonicalLine joins tokens with the gaps of canonicalGap.
func canonicalLine(toks []string) string {
	var b strings.Builder
	for i, tok := range toks {
		if i > 0 {
			b.WriteString(canonicalGap(toks[i-1], tok))
		}
		b.WriteString(tok)
	}
	return b.String()
}

// canonicalItem is an item of a bracketed value: its name or key, if any, and its value.
type canonicalItem struct {
	name  string
	value Value
}

// canonicalParts splits a bracketed value into what opens it, its items and
// what closes it; a value of no brackets has none.
func canonicalParts(v Value) (string, []canonicalItem, string) {
	var items []canonicalItem
	switch v := v.(type) {
	case *EncodedValue:
		return "encoded(", []canonicalItem{{value: v.Value}}, ")"
	case *ListValue:
		for _, x := range v.Items {
			items = append(items, canonicalItem{value: x})
		}
		return "[", items, "]"
	case *BraceValue:
		for _, e := range v.Entries {
			key := ""
			switch {
			case e.Key.Name != nil:
				key = string(*e.Key.Name)
			case e.Key.String != nil:
				key = quote(string(*e.Key.String))
			default:
				key = string(*e.Key.Number)
			}
			items = append(items, canonicalItem{key, e.Value})
		}
		return "{", items, "}"
	case *CallValue:
		for _, a := range v.Args {
			items = append(items, canonicalItem{string(a.Name), a.Value})
		}
		return string(v.Op) + "(", items, ")"
	}
	return "", nil, ""
}

// canonicalFlat writes a value on one line: its items apart by a comma and a
// space, with no comma after the last.
func canonicalFlat(v Value) string {
	switch v := v.(type) {
	case *NumberValue:
		return string(v.Text)
	case *StringValue:
		return quote(string(v.Text))
	case *BoolValue:
		if v.Value {
			return "true"
		}
		return "false"
	case *NoneValue:
		return "none"
	case *NameValue:
		return string(v.Name)
	}
	open, items, closer := canonicalParts(v)
	flat := make([]string, len(items))
	for i, it := range items {
		flat[i] = it.name
		if it.name != "" {
			flat[i] += ": "
		}
		flat[i] += canonicalFlat(it.value)
	}
	return open + strings.Join(flat, ", ") + closer
}

// canonicalValue writes v, then suffix, after the tokens of the current line:
// on that line when it then fits in valueWidth columns with its indentation,
// and otherwise broken, each item on a line of its own one level deeper with
// a comma after it (P6).
func (w *writer) canonicalValue(v Value, suffix string) {
	flat := canonicalFlat(v)
	open, items, closer := canonicalParts(v)
	line := append(append([]string{}, w.line...), flat)
	if suffix != "" {
		line = append(line, suffix)
	}
	if len(items) == 0 || 2*w.depth+valueColumns.StringWidth(canonicalLine(line)) <= valueWidth {
		w.tok(line[len(w.line):]...)
		return
	}
	w.tok(open)
	w.flush()
	w.depth++
	for _, it := range items {
		if it.name != "" {
			w.tok(it.name, ":")
		}
		w.canonicalValue(it.value, ",")
		w.flush()
	}
	w.depth--
	w.tok(closer)
	if suffix != "" {
		w.tok(suffix)
	}
}

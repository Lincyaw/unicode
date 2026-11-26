package syntax

import (
	"slices"
	"strings"
	"unicode"

	"pgregory.net/rapid"
)

// This file builds random valid syntax trees covering every construct of
// docs/format.md §3, and writes them as text with random legal layout.

// keywords are drawn as names now and then: names are not reserved, so a
// name may equal a keyword wherever its class allows.
var keywords = []string{
	"system", "notation", "language", "root", "actor", "person", "external", "component", "label", "focus",
	"paths", "uses", "type", "extends", "implements", "field", "value", "op", "raises", "handles", "flow",
	"on", "async", "alt", "else", "opt", "loop", "emits", "property", "about", "forall", "generated",
	"where", "in", "size", "case", "corpus", "coverage", "counterexample", "list", "set", "optional", "map",
	"call", "bool", "int", "float", "string", "bytes", "time", "duration", "core", "ddd",
}

var (
	kebabNames = rapid.StringMatching(`[a-z][a-z0-9]{0,5}(-[a-z0-9]{1,4}){0,2}`)
	upperNames = rapid.StringMatching(`[A-Z][A-Za-z0-9]{0,6}`)
	lowerNames = rapid.StringMatching(`[a-z][A-Za-z0-9]{0,6}`)
)

func genKebab(t *rapid.T, label string) Kebab {
	return Kebab(rapid.OneOf(kebabNames, kebabNames, kebabNames, rapid.SampledFrom(keywords)).Draw(t, label))
}

func genUpper(t *rapid.T, label string) Upper { return Upper(upperNames.Draw(t, label)) }

// genLower draws a lower-camel name, which may be a keyword but not a reserved literal.
func genLower(t *rapid.T, label string) Lower {
	gen := rapid.OneOf(lowerNames, lowerNames, lowerNames, rapid.SampledFrom(keywords))
	return Lower(gen.Filter(func(s string) bool { return !slices.Contains(reserved, s) }).Draw(t, label))
}

// interestingRunes are the characters strings are mostly made of: the ones
// that need escaping, spaces, CJK, and characters of the format's own syntax.
var interestingRunes = []rune(`"\ ab Z0{}[]()<>,:.=-/` + "中文　é  ")

// wideRunes make long strings whose display width is well below their
// length in bytes, so that values come near the width limit of §3.1 by one
// measure and past it by the other.
var wideRunes = []rune("中文字符値a ")

func genRune() *rapid.Generator[rune] {
	return rapid.OneOf(rapid.SampledFrom(interestingRunes), rapid.SampledFrom(interestingRunes),
		rapid.Rune().Filter(func(r rune) bool { return !unicode.IsControl(r) }))
}

// genString is any string value: it may hold a newline or a tab, which the format escapes.
func genString(t *rapid.T, label string) Text {
	return Text(rapid.StringOfN(rapid.OneOf(genRune(), genRune(), genRune(), rapid.SampledFrom([]rune{'\n', '\t'})), 0, 24, -1).Draw(t, label))
}

func genSummary(t *rapid.T, label string) Summary {
	return Summary(rapid.StringOfN(genRune(), 1, 24, -1).Filter(func(s string) bool {
		r := []rune(s)
		return !unicode.IsSpace(r[0]) && !unicode.IsSpace(r[len(r)-1])
	}).Draw(t, label))
}

func genOptSummary(t *rapid.T, label string) *Summary {
	if !rapid.Bool().Draw(t, label+"?") {
		return nil
	}
	s := genSummary(t, label)
	return &s
}

// genNumber draws a number in its canonical form; the writer may spell it otherwise.
func genNumber(t *rapid.T, integer bool) Number {
	whole := rapid.StringMatching(`0|[1-9][0-9]{0,3}`).Draw(t, "whole")
	if !integer && rapid.Bool().Draw(t, "fraction?") {
		whole += "." + rapid.StringMatching(`0|[0-9]{0,2}[1-9]`).Draw(t, "fraction")
	}
	if whole != "0" && whole != "0.0" && rapid.Bool().Draw(t, "negative") {
		whole = "-" + whole
	}
	return Number(whole)
}

// some draws between 0 and max items.
func some[V any](t *rapid.T, max int, gen func() V) []V {
	var out []V
	for range rapid.IntRange(0, max).Draw(t, "count") {
		out = append(out, gen())
	}
	return out
}

// commented puts random comment lines between the lines and after the last.
func commented[L any](t *rapid.T, ls []L) []L {
	var out []L
	for i := 0; i <= len(ls); i++ {
		for range max(0, rapid.IntRange(-3, 2).Draw(t, "comments")) {
			text := rapid.StringOfN(genRune(), 0, 20, -1).Draw(t, "comment")
			out = append(out, any(&Comment{Text: CommentText(strings.TrimRight(text, " "))}).(L))
		}
		if i < len(ls) {
			out = append(out, ls[i])
		}
	}
	return out
}

func genSegments(t *rapid.T, last func() string) []string {
	segs := some(t, 2, func() string { return string(genKebab(t, "segment")) })
	return append(segs, last())
}

func genTypeRef(t *rapid.T) TypeRef {
	return TypeRef{Segments: genSegments(t, func() string { return string(genUpper(t, "type")) })}
}

func genOpRef(t *rapid.T) OpRef {
	segs := some(t, 2, func() string { return string(genKebab(t, "segment")) })
	if rapid.Bool().Draw(t, "type?") {
		segs = append(segs, string(genUpper(t, "type")))
	}
	return OpRef{Segments: append(segs, string(genLower(t, "op")))}
}

func genParticipant(t *rapid.T) Participant {
	return Participant{Segments: genSegments(t, func() string {
		if rapid.Bool().Draw(t, "upper?") {
			return string(genUpper(t, "type"))
		}
		return string(genKebab(t, "name"))
	})}
}

func genRef(t *rapid.T) Ref {
	var segs []string
	for range rapid.IntRange(1, 3).Draw(t, "segments") {
		switch rapid.IntRange(0, 2).Draw(t, "class") {
		case 0:
			segs = append(segs, string(genKebab(t, "segment")))
		case 1:
			segs = append(segs, string(genUpper(t, "segment")))
		default:
			segs = append(segs, string(genLower(t, "segment")))
		}
	}
	return Ref{Segments: segs}
}

func genRefs(t *rapid.T) []Ref {
	refs := []Ref{genRef(t)}
	return append(refs, some(t, 2, func() Ref { return genRef(t) })...)
}

func genTypeExpr(t *rapid.T, depth int) TypeExpr {
	kind := rapid.IntRange(0, 5).Draw(t, "typeExpr")
	if depth <= 0 {
		kind %= 2
	}
	switch kind {
	case 0:
		return &Builtin{Name: rapid.SampledFrom([]string{"bool", "int", "float", "string", "bytes", "time", "duration"}).Draw(t, "builtin")}
	case 1:
		return &NamedType{Ref: genTypeRef(t)}
	case 2, 3:
		return &ElemType{Kind: rapid.SampledFrom([]string{"list", "set", "optional"}).Draw(t, "elem"), Elem: genTypeExpr(t, depth-1)}
	case 4:
		return &MapType{Key: genTypeExpr(t, depth-1), Value: genTypeExpr(t, depth-1)}
	}
	return &CallType{Targets: genRefs(t)}
}

func genValue(t *rapid.T, depth int) Value {
	kind := rapid.IntRange(0, 7).Draw(t, "value")
	if depth <= 0 {
		kind %= 5
	}
	switch kind {
	case 0:
		return &NumberValue{Text: genNumber(t, false)}
	case 1:
		if rapid.Bool().Draw(t, "wide?") {
			return &StringValue{Text: Text(rapid.StringOfN(rapid.SampledFrom(wideRunes), 8, 48, -1).Draw(t, "wide"))}
		}
		return &StringValue{Text: genString(t, "string")}
	case 2:
		if rapid.Bool().Draw(t, "none?") {
			return &NoneValue{None: true}
		}
		return &BoolValue{Value: Boolean(rapid.Bool().Draw(t, "bool"))}
	case 3, 4:
		return &NameValue{Name: genLower(t, "name")}
	case 5:
		return &CallValue{Op: genLower(t, "op"), Args: some(t, 3, func() Arg {
			return Arg{Name: genLower(t, "param"), Value: genValue(t, depth-1)}
		})}
	case 6:
		return &ListValue{Items: some(t, 6, func() Value { return genValue(t, depth-1) })}
	}
	return &BraceValue{Entries: some(t, 4, func() Entry {
		var key Key
		switch rapid.IntRange(0, 2).Draw(t, "key") {
		case 0:
			name := genLower(t, "key")
			key.Name = &name
		case 1:
			s := genString(t, "key")
			key.String = &s
		default:
			n := genNumber(t, false)
			key.Number = &n
		}
		return Entry{Key: key, Value: genValue(t, depth-1)}
	})}
}

func genLabels[L any](t *rapid.T) []L {
	return some(t, 2, func() L { return any(&Label{Name: genKebab(t, "label")}).(L) })
}

func genOp(t *rapid.T) *OpDecl {
	o := &OpDecl{Name: genLower(t, "op"), Params: some(t, 3, func() Param {
		return Param{Name: genLower(t, "param"), Type: genTypeExpr(t, 2)}
	})}
	if rapid.Bool().Draw(t, "result?") {
		o.Result = genTypeExpr(t, 2)
	}
	o.Raises = some(t, 2, func() TypeRef { return genTypeRef(t) })
	if rapid.Bool().Draw(t, "handles?") {
		h := genTypeRef(t)
		o.Handles = &h
	}
	o.Summary = genOptSummary(t, "opSummary")
	return o
}

func genType(t *rapid.T) *TypeDecl {
	d := &TypeDecl{Name: genUpper(t, "type"), Form: rapid.SampledFrom(typeForms).Draw(t, "form"), Summary: genSummary(t, "summary")}
	if rapid.IntRange(0, 2).Draw(t, "body") == 0 {
		return d
	}
	ls := genLabels[Member](t)
	ls = append(ls, some(t, 1, func() Member { return &Extends{Type: genTypeRef(t)} })...)
	ls = append(ls, some(t, 1, func() Member { return &Implements{Type: genTypeRef(t)} })...)
	switch d.Form {
	case "enum":
		ls = append(ls, some(t, 3, func() Member { return &EnumValue{Name: genLower(t, "value")} })...)
	case "interface":
	default:
		ls = append(ls, some(t, 3, func() Member { return &Field{Name: genLower(t, "field"), Type: genTypeExpr(t, 2)} })...)
	}
	ls = append(ls, some(t, 2, func() Member { return genOp(t) })...)
	d.Lines = commented(t, ls)
	return d
}

func genSymbol(t *rapid.T) *Symbol {
	s := &Symbol{Name: genLower(t, "symbol"), Type: genTypeExpr(t, 2)}
	if rapid.IntRange(0, 3).Draw(t, "generated") == 0 {
		domain := genSummary(t, "domain")
		s.Domain = &domain
	}
	return s
}

// genLimit draws a *SubsetLimit or an *IntervalLimit.
func genLimit(t *rapid.T) any {
	path := []Lower{genLower(t, "path")}
	path = append(path, some(t, 2, func() Lower { return genLower(t, "path") })...)
	switch rapid.IntRange(0, 2).Draw(t, "limit") {
	case 0:
		return &IntervalLimit{Path: path, Op: "in", Low: genNumber(t, false), High: genNumber(t, false)}
	case 1:
		members := []Lower{genLower(t, "member")}
		return &SubsetLimit{Path: path, Members: append(members, some(t, 2, func() Lower { return genLower(t, "member") })...)}
	}
	return &IntervalLimit{Path: path, Op: "size", Low: genNumber(t, true), High: genNumber(t, true)}
}

func genAssignments(t *rapid.T) []CaseLine {
	return commented(t, some(t, 3, func() CaseLine { return &Assignment{Symbol: genLower(t, "symbol"), Value: genValue(t, 3)} }))
}

func genCase(t *rapid.T) *CaseDecl {
	return &CaseDecl{Name: genKebab(t, "case"), Summary: genOptSummary(t, "caseSummary"), Lines: genAssignments(t)}
}

func genArgs(t *rapid.T) []Lower {
	return some(t, 3, func() Lower { return genLower(t, "arg") })
}

// genSteps draws a step list; each call may be followed by `on` blocks.
func genSteps[L any](t *rapid.T, depth int) []L {
	if depth <= 0 {
		return nil
	}
	var ls []L
	for range rapid.IntRange(0, 3).Draw(t, "steps") {
		s := genStep(t, depth-1)
		ls = append(ls, any(s).(L))
		if _, ok := s.(*Call); ok {
			for range max(0, rapid.IntRange(-1, 2).Draw(t, "ons")) {
				ls = append(ls, any(&OnBlock{Error: genTypeRef(t), Lines: genSteps[Step](t, depth-1)}).(L))
			}
		}
	}
	return commented(t, ls)
}

func genStep(t *rapid.T, depth int) Step {
	switch rapid.IntRange(0, 6).Draw(t, "step") {
	case 0:
		return &Call{From: genParticipant(t), Op: genOpRef(t), Args: genArgs(t), Lines: genSteps[Step](t, depth)}
	case 1:
		return &Send{From: genParticipant(t), Op: genOpRef(t), Args: genArgs(t)}
	case 2:
		return &Emit{From: genParticipant(t), Event: genTypeRef(t), Args: genArgs(t)}
	case 3:
		return &Async{Op: genOpRef(t), Lines: genSteps[Step](t, depth)}
	case 4:
		return genAlt(t, depth, 1)
	case 5:
		return &Opt{Label: genSummary(t, "label"), Lines: genSteps[Step](t, depth)}
	}
	return &Loop{Label: genSummary(t, "label"), Lines: genSteps[Step](t, depth)}
}

// genAlt draws an alt fragment of fewest to three branches.
func genAlt(t *rapid.T, depth, fewest int) *Alt {
	a := &Alt{}
	for range rapid.IntRange(fewest, 3).Draw(t, "branches") {
		a.Branches = append(a.Branches, Branch{Label: genSummary(t, "label"), Lines: genSteps[Step](t, depth)})
	}
	return a
}

func genFlow(t *rapid.T) *FlowDecl {
	ls := some(t, 2, func() FlowLine { return genSymbol(t) })
	ls = append(ls, some(t, 3, func() FlowLine { return genLimit(t).(FlowLine) })...)
	ls = append(ls, genSteps[FlowLine](t, 3)...)
	ls = append(ls, some(t, 2, func() FlowLine { return genCase(t) })...)
	return &FlowDecl{Name: genKebab(t, "flow"), Summary: genSummary(t, "summary"), Lines: commented(t, ls)}
}

func genProperty(t *rapid.T) *PropertyDecl {
	ls := []PropertyLine{&About{Refs: genRefs(t)}}
	ls = append(ls, some(t, 2, func() PropertyLine { return genSymbol(t) })...)
	ls = append(ls, some(t, 3, func() PropertyLine { return genLimit(t).(PropertyLine) })...)
	ls = append(ls, some(t, 2, func() PropertyLine { return genCase(t) })...)
	return &PropertyDecl{
		Name: genKebab(t, "property"), Form: rapid.SampledFrom(propertyForms).Draw(t, "form"),
		Summary: genSummary(t, "summary"), Lines: commented(t, ls),
	}
}

func genSystemDecls(t *rapid.T) []Decl {
	ds := []Decl{&SystemDecl{Name: genKebab(t, "system"), Summary: genSummary(t, "summary")}}
	ds = append(ds, some(t, 2, func() Decl {
		return &NotationDecl{Name: rapid.SampledFrom([]string{"ddd", "c4"}).Draw(t, "notation")}
	})...)
	ds = append(ds, some(t, 2, func() Decl {
		roots := []LanguageLine{&Root{Path: genString(t, "root")}}
		roots = append(roots, some(t, 1, func() LanguageLine { return &Root{Path: genString(t, "root")} })...)
		return &LanguageDecl{Name: rapid.SampledFrom([]string{"go", "python"}).Draw(t, "language"), Lines: commented(t, roots)}
	})...)
	return append(ds, some(t, 3, func() Decl {
		if rapid.Bool().Draw(t, "person?") {
			return &PersonDecl{Name: genKebab(t, "actor"), Summary: genSummary(t, "summary")}
		}
		ls := some(t, 2, func() ExternalLine { return genType(t) })
		ls = append(ls, some(t, 2, func() ExternalLine { return genOp(t) })...)
		return &ExternalDecl{Name: genKebab(t, "actor"), Summary: genSummary(t, "summary"), Lines: commented(t, ls)}
	})...)
}

func genComponentDecls(t *rapid.T) []Decl {
	ls := genLabels[ComponentLine](t)
	if rapid.Bool().Draw(t, "focus?") {
		ls = append(ls, &Focus{Level: rapid.SampledFrom([]string{"core", "support", "utility"}).Draw(t, "focus")})
	}
	paths := &Paths{Patterns: []Text{genString(t, "pattern")}}
	paths.Patterns = append(paths.Patterns, some(t, 1, func() Text { return genString(t, "pattern") })...)
	ls = append(ls, paths)
	if rapid.Bool().Draw(t, "uses?") {
		ls = append(ls, &Uses{Refs: genRefs(t)})
	}
	path := Path{Segments: genSegments(t, func() string { return string(genKebab(t, "segment")) })}
	ds := []Decl{&ComponentDecl{Path: path, Summary: genSummary(t, "summary"), Lines: commented(t, ls)}}
	ds = append(ds, some(t, 3, func() Decl { return genType(t) })...)
	ds = append(ds, some(t, 2, func() Decl { return genOp(t) })...)
	ds = append(ds, some(t, 2, func() Decl { return genFlow(t) })...)
	return append(ds, some(t, 2, func() Decl { return genProperty(t) })...)
}

func genCorpusDecls(t *rapid.T) []Decl {
	ds := []Decl{&CorpusDecl{Target: genRef(t)}}
	return append(ds, some(t, 3, func() Decl {
		return &RecordedCase{
			Name: genKebab(t, "case"), Reason: rapid.SampledFrom([]string{"coverage", "counterexample"}).Draw(t, "reason"),
			Lines: genAssignments(t),
		}
	})...)
}

var (
	typeForms     = []string{"object", "record", "interface", "enum", "error", "event"}
	propertyForms = []string{"invariant", "postcondition", "relation", "transition"}
)

var fileKinds = []FileKind{FileKindSystem, FileKindComponent, FileKindCorpus}

func genFile(t *rapid.T) *File {
	return genFileOf(t, rapid.SampledFrom(fileKinds).Draw(t, "kind"))
}

func genFileOf(t *rapid.T, kind FileKind) *File {
	f := &File{Kind: kind}
	switch f.Kind {
	case FileKindSystem:
		f.Decls = genSystemDecls(t)
	case FileKindComponent:
		f.Decls = genComponentDecls(t)
	default:
		f.Decls = genCorpusDecls(t)
	}
	f.Decls = commented(t, f.Decls)
	return f
}

// writer writes a tree as text with random layout that the grammar allows:
// any indentation, any number of spaces between tokens (none where the
// tokens stay apart), trailing spaces, blank lines between lines, braces
// around an empty optional block, values broken between their items with an
// optional trailing comma, and any number of final newlines.
//
// It records where mark stands in markLine and markColumn, and ends that
// line with markComment when it is set, which then is what stands there. The
// mark is a node (a line of any level, a value or a branch of an alt), which
// stands at its first token, or the address of a name, a reference's
// segment, a summary or a number in the tree, which stands at its token.
// When elseApart is set and the mark is a branch, its else starts a line of
// its own below the } of the branch before it.
type writer struct {
	t    *rapid.T
	b    strings.Builder
	line []string

	mark        any
	markComment string
	elseApart   bool
	markLine    int
	markColumn  int
	markToken   int    // the index in line of the marked token, while marking
	markOffset  int    // where the mark stands inside that token, in bytes
	marking     bool   // the current line holds the mark
	comment     string // markComment, until the marked node's first line ends
}

func (w *writer) tok(toks ...string) { w.line = append(w.line, toks...) }

// at notes that p, the node or the address of the value written next, starts
// with the next token of the current line.
func (w *writer) at(p any) {
	if p != w.mark {
		return
	}
	w.markLine = strings.Count(w.b.String(), "\n") + 1
	w.markToken, w.marking = len(w.line), true
	w.comment = w.markComment
}

// word writes tok, the text of the value at p.
func (w *writer) word(p any, tok string) {
	w.at(p)
	w.tok(tok)
}

func (w *writer) summary(s *Summary) { w.word(s, quote(string(*s))) }

func (w *writer) spaces(label string, max int) string {
	return strings.Repeat(" ", rapid.IntRange(0, max).Draw(w.t, label))
}

// flush ends the current line, then writes blank lines.
func (w *writer) flush() {
	if w.comment != "" {
		w.markToken = len(w.line)
		w.tok(w.comment)
		w.comment = ""
	}
	start := w.b.Len()
	w.b.WriteString(w.spaces("indent", 6))
	for i, tok := range w.line {
		if i > 0 {
			sep := w.spaces("sep", 2)
			if sep == "" && needsSpace(w.line[i-1], tok) {
				sep = " "
			}
			w.b.WriteString(sep)
		}
		if w.marking && i == w.markToken {
			w.markColumn, w.marking = w.b.Len()-start+w.markOffset+1, false
		}
		w.b.WriteString(tok)
	}
	w.b.WriteString(w.spaces("trailing", 2))
	w.b.WriteByte('\n')
	w.line = w.line[:0]
	for range max(0, rapid.IntRange(-2, 2).Draw(w.t, "blank")) {
		w.b.WriteString(w.spaces("blankSpaces", 2) + "\n")
	}
}

// needsSpace tells whether two tokens written together would lex as one.
func needsSpace(a, b string) bool {
	alnum := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' }
	return alnum(a[len(a)-1]) && (alnum(b[0]) || b[0] == '-' && len(b) > 1 && alnum(b[1]))
}

func (w *writer) ref(segs []string) {
	for i, s := range segs {
		if i > 0 {
			w.tok(".")
		}
		w.word(&segs[i], s)
	}
}

func (w *writer) refs(rs []Ref) {
	for i, r := range rs {
		if i > 0 {
			w.tok(",")
		}
		w.ref(r.Segments)
	}
}

func (w *writer) names(ns []Lower, sep string) {
	for i, n := range ns {
		if i > 0 {
			w.tok(sep)
		}
		w.word(&ns[i], string(n))
	}
}

func (w *writer) args(as []Lower) {
	w.tok("(")
	w.names(as, ",")
	w.tok(")")
}

// block writes `{`, the lines and `}`; the caller ends the line.
func writeBlock[L any](w *writer, ls []L) {
	w.tok("{")
	w.flush()
	for _, l := range ls {
		w.node(l)
	}
	w.tok("}")
}

// optBlock writes an optional block, which may be left out when it is empty.
func writeOptBlock[L any](w *writer, ls []L) {
	if len(ls) == 0 && rapid.Bool().Draw(w.t, "noBraces") {
		return
	}
	writeBlock(w, ls)
}

func (w *writer) file(f *File) {
	for range max(0, rapid.IntRange(-2, 2).Draw(w.t, "leadingBlanks")) {
		w.b.WriteString(w.spaces("blankSpaces", 2) + "\n")
	}
	for _, d := range f.Decls {
		w.node(d)
	}
	text := strings.TrimRight(w.b.String(), "\n ")
	w.b.Reset()
	w.b.WriteString(text + strings.Repeat("\n", rapid.IntRange(0, 2).Draw(w.t, "finalNewlines")))
}

func (w *writer) node(n any) {
	if r, ok := n.(*rawLines); ok {
		w.raw(r)
		return
	}
	w.at(n)
	switch n := n.(type) {
	case *Comment:
		w.tok("//" + string(n.Text))
	case *SystemDecl:
		w.tok("system")
		w.word(&n.Name, string(n.Name))
		w.summary(&n.Summary)
	case *NotationDecl:
		w.tok("notation", n.Name)
	case *LanguageDecl:
		w.tok("language", n.Name)
		writeBlock(w, n.Lines)
	case *Root:
		w.tok("root", quote(string(n.Path)))
	case *PersonDecl:
		w.tok("actor")
		w.word(&n.Name, string(n.Name))
		w.tok("person")
		w.summary(&n.Summary)
	case *ExternalDecl:
		w.tok("actor")
		w.word(&n.Name, string(n.Name))
		w.tok("external")
		w.summary(&n.Summary)
		writeOptBlock(w, n.Lines)
	case *ComponentDecl:
		w.tok("component")
		w.ref(n.Path.Segments)
		w.summary(&n.Summary)
		writeBlock(w, n.Lines)
	case *Label:
		w.tok("label")
		w.word(&n.Name, string(n.Name))
	case *Focus:
		w.tok("focus", n.Level)
	case *Paths:
		w.tok("paths")
		for i, p := range n.Patterns {
			if i > 0 {
				w.tok(",")
			}
			w.tok(quote(string(p)))
		}
	case *Uses:
		w.tok("uses")
		w.refs(n.Refs)
	case *TypeDecl:
		w.tok("type")
		w.word(&n.Name, string(n.Name))
		w.tok(n.Form)
		w.summary(&n.Summary)
		writeOptBlock(w, n.Lines)
	case *Extends:
		w.tok("extends")
		w.ref(n.Type.Segments)
	case *Implements:
		w.tok("implements")
		w.ref(n.Type.Segments)
	case *Field:
		w.tok("field")
		w.word(&n.Name, string(n.Name))
		w.tok(":")
		w.typeExpr(n.Type)
	case *EnumValue:
		w.tok("value")
		w.word(&n.Name, string(n.Name))
	case *OpDecl:
		w.opDecl(n)
	case *FlowDecl:
		w.tok("flow")
		w.word(&n.Name, string(n.Name))
		w.summary(&n.Summary)
		writeBlock(w, n.Lines)
	case *PropertyDecl:
		w.tok("property")
		w.word(&n.Name, string(n.Name))
		w.tok(n.Form)
		w.summary(&n.Summary)
		writeBlock(w, n.Lines)
	case *About:
		w.tok("about")
		w.refs(n.Refs)
	case *Symbol:
		w.tok("forall")
		w.word(&n.Name, string(n.Name))
		w.tok(":")
		w.typeExpr(n.Type)
		if n.Domain != nil {
			w.tok("generated")
			w.summary(n.Domain)
		}
	case *SubsetLimit:
		w.tok("where")
		w.names(n.Path, ".")
		w.tok("in", "{")
		w.names(n.Members, ",")
		w.tok("}")
	case *IntervalLimit:
		w.tok("where")
		w.names(n.Path, ".")
		w.tok(n.Op)
		w.word(&n.Low, w.number(n.Low))
		w.tok("..")
		w.word(&n.High, w.number(n.High))
	case *CaseDecl:
		w.tok("case")
		w.word(&n.Name, string(n.Name))
		if n.Summary != nil {
			w.summary(n.Summary)
		}
		writeBlock(w, n.Lines)
	case *Assignment:
		w.word(&n.Symbol, string(n.Symbol))
		w.tok("=")
		w.value(n.Value)
	case *CorpusDecl:
		w.tok("corpus")
		w.ref(n.Target.Segments)
	case *RecordedCase:
		w.tok("case")
		w.word(&n.Name, string(n.Name))
		w.tok(n.Reason)
		writeBlock(w, n.Lines)
	default:
		w.step(n)
	}
	w.flush()
}

func (w *writer) opDecl(o *OpDecl) {
	w.tok("op")
	w.word(&o.Name, string(o.Name))
	w.tok("(")
	for i := range o.Params {
		p := &o.Params[i]
		if i > 0 {
			w.tok(",")
		}
		w.word(&p.Name, string(p.Name))
		w.tok(":")
		w.typeExpr(p.Type)
	}
	w.tok(")")
	if o.Result != nil {
		w.tok("->")
		w.typeExpr(o.Result)
	}
	for i, r := range o.Raises {
		if i == 0 {
			w.tok("raises")
		} else {
			w.tok(",")
		}
		w.ref(r.Segments)
	}
	if o.Handles != nil {
		w.tok("handles")
		w.ref(o.Handles.Segments)
	}
	if o.Summary != nil {
		w.summary(o.Summary)
	}
}

func (w *writer) step(n any) {
	switch n := n.(type) {
	case *Call:
		w.ref(n.From.Segments)
		w.tok("->")
		w.ref(n.Op.Segments)
		w.args(n.Args)
		writeOptBlock(w, n.Lines)
	case *Send:
		w.ref(n.From.Segments)
		w.tok("->>")
		w.ref(n.Op.Segments)
		w.args(n.Args)
	case *Emit:
		w.ref(n.From.Segments)
		w.tok("emits")
		w.ref(n.Event.Segments)
		w.args(n.Args)
	case *OnBlock:
		w.tok("on")
		w.ref(n.Error.Segments)
		writeBlock(w, n.Lines)
	case *Async:
		w.tok("async")
		w.ref(n.Op.Segments)
		writeBlock(w, n.Lines)
	case *Alt:
		kw := "alt"
		for i := range n.Branches {
			br := &n.Branches[i]
			if w.elseApart && any(br) == w.mark {
				w.apart()
			}
			w.at(br)
			w.tok(kw)
			w.summary(&br.Label)
			writeBlock(w, br.Lines)
			kw = "else"
		}
	case *Opt:
		w.tok("opt")
		w.summary(&n.Label)
		writeBlock(w, n.Lines)
	case *Loop:
		w.tok("loop")
		w.summary(&n.Label)
		writeBlock(w, n.Lines)
	}
}

func (w *writer) typeExpr(t TypeExpr) {
	switch t := t.(type) {
	case *Builtin:
		w.tok(t.Name)
	case *NamedType:
		w.ref(t.Ref.Segments)
	case *ElemType:
		w.tok(t.Kind, "<")
		w.typeExpr(t.Elem)
		w.tok(">")
	case *MapType:
		w.tok("map", "<")
		w.typeExpr(t.Key)
		w.tok(",")
		w.typeExpr(t.Value)
		w.tok(">")
	case *CallType:
		w.tok("call", "<")
		w.refs(t.Targets)
		w.tok(">")
	}
}

// number spells a canonical number with leading zeros, trailing zeros in its
// fraction, or a minus on zero, all of which parsing normalizes away.
func (w *writer) number(n Number) string {
	sign, digits := "", string(n)
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	if sign == "" && strings.Trim(digits, "0.") == "" && rapid.Bool().Draw(w.t, "minusZero") {
		sign = "-"
	}
	digits = strings.Repeat("0", rapid.IntRange(0, 2).Draw(w.t, "leadingZeros")) + digits
	if strings.Contains(digits, ".") {
		digits += strings.Repeat("0", rapid.IntRange(0, 2).Draw(w.t, "trailingZeros"))
	}
	return sign + digits
}

// value writes v; between its items it may break the line.
func (w *writer) value(v Value) {
	w.at(v)
	open, items, closer := parts(v)
	switch v := v.(type) {
	case *NumberValue:
		w.tok(w.number(v.Text))
		return
	case *NameValue:
		w.at(&v.Name)
	case *CallValue:
		w.at(&v.Op)
	}
	if open == "" {
		w.tok(flatValue(v))
		return
	}
	breakable := func() {
		if rapid.IntRange(0, 3).Draw(w.t, "valueBreak") == 0 {
			w.flush()
		}
	}
	w.tok(open)
	for i, it := range items {
		breakable()
		if it.prefix != "" {
			w.word(itemName(v, i), strings.TrimSuffix(it.prefix, ": "))
			w.tok(":")
		}
		w.value(it.value)
		breakable()
		if i < len(items)-1 || rapid.Bool().Draw(w.t, "trailingComma") {
			w.tok(",")
			breakable()
		}
	}
	w.tok(closer)
}

// itemName is the address of the name of item i of a bracketed value: an
// argument's name, or an entry's key when it is a name.
func itemName(v Value, i int) any {
	switch v := v.(type) {
	case *CallValue:
		return &v.Args[i].Name
	case *BraceValue:
		return v.Entries[i].Key.Name
	}
	return nil
}

// apart ends the line of the } that closes a branch, and writes comments
// below it before its else.
func (w *writer) apart() {
	w.flush()
	for range rapid.IntRange(0, 2).Draw(w.t, "between") {
		w.tok("//" + strings.TrimRight(rapid.StringOfN(genRune(), 0, 12, -1).Draw(w.t, "comment"), " "))
		w.flush()
	}
}

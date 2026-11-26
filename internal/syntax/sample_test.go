package syntax

import (
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

func genName(c nameClass) *rapid.Generator[string] {
	patterns := [...]string{
		kebab: `[a-z][a-z0-9]{0,5}(-[a-z0-9]{1,4}){0,2}`,
		upper: `[A-Z][A-Za-z0-9]{0,6}`,
		lower: `[a-z][A-Za-z0-9]{0,6}`,
	}
	gen := rapid.StringMatching(patterns[c])
	if c == upper {
		return gen
	}
	return rapid.OneOf(gen, gen, gen, rapid.SampledFrom(keywords).Filter(c.match))
}

// genValueName is a name that a value may be, so not one of the literals.
func genValueName() *rapid.Generator[string] {
	return genName(lower).Filter(func(s string) bool { return s != "true" && s != "false" && s != "none" })
}

// interestingRunes are the characters strings are mostly made of: the ones
// that need escaping, spaces, CJK, and characters of the format's own syntax.
var interestingRunes = []rune(`"\ ab Z0{}[]()<>,:.=-/` + "中文　é  ")

func genRune() *rapid.Generator[rune] {
	return rapid.OneOf(rapid.SampledFrom(interestingRunes), rapid.SampledFrom(interestingRunes),
		rapid.Rune().Filter(func(r rune) bool { return !unicode.IsControl(r) }))
}

// genString is any string value: it may hold a newline or a tab, which the format escapes.
func genString() *rapid.Generator[string] {
	return rapid.StringOfN(rapid.OneOf(genRune(), genRune(), genRune(), rapid.SampledFrom([]rune{'\n', '\t'})), 0, 24, -1)
}

func genSummary() *rapid.Generator[string] {
	return rapid.StringOfN(genRune(), 1, 24, -1).Filter(func(s string) bool {
		first, last := []rune(s)[0], []rune(s)[len([]rune(s))-1]
		return !unicode.IsSpace(first) && !unicode.IsSpace(last)
	})
}

func genOptSummary(t *rapid.T, label string) string {
	if rapid.Bool().Draw(t, label+"?") {
		return genSummary().Draw(t, label)
	}
	return ""
}

// genComments draws up to two comments, none more often than not.
func genComments(t *rapid.T, label string) []Comment {
	n := max(0, rapid.IntRange(-2, 2).Draw(t, label+"#"))
	var cs []Comment
	for range n {
		text := rapid.StringOfN(genRune(), 0, 20, -1).Draw(t, label)
		cs = append(cs, Comment{Text: strings.TrimRight(text, " ")})
	}
	return cs
}

func genNumber(integer bool) *rapid.Generator[string] {
	if integer {
		return rapid.StringMatching(`-?[0-9]{1,4}`)
	}
	return rapid.StringMatching(`-?[0-9]{1,4}(\.[0-9]{1,3})?`)
}

// some draws between 0 and max items.
func some[V any](t *rapid.T, label string, max int, gen func(t *rapid.T, label string) V) []V {
	n := rapid.IntRange(0, max).Draw(t, label+"#")
	var out []V
	for range n {
		out = append(out, gen(t, label))
	}
	return out
}

func genSegments(t *rapid.T, label string, kebabs int, last ...nameClass) []string {
	segs := some(t, label, kebabs, func(t *rapid.T, label string) string { return genName(kebab).Draw(t, label) })
	c := rapid.SampledFrom(last).Draw(t, label+".class")
	return append(segs, genName(c).Draw(t, label))
}

func genTypeRef(t *rapid.T, label string) Ref {
	return Ref{Segments: genSegments(t, label, 2, upper)}
}

func genOpRef(t *rapid.T, label string) Ref {
	segs := some(t, label, 2, func(t *rapid.T, label string) string { return genName(kebab).Draw(t, label) })
	if rapid.Bool().Draw(t, label+".type?") {
		segs = append(segs, genName(upper).Draw(t, label))
	}
	return Ref{Segments: append(segs, genName(lower).Draw(t, label))}
}

func genParticipant(t *rapid.T, label string) Ref {
	return Ref{Segments: genSegments(t, label, 2, kebab, upper)}
}

func genAnyRef(t *rapid.T, label string) Ref {
	n := rapid.IntRange(1, 3).Draw(t, label+"#")
	var segs []string
	for range n {
		segs = append(segs, genName(rapid.SampledFrom([]nameClass{kebab, upper, lower}).Draw(t, label+".class")).Draw(t, label))
	}
	return Ref{Segments: segs}
}

func genTypeExpr(t *rapid.T, depth int) TypeExpr {
	kind := rapid.IntRange(0, 5).Draw(t, "typeExpr")
	if depth <= 0 {
		kind %= 2
	}
	switch kind {
	case 0:
		return &BuiltinType{Name: rapid.SampledFrom([]string{"bool", "int", "float", "string", "bytes", "time", "duration"}).Draw(t, "builtin")}
	case 1:
		return &NamedType{Ref: genTypeRef(t, "typeRef")}
	case 2, 3:
		return &ElemType{Kind: rapid.SampledFrom([]string{"list", "set", "optional"}).Draw(t, "elemKind"), Elem: genTypeExpr(t, depth-1)}
	case 4:
		return &MapType{Key: genTypeExpr(t, depth-1), Value: genTypeExpr(t, depth-1)}
	}
	n := rapid.IntRange(1, 3).Draw(t, "callTargets")
	c := &CallType{}
	for range n {
		c.Targets = append(c.Targets, genAnyRef(t, "callTarget"))
	}
	return c
}

func genValue(t *rapid.T, depth int) Value {
	kind := rapid.IntRange(0, 7).Draw(t, "value")
	if depth <= 0 {
		kind %= 5
	}
	switch kind {
	case 0:
		return &NumberValue{Text: genNumber(false).Draw(t, "number")}
	case 1:
		return &StringValue{Text: genString().Draw(t, "string")}
	case 2:
		if rapid.Bool().Draw(t, "none?") {
			return &NoneValue{}
		}
		return &BoolValue{Value: rapid.Bool().Draw(t, "bool")}
	case 3, 4:
		return &NameValue{Name: genValueName().Draw(t, "name")}
	case 5:
		c := &CallValue{Op: genName(lower).Draw(t, "callOp")}
		c.Args = some(t, "arg", 3, func(t *rapid.T, label string) Arg {
			return Arg{Name: genName(lower).Draw(t, label), Value: genValue(t, depth-1)}
		})
		return c
	case 6:
		l := &ListValue{}
		l.Items = some(t, "item", 6, func(t *rapid.T, _ string) Value { return genValue(t, depth-1) })
		return l
	}
	b := &BraceValue{}
	b.Entries = some(t, "entry", 4, func(t *rapid.T, label string) Entry {
		var key Value
		switch rapid.IntRange(0, 2).Draw(t, "keyKind") {
		case 0:
			key = &NameValue{Name: genName(lower).Draw(t, label)}
		case 1:
			key = &StringValue{Text: genString().Draw(t, label)}
		default:
			key = &NumberValue{Text: genNumber(false).Draw(t, label)}
		}
		return Entry{Key: key, Value: genValue(t, depth-1)}
	})
	return b
}

func genLabels(t *rapid.T) []Label {
	return some(t, "label", 2, func(t *rapid.T, label string) Label {
		return Label{Comments: genComments(t, "labelComment"), Name: genName(kebab).Draw(t, label)}
	})
}

func genOp(t *rapid.T, _ string) OpDecl {
	o := OpDecl{Comments: genComments(t, "opComment"), Name: genName(lower).Draw(t, "opName")}
	o.Params = some(t, "param", 3, func(t *rapid.T, label string) Param {
		return Param{Name: genName(lower).Draw(t, label), Type: genTypeExpr(t, 2)}
	})
	if rapid.Bool().Draw(t, "result?") {
		o.Result = genTypeExpr(t, 2)
	}
	o.Raises = some(t, "raises", 2, genTypeRef)
	if rapid.Bool().Draw(t, "handles?") {
		h := genTypeRef(t, "handles")
		o.Handles = &h
	}
	o.Summary = genOptSummary(t, "opSummary")
	return o
}

func genType(t *rapid.T, _ string) TypeDecl {
	d := TypeDecl{
		Comments: genComments(t, "typeComment"),
		Name:     genName(upper).Draw(t, "typeName"),
		Form:     rapid.SampledFrom(typeForms).Draw(t, "typeForm"),
		Summary:  genSummary().Draw(t, "typeSummary"),
	}
	if rapid.IntRange(0, 2).Draw(t, "typeBody") == 0 {
		return d
	}
	d.Labels = genLabels(t)
	supertype := func(t *rapid.T, label string) Supertype {
		return Supertype{Comments: genComments(t, label+"Comment"), Type: genTypeRef(t, label)}
	}
	d.Extends = some(t, "extends", 1, supertype)
	d.Implements = some(t, "implements", 1, supertype)
	switch d.Form {
	case "enum":
		d.Values = some(t, "enumValue", 3, func(t *rapid.T, label string) EnumValue {
			return EnumValue{Comments: genComments(t, label+"Comment"), Name: genName(lower).Draw(t, label)}
		})
	case "interface":
	default:
		d.Fields = some(t, "field", 3, func(t *rapid.T, label string) Field {
			return Field{Comments: genComments(t, label+"Comment"), Name: genName(lower).Draw(t, label), Type: genTypeExpr(t, 2)}
		})
	}
	d.Ops = some(t, "typeOp", 2, genOp)
	d.Trailing = genComments(t, "typeTrailing")
	return d
}

func genSymbol(t *rapid.T, label string) Symbol {
	s := Symbol{Comments: genComments(t, label+"Comment"), Name: genName(lower).Draw(t, label), Type: genTypeExpr(t, 2)}
	if rapid.IntRange(0, 3).Draw(t, label+"generated") == 0 {
		s.Generated = true
		s.Domain = genString().Draw(t, label+"Domain")
	}
	return s
}

func genLimit(t *rapid.T, label string) Limit {
	n := rapid.IntRange(1, 3).Draw(t, label+"path#")
	l := Limit{Comments: genComments(t, label+"Comment")}
	for range n {
		l.Path = append(l.Path, genName(lower).Draw(t, label+"path"))
	}
	switch rapid.IntRange(0, 2).Draw(t, label+"kind") {
	case 0:
		l.Kind = LimitRange
		l.Low, l.High = genNumber(false).Draw(t, "low"), genNumber(false).Draw(t, "high")
	case 1:
		l.Kind = LimitMembers
		m := rapid.IntRange(1, 3).Draw(t, label+"members#")
		for range m {
			l.Members = append(l.Members, genName(lower).Draw(t, label+"member"))
		}
	default:
		l.Kind = LimitSize
		l.Low, l.High = genNumber(true).Draw(t, "low"), genNumber(true).Draw(t, "high")
	}
	return l
}

func genAssignment(t *rapid.T, label string) Assignment {
	return Assignment{Comments: genComments(t, label+"Comment"), Symbol: genName(lower).Draw(t, label), Value: genValue(t, 3)}
}

func genCase(t *rapid.T, label string) CaseDecl {
	return CaseDecl{
		Comments:    genComments(t, label+"Comment"),
		Name:        genName(kebab).Draw(t, label),
		Summary:     genOptSummary(t, label+"Summary"),
		Assignments: some(t, label+"Assignment", 3, genAssignment),
		Trailing:    genComments(t, label+"Trailing"),
	}
}

func genArgs(t *rapid.T) []string {
	return some(t, "arg", 3, func(t *rapid.T, label string) string { return genName(lower).Draw(t, label) })
}

func genSteps(t *rapid.T, depth int) []Step {
	max := 3
	if depth <= 0 {
		max = 0
	}
	return some(t, "step", max, func(t *rapid.T, _ string) Step { return genStep(t, depth-1) })
}

func genStep(t *rapid.T, depth int) Step {
	comments := genComments(t, "stepComment")
	switch rapid.IntRange(0, 6).Draw(t, "stepKind") {
	case 0:
		s := &CallStep{Comments: comments, From: genParticipant(t, "from"), Op: genOpRef(t, "op"), Args: genArgs(t)}
		s.Steps = genSteps(t, depth)
		s.Trailing = genComments(t, "callTrailing")
		s.Handlers = some(t, "on", 2, func(t *rapid.T, label string) OnBlock {
			return OnBlock{Comments: genComments(t, label+"Comment"), Error: genTypeRef(t, label), Steps: genSteps(t, depth), Trailing: genComments(t, label+"Trailing")}
		})
		return s
	case 1:
		return &SendStep{Comments: comments, From: genParticipant(t, "from"), Op: genOpRef(t, "op"), Args: genArgs(t)}
	case 2:
		return &EmitStep{Comments: comments, From: genParticipant(t, "from"), Event: genTypeRef(t, "event"), Args: genArgs(t)}
	case 3:
		return &AsyncStep{Comments: comments, Op: genOpRef(t, "async"), Steps: genSteps(t, depth), Trailing: genComments(t, "asyncTrailing")}
	case 4:
		s := &AltStep{Comments: comments}
		n := rapid.IntRange(1, 3).Draw(t, "branches")
		for range n {
			s.Branches = append(s.Branches, Branch{Label: genString().Draw(t, "branchLabel"), Steps: genSteps(t, depth), Trailing: genComments(t, "branchTrailing")})
		}
		return s
	case 5:
		return &OptStep{Comments: comments, Label: genString().Draw(t, "optLabel"), Steps: genSteps(t, depth), Trailing: genComments(t, "optTrailing")}
	}
	return &LoopStep{Comments: comments, Label: genString().Draw(t, "loopLabel"), Steps: genSteps(t, depth), Trailing: genComments(t, "loopTrailing")}
}

func genFlow(t *rapid.T, label string) FlowDecl {
	return FlowDecl{
		Comments: genComments(t, label+"Comment"),
		Name:     genName(kebab).Draw(t, label),
		Summary:  genSummary().Draw(t, label+"Summary"),
		Symbols:  some(t, "symbol", 2, genSymbol),
		Limits:   some(t, "limit", 3, genLimit),
		Steps:    genSteps(t, 3),
		Cases:    some(t, "case", 2, genCase),
		Trailing: genComments(t, label+"Trailing"),
	}
}

func genProperty(t *rapid.T, label string) PropertyDecl {
	about := About{Comments: genComments(t, "aboutComment")}
	n := rapid.IntRange(1, 3).Draw(t, "about#")
	for range n {
		about.Refs = append(about.Refs, genAnyRef(t, "about"))
	}
	return PropertyDecl{
		Comments: genComments(t, label+"Comment"),
		Name:     genName(kebab).Draw(t, label),
		Form:     rapid.SampledFrom(propertyForms).Draw(t, label+"Form"),
		Summary:  genSummary().Draw(t, label+"Summary"),
		About:    about,
		Symbols:  some(t, "symbol", 2, genSymbol),
		Limits:   some(t, "limit", 3, genLimit),
		Cases:    some(t, "case", 2, genCase),
		Trailing: genComments(t, label+"Trailing"),
	}
}

func genSystemFile(t *rapid.T) *SystemFile {
	sf := &SystemFile{System: SystemDecl{
		Comments: genComments(t, "systemComment"),
		Name:     genName(kebab).Draw(t, "system"),
		Summary:  genSummary().Draw(t, "systemSummary"),
	}}
	sf.Notations = some(t, "notation", 2, func(t *rapid.T, label string) NotationDecl {
		return NotationDecl{Comments: genComments(t, label+"Comment"), Name: rapid.SampledFrom([]string{"ddd", "c4"}).Draw(t, label)}
	})
	sf.Languages = some(t, "language", 2, func(t *rapid.T, label string) LanguageDecl {
		d := LanguageDecl{Comments: genComments(t, label+"Comment"), Name: rapid.SampledFrom([]string{"go", "python"}).Draw(t, label)}
		n := rapid.IntRange(1, 2).Draw(t, "roots")
		for range n {
			d.Roots = append(d.Roots, Root{Comments: genComments(t, "rootComment"), Path: genString().Draw(t, "root")})
		}
		d.Trailing = genComments(t, label+"Trailing")
		return d
	})
	sf.Actors = some(t, "actor", 3, func(t *rapid.T, label string) ActorDecl {
		d := ActorDecl{
			Comments: genComments(t, label+"Comment"),
			Name:     genName(kebab).Draw(t, label),
			Form:     rapid.SampledFrom([]string{"person", "external"}).Draw(t, label+"Form"),
			Summary:  genSummary().Draw(t, label+"Summary"),
		}
		if d.Form == "external" {
			d.Types = some(t, "actorType", 2, genType)
			d.Ops = some(t, "actorOp", 2, genOp)
			d.Trailing = genComments(t, label+"Trailing")
		}
		return d
	})
	return sf
}

func genComponentFile(t *rapid.T) *ComponentFile {
	c := ComponentDecl{
		Comments: genComments(t, "componentComment"),
		Path:     genSegments(t, "componentPath", 2, kebab),
		Summary:  genSummary().Draw(t, "componentSummary"),
		Labels:   genLabels(t),
		Paths:    Paths{Comments: genComments(t, "pathsComment")},
		Trailing: genComments(t, "componentTrailing"),
	}
	if rapid.Bool().Draw(t, "focus?") {
		c.Focus = &Focus{Comments: genComments(t, "focusComment"), Level: rapid.SampledFrom([]string{"core", "support", "utility"}).Draw(t, "focus")}
	}
	n := rapid.IntRange(1, 2).Draw(t, "paths#")
	for range n {
		c.Paths.Patterns = append(c.Paths.Patterns, genString().Draw(t, "paths"))
	}
	if rapid.Bool().Draw(t, "uses?") {
		c.Uses = &Uses{Comments: genComments(t, "usesComment")}
		m := rapid.IntRange(1, 3).Draw(t, "uses#")
		for range m {
			c.Uses.Refs = append(c.Uses.Refs, genAnyRef(t, "uses"))
		}
	}
	return &ComponentFile{
		Component:  c,
		Types:      some(t, "type", 3, genType),
		Ops:        some(t, "op", 2, genOp),
		Flows:      some(t, "flow", 2, genFlow),
		Properties: some(t, "property", 2, genProperty),
	}
}

func genCorpusFile(t *rapid.T) *CorpusFile {
	return &CorpusFile{
		Corpus: CorpusDecl{Comments: genComments(t, "corpusComment"), Target: genAnyRef(t, "corpus")},
		Cases: some(t, "recorded", 3, func(t *rapid.T, label string) RecordedCase {
			return RecordedCase{
				Comments:    genComments(t, label+"Comment"),
				Name:        genName(kebab).Draw(t, label),
				Reason:      rapid.SampledFrom([]string{"coverage", "counterexample"}).Draw(t, label+"Reason"),
				Assignments: some(t, label+"Assignment", 3, genAssignment),
				Trailing:    genComments(t, label+"Trailing"),
			}
		}),
	}
}

func genFile(t *rapid.T) *File {
	f := &File{Kind: rapid.SampledFrom([]FileKind{FileKindSystem, FileKindComponent, FileKindCorpus}).Draw(t, "kind")}
	switch f.Kind {
	case FileKindSystem:
		f.System = genSystemFile(t)
	case FileKindComponent:
		f.Component = genComponentFile(t)
	default:
		f.Corpus = genCorpusFile(t)
	}
	f.Trailing = genComments(t, "fileTrailing")
	return f
}

// writer writes a tree as text with random layout that the grammar allows:
// any indentation, any number of spaces between tokens (none where the
// tokens stay apart), trailing spaces, blank lines anywhere between lines,
// optional braces around an empty optional block, values broken between their
// items with an optional trailing comma, and any number of final newlines.
type writer struct {
	t    *rapid.T
	b    strings.Builder
	line []string
}

func (w *writer) tok(toks ...string) { w.line = append(w.line, toks...) }

func (w *writer) spaces(label string, max int) string {
	return strings.Repeat(" ", rapid.IntRange(0, max).Draw(w.t, label))
}

// flush ends the current line, then writes blank lines.
func (w *writer) flush() {
	w.b.WriteString(w.spaces("indent", 6))
	for i, tok := range w.line {
		if i > 0 {
			sep := w.spaces("sep", 2)
			if sep == "" && needsSpace(w.line[i-1], tok) {
				sep = " "
			}
			w.b.WriteString(sep)
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
	alnum := func(c byte) bool { return isLetter(c) || isDigit(c) }
	return alnum(a[len(a)-1]) && (alnum(b[0]) || b[0] == '-' && len(b) > 1 && alnum(b[1]))
}

func (w *writer) comments(cs []Comment) {
	for _, c := range cs {
		w.tok("//" + c.Text)
		w.flush()
	}
}

func (w *writer) ref(r Ref) {
	for i, s := range r.Segments {
		if i > 0 {
			w.tok(".")
		}
		w.tok(s)
	}
}

func (w *writer) refs(rs []Ref) {
	for i, r := range rs {
		if i > 0 {
			w.tok(",")
		}
		w.ref(r)
	}
}

func (w *writer) open() {
	w.tok("{")
	w.flush()
}

func (w *writer) close(trailing []Comment) {
	w.comments(trailing)
	w.tok("}")
}

// optBlock writes a block that may be left out when it has nothing in it.
func (w *writer) optBlock(empty bool, members func(), trailing []Comment) {
	if empty && len(trailing) == 0 && rapid.Bool().Draw(w.t, "noBraces") {
		return
	}
	w.open()
	members()
	w.close(trailing)
}

func (w *writer) file(f *File) {
	switch {
	case f.System != nil:
		w.systemFile(f.System)
	case f.Component != nil:
		w.componentFile(f.Component)
	case f.Corpus != nil:
		w.corpusFile(f.Corpus)
	}
	w.comments(f.Trailing)
	text := strings.TrimRight(w.b.String(), "\n ")
	w.b.Reset()
	w.b.WriteString(text + strings.Repeat("\n", rapid.IntRange(0, 2).Draw(w.t, "finalNewlines")))
}

func (w *writer) systemFile(sf *SystemFile) {
	w.comments(sf.System.Comments)
	w.tok("system", sf.System.Name, quote(sf.System.Summary))
	w.flush()
	for _, n := range sf.Notations {
		w.comments(n.Comments)
		w.tok("notation", n.Name)
		w.flush()
	}
	for _, l := range sf.Languages {
		w.comments(l.Comments)
		w.tok("language", l.Name)
		w.open()
		for _, r := range l.Roots {
			w.comments(r.Comments)
			w.tok("root", quote(r.Path))
			w.flush()
		}
		w.close(l.Trailing)
		w.flush()
	}
	for _, a := range sf.Actors {
		w.comments(a.Comments)
		w.tok("actor", a.Name, a.Form, quote(a.Summary))
		if a.Form == "external" {
			w.optBlock(len(a.Types)+len(a.Ops) == 0, func() {
				for _, t := range a.Types {
					w.typeDecl(t)
				}
				for _, o := range a.Ops {
					w.opDecl(o)
				}
			}, a.Trailing)
		}
		w.flush()
	}
}

func (w *writer) componentFile(cf *ComponentFile) {
	c := cf.Component
	w.comments(c.Comments)
	w.tok("component")
	w.ref(Ref{Segments: c.Path})
	w.tok(quote(c.Summary))
	w.open()
	w.labels(c.Labels)
	if c.Focus != nil {
		w.comments(c.Focus.Comments)
		w.tok("focus", c.Focus.Level)
		w.flush()
	}
	w.comments(c.Paths.Comments)
	w.tok("paths")
	for i, p := range c.Paths.Patterns {
		if i > 0 {
			w.tok(",")
		}
		w.tok(quote(p))
	}
	w.flush()
	if c.Uses != nil {
		w.comments(c.Uses.Comments)
		w.tok("uses")
		w.refs(c.Uses.Refs)
		w.flush()
	}
	w.close(c.Trailing)
	w.flush()
	for _, t := range cf.Types {
		w.typeDecl(t)
	}
	for _, o := range cf.Ops {
		w.opDecl(o)
	}
	for _, f := range cf.Flows {
		w.flow(f)
	}
	for _, p := range cf.Properties {
		w.property(p)
	}
}

func (w *writer) labels(ls []Label) {
	for _, l := range ls {
		w.comments(l.Comments)
		w.tok("label", l.Name)
		w.flush()
	}
}

func (w *writer) typeDecl(t TypeDecl) {
	w.comments(t.Comments)
	w.tok("type", t.Name, t.Form, quote(t.Summary))
	empty := len(t.Labels)+len(t.Extends)+len(t.Implements)+len(t.Fields)+len(t.Values)+len(t.Ops) == 0
	w.optBlock(empty, func() {
		w.labels(t.Labels)
		for _, s := range t.Extends {
			w.comments(s.Comments)
			w.tok("extends")
			w.ref(s.Type)
			w.flush()
		}
		for _, s := range t.Implements {
			w.comments(s.Comments)
			w.tok("implements")
			w.ref(s.Type)
			w.flush()
		}
		for _, f := range t.Fields {
			w.comments(f.Comments)
			w.tok("field", f.Name, ":")
			w.typeExpr(f.Type)
			w.flush()
		}
		for _, v := range t.Values {
			w.comments(v.Comments)
			w.tok("value", v.Name)
			w.flush()
		}
		for _, o := range t.Ops {
			w.opDecl(o)
		}
	}, t.Trailing)
	w.flush()
}

func (w *writer) opDecl(o OpDecl) {
	w.comments(o.Comments)
	w.tok("op", o.Name, "(")
	for i, p := range o.Params {
		if i > 0 {
			w.tok(",")
		}
		w.tok(p.Name, ":")
		w.typeExpr(p.Type)
	}
	w.tok(")")
	if o.Result != nil {
		w.tok("->")
		w.typeExpr(o.Result)
	}
	if len(o.Raises) > 0 {
		w.tok("raises")
		w.refs(o.Raises)
	}
	if o.Handles != nil {
		w.tok("handles")
		w.ref(*o.Handles)
	}
	if o.Summary != "" {
		w.tok(quote(o.Summary))
	}
	w.flush()
}

func (w *writer) typeExpr(t TypeExpr) {
	switch t := t.(type) {
	case *BuiltinType:
		w.tok(t.Name)
	case *NamedType:
		w.ref(t.Ref)
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

func (w *writer) flow(f FlowDecl) {
	w.comments(f.Comments)
	w.tok("flow", f.Name, quote(f.Summary))
	w.open()
	w.symbols(f.Symbols)
	w.limits(f.Limits)
	w.steps(f.Steps)
	w.cases(f.Cases)
	w.close(f.Trailing)
	w.flush()
}

func (w *writer) property(p PropertyDecl) {
	w.comments(p.Comments)
	w.tok("property", p.Name, p.Form, quote(p.Summary))
	w.open()
	w.comments(p.About.Comments)
	w.tok("about")
	w.refs(p.About.Refs)
	w.flush()
	w.symbols(p.Symbols)
	w.limits(p.Limits)
	w.cases(p.Cases)
	w.close(p.Trailing)
	w.flush()
}

func (w *writer) symbols(ss []Symbol) {
	for _, s := range ss {
		w.comments(s.Comments)
		w.tok("forall", s.Name, ":")
		w.typeExpr(s.Type)
		if s.Generated {
			w.tok("generated", quote(s.Domain))
		}
		w.flush()
	}
}

func (w *writer) limits(ls []Limit) {
	for _, l := range ls {
		w.comments(l.Comments)
		w.tok("where")
		w.ref(Ref{Segments: l.Path})
		switch l.Kind {
		case LimitRange:
			w.tok("in", l.Low, "..", l.High)
		case LimitMembers:
			w.tok("in", "{")
			for i, m := range l.Members {
				if i > 0 {
					w.tok(",")
				}
				w.tok(m)
			}
			w.tok("}")
		case LimitSize:
			w.tok("size", l.Low, "..", l.High)
		}
		w.flush()
	}
}

func (w *writer) cases(cs []CaseDecl) {
	for _, c := range cs {
		w.comments(c.Comments)
		w.tok("case", c.Name)
		if c.Summary != "" {
			w.tok(quote(c.Summary))
		}
		w.open()
		w.assignments(c.Assignments)
		w.close(c.Trailing)
		w.flush()
	}
}

func (w *writer) assignments(as []Assignment) {
	for _, a := range as {
		w.comments(a.Comments)
		w.tok(a.Symbol, "=")
		w.value(a.Value)
		w.flush()
	}
}

// value writes v; between its items it may break the line.
func (w *writer) value(v Value) {
	open, items, closer := parts(v)
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
			w.tok(strings.TrimSuffix(it.prefix, ": "), ":")
		}
		w.value(it.value)
		breakable()
		if i < len(items)-1 || rapid.Bool().Draw(w.t, "trailingComma") {
			w.tok(",")
		}
	}
	breakable()
	w.tok(closer)
}

func (w *writer) steps(steps []Step) {
	for _, s := range steps {
		w.step(s)
	}
}

func (w *writer) stepBlock(steps []Step, trailing []Comment) {
	w.open()
	w.steps(steps)
	w.close(trailing)
}

func (w *writer) step(s Step) {
	switch s := s.(type) {
	case *CallStep:
		w.comments(s.Comments)
		w.ref(s.From)
		w.tok("->")
		w.ref(s.Op)
		w.args(s.Args)
		w.optBlock(len(s.Steps) == 0, func() { w.steps(s.Steps) }, s.Trailing)
		w.flush()
		for _, h := range s.Handlers {
			w.comments(h.Comments)
			w.tok("on")
			w.ref(h.Error)
			w.stepBlock(h.Steps, h.Trailing)
			w.flush()
		}
	case *SendStep:
		w.comments(s.Comments)
		w.ref(s.From)
		w.tok("->>")
		w.ref(s.Op)
		w.args(s.Args)
		w.flush()
	case *EmitStep:
		w.comments(s.Comments)
		w.ref(s.From)
		w.tok("emits")
		w.ref(s.Event)
		w.args(s.Args)
		w.flush()
	case *AsyncStep:
		w.comments(s.Comments)
		w.tok("async")
		w.ref(s.Op)
		w.stepBlock(s.Steps, s.Trailing)
		w.flush()
	case *AltStep:
		w.comments(s.Comments)
		for i, br := range s.Branches {
			if i == 0 {
				w.tok("alt", quote(br.Label))
			} else {
				w.tok("else", quote(br.Label))
			}
			w.stepBlock(br.Steps, br.Trailing)
		}
		w.flush()
	case *OptStep:
		w.comments(s.Comments)
		w.tok("opt", quote(s.Label))
		w.stepBlock(s.Steps, s.Trailing)
		w.flush()
	case *LoopStep:
		w.comments(s.Comments)
		w.tok("loop", quote(s.Label))
		w.stepBlock(s.Steps, s.Trailing)
		w.flush()
	}
}

func (w *writer) args(as []string) {
	w.tok("(")
	for i, a := range as {
		if i > 0 {
			w.tok(",")
		}
		w.tok(a)
	}
	w.tok(")")
}

func (w *writer) corpusFile(cf *CorpusFile) {
	w.comments(cf.Corpus.Comments)
	w.tok("corpus")
	w.ref(cf.Corpus.Target)
	w.flush()
	for _, c := range cf.Cases {
		w.comments(c.Comments)
		w.tok("case", c.Name, c.Reason)
		w.open()
		w.assignments(c.Assignments)
		w.close(c.Trailing)
		w.flush()
	}
}

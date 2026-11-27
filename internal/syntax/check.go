package syntax

import (
	"cmp"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/diag"
)

// check reports what the grammar's tags cannot say: which declarations a file
// of each kind holds and in what order, the order and number of the members of
// each block, the three name classes, the forms of references, well-formed
// summaries, reserved names, and the few rules of docs/language.md §3 that need a form.
func check(at locator, f *File) []diag.Diagnostic {
	c := &checker{at: at}
	c.file(f)
	c.walk(reflect.ValueOf(f.Decls), false)
	c.checkExpressions(reflect.ValueOf(f.Decls))
	slices.SortStableFunc(c.diags, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column), strings.Compare(a.Code, b.Code))
	})
	return c.diags
}

type checker struct {
	at    locator
	diags []diag.Diagnostic
	next  int // the offset up to which walk has placed the tree's tokens
}

func (c *checker) report(pos lexer.Position, code, message, fix string) {
	c.diags = append(c.diags, c.at.diagnostic(pos.Offset, code, message, fix))
}

// order is the sequence in which the lines of one level must come: the
// keywords of each rank, the first rank first; the single ones may appear at
// most once.
type order struct {
	what   string
	code   string
	first  string
	ranks  map[string]int
	single []string
}

func newOrder(what, code string, ranks [][]string, single ...string) order {
	o := order{what: what, code: code, first: ranks[0][0], ranks: map[string]int{}, single: single}
	for i, group := range ranks {
		for _, kw := range group {
			o.ranks[kw] = i
		}
	}
	return o
}

// step names the steps that start with a sender rather than a keyword.
const step = "step"

var (
	fileOrders = map[FileKind]order{
		FileKindSystem: newOrder("system file", "syntax.declaration-order",
			[][]string{{"system"}, {"notation"}, {"language"}, {"actor"}}, "system"),
		FileKindComponent: newOrder("component file", "syntax.declaration-order",
			[][]string{{"component"}, {"type"}, {"op"}, {"flow"}, {"property"}}, "component"),
		FileKindCorpus: newOrder("corpus file", "syntax.declaration-order", [][]string{{"corpus"}, {"case"}}, "corpus"),
	}
	languageOrder  = newOrder("language block", "syntax.member-order", [][]string{{"root"}})
	componentOrder = newOrder("component block", "syntax.member-order", [][]string{{"label"}, {"focus"}, {"paths"}, {"uses"}}, "focus", "paths", "uses")
	externalOrder  = newOrder("external actor", "syntax.member-order", [][]string{{"type"}, {"op"}})
	typeOrder      = newOrder("type", "syntax.member-order", [][]string{{"label"}, {"extends"}, {"implements"}, {"field", "value"}, {"op"}})
	flowOrder      = newOrder("flow", "syntax.member-order",
		[][]string{{"forall"}, {"where"}, {step, "on", "async", "alt", "opt", "loop"}, {"within"}, {"ensure", "manual"}, {"case"}}, "within", "manual")
	propertyOrder = newOrder("property", "syntax.member-order", [][]string{{"about"}, {"forall"}, {"where"}, {"observe"}, {"requires"}, {"changes"}, {"call"}, {"within"}, {"ensure", "manual"}, {"case"}}, "about", "changes", "call", "within", "manual")
)

var (
	commentType = reflect.TypeFor[Comment]()
	leadingWord = regexp.MustCompile(`^'([a-z]+)'`)
)

// keyword names a line by the keyword its grammar starts with: "" for a
// comment, and step for a step that starts with its sender.
func keyword(line any) string {
	t := reflect.TypeOf(line).Elem()
	if t == commentType {
		return ""
	}
	if kw := lead(t); kw != "" {
		return kw
	}
	return step
}

// lead is the keyword that the grammar of the struct type t starts with, read
// from the struct tag of its first field, or "" when it starts otherwise.
func lead(t reflect.Type) string {
	for i := range t.NumField() {
		if tag, ok := t.Field(i).Tag.Lookup("parser"); ok {
			if m := leadingWord.FindStringSubmatch(tag); m != nil {
				return m[1]
			}
			return ""
		}
	}
	return ""
}

func position(line any) lexer.Position {
	return reflect.ValueOf(line).Elem().FieldByName("Pos").Interface().(lexer.Position)
}

// lines checks the order of one level's lines and returns how often each keyword appears.
func lines[L any](c *checker, o order, ls []L) map[string]int {
	seen := map[string]int{}
	last := ""
	for _, l := range ls {
		kw := keyword(l)
		if kw == "" {
			continue
		}
		pos := position(l)
		rank, ok := o.ranks[kw]
		switch {
		case !ok:
			c.report(pos, "syntax.unknown-declaration", fmt.Sprintf("a %s may not contain a %s declaration", o.what, kw), "remove it, or move it to a file that may hold it")
		case slices.Contains(o.single, kw) && seen[kw] > 0:
			c.report(pos, "syntax.duplicate-member", fmt.Sprintf("a %s may have only one %s", o.what, kw), "remove the repeated "+kw)
		case last != "" && rank < o.ranks[last]:
			c.report(pos, o.code, fmt.Sprintf("in a %s, %s must come before %s", o.what, kw, last), fmt.Sprintf("move this %s before every %s", kw, last))
		}
		seen[kw]++
		if ok && (last == "" || rank > o.ranks[last]) {
			last = kw
		}
	}
	return seen
}

func (c *checker) require(seen map[string]int, kw string, pos lexer.Position, message, fix string) {
	if seen[kw] == 0 {
		c.report(pos, "syntax.missing-member", message, fix)
	}
}

func (c *checker) file(f *File) {
	o := fileOrders[f.Kind]
	header := o.first
	first := slices.IndexFunc(f.Decls, func(d Decl) bool { return keyword(d) != "" })
	switch {
	case first < 0:
		c.report(lexer.Position{}, "syntax.missing-header", fmt.Sprintf("a %s must start with a %s declaration", o.what, header), "start the file with a "+header+" declaration")
	case keyword(f.Decls[first]) != header:
		c.report(position(f.Decls[first]), "syntax.missing-header", fmt.Sprintf("a %s must start with a %s declaration", o.what, header), "start the file with a "+header+" declaration")
	}
	lines(c, o, f.Decls)
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *LanguageDecl:
			seen := lines(c, languageOrder, d.Lines)
			c.require(seen, "root", d.Pos, "a language block needs at least one root", `add root "directory" to the block`)
		case *ExternalDecl:
			lines(c, externalOrder, d.Lines)
			for _, l := range d.Lines {
				if t, ok := l.(*TypeDecl); ok {
					c.typeDecl(t)
				}
				if op, ok := l.(*OpDecl); ok {
					c.expressionName(string(op.Name), op.Pos)
				}
			}
		case *OpDecl:
			c.expressionName(string(d.Name), d.Pos)
		case *ComponentDecl:
			seen := lines(c, componentOrder, d.Lines)
			c.require(seen, "paths", d.Pos, "a component block needs paths", `add paths "pattern" to the component block`)
		case *TypeDecl:
			c.typeDecl(d)
		case *FlowDecl:
			seen := lines(c, flowOrder, d.Lines)
			steps(c, d.Lines)
			if seen["manual"] != 0 && seen["ensure"] != 0 {
				c.report(d.Pos, "syntax.mixed-claim", "manual cannot be combined with ensure", "use either a generated flow claim or a manual reason")
			}
			if seen["ensure"] != 0 || seen["within"] != 0 {
				found := false
				for _, line := range d.Lines {
					if _, ok := line.(*Comment); ok {
						continue
					}
					if _, ok := line.(Step); ok {
						found = true
						if _, call := line.(*Call); !call {
							c.report(position(line), "syntax.invalid-flow-call", "a checked flow must start with a synchronous call", "start the flow's steps with sender -> op(args)")
						}
						break
					}
				}
				if !found {
					c.report(d.Pos, "syntax.invalid-flow-call", "a checked flow needs a first synchronous call", "start the flow's steps with sender -> op(args)")
				}
			}
		case *PropertyDecl:
			seen := lines(c, propertyOrder, d.Lines)
			c.require(seen, "about", d.Pos, "a property needs about", "add about and what the property constrains as the first line of its block")
			c.propertyClaim(d, seen)
		}
	}
}

func (c *checker) propertyClaim(d *PropertyDecl, seen map[string]int) {
	if seen["ensure"]+seen["within"]+seen["manual"] == 0 {
		c.report(d.Pos, "syntax.missing-claim", "a property needs ensure, within or manual", "state its claim as ensure expressions or declare manual with a reason")
	}
	if seen["manual"] != 0 && seen["ensure"]+seen["within"] != 0 {
		c.report(d.Pos, "syntax.mixed-claim", "manual cannot be combined with ensure or within", "use either a generated claim or a manual reason")
	}
	checked := seen["ensure"]+seen["within"] != 0
	if d.Form == "postcondition" && checked && seen["call"] != 1 || d.Form != "postcondition" && seen["call"] != 0 || seen["manual"] != 0 && seen["call"] != 0 {
		c.report(d.Pos, "syntax.invalid-tested-call", "only a generated postcondition has exactly one call clause", "add one call to a generated postcondition, or remove the call from another property form")
	}
	if seen["within"] != 0 && seen["call"] == 0 {
		c.report(d.Pos, "syntax.invalid-time-budget", "a property's time budget needs a tested call", "add a call in a postcondition or remove within")
	}
	if seen["observe"] != 0 || seen["changes"] != 0 {
		if seen["observe"] == 0 || seen["changes"] != 1 || seen["call"] != 1 || d.Form != "postcondition" || seen["manual"] != 0 {
			c.report(d.Pos, "syntax.invalid-observation-scope", "observe and changes form a pair on a generated postcondition with one call", "declare both observe and changes in a generated postcondition")
		}
	}
	var none int
	for _, line := range d.Lines {
		switch x := line.(type) {
		case *Observe:
			if x.None {
				none++
			}
		case *Changes:
			if none != 0 && !x.None {
				c.report(x.Pos, "syntax.invalid-observation-scope", "observe none permits only changes none", "use changes none or observe the changed roots")
			}
		}
	}
	if none != 0 && seen["observe"] != 1 {
		c.report(d.Pos, "syntax.invalid-observation-scope", "observe none must be the only observation", "remove the other observe clauses")
	}
}

func (c *checker) typeDecl(d *TypeDecl) {
	lines(c, typeOrder, d.Lines)
	for _, l := range d.Lines {
		switch l := l.(type) {
		case *Field:
			if d.Form == "enum" || d.Form == "interface" {
				c.report(l.Pos, "syntax.field-not-allowed", fmt.Sprintf("an %s type may not have fields", d.Form), "remove the field, or change the type's form")
			}
		case *EnumValue:
			if d.Form != "enum" {
				c.report(l.Pos, "syntax.value-outside-enum", "only an enum may have values", "remove the value, or make the type an enum")
			}
		}
	}
}

// steps checks that each `on` follows a call or another `on`, in every step list.
func steps[L any](c *checker, ls []L) {
	prev := ""
	for _, l := range ls {
		kw := keyword(l)
		if kw == "" {
			continue
		}
		if kw == "on" && prev != "call" && prev != "on" {
			c.report(position(l), "syntax.on-without-call", "on must follow a call (->) or another on", "move the on block right after the call whose error it handles")
		}
		prev = kw
		switch l := any(l).(type) {
		case *Call:
			prev = "call"
			steps(c, l.Lines)
		case *OnBlock:
			steps(c, l.Lines)
		case *Async:
			steps(c, l.Lines)
		case *Alt:
			for _, b := range l.Branches {
				steps(c, b.Lines)
			}
		case *Opt:
			steps(c, l.Lines)
		case *Loop:
			steps(c, l.Lines)
		}
	}
}

var (
	positionType  = reflect.TypeFor[lexer.Position]()
	refType       = reflect.TypeFor[Ref]()
	lowerType     = reflect.TypeFor[Lower]()
	summaryType   = reflect.TypeFor[Summary]()
	textType      = reflect.TypeFor[Text]()
	numberType    = reflect.TypeFor[Number]()
	isStringToken = ofType("String")
	isNumberToken = ofType("Number")
)

func ofType(name string) func(lexer.Token) bool {
	return func(t lexer.Token) bool { return t.Type == symbols[name] }
}

func isWord(s string) func(lexer.Token) bool {
	return func(t lexer.Token) bool { return t.Type == symbols["Ident"] && t.Value == s }
}

// walk checks every name, reference, summary and size bound in the tree by
// its type. It visits the tree in the order of the text and places each of
// these tokens in the source, so that a diagnostic stands at the token it is
// about; sizeBound is set inside a size limit.
func (c *checker) walk(v reflect.Value, sizeBound bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			c.walk(v.Elem(), sizeBound)
		}
	case reflect.Slice:
		for i := range v.Len() {
			c.walk(v.Index(i), sizeBound)
		}
	case reflect.Struct:
		if v.Type() == positionType {
			return
		}
		if within, ok := v.Interface().(Within); ok {
			if _, err := time.ParseDuration(string(within.Duration)); err != nil {
				c.report(within.Pos, "syntax.invalid-time-budget", "within must contain a duration", `write a duration such as within "500ms"`)
			}
		}
		switch n := v.Interface().(type) {
		case Symbol:
			c.expressionName(string(n.Name), n.Pos)
		case NamedObservation:
			c.expressionName(string(n.Name), n.Pos)
		}
		if p := v.FieldByName("Pos"); p.IsValid() {
			// What a node holds comes after the keyword it starts with, which may be spelled like a name.
			c.next = max(c.next, p.Interface().(lexer.Position).Offset+len(lead(v.Type())))
		}
		if observe, ok := v.Interface().(Observe); ok && len(observe.Paths) != 0 {
			for _, path := range observe.Paths {
				for _, segment := range path.Segments {
					c.text(reflect.ValueOf(Lower(segment)), false)
				}
			}
			return
		}
		if c.reference(v) {
			return
		}
		l, ok := v.Interface().(IntervalLimit)
		for i := range v.NumField() {
			c.walk(v.Field(i), ok && l.Op == "size")
		}
	case reflect.String:
		c.text(v, sizeBound)
	}
}

// place finds the first token from next to the end of its line that match
// accepts, moves next past it and returns its position; when none does, it
// returns the position of next.
func (c *checker) place(match func(lexer.Token) bool) lexer.Position {
	for _, t := range c.at.tokens(c.next) {
		if match(t) {
			c.next = t.Pos.Offset + len(t.Value)
			return t.Pos
		}
	}
	return lexer.Position{Offset: c.next}
}

// reserved are the literals of values, which no lower-camel name may be.
var reserved = []string{"true", "false", "none"}

func matcher(pattern string) func(string) bool { return regexp.MustCompile(pattern).MatchString }

// nameClass is one of the three ways of writing a name (docs/language.md §2).
type nameClass struct {
	name    string
	pattern func(string) bool
}

var (
	kebabClass = nameClass{"in kebab-case, like order-line", matcher(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)}
	upperClass = nameClass{"in UpperCamelCase, like OrderLine", matcher(`^[A-Z][A-Za-z0-9]*$`)}
	lowerClass = nameClass{"in lowerCamelCase, like orderLine", matcher(`^[a-z][A-Za-z0-9]*$`)}
	classes    = map[reflect.Type]nameClass{
		reflect.TypeFor[Kebab](): kebabClass,
		reflect.TypeFor[Upper](): upperClass,
		reflect.TypeFor[Lower](): lowerClass,
	}
)

func (c *checker) text(v reflect.Value, sizeBound bool) {
	s := v.String()
	switch v.Type() {
	case summaryType:
		c.summary(s, c.place(isStringToken))
	case textType:
		c.place(isStringToken)
	case numberType:
		if pos := c.place(isNumberToken); sizeBound && strings.Contains(s, ".") {
			c.report(pos, "syntax.not-an-integer", fmt.Sprintf("a size bound must be an integer, not %s", s), "make the bound an integer")
		}
	}
	class, ok := classes[v.Type()]
	if !ok {
		return
	}
	pos := c.place(isWord(s))
	if !class.pattern(s) {
		c.report(pos, "syntax.invalid-name", fmt.Sprintf("the name %s must be written %s", s, class.name), "rename it "+class.name)
	}
	if v.Type() == lowerType && slices.Contains(reserved, s) {
		c.report(pos, "syntax.reserved-name", fmt.Sprintf("%s is reserved and may not be a name", s), "choose another name; true, false and none are literal values")
	}
}

func (c *checker) summary(s string, pos lexer.Position) {
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	switch {
	case s == "":
		c.report(pos, "syntax.empty-summary", "a summary may not be empty", "write one sentence that says what it is")
	case strings.ContainsAny(s, "\n\t"):
		c.report(pos, "syntax.summary-tab-or-newline", "a summary may not contain a newline or a tab", `write the summary on one line, without \n and \t`)
	case unicode.IsSpace(first) || unicode.IsSpace(last):
		c.report(pos, "syntax.summary-surrounding-space", "a summary may not begin or end with white space", "remove the white space at the summary's ends")
	}
}

// refForm is how the segments of one kind of reference are written: the
// classes its last segment may have, whether a type may stand before an
// operation, and kebab for the rest.
type refForm struct {
	what, example string
	last          []nameClass
	typeBefore    bool
}

var refForms = map[reflect.Type]refForm{
	reflect.TypeFor[TypeRef]():     {"type reference", "[component.]Type, like Order or inventory.Stock", []nameClass{upperClass}, false},
	reflect.TypeFor[OpRef]():       {"operation reference", "[component.][Type.]op, like parse, Order.total or inventory.Stock.reserve", []nameClass{lowerClass}, true},
	reflect.TypeFor[Participant](): {"sender", "an actor, a component or [component.]Type, like customer, payment or payment.Payments", []nameClass{kebabClass, upperClass}, false},
	reflect.TypeFor[Path]():        {"component path", "kebab-case names joined by ., like order or lang.golang", []nameClass{kebabClass}, false},
}

// reference places the segments of a reference and checks them, and tells
// whether v is a reference.
func (c *checker) reference(v reflect.Value) bool {
	form, typed := refForms[v.Type()]
	if !typed && v.Type() != refType {
		return false
	}
	segs := v.FieldByName("Segments").Interface().([]string)
	at := make([]lexer.Position, len(segs))
	for i, s := range segs {
		at[i] = c.place(isWord(s))
	}
	if !typed {
		for i, s := range segs {
			if !kebabClass.pattern(s) && !upperClass.pattern(s) && !lowerClass.pattern(s) {
				c.report(at[i], "syntax.invalid-name", fmt.Sprintf("%s in a reference is not a valid name", s), "write each segment in kebab-case, UpperCamelCase or lowerCamelCase")
				return true
			}
		}
		return true
	}
	valid := slices.ContainsFunc(form.last, func(nc nameClass) bool { return nc.pattern(segs[len(segs)-1]) })
	rest := segs[:len(segs)-1]
	if form.typeBefore && len(rest) > 0 && upperClass.pattern(rest[len(rest)-1]) {
		rest = rest[:len(rest)-1]
	}
	for _, s := range rest {
		valid = valid && kebabClass.pattern(s)
	}
	if !valid {
		c.report(at[0], "syntax.invalid-reference", fmt.Sprintf("the %s %s is not written %s", form.what, strings.Join(segs, "."), form.example),
			"rewrite the reference as "+form.example)
	}
	return true
}

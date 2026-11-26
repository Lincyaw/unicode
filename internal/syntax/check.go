package syntax

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/diag"
)

// check reports what the grammar's tags cannot say: which declarations a file
// of each kind holds and in what order, the order and number of the members of
// each block, the three name classes, the forms of references, well-formed
// summaries, reserved names, and the few rules of docs/format.md §3 that need a form.
func check(at locator, f *File) []diag.Diagnostic {
	c := &checker{at: at}
	c.file(f)
	c.walk(reflect.ValueOf(f.Decls), lexer.Position{})
	slices.SortStableFunc(c.diags, func(a, b diag.Diagnostic) int {
		if a.Position.Line != b.Position.Line {
			return a.Position.Line - b.Position.Line
		}
		return a.Position.Column - b.Position.Column
	})
	return c.diags
}

type checker struct {
	at    locator
	diags []diag.Diagnostic
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
		[][]string{{"forall"}, {"where"}, {step, "on", "async", "alt", "opt", "loop"}, {"case"}})
	propertyOrder = newOrder("property", "syntax.member-order", [][]string{{"about"}, {"forall"}, {"where"}, {"case"}}, "about")
)

var (
	commentType = reflect.TypeFor[Comment]()
	leadingWord = regexp.MustCompile(`^'([a-z]+)'`)
)

// keyword names a line by the keyword its grammar starts with, read from the
// struct tag of its first field: "" for a comment, and step for a step that
// starts with its sender.
func keyword(line any) string {
	t := reflect.TypeOf(line).Elem()
	if t == commentType {
		return ""
	}
	for i := range t.NumField() {
		if tag, ok := t.Field(i).Tag.Lookup("parser"); ok {
			if m := leadingWord.FindStringSubmatch(tag); m != nil {
				return m[1]
			}
			return step
		}
	}
	return step
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
			}
		case *ComponentDecl:
			seen := lines(c, componentOrder, d.Lines)
			c.require(seen, "paths", d.Pos, "a component block needs paths", `add paths "pattern" to the component block`)
		case *TypeDecl:
			c.typeDecl(d)
		case *FlowDecl:
			lines(c, flowOrder, d.Lines)
			steps(c, d.Lines)
		case *PropertyDecl:
			seen := lines(c, propertyOrder, d.Lines)
			c.require(seen, "about", d.Pos, "a property needs about", "add about and what the property constrains as the first line of its block")
		}
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

var positionType = reflect.TypeFor[lexer.Position]()

// walk checks every name, reference and summary in the tree by its type; a
// diagnostic stands at the start of the line or reference that holds it.
func (c *checker) walk(v reflect.Value, pos lexer.Position) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			c.walk(v.Elem(), pos)
		}
	case reflect.Slice:
		for i := range v.Len() {
			c.walk(v.Index(i), pos)
		}
	case reflect.Struct:
		if v.Type() == positionType {
			return
		}
		if p := v.FieldByName("Pos"); p.IsValid() {
			pos = p.Interface().(lexer.Position)
		}
		c.reference(v, pos)
		if l, ok := v.Interface().(IntervalLimit); ok && l.Op == "size" {
			c.integers(l)
		}
		for i := range v.NumField() {
			c.walk(v.Field(i), pos)
		}
	case reflect.String:
		c.text(v, pos)
	}
}

func (c *checker) integers(l IntervalLimit) {
	for _, n := range []Number{l.Low, l.High} {
		if strings.Contains(string(n), ".") {
			c.report(l.Pos, "syntax.not-an-integer", fmt.Sprintf("a size bound must be an integer, not %s", n), "make the bound an integer")
		}
	}
}

// reserved are the literals of values, which no lower-camel name may be.
var reserved = []string{"true", "false", "none"}

func matcher(pattern string) func(string) bool { return regexp.MustCompile(pattern).MatchString }

// nameClass is one of the three ways of writing a name (docs/format.md §2).
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

func (c *checker) text(v reflect.Value, pos lexer.Position) {
	s := v.String()
	if class, ok := classes[v.Type()]; ok && !class.pattern(s) {
		c.report(pos, "syntax.invalid-name", fmt.Sprintf("the name %s must be written %s", s, class.name), "rename it "+class.name)
	}
	if v.Type() == reflect.TypeFor[Lower]() && slices.Contains(reserved, s) {
		c.report(pos, "syntax.reserved-name", fmt.Sprintf("%s is reserved and may not be a name", s), "choose another name; true, false and none are literal values")
	}
	if v.Type() != reflect.TypeFor[Summary]() {
		return
	}
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

func (c *checker) reference(v reflect.Value, pos lexer.Position) {
	if v.Type() == reflect.TypeFor[Ref]() {
		for _, s := range v.Interface().(Ref).Segments {
			if !kebabClass.pattern(s) && !upperClass.pattern(s) && !lowerClass.pattern(s) {
				c.report(pos, "syntax.invalid-name", fmt.Sprintf("%s in a reference is not a valid name", s), "write each segment in kebab-case, UpperCamelCase or lowerCamelCase")
				return
			}
		}
		return
	}
	form, ok := refForms[v.Type()]
	if !ok {
		return
	}
	segs := v.FieldByName("Segments").Interface().([]string)
	valid := slices.ContainsFunc(form.last, func(nc nameClass) bool { return nc.pattern(segs[len(segs)-1]) })
	rest := segs[:len(segs)-1]
	if form.typeBefore && len(rest) > 0 && upperClass.pattern(rest[len(rest)-1]) {
		rest = rest[:len(rest)-1]
	}
	for _, s := range rest {
		valid = valid && kebabClass.pattern(s)
	}
	if !valid {
		c.report(pos, "syntax.invalid-reference", fmt.Sprintf("the %s %s is not written %s", form.what, strings.Join(segs, "."), form.example),
			"rewrite the reference as "+form.example)
	}
}

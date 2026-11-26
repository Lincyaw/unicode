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
const step = "步骤"

var (
	fileOrders = map[FileKind]order{
		FileKindSystem: newOrder("系统文件", "syntax.declaration-order",
			[][]string{{"system"}, {"notation"}, {"language"}, {"actor"}}, "system"),
		FileKindComponent: newOrder("组件文件", "syntax.declaration-order",
			[][]string{{"component"}, {"type"}, {"op"}, {"flow"}, {"property"}}, "component"),
		FileKindCorpus: newOrder("录制文件", "syntax.declaration-order", [][]string{{"corpus"}, {"case"}}, "corpus"),
	}
	languageOrder  = newOrder("language 块", "syntax.member-order", [][]string{{"root"}})
	componentOrder = newOrder("组件块", "syntax.member-order", [][]string{{"label"}, {"focus"}, {"paths"}, {"uses"}}, "focus", "paths", "uses")
	externalOrder  = newOrder("外部系统", "syntax.member-order", [][]string{{"type"}, {"op"}})
	typeOrder      = newOrder("类型", "syntax.member-order", [][]string{{"label"}, {"extends"}, {"implements"}, {"field", "value"}, {"op"}})
	flowOrder      = newOrder("流程", "syntax.member-order",
		[][]string{{"forall"}, {"where"}, {step, "on", "async", "alt", "opt", "loop"}, {"case"}})
	propertyOrder = newOrder("性质", "syntax.member-order", [][]string{{"about"}, {"forall"}, {"where"}, {"case"}}, "about")
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
			c.report(pos, "syntax.unknown-declaration", fmt.Sprintf("%s中不能有 %s 声明", o.what, kw), "删去它，或把它移到能容纳它的文件中")
		case slices.Contains(o.single, kw) && seen[kw] > 0:
			c.report(pos, "syntax.duplicate-member", fmt.Sprintf("%s中只能有一个 %s", o.what, kw), "删去重复的 "+kw)
		case last != "" && rank < o.ranks[last]:
			c.report(pos, o.code, fmt.Sprintf("%s中的 %s 必须写在 %s 之前", o.what, kw, last), fmt.Sprintf("把这个 %s 移到所有 %s 之前", kw, last))
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
		c.report(lexer.Position{}, "syntax.missing-header", fmt.Sprintf("%s必须以 %s 声明开头", o.what, header), "在文件开头写上 "+header+" 声明")
	case keyword(f.Decls[first]) != header:
		c.report(position(f.Decls[first]), "syntax.missing-header", fmt.Sprintf("%s必须以 %s 声明开头", o.what, header), "在文件开头写上 "+header+" 声明")
	}
	lines(c, o, f.Decls)
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *LanguageDecl:
			seen := lines(c, languageOrder, d.Lines)
			c.require(seen, "root", d.Pos, "language 块中至少要有一个 root", "在块中写上 root \"源码根目录\"")
		case *ExternalDecl:
			lines(c, externalOrder, d.Lines)
			for _, l := range d.Lines {
				if t, ok := l.(*TypeDecl); ok {
					c.typeDecl(t)
				}
			}
		case *ComponentDecl:
			seen := lines(c, componentOrder, d.Lines)
			c.require(seen, "paths", d.Pos, "组件块中必须有 paths", "在组件块中写上 paths \"路径模式\"")
		case *TypeDecl:
			c.typeDecl(d)
		case *FlowDecl:
			lines(c, flowOrder, d.Lines)
			steps(c, d.Lines)
		case *PropertyDecl:
			seen := lines(c, propertyOrder, d.Lines)
			c.require(seen, "about", d.Pos, "性质必须写 about", "在性质的块中第一行写上 about 和它约束的对象")
		}
	}
}

func (c *checker) typeDecl(d *TypeDecl) {
	lines(c, typeOrder, d.Lines)
	for _, l := range d.Lines {
		switch l := l.(type) {
		case *Field:
			if d.Form == "enum" || d.Form == "interface" {
				c.report(l.Pos, "syntax.field-not-allowed", fmt.Sprintf("%s 类型不能有 field", d.Form), "删去这个 field，或改变类型的形式")
			}
		case *EnumValue:
			if d.Form != "enum" {
				c.report(l.Pos, "syntax.value-outside-enum", "只有 enum 类型才能有 value", "删去这个 value，或把类型的形式改成 enum")
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
			c.report(position(l), "syntax.on-without-call", "on 只能紧跟在一个调用（->）或另一个 on 之后", "把这个 on 块移到它处理的那个调用之后")
		}
		prev = kw
		switch l := any(l).(type) {
		case *Call:
			prev = "call"
			steps(c, l.Lines)
		case *On:
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
			c.report(l.Pos, "syntax.not-an-integer", fmt.Sprintf("大小的界限必须是整数，不能是 %s", n), "把界限改成整数")
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
	kebabClass = nameClass{"短横线写法（如 order-line）", matcher(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)}
	upperClass = nameClass{"大驼峰写法（如 OrderLine）", matcher(`^[A-Z][A-Za-z0-9]*$`)}
	lowerClass = nameClass{"小驼峰写法（如 orderLine）", matcher(`^[a-z][A-Za-z0-9]*$`)}
	classes    = map[reflect.Type]nameClass{
		reflect.TypeFor[Kebab](): kebabClass,
		reflect.TypeFor[Upper](): upperClass,
		reflect.TypeFor[Lower](): lowerClass,
	}
)

func (c *checker) text(v reflect.Value, pos lexer.Position) {
	s := v.String()
	if class, ok := classes[v.Type()]; ok && !class.pattern(s) {
		c.report(pos, "syntax.invalid-name", fmt.Sprintf("名称 %s 必须用%s", s, class.name), "把它改成"+class.name)
	}
	if v.Type() == reflect.TypeFor[Lower]() && slices.Contains(reserved, s) {
		c.report(pos, "syntax.reserved-name", fmt.Sprintf("%s 是保留字，不能用作名称", s), "换一个名称；true、false 和 none 是值的字面量")
	}
	if v.Type() != reflect.TypeFor[Summary]() {
		return
	}
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	switch {
	case s == "":
		c.report(pos, "syntax.empty-summary", "摘要不能为空", "写一句话说明它")
	case strings.ContainsAny(s, "\n\t"):
		c.report(pos, "syntax.summary-tab-or-newline", "摘要不能含换行或制表符", "把摘要写成一行，去掉其中的 \\n 和 \\t")
	case unicode.IsSpace(first) || unicode.IsSpace(last):
		c.report(pos, "syntax.summary-surrounding-space", "摘要的首尾不能有空白", "删去摘要首尾的空白")
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
	reflect.TypeFor[TypeRef]():       {"类型引用", "[组件.]类型，例如 Order、inventory.Stock", []nameClass{upperClass}, false},
	reflect.TypeFor[OpRef]():         {"操作引用", "[组件.][类型.]操作，例如 parse、Order.total、inventory.Stock.reserve", []nameClass{lowerClass}, true},
	reflect.TypeFor[Participant]():   {"发起方", "参与者、组件或 [组件.]类型，例如 customer、payment、payment.Payments", []nameClass{kebabClass, upperClass}, false},
	reflect.TypeFor[ComponentPath](): {"组件路径", "以 . 连接的短横线名称，例如 order、lang.golang", []nameClass{kebabClass}, false},
}

func (c *checker) reference(v reflect.Value, pos lexer.Position) {
	if v.Type() == reflect.TypeFor[Ref]() {
		for _, s := range v.Interface().(Ref).Segments {
			if !kebabClass.pattern(s) && !upperClass.pattern(s) && !lowerClass.pattern(s) {
				c.report(pos, "syntax.invalid-name", fmt.Sprintf("引用中的 %s 不是合法的名称", s), "名称用短横线、大驼峰或小驼峰写法")
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
		c.report(pos, "syntax.invalid-reference", fmt.Sprintf("%s %s 不合写法：应当写作 %s", form.what, strings.Join(segs, "."), form.example),
			"按 "+form.example+" 改写这个引用")
	}
}

package syntax

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lincyaw/unicode/internal/diag"
)

// Parsed is the result of Parse. File is set exactly when there are no diagnostics.
//
//uc:realizes syntax.Parsed
type Parsed struct {
	File        *File
	Diagnostics []diag.Diagnostic
}

// Parse parses one file with the lexis and grammar of docs/format.md §2–§3.
// It never panics: every problem is a diagnostic.
//
//uc:realizes syntax.parse
func Parse(path string, kind FileKind, src []byte) Parsed {
	p := &parser{path: path}
	p.toks, p.bad = lex(src, p.add)
	f := &File{Path: path, Kind: kind}
	switch kind {
	case FileKindSystem:
		f.System = p.systemFile()
	case FileKindComponent:
		f.Component = p.componentFile()
	case FileKindCorpus:
		f.Corpus = p.corpusFile()
	}
	f.Trailing = p.trailing
	if len(p.diags) > 0 {
		slices.SortStableFunc(p.diags, func(a, b diag.Diagnostic) int {
			return cmp.Or(cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column))
		})
		return Parsed{Diagnostics: p.diags}
	}
	return Parsed{File: f}
}

type parser struct {
	path     string
	toks     []token
	i        int
	bad      map[int]bool // lines the lexer already reported as unreadable
	diags    []diag.Diagnostic
	trailing []Comment
}

// bailout unwinds the parse of one line-level item after a diagnostic that
// leaves the rest of the item unreadable; item recovers it.
type bailout struct{}

func (p *parser) add(code string, pos Pos, message, fix string) {
	p.diags = append(p.diags, diag.Diagnostic{
		Code:     code,
		Subject:  p.path,
		Position: diag.Position{Path: p.path, Line: pos.Line, Column: pos.Column},
		Message:  message,
		Fix:      fix,
	})
}

// report records a diagnostic of the grammar, unless the lexer already reported its line.
func (p *parser) report(code string, pos Pos, message, fix string) {
	if !p.bad[pos.Line] {
		p.add(code, pos, message, fix)
	}
}

func (p *parser) fail(code string, pos Pos, message, fix string) {
	p.report(code, pos, message, fix)
	panic(bailout{})
}

func (p *parser) tok() token { return p.toks[p.i] }

// peekAt looks n tokens ahead; it is only used past a word, and the tokens end with EOF.
func (p *parser) peekAt(n int) token { return p.toks[p.i+n] }

func (p *parser) advance() token {
	t := p.toks[p.i]
	if t.kind != tokEOF {
		p.i++
	}
	return t
}

func (p *parser) atWord(text string) bool {
	t := p.tok()
	return t.kind == tokWord && t.text == text
}

// unexpected fails at the current token: want says what belongs there.
func (p *parser) unexpected(want string) {
	t := p.tok()
	switch t.kind {
	case tokNL:
		p.fail("syntax.unexpected-token", t.pos, fmt.Sprintf("这一行在这里提前结束，这里应当是 %s", want), "在这里补上 "+want)
	case tokEOF:
		p.fail("syntax.unexpected-token", t.pos, fmt.Sprintf("文件在这里意外结束，这里应当是 %s", want), "在这里补上 "+want)
	}
	p.fail("syntax.unexpected-token", t.pos, fmt.Sprintf("这里应当是 %s，却是 %s", want, t.describe()), "把它改成 "+want)
}

func (p *parser) expect(kind tokenKind, want string) token {
	if p.tok().kind != kind {
		p.unexpected(want)
	}
	return p.advance()
}

// item parses one line-level item. After a bailout it skips the rest of the
// item, including any block the item opened, so the next item starts clean.
func (p *parser) item(parse func()) {
	start := p.i
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bailout); !ok {
				panic(r)
			}
			p.skipItem(start)
		}
	}()
	parse()
}

// skipItem moves past the item that starts at token start: to the end of the
// line where the brackets the item opened are closed again. A bracket left open
// in the middle of a line, rather than at its end, is the error itself and is
// not waited for.
func (p *parser) skipItem(start int) {
	depth := 0
	for i := start; ; i++ {
		net := 0
		for ; p.toks[i].kind != tokNL && p.toks[i].kind != tokEOF; i++ {
			switch {
			case isOpener(p.toks[i].kind):
				net++
			case isCloser(p.toks[i].kind):
				net--
			}
		}
		if net < 0 || i > start && isOpener(p.toks[i-1].kind) {
			depth = max(0, depth+net)
		}
		if p.toks[i].kind == tokEOF {
			p.i = i
			return
		}
		if depth == 0 && i >= p.i {
			p.i = i + 1
			return
		}
	}
}

func isOpener(k tokenKind) bool { return k == tokLBrace || k == tokLBracket || k == tokLParen }
func isCloser(k tokenKind) bool { return k == tokRBrace || k == tokRBracket || k == tokRParen }

// comments skips blank lines and collects the comment lines before the next item.
func (p *parser) comments() []Comment {
	var cs []Comment
	for {
		switch t := p.tok(); t.kind {
		case tokNL:
			p.advance()
		case tokComment:
			cs = append(cs, Comment{Pos: t.pos, Text: t.text})
			p.advance()
		default:
			return cs
		}
	}
}

// endLine ends a line-level item. A `}` here is reported, since it must start
// its own line, and left for the block to close.
func (p *parser) endLine() {
	switch t := p.tok(); t.kind {
	case tokNL:
		p.advance()
	case tokEOF:
	case tokRBrace:
		p.report("syntax.brace-position", t.pos, "} 必须写在行首", "把 } 移到下一行的行首")
	default:
		p.fail("syntax.unexpected-token", t.pos, fmt.Sprintf("这一行在 %s 处多出了内容", t.describe()), "删去多余的内容，或把它写到下一行")
	}
}

// openBlock reads the `{` of a block, which must end its line.
func (p *parser) openBlock() token {
	open := p.expect(tokLBrace, "{")
	if t := p.tok(); t.kind == tokNL {
		p.advance()
	} else if t.kind != tokEOF {
		p.report("syntax.brace-position", t.pos, "{ 必须写在行尾", "把 { 之后的内容移到下一行")
	}
	return open
}

// block parses members until the `}` that closes the block opened by open,
// consumes the `}`, and returns the comments before it.
func (p *parser) block(open token, member func(comments []Comment)) []Comment {
	for {
		comments := p.comments()
		switch t := p.tok(); t.kind {
		case tokRBrace:
			p.advance()
			return comments
		case tokEOF:
			p.report("syntax.unclosed-block", open.pos, "这个 { 开始的块没有闭合", "在块的末尾补上独占一行的 }")
			return comments
		}
		p.item(func() { member(comments) })
	}
}

// topLevel parses the declarations of a file.
func (p *parser) topLevel(decl func(comments []Comment)) {
	for {
		comments := p.comments()
		if p.tok().kind == tokEOF {
			p.trailing = comments
			return
		}
		p.item(func() { decl(comments) })
	}
}

// sequence checks that the declarations or members of one level come in the
// order of their ranks, and that a single one is not repeated. orderCode is
// the code of an order violation.
type sequence struct {
	what      string
	orderCode string
	order     []string
	ranks     map[string]int
	single    map[string]bool
	last      string
	seen      map[string]bool
}

func newSequence(what string, order []string, single ...string) *sequence {
	s := &sequence{what: what, orderCode: "syntax.member-order", order: order, ranks: map[string]int{}, single: map[string]bool{}, seen: map[string]bool{}}
	for i, kw := range order {
		for _, k := range strings.Split(kw, "|") {
			s.ranks[k] = i
		}
	}
	for _, k := range single {
		s.single[k] = true
	}
	return s
}

// keywords lists the keywords in their order, for messages.
func (s *sequence) keywords() string {
	return strings.ReplaceAll(strings.Join(s.order, "、"), "|", "、")
}

func (s *sequence) has(kw string) bool {
	_, ok := s.ranks[kw]
	return ok
}

// next records kw at pos and reports when it is out of order or repeated.
func (p *parser) next(s *sequence, kw string, pos Pos) {
	switch {
	case s.single[kw] && s.seen[kw]:
		p.report("syntax.duplicate-member", pos, fmt.Sprintf("%s中只能有一个 %s", s.what, kw), "删去重复的 "+kw)
	case s.last != "" && s.ranks[kw] < s.ranks[s.last]:
		p.report(s.orderCode, pos, fmt.Sprintf("%s中的 %s 必须写在 %s 之前", s.what, kw, s.last),
			fmt.Sprintf("把这个 %s 移到所有 %s 之前", kw, s.last))
	}
	s.seen[kw] = true
	if s.last == "" || s.ranks[kw] > s.ranks[s.last] {
		s.last = kw
	}
}

// nameClass is one of the three ways of writing a name (docs/format.md §2).
type nameClass int

const (
	kebab nameClass = iota
	upper
	lower
)

var classPatterns = [...]*regexp.Regexp{
	kebab: regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`),
	upper: regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`),
	lower: regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`),
}

var classNames = [...]string{
	kebab: "短横线写法（如 order-line）",
	upper: "大驼峰写法（如 OrderLine）",
	lower: "小驼峰写法（如 orderLine）",
}

func (c nameClass) match(s string) bool { return classPatterns[c].MatchString(s) }

func anyClass(s string) bool { return kebab.match(s) || upper.match(s) || lower.match(s) }

// name reads a name of class c; what says what it names.
func (p *parser) name(c nameClass, what string) string {
	t := p.expect(tokWord, what)
	if !c.match(t.text) {
		p.report("syntax.invalid-name", t.pos, fmt.Sprintf("%s %s 必须用%s", what, t.text, classNames[c]), "把它改成"+classNames[c])
	}
	return t.text
}

// oneOf reads a word that must be one of options; code names the diagnostic.
func (p *parser) oneOf(code, what string, options ...string) string {
	t := p.expect(tokWord, what)
	for _, o := range options {
		if t.text == o {
			return t.text
		}
	}
	p.report(code, t.pos, fmt.Sprintf("%s只能是 %s，不能是 %s", what, strings.Join(options, "、"), t.text),
		"改成 "+strings.Join(options, "、")+" 之一")
	return t.text
}

// dotted reads `word { "." word }`.
func (p *parser) dotted(what string) Ref {
	first := p.expect(tokWord, what)
	r := Ref{Pos: first.pos, Segments: []string{first.text}}
	for p.tok().kind == tokDot {
		p.advance()
		r.Segments = append(r.Segments, p.expect(tokWord, "引用的下一段名称").text)
	}
	return r
}

// ref reads a reference whose every segment is a name of some class.
func (p *parser) ref(what string) Ref {
	r := p.dotted(what)
	if i := slices.IndexFunc(r.Segments, func(s string) bool { return !anyClass(s) }); i >= 0 {
		p.report("syntax.invalid-name", r.Pos, fmt.Sprintf("引用 %s 中的 %s 不是合法的名称", strings.Join(r.Segments, "."), r.Segments[i]),
			"名称用短横线、大驼峰或小驼峰写法")
	}
	return r
}

// refForm reads a reference and checks it against form: the classes of its
// last segment, of an optional type segment before it, and kebab for the rest.
func (p *parser) refForm(what, form string, last []nameClass, typeBeforeLast bool) Ref {
	r := p.dotted(what)
	segs := r.Segments
	ok := false
	for _, c := range last {
		ok = ok || c.match(segs[len(segs)-1])
	}
	rest := segs[:len(segs)-1]
	if typeBeforeLast && len(rest) > 0 && upper.match(rest[len(rest)-1]) {
		rest = rest[:len(rest)-1]
	}
	for _, s := range rest {
		ok = ok && kebab.match(s)
	}
	if !ok {
		p.report("syntax.invalid-reference", r.Pos, fmt.Sprintf("%s %s 不合写法：应当写作 %s", what, strings.Join(segs, "."), form),
			"按 "+form+" 改写这个引用")
	}
	return r
}

func (p *parser) typeRef() Ref {
	return p.refForm("类型引用", "[组件.]类型，例如 Order、inventory.Stock", []nameClass{upper}, false)
}

func (p *parser) opRef() Ref {
	return p.refForm("操作引用", "[组件.][类型.]操作，例如 parse、Order.total、inventory.Stock.reserve", []nameClass{lower}, true)
}

func (p *parser) participant() Ref {
	return p.refForm("发起方", "参与者、组件或 [组件.]类型，例如 customer、payment、payment.Payments", []nameClass{kebab, upper}, false)
}

func (p *parser) componentPath() []string {
	return p.refForm("组件路径", "以 . 连接的短横线名称，例如 order、lang.golang", []nameClass{kebab}, false).Segments
}

func (p *parser) str(what string) string {
	return p.expect(tokString, what).text
}

// summary reads a required summary; what names its owner.
func (p *parser) summary(what string) string {
	t := p.tok()
	if t.kind != tokString {
		p.report("syntax.missing-summary", t.pos, what+"必须有摘要", "在名称和形式之后写上摘要字符串")
		return ""
	}
	return p.optSummary()
}

// optSummary reads a summary if one follows.
func (p *parser) optSummary() string {
	t := p.tok()
	if t.kind != tokString {
		return ""
	}
	p.advance()
	first, _ := utf8.DecodeRuneInString(t.text)
	last, _ := utf8.DecodeLastRuneInString(t.text)
	switch {
	case t.text == "":
		p.report("syntax.empty-summary", t.pos, "摘要不能为空", "写一句话说明它")
	case strings.ContainsAny(t.text, "\n\t"):
		p.report("syntax.summary-tab-or-newline", t.pos, "摘要不能含换行或制表符", "把摘要写成一行，去掉其中的 \\n 和 \\t")
	case unicode.IsSpace(first) || unicode.IsSpace(last):
		p.report("syntax.summary-surrounding-space", t.pos, "摘要的首尾不能有空白", "删去摘要首尾的空白")
	}
	return t.text
}

// list reads `item { "," item }`.
func (p *parser) list(item func()) {
	item()
	for p.tok().kind == tokComma {
		p.advance()
		item()
	}
}

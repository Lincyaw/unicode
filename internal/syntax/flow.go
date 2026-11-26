package syntax

import (
	"fmt"
	"strings"
)

func (p *parser) flowDecl(comments []Comment) FlowDecl {
	d := FlowDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(kebab, "流程名")
	d.Summary = p.summary("流程")
	open := p.openBlock()
	members := newSequence("流程", []string{"forall", "where", "步骤", "case"})
	d.Trailing = p.block(open, func(comments []Comment) {
		kw := p.tok()
		if kw.kind == tokWord && !p.looksLikeStep() {
			switch kw.text {
			case "forall":
				p.next(members, kw.text, kw.pos)
				d.Symbols = append(d.Symbols, p.symbol(comments))
				return
			case "where":
				p.next(members, kw.text, kw.pos)
				d.Limits = append(d.Limits, p.limit(comments))
				return
			case "case":
				p.next(members, kw.text, kw.pos)
				d.Cases = append(d.Cases, p.caseDecl(comments))
				return
			}
		}
		p.next(members, "步骤", kw.pos)
		p.step(&d.Steps, comments)
	})
	p.endLine()
	return d
}

func (p *parser) propertyDecl(comments []Comment) PropertyDecl {
	d := PropertyDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(kebab, "性质名")
	d.Form = p.oneOf("syntax.unknown-form", "性质的形式", propertyForms...)
	d.Summary = p.summary("性质")
	open := p.openBlock()
	members := newSequence("性质", []string{"about", "forall", "where", "case"}, "about")
	d.Trailing = p.block(open, func(comments []Comment) {
		kw := p.tok()
		if kw.kind != tokWord || !members.has(kw.text) {
			p.fail("syntax.unknown-member", kw.pos, "性质中只能有 about、forall、where 和 case", "改用其中一个关键字开头，或删去这一行")
		}
		p.next(members, kw.text, kw.pos)
		switch kw.text {
		case "about":
			d.About = About{Comments: comments, Pos: p.advance().pos}
			p.list(func() { d.About.Refs = append(d.About.Refs, p.ref("性质约束的对象")) })
			p.endLine()
		case "forall":
			d.Symbols = append(d.Symbols, p.symbol(comments))
		case "where":
			d.Limits = append(d.Limits, p.limit(comments))
		case "case":
			d.Cases = append(d.Cases, p.caseDecl(comments))
		}
	})
	p.endLine()
	if !members.seen["about"] {
		p.report("syntax.missing-member", d.Pos, "性质必须写 about", "在性质的块中第一行写上 about 和它约束的对象")
	}
	return d
}

func (p *parser) symbol(comments []Comment) Symbol {
	s := Symbol{Comments: comments, Pos: p.advance().pos}
	s.Name = p.name(lower, "符号名")
	p.expect(tokColon, ":")
	s.Type = p.typeExpr()
	if p.atWord("generated") {
		p.advance()
		s.Generated = true
		s.Domain = p.str("说明取值域的字符串")
	}
	p.endLine()
	return s
}

func (p *parser) limit(comments []Comment) Limit {
	l := Limit{Comments: comments, Pos: p.advance().pos}
	l.Path = append(l.Path, p.name(lower, "符号名"))
	for p.tok().kind == tokDot {
		p.advance()
		l.Path = append(l.Path, p.name(lower, "符号路径的一段"))
	}
	switch {
	case p.atWord("in") && p.peekAt(1).kind == tokLBrace:
		p.advance()
		p.advance()
		l.Kind = LimitMembers
		p.list(func() { l.Members = append(l.Members, p.name(lower, "枚举值")) })
		p.expect(tokRBrace, "}")
	case p.atWord("in"):
		p.advance()
		l.Kind = LimitRange
		l.Low, l.High = p.interval(false)
	case p.atWord("size"):
		p.advance()
		l.Kind = LimitSize
		l.Low, l.High = p.interval(true)
	default:
		p.unexpected("in 或 size")
	}
	p.endLine()
	return l
}

// interval reads `Number ".." Number`; integers says the bounds are Int.
func (p *parser) interval(integers bool) (string, string) {
	bound := func() string {
		t := p.expect(tokNumber, "数")
		if integers && strings.Contains(t.text, ".") {
			p.report("syntax.not-an-integer", t.pos, fmt.Sprintf("大小的界限必须是整数，不能是 %s", t.text), "把界限改成整数")
		}
		return t.text
	}
	low := bound()
	p.expect(tokDotDot, "..")
	return low, bound()
}

func (p *parser) caseDecl(comments []Comment) CaseDecl {
	d := CaseDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(kebab, "用例名")
	d.Summary = p.optSummary()
	open := p.openBlock()
	d.Trailing = p.block(open, func(comments []Comment) { d.Assignments = append(d.Assignments, p.assignment(comments)) })
	p.endLine()
	return d
}

func (p *parser) assignment(comments []Comment) Assignment {
	if p.tok().kind != tokWord {
		p.fail("syntax.unknown-member", p.tok().pos, "用例中的每一行都写作 符号 = 值", "把这一行改成 符号 = 值，或删去它")
	}
	a := Assignment{Comments: comments, Pos: p.tok().pos}
	a.Symbol = p.name(lower, "符号名")
	p.expect(tokEq, "=")
	a.Value = p.value()
	p.endLine()
	return a
}

func (p *parser) corpusFile() *CorpusFile {
	cf := &CorpusFile{}
	h := newHeader("corpus", "录制文件", "corpus", "case")
	p.topLevel(func(comments []Comment) {
		kw := p.tok()
		if kw.kind != tokWord || !h.groups.has(kw.text) {
			p.unknownDecl(h)
		}
		p.check(h, kw)
		if kw.text == "corpus" {
			cf.Corpus = CorpusDecl{Comments: comments, Pos: p.advance().pos}
			cf.Corpus.Target = p.ref("性质或流程的标识")
			p.endLine()
			return
		}
		cf.Cases = append(cf.Cases, p.recordedCase(comments))
	})
	if !h.seen {
		p.report("syntax.missing-header", Pos{Line: 1, Column: 1}, "录制文件必须以 corpus 声明开头", "在文件开头写上 corpus 和性质或流程的标识")
	}
	return cf
}

func (p *parser) recordedCase(comments []Comment) RecordedCase {
	d := RecordedCase{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(kebab, "用例名")
	d.Reason = p.oneOf("syntax.unknown-reason", "录制的原因", "coverage", "counterexample")
	open := p.openBlock()
	d.Trailing = p.block(open, func(comments []Comment) { d.Assignments = append(d.Assignments, p.assignment(comments)) })
	p.endLine()
	return d
}

// looksLikeStep tells whether a line that starts with a word is a message
// step: its first word is followed by `->`, `->>`, `.`, or `emits` and a type
// reference. A keyword that starts another line kind never is.
func (p *parser) looksLikeStep() bool {
	second := p.peekAt(1)
	switch second.kind {
	case tokArrow, tokSendArrow, tokDot:
		return true
	case tokWord:
		third := p.peekAt(2)
		return second.text == "emits" && third.kind == tokWord && (upper.match(third.text) || p.peekAt(3).kind == tokDot)
	}
	return false
}

// step parses one step line into steps; an `on` block joins the call before it.
func (p *parser) step(steps *[]Step, comments []Comment) {
	kw := p.tok()
	if kw.kind == tokWord && !p.looksLikeStep() {
		switch kw.text {
		case "on":
			on := p.onBlock(comments)
			if n := len(*steps); n > 0 {
				if call, ok := (*steps)[n-1].(*CallStep); ok {
					call.Handlers = append(call.Handlers, on)
					return
				}
			}
			p.report("syntax.on-without-call", kw.pos, "on 只能紧跟在一个调用（->）之后", "把这个 on 块移到它处理的那个调用之后")
			return
		case "async":
			s := &AsyncStep{Comments: comments, Pos: p.advance().pos}
			s.Op = p.opRef()
			s.Steps, s.Trailing = p.stepBlock()
			p.endLine()
			*steps = append(*steps, s)
			return
		case "alt":
			*steps = append(*steps, p.altStep(comments))
			return
		case "opt":
			s := &OptStep{Comments: comments, Pos: p.advance().pos}
			s.Label = p.str("片段的标签字符串")
			s.Steps, s.Trailing = p.stepBlock()
			p.endLine()
			*steps = append(*steps, s)
			return
		case "loop":
			s := &LoopStep{Comments: comments, Pos: p.advance().pos}
			s.Label = p.str("片段的标签字符串")
			s.Steps, s.Trailing = p.stepBlock()
			p.endLine()
			*steps = append(*steps, s)
			return
		case "else":
			p.fail("syntax.misplaced-else", kw.pos, "else 必须紧跟在 alt 分支的 } 之后", "写作 } else \"标签\" {")
		}
	}
	if kw.kind != tokWord {
		p.fail("syntax.unknown-member", kw.pos,
			"这一行应当是步骤：发起方 -> 操作(实参)、发起方 ->> 操作(实参)、发起方 emits 事件(实参)，或 on、async、alt、opt、loop 块",
			"按步骤的写法改写这一行，或删去它")
	}
	*steps = append(*steps, p.messageStep(comments))
}

func (p *parser) messageStep(comments []Comment) Step {
	pos := p.tok().pos
	from := p.participant()
	switch t := p.tok(); {
	case t.kind == tokArrow:
		p.advance()
		s := &CallStep{Comments: comments, Pos: pos, From: from, Op: p.opRef()}
		s.Args = p.args()
		if p.tok().kind == tokLBrace {
			s.Steps, s.Trailing = p.stepBlock()
		}
		p.endLine()
		return s
	case t.kind == tokSendArrow:
		p.advance()
		s := &SendStep{Comments: comments, Pos: pos, From: from, Op: p.opRef()}
		s.Args = p.args()
		p.endLine()
		return s
	case t.kind == tokWord && t.text == "emits":
		p.advance()
		s := &EmitStep{Comments: comments, Pos: pos, From: from, Event: p.typeRef()}
		s.Args = p.args()
		p.endLine()
		return s
	}
	p.unexpected("->、->> 或 emits")
	return nil
}

func (p *parser) onBlock(comments []Comment) OnBlock {
	b := OnBlock{Comments: comments, Pos: p.advance().pos}
	b.Error = p.typeRef()
	b.Steps, b.Trailing = p.stepBlock()
	p.endLine()
	return b
}

func (p *parser) altStep(comments []Comment) *AltStep {
	s := &AltStep{Comments: comments, Pos: p.tok().pos}
	br := Branch{Pos: p.advance().pos}
	for {
		br.Label = p.str("分支的标签字符串")
		br.Steps, br.Trailing = p.stepBlock()
		s.Branches = append(s.Branches, br)
		if !p.atWord("else") {
			break
		}
		br = Branch{Pos: p.advance().pos}
	}
	p.endLine()
	return s
}

// stepBlock reads `{ NL { Step } }`.
func (p *parser) stepBlock() ([]Step, []Comment) {
	var steps []Step
	open := p.openBlock()
	trailing := p.block(open, func(comments []Comment) { p.step(&steps, comments) })
	return steps, trailing
}

func (p *parser) args() []string {
	var args []string
	p.expect(tokLParen, "(")
	if p.tok().kind != tokRParen {
		p.list(func() { args = append(args, p.name(lower, "实参")) })
	}
	p.expect(tokRParen, ")")
	return args
}

// valueLayout skips the line breaks inside a bracketed value; a comment there is reported.
func (p *parser) valueLayout(open token) {
	for {
		switch t := p.tok(); t.kind {
		case tokNL:
			p.advance()
		case tokComment:
			p.report("syntax.comment-in-value", t.pos, "值的内部不能写注释", "把注释移到这个用例的赋值之上")
			p.advance()
		case tokEOF:
			p.fail("syntax.unclosed-bracket", open.pos, fmt.Sprintf("值的 %s 没有闭合", open.text), "补上与它配对的括号")
		default:
			return
		}
	}
}

// items reads the comma-separated items of a bracketed value up to closer;
// a trailing comma is allowed.
func (p *parser) items(open token, closer tokenKind, item func()) {
	for {
		p.valueLayout(open)
		if p.tok().kind == closer {
			p.advance()
			return
		}
		item()
		p.valueLayout(open)
		switch p.tok().kind {
		case tokComma:
			p.advance()
		case closer:
			p.advance()
			return
		default:
			p.unexpected(", 或 " + tokenNames[closer])
		}
	}
}

func (p *parser) value() Value {
	t := p.tok()
	switch t.kind {
	case tokNumber:
		p.advance()
		return &NumberValue{Pos: t.pos, Text: t.text}
	case tokString:
		p.advance()
		return &StringValue{Pos: t.pos, Text: t.text}
	case tokLBracket:
		l := &ListValue{Pos: t.pos}
		p.items(p.advance(), tokRBracket, func() { l.Items = append(l.Items, p.value()) })
		return l
	case tokLBrace:
		b := &BraceValue{Pos: t.pos}
		p.items(p.advance(), tokRBrace, func() {
			e := Entry{Pos: p.tok().pos, Key: p.entryKey()}
			p.expect(tokColon, ":")
			e.Value = p.value()
			b.Entries = append(b.Entries, e)
		})
		return b
	case tokWord:
		if p.peekAt(1).kind == tokLParen {
			c := &CallValue{Pos: t.pos, Op: p.name(lower, "操作名")}
			p.items(p.advance(), tokRParen, func() {
				a := Arg{Pos: p.tok().pos, Name: p.name(lower, "参数名")}
				p.expect(tokColon, ":")
				a.Value = p.value()
				c.Args = append(c.Args, a)
			})
			return c
		}
		switch t.text {
		case "true", "false":
			p.advance()
			return &BoolValue{Pos: t.pos, Value: t.text == "true"}
		case "none":
			p.advance()
			return &NoneValue{Pos: t.pos}
		}
		return &NameValue{Pos: t.pos, Name: p.name(lower, "枚举值")}
	}
	p.unexpected("值")
	return nil
}

func (p *parser) entryKey() Value {
	t := p.tok()
	switch t.kind {
	case tokWord:
		return &NameValue{Pos: t.pos, Name: p.name(lower, "字段名")}
	case tokString:
		p.advance()
		return &StringValue{Pos: t.pos, Text: t.text}
	case tokNumber:
		p.advance()
		return &NumberValue{Pos: t.pos, Text: t.text}
	}
	p.unexpected("字段名、字符串或数")
	return nil
}

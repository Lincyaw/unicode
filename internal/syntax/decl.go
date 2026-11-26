package syntax

import "fmt"

var (
	typeForms     = []string{"object", "record", "interface", "enum", "error", "event"}
	propertyForms = []string{"invariant", "postcondition", "relation", "transition"}
	builtins      = map[string]bool{"bool": true, "int": true, "float": true, "string": true, "bytes": true, "time": true, "duration": true}
)

// header tracks the declaration that must open a file and the order of the
// declaration groups after it.
type header struct {
	keyword string
	seen    bool
	groups  *sequence
}

func newHeader(keyword, what string, groups ...string) *header {
	s := newSequence(what, groups)
	s.orderCode = "syntax.declaration-order"
	return &header{keyword: keyword, groups: s}
}

// check reports a declaration that comes before the file's opening one, a
// second opening one, or one out of group order.
func (p *parser) check(h *header, kw token) {
	switch {
	case kw.text == h.keyword && h.seen:
		p.report("syntax.duplicate-header", kw.pos, fmt.Sprintf("文件中只能有一个 %s 声明", h.keyword), "删去这个 "+h.keyword+" 声明")
	case kw.text != h.keyword && !h.seen:
		p.report("syntax.missing-header", kw.pos, fmt.Sprintf("文件必须以 %s 声明开头", h.keyword), "在文件开头写上 "+h.keyword+" 声明")
	}
	h.seen = true
	p.next(h.groups, kw.text, kw.pos)
}

// unknownDecl fails at a line that starts no declaration of the file.
func (p *parser) unknownDecl(h *header) {
	t := p.tok()
	if t.kind == tokRBrace {
		p.fail("syntax.unexpected-token", t.pos, "这里多了一个 }", "删去它，或检查前面的块是否多写了 }")
	}
	p.fail("syntax.unknown-declaration", t.pos, fmt.Sprintf("这一行不是%s中的声明；声明以 %s 开头", h.groups.what, h.groups.keywords()),
		"改用其中一个关键字开头，或删去这一行")
}

func (p *parser) systemFile() *SystemFile {
	sf := &SystemFile{}
	h := newHeader("system", "系统文件", "system", "notation", "language", "actor")
	p.topLevel(func(comments []Comment) {
		kw := p.tok()
		if kw.kind != tokWord || !h.groups.has(kw.text) {
			p.unknownDecl(h)
		}
		p.check(h, kw)
		switch kw.text {
		case "system":
			sf.System = p.systemDecl(comments)
		case "notation":
			sf.Notations = append(sf.Notations, p.notationDecl(comments))
		case "language":
			sf.Languages = append(sf.Languages, p.languageDecl(comments))
		case "actor":
			sf.Actors = append(sf.Actors, p.actorDecl(comments))
		}
	})
	if !h.seen {
		p.report("syntax.missing-header", Pos{Line: 1, Column: 1}, "系统文件必须以 system 声明开头", "在文件开头写上 system 声明")
	}
	return sf
}

func (p *parser) systemDecl(comments []Comment) SystemDecl {
	d := SystemDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(kebab, "系统名")
	d.Summary = p.summary("系统")
	p.endLine()
	return d
}

func (p *parser) notationDecl(comments []Comment) NotationDecl {
	d := NotationDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.oneOf("syntax.unknown-notation", "记法", "ddd", "c4")
	p.endLine()
	return d
}

func (p *parser) languageDecl(comments []Comment) LanguageDecl {
	d := LanguageDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.oneOf("syntax.unknown-language", "语言", "go", "python")
	open := p.openBlock()
	d.Trailing = p.block(open, func(comments []Comment) {
		if !p.atWord("root") {
			p.fail("syntax.unknown-member", p.tok().pos, "language 块中只能有 root", "把这一行改成 root \"源码根目录\"，或删去它")
		}
		r := Root{Comments: comments, Pos: p.advance().pos}
		r.Path = p.str("源码根目录的字符串")
		p.endLine()
		d.Roots = append(d.Roots, r)
	})
	p.endLine()
	if len(d.Roots) == 0 {
		p.report("syntax.missing-member", d.Pos, "language 块中至少要有一个 root", "在块中写上 root \"源码根目录\"")
	}
	return d
}

func (p *parser) actorDecl(comments []Comment) ActorDecl {
	d := ActorDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(kebab, "参与者名")
	formTok := p.tok()
	d.Form = p.oneOf("syntax.unknown-form", "参与者的形式", "person", "external")
	d.Summary = p.summary("参与者")
	if p.tok().kind == tokLBrace {
		if d.Form == "person" {
			p.report("syntax.block-not-allowed", formTok.pos, "人（person）不能带块：只有外部系统（external）有类型和操作", "删去这个块，或把形式改成 external")
		}
		open := p.openBlock()
		members := newSequence("外部系统", []string{"type", "op"})
		d.Trailing = p.block(open, func(comments []Comment) {
			kw := p.tok()
			if kw.kind != tokWord || !members.has(kw.text) {
				p.fail("syntax.unknown-member", kw.pos, "外部系统中只能有 type 和 op", "改用 type 或 op 开头，或删去这一行")
			}
			p.next(members, kw.text, kw.pos)
			if kw.text == "type" {
				d.Types = append(d.Types, p.typeDecl(comments))
			} else {
				d.Ops = append(d.Ops, p.opDecl(comments))
			}
		})
	}
	p.endLine()
	return d
}

func (p *parser) componentFile() *ComponentFile {
	cf := &ComponentFile{}
	h := newHeader("component", "组件文件", "component", "type", "op", "flow", "property")
	p.topLevel(func(comments []Comment) {
		kw := p.tok()
		if kw.kind != tokWord || !h.groups.has(kw.text) {
			p.unknownDecl(h)
		}
		p.check(h, kw)
		switch kw.text {
		case "component":
			cf.Component = p.componentDecl(comments)
		case "type":
			cf.Types = append(cf.Types, p.typeDecl(comments))
		case "op":
			cf.Ops = append(cf.Ops, p.opDecl(comments))
		case "flow":
			cf.Flows = append(cf.Flows, p.flowDecl(comments))
		case "property":
			cf.Properties = append(cf.Properties, p.propertyDecl(comments))
		}
	})
	if !h.seen {
		p.report("syntax.missing-header", Pos{Line: 1, Column: 1}, "组件文件必须以 component 声明开头", "在文件开头写上 component 声明")
	}
	return cf
}

func (p *parser) componentDecl(comments []Comment) ComponentDecl {
	d := ComponentDecl{Comments: comments, Pos: p.advance().pos}
	d.Path = p.componentPath()
	d.Summary = p.summary("组件")
	open := p.openBlock()
	members := newSequence("组件块", []string{"label", "focus", "paths", "uses"}, "focus", "paths", "uses")
	d.Trailing = p.block(open, func(comments []Comment) {
		kw := p.tok()
		if kw.kind != tokWord || !members.has(kw.text) {
			p.fail("syntax.unknown-member", kw.pos, "组件块中只能有 label、focus、paths 和 uses", "改用其中一个关键字开头，或删去这一行")
		}
		p.next(members, kw.text, kw.pos)
		switch kw.text {
		case "label":
			d.Labels = append(d.Labels, p.label(comments))
		case "focus":
			f := Focus{Comments: comments, Pos: p.advance().pos}
			f.Level = p.oneOf("syntax.unknown-focus", "关注度", "core", "support", "utility")
			p.endLine()
			d.Focus = &f
		case "paths":
			d.Paths = Paths{Comments: comments, Pos: p.advance().pos}
			p.list(func() { d.Paths.Patterns = append(d.Paths.Patterns, p.str("路径模式的字符串")) })
			p.endLine()
		case "uses":
			u := Uses{Comments: comments, Pos: p.advance().pos}
			p.list(func() { u.Refs = append(u.Refs, p.ref("依赖的组件或外部系统")) })
			p.endLine()
			d.Uses = &u
		}
	})
	p.endLine()
	if !members.seen["paths"] {
		p.report("syntax.missing-member", d.Pos, "组件块中必须有 paths", "在组件块中写上 paths \"路径模式\"")
	}
	return d
}

func (p *parser) label(comments []Comment) Label {
	l := Label{Comments: comments, Pos: p.advance().pos}
	l.Name = p.name(kebab, "标签")
	p.endLine()
	return l
}

func (p *parser) typeDecl(comments []Comment) TypeDecl {
	d := TypeDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(upper, "类型名")
	d.Form = p.oneOf("syntax.unknown-form", "类型的形式", typeForms...)
	d.Summary = p.summary("类型")
	if p.tok().kind == tokLBrace {
		open := p.openBlock()
		members := newSequence("类型", []string{"label", "extends", "implements", "field|value", "op"})
		d.Trailing = p.block(open, func(comments []Comment) { p.typeMember(&d, members, comments) })
	}
	p.endLine()
	return d
}

func (p *parser) typeMember(d *TypeDecl, members *sequence, comments []Comment) {
	kw := p.tok()
	if kw.kind != tokWord || !members.has(kw.text) {
		p.fail("syntax.unknown-member", kw.pos, "类型中只能有 label、extends、implements、field、value 和 op", "改用其中一个关键字开头，或删去这一行")
	}
	p.next(members, kw.text, kw.pos)
	switch kw.text {
	case "label":
		d.Labels = append(d.Labels, p.label(comments))
	case "extends", "implements":
		s := Supertype{Comments: comments, Pos: p.advance().pos}
		s.Type = p.typeRef()
		p.endLine()
		if kw.text == "extends" {
			d.Extends = append(d.Extends, s)
		} else {
			d.Implements = append(d.Implements, s)
		}
	case "field":
		if d.Form == "enum" || d.Form == "interface" {
			p.report("syntax.field-not-allowed", kw.pos, fmt.Sprintf("%s 类型不能有 field", d.Form), "删去这个 field，或改变类型的形式")
		}
		f := Field{Comments: comments, Pos: p.advance().pos}
		f.Name = p.name(lower, "字段名")
		p.expect(tokColon, ":")
		f.Type = p.typeExpr()
		p.endLine()
		d.Fields = append(d.Fields, f)
	case "value":
		if d.Form != "enum" {
			p.report("syntax.value-outside-enum", kw.pos, "只有 enum 类型才能有 value", "删去这个 value，或把类型的形式改成 enum")
		}
		v := EnumValue{Comments: comments, Pos: p.advance().pos}
		v.Name = p.name(lower, "枚举值")
		p.endLine()
		d.Values = append(d.Values, v)
	case "op":
		d.Ops = append(d.Ops, p.opDecl(comments))
	}
}

func (p *parser) opDecl(comments []Comment) OpDecl {
	d := OpDecl{Comments: comments, Pos: p.advance().pos}
	d.Name = p.name(lower, "操作名")
	p.expect(tokLParen, "(")
	if p.tok().kind != tokRParen {
		p.list(func() {
			pr := Param{Pos: p.tok().pos}
			pr.Name = p.name(lower, "参数名")
			p.expect(tokColon, ":")
			pr.Type = p.typeExpr()
			d.Params = append(d.Params, pr)
		})
	}
	p.expect(tokRParen, ")")
	if p.tok().kind == tokArrow {
		p.advance()
		d.Result = p.typeExpr()
	}
	if p.atWord("raises") {
		p.advance()
		p.list(func() { d.Raises = append(d.Raises, p.typeRef()) })
	}
	if p.atWord("handles") {
		p.advance()
		h := p.typeRef()
		d.Handles = &h
	}
	d.Summary = p.optSummary()
	p.endLine()
	return d
}

func (p *parser) typeExpr() TypeExpr {
	t := p.tok()
	if t.kind != tokWord {
		p.unexpected("类型")
	}
	if p.peekAt(1).kind != tokDot {
		switch {
		case builtins[t.text]:
			p.advance()
			return &BuiltinType{Pos: t.pos, Name: t.text}
		case t.text == "list" || t.text == "set" || t.text == "optional":
			p.advance()
			p.expect(tokLAngle, "<")
			e := &ElemType{Pos: t.pos, Kind: t.text, Elem: p.typeExpr()}
			p.expect(tokRAngle, ">")
			return e
		case t.text == "map":
			p.advance()
			p.expect(tokLAngle, "<")
			m := &MapType{Pos: t.pos, Key: p.typeExpr()}
			p.expect(tokComma, ",")
			m.Value = p.typeExpr()
			p.expect(tokRAngle, ">")
			return m
		case t.text == "call":
			p.advance()
			p.expect(tokLAngle, "<")
			c := &CallType{Pos: t.pos}
			p.list(func() { c.Targets = append(c.Targets, p.ref("call 的操作或类型")) })
			p.expect(tokRAngle, ">")
			return c
		case !upper.match(t.text):
			p.report("syntax.invalid-type", t.pos,
				fmt.Sprintf("%s 不是内置类型；内置类型只有 bool、int、float、string、bytes、time、duration，类型引用以大驼峰的类型名结尾", t.text),
				"改成内置类型、list<…>、set<…>、optional<…>、map<…, …>、call<…>，或一个类型引用")
			p.advance()
			return &NamedType{Pos: t.pos, Ref: Ref{Pos: t.pos, Segments: []string{t.text}}}
		}
	}
	return &NamedType{Pos: t.pos, Ref: p.typeRef()}
}

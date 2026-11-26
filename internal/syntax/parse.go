package syntax

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"

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
// A file that does not parse gets one diagnostic, the first problem found; a
// file that parses gets one for each rule of check.go it breaks.
//
//uc:realizes syntax.parse
func Parse(path string, kind FileKind, src []byte) Parsed {
	at := locator{path: path, src: src}
	if diags := forbidden(at); len(diags) > 0 {
		return Parsed{Diagnostics: diags}
	}
	text := src
	if !bytes.HasSuffix(text, []byte("\n")) {
		// The grammar ends every line with NL; a missing final newline is only layout.
		text = append(bytes.Clone(src), '\n')
	}
	f, err := ucParser.ParseBytes(path, text)
	if err != nil {
		return Parsed{Diagnostics: []diag.Diagnostic{parseError(at, err)}}
	}
	f.Path, f.Kind = path, kind
	if diags := check(at, f); len(diags) > 0 {
		return Parsed{Diagnostics: diags}
	}
	return Parsed{File: f}
}

// locator turns byte offsets of one file into diagnostics.
type locator struct {
	path string
	src  []byte
}

func (l locator) diagnostic(offset int, code, message, fix string) diag.Diagnostic {
	offset = min(max(offset, 0), len(l.src))
	lineStart := bytes.LastIndexByte(l.src[:offset], '\n') + 1
	return diag.Diagnostic{
		Code:    code,
		Subject: l.path,
		Position: diag.Position{
			Path:   l.path,
			Line:   bytes.Count(l.src[:offset], []byte{'\n'}) + 1,
			Column: offset - lineStart + 1,
		},
		Message: message,
		Fix:     fix,
	}
}

// parseError turns what participle returned into a diagnostic.
func parseError(at locator, err error) diag.Diagnostic {
	var te *tokenError
	if errors.As(err, &te) {
		return at.diagnostic(te.offset, te.code, te.message, te.fix)
	}
	var lexErr *lexer.Error
	if errors.As(err, &lexErr) {
		off := lexErr.Pos.Offset
		if off < len(at.src) && at.src[off] == '"' {
			return at.diagnostic(off, "syntax.unterminated-string", "字符串在行尾之前没有结束", "在字符串末尾补上 \"；字符串中的换行写作 \\n")
		}
		r, _ := utf8.DecodeRune(at.src[min(off, len(at.src)):])
		return at.diagnostic(off, "syntax.unexpected-character", fmt.Sprintf("字符 %q 不能出现在字符串和注释之外", r),
			"删去这个字符，或把它写进字符串或注释中")
	}
	var unexpected *participle.UnexpectedTokenError
	if errors.As(err, &unexpected) {
		return at.unexpected(unexpected.Unexpected)
	}
	// Participle's other errors (a capture that fails, too many repetitions)
	// cannot arise from this grammar; one is still reported where it stands.
	offset := 0
	var perr participle.Error
	if errors.As(err, &perr) {
		offset = perr.Position().Offset
	}
	return at.diagnostic(offset, "syntax.unexpected-token", "这里不合 docs/format.md §3 的语法", "按 docs/format.md §3 改写这一行")
}

// unexpected describes in Chinese a token the grammar did not expect. The
// construct of its line, named by the line's first keyword, says what was
// expected there and where the construct belongs.
func (l locator) unexpected(t lexer.Token) diag.Diagnostic {
	off := t.Pos.Offset
	if t.EOF() {
		return l.diagnostic(off, "syntax.unexpected-end", "文件在这里意外结束：最后一行没有写完，或者有块缺少 }",
			"补全最后一行，并为每个 { 补上独占一行的 }")
	}
	line := l.line(off)
	key := line.construct()
	first := len(line) > 0 && line[0].Pos.Offset == off
	value := key == "assignment" || key == "literal" || key == "where"
	switch {
	case t.Type == symbols["Comment"] && first:
		return l.diagnostic(off, "syntax.comment-in-value", "值的内部不能写注释", "把注释移到这个赋值之上")
	case t.Type == symbols["Comment"]:
		return l.diagnostic(off, "syntax.trailing-comment", "注释必须独占一行", "把注释移到它所说明的那一行之上")
	case key == "else":
		return l.diagnostic(line[0].Pos.Offset, "syntax.misplaced-else", "else 必须与前一个分支的 } 写在同一行",
			`把这一行接到上一行的 } 之后，写作 } else "标签" {`)
	case t.Value == "}" && !first && !value:
		return l.diagnostic(off, "syntax.brace-position", "} 必须写在行首", "把 } 移到下一行的行首")
	case l.after(off) == '{' && !value:
		return l.diagnostic(off, "syntax.brace-position", "{ 必须写在行尾", "把 { 之后的内容移到下一行")
	case key == "close" && first:
		return l.diagnostic(off, "syntax.unexpected-token", "这个 } 没有可以结束的块：它之前有没有写完的值，或者它是多余的",
			"补全它之前的值，让每个 [、( 和 { 都有配对的括号，或删去这个 }")
	}
	c := constructs[key]
	switch {
	case first && key == "unknown":
		return l.diagnostic(off, "syntax.unexpected-token", fmt.Sprintf("这一行开头的 %s 不是关键字", describe(t)),
			"以关键字开头改写这一行，例如 type、op、flow、property 或 case；流程中的步骤写作 发起方 -> 操作(实参)")
	case first && key != "literal":
		return l.diagnostic(off, "syntax.unexpected-token", c.name+"不能写在这里", c.name+"只能写在"+c.place)
	}
	return l.diagnostic(off, "syntax.unexpected-token", fmt.Sprintf("%s中不应当出现%s", c.name, describe(t)), c.name+"写作 "+c.form)
}

// construct is one kind of line of docs/format.md §3: its name, how it is
// written, and where it may stand.
type construct struct{ name, form, place string }

var constructs = map[string]construct{
	"system":     {"系统声明", `system 名称 "摘要"`, "系统文件的开头"},
	"notation":   {"记法声明", "notation ddd 或 notation c4", "系统文件的顶层"},
	"language":   {"语言声明", "language go { 或 language python {，块中每行一个 root", "系统文件的顶层"},
	"root":       {"源码根目录", `root "目录"`, "language 块中"},
	"actor":      {"参与者声明", `actor 名称 person "摘要"，或 actor 名称 external "摘要"，后者可以带 { 类型和操作 }`, "系统文件的顶层"},
	"component":  {"组件声明", `component 路径 "摘要" {`, "组件文件的开头"},
	"label":      {"标签", "label 名称", "组件块或类型的块中"},
	"focus":      {"关注度", "focus core、focus support 或 focus utility", "组件块中"},
	"paths":      {"路径模式", `paths "模式", …`, "组件块中"},
	"uses":       {"依赖", "uses 组件或外部系统, …", "组件块中"},
	"type":       {"类型声明", `type 名称 形式 "摘要"，可以带 {，形式是 object、record、interface、enum、error 或 event`, "组件文件的顶层或外部系统的块中"},
	"extends":    {"继承", "extends 类型", "类型的块中"},
	"implements": {"实现", "implements 类型", "类型的块中"},
	"field":      {"字段", "field 名称: 类型", "类型的块中"},
	"value":      {"枚举值", "value 名称", "enum 类型的块中"},
	"literal":    {"值", "数、字符串、true、false、none、枚举值、[元素, …]、{键: 值, …} 或 操作名(参数: 值, …)；值只能在开括号之后、逗号前后和闭括号之前换行", "用例的赋值中"},
	"op":         {"操作声明", `op 名称(参数: 类型, …)，之后依次可以有 -> 类型、raises 类型, …、handles 类型 和 "摘要"`, "组件文件的顶层、类型的块中或外部系统的块中"},
	"flow":       {"流程声明", `flow 名称 "摘要" {`, "组件文件的顶层"},
	"property":   {"性质声明", `property 名称 形式 "摘要" {，形式是 invariant、postcondition、relation 或 transition`, "组件文件的顶层"},
	"about":      {"约束的对象", "about 引用, …", "性质的块中"},
	"forall":     {"符号", `forall 名称: 类型，生成的符号之后再写 generated "取值域的说明"`, "流程或性质的块中"},
	"where":      {"限定", "where 符号路径 in 数..数、where 符号路径 in {枚举值, …} 或 where 符号路径 size 整数..整数", "流程或性质的块中"},
	"case":       {"用例", `case 名称 "摘要" {，摘要可以省略；录制的用例写作 case 名称 coverage { 或 case 名称 counterexample {`, "流程或性质的块中，或录制文件的顶层"},
	"corpus":     {"录制文件的声明", "corpus 性质或流程的标识", "录制文件的开头"},
	"on":         {"错误处理", "on 错误类型 {，紧跟在调用之后", "流程的步骤中"},
	"async":      {"异步片段", "async 操作 {", "流程的步骤中"},
	"alt":        {"分支片段", `alt "标签" {，其余的分支写作 } else "标签" {`, "流程的步骤中"},
	"opt":        {"可选片段", `opt "标签" {`, "流程的步骤中"},
	"loop":       {"循环片段", `loop "标签" {`, "流程的步骤中"},
	step:         {"步骤", "发起方 -> 操作(实参, …)，可以带 {；发起方 ->> 操作(实参, …)；或 发起方 emits 事件(实参, …)", "流程的步骤中"},
	"assignment": {"赋值", "符号 = 值", "用例的块中"},
	"unknown":    {"这一行", "关键字开头的声明或成员；流程中的步骤写作 发起方 -> 操作(实参)；用例中的赋值写作 符号 = 值", "块中"},
}

// lineTokens are the tokens of one line, without spaces.
type lineTokens []lexer.Token

// line lexes the line that holds offset; its tokens keep their offsets in the file.
func (l locator) line(offset int) lineTokens {
	start := bytes.LastIndexByte(l.src[:min(offset, len(l.src))], '\n') + 1
	end := bytes.IndexByte(l.src[start:], '\n')
	if end < 0 {
		end = len(l.src) - start
	}
	lex, err := ucLexer.LexString("", string(l.src[start:start+end]))
	if err != nil {
		return nil
	}
	var toks lineTokens
	for {
		t, err := lex.Next()
		if err != nil || t.EOF() {
			return toks
		}
		if t.Type != symbols["Whitespace"] {
			t.Pos.Offset += start
			toks = append(toks, t)
		}
	}
}

// construct names the kind of the line: its first keyword, a misplaced else,
// a step, an assignment, a closing brace, part of a broken value, or unknown.
// A first word followed by ->, ->>, . or emits starts a step, as in
// docs/format.md §3.
func (ts lineTokens) construct() string {
	if len(ts) == 0 {
		return "unknown"
	}
	first := ts[0]
	startsStep := len(ts) > 1 && (ts[1].Type == symbols["Arrow"] || ts[1].Type == symbols["SendArrow"] ||
		ts[1].Value == "." || ts[1].Value == "emits")
	_, keyword := constructs[first.Value]
	word := first.Type == symbols["Ident"]
	switch {
	case word && first.Value == "else" && !startsStep:
		return "else"
	case word && keyword && !startsStep:
		return first.Value
	case startsStep:
		return step
	case slices.ContainsFunc(ts, func(t lexer.Token) bool { return t.Value == "=" }):
		return "assignment"
	case first.Value == "}":
		return "close"
	case !word || len(ts) == 1 || ts[1].Value == ":" || ts[1].Value == "," || ts[1].Value == "(":
		return "literal"
	}
	return "unknown"
}

// after returns the last character before offset that is not a space.
func (l locator) after(offset int) byte {
	before := bytes.TrimRight(l.src[:min(offset, len(l.src))], " ")
	if len(before) == 0 {
		return 0
	}
	return before[len(before)-1]
}

// tokenKinds are the Chinese names of the lexer's kinds of token.
var tokenKinds = map[string]string{
	"Comment": "注释", "String": "字符串", "Number": "数", "Ident": "名称", "SendArrow": "箭头", "Arrow": "箭头",
	"DotDot": "区间符", "Punct": "标点",
}

// describe names a token in Chinese: its kind and its text.
func describe(t lexer.Token) string {
	if t.Type == symbols["NL"] {
		return "换行"
	}
	for name, typ := range symbols {
		if typ == t.Type {
			return tokenKinds[name] + " " + t.Value
		}
	}
	return t.Value
}

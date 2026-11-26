package syntax

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"
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
	if d, ok := forbidden(at); ok {
		return Parsed{Diagnostics: []diag.Diagnostic{d}}
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

// forbidden finds the first character that no file may contain.
func forbidden(at locator) (diag.Diagnostic, bool) {
	for off := 0; off < len(at.src); {
		r, size := utf8.DecodeRune(at.src[off:])
		switch {
		case r == utf8.RuneError && size <= 1:
			return at.diagnostic(off, "syntax.invalid-utf8", "文件在这里不是合法的 UTF-8 编码", "把文件保存为 UTF-8 文本"), true
		case r == '\t':
			return at.diagnostic(off, "syntax.tab", "文件中不能有制表符", "把制表符换成空格；字符串中的制表符写作 \\t"), true
		case r == '\r':
			return at.diagnostic(off, "syntax.carriage-return", "文件中不能有回车符，换行只用 LF", "把 CRLF 换行改成 LF"), true
		case r != '\n' && unicode.IsControl(r):
			return at.diagnostic(off, "syntax.control-character", fmt.Sprintf("文件中不能有控制字符 %U", r), "删去这个控制字符"), true
		}
		off += size
	}
	return diag.Diagnostic{}, false
}

// tokenError is a problem in one token, found while mapping it.
type tokenError struct {
	offset             int
	code, message, fix string
}

func (e *tokenError) Error() string { return e.message }

var escapes = map[byte]byte{'"': '"', '\\': '\\', 'n': '\n', 't': '\t'}

// checkEscapes reports an escape in a string token other than the four of docs/format.md §2.
func checkEscapes(t lexer.Token) (lexer.Token, error) {
	for i := 1; i < len(t.Value)-1; i++ {
		if t.Value[i] != '\\' {
			continue
		}
		if _, ok := escapes[t.Value[i+1]]; !ok {
			return t, &tokenError{t.Pos.Offset + i, "syntax.invalid-escape",
				"字符串中只有四种转义：\\\"、\\\\、\\n 和 \\t", "把反斜杠写作 \\\\，或改用这四种转义之一"}
		}
		i++
	}
	return t, nil
}

// unquoted resolves the escapes of a string token that checkEscapes accepted.
func unquoted(raw string) string {
	var b strings.Builder
	for i := 1; i < len(raw)-1; i++ {
		if raw[i] == '\\' {
			i++
			b.WriteByte(escapes[raw[i]])
			continue
		}
		b.WriteByte(raw[i])
	}
	return b.String()
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
		t := unexpected.Unexpected
		expected := ""
		if _, rest, ok := strings.Cut(unexpected.Message(), "(expected "); ok {
			expected = "；应当是 " + strings.TrimSuffix(rest, ")")
		}
		switch {
		case t.EOF():
			return at.diagnostic(t.Pos.Offset, "syntax.unexpected-end", "文件在这里意外结束"+expected, "补全最后一个声明，并检查每个 { 都有配对的 }")
		case t.Type == symbols["Comment"] && at.startsLine(t.Pos.Offset):
			return at.diagnostic(t.Pos.Offset, "syntax.comment-in-value", "值的内部不能写注释", "把注释移到这个赋值之上")
		case t.Type == symbols["Comment"]:
			return at.diagnostic(t.Pos.Offset, "syntax.trailing-comment", "注释必须独占一行", "把注释移到它所说明的那一行之上")
		}
		return at.diagnostic(t.Pos.Offset, "syntax.unexpected-token", fmt.Sprintf("这里不应当是 %s%s", describe(t), expected),
			"按 docs/format.md §3 的语法改写这一行")
	}
	// Participle's other errors (a capture that fails, too many repetitions)
	// cannot arise from this grammar; one is still reported where it stands.
	var perr participle.Error
	errors.As(err, &perr)
	return at.diagnostic(perr.Position().Offset, "syntax.unexpected-token", "这一行不合语法："+err.Error(),
		"按 docs/format.md §3 的语法改写这一行")
}

var symbols = ucLexer.Symbols()

// startsLine tells whether only spaces stand before offset on its line.
func (l locator) startsLine(offset int) bool {
	lineStart := bytes.LastIndexByte(l.src[:offset], '\n') + 1
	return len(bytes.Trim(l.src[lineStart:offset], " ")) == 0
}

func describe(t lexer.Token) string {
	switch t.Type {
	case symbols["NL"]:
		return "换行"
	case symbols["String"]:
		return "字符串 " + t.Value
	}
	return fmt.Sprintf("%q", t.Value)
}

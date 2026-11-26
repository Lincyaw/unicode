package syntax

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/diag"
)

// The lexis of docs/format.md §2: the tokens, the characters no file may
// contain, the escapes of strings and the canonical form of numbers.

// ucLexer keeps strings and comments as written, quotes and `//` included, so
// that no string or comment can stand for a keyword or a punctuation of the
// grammar: participle matches a literal by the token's text.
var ucLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `//[^\n]*`},
	{Name: "String", Pattern: `"(\\.|[^"\\\n])*"`},
	{Name: "Number", Pattern: `-?[0-9]+(\.[0-9]+)?`},
	{Name: "Ident", Pattern: `[A-Za-z][A-Za-z0-9]*(-[A-Za-z0-9]+)*`},
	{Name: "SendArrow", Pattern: `->>`},
	{Name: "Arrow", Pattern: `->`},
	{Name: "DotDot", Pattern: `\.\.`},
	{Name: "Punct", Pattern: `[{}()\[\]<>,:.=]`},
	{Name: "NL", Pattern: `\n([ ]*\n)*`},
	{Name: "Whitespace", Pattern: `[ ]+`},
})

var symbols = ucLexer.Symbols()

// forbidden reports the characters that no file may contain; a run of the
// same problem counts once.
func forbidden(at locator) []diag.Diagnostic {
	var diags []diag.Diagnostic
	last := ""
	for off := 0; off < len(at.src); {
		r, size := utf8.DecodeRune(at.src[off:])
		code, message, fix := "", "", ""
		switch {
		case r == utf8.RuneError && size <= 1:
			code, message, fix = "syntax.invalid-utf8", "文件在这里不是合法的 UTF-8 编码", "把文件保存为 UTF-8 文本"
		case r == '\t':
			code, message, fix = "syntax.tab", "文件中不能有制表符", "把制表符换成空格；字符串中的制表符写作 \\t"
		case r == '\r':
			code, message, fix = "syntax.carriage-return", "文件中不能有回车符，换行只用 LF", "把 CRLF 换行改成 LF"
		case r != '\n' && unicode.IsControl(r):
			code, message, fix = "syntax.control-character", fmt.Sprintf("文件中不能有控制字符 %U", r), "删去这个控制字符"
		}
		if code != "" && code != last {
			diags = append(diags, at.diagnostic(off, code, message, fix))
		}
		last = code
		off += size
	}
	return diags
}

// escapes are the four escapes of strings: the letter after `\` and the character it stands for.
var escapes = []struct{ letter, char byte }{{'"', '"'}, {'\\', '\\'}, {'n', '\n'}, {'t', '\t'}}

// unescape maps the letter of an escape to its character; quoter writes the characters back as escapes.
var (
	unescape = func() map[byte]byte {
		m := map[byte]byte{}
		for _, e := range escapes {
			m[e.letter] = e.char
		}
		return m
	}()
	quoter = func() *strings.Replacer {
		var pairs []string
		for _, e := range escapes {
			pairs = append(pairs, string(e.char), `\`+string(e.letter))
		}
		return strings.NewReplacer(pairs...)
	}()
)

// quote writes s as a string literal.
func quote(s string) string { return `"` + quoter.Replace(s) + `"` }

// tokenError is a problem in one token, found while mapping it.
type tokenError struct {
	offset             int
	code, message, fix string
}

func (e *tokenError) Error() string { return e.message }

// checkEscapes reports an escape in a string token that is not one of the four.
func checkEscapes(t lexer.Token) (lexer.Token, error) {
	for i := 1; i < len(t.Value)-1; i++ {
		if t.Value[i] != '\\' {
			continue
		}
		if _, ok := unescape[t.Value[i+1]]; !ok {
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
			b.WriteByte(unescape[raw[i]])
			continue
		}
		b.WriteByte(raw[i])
	}
	return b.String()
}

// commentText is what follows `//` on a comment line, without trailing spaces.
func commentText(raw string) string {
	return strings.TrimRight(strings.TrimPrefix(raw, "//"), " ")
}

// canonicalNumber writes a number without leading zeros in its integer part,
// without -0, and without trailing zeros in its fraction beyond the first digit.
func canonicalNumber(s string) string {
	sign, digits := "", s
	if strings.HasPrefix(s, "-") {
		sign, digits = "-", s[1:]
	}
	whole, fraction, decimal := strings.Cut(digits, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	if decimal {
		fraction = strings.TrimRight(fraction, "0")
		if fraction == "" {
			fraction = "0"
		}
		whole += "." + fraction
	}
	if strings.Trim(whole, "0.") == "" {
		sign = ""
	}
	return sign + whole
}

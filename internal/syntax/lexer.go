package syntax

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNL
	tokComment
	tokWord
	tokNumber
	tokString
	tokLBrace
	tokRBrace
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokLAngle
	tokRAngle
	tokComma
	tokColon
	tokDot
	tokDotDot
	tokEq
	tokArrow
	tokSendArrow
	tokIllegal
)

// tokenNames are the Chinese descriptions used in diagnostics.
var tokenNames = map[tokenKind]string{
	tokEOF:       "文件结尾",
	tokNL:        "换行",
	tokComment:   "注释",
	tokWord:      "名称",
	tokNumber:    "数",
	tokString:    "字符串",
	tokLBrace:    "{",
	tokRBrace:    "}",
	tokLParen:    "(",
	tokRParen:    ")",
	tokLBracket:  "[",
	tokRBracket:  "]",
	tokLAngle:    "<",
	tokRAngle:    ">",
	tokComma:     ",",
	tokColon:     ":",
	tokDot:       ".",
	tokDotDot:    "..",
	tokEq:        "=",
	tokArrow:     "->",
	tokSendArrow: "->>",
	tokIllegal:   "非法字符",
}

var punctuation = map[byte]tokenKind{
	'{': tokLBrace,
	'}': tokRBrace,
	'(': tokLParen,
	')': tokRParen,
	'[': tokLBracket,
	']': tokRBracket,
	'<': tokLAngle,
	'>': tokRAngle,
	',': tokComma,
	':': tokColon,
	'=': tokEq,
}

// token is one lexeme. Text is the word or number as written, the resolved
// string, or the comment text after `//`.
type token struct {
	kind tokenKind
	text string
	pos  Pos
}

func (t token) describe() string {
	switch t.kind {
	case tokWord, tokNumber:
		return fmt.Sprintf("%s %s", tokenNames[t.kind], t.text)
	case tokString:
		return "字符串 " + quote(t.text)
	}
	return tokenNames[t.kind]
}

// lexer turns a file into tokens. Space and LF are the only layout characters;
// it reports every other problem and keeps going, so that one bad character
// costs one diagnostic.
type lexer struct {
	src      []byte
	off      int
	line     int
	lineOff  int
	tokens   []token
	lineToks int // tokens emitted on the current line
	report   func(code string, pos Pos, message, fix string)
	badLines map[int]bool
}

func lex(src []byte, report func(code string, pos Pos, message, fix string)) ([]token, map[int]bool) {
	lx := &lexer{src: src, line: 1, report: report, badLines: map[int]bool{}}
	lx.run()
	return lx.tokens, lx.badLines
}

func (lx *lexer) pos() Pos {
	return Pos{Line: lx.line, Column: lx.off - lx.lineOff + 1}
}

func (lx *lexer) emit(kind tokenKind, text string, pos Pos) {
	lx.tokens = append(lx.tokens, token{kind: kind, text: text, pos: pos})
	lx.lineToks++
}

func (lx *lexer) bad(code string, pos Pos, message, fix string) {
	lx.badLines[pos.Line] = true
	lx.report(code, pos, message, fix)
}

func (lx *lexer) newline() {
	lx.emit(tokNL, "", lx.pos())
	lx.off++
	lx.line++
	lx.lineOff = lx.off
	lx.lineToks = 0
}

func (lx *lexer) run() {
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		switch {
		case c == '\n':
			lx.newline()
		case c == ' ':
			lx.off++
		case c == '/' && lx.peek(1) == '/':
			lx.comment()
		case c == '"':
			lx.string()
		case isLetter(c):
			lx.word()
		case isDigit(c) || c == '-' && isDigit(lx.peek(1)):
			lx.number()
		case c == '-' && lx.peek(1) == '>':
			pos := lx.pos()
			if lx.peek(2) == '>' {
				lx.off += 3
				lx.emit(tokSendArrow, "->>", pos)
			} else {
				lx.off += 2
				lx.emit(tokArrow, "->", pos)
			}
		case c == '.':
			pos := lx.pos()
			if lx.peek(1) == '.' {
				lx.off += 2
				lx.emit(tokDotDot, "..", pos)
			} else {
				lx.off++
				lx.emit(tokDot, ".", pos)
			}
		case punctuation[c] != 0:
			lx.emit(punctuation[c], string(c), lx.pos())
			lx.off++
		default:
			lx.layoutOrIllegal()
		}
	}
	lx.emit(tokEOF, "", lx.pos())
}

func (lx *lexer) peek(n int) byte {
	if lx.off+n < len(lx.src) {
		return lx.src[lx.off+n]
	}
	return 0
}

// layoutOrIllegal handles a character that starts no token: a forbidden
// character is reported and read as a space; anything else is a run of
// illegal characters.
func (lx *lexer) layoutOrIllegal() {
	pos := lx.pos()
	if size, found := lx.skipForbidden(lx.off); found {
		lx.off += size
		return
	}
	start := lx.off
	_, size := utf8.DecodeRune(lx.src[lx.off:])
	lx.off += size
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if c == ' ' || c == '\n' || startsToken(lx.src, lx.off) {
			break
		}
		r, size := utf8.DecodeRune(lx.src[lx.off:])
		if r == utf8.RuneError && size <= 1 || r == '\t' || r == '\r' || isControl(r) {
			break
		}
		lx.off += size
	}
	lx.emit(tokIllegal, string(lx.src[start:lx.off]), pos)
	lx.bad("syntax.unexpected-character", pos,
		fmt.Sprintf("字符 %q 不能出现在字符串和注释之外", string(lx.src[start:lx.off])),
		"删去这些字符，或把它们写进字符串或注释中")
}

// skipForbidden reports the character at off if no file may contain it, and
// returns its size; a run of bytes that are not UTF-8 counts as one character.
func (lx *lexer) skipForbidden(off int) (int, bool) {
	pos := Pos{Line: lx.line, Column: off - lx.lineOff + 1}
	r, size := utf8.DecodeRune(lx.src[off:])
	switch {
	case r == utf8.RuneError && size <= 1:
		end := off + 1
		for end < len(lx.src) {
			if r, size := utf8.DecodeRune(lx.src[end:]); r != utf8.RuneError || size > 1 {
				break
			}
			end++
		}
		lx.bad("syntax.invalid-utf8", pos, "文件在这里不是合法的 UTF-8 编码", "把文件保存为 UTF-8 文本")
		return end - off, true
	case r == '\t':
		lx.report("syntax.tab", pos, "文件中不能有制表符", "把制表符换成空格；字符串中的制表符写作 \\t")
	case r == '\r':
		lx.report("syntax.carriage-return", pos, "文件中不能有回车符，换行只用 LF", "把 CRLF 换行改成 LF")
	case isControl(r):
		lx.report("syntax.control-character", pos, fmt.Sprintf("文件中不能有控制字符 %U", r), "删去这个控制字符")
	default:
		return size, false
	}
	return size, true
}

func isControl(r rune) bool {
	return r != '\n' && unicode.IsControl(r)
}

func startsToken(src []byte, off int) bool {
	c := src[off]
	return c == '"' || c == '/' || c == '.' || c == '-' || isLetter(c) || isDigit(c) || punctuation[c] != 0
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }

func (lx *lexer) comment() {
	pos := lx.pos()
	end := lx.off
	for end < len(lx.src) && lx.src[end] != '\n' {
		end++
	}
	var text strings.Builder
	for off := lx.off + 2; off < end; {
		size, found := lx.skipForbidden(off)
		if !found {
			text.Write(lx.src[off : off+size])
		}
		off += size
	}
	if lx.lineToks > 0 {
		lx.report("syntax.trailing-comment", pos, "注释必须独占一行", "把注释移到它所说明的那一行之上")
	} else {
		lx.emit(tokComment, strings.TrimRight(text.String(), " "), pos)
	}
	lx.off = end
}

func (lx *lexer) word() {
	pos := lx.pos()
	start := lx.off
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if isLetter(c) || isDigit(c) || c == '-' && (isLetter(lx.peek(1)) || isDigit(lx.peek(1))) {
			lx.off++
			continue
		}
		break
	}
	lx.emit(tokWord, string(lx.src[start:lx.off]), pos)
}

func (lx *lexer) number() {
	pos := lx.pos()
	start := lx.off
	if lx.src[lx.off] == '-' {
		lx.off++
	}
	lx.digits()
	if lx.off < len(lx.src) && lx.src[lx.off] == '.' && isDigit(lx.peek(1)) {
		lx.off++
		lx.digits()
	}
	lx.emit(tokNumber, string(lx.src[start:lx.off]), pos)
}

func (lx *lexer) digits() {
	for lx.off < len(lx.src) && isDigit(lx.src[lx.off]) {
		lx.off++
	}
}

func (lx *lexer) string() {
	pos := lx.pos()
	lx.off++
	var b strings.Builder
	for {
		if lx.off >= len(lx.src) || lx.src[lx.off] == '\n' {
			lx.bad("syntax.unterminated-string", pos, "字符串在行尾之前没有结束", "在字符串末尾补上 \"；字符串中的换行写作 \\n")
			lx.emit(tokString, b.String(), pos)
			return
		}
		c := lx.src[lx.off]
		switch c {
		case '"':
			lx.off++
			lx.emit(tokString, b.String(), pos)
			return
		case '\\':
			lx.escape(&b)
			continue
		}
		size, found := lx.skipForbidden(lx.off)
		if !found {
			b.Write(lx.src[lx.off : lx.off+size])
		}
		lx.off += size
	}
}

var escapes = map[byte]byte{'"': '"', '\\': '\\', 'n': '\n', 't': '\t'}

func (lx *lexer) escape(b *strings.Builder) {
	pos := lx.pos()
	if c, ok := escapes[lx.peek(1)]; ok {
		b.WriteByte(c)
		lx.off += 2
		return
	}
	lx.report("syntax.invalid-escape", pos, "字符串中只有四种转义：\\\"、\\\\、\\n 和 \\t", "把反斜杠写作 \\\\，或改用这四种转义之一")
	lx.off++
}

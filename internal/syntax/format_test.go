package syntax

import (
	"bytes"
	"cmp"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/mattn/go-runewidth"
	"pgregory.net/rapid"
)

// Sample is a valid file: its kind and its text.
//
//uc:realizes syntax.Sample
type Sample struct {
	Kind FileKind
	Text string
}

var samplePaths = map[FileKind]string{
	FileKindSystem:    "design/system.uc",
	FileKindComponent: "design/sample.uc",
	FileKindCorpus:    "corpus/sample.uc",
}

// genSample draws a random valid syntax tree and writes it with random legal layout.
//
//uc:generates syntax.format-is-stable.sample
func genSample() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample {
		if rapid.IntRange(0, 2).Draw(t, "expression") == 0 {
			return expressionSample(t)
		}
		f := genFile(t)
		w := &writer{t: t}
		w.file(f)
		return Sample{Kind: f.Kind, Text: w.b.String()}
	})
}

//uc:checks syntax.format-is-stable
func checkFormatIsStable(t rapid.TB, sample Sample) {
	path := samplePaths[sample.Kind]
	first := Parse(path, sample.Kind, []byte(sample.Text))
	if len(first.Diagnostics) > 0 {
		t.Fatalf("the valid text s got diagnostics:%s\ns:\n%s", describeDiagnostics(first), sample.Text)
	}
	printed := first.File.Print()
	if problem := cmp.Or(canonicalProblem(printed), spacingProblem(printed), numberProblem(printed), valueLayoutProblem(printed)); problem != "" {
		t.Fatalf("t, printed from s, is not canonical: %s\nt:\n%s", problem, printed)
	}
	second := Parse(path, sample.Kind, printed)
	if len(second.Diagnostics) > 0 {
		t.Fatalf("t, printed from s, got diagnostics:%s\nt:\n%s", describeDiagnostics(second), printed)
	}
	if where := treeDifference(reflect.ValueOf(first.File), reflect.ValueOf(second.File), "File"); where != "" {
		t.Fatalf("t parses into another tree than s, first at %s\ns:\n%s\nt:\n%s", where, sample.Text, printed)
	}
	if again := second.File.Print(); !bytes.Equal(again, printed) {
		t.Fatalf("t parsed and printed again is not t\nt:\n%s\nprinted again:\n%s", printed, again)
	}
}

func TestFormatIsStable(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFormatIsStable(t, genSample().Draw(t, "sample"))
	})
}

func FuzzFormatIsStable(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFormatIsStable(t, genSample().Draw(t, "sample"))
	}))
}

func describeDiagnostics(p Parsed) string {
	var b strings.Builder
	for _, d := range p.Diagnostics {
		fmt.Fprintf(&b, "\n  %d:%d: %s: %s", d.Position.Line, d.Position.Column, d.Code, d.Message)
	}
	return b.String()
}

var posType = reflect.TypeFor[lexer.Position]()

// treeDifference returns the path of the first place where two trees differ,
// ignoring positions and the difference between a nil and an empty slice.
func treeDifference(a, b reflect.Value, path string) string {
	if a.Type() != b.Type() {
		return path + " (its type)"
	}
	if a.Type() == posType {
		return ""
	}
	if a.Type() == reflect.TypeFor[expression]() {
		x, y := a.Interface().(expression), b.Interface().(expression)
		return treeDifference(reflect.ValueOf(x.Expr()), reflect.ValueOf(y.Expr()), path+".normalized")
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				return path
			}
			return ""
		}
		return treeDifference(a.Elem(), b.Elem(), path)
	case reflect.Struct:
		for i := range a.NumField() {
			if d := treeDifference(a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name); d != "" {
				return d
			}
		}
		return ""
	case reflect.Slice:
		if a.Len() != b.Len() {
			return path + " (its length)"
		}
		for i := range a.Len() {
			if d := treeDifference(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i)); d != "" {
				return d
			}
		}
		return ""
	}
	if !a.Equal(b) {
		return path
	}
	return ""
}

// canonicalProblem describes how text breaks the layout of docs/language.md §3,
// or returns "": no tab or CR, no trailing space, one final newline, each
// line indented two spaces per open block or bracket, no blank line inside a
// block, exactly one blank line between top-level items, and comments right
// above what follows them.
func canonicalProblem(text []byte) string {
	s := string(text)
	switch {
	case !strings.HasSuffix(s, "\n"):
		return "the file does not end with a newline"
	case strings.HasSuffix(s, "\n\n"):
		return "the file ends with a blank line"
	case strings.ContainsAny(s, "\t\r"):
		return "it contains a tab or a carriage return"
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	var depth int
	for i, l := range lines {
		n := i + 1
		if l == "" {
			switch {
			case i == 0 || lines[i-1] == "":
				return fmt.Sprintf("line %d: an extra blank line", n)
			case depth > 0:
				return fmt.Sprintf("line %d: a blank line inside a block or a value", n)
			case strings.HasPrefix(lines[i-1], "//"):
				return fmt.Sprintf("line %d: a blank line between a comment and what follows it", n)
			}
			continue
		}
		if strings.HasSuffix(l, " ") {
			return fmt.Sprintf("line %d: a trailing space", n)
		}
		body := strings.TrimLeft(l, " ")
		comment := strings.HasPrefix(body, "//")
		if !comment && strings.ContainsRune("}])", rune(body[0])) {
			if depth == 0 {
				return fmt.Sprintf("line %d: an extra closing bracket", n)
			}
			depth--
		}
		if indent := len(l) - len(body); indent != 2*depth {
			return fmt.Sprintf("line %d: an indentation of %d spaces instead of %d", n, indent, 2*depth)
		}
		startsItem := depth == 0 && !strings.ContainsRune("}])", rune(body[0])) && i > 0 && !strings.HasPrefix(lines[i-1], "//")
		if startsItem && lines[i-1] != "" {
			return fmt.Sprintf("line %d: a top-level declaration without exactly one blank line before it", n)
		}
		if !comment && strings.ContainsRune("{[(", rune(body[len(body)-1])) {
			depth++
		}
	}
	if depth > 0 {
		return "a block or a value is not closed"
	}
	return ""
}

// spacingProblem describes the first pair of tokens on one line of text whose
// gap breaks the spacing of docs/language.md §3, or returns "".
func spacingProblem(text []byte) string {
	lines := strings.Split(string(text), "\n")
	lex, err := ucLexer.LexString("", string(text))
	if err != nil {
		return "the printed text does not lex: " + err.Error()
	}
	var prev lexer.Token
	line := 1
	for {
		tok, err := lex.Next()
		if err != nil || tok.EOF() {
			return ""
		}
		switch tok.Type {
		case symbols["Whitespace"]:
			continue
		case symbols["NL"], symbols["Comment"]:
			prev = lexer.Token{}
			line = tok.Pos.Line + strings.Count(tok.Value, "\n")
			continue
		}
		if prev.Value != "" {
			want := wantGap(prev, tok)
			if line <= len(lines) && expressionLine(lines[line-1]) {
				want = expressionGap(prev, tok)
			}
			if gap := tok.Pos.Offset - prev.Pos.Offset - len(prev.Value); gap != want {
				return fmt.Sprintf("line %d: %d spaces between %s and %s instead of %d", line, gap, prev.Value, tok.Value, want)
			}
		}
		prev = tok
	}
}

func expressionLine(line string) bool {
	for _, prefix := range []string{"ensure ", "requires ", "call ", "changes "} {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
}

func expressionGap(a, b lexer.Token) int {
	if a.Value == "." || b.Value == "." {
		return 0
	}
	if b.Value == "[" {
		return 0
	}
	operators := []string{"==", "!=", "<", "<=", ">", ">=", "+", "-", "*", "/", "and", "or", "implies", "in"}
	if slices.Contains(operators, a.Value) || slices.Contains(operators, b.Value) {
		return 1
	}
	if b.Value == "(" && slices.Contains([]string{"not", "ensure", "requires", "call"}, a.Value) {
		return 1
	}
	return wantGap(a, b)
}

// wantGap is the number of spaces §3 puts between two tokens on one line: none
// before a comma, a colon, a closing bracket and around a dot or a range, none
// inside brackets and before an opening parenthesis or angle bracket, and one
// everywhere else, around ->, ->>, =, emits and after a comma or a colon included.
func wantGap(a, b lexer.Token) int {
	punct := func(t lexer.Token, values ...string) bool {
		return (t.Type == symbols["Punct"] || t.Type == symbols["DotDot"]) && slices.Contains(values, t.Value)
	}
	switch {
	case punct(b, ",", ":", ")", "]", ">", "}", ".", "..", "(", "<"):
		return 0
	case punct(a, "(", "[", "<", "{", ".", ".."):
		return 0
	}
	return 1
}

// canonicalNumber is the canonical form of a number (docs/language.md §2): no
// leading zeros in the integer part, no -0, and a fraction without trailing
// zeros beyond its first digit.
var canonicalNumberForm = regexp.MustCompile(`^(0(\.(0|[0-9]*[1-9]))?|-0\.[0-9]*[1-9]|-?[1-9][0-9]*(\.(0|[0-9]*[1-9]))?)$`)

// numberProblem describes the first number of text that is not in its canonical form, or returns "".
func numberProblem(text []byte) string {
	lex, err := ucLexer.LexString("", string(text))
	if err != nil {
		return "the printed text does not lex: " + err.Error()
	}
	for {
		tok, err := lex.Next()
		if err != nil || tok.EOF() {
			return ""
		}
		if tok.Type == symbols["Number"] && !canonicalNumberForm.MatchString(tok.Value) {
			return fmt.Sprintf("line %d: the number %s is not in its canonical form", tok.Pos.Line, tok.Value)
		}
	}
}

// valueColumns measures display width with its own instance of the printer's
// fixed condition: East Asian wide characters take two columns.
var valueColumns = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: true}

const valueWidth = 100

// valueLayoutProblem describes the first line of a value in canonical text
// that breaks docs/language.md §3 P6, or returns "": a line wider than 100
// columns that still holds a bracketed value with items, or a value broken
// across lines that would fit on one line at its indentation.
func valueLayoutProblem(text []byte) string {
	lines := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	for i, inValue := range valueLines(lines) {
		if !inValue {
			continue
		}
		toks := tokensOf(lines[i])
		if len(toks) == 0 {
			continue
		}
		if width := valueColumns.StringWidth(lines[i]); width > valueWidth && holdsItems(toks) {
			return fmt.Sprintf("line %d is %d columns wide, yet it holds a bracketed value with items, which should be broken", i+1, width)
		}
		if slices.Contains([]string{"[", "{", "("}, toks[len(toks)-1].Value) {
			if flat, width := flattened(lines, i); width <= valueWidth {
				return fmt.Sprintf("line %d starts a value broken across lines that fits in %d columns on one line: %s", i+1, width, flat)
			}
		}
	}
	return ""
}

// valueLines marks the lines that hold values: the lines of case blocks, comments aside.
func valueLines(lines []string) []bool {
	marks := make([]bool, len(lines))
	caseIndent := -1
	for i, l := range lines {
		body := strings.TrimLeft(l, " ")
		indent := len(l) - len(body)
		switch toks := tokensOf(body); {
		case caseIndent >= 0 && indent == caseIndent && body == "}":
			caseIndent = -1
		case caseIndent >= 0:
			marks[i] = !strings.HasPrefix(body, "//")
		case len(toks) >= 3 && toks[0].Value == "case" && toks[1].Type == symbols["Ident"] && toks[len(toks)-1].Value == "{":
			caseIndent = indent
		}
	}
	return marks
}

// tokensOf lexes one line, without spaces.
func tokensOf(line string) []lexer.Token {
	lex, err := ucLexer.LexString("", line)
	if err != nil {
		return nil
	}
	var toks []lexer.Token
	for {
		tok, err := lex.Next()
		if err != nil || tok.EOF() {
			return toks
		}
		if tok.Type != symbols["Whitespace"] {
			toks = append(toks, tok)
		}
	}
}

// holdsItems tells whether a bracket opens and closes on the line with something between.
func holdsItems(toks []lexer.Token) bool {
	var open []int
	for i, tok := range toks {
		switch tok.Value {
		case "[", "{", "(":
			open = append(open, i)
		case "]", "}", ")":
			if len(open) > 0 && open[len(open)-1] < i-1 {
				return true
			}
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	return false
}

// flattened writes the value that line i starts and the lines indented
// below it end on one line, as the printer would, and returns it with its
// width at the indentation of line i.
func flattened(lines []string, i int) (string, int) {
	indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
	flat := strings.TrimLeft(lines[i], " ")
	for _, l := range lines[i+1:] {
		body := strings.TrimLeft(l, " ")
		switch {
		case body == "":
		case strings.ContainsRune("[{(", rune(flat[len(flat)-1])):
			flat += body
		case strings.ContainsRune("]})", rune(body[0])):
			flat = strings.TrimSuffix(flat, ",") + body
		default:
			flat += " " + body
		}
		if len(l)-len(body) == indent {
			break
		}
	}
	return flat, indent + valueColumns.StringWidth(flat)
}

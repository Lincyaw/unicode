package syntax

import (
	"bytes"
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/participle/v2/lexer"
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
	if problem := cmp.Or(canonicalProblem(printed), spacingProblem(printed)); problem != "" {
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

// canonicalProblem describes how text breaks the layout of docs/format.md §3,
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
// gap breaks the spacing of docs/format.md §3, or returns "".
func spacingProblem(text []byte) string {
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
			if gap, want := tok.Pos.Offset-prev.Pos.Offset-len(prev.Value), wantGap(prev, tok); gap != want {
				return fmt.Sprintf("line %d: %d spaces between %s and %s instead of %d", line, gap, prev.Value, tok.Value, want)
			}
		}
		prev = tok
	}
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

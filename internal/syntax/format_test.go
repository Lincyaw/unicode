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
		t.Fatalf("合法的文本 s 解析出了诊断：%s\ns:\n%s", describeDiagnostics(first), sample.Text)
	}
	printed := first.File.Print()
	if problem := cmp.Or(canonicalProblem(printed), spacingProblem(printed)); problem != "" {
		t.Fatalf("s 解析后打印得到的 t 不是规范格式：%s\nt:\n%s", problem, printed)
	}
	second := Parse(path, sample.Kind, printed)
	if len(second.Diagnostics) > 0 {
		t.Fatalf("s 解析后打印得到的 t 解析出了诊断：%s\nt:\n%s", describeDiagnostics(second), printed)
	}
	if where := treeDifference(reflect.ValueOf(first.File), reflect.ValueOf(second.File), "File"); where != "" {
		t.Fatalf("t 解析出的语法树与 s 的不同，最先不同的是 %s\ns:\n%s\nt:\n%s", where, sample.Text, printed)
	}
	if again := second.File.Print(); !bytes.Equal(again, printed) {
		t.Fatalf("t 再解析再打印得到的不是 t\nt:\n%s\n再打印:\n%s", printed, again)
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
		return path + " 的类型"
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
			return path + " 的长度"
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
		return "文件末尾没有换行"
	case strings.HasSuffix(s, "\n\n"):
		return "文件末尾有空行"
	case strings.ContainsAny(s, "\t\r"):
		return "含有制表符或回车符"
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	var depth int
	for i, l := range lines {
		n := i + 1
		if l == "" {
			switch {
			case i == 0 || lines[i-1] == "":
				return fmt.Sprintf("第 %d 行：多余的空行", n)
			case depth > 0:
				return fmt.Sprintf("第 %d 行：块或值的内部有空行", n)
			case strings.HasPrefix(lines[i-1], "//"):
				return fmt.Sprintf("第 %d 行：注释与它之后的内容之间有空行", n)
			}
			continue
		}
		if strings.HasSuffix(l, " ") {
			return fmt.Sprintf("第 %d 行：行尾有空格", n)
		}
		body := strings.TrimLeft(l, " ")
		comment := strings.HasPrefix(body, "//")
		if !comment && strings.ContainsRune("}])", rune(body[0])) {
			if depth == 0 {
				return fmt.Sprintf("第 %d 行：多余的闭括号", n)
			}
			depth--
		}
		if indent := len(l) - len(body); indent != 2*depth {
			return fmt.Sprintf("第 %d 行：缩进是 %d 个空格，应当是 %d 个", n, indent, 2*depth)
		}
		startsItem := depth == 0 && !strings.ContainsRune("}])", rune(body[0])) && i > 0 && !strings.HasPrefix(lines[i-1], "//")
		if startsItem && lines[i-1] != "" {
			return fmt.Sprintf("第 %d 行：顶层的声明之前应当恰有一个空行", n)
		}
		if !comment && strings.ContainsRune("{[(", rune(body[len(body)-1])) {
			depth++
		}
	}
	if depth > 0 {
		return "块或值没有闭合"
	}
	return ""
}

// spacingProblem describes the first pair of tokens on one line of text whose
// gap breaks the spacing of docs/format.md §3, or returns "".
func spacingProblem(text []byte) string {
	lex, err := ucLexer.LexString("", string(text))
	if err != nil {
		return "打印的结果不能分成记号：" + err.Error()
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
				return fmt.Sprintf("第 %d 行：%s 与 %s 之间有 %d 个空格，应当是 %d 个", line, prev.Value, tok.Value, gap, want)
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

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lincyaw/unicode/internal/syntax"
	"pgregory.net/rapid"
)

// layout writes lines of tokens with random legal layout: indentation,
// extra spaces, trailing spaces, blank lines and final newlines.
func layout(t *rapid.T, lines [][]string) string {
	var b strings.Builder
	for _, toks := range lines {
		b.WriteString(strings.Repeat(" ", rapid.IntRange(0, 4).Draw(t, "indent")))
		for i, tok := range toks {
			if i > 0 {
				b.WriteString(strings.Repeat(" ", rapid.IntRange(1, 2).Draw(t, "sep")))
			}
			b.WriteString(tok)
		}
		b.WriteString(strings.Repeat(" ", rapid.IntRange(0, 1).Draw(t, "trailing")))
		b.WriteString(strings.Repeat("\n", rapid.IntRange(1, 3).Draw(t, "newlines")))
	}
	return b.String()
}

var (
	genKebab = rapid.StringMatching(`[a-z][a-z0-9]{0,4}(-[a-z0-9]{1,3})?`)
	genUpper = rapid.StringMatching(`[A-Z][a-z0-9]{0,5}`)
	genLower = rapid.StringMatching(`[a-z][A-Za-z0-9]{0,5}`).Filter(func(s string) bool { return s != "true" && s != "false" && s != "none" })
	genText  = rapid.StringMatching(`[a-z中文][a-z 中文]{0,8}[a-z中文]`)
)

func quoted(t *rapid.T, label string) string { return `"` + genText.Draw(t, label) + `"` }

// validLines draws the lines of a valid file of the given kind.
func validLines(t *rapid.T, kind syntax.FileKind) [][]string {
	var lines [][]string
	switch kind {
	case syntax.FileKindSystem:
		lines = append(lines, []string{"system", genKebab.Draw(t, "system"), quoted(t, "summary")})
		if rapid.Bool().Draw(t, "notation?") {
			lines = append(lines, []string{"notation", "ddd"})
		}
		lines = append(lines, []string{"language", "go", "{"}, []string{"root", `"."`}, []string{"}"})
		lines = append(lines, []string{"actor", genKebab.Draw(t, "actor"), "person", quoted(t, "summary")})
	case syntax.FileKindComponent:
		lines = append(lines, []string{"component", genKebab.Draw(t, "component"), quoted(t, "summary"), "{"},
			[]string{"paths", `"internal/x/**"`}, []string{"}"})
		for range rapid.IntRange(0, 2).Draw(t, "types") {
			lines = append(lines, []string{"type", genUpper.Draw(t, "type"), "record", quoted(t, "summary"), "{"},
				[]string{"field", genLower.Draw(t, "field"), ":", "map", "<", "string", ",", "int", ">"}, []string{"}"})
		}
		for range rapid.IntRange(0, 2).Draw(t, "ops") {
			lines = append(lines, []string{"op", genLower.Draw(t, "op"), "(", genLower.Draw(t, "param"), ":", "int", ")", "->", "string"})
		}
	case syntax.FileKindCorpus:
		lines = append(lines, []string{"corpus", genKebab.Draw(t, "target") + "." + genKebab.Draw(t, "target")})
		for range rapid.IntRange(0, 2).Draw(t, "cases") {
			lines = append(lines, []string{"case", genKebab.Draw(t, "case"), "coverage", "{"},
				[]string{genLower.Draw(t, "symbol"), "=", "[", "1", ",", quoted(t, "value"), "]"}, []string{"}"})
		}
	}
	return lines
}

// genFileContent draws the content of a file at path: valid in the canonical
// format, valid with random layout, or invalid.
func genFileContent(t *rapid.T, path string) string {
	kind, err := syntax.KindOf(path)
	if err != nil {
		return genText.Draw(t, "other")
	}
	text := layout(t, validLines(t, kind))
	switch rapid.IntRange(0, 2).Draw(t, "variant") {
	case 0:
		return string(syntax.Parse(path, kind, []byte(text)).File.Print())
	case 1:
		return text
	}
	return rapid.SampledFrom([]string{"@\n" + text, text + "\n}\n", text + "type\n", strings.Replace(text, "{", "", 1)}).Draw(t, "invalid")
}

// genFiles draws the design and corpus files of a repository, always with a design/system.uc.
//
//uc:generates cli.format-files.files
func genFiles() *rapid.Generator[map[string]string] {
	return rapid.Custom(func(t *rapid.T) map[string]string {
		files := map[string]string{"design/system.uc": genFileContent(t, "design/system.uc")}
		for range rapid.IntRange(0, 6).Draw(t, "files") {
			var path string
			switch rapid.IntRange(0, 4).Draw(t, "where") {
			case 0:
				path = "design/" + genKebab.Draw(t, "name") + ".uc"
			case 1:
				path = "design/" + genKebab.Draw(t, "dir") + "/" + genKebab.Draw(t, "name") + ".uc"
			case 2, 3:
				path = "corpus/" + genKebab.Draw(t, "dir") + "/" + genKebab.Draw(t, "name") + ".uc"
			default:
				path = rapid.SampledFrom([]string{"design/", "corpus/"}).Draw(t, "dir") + genKebab.Draw(t, "name") + ".txt"
			}
			if path != "design/system.uc" {
				files[path] = genFileContent(t, path)
			}
		}
		return files
	})
}

// expectedStatus is what fmt must report for one file, by syntax's own verdict.
func expectedStatus(path, content string, check bool) (string, []byte) {
	kind, err := syntax.KindOf(path)
	if err != nil {
		return statusInvalid, nil
	}
	parsed := syntax.Parse(path, kind, []byte(content))
	if len(parsed.Diagnostics) > 0 {
		return statusInvalid, nil
	}
	printed := parsed.File.Print()
	switch {
	case string(printed) == content:
		return statusUnchanged, printed
	case check:
		return statusNotCanonical, printed
	}
	return statusRewritten, printed
}

// runMain runs Main with standard output and standard error captured.
func runMain(t rapid.TB, args []string) (int, string, string) {
	dir, err := os.MkdirTemp("", "unicode-out-")
	if err != nil {
		t.Fatalf("前提不成立：建不了临时目录：%v", err)
	}
	defer removeAll(t, dir)
	stdout, err1 := os.Create(filepath.Join(dir, "stdout"))
	stderr, err2 := os.Create(filepath.Join(dir, "stderr"))
	if err1 != nil || err2 != nil {
		t.Fatalf("前提不成立：建不了输出文件：%v %v", err1, err2)
	}
	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	code := Main(args)
	os.Stdout, os.Stderr = savedOut, savedErr
	out, err1 := readClosing(stdout)
	errOut, err2 := readClosing(stderr)
	if err1 != nil || err2 != nil {
		t.Fatalf("前提不成立：读不出捕获的输出：%v %v", err1, err2)
	}
	return code, string(out), string(errOut)
}

func readClosing(f *os.File) ([]byte, error) {
	if err := f.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(f.Name())
}

func removeAll(t rapid.TB, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("清理临时目录 %s 失败：%v", dir, err)
	}
}

func writeRepo(t rapid.TB, files map[string]string) string {
	root, err := os.MkdirTemp("", "unicode-repo-")
	if err != nil {
		t.Fatalf("前提不成立：建不了临时仓库：%v", err)
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("前提不成立：建不了目录：%v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("前提不成立：写不了 %s：%v", path, err)
		}
	}
	return root
}

func readRepo(t rapid.TB, root string) map[string]string {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		content, err := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = string(content)
		return err
	})
	if err != nil {
		t.Fatalf("读不出仓库中的文件：%v", err)
	}
	return files
}

//uc:checks cli.format-files
func checkFlowFormatFiles(t rapid.TB, files map[string]string, check bool) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	want := map[string]string{}
	canonical := map[string][]byte{}
	wantCode := exitOK
	for _, p := range paths {
		want[p], canonical[p] = expectedStatus(p, files[p], check)
		if want[p] == statusInvalid || want[p] == statusNotCanonical {
			wantCode = exitDiagnostics
		}
	}
	flags := []string{"fmt"}
	if check {
		flags = append(flags, "--check")
	}

	jsonRoot, textRoot := writeRepo(t, files), writeRepo(t, files)
	defer removeAll(t, jsonRoot)
	defer removeAll(t, textRoot)

	code, stdout, stderr := runMain(t, append([]string{"--root", jsonRoot, "--format", "json"}, flags...))
	if code != wantCode {
		t.Fatalf("unicode fmt 的退出码是 %d，应当是 %d\n标准输出：%s\n标准错误：%s", code, wantCode, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("--format json 时标准错误应当为空，却是：%s", stderr)
	}
	var report fmtReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("标准输出不是 fmt 的 JSON 报告：%v\n%s", err, stdout)
	}
	if len(report.Files) != len(paths) {
		t.Fatalf("报告列出了 %d 个文件，仓库中有 %d 个：%v", len(report.Files), len(paths), report.Files)
	}
	diagnosed := map[string][]string{}
	for _, d := range report.Diagnostics {
		if d.Code == "" || d.Message == "" || d.Fix == "" || d.Line < 1 || d.Column < 1 {
			t.Fatalf("诊断缺少诊断码、原因、修正或位置：%+v", d)
		}
		diagnosed[d.Path] = append(diagnosed[d.Path], d.Code)
	}
	for i, f := range report.Files {
		p := paths[i]
		if f.Path != p || f.Status != want[p] {
			t.Fatalf("报告中第 %d 个文件是 %s（%s），应当是 %s（%s）", i+1, f.Path, f.Status, p, want[p])
		}
		switch {
		case want[p] == statusInvalid && len(diagnosed[p]) == 0:
			t.Fatalf("不合法的文件 %s 没有诊断", p)
		case want[p] == statusNotCanonical && !slices.Equal(diagnosed[p], []string{"cli.not-canonical"}):
			t.Fatalf("不是规范格式的文件 %s 的诊断应当恰是一条 cli.not-canonical，却是 %v", p, diagnosed[p])
		case (want[p] == statusUnchanged || want[p] == statusRewritten) && len(diagnosed[p]) > 0:
			t.Fatalf("合法的文件 %s 不应当有诊断，却有 %v", p, diagnosed[p])
		}
	}
	after := readRepo(t, jsonRoot)
	if len(after) != len(files) {
		t.Fatalf("unicode fmt 之后仓库中有 %d 个文件，之前有 %d 个", len(after), len(files))
	}
	for _, p := range paths {
		switch {
		case (check || want[p] == statusInvalid) && after[p] != files[p]:
			t.Fatalf("%s 不应当被改写（--check %v，状态 %s），却被改写了", p, check, want[p])
		case !check && want[p] != statusInvalid && after[p] != string(canonical[p]):
			t.Fatalf("合法的文件 %s 在 unicode fmt 之后不是规范格式\n之后：\n%s\n规范格式：\n%s", p, after[p], canonical[p])
		}
	}

	code, stdout, stderr = runMain(t, append([]string{"--root", textRoot}, flags...))
	if code != wantCode {
		t.Fatalf("--format text 时 unicode fmt 的退出码是 %d，应当是 %d", code, wantCode)
	}
	var listed []string
	for _, p := range paths {
		if want[p] == statusRewritten || want[p] == statusNotCanonical {
			listed = append(listed, p+"\n")
		}
	}
	if stdout != strings.Join(listed, "") {
		t.Fatalf("--format text 时标准输出应当逐行列出改写或不是规范格式的文件：\n%s\n却是：\n%s", strings.Join(listed, ""), stdout)
	}
	if lines := strings.Count(stderr, "\n"); lines != len(report.Diagnostics) {
		t.Fatalf("--format text 时标准错误应当每条诊断一行，共 %d 行，却有 %d 行：\n%s", len(report.Diagnostics), lines, stderr)
	}
	for _, d := range report.Diagnostics {
		line := fmt.Sprintf("%s:%d:%d: %s: %s（修正：%s）\n", d.Path, d.Line, d.Column, d.Code, d.Message, d.Fix)
		if !strings.Contains(stderr, line) {
			t.Fatalf("--format text 时标准错误中没有诊断 %q", line)
		}
	}
	if !maps.Equal(readRepo(t, textRoot), after) {
		t.Fatalf("--format text 与 --format json 对同一个仓库改写的结果不同")
	}
}

func TestFlowFormatFiles(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFlowFormatFiles(t, genFiles().Draw(t, "files"), rapid.Bool().Draw(t, "check"))
	})
}

func FuzzFlowFormatFiles(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFlowFormatFiles(t, genFiles().Draw(t, "files"), rapid.Bool().Draw(t, "check"))
	}))
}

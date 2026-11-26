package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
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
		t.Fatalf("setup failed: cannot create a temporary directory: %v", err)
	}
	defer removeAll(t, dir)
	stdout, err1 := os.Create(filepath.Join(dir, "stdout"))
	stderr, err2 := os.Create(filepath.Join(dir, "stderr"))
	if err1 != nil || err2 != nil {
		t.Fatalf("setup failed: cannot create the output files: %v %v", err1, err2)
	}
	code := func() int {
		savedOut, savedErr := os.Stdout, os.Stderr
		defer func() { os.Stdout, os.Stderr = savedOut, savedErr }()
		os.Stdout, os.Stderr = stdout, stderr
		return Main(args)
	}()
	out, err1 := readClosing(stdout)
	errOut, err2 := readClosing(stderr)
	if err1 != nil || err2 != nil {
		t.Fatalf("setup failed: cannot read the captured output: %v %v", err1, err2)
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
		t.Fatalf("cannot remove the temporary directory %s: %v", dir, err)
	}
}

// modes are the permissions a repository's files are written with; modeOf
// picks one by the path, since a predicate takes only its symbols.
var modes = []os.FileMode{0o644, 0o600, 0o755, 0o640}

func modeOf(path string) os.FileMode {
	h := fnv.New32a()
	h.Write([]byte(path))
	return modes[h.Sum32()%uint32(len(modes))]
}

// writeRepo writes the files into a new directory, each with the mode modeOf gives it.
func writeRepo(t rapid.TB, files map[string]string) string {
	root, err := os.MkdirTemp("", "unicode-repo-")
	if err != nil {
		t.Fatalf("setup failed: cannot create a temporary repository: %v", err)
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("setup failed: cannot create a directory: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil || os.Chmod(full, modeOf(path)) != nil {
			t.Fatalf("setup failed: cannot write %s: %v", path, err)
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
		t.Fatalf("cannot read the repository's files: %v", err)
	}
	return files
}

// differsAt is the line and byte column of the first byte where a and b
// differ, counted line by line.
func differsAt(a, b string) (int, int) {
	al, bl := strings.SplitAfter(a, "\n"), strings.SplitAfter(b, "\n")
	for i := range al {
		if i < len(bl) && al[i] == bl[i] {
			continue
		}
		column := 1
		for column <= len(al[i]) && i < len(bl) && column <= len(bl[i]) && al[i][column-1] == bl[i][column-1] {
			column++
		}
		return i + 1, column
	}
	return len(al), len(al[len(al)-1]) + 1
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
		t.Fatalf("unicode fmt exited with %d instead of %d\nstdout: %s\nstderr: %s", code, wantCode, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("with --format json, stderr should be empty, yet it is: %s", stderr)
	}
	var report fmtReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not fmt's JSON report: %v\n%s", err, stdout)
	}
	if len(report.Files) != len(paths) {
		t.Fatalf("the report lists %d files, the repository has %d: %v", len(report.Files), len(paths), report.Files)
	}
	diagnosed := map[string][]string{}
	for _, d := range report.Diagnostics {
		if d.Code == "" || d.Message == "" || d.Fix == "" || d.Line < 1 || d.Column < 1 {
			t.Fatalf("a diagnostic lacks its code, cause, fix or position: %+v", d)
		}
		diagnosed[d.Path] = append(diagnosed[d.Path], d.Code)
		if d.Code != "cli.not-canonical" {
			continue
		}
		if line, column := differsAt(files[d.Path], string(canonical[d.Path])); d.Line != line || d.Column != column {
			t.Fatalf("the cli.not-canonical of %s stands at %d:%d, yet the first byte that differs from the canonical form is at %d:%d", d.Path, d.Line, d.Column, line, column)
		}
	}
	for i, f := range report.Files {
		p := paths[i]
		if f.Path != p || f.Status != want[p] {
			t.Fatalf("file %d of the report is %s (%s) instead of %s (%s)", i+1, f.Path, f.Status, p, want[p])
		}
		switch {
		case want[p] == statusInvalid && len(diagnosed[p]) == 0:
			t.Fatalf("the invalid file %s has no diagnostic", p)
		case want[p] == statusNotCanonical && !slices.Equal(diagnosed[p], []string{"cli.not-canonical"}):
			t.Fatalf("the non-canonical file %s should have exactly one cli.not-canonical, yet it has %v", p, diagnosed[p])
		case (want[p] == statusUnchanged || want[p] == statusRewritten) && len(diagnosed[p]) > 0:
			t.Fatalf("the valid file %s should have no diagnostic, yet it has %v", p, diagnosed[p])
		}
	}
	after := readRepo(t, jsonRoot)
	if len(after) != len(files) {
		t.Fatalf("after unicode fmt the repository has %d files, before it had %d", len(after), len(files))
	}
	for _, p := range paths {
		info, err := os.Stat(filepath.Join(jsonRoot, filepath.FromSlash(p)))
		if err != nil {
			t.Fatalf("after unicode fmt, %s cannot be read: %v", p, err)
		}
		if info.Mode().Perm() != modeOf(p) {
			t.Fatalf("after unicode fmt the mode of %s is %v, before it was %v", p, info.Mode().Perm(), modeOf(p))
		}
	}
	for _, p := range paths {
		switch {
		case (check || want[p] == statusInvalid) && after[p] != files[p]:
			t.Fatalf("%s should not be rewritten (--check %v, status %s), yet it was", p, check, want[p])
		case !check && want[p] != statusInvalid && after[p] != string(canonical[p]):
			t.Fatalf("the valid file %s is not canonical after unicode fmt\nafter:\n%s\ncanonical:\n%s", p, after[p], canonical[p])
		}
	}

	code, stdout, stderr = runMain(t, append([]string{"--root", textRoot}, flags...))
	if code != wantCode {
		t.Fatalf("with --format text, unicode fmt exited with %d instead of %d", code, wantCode)
	}
	var listed []string
	for _, p := range paths {
		if want[p] == statusRewritten || want[p] == statusNotCanonical {
			listed = append(listed, p+"\n")
		}
	}
	if stdout != strings.Join(listed, "") {
		t.Fatalf("with --format text, stdout should list the rewritten or non-canonical files one per line:\n%s\nyet it is:\n%s", strings.Join(listed, ""), stdout)
	}
	if lines := strings.Count(stderr, "\n"); lines != len(report.Diagnostics) {
		t.Fatalf("with --format text, stderr should have one line per diagnostic, %d in all, yet it has %d:\n%s", len(report.Diagnostics), lines, stderr)
	}
	for _, d := range report.Diagnostics {
		line := fmt.Sprintf("%s:%d:%d: %s: %s (fix: %s)\n", d.Path, d.Line, d.Column, d.Code, d.Message, d.Fix)
		if !strings.Contains(stderr, line) {
			t.Fatalf("with --format text, stderr lacks the diagnostic %q", line)
		}
	}
	if !maps.Equal(readRepo(t, textRoot), after) {
		t.Fatalf("--format text and --format json rewrite the same repository differently")
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

package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/design"
	"github.com/lincyaw/unicode/internal/syntax"
)

// canonical writes text in the canonical format; the text is valid.
func canonical(t *rapid.T, path, text string) string {
	kind, err := syntax.KindOf(path)
	if err != nil {
		t.Fatalf("setup failed: %s is not a design or corpus path: %v", path, err)
	}
	parsed := syntax.Parse(path, kind, []byte(text))
	if parsed.File == nil {
		t.Fatalf("setup failed: the generated file %s does not parse: %v\n%s", path, parsed.Diagnostics, text)
	}
	return string(parsed.File.Print())
}

// genDesign draws the files of a small valid design: a system file with a
// person, top-level components whose records refer to those of the components
// they use, a flow in which the person calls an operation, and a property
// with a limited symbol and a case, now and then with recorded cases.
func genDesign(t *rapid.T) map[string]string {
	files := map[string]string{}
	system := "system sys-" + genKebab.Draw(t, "system") + " \"the system\"\n\n"
	if rapid.Bool().Draw(t, "notation?") {
		system += "notation ddd\n\n"
	}
	files["design/system.uc"] = canonical(t, "design/system.uc", system+"language go {\nroot \".\"\n}\n\nactor who person \"a person\"\n")
	var names []string
	for i := range rapid.IntRange(1, 3).Draw(t, "components") {
		name := genKebab.Draw(t, "component") + "-" + strconv.Itoa(i)
		var b strings.Builder
		fmt.Fprintf(&b, "component %s \"component %d\" {\nfocus %s\npaths \"internal/%s/**\"\n", name, i,
			rapid.SampledFrom([]string{"core", "support", "utility"}).Draw(t, "focus"), name)
		used := ""
		if len(names) > 0 && rapid.Bool().Draw(t, "uses?") {
			used = rapid.SampledFrom(names).Draw(t, "used")
			fmt.Fprintf(&b, "uses %s\n", used)
		}
		b.WriteString("}\n\ntype Item record \"an item\" {\nfield n: int\n")
		if used != "" {
			fmt.Fprintf(&b, "field other: optional<%s.Item>\n", used)
		}
		b.WriteString("}\n\nop run(x: int) -> Item \"runs\"\n\nflow runs \"the person runs it\" {\nwho -> run(a)\n}\n\n")
		low := rapid.IntRange(0, 5).Draw(t, "low")
		fmt.Fprintf(&b, "property bounded invariant \"n stays small\" {\nabout Item, run\nforall n: int\nwhere n in %d..%d\ncase one {\nn = %d\n}\n}\n",
			low, low+4, low+rapid.IntRange(0, 4).Draw(t, "value"))
		path := "design/" + name + ".uc"
		files[path] = canonical(t, path, b.String())
		if rapid.Bool().Draw(t, "corpus?") {
			corpus := "corpus/" + name + "/bounded.uc"
			files[corpus] = canonical(t, corpus, fmt.Sprintf("corpus %s.bounded\n\ncase r-%08x coverage {\nn = %d\n}\n", name,
				rapid.Uint32().Draw(t, "recorded"), low+rapid.IntRange(0, 4).Draw(t, "recordedValue")))
		}
		names = append(names, name)
	}
	return files
}

// changes break a valid design in one place: a syntax error, a layout that is
// not canonical, a violated constraint, or a file that does not belong.
var changes = []func(files map[string]string, path string){
	func(files map[string]string, path string) { files[path] += "@\n" },
	func(files map[string]string, path string) {
		files[path] = strings.Replace(files[path], "\n", "\n\n", 1)
	},
	func(files map[string]string, path string) { files[path] = strings.Replace(files[path], "  ", "   ", 1) },
	func(files map[string]string, path string) {
		files[path] = strings.Replace(files[path], "  focus ", "  // focus ", 1)
	},
	func(files map[string]string, path string) {
		files[path] = strings.Replace(files[path], "n = ", "n = 1", 1)
	},
	func(files map[string]string, path string) {
		files[path] = strings.Replace(files[path], "  paths", "  uses nowhere\n  paths", 1)
	},
	func(files map[string]string, path string) {
		if path != "design/system.uc" && strings.HasPrefix(path, "design/") {
			files["design/moved/"+strings.TrimPrefix(path, "design/")] = files[path]
			delete(files, path)
		}
	},
	func(files map[string]string, _ string) { delete(files, "design/system.uc") },
	func(files map[string]string, _ string) { files["design/readme.txt"] = "notes\n" },
}

// genCheckFiles draws the design and corpus files of a repository: a valid
// design, the same with one or two changes that break it, or random files as
// format-files draws them; now and then a symbolic link is among them.
//
//uc:generates cli.check-design.files
func genCheckFiles() *rapid.Generator[map[string]RepoFile] {
	return rapid.Custom(func(t *rapid.T) map[string]RepoFile {
		if rapid.IntRange(0, 3).Draw(t, "random?") == 0 {
			return genFiles(readableModes).Draw(t, "random")
		}
		texts := genDesign(t)
		for range rapid.IntRange(0, 2).Draw(t, "changes") {
			path := rapid.SampledFrom(slices.Sorted(maps.Keys(texts))).Draw(t, "changed")
			changes[rapid.IntRange(0, len(changes)-1).Draw(t, "change")](texts, path)
		}
		files := map[string]RepoFile{}
		for _, p := range slices.Sorted(maps.Keys(texts)) {
			files[p] = RepoFile{Content: texts[p], Mode: rapid.SampledFrom([]int{0o644, 0o600, 0o755}).Draw(t, "mode")}
		}
		if rapid.IntRange(0, 4).Draw(t, "link?") == 0 {
			files["design/link.uc"] = genLink(t, "design/link.uc", files)
		}
		return files
	})
}

func sortJSON(ds []diagnosticJSON) []diagnosticJSON {
	return slices.SortedFunc(slices.Values(ds), func(a, b diagnosticJSON) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column),
			cmp.Compare(a.Code, b.Code), cmp.Compare(a.Message, b.Message))
	})
}

// expectedDiagnostics are the diagnostics of the files as syntax.KindOf,
// syntax.Parse, the canonical comparison and design.Load give them, run
// directly. The CLI's own diagnostics (cli.*) carry only their code and position.
func expectedDiagnostics(files map[string]RepoFile) []diagnosticJSON {
	var want []diagnosticJSON
	var trees []syntax.File
	cli := func(code, path string, line, column int) {
		want = append(want, diagnosticJSON{Code: code, Subject: path, Path: path, Line: line, Column: column})
	}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		f := files[p]
		kind, err := syntax.KindOf(p)
		switch {
		case isSymlink(f):
			cli("cli.symlink", p, 1, 1)
			continue
		case err != nil:
			cli("cli.not-a-design-file", p, 1, 1)
			continue
		}
		parsed := syntax.Parse(p, kind, []byte(f.Content))
		if parsed.File == nil {
			want = append(want, diagnosticsJSON(parsed.Diagnostics)...)
			continue
		}
		if printed := string(parsed.File.Print()); printed != f.Content {
			line, column := differsAt(f.Content, printed)
			cli("cli.not-canonical", p, line, column)
		}
		trees = append(trees, *parsed.File)
	}
	return append(want, diagnosticsJSON(design.Load(trees).Diagnostics)...)
}

//uc:checks cli.check-design
func checkFlowCheckDesign(t rapid.TB, files map[string]RepoFile, designOnly bool) {
	root := writeRepo(t, files)
	defer removeAll(t, filepath.Dir(root))
	args := func(format string) []string {
		args := []string{"--root", root, "--format", format, "check"}
		if designOnly {
			args = append(args, "--design-only")
		}
		return args
	}
	code, stdout, stderr := runMain(t, args("json"))
	if !maps.Equal(readRepo(t, root), files) {
		t.Fatalf("unicode check changed the repository")
	}
	if !designOnly {
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "--design-only") {
			t.Fatalf("unicode check without --design-only should be a usage error that names --design-only, yet it exited with %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		return
	}
	want := sortJSON(expectedDiagnostics(files))
	wantCode := exitOK
	if len(want) > 0 {
		wantCode = exitDiagnostics
	}
	if code != wantCode || stderr != "" {
		t.Fatalf("unicode check --design-only --format json exited with %d instead of %d, or wrote to stderr\nstdout: %s\nstderr: %s", code, wantCode, stdout, stderr)
	}
	var report checkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.Diagnostics == nil {
		t.Fatalf("stdout is not check's JSON report {\"diagnostics\": [...]}: %v\n%s", err, stdout)
	}
	got := sortJSON(report.Diagnostics)
	if len(got) != len(want) {
		t.Fatalf("unicode check reported %d diagnostics, Parse, the canonical comparison and Load give %d\ngot: %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i, g := range got {
		w := want[i]
		switch {
		case g.Code != w.Code || g.Path != w.Path || g.Line != w.Line || g.Column != w.Column:
			t.Fatalf("diagnostic %d is %s at %s:%d:%d, yet it should be %s at %s:%d:%d", i+1, g.Code, g.Path, g.Line, g.Column, w.Code, w.Path, w.Line, w.Column)
		case strings.HasPrefix(g.Code, "cli.") && (g.Subject != g.Path || g.Message == "" || g.Fix == ""):
			t.Fatalf("the diagnostic %s of %s lacks its subject, cause or fix: %+v", g.Code, g.Path, g)
		case !strings.HasPrefix(g.Code, "cli.") && g != w:
			t.Fatalf("the diagnostic %s is reported as %+v, yet Parse or Load gives %+v", g.Code, g, w)
		}
	}
	code, stdout, stderr = runMain(t, args("text"))
	if code != wantCode || stdout != "" {
		t.Fatalf("with --format text, unicode check exited with %d instead of %d, or wrote to stdout: %s", code, wantCode, stdout)
	}
	if lines := strings.Count(stderr, "\n"); lines != len(got) {
		t.Fatalf("with --format text, stderr should have one line per diagnostic, %d in all, yet it has %d:\n%s", len(got), lines, stderr)
	}
	for _, d := range got {
		if line := fmt.Sprintf("%s:%d:%d: %s: %s (fix: %s)\n", d.Path, d.Line, d.Column, d.Code, d.Message, d.Fix); !strings.Contains(stderr, line) {
			t.Fatalf("with --format text, stderr lacks the diagnostic %q", line)
		}
	}
}

func TestFlowCheckDesign(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFlowCheckDesign(t, genCheckFiles().Draw(t, "files"), rapid.IntRange(0, 4).Draw(t, "designOnly") > 0)
	})
}

func FuzzFlowCheckDesign(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFlowCheckDesign(t, genCheckFiles().Draw(t, "files"), rapid.IntRange(0, 4).Draw(t, "designOnly") > 0)
	}))
}

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
	want, trees := readExpected(files)
	return append(want, diagnosticsJSON(design.Load(trees).Diagnostics)...)
}

// readExpected gives the diagnostics of reading the design and corpus files
// as syntax.KindOf, syntax.Parse and the canonical comparison give them, and
// the trees of the files that parse; the other files of the repository are
// not read.
func readExpected(files map[string]RepoFile) ([]diagnosticJSON, []syntax.File) {
	var want []diagnosticJSON
	var trees []syntax.File
	cli := func(code, path string, line, column int) {
		want = append(want, diagnosticJSON{Code: code, Subject: path, Path: path, Line: line, Column: column})
	}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if !strings.HasPrefix(p, "design/") && !strings.HasPrefix(p, "corpus/") {
			continue
		}
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
	return want, trees
}

//uc:checks cli.check-design
func checkFlowCheckDesign(t rapid.TB, files map[string]RepoFile, designOnly bool) {
	root := writeRepo(t, files)
	defer removeAll(t, filepath.Dir(root))
	files = readRepo(t, root)
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
	want := sortJSON(expectedDiagnostics(files))
	if !designOnly {
		var report codeReport
		if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.Diagnostics == nil || report.Statuses == nil || stderr != "" {
			t.Fatalf("stdout of unicode check --format json is not its report {\"diagnostics\": [...], \"statuses\": {...}}, or it wrote to stderr: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}
		if wantCode := map[bool]int{false: exitOK, true: exitDiagnostics}[len(report.Diagnostics) > 0]; code != wantCode {
			t.Fatalf("unicode check exited with %d, yet it reported %d diagnostics", code, len(report.Diagnostics))
		}
		for _, w := range want {
			if !slices.ContainsFunc(report.Diagnostics, func(g diagnosticJSON) bool {
				return g.Code == w.Code && g.Path == w.Path && g.Line == w.Line && g.Column == w.Column && (strings.HasPrefix(g.Code, "cli.") || g == w)
			}) {
				t.Fatalf("unicode check without --design-only leaves out the diagnostic %+v, which the files' design alone gives", w)
			}
		}
		return
	}
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

// replace changes the first old in the file at path to new.
func replace(files map[string]string, path, old, new string) {
	files[path] = strings.Replace(files[path], old, new, 1)
}

// codeBreaks change a drawn repository in one place of component c, so that
// it violates one constraint of the design, of the code or of the bindings
// between them, its code does not load, or it configures a language that has
// no backend; some changes cascade into further diagnostics.
var codeBreaks = []func(t *rapid.T, r repo, c repoComponent, files map[string]string){
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		files["design/"+c.name+".uc"] += "@\n"
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "design/"+c.name+".uc", "\n", "\n\n")
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "design/"+c.name+".uc", "field n: int", "field n: Nothing")
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "design/"+c.name+".uc", "field n: int\n", "field n: int\n  field label: string\n")
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "internal/"+c.name+"/"+c.name+".go", "//uc:realizes "+c.name+".Item\n", "")
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "internal/"+c.name+"/"+c.name+".go", "func Make(n int) Item {\n\treturn Item{N: n}", "func Make(n int) *Item {\n\treturn &Item{N: n}")
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "internal/"+c.name+"/"+c.name+".go", "return Item{N: n}", `panic("unicode: unimplemented")`)
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "internal/"+c.name+"/"+c.name+"_test.go", "//uc:checks "+c.name+".keeps\n", "")
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "internal/"+c.name+"/"+c.name+".go", "//uc:realizes "+c.name+".make", "//uc:realise "+c.name+".make")
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		replace(files, "internal/"+c.name+"/"+c.name+".go", "//uc:realizes "+c.name+".make", "//uc:realizes "+c.name+".makeIt")
	},
	func(_ *rapid.T, _ repo, _ repoComponent, files map[string]string) {
		files["internal/stray/stray.go"] = "// Package stray belongs to no component.\npackage stray\n"
	},
	func(t *rapid.T, r repo, _ repoComponent, files map[string]string) {
		// A component calls into an earlier one that it does not use; the earlier never imports the later.
		type pair struct{ from, to string }
		var pairs []pair
		for i, c := range r.components {
			for _, o := range r.components[:i] {
				if o.name != c.uses {
					pairs = append(pairs, pair{c.name, o.name})
				}
			}
		}
		if len(pairs) == 0 {
			return
		}
		p := rapid.SampledFrom(pairs).Draw(t, "undeclared")
		files["internal/"+p.from+"/extra.go"] = fmt.Sprintf("package %s\n\nimport \"%s/internal/%s\"\n\nfunc makeOther() { _ = %s.Make(1) }\n", p.from, modulePath, p.to, p.to)
	},
	func(_ *rapid.T, _ repo, c repoComponent, files map[string]string) {
		files["internal/"+c.name+"/"+c.name+".go"] += "\nfunc broken() int { return undefined }\n"
	},
	func(_ *rapid.T, _ repo, _ repoComponent, files map[string]string) {
		replace(files, "design/system.uc", "actor who", "language python {\n  root \"py\"\n}\n\nactor who")
	},
	func(_ *rapid.T, _ repo, _ repoComponent, files map[string]string) {
		replace(files, "design/system.uc", `root "."`, `root "src"`)
	},
}

// genCodeRepo draws a small repository of design files and Go code that
// agree everywhere, or that one of codeBreaks has changed in one place.
//
//uc:generates cli.check-code.files
func genCodeRepo() *rapid.Generator[map[string]RepoFile] {
	return rapid.Custom(func(t *rapid.T) map[string]RepoFile {
		r := drawRepo(t)
		texts := r.texts(t)
		if rapid.IntRange(0, 3).Draw(t, "broken?") > 0 {
			c := rapid.SampledFrom(r.components).Draw(t, "brokenComponent")
			codeBreaks[rapid.IntRange(0, len(codeBreaks)-1).Draw(t, "break")](t, r, c, texts)
		}
		return repoFiles(texts)
	})
}

//uc:checks cli.check-code
func checkFlowCheckCode(t rapid.TB, files map[string]RepoFile) {
	root := writeRepo(t, files)
	defer removeAll(t, filepath.Dir(root))
	want, statuses := checkDirectly(t, root, files)
	wantCode := exitOK
	if len(want) > 0 {
		wantCode = exitDiagnostics
	}
	code, stdout, stderr := runMain(t, []string{"--root", root, "--format", "json", "check"})
	if code != wantCode || stderr != "" {
		t.Fatalf("unicode check --format json exited with %d instead of %d, or wrote to stderr\nstdout: %s\nstderr: %s", code, wantCode, stdout, stderr)
	}
	if !maps.Equal(readRepo(t, root), files) {
		t.Fatalf("unicode check changed the repository")
	}
	var report codeReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.Diagnostics == nil || report.Statuses == nil {
		t.Fatalf("stdout is not check's JSON report {\"diagnostics\": [...], \"statuses\": {...}}: %v\n%s", err, stdout)
	}
	if problem := sameDiagnostics(sortJSON(report.Diagnostics), want); problem != "" {
		t.Fatalf("unicode check reports other diagnostics than Load, Extract, AddFragment, Derive and Conform give: %s", problem)
	}
	if !maps.Equal(report.Statuses, statuses) {
		for _, id := range sortedKeys(statuses) {
			if report.Statuses[id] != statuses[id] {
				t.Fatalf("unicode check gives %s the status %q, yet Conform gives %q", id, report.Statuses[id], statuses[id])
			}
		}
		t.Fatalf("unicode check gives statuses to items that Conform gives none: %v", report.Statuses)
	}
	code, stdout, stderr = runMain(t, []string{"--root", root, "check"})
	if code != wantCode || stdout != "" {
		t.Fatalf("with --format text, unicode check exited with %d instead of %d, or wrote to stdout: %s", code, wantCode, stdout)
	}
	if lines := strings.Count(stderr, "\n"); lines != len(report.Diagnostics) {
		t.Fatalf("with --format text, stderr should have one line per diagnostic, %d in all, yet it has %d:\n%s", len(report.Diagnostics), lines, stderr)
	}
	for _, d := range report.Diagnostics {
		if line := fmt.Sprintf("%s:%d:%d: %s: %s (fix: %s)\n", d.Path, d.Line, d.Column, d.Code, d.Message, d.Fix); !strings.Contains(stderr, line) {
			t.Fatalf("with --format text, stderr lacks the diagnostic %q", line)
		}
	}
}

func TestFlowCheckCode(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFlowCheckCode(t, genCodeRepo().Draw(t, "files"))
	})
}

func FuzzFlowCheckCode(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFlowCheckCode(t, genCodeRepo().Draw(t, "files"))
	}))
}

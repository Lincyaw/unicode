package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
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

// pathKind is the kind of the file at the clean slash path p relative to the
// root (docs/language.md §1), and false for a path that holds no design or
// corpus file: one outside design/ and corpus/, or not named <name>.uc.
func pathKind(p string) (syntax.FileKind, bool) {
	named := strings.HasSuffix(p, ".uc") && len(path.Base(p)) > len(".uc")
	switch {
	case !named:
	case p == "design/system.uc":
		return syntax.FileKindSystem, true
	case strings.HasPrefix(p, "design/"):
		return syntax.FileKindComponent, true
	case strings.HasPrefix(p, "corpus/"):
		return syntax.FileKindCorpus, true
	}
	return "", false
}

// genFileContent draws the content of a file at path: valid in the canonical
// format, valid with random layout, or invalid.
func genFileContent(t *rapid.T, path string) string {
	kind, ok := pathKind(path)
	if !ok {
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

// RepoFile is a file of a repository: its content and permission bits. A
// symbolic link has os.ModeSymlink among its bits and the path it points to
// as its content, as in git.
//
//uc:realizes cli.RepoFile
type RepoFile struct {
	Content string
	Mode    int
}

// symlinkMode is the mode of every symbolic link on Linux.
const symlinkMode = int(os.ModeSymlink | 0o777)

func isSymlink(f RepoFile) bool { return os.FileMode(f.Mode)&os.ModeSymlink != 0 }

// readableModes are permission bits that let the owner read a file;
// fileModes add bits that let no one read it.
var (
	readableModes = []int{0o644, 0o600, 0o755, 0o640}
	fileModes     = append(slices.Clone(readableModes), 0o000)
)

// genRepoFile draws the content of a file at path and its permission bits among modes.
func genRepoFile(t *rapid.T, path string, modes []int) RepoFile {
	return RepoFile{Content: genFileContent(t, path), Mode: rapid.SampledFrom(modes).Draw(t, "mode")}
}

// genLink draws a symbolic link at link: to another file of files, to its own
// directory or the one above it, or to a path that does not exist.
func genLink(t *rapid.T, link string, files map[string]RepoFile) RepoFile {
	targets := []string{".", "..", "missing_" + genKebab.Draw(t, "missing") + ".uc"}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		rel, _ := filepath.Rel(path.Dir(link), p)
		targets = append(targets, filepath.ToSlash(rel))
	}
	return RepoFile{Content: rapid.SampledFrom(targets).Draw(t, "target"), Mode: symlinkMode}
}

// genFiles draws the design and corpus files of a repository, always with a
// design/system.uc, each with permission bits among modes, and now and then
// a symbolic link among them. No other directory of the repository holds a
// design/system.uc, which would make it the root of a repository of its own.
//
//uc:generates cli.format-files.files
func genFiles(modes []int) *rapid.Generator[map[string]RepoFile] {
	return rapid.Custom(func(t *rapid.T) map[string]RepoFile {
		files := map[string]RepoFile{"design/system.uc": genRepoFile(t, "design/system.uc", modes)}
		for range rapid.IntRange(0, 6).Draw(t, "files") {
			var path string
			where := rapid.IntRange(0, 5).Draw(t, "where")
			switch where {
			case 0:
				path = "design/" + genKebab.Draw(t, "name") + ".uc"
			case 1:
				path = "design/" + genKebab.Draw(t, "dir") + "/" + genKebab.Draw(t, "name") + ".uc"
			case 2, 3:
				path = "corpus/" + genKebab.Draw(t, "dir") + "/" + genKebab.Draw(t, "name") + ".uc"
			case 4:
				path = rapid.SampledFrom([]string{"design/", "corpus/"}).Draw(t, "dir") + genKebab.Draw(t, "name") + ".txt"
			default:
				path = rapid.SampledFrom([]string{"design/", "corpus/", "design/sub/", "corpus/sub/"}).Draw(t, "dir") + genKebab.Draw(t, "name") + ".uc"
			}
			switch {
			case strings.HasSuffix("/"+path, "/design/system.uc"):
			case where == 5:
				files[path] = genLink(t, path, files)
			default:
				files[path] = genRepoFile(t, path, modes)
			}
		}
		return files
	})
}

// genTargets draws the path arguments of fmt, relative to the repository's
// root: none, or files and directories of files in design/ and corpus/, now
// and then with a path that does not exist or one outside those two directories.
//
//uc:generates cli.format-files.targets
func genTargets(files map[string]RepoFile) *rapid.Generator[[]string] {
	var existing []string
	for p := range files {
		for ; p != "."; p = path.Dir(p) {
			existing = append(existing, p)
		}
	}
	slices.Sort(existing)
	existing = slices.Compact(existing)
	return rapid.Custom(func(t *rapid.T) []string {
		var targets []string
		for range rapid.IntRange(0, 3).Draw(t, "targets") {
			targets = append(targets, rapid.SampledFrom(existing).Draw(t, "target"))
		}
		if rapid.IntRange(0, 3).Draw(t, "stray?") == 0 {
			missing := genKebab.Draw(t, "missing")
			stray := rapid.SampledFrom([]string{".", "..", "README.md", "docs", "designs", "design/missing_" + missing + ".uc", "corpus/missing_" + missing}).Draw(t, "stray")
			targets = slices.Insert(targets, rapid.IntRange(0, len(targets)).Draw(t, "at"), stray)
		}
		return targets
	})
}

// genAt draws the working directory of the command, relative to the root:
// the root, a directory of files, a subdirectory that holds no design file,
// or a directory outside the repository.
//
//uc:generates cli.format-files.at
func genAt(files map[string]RepoFile) *rapid.Generator[string] {
	inside := []string{".", "docs", "docs/notes"}
	for p := range files {
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			inside = append(inside, d)
		}
	}
	slices.Sort(inside)
	inside = slices.Compact(inside)
	return rapid.OneOf(rapid.SampledFrom(inside), rapid.SampledFrom(inside), rapid.SampledFrom([]string{"..", "../outside", "../outside/deeper"}))
}

// strayTarget describes the first target that is outside design/ and corpus/
// or names no file or directory of files, or returns "".
func strayTarget(files map[string]RepoFile, targets []string) string {
	for _, target := range targets {
		clean := path.Clean(target)
		top, _, _ := strings.Cut(clean, "/")
		exists := slices.ContainsFunc(slices.Collect(maps.Keys(files)), func(p string) bool { return p == clean || strings.HasPrefix(p, clean+"/") })
		switch {
		case top != "design" && top != "corpus":
			return target + ", which is outside design/ and corpus/"
		case !exists:
			return target + ", which does not exist"
		}
	}
	return ""
}

// under tells whether p is one of the targets or lies under one; with no targets every path is.
func under(p string, targets []string) bool {
	return len(targets) == 0 || slices.ContainsFunc(targets, func(target string) bool {
		clean := path.Clean(target)
		return p == clean || strings.HasPrefix(p, clean+"/")
	})
}

// expectation is what fmt must report for one file: its status, the codes of
// its diagnostics in order, and its canonical form when it has one.
type expectation struct {
	status    string
	codes     []string
	canonical []byte
}

// expected is the expectation for the file f at path, which the process can
// read when readable is set. A symbolic link, a path that holds no design or
// corpus file and a file that cannot be read are invalid, in this order, with
// a diagnostic of the command line's; a file that syntax rejects is invalid
// with syntax's diagnostics.
func expected(path string, f RepoFile, readable, check bool) expectation {
	kind, design := pathKind(path)
	switch {
	case isSymlink(f):
		return expectation{status: statusInvalid, codes: []string{"cli.symlink"}}
	case !design:
		return expectation{status: statusInvalid, codes: []string{"cli.not-a-design-file"}}
	case !readable:
		return expectation{status: statusInvalid, codes: []string{"cli.unreadable-file"}}
	}
	parsed := syntax.Parse(path, kind, []byte(f.Content))
	if len(parsed.Diagnostics) > 0 {
		e := expectation{status: statusInvalid}
		for _, d := range parsed.Diagnostics {
			e.codes = append(e.codes, d.Code)
		}
		return e
	}
	printed := parsed.File.Print()
	switch {
	case string(printed) == f.Content:
		return expectation{status: statusUnchanged, canonical: printed}
	case check:
		return expectation{status: statusNotCanonical, codes: []string{"cli.not-canonical"}, canonical: printed}
	}
	return expectation{status: statusRewritten, canonical: printed}
}

// readable tells whether the process can open the file at full for reading.
func readable(full string) bool {
	f, err := os.Open(full)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

// runIn runs Main in the working directory dir.
func runIn(t rapid.TB, dir string, args []string) (int, string, string) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("setup failed: cannot read the working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("setup failed: cannot enter %s: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("cannot return to the working directory %s: %v", wd, err)
		}
	}()
	return runMain(t, args)
}

// workDir makes the directory at, relative to root, and returns it. A
// directory outside the repository must have no ancestor that holds a
// design/system.uc, or the command would find a root there.
func workDir(t rapid.TB, root, at string) string {
	dir := filepath.Join(root, filepath.FromSlash(at))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup failed: cannot create the working directory %s: %v", at, err)
	}
	if !isOutside(at) {
		return dir
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "design", "system.uc")); err == nil {
			t.Fatalf("setup failed: %s, above the working directory %s outside the repository, holds a design/system.uc", d, dir)
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

// isOutside tells whether the working directory at, relative to the root, lies outside the repository.
func isOutside(at string) bool {
	return at == ".." || strings.HasPrefix(at, "../")
}

// relative is the path p as seen from the directory dir.
func relative(t rapid.TB, dir, p string) string {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		t.Fatalf("setup failed: cannot write %s relative to %s: %v", p, dir, err)
	}
	return rel
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

// writeRepo writes the files into a new repository, each with its own mode,
// and returns its root. The root is alone in a temporary directory, so that
// the directory above it holds nothing else.
func writeRepo(t rapid.TB, files map[string]RepoFile) string {
	base, err := os.MkdirTemp("", "unicode-repo-")
	if err != nil {
		t.Fatalf("setup failed: cannot create a temporary directory: %v", err)
	}
	root := filepath.Join(base, "repo")
	for path, f := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("setup failed: cannot create a directory: %v", err)
		}
		if isSymlink(f) {
			if err := os.Symlink(f.Content, full); err != nil {
				t.Fatalf("setup failed: cannot link %s: %v", path, err)
			}
			continue
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			t.Fatalf("setup failed: cannot write %s: %v", path, err)
		}
		if err := os.Chmod(full, os.FileMode(f.Mode)); err != nil {
			t.Fatalf("setup failed: cannot set the mode of %s: %v", path, err)
		}
	}
	return root
}

// readRepo reads the files of a repository without following symbolic
// links; a file that no one may read is read with its owner's read bit set
// for the moment.
func readRepo(t rapid.TB, root string) map[string]RepoFile {
	files := map[string]RepoFile{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		f := RepoFile{Mode: int(info.Mode() & (os.ModeSymlink | os.ModePerm))}
		var content []byte
		switch {
		case isSymlink(f):
			f.Content, err = os.Readlink(p)
		case info.Mode()&0o400 == 0:
			content, err = readUnreadable(p, info.Mode())
			f.Content = string(content)
		default:
			content, err = os.ReadFile(p)
			f.Content = string(content)
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = f
		return err
	})
	if err != nil {
		t.Fatalf("cannot read the repository's files: %v", err)
	}
	return files
}

// readUnreadable reads the file at p, whose mode has no owner's read bit, and restores the mode.
func readUnreadable(p string, mode os.FileMode) ([]byte, error) {
	if err := os.Chmod(p, mode|0o400); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(p)
	if err := os.Chmod(p, mode); err != nil {
		return nil, err
	}
	return content, err
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
func checkFlowFormatFiles(t rapid.TB, files map[string]RepoFile, check bool, targets []string, at string, explicitRoot bool) {
	if nested := slices.IndexFunc(slices.Sorted(maps.Keys(files)), func(p string) bool {
		return strings.HasSuffix("/"+p, "/design/system.uc") && p != "design/system.uc"
	}); nested >= 0 {
		t.Fatalf("setup failed: the files hold %s, which makes a second repository inside the first", slices.Sorted(maps.Keys(files))[nested])
	}
	flags := []string{"fmt"}
	if check {
		flags = append(flags, "--check")
	}
	jsonRoot, textRoot := writeRepo(t, files), writeRepo(t, files)
	defer removeAll(t, filepath.Dir(jsonRoot))
	defer removeAll(t, filepath.Dir(textRoot))
	// run runs fmt on the repository at root from the working directory at,
	// with the root and the path arguments written relative to it.
	run := func(root string, options ...string) (int, string, string) {
		dir := workDir(t, root, at)
		var args []string
		if explicitRoot {
			args = append(args, "--root", relative(t, dir, root))
		}
		args = append(append(args, options...), flags...)
		for _, target := range targets {
			args = append(args, relative(t, dir, filepath.Join(root, filepath.FromSlash(target))))
		}
		return runIn(t, dir, args)
	}

	usage := ""
	switch stray := strayTarget(files, targets); {
	case !explicitRoot && isOutside(at):
		usage = "run in " + at + ", outside the repository, without --root"
	case stray != "":
		usage = "with the path argument " + stray
	}
	if usage != "" {
		code, stdout, stderr := run(jsonRoot, "--format", "json")
		switch {
		case code != exitUsage:
			t.Fatalf("%s, unicode fmt exited with %d instead of %d\nstdout: %s\nstderr: %s", usage, code, exitUsage, stdout, stderr)
		case stdout != "" || stderr == "":
			t.Fatalf("%s, stdout should be empty and stderr should give the usage error, yet\nstdout: %s\nstderr: %s", usage, stdout, stderr)
		case !maps.Equal(readRepo(t, jsonRoot), files):
			t.Fatalf("%s, unicode fmt rejected its arguments, yet it changed the repository", usage)
		}
		return
	}

	var paths []string
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if under(p, targets) {
			paths = append(paths, p)
		}
	}
	want := map[string]expectation{}
	wantCode := exitOK
	for _, p := range paths {
		want[p] = expected(p, files[p], readable(filepath.Join(jsonRoot, filepath.FromSlash(p))), check)
		if len(want[p].codes) > 0 {
			wantCode = exitDiagnostics
		}
	}

	code, stdout, stderr := run(jsonRoot, "--format", "json")
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
		t.Fatalf("the report lists %d files, the path arguments %v hold %d: %v", len(report.Files), targets, len(paths), report.Files)
	}
	diagnosed := map[string][]string{}
	for _, d := range report.Diagnostics {
		if d.Code == "" || d.Message == "" || d.Fix == "" || d.Line < 1 || d.Column < 1 {
			t.Fatalf("a diagnostic lacks its code, cause, fix or position: %+v", d)
		}
		if !slices.Contains(paths, d.Path) {
			t.Fatalf("the diagnostic %s is about %s, which is not under the path arguments %v", d.Code, d.Path, targets)
		}
		diagnosed[d.Path] = append(diagnosed[d.Path], d.Code)
		if d.Code != "cli.not-canonical" {
			continue
		}
		if line, column := differsAt(files[d.Path].Content, string(want[d.Path].canonical)); d.Line != line || d.Column != column {
			t.Fatalf("the cli.not-canonical of %s stands at %d:%d, yet the first byte that differs from the canonical form is at %d:%d", d.Path, d.Line, d.Column, line, column)
		}
	}
	for i, f := range report.Files {
		p := paths[i]
		switch {
		case f.Path != p || f.Status != want[p].status:
			t.Fatalf("file %d of the report is %s (%s) instead of %s (%s)", i+1, f.Path, f.Status, p, want[p].status)
		case !slices.Equal(diagnosed[p], want[p].codes):
			t.Fatalf("%s (%s) should have the diagnostics %v, yet it has %v", p, want[p].status, want[p].codes, diagnosed[p])
		}
	}
	after := readRepo(t, jsonRoot)
	if len(after) != len(files) {
		t.Fatalf("after unicode fmt the repository has %d files, before it had %d", len(after), len(files))
	}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		before, now := files[p], after[p]
		rewritten := want[p].status == statusRewritten
		switch {
		case now.Mode != before.Mode:
			t.Fatalf("after unicode fmt the mode of %s is %v, before it was %v", p, os.FileMode(now.Mode), os.FileMode(before.Mode))
		case isSymlink(before) && now != before:
			t.Fatalf("unicode fmt changed the symbolic link %s: it pointed to %s, and now to %s", p, before.Content, now.Content)
		case !rewritten && now != before:
			t.Fatalf("%s should not be rewritten (--check %v, status %q), yet it was", p, check, want[p].status)
		case rewritten && now.Content != string(want[p].canonical):
			t.Fatalf("the valid file %s is not canonical after unicode fmt\nafter:\n%s\ncanonical:\n%s", p, now.Content, want[p].canonical)
		}
	}

	code, stdout, stderr = run(textRoot)
	if code != wantCode {
		t.Fatalf("with --format text, unicode fmt exited with %d instead of %d", code, wantCode)
	}
	var listed []string
	for _, p := range paths {
		if want[p].status == statusRewritten || want[p].status == statusNotCanonical {
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
		files := genFiles(fileModes).Draw(t, "files")
		checkFlowFormatFiles(t, files, rapid.Bool().Draw(t, "check"), genTargets(files).Draw(t, "targets"),
			genAt(files).Draw(t, "at"), rapid.Bool().Draw(t, "explicitRoot"))
	})
}

func FuzzFlowFormatFiles(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		files := genFiles(fileModes).Draw(t, "files")
		checkFlowFormatFiles(t, files, rapid.Bool().Draw(t, "check"), genTargets(files).Draw(t, "targets"),
			genAt(files).Draw(t, "at"), rapid.Bool().Draw(t, "explicitRoot"))
	}))
}

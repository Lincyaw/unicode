package golang

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
)

// goInputKey is the cache key of one load of the Go code under the source
// roots of root, in the given mode, with overlay (absolute path to content)
// replacing or adding files: the SHA-256 of the executable that loads, the
// go command's version and build settings, the kind of each source root, and
// the path and content of every file the go command can read for the load.
// Paths are relative to root, so the same code at another place has the same
// key. It returns false when an input cannot be read; the load then goes
// uncached.
func goInputKey(root string, srcRoots []string, mode string, overlay map[string][]byte) (string, bool) {
	exe, ok := executableDigest()
	if !ok {
		return "", false
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	h := sha256.New()
	record := func(fields ...string) {
		for _, f := range fields {
			_, _ = fmt.Fprintf(h, "%d:%s", len(f), f)
		}
		_, _ = h.Write([]byte{'\n'})
	}
	record("unicode-go-input", "1", mode, exe)
	record("root", fileKind(abs))
	for _, r := range srcRoots {
		record("source-root", r, fileKind(filepath.Join(abs, r)), fmt.Sprint(filepath.IsLocal(r) && ordinarySourceRoot(abs, r)))
	}
	env, ok := goEnvironment(abs)
	if !ok {
		return "", false
	}
	files := map[string]string{}
	for _, name := range goEnvironmentNames {
		record("env", name, env[name])
	}
	for _, name := range []string{"GOMOD", "GOWORK"} {
		location := env[name]
		if location == "" || location == "off" || location == os.DevNull {
			record("env", name, location)
			continue
		}
		rel, err := filepath.Rel(abs, location)
		if err != nil {
			return "", false
		}
		record("env", name, filepath.ToSlash(rel))
		dir := filepath.Dir(location)
		sums := map[string]string{"GOMOD": "go.sum", "GOWORK": "go.work.sum"}
		for _, file := range []string{location, filepath.Join(dir, sums[name])} {
			if err := addInputFile(files, abs, file); err != nil {
				return "", false
			}
		}
		for _, linked := range linkedModules(name, location) {
			if err := walkGoInputs(files, abs, linked); err != nil {
				return "", false
			}
		}
	}
	if err := walkGoInputs(files, abs, abs); err != nil {
		return "", false
	}
	for name, content := range overlay {
		rel, err := filepath.Rel(abs, name)
		if err != nil {
			return "", false
		}
		sum := sha256.Sum256(content)
		files[filepath.ToSlash(rel)] = "overlay " + hex.EncodeToString(sum[:])
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		record("file", name, files[name])
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// executableDigest is the SHA-256 of the running executable, so that a
// change to any code of the loader changes every key.
var executableDigest = sync.OnceValues(func() (string, bool) {
	name, err := os.Executable()
	if err != nil {
		return "", false
	}
	f, err := os.Open(name)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
})

// goEnvironmentNames are the go command's settings that change which files
// a load reads or how it type-checks them. GOMOD and GOWORK, which are
// paths, enter the key relative to the root.
var goEnvironmentNames = []string{
	"GOVERSION", "GOROOT", "GOOS", "GOARCH", "GOFLAGS", "GOEXPERIMENT", "GOTOOLCHAIN", "GO111MODULE",
	"GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM",
	"GOMODCACHE", "CGO_ENABLED", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CC", "CXX",
}

// goEnvironment asks the go command for its settings as it runs in dir,
// after it has chosen the toolchain that dir's module asks for.
func goEnvironment(dir string) (map[string]string, bool) {
	cmd := exec.Command("go", append([]string{"env", "-json"}, append(slices.Clone(goEnvironmentNames), "GOMOD", "GOWORK")...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	env := map[string]string{}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, false
	}
	return env, true
}

// linkedModules are the directories whose packages a module or workspace
// file brings in from the file system: the local replacements of go.mod, and
// the used modules of go.work.
func linkedModules(name, location string) []string {
	data, err := os.ReadFile(location)
	if err != nil {
		return nil
	}
	dir := filepath.Dir(location)
	var dirs []string
	local := func(p string) {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		dirs = append(dirs, p)
	}
	if name == "GOMOD" {
		f, err := modfile.ParseLax(location, data, nil)
		if err != nil {
			return nil
		}
		for _, r := range f.Replace {
			if r.New.Version == "" {
				local(r.New.Path)
			}
		}
		return dirs
	}
	f, err := modfile.ParseWork(location, data, nil)
	if err != nil {
		return nil
	}
	for _, u := range f.Use {
		local(u.Path)
	}
	return dirs
}

// goSourceExtensions are the file extensions go/build reads into a package.
var goSourceExtensions = []string{
	".go", ".c", ".cc", ".cxx", ".cpp", ".m", ".h", ".hh", ".hpp", ".hxx", ".f", ".F", ".for", ".f90",
	".s", ".S", ".sx", ".swig", ".swigcxx", ".syso",
}

// walkGoInputs adds to files every file under start that the go command can
// read for a package pattern: it skips the directories that `./...` skips
// (names starting with `.` or `_`, testdata, and nested modules, which it
// marks), never follows a directory link, and keeps the files whose
// extension go/build reads, the module and workspace files, and vendor's
// modules.txt.
func walkGoInputs(files map[string]string, root, start string) error {
	return filepath.WalkDir(start, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if name == start {
				return nil
			}
			base := entry.Name()
			if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || base == "testdata" {
				return filepath.SkipDir
			}
			if _, err := os.Lstat(filepath.Join(name, "go.mod")); err == nil {
				files[rel] = "module"
				return filepath.SkipDir
			}
			return nil
		}
		base := entry.Name()
		vendored := base == "modules.txt" && path.Base(path.Dir(rel)) == "vendor"
		if !vendored && !slices.Contains(goSourceExtensions, filepath.Ext(base)) && !slices.Contains([]string{"go.mod", "go.sum", "go.work", "go.work.sum"}, base) {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(name); err == nil && info.IsDir() {
				files[rel] = "directory link"
				return nil
			}
		}
		return addInputFile(files, root, name)
	})
}

// addInputFile adds the file at name to files, keyed by its path relative
// to root and valued by its kind and content: a link is read through, a
// missing file is recorded as missing.
func addInputFile(files map[string]string, root, name string) error {
	rel, err := filepath.Rel(root, name)
	if err != nil {
		return err
	}
	kind := fileKind(name)
	data, err := os.ReadFile(name)
	if err != nil {
		if os.IsNotExist(err) {
			files[filepath.ToSlash(rel)] = kind
			return nil
		}
		return err
	}
	sum := sha256.Sum256(data)
	files[filepath.ToSlash(rel)] = kind + " " + hex.EncodeToString(sum[:])
	return nil
}

// fileKind names what is at name without following a link.
func fileKind(name string) string {
	info, err := os.Lstat(name)
	switch {
	case err != nil:
		return "missing"
	case info.Mode()&fs.ModeSymlink != 0:
		return "link"
	case info.IsDir():
		return "directory"
	}
	return "file"
}

package golang

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
)

// inputSet is what one load read from the file system, outside the module
// cache and GOROOT, whose content never changes: for each package the go
// command listed, the names in its directory and every file it reads
// (sources, ignored and embedded files), and the go.mod and go.sum of its
// module. Keys are "dir:" or "file:" with a slash path relative to the root,
// which may leave it ("../sibling"); values are SHA-256 digests or
// "missing". A cache entry carries the set its result was computed from, so
// that a hit is answered only while every one of them is unchanged.
type inputSet map[string]string

// goListedPackage holds the fields of go list -json that name what a
// package reads.
type goListedPackage struct {
	Dir, ImportPath string
	Standard        bool
	Module          *struct {
		Version, Dir, GoMod string
		Replace             *struct{ Version, Dir, GoMod string }
	}
	GoFiles, CgoFiles, IgnoredGoFiles, IgnoredOtherFiles, CFiles, CXXFiles, MFiles, HFiles, FFiles []string
	SFiles, SwigFiles, SwigCXXFiles, SysoFiles, TestGoFiles, XTestGoFiles                          []string
	EmbedFiles, TestEmbedFiles, XTestEmbedFiles                                                    []string
}

// goListFields are the fields goListedPackage reads.
const goListFields = "Dir,ImportPath,Standard,Module,GoFiles,CgoFiles,IgnoredGoFiles,IgnoredOtherFiles,CFiles,CXXFiles,MFiles,HFiles,FFiles," +
	"SFiles,SwigFiles,SwigCXXFiles,SysoFiles,TestGoFiles,XTestGoFiles,EmbedFiles,TestEmbedFiles,XTestEmbedFiles"

// readInputs lists, without compiling anything, the packages a load of the
// source roots reads, their dependencies and tests included, and the
// packages that the import paths in extra name, and records their inputs.
func readInputs(root string, srcRoots []string, forGeneration bool, extra []string) (inputSet, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	args := []string{"list", "-e", "-deps", "-test", "-json=" + goListFields, "-buildvcs=false"}
	if forGeneration {
		args = append(args, "-mod=readonly")
	}
	var patterns []string
	for _, r := range srcRoots {
		if info, err := os.Stat(filepath.Join(abs, r)); err != nil || !info.IsDir() {
			continue
		}
		if rel := path.Clean(filepath.ToSlash(r)); rel == "." {
			patterns = append(patterns, "./...")
		} else {
			patterns = append(patterns, "./"+rel+"/...")
		}
	}
	patterns = append(patterns, extra...)
	inputs := inputSet{}
	if len(patterns) == 0 {
		return inputs, nil
	}
	env, ok := goEnvironment(abs)
	if !ok {
		return nil, errors.New("go env failed")
	}
	cmd := exec.Command("go", append(args, patterns...)...)
	cmd.Dir = abs
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	immutable := []string{env["GOROOT"], env["GOMODCACHE"]}
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var p goListedPackage
		if err := decoder.Decode(&p); err != nil {
			return nil, err
		}
		if p.Standard || p.Dir == "" || strings.HasSuffix(p.ImportPath, ".test") || slices.ContainsFunc(immutable, func(d string) bool { return d != "" && within(d, p.Dir) }) {
			continue
		}
		if err := inputs.addDir(abs, p.Dir); err != nil {
			return nil, err
		}
		for _, files := range [][]string{p.GoFiles, p.CgoFiles, p.IgnoredGoFiles, p.IgnoredOtherFiles, p.CFiles, p.CXXFiles, p.MFiles, p.HFiles, p.FFiles, p.SFiles, p.SwigFiles, p.SwigCXXFiles, p.SysoFiles, p.TestGoFiles, p.XTestGoFiles, p.EmbedFiles, p.TestEmbedFiles, p.XTestEmbedFiles} {
			for _, f := range files {
				if filepath.IsAbs(f) {
					continue
				}
				if err := inputs.addFile(abs, filepath.Join(p.Dir, f)); err != nil {
					return nil, err
				}
			}
		}
		if m := p.Module; m != nil {
			goMod, dir := m.GoMod, m.Dir
			if m.Replace != nil {
				goMod, dir = m.Replace.GoMod, m.Replace.Dir
			}
			for _, f := range []string{goMod, filepath.Join(dir, "go.sum")} {
				if f != "" && dir != "" && !slices.ContainsFunc(immutable, func(d string) bool { return d != "" && within(d, f) }) {
					if err := inputs.addFile(abs, f); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	for _, name := range []string{"GOWORK"} {
		if w := env[name]; w != "" && w != "off" {
			for _, f := range []string{w, w + ".sum"} {
				if err := inputs.addFile(abs, f); err != nil {
					return nil, err
				}
			}
		}
	}
	return inputs, nil
}

// within tells whether name lies in dir.
func within(dir, name string) bool {
	rel, err := filepath.Rel(dir, name)
	return err == nil && filepath.IsLocal(rel)
}

func (s inputSet) addDir(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return err
	}
	s["dir:"+filepath.ToSlash(rel)] = dirDigest(dir)
	return nil
}

func (s inputSet) addFile(root, name string) error {
	rel, err := filepath.Rel(root, name)
	if err != nil {
		return err
	}
	s["file:"+filepath.ToSlash(rel)] = fileDigest(name)
	return nil
}

// current tells whether every input still has its recorded digest, with
// paths taken relative to root and the files of overlay (absolute path to
// content) read as if they were written.
func (s inputSet) current(root string, overlay map[string][]byte) bool {
	return maps.Equal(s, s.recomputed(root, overlay))
}

// recomputed is the set with the same keys, digested afresh.
func (s inputSet) recomputed(root string, overlay map[string][]byte) inputSet {
	out := make(inputSet, len(s))
	for key := range s {
		kind, rel, ok := strings.Cut(key, ":")
		if !ok {
			out[key] = "invalid"
			continue
		}
		name := filepath.Join(root, filepath.FromSlash(rel))
		if kind == "dir" {
			var extra []string
			for path := range overlay {
				if filepath.Dir(path) == name {
					extra = append(extra, filepath.Base(path))
				}
			}
			out[key] = dirDigest(name, extra...)
		} else if content, ok := overlay[name]; ok {
			sum := sha256.Sum256(content)
			out[key] = hex.EncodeToString(sum[:])
		} else {
			out[key] = fileDigest(name)
		}
	}
	return out
}

// fileDigest is the SHA-256 of the file at name, read through links, or
// "missing" (or "unreadable") when it cannot be read.
func fileDigest(name string) string {
	f, err := os.Open(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "missing"
		}
		return "unreadable"
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unreadable"
	}
	return hex.EncodeToString(h.Sum(nil))
}

// dirDigest is the SHA-256 of the names and kinds of the entries of the
// directory at name, extra counted as regular files in it, or "missing".
func dirDigest(name string, extra ...string) string {
	entries, err := os.ReadDir(name)
	if err != nil && len(extra) == 0 {
		return "missing"
	}
	kinds := map[string]fs.FileMode{}
	for _, e := range entries {
		kinds[e.Name()] = e.Type()
	}
	for _, x := range extra {
		kinds[x] = 0
	}
	h := sha256.New()
	for _, n := range slices.Sorted(maps.Keys(kinds)) {
		_, _ = fmt.Fprintf(h, "%s\x00%v\n", n, kinds[n])
	}
	return hex.EncodeToString(h.Sum(nil))
}

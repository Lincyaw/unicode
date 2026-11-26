package golang

import (
	"errors"
	"fmt"
	"go/ast"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedForTest

// loaded is the code under the source roots: one variant of each package,
// sorted by package path.
type loaded struct {
	root     string // absolute
	packages []*packages.Package
}

// load loads the packages under the source roots, which are relative to
// root, with their test files. A package with test files comes in several
// variants; the one compiled for its own tests holds all its files, and it is
// the one kept.
func load(root string, srcRoots []string) (loaded, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return loaded{}, err
	}
	var patterns []string
	for _, r := range srcRoots {
		if !filepath.IsLocal(r) {
			return loaded{}, fmt.Errorf("the source root %q is not a relative path inside the root", r)
		}
		if info, err := os.Stat(filepath.Join(abs, r)); err != nil || !info.IsDir() {
			return loaded{}, fmt.Errorf("the source root %q is not a directory", r)
		}
		if rel := path.Clean(filepath.ToSlash(r)); rel == "." {
			patterns = append(patterns, "./...")
		} else {
			patterns = append(patterns, "./"+rel+"/...")
		}
	}
	if len(patterns) == 0 {
		return loaded{root: abs}, nil
	}
	all, err := packages.Load(&packages.Config{Mode: loadMode, Dir: abs, Tests: true}, patterns...)
	if err != nil {
		return loaded{}, err
	}
	kept := map[string]*packages.Package{}
	for _, p := range all {
		if len(filesUnder(abs, p)) == 0 {
			continue
		}
		if other, ok := kept[p.PkgPath]; !ok || moreFiles(p, other) {
			kept[p.PkgPath] = p
		}
	}
	var problems []error
	l := loaded{root: abs}
	for _, pkgPath := range slices.Sorted(maps.Keys(kept)) {
		p := kept[pkgPath]
		for _, e := range p.Errors {
			problems = append(problems, e)
		}
		l.packages = append(l.packages, p)
	}
	if len(problems) > 0 {
		return loaded{}, errors.Join(problems...)
	}
	return l, nil
}

// moreFiles tells whether p is the better variant of the package than other:
// it has more files, or as many and is not compiled for another package's tests.
func moreFiles(p, other *packages.Package) bool {
	if len(p.CompiledGoFiles) != len(other.CompiledGoFiles) {
		return len(p.CompiledGoFiles) > len(other.CompiledGoFiles)
	}
	return p.ID == p.PkgPath
}

// sourceFile is one file of a package under the root.
type sourceFile struct {
	rel    string // slash-separated, relative to the root
	syntax *ast.File
}

// filesUnder returns the files of p that lie under root, sorted by path.
func filesUnder(root string, p *packages.Package) []sourceFile {
	var files []sourceFile
	for _, f := range p.Syntax {
		name := p.Fset.File(f.Pos()).Name()
		rel, err := filepath.Rel(root, name)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		files = append(files, sourceFile{rel: filepath.ToSlash(rel), syntax: f})
	}
	slices.SortFunc(files, func(a, b sourceFile) int { return strings.Compare(a.rel, b.rel) })
	return files
}

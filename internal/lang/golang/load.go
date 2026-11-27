package golang

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
	return loadPackages(root, srcRoots, false)
}

// loadForGeneration allows stale generated tests to be replaced without first
// requiring them to compile. All handwritten code still has to load.
func loadForGeneration(root string, srcRoots []string) (loaded, error) {
	return loadPackages(root, srcRoots, true)
}

func loadPackages(root string, srcRoots []string, forGeneration bool) (loaded, error) {
	return loadPackagesOverlay(root, srcRoots, forGeneration, nil)
}

func loadPackagesOverlay(root string, srcRoots []string, forGeneration bool, extra map[string][]byte) (loaded, error) {
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
			if forGeneration && errors.Is(err, os.ErrNotExist) {
				continue
			}
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
	// Extraction needs no version control stamps; making them fails in a
	// checkout whose git status cannot be read.
	cfg := &packages.Config{Mode: loadMode, Dir: abs, Tests: true, BuildFlags: []string{"-buildvcs=false"}}
	if forGeneration {
		cfg.Overlay, err = generatedTestOverlay(abs, srcRoots)
		if err != nil {
			return loaded{}, err
		}
	}
	if len(extra) > 0 {
		if cfg.Overlay == nil {
			cfg.Overlay = map[string][]byte{}
		}
		for name, content := range extra {
			cfg.Overlay[name] = content
		}
	}
	all, err := packages.Load(cfg, patterns...)
	if err != nil {
		return loaded{}, err
	}
	return keepLoadedPackages(abs, all)
}

func keepLoadedPackages(abs string, all []*packages.Package) (loaded, error) {
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

// generatedTestOverlay reads only ordinary files within the configured roots.
// The overlay retains the package prefix, including build constraints and the
// ownership marker, while excluding declarations from old generated output.
func generatedTestOverlay(root string, srcRoots []string) (map[string][]byte, error) {
	overlay := map[string][]byte{}
	for _, srcRoot := range srcRoots {
		if !ordinarySourceRoot(root, srcRoot) {
			continue
		}
		start := filepath.Join(root, srcRoot)
		err := filepath.WalkDir(start, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if name != start && (strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || entry.Name() == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			source, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			set := token.NewFileSet()
			file, err := parser.ParseFile(set, name, source, parser.PackageClauseOnly|parser.ParseComments)
			if err != nil || !unicodeGenerated(file) {
				return nil
			}
			prefix := source[:set.Position(file.Package).Offset]
			empty := append([]byte{}, prefix...)
			empty = append(empty, "package "+file.Name.Name+"\n"...)
			if declarations, _ := parser.ParseFile(set, name, source, parser.ParseComments|parser.AllErrors); declarations != nil {
				empty = append(empty, retainGeneratedCarriers(declarations, source, set)...)
			}
			overlay[name] = empty
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return overlay, nil
}

// ordinarySourceRoot rejects symlinks in the configured relative path as well
// as a symlink used as the repository root. WalkDir never follows child links.
func ordinarySourceRoot(root, relative string) bool {
	name := root
	parts := append([]string{"."}, strings.Split(filepath.Clean(relative), string(filepath.Separator))...)
	for _, part := range parts {
		name = filepath.Join(name, part)
		info, err := os.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
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

package golang

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// loadWithSkeletons keeps every prospective declaration off the repository.
// Go's overlay cannot add directories; a temporary copy is needed only when a
// planned package directory does not exist yet.
func loadWithSkeletons(plan lang.GenPlan, edits []lang.Edit) (loaded, error) {
	return loadPlannedGo(plan, edits, true)
}

// checkPlannedGo tells whether the code loads once edits are written. A
// success is kept in the content cache under the inputs of the load with
// the edits in place, so the same planned code is type-checked once
// (docs/design.md §6.3); the paths of the edits are checked every time.
func checkPlannedGo(plan lang.GenPlan, edits []lang.Edit) error {
	const kind = cachePlanned
	abs, err := filepath.Abs(plan.Root)
	if err != nil {
		return err
	}
	if err := validatePlannedGoPaths(abs, edits); err != nil {
		return err
	}
	overlay := make(map[string][]byte, len(edits))
	for _, edit := range edits {
		overlay[filepath.Join(abs, filepath.FromSlash(edit.Path))] = edit.Content
	}
	cache := sharedCache()
	key, keyed := goInputKey(abs, plan.Roots, "planned-generation", overlay)
	if keyed {
		if _, hit := cache.get(kind, key); hit {
			return nil
		}
	}
	if _, err := loadPlannedGo(plan, edits, false); err != nil {
		return err
	}
	if after, ok := goInputKey(abs, plan.Roots, "planned-generation", overlay); keyed && ok && after == key {
		cache.put(kind, key, []byte("loads\n"))
	}
	return nil
}

func loadPlannedGo(plan lang.GenPlan, edits []lang.Edit, bootstrapCarriers bool) (loaded, error) {
	root, roots := plan.Root, plan.Roots
	abs, err := filepath.Abs(root)
	if err != nil {
		return loaded{}, err
	}
	if err := validatePlannedGoPaths(abs, edits); err != nil {
		return loaded{}, err
	}
	extra := map[string][]byte{}
	mirror := false
	for _, edit := range edits {
		name := filepath.Join(abs, filepath.FromSlash(edit.Path))
		extra[name] = edit.Content
		if _, err := os.Stat(filepath.Dir(name)); errors.Is(err, os.ErrNotExist) {
			mirror = true
		} else if err != nil {
			return loaded{}, err
		}
	}
	if bootstrapCarriers {
		if err := addSkeletonCarriers(plan, edits, extra); err != nil {
			return loaded{}, err
		}
	}
	if !mirror {
		return loadPackagesOverlay(root, roots, true, extra)
	}
	temp, err := os.MkdirTemp("", "unicode-skeleton-load-")
	if err != nil {
		return loaded{}, err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	if err = copySkeletonModule(abs, temp); err != nil {
		return loaded{}, err
	}
	overlay, err := generatedTestOverlay(abs, roots)
	if err != nil {
		return loaded{}, err
	}
	for name, content := range extra {
		overlay[name] = content
	}
	for name, content := range overlay {
		rel, err := filepath.Rel(abs, name)
		if err != nil || !filepath.IsLocal(rel) {
			return loaded{}, fmt.Errorf("overlay path %q is outside the repository", name)
		}
		target := filepath.Join(temp, rel)
		if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return loaded{}, err
		}
		if err = os.WriteFile(target, content, 0o600); err != nil {
			return loaded{}, err
		}
	}
	if err = rewriteSkeletonModules(abs, temp); err != nil {
		return loaded{}, err
	}
	return loadSkeletonMirror(abs, temp, roots)
}
func copySkeletonModule(root, temp string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(temp, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		regular := entry.Type().IsRegular()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(name)
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Directory links are retained for read-only package imports.
				// Resolve their original destination before moving the link.
				resolved, err := filepath.EvalSymlinks(name)
				if err != nil {
					return err
				}
				return os.Symlink(resolved, target)
			}
			regular = info.Mode().IsRegular()
		}
		if !regular {
			return fmt.Errorf("cannot copy nonregular module entry %s for skeleton loading", rel)
		}
		src, err := os.Open(name)
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return errors.Join(err, src.Close())
		}
		_, err = io.Copy(dst, src)
		return errors.Join(err, src.Close(), dst.Close())
	})
}

// Relative local replacements keep referring to their original directories,
// including modules outside the repository. The source go.mod is unchanged.
func rewriteSkeletonModules(root, temp string) error {
	// Replacement directives are effective only in main modules. Unrelated
	// go.mod assets and replacement dependencies must stay untouched.
	directories := map[string]bool{".": true}
	_, workspace, err := skeletonWorkspace(root)
	if err != nil {
		return err
	}
	if workspace != nil {
		for _, use := range workspace.file.Use {
			dir := use.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(filepath.Dir(workspace.name), dir)
			}
			if relative, ok := skeletonRelative(root, dir); ok {
				directories[relative] = true
			}
		}
	}
	for _, rel := range slices.Sorted(maps.Keys(directories)) {
		name := filepath.Join(temp, rel, "go.mod")
		if err := validatePlannedGoPaths(temp, []lang.Edit{{Path: filepath.ToSlash(filepath.Join(rel, "go.mod"))}}); err != nil {
			return err
		}
		source, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		file, err := modfile.Parse(name, source, nil)
		if err != nil {
			return err
		}
		for _, replacement := range file.Replace {
			if replacement.New.Version != "" || filepath.IsAbs(replacement.New.Path) {
				continue
			}
			original := filepath.Clean(filepath.Join(root, rel, replacement.New.Path))
			if within, ok := skeletonRelative(root, original); ok {
				original = filepath.Join(temp, within)
			}
			if err = file.AddReplace(replacement.Old.Path, replacement.Old.Version, original, ""); err != nil {
				return err
			}
		}
		out, err := file.Format()
		if err != nil {
			return err
		}
		if err = os.WriteFile(name, out, 0o600); err != nil {
			return err
		}
	}
	return nil
}

type skeletonWork struct {
	name string
	file *modfile.WorkFile
}

func skeletonWorkspace(root string) (string, *skeletonWork, error) {
	command := exec.Command("go", "env", "GOWORK")
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		return "", nil, err
	}
	name := strings.TrimSpace(string(data))
	if name == "" || name == "off" {
		return name, nil, nil
	}
	source, err := os.ReadFile(name)
	if err != nil {
		return name, nil, err
	}
	file, err := modfile.ParseWork(name, source, nil)
	if err != nil {
		return name, nil, err
	}
	return name, &skeletonWork{name: name, file: file}, nil
}
func skeletonRelative(root, dir string) (string, bool) {
	// Missing replacement directories are resolved lexically; existing use
	// modules are resolved through links so no copied directory link is written.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	rel, err := filepath.Rel(root, dir)
	return rel, err == nil && (rel == "." || filepath.IsLocal(rel))
}

func loadSkeletonMirror(root, temp string, roots []string) (loaded, error) {
	patterns := []string{}
	for _, r := range roots {
		if !filepath.IsLocal(r) {
			return loaded{}, fmt.Errorf("invalid source root %q", r)
		}
		if _, err := os.Stat(filepath.Join(temp, r)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return loaded{}, err
		}
		patterns = append(patterns, "./"+filepath.ToSlash(filepath.Clean(r))+"/...")
	}
	if len(patterns) == 0 {
		return loaded{root: root}, nil
	}
	env := os.Environ()
	// A parent workspace must include the temporary copy instead of the
	// original module. Preserve its other modules and explicit replacements.
	_, workspace, err := skeletonWorkspace(root)
	if err != nil {
		return loaded{}, err
	}
	env = append(env, "GOWORK=off")
	if workspace != nil {
		parsed := workspace.file
		for _, use := range parsed.Use {
			dir := use.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(filepath.Dir(workspace.name), dir)
			}
			if relative, ok := skeletonRelative(root, dir); ok {
				dir = filepath.Join(temp, relative)
			}
			if err = parsed.AddUse(dir, ""); err != nil {
				return loaded{}, err
			}
			if use.Path != dir {
				if err = parsed.DropUse(use.Path); err != nil {
					return loaded{}, err
				}
			}
		}
		for _, replacement := range parsed.Replace {
			if replacement.New.Version == "" && !filepath.IsAbs(replacement.New.Path) {
				absolute := filepath.Join(filepath.Dir(workspace.name), replacement.New.Path)
				if relative, ok := skeletonRelative(root, absolute); ok {
					absolute = filepath.Join(temp, relative)
				}
				if err = parsed.AddReplace(replacement.Old.Path, replacement.Old.Version, absolute, ""); err != nil {
					return loaded{}, err
				}
			}
		}
		file, err := os.CreateTemp(temp, "unicode-skeleton-*.go.work")
		if err != nil {
			return loaded{}, err
		}
		_, writeErr := file.Write(modfile.Format(parsed.Syntax))
		if err = errors.Join(writeErr, file.Close()); err != nil {
			return loaded{}, err
		}
		env = append(env, "GOWORK="+file.Name())
	}

	cfg := &packages.Config{Mode: loadMode, Dir: temp, Tests: true, Env: env, BuildFlags: []string{"-buildvcs=false", "-mod=readonly"}}
	cfg.ParseFile = func(set *token.FileSet, name string, source []byte) (*ast.File, error) {
		rel, err := filepath.Rel(temp, name)
		if err == nil && filepath.IsLocal(rel) {
			name = filepath.Join(root, rel)
		}
		return parseFile(set, name, source)
	}
	all, err := packages.Load(cfg, patterns...)
	if err != nil {
		return loaded{}, err
	}
	return keepLoadedPackages(root, all)
}

// addSkeletonCarriers makes fresh custom generators and manual predicates
// loadable before the first owned driver is written. Only validated carriers
// from the previous owned output are retained beside the new declarations.
func addSkeletonCarriers(plan lang.GenPlan, edits []lang.Edit, extra map[string][]byte) error {
	root, err := filepath.Abs(plan.Root)
	if err != nil {
		return err
	}
	owned, err := generatedTestOverlay(root, plan.Roots)
	if err != nil {
		return err
	}
	typesByDirectory := map[string]map[string]bool{}
	packagesByDirectory := map[string]string{}
	for _, node := range plan.Graph.Nodes() {
		if node.Layer != graph.LayerDesign || (node.Kind != graph.NodeKindProperty && node.Kind != graph.NodeKindFlow) {
			continue
		}
		for _, edit := range edits {
			if !strings.HasSuffix(edit.Path, "_test.go") || !strings.Contains(string(edit.Content), "//uc:checks "+node.Id+"\n") && !strings.Contains(string(edit.Content), "//uc:generates "+node.Id+".") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), edit.Path, edit.Content, parser.PackageClauseOnly)
			if err != nil {
				return err
			}
			dir := filepath.Dir(edit.Path)
			packagesByDirectory[dir] = parsed.Name.Name
			if typesByDirectory[dir] == nil {
				typesByDirectory[dir] = map[string]bool{}
			}
			var collect func(string)
			collect = func(model string) {
				kind, args := ucSplitType(model)
				if kind == "call" || kind == "replies" {
					typesByDirectory[dir][model] = true
					return
				}
				for _, arg := range args {
					collect(arg)
				}
			}
			for _, symbol := range node.Attrs["symbols"] {
				_, model, _ := strings.Cut(symbol, ": ")
				collect(strings.TrimSuffix(model, " generated"))
			}
		}
	}
	for dir, models := range typesByDirectory {
		if len(models) == 0 {
			continue
		}
		name := filepath.Join(root, dir, "unicode_contracts_gen_test.go")
		source, exists := owned[name]
		if !exists {
			if _, err := os.Lstat(name); err == nil {
				return fmt.Errorf("the prospective carrier file %s is not owned by unicode", name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			source = []byte(unicodeGeneratedHeader + "\npackage " + packagesByDirectory[dir] + "\n")
		}
		for _, model := range slices.Sorted(maps.Keys(models)) {
			if !bytes.Contains(source, []byte("type "+modelCarrierName(model)+" ")) {
				source = append(source, modelCarrierDeclaration(model)...)
			}
		}
		extra[name] = source
	}
	return nil
}

// validatePlannedGoPaths runs before any temporary mirror writes. Copying a
// source symlink is harmless, but no planned path may traverse one: otherwise
// writing through a copied absolute link could change the original repository.
func validatePlannedGoPaths(root string, edits []lang.Edit) error {
	for _, edit := range edits {
		if !filepath.IsLocal(edit.Path) || edit.Path == "." || strings.ContainsAny(edit.Path, "\\\x00") || path.Clean(edit.Path) != edit.Path {
			return fmt.Errorf("the planned path %q is not a clean relative repository file", edit.Path)
		}
		parts := strings.Split(edit.Path, "/")
		name := root
		for i := -1; i < len(parts); i++ {
			if i >= 0 {
				name = filepath.Join(name, parts[i])
			}
			info, err := os.Lstat(name)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("the planned path %q traverses a symbolic link at %s", edit.Path, name)
			}
			if i < len(parts)-1 && !info.IsDir() {
				return fmt.Errorf("the planned path %q has a non-directory parent %s", edit.Path, name)
			}
			if i == len(parts)-1 && !info.Mode().IsRegular() {
				return fmt.Errorf("the planned target %q is not a regular file", edit.Path)
			}
		}
	}
	return nil
}

package golang

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// The code outside the source roots that the code calls, the packages of
// dependency modules and the main modules' other packages (go.work modules
// included), is walked by the same rules as the code layer (docs/design.md
// §4): it is extracted the same way, functions, methods, interfaces, calls
// and the closures package-level variables lend, its functions get their
// direct effects from the table, and D8 gives them their effects, interface
// dispatch included. What that gives, the effects of every function there
// and the diagnostics of packages the table does not classify, is kept in
// the content cache (cache.go) as one entry, under a key that holds the
// unicode executable, the go command's settings, and every walked package:
// its module and version when the module is versioned, the content of its
// files otherwise.

// walkedResult is the cached form of what the walk gives.
type walkedResult struct {
	Effects     map[string][]string
	Diagnostics []cachedDiagnostic
}

// outsideEffects gives the effects of every function of the walked packages
// among known, and the diagnostics of the standard library packages they
// call that the table does not classify.
func outsideEffects(l loaded, known map[string]*packages.Package) (map[string]effectMask, []diag.Diagnostic, error) {
	roots := map[string]bool{}
	for _, p := range l.packages {
		roots[p.PkgPath] = true
	}
	var paths []string
	for _, path := range slices.Sorted(maps.Keys(known)) {
		if !roots[path] && place(known[path]) == walked {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return nil, nil, nil
	}
	cache, err := sharedCache()
	if err != nil {
		return nil, nil, err
	}
	key, keyed := walkKey(l.root, paths, known)
	var result walkedResult
	if data, hit, err := cache.get(cacheEffects, key); err != nil {
		return nil, nil, err
	} else if keyed && hit {
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, nil, cache.corrupt(cache.entryPath(cacheEffects, key), err)
		}
	} else {
		if result, err = walk(l.root, paths, known); err != nil {
			return nil, nil, err
		}
		if keyed {
			if data, err := json.Marshal(result); err == nil {
				cache.put(cacheEffects, key, data)
			}
		}
	}
	out := map[string]effectMask{}
	for id, kinds := range result.Effects {
		var m effectMask
		for _, k := range kinds {
			m |= mask(graph.Effect(k))
		}
		out[id] = m
	}
	return out, convertAll(result.Diagnostics, func(d cachedDiagnostic) diag.Diagnostic { return diag.Diagnostic(d) }), nil
}

// walkKey is the cache key of the walk of the packages, or false when an
// input cannot be read.
func walkKey(root string, paths []string, known map[string]*packages.Package) (string, bool) {
	exe, ok := executableDigest()
	if !ok {
		return "", false
	}
	env, ok := goEnvironment(root)
	if !ok {
		return "", false
	}
	h := sha256.New()
	record := func(fields ...string) {
		for _, f := range fields {
			_, _ = fmt.Fprintf(h, "%d:%s", len(f), f)
		}
		_, _ = h.Write([]byte{'\n'})
	}
	record("unicode-go-effects", exe)
	for _, name := range goEnvironmentNames {
		record("env", name, env[name])
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	for _, path := range paths {
		p := known[path]
		m := p.Module
		if m.Replace != nil {
			m = m.Replace
		}
		record("package", path, m.Path, m.Version)
		files := slices.Clone(p.GoFiles)
		slices.Sort(files)
		for _, f := range files {
			if m.Version != "" {
				// A module version's files never change; their names say
				// which ones the build selected.
				record("file", filepath.Base(f))
				continue
			}
			content, err := os.ReadFile(f)
			if err != nil {
				return "", false
			}
			rel, err := filepath.Rel(abs, f)
			if err != nil {
				rel = f
			}
			sum := sha256.Sum256(content)
			record("file", filepath.ToSlash(rel), hex.EncodeToString(sum[:]))
		}
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// walk loads the packages from their source, as the module at root selects
// them, extracts them as the code layer is extracted, and gives every
// function D8's effects.
func walk(root string, paths []string, known map[string]*packages.Package) (walkedResult, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedModule | packages.NeedImports,
		Dir: root, BuildFlags: []string{"-buildvcs=false"}, ParseFile: parseFile,
	}
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return walkedResult{}, err
	}
	var problems []error
	for _, p := range pkgs {
		for _, e := range p.Errors {
			problems = append(problems, e)
		}
	}
	if len(problems) > 0 {
		return walkedResult{}, errors.Join(problems...)
	}
	slices.SortFunc(pkgs, func(a, b *packages.Package) int { return strings.Compare(a.PkgPath, b.PkgPath) })
	// The walked files lie anywhere; the extraction is rooted at the file
	// system's root, and a diagnostic's path is made relative to root.
	x := newExtraction(string(filepath.Separator))
	for _, p := range pkgs {
		x.addPackage(p)
	}
	for _, d := range x.declared {
		if d.fn != nil {
			n := x.nodes[d.id]
			n.Attrs["name"] = []string{designName(d.fn.Name())}
			x.nodes[d.id] = n
		}
		if d.name != nil {
			x.addTypeEdges(d)
		} else {
			x.addFuncEdges(d)
		}
	}
	x.addImplements()
	x.diagnostics = nil
	direct, _ := x.tableEffects(pkgs, known)
	for id, m := range direct {
		if n, ok := x.nodes[id]; ok && n.Kind == graph.NodeKindOp && m != none {
			n.Attrs["directEffects"] = m.written()
			x.nodes[id] = n
		}
	}
	diagnostics := slices.Clone(x.diagnostics)
	abs, err := filepath.Abs(root)
	if err != nil {
		return walkedResult{}, err
	}
	for i, d := range diagnostics {
		if rel, err := filepath.Rel(abs, string(filepath.Separator)+filepath.FromSlash(d.Position.Path)); err == nil {
			diagnostics[i].Position.Path = filepath.ToSlash(rel)
		}
	}
	g := graph.NewGraph()
	if err := g.AddFragment(x.result().Fragment); err != nil {
		return walkedResult{}, err
	}
	out := walkedResult{Effects: map[string][]string{}, Diagnostics: convertAll(diagnostics, func(d diag.Diagnostic) cachedDiagnostic { return cachedDiagnostic(d) })}
	for id, r := range g.Effects() {
		var kinds []string
		for _, k := range r.Effects {
			kinds = append(kinds, string(k))
		}
		out.Effects[id] = kinds
	}
	return out, nil
}

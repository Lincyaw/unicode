package golang

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/google/renameio/v2/maybe"
	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
)

// The effects of a function of a dependency module (docs/design.md §4) are
// D8 (docs/model.md §9) along calls on the code of the dependency modules:
// each function's direct effects are the kinds of the standard library
// functions it calls, by the same table and the same calls as the code
// layer, where a package-level variable lends its function literals only to
// the functions of its own package. What a dependency package gives, its
// functions with their direct effects and the functions of dependency
// packages they call, is summarized once and kept in the user's cache,
// keyed by the package, the version of its module and the files the build
// selects. A module without a version, such as one replaced by a directory,
// is summarized again on every extraction.

// packageSummary is what one dependency package gives, by function.
type packageSummary struct {
	Functions map[string]functionSummary `json:"functions"`
}

// functionSummary is a function's direct effects and the functions of
// dependency packages it calls, its own package's included.
type functionSummary struct {
	Effects []string `json:"effects,omitempty"`
	Calls   []string `json:"calls,omitempty"`
}

// dependencyEffects gives every function of the dependency packages among
// known its effects.
func dependencyEffects(root string, known map[string]*packages.Package) (map[string][]graph.Effect, error) {
	summaries := map[string]packageSummary{}
	keys := map[string]string{}
	var missing []string
	for _, path := range slices.Sorted(maps.Keys(known)) {
		p := known[path]
		if !dependency(p) {
			continue
		}
		key := summaryKey(p)
		keys[path] = key
		if s, ok := readSummary(key); ok {
			summaries[path] = s
		} else {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		loaded, err := loadDependencies(root, missing)
		if err != nil {
			return nil, err
		}
		for _, p := range loaded {
			s := summarize(p, known)
			summaries[p.PkgPath] = s
			writeSummary(keys[p.PkgPath], s)
		}
	}
	g := graph.NewGraph()
	for _, path := range slices.Sorted(maps.Keys(summaries)) {
		for _, id := range slices.Sorted(maps.Keys(summaries[path].Functions)) {
			attrs := map[string][]string{"directEffects": summaries[path].Functions[id].Effects}
			if err := g.AddNode(graph.Node{Id: id, Kind: graph.NodeKindOp, Form: "func", Layer: graph.LayerCode, Source: graph.SourceExtracted, Attrs: attrs}); err != nil {
				return nil, err
			}
		}
	}
	for _, s := range summaries {
		for from, f := range s.Functions {
			for _, to := range f.Calls {
				if g.Node(to) != nil {
					if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindCalls, From: from, To: to, Source: graph.SourceExtracted}); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	out := map[string][]graph.Effect{}
	for id, r := range g.Effects() {
		out[id] = r.Effects
	}
	return out, nil
}

// summaryKey names a dependency package's summary in the cache, or is ""
// for a package of a module without a version, such as one replaced by a
// directory, whose files may change under the same name.
func summaryKey(p *packages.Package) string {
	m := p.Module
	if m.Replace != nil {
		m = m.Replace
	}
	if m.Version == "" {
		return ""
	}
	h := sha256.New()
	write := func(s string) { h.Write([]byte(s + "\x00")) }
	write(effectsRules())
	write(runtime.Version())
	write(p.PkgPath)
	write(m.Path + "@" + m.Version)
	files := slices.Clone(p.GoFiles)
	slices.Sort(files)
	for _, f := range files {
		write(filepath.Base(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// summaryPath is where the cache keeps the summary of key, or "" when the
// user has no cache directory.
func summaryPath(key string) string {
	dir, err := os.UserCacheDir()
	if err != nil || key == "" {
		return ""
	}
	return filepath.Join(dir, "unicode", "effects", key+".json")
}

func readSummary(key string) (packageSummary, bool) {
	var s packageSummary
	path := summaryPath(key)
	if path == "" {
		return s, false
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &s) != nil || s.Functions == nil {
		return packageSummary{}, false
	}
	return s, true
}

// writeSummary keeps a summary in the cache; a cache it cannot write only
// makes the next extraction summarize again.
func writeSummary(key string, s packageSummary) {
	path := summaryPath(key)
	if path == "" || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	if data, err := json.Marshal(s); err == nil {
		_ = maybe.WriteFile(path, data, 0o644)
	}
}

// loadDependencies loads the dependency packages of the paths from their
// source, as the module at root selects them.
func loadDependencies(root string, paths []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedModule,
		Dir: root, BuildFlags: []string{"-buildvcs=false"}, ParseFile: parseFile,
	}
	loaded, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, err
	}
	var problems []error
	for _, p := range loaded {
		for _, e := range p.Errors {
			problems = append(problems, e)
		}
	}
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	return loaded, nil
}

// summarize reads a dependency package's functions with the calls the code
// layer would give them, and splits their callees into the standard library
// functions, whose kinds are direct effects, and the functions of
// dependency packages.
func summarize(p *packages.Package, known map[string]*packages.Package) packageSummary {
	x := newExtraction("")
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range gen.Specs {
					if value, ok := spec.(*ast.ValueSpec); ok {
						x.addInitializers(p, value)
					}
				}
			}
		}
	}
	s := packageSummary{Functions: map[string]functionSummary{}}
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || !funcIsNode(fd) {
				continue
			}
			fn, ok := p.TypesInfo.Defs[fd.Name].(*types.Func)
			if !ok {
				continue
			}
			x.addFuncEdges(declared{id: funcID(fn), pkg: p, pos: fd.Name.Pos(), decl: fd, fn: fn})
			s.Functions[funcID(fn)] = functionSummary{}
		}
	}
	for from, callees := range x.externals(usedFuncs([]*packages.Package{p}), known) {
		f, ok := s.Functions[from]
		if !ok {
			continue
		}
		for _, c := range callees {
			if dependency(c.pkg) {
				f.Calls = append(f.Calls, funcID(c.fn))
			} else if k := c.effect(); k != "" && !slices.Contains(f.Effects, string(k)) {
				f.Effects = append(f.Effects, string(k))
			}
		}
		s.Functions[from] = f
	}
	for id, f := range s.Functions {
		slices.SortFunc(f.Effects, func(a, b string) int { return effectRank(a) - effectRank(b) })
		s.Functions[id] = f
	}
	return s
}

func effectRank(e string) int { return slices.Index(effectKinds, graph.Effect(e)) }

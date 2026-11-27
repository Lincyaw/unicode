package syntax

import (
	"errors"
	"slices"
	"strings"

	"pgregory.net/rapid"
)

//uc:checks syntax.kind-of-follows-path
func checkKindOfFollowsPath(t rapid.TB, path string) {
	segs := strings.Split(path, "/")
	clean := !slices.ContainsFunc(segs, func(s string) bool { return s == "" || s == "." || s == ".." })
	name := segs[len(segs)-1]
	named := len(segs) > 1 && strings.HasSuffix(name, ".uc") && len(name) > len(".uc")
	var want FileKind
	switch {
	case !clean || !named:
	case path == "design/system.uc":
		want = FileKindSystem
	case segs[0] == "design":
		want = FileKindComponent
	case segs[0] == "corpus":
		want = FileKindCorpus
	}
	kind, err := KindOf(path)
	switch {
	case want == "" && err == nil:
		t.Fatalf("KindOf(%q) gave the kind %s, yet the path is not a clean path to a file <name>.uc under design/ or corpus/, so it should be rejected", path, kind)
	case want == "" && !errors.As(err, new(NotADesignFile)):
		t.Fatalf("KindOf(%q) rejected the path with %v, which is not NotADesignFile", path, err)
	case want != "" && err != nil:
		t.Fatalf("KindOf(%q) rejected the path (%v), yet it is a %s file", path, err, want)
	case kind != want:
		t.Fatalf("KindOf(%q) gave the kind %s, yet the path makes it a %s file", path, kind, want)
	}
}

// pathTops are the first segments of the paths drawn: the two directories
// of design and corpus files, and names that are close to them.
var pathTops = []string{"design", "design", "design", "corpus", "corpus", "designs", "Design", "corpora", "docs", "design.uc", "x"}

// pathNames are the names of the segments after the first, a file's extension aside.
var pathNames = rapid.OneOf(rapid.SampledFrom([]string{"system", "design", "corpus", "", "x y", "中文"}),
	rapid.StringMatching(`[a-z][a-z0-9-]{0,6}`))

// pathExtensions are .uc and what comes close to it.
var pathExtensions = []string{".uc", ".uc", ".uc", "", ".UC", ".ucx", ".uc.bak", ".txt", "uc", ".u"}

// genKindPath draws a slash-separated path relative to the root of a
// repository: under design/, under corpus/ or elsewhere, ending in .uc, in a
// file named only .uc or in something close to it, and now and then unclean,
// with a segment . or .., a doubled / or a / at either end.
//
//uc:generates syntax.kind-of-follows-path.path
func genKindPath() *rapid.Generator[string] {
	drawn := rapid.Custom(func(t *rapid.T) string {
		segs := []string{rapid.SampledFrom(pathTops).Draw(t, "top")}
		for range rapid.IntRange(0, 2).Draw(t, "dirs") {
			segs = append(segs, pathNames.Filter(func(s string) bool { return s != "" }).Draw(t, "dir"))
		}
		if rapid.IntRange(0, 5).Draw(t, "file?") > 0 {
			base := pathNames.Draw(t, "name") + rapid.SampledFrom(pathExtensions).Draw(t, "extension")
			if base == "" {
				base = "x"
			}
			segs = append(segs, base)
		}
		if rapid.IntRange(0, 3).Draw(t, "unclean?") == 0 {
			// An empty segment doubles a / or, at either end, adds one.
			segs = slices.Insert(segs, rapid.IntRange(0, len(segs)).Draw(t, "at"), rapid.SampledFrom([]string{".", "..", ""}).Draw(t, "segment"))
		}
		return strings.Join(segs, "/")
	})
	edges := rapid.SampledFrom([]string{"design/system.uc", "design/sub/system.uc", "corpus/system.uc", "system.uc",
		"design/.uc", "corpus/.uc", "corpus/sub/.uc", "design/..uc", "design/system.UC", "design/system.ucx", "design", "corpus", "design.uc",
		"design/./system.uc", "design//x.uc", "design/../design/x.uc", "./design/x.uc", "/design/x.uc", "design/x.uc/", "corpus/sub/../x.uc"})
	return rapid.OneOf(edges, drawn, drawn, drawn)
}

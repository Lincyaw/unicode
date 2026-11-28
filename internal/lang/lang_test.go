package lang

import (
	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// RootsSample is a graph and the source roots written into its system node.
//
//uc:realizes lang.RootsSample
type RootsSample struct {
	Graph *graph.Graph
	Roots map[string][]string
}

// genRootsSample draws each language's roots first, then writes them into
// the system node's roots attr with the languages interleaved, each
// language's roots in their order; a component carries a roots attr too.
//
//uc:generates lang.roots-follow-system-file.sample
func genRootsSample() *rapid.Generator[RootsSample] {
	return rapid.Custom(func(t *rapid.T) RootsSample {
		roots := map[string][]string{}
		var pending []string
		for _, language := range []string{"go", "python", "rust"} {
			if !rapid.Bool().Draw(t, language) {
				continue
			}
			for range rapid.IntRange(1, 3).Draw(t, language+"Roots") {
				roots[language] = append(roots[language], rapid.StringMatching(`[a-z./]{1,3}`).Draw(t, "dir"))
				pending = append(pending, language)
			}
		}
		next := map[string]int{}
		var entries []string
		for len(pending) > 0 {
			i := rapid.IntRange(0, len(pending)-1).Draw(t, "interleave")
			language := pending[i]
			pending = append(pending[:i], pending[i+1:]...)
			entries = append(entries, language+": "+roots[language][next[language]])
			next[language]++
		}
		g := graph.NewGraph()
		at := diag.Position{Path: "design/system.uc", Line: 1, Column: 1}
		for _, n := range []graph.Node{
			{Id: "sys", Kind: graph.NodeKindContainer, Form: "system", Layer: graph.LayerDesign, Origin: at, Attrs: map[string][]string{"roots": entries}},
			{Id: "k", Kind: graph.NodeKindContainer, Form: "component", Layer: graph.LayerDesign, Origin: at, Attrs: map[string][]string{"roots": {"go: other"}}},
		} {
			if err := g.AddNode(n); err != nil {
				t.Fatalf("AddNode refused a node of a new graph: %v", err)
			}
		}
		return RootsSample{Graph: &g, Roots: roots}
	})
}

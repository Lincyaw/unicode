package graph

import (
	"fmt"
	"slices"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
)

// FingerprintSample changes only the dimensions named by the fingerprint claim.
//
//uc:realizes graph.FingerprintSample
type FingerprintSample struct {
	Base          *Graph
	Artifacts     *Graph
	Moved         *Graph
	Edge          Edge
	RelocatedEdge Edge
	ChangedEdge   Edge
}

//uc:generates graph.fingerprints-separate-inputs-and-artifacts.sample
func genFingerprintSample() *rapid.Generator[FingerprintSample] {
	return rapid.Custom(func(t *rapid.T) FingerprintSample {
		line := rapid.IntRange(1, 1000).Draw(t, "line")
		text := rapid.StringMatching(`[a-zA-Z0-9<> ]{0,30}`).Draw(t, "body")
		base := NewGraph()
		addNode := func(g *Graph, n Node) {
			if err := g.AddNode(n); err != nil {
				t.Fatal(err)
			}
		}
		addEdge := func(g *Graph, e Edge) {
			if err := g.AddEdge(e); err != nil {
				t.Fatal(err)
			}
		}
		addNode(&base, Node{Id: "system", Kind: NodeKindContainer, Form: "system", Layer: LayerDesign, Origin: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}})
		addNode(&base, Node{Id: "system.claim", Kind: NodeKindProperty, Form: "invariant", Layer: LayerDesign, Summary: text})
		addNode(&base, Node{Id: "file:source.go", Kind: NodeKindContainer, Form: "file", Layer: LayerCode, Attrs: map[string][]string{"path": {"source.go"}}})
		for index := range rapid.IntRange(1, 5).Draw(t, "functions") {
			id := fmt.Sprintf("go:sample.F%d", index)
			addNode(&base, Node{Id: id, Kind: NodeKindOp, Form: "func", Layer: LayerCode, Source: SourceExtracted, Origin: diag.Position{Path: "source.go", Line: line + index, Column: 1}, Attrs: map[string][]string{"body": {text}}})
			addEdge(&base, Edge{Kind: EdgeKindContains, From: "file:source.go", To: id})
		}
		addEdge(&base, Edge{Kind: EdgeKindContains, From: "system", To: "system.claim"})
		edge := Edge{Kind: EdgeKindCalls, From: "go:sample.F0", To: "go:sample.F0", Source: SourceExtracted, Origin: diag.Position{Path: "source.go", Line: line, Column: 2}, Attrs: map[string][]string{"sites": {fmt.Sprintf("source.go:%d:2", line)}, "labels": {text}}}
		addEdge(&base, edge)
		artifacts, moved := NewGraph(), NewGraph()
		for _, node := range base.Nodes() {
			addNode(&artifacts, node)
			if node.Id == "go:sample.F0" {
				node.Origin.Line += rapid.IntRange(1, 10).Draw(t, "move")
			}
			addNode(&moved, node)
		}
		for _, e := range base.Edges() {
			addEdge(&artifacts, e)
			addEdge(&moved, e)
		}
		for _, node := range []Node{
			{Id: "system.claim.recorded", Kind: NodeKindCase, Form: "recorded", Layer: LayerDesign, Source: SourceProduced, Attrs: map[string][]string{"values": {text}}},
			{Id: "run:sample", Kind: NodeKindRun, Layer: LayerEvidence, Source: SourceProduced},
			{Id: "result:sample", Kind: NodeKindResult, Layer: LayerEvidence, Source: SourceProduced},
			{Id: "review:sample", Kind: NodeKindReview, Layer: LayerReview, Source: SourceProduced},
			{Id: "file:unicode_contracts_gen_test.go", Kind: NodeKindContainer, Form: "file", Layer: LayerCode, Attrs: map[string][]string{"generated": {"true"}}},
			{Id: "go:sample.Generated", Kind: NodeKindType, Form: "struct", Layer: LayerCode},
			{Id: "go:sample.Generated.Read", Kind: NodeKindOp, Form: "method", Layer: LayerCode, Attrs: map[string][]string{"body": {text}}},
		} {
			addNode(&artifacts, node)
		}
		for _, e := range []Edge{
			{Kind: EdgeKindContains, From: "system.claim", To: "system.claim.recorded"},
			{Kind: EdgeKindContains, From: "run:sample", To: "result:sample", Source: SourceProduced},
			{Kind: EdgeKindAbout, From: "review:sample", To: "go:sample.F0", Source: SourceProduced},
			{Kind: EdgeKindCovers, From: "system.claim.recorded", To: "go:sample.F0", Source: SourceProduced},
			{Kind: EdgeKindContains, From: "file:unicode_contracts_gen_test.go", To: "go:sample.Generated"},
			{Kind: EdgeKindContains, From: "go:sample.Generated", To: "go:sample.Generated.Read"},
		} {
			addEdge(&artifacts, e)
		}
		relocated, changed := edge, edge
		relocated.Origin = diag.Position{Path: "elsewhere.go", Line: line + 20, Column: 9}
		relocated.Source = SourceDerived
		changed.Attrs = cloneAttrs(edge.Attrs)
		changed.Attrs["labels"] = append(slices.Clone(changed.Attrs["labels"]), text+" changed")
		return FingerprintSample{Base: &base, Artifacts: &artifacts, Moved: &moved, Edge: edge, RelocatedEdge: relocated, ChangedEdge: changed}
	})
}

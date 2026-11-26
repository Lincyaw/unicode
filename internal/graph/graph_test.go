package graph

import (
	"bytes"
	"cmp"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
)

var (
	nodeKinds = []NodeKind{NodeKindContainer, NodeKindType, NodeKindOp, NodeKindActor, NodeKindFlow, NodeKindStep,
		NodeKindProperty, NodeKindCase, NodeKindRun, NodeKindResult, NodeKindReview}
	edgeKinds = []EdgeKind{EdgeKindContains, EdgeKindUses, EdgeKindExtends, EdgeKindImplements, EdgeKindRefers,
		EdgeKindSubscribes, EdgeKindImports, EdgeKindCalls, EdgeKindFrom, EdgeKindEmits, EdgeKindHandles, EdgeKindAbout,
		EdgeKindRealizes, EdgeKindChecks, EdgeKindGenerates, EdgeKindOwns, EdgeKindDepends, EdgeKindTraces,
		EdgeKindExercises, EdgeKindReaches, EdgeKindCovers}
	layers  = []Layer{LayerDesign, LayerCode, LayerEvidence, LayerReview}
	sources = []Source{SourceDeclared, SourceExtracted, SourceDerived, SourceProduced}
)

// genWord draws short strings over a tiny alphabet, so that two draws are
// often equal, or one is the other split in two; JSON's special characters
// and a wide character are among them.
func genWord() *rapid.Generator[string] {
	return rapid.StringOfN(rapid.SampledFrom([]rune(`ab"<中`)), 0, 3, -1)
}

// genAttrs draws attrs with few keys and short lists; a list may be nil or
// empty, and the map itself may be nil.
func genAttrs(t *rapid.T, label string) map[string][]string {
	if rapid.IntRange(0, 4).Draw(t, label+"Nil") == 0 {
		return nil
	}
	attrs := map[string][]string{}
	for range rapid.IntRange(0, 3).Draw(t, label+"Keys") {
		var values []string
		if rapid.Bool().Draw(t, label+"Empty") {
			values = []string{}
		}
		values = append(values, rapid.SliceOfN(genWord(), 0, 2).Draw(t, label+"Values")...)
		attrs[genWord().Draw(t, label+"Key")] = values
	}
	return attrs
}

func genPosition(t *rapid.T) diag.Position {
	return diag.Position{
		Path:   rapid.SampledFrom([]string{"", "design/a.uc", "corpus/b/c.uc"}).Draw(t, "path"),
		Line:   rapid.IntRange(0, 3).Draw(t, "line"),
		Column: rapid.IntRange(0, 3).Draw(t, "column"),
	}
}

// genNodeWith draws a node with the given identity.
func genNodeWith(t *rapid.T, id string) Node {
	return Node{
		Id:      id,
		Kind:    rapid.SampledFrom(nodeKinds[:3]).Draw(t, "kind"),
		Form:    genWord().Draw(t, "form"),
		Layer:   rapid.SampledFrom(layers).Draw(t, "layer"),
		Source:  rapid.SampledFrom(sources).Draw(t, "source"),
		Origin:  genPosition(t),
		Summary: genWord().Draw(t, "summary"),
		Attrs:   genAttrs(t, "attrs"),
	}
}

func genNode() *rapid.Generator[Node] {
	return rapid.Custom(func(t *rapid.T) Node { return genNodeWith(t, genWord().Draw(t, "id")) })
}

// genFragment draws nodes with distinct identities and distinct edges between them.
//
//uc:generates graph.encode-ignores-order.content
func genFragment() *rapid.Generator[Fragment] {
	return rapid.Custom(func(t *rapid.T) Fragment {
		var c Fragment
		ids := map[string]bool{}
		for range rapid.IntRange(0, 8).Draw(t, "nodes") {
			id := rapid.StringOfN(rapid.SampledFrom([]rune("ab.#-")), 1, 4, -1).Draw(t, "id")
			if !ids[id] {
				ids[id] = true
				n := genNodeWith(t, id)
				n.Kind = rapid.SampledFrom(nodeKinds).Draw(t, "anyKind")
				c.Nodes = append(c.Nodes, n)
			}
		}
		if len(c.Nodes) == 0 {
			return c
		}
		keys := map[edgeKey]bool{}
		for range rapid.IntRange(0, 12).Draw(t, "edges") {
			e := Edge{
				Kind:   rapid.SampledFrom(edgeKinds).Draw(t, "edgeKind"),
				From:   rapid.SampledFrom(c.Nodes).Draw(t, "from").Id,
				To:     rapid.SampledFrom(c.Nodes).Draw(t, "to").Id,
				Source: rapid.SampledFrom(sources).Draw(t, "edgeSource"),
				Origin: genPosition(t),
				Attrs:  genAttrs(t, "edgeAttrs"),
			}
			if key := (edgeKey{e.Kind, e.From, e.To}); !keys[key] {
				keys[key] = true
				c.Edges = append(c.Edges, e)
			}
		}
		return c
	})
}

func rotate[T any](s []T, by int) []T {
	if len(s) == 0 {
		return s
	}
	by %= len(s)
	return append(slices.Clone(s[by:]), s[:by]...)
}

// build adds the nodes, then the edges, in the order given.
func build(t rapid.TB, nodes []Node, edges []Edge) Graph {
	g := NewGraph()
	for _, n := range nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatalf("AddNode refused the node %q, whose identity is new: %v", n.Id, err)
		}
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("AddEdge refused the new edge %s %q %q, whose ends are nodes of the graph: %v", e.Kind, e.From, e.To, err)
		}
	}
	return g
}

// sameAttrs tells whether a and b have the same keys with the same lists,
// a nil list being the same as an empty one.
func sameAttrs(a, b map[string][]string) bool {
	return maps.EqualFunc(a, b, func(x, y []string) bool { return slices.Equal(x, y) })
}

// The snapshot as docs/design.md §5.2 describes it, read back.
type readSnapshot struct {
	Nodes []struct {
		Id, Kind, Form, Layer, Source, Summary, Digest string
		Origin                                         struct{ Path, Line, Column any }
		Attrs                                          map[string][]string
	}
	Edges []struct {
		Kind, From, To, Source string
		Origin                 struct{ Path, Line, Column any }
		Attrs                  map[string][]string
	}
}

func sameOrigin(p diag.Position, o struct{ Path, Line, Column any }) bool {
	return o.Path == p.Path && o.Line == float64(p.Line) && o.Column == float64(p.Column)
}

//uc:checks graph.encode-ignores-order
func checkEncodeIgnoresOrder(t rapid.TB, content Fragment, rotation int) {
	inOrder := build(t, content.Nodes, content.Edges)
	rotated := build(t, rotate(content.Nodes, rotation), rotate(content.Edges, rotation))
	snapshot := inOrder.Encode()
	if other := rotated.Encode(); !bytes.Equal(snapshot, other) {
		t.Fatalf("the same nodes and edges, added in another order, encode differently\nin order:\n%s\nrotated by %d:\n%s", snapshot, rotation, other)
	}
	var read readSnapshot
	if err := json.Unmarshal(snapshot, &read); err != nil {
		t.Fatalf("the snapshot is not JSON: %v\n%s", err, snapshot)
	}
	if read.Nodes == nil || read.Edges == nil {
		t.Fatalf("the snapshot lacks its nodes or its edges list: %s", snapshot)
	}
	nodes := slices.SortedFunc(slices.Values(content.Nodes), func(a, b Node) int { return cmp.Compare(a.Id, b.Id) })
	if len(read.Nodes) != len(nodes) {
		t.Fatalf("the snapshot holds %d nodes, the graph %d\n%s", len(read.Nodes), len(nodes), snapshot)
	}
	for i, n := range nodes {
		r := read.Nodes[i]
		switch {
		case r.Id != n.Id:
			t.Fatalf("node %d of the snapshot is %q, yet sorted by identity it should be %q", i+1, r.Id, n.Id)
		case r.Kind != string(n.Kind) || r.Form != n.Form || r.Layer != string(n.Layer) || r.Source != string(n.Source) || r.Summary != n.Summary:
			t.Fatalf("the snapshot writes the node %q as %+v, not as %+v", n.Id, r, n)
		case !sameOrigin(n.Origin, r.Origin):
			t.Fatalf("the snapshot gives the node %q the origin %v, not %v", n.Id, r.Origin, n.Origin)
		case !sameAttrs(r.Attrs, n.Attrs) || r.Attrs == nil:
			t.Fatalf("the snapshot gives the node %q the attrs %v, not %v", n.Id, r.Attrs, n.Attrs)
		case r.Digest != inOrder.Digest(n.Id):
			t.Fatalf("the snapshot gives the node %q the digest %q, Digest gives %q", n.Id, r.Digest, inOrder.Digest(n.Id))
		}
	}
	edges := slices.SortedFunc(slices.Values(content.Edges), func(a, b Edge) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	if len(read.Edges) != len(edges) {
		t.Fatalf("the snapshot holds %d edges, the graph %d\n%s", len(read.Edges), len(edges), snapshot)
	}
	for i, e := range edges {
		r := read.Edges[i]
		switch {
		case r.Kind != string(e.Kind) || r.From != e.From || r.To != e.To:
			t.Fatalf("edge %d of the snapshot is %s %q %q, yet sorted by kind, start and end it should be %s %q %q", i+1, r.Kind, r.From, r.To, e.Kind, e.From, e.To)
		case r.Source != string(e.Source) || !sameOrigin(e.Origin, r.Origin) || !sameAttrs(r.Attrs, e.Attrs) || r.Attrs == nil:
			t.Fatalf("the snapshot writes the edge %s %q %q as %+v, not as %+v", e.Kind, e.From, e.To, r, e)
		}
	}
}

func TestEncodeIgnoresOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkEncodeIgnoresOrder(t, genFragment().Draw(t, "content"), rapid.IntRange(0, 1000).Draw(t, "rotation"))
	})
}

func FuzzEncodeIgnoresOrder(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkEncodeIgnoresOrder(t, genFragment().Draw(t, "content"), rapid.IntRange(0, 1000).Draw(t, "rotation"))
	}))
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

//uc:checks graph.digest-follows-content
func checkDigestFollowsContent(t rapid.TB, first Node, second Node) {
	a, b := NewGraph(), NewGraph()
	if err := a.AddNode(first); err != nil {
		t.Fatalf("AddNode refused a node in an empty graph: %v", err)
	}
	if err := b.AddNode(second); err != nil {
		t.Fatalf("AddNode refused a node in an empty graph: %v", err)
	}
	da, db := a.Digest(first.Id), b.Digest(second.Id)
	for _, d := range []string{da, db} {
		if !sha256Hex.MatchString(d) {
			t.Fatalf("the digest %q of a node of the graph is not a SHA-256 in lower-case hex", d)
		}
	}
	same := first.Kind == second.Kind && first.Form == second.Form && first.Summary == second.Summary && sameAttrs(first.Attrs, second.Attrs)
	switch {
	case same && da != db:
		t.Fatalf("two nodes with the same kind, form, summary and attrs have different digests\nfirst:  %+v\nsecond: %+v", first, second)
	case !same && da == db:
		t.Fatalf("two nodes that differ in kind, form, summary or attrs have the same digest\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

// genSecond draws the second node: an independent one, one with the first's
// content and other identity, layer, source and origin, or the first with
// one part of its content changed.
func genSecond(t *rapid.T, first Node) Node {
	switch rapid.IntRange(0, 2).Draw(t, "relation") {
	case 0:
		return genNode().Draw(t, "second")
	case 1:
		other := genNodeWith(t, genWord().Draw(t, "id"))
		other.Kind, other.Form, other.Summary, other.Attrs = first.Kind, first.Form, first.Summary, map[string][]string{}
		for _, k := range slices.Sorted(maps.Keys(first.Attrs)) {
			v := first.Attrs[k]
			if len(v) == 0 && rapid.Bool().Draw(t, "swapEmpty") {
				v = nil
			}
			other.Attrs[k] = slices.Clone(v)
		}
		if first.Attrs == nil && rapid.Bool().Draw(t, "nilAttrs") {
			other.Attrs = nil
		}
		return other
	}
	second := first
	second.Attrs = maps.Clone(first.Attrs)
	switch rapid.IntRange(0, 4).Draw(t, "change") {
	case 0:
		second.Kind = rapid.SampledFrom(nodeKinds).Draw(t, "kind")
	case 1:
		second.Form, second.Summary = second.Summary, second.Form
	case 2:
		second.Summary += genWord().Draw(t, "more")
	case 3:
		if second.Attrs == nil {
			second.Attrs = map[string][]string{}
		}
		second.Attrs[genWord().Draw(t, "key")] = rapid.SliceOfN(genWord(), 0, 2).Draw(t, "values")
	default:
		if keys := slices.Sorted(maps.Keys(second.Attrs)); len(keys) > 0 {
			k := rapid.SampledFrom(keys).Draw(t, "changedKey")
			second.Attrs[k] = append(slices.Clone(second.Attrs[k]), genWord().Draw(t, "value"))
		}
	}
	return second
}

func TestDigestFollowsContent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		first := genNode().Draw(t, "first")
		checkDigestFollowsContent(t, first, genSecond(t, first))
	})
}

func FuzzDigestFollowsContent(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		first := genNode().Draw(t, "first")
		checkDigestFollowsContent(t, first, genSecond(t, first))
	}))
}

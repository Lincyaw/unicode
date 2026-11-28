package graph

import (
	"encoding/json"
	"maps"
	"math/rand/v2"
	"reflect"
	"regexp"
	"slices"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
)

var (
	nodeKinds = []NodeKind{NodeKindContainer, NodeKindType, NodeKindOp, NodeKindActor, NodeKindFlow, NodeKindStep,
		NodeKindProperty, NodeKindObservation, NodeKindCase, NodeKindRun, NodeKindResult, NodeKindReview}
	edgeKinds = []EdgeKind{EdgeKindContains, EdgeKindUses, EdgeKindExtends, EdgeKindImplements, EdgeKindRefers,
		EdgeKindSubscribes, EdgeKindImports, EdgeKindCalls, EdgeKindFrom, EdgeKindEmits, EdgeKindHandles, EdgeKindAbout,
		EdgeKindRealizes, EdgeKindChecks, EdgeKindGenerates, EdgeKindObserves, EdgeKindSubstitutes, EdgeKindPublishes, EdgeKindOwns, EdgeKindDepends, EdgeKindTraces,
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
		Salient: rapid.Bool().Draw(t, "salient"),
		Attrs:   genAttrs(t, "attrs"),
	}
}

//uc:generates graph.digest-follows-content.first
func genNode() *rapid.Generator[Node] {
	return rapid.Custom(func(t *rapid.T) Node { return genNodeWith(t, genWord().Draw(t, "id")) })
}

// genFragment draws nodes with distinct identities and distinct edges between them.
//
//uc:generates graph.encode-ignores-order.content
//uc:generates graph.snapshot-copies-content.content
//uc:generates graph.snapshot-json-is-the-record.content
//uc:generates graph.decode-inverts-encode.content
//uc:generates graph.decode-is-total.content
//uc:generates graph.digest-has-sha256-length.content
//uc:generates graph.digest-is-lowercase-hex.content
//uc:generates graph.compare-with-itself-is-empty.content
//uc:generates graph.compare-lists-changed-nodes.base
//uc:generates graph.compare-lists-changed-edges.base
//uc:generates graph.compare-lists-changed-attrs.base
//uc:generates graph.compare-balances-attr-values.base
//uc:generates graph.compare-is-canonical.base
//uc:generates graph.compare-lists-are-present.base
//uc:generates graph.compare-reverses.base
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

//uc:generates graph.node-sequence-keeps-index.snapshot
func genComparisonSnapshot() *rapid.Generator[*Graph] {
	return rapid.Custom(func(t *rapid.T) *Graph {
		fragment := genFragment().Draw(t, "fragment")
		snapshot, err := graphOf(fragment.Nodes, fragment.Edges)
		if err != nil {
			t.Fatalf("AddFragment refused nodes with distinct identities and distinct edges between them: %v", err)
		}
		return snapshot
	})
}

// deriveContentGraph adds the nodes, then the edges, of content in their
// order to a new graph.
//
//uc:generates graph.compare-with-itself-is-empty.snapshot
//uc:generates graph.encode-ignores-order.inOrder
//uc:generates graph.snapshot-copies-content.g
//uc:generates graph.decode-inverts-encode.g
//uc:generates graph.digest-has-sha256-length.g
func deriveContentGraph(content Fragment) (*Graph, error) {
	return graphOf(content.Nodes, content.Edges)
}

//uc:generates graph.compare-with-itself-is-empty.reordered
//uc:generates graph.encode-ignores-order.rotated
func deriveRotatedGraph(content Fragment, rotation int) (*Graph, error) {
	return graphOf(rotate(content.Nodes, rotation), rotate(content.Edges, rotation))
}

//uc:generates graph.encode-ignores-order.shuffled
func deriveShuffledGraph(content Fragment, rotation int) (*Graph, error) {
	return graphOf(shuffled(content.Nodes, uint64(rotation)), shuffled(content.Edges, uint64(rotation)))
}

//uc:generates graph.digest-follows-content.a
func deriveFirstGraph(first Node) (*Graph, error) { return graphOf([]Node{first}, nil) }

//uc:generates graph.digest-follows-content.b
func deriveSecondGraph(second Node) (*Graph, error) { return graphOf([]Node{second}, nil) }

func rotate[T any](s []T, by int) []T {
	if len(s) == 0 {
		return s
	}
	by %= len(s)
	return append(slices.Clone(s[by:]), s[:by]...)
}

func shuffled[T any](values []T, seed uint64) []T {
	copy := slices.Clone(values)
	random := rand.New(rand.NewPCG(seed, seed+1))
	random.Shuffle(len(copy), func(i, j int) { copy[i], copy[j] = copy[j], copy[i] })
	return copy
}

// genSecond draws the second node: an independent one, one with the first's
// content and other identity, layer, source and origin, or the first with
// one part of its content changed.
//
//uc:generates graph.digest-follows-content.second
func genSecond(first Node) *rapid.Generator[Node] {
	return rapid.Custom(func(t *rapid.T) Node {
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
	})
}

// attrsJSON is how attrs read back from JSON look when every list is written.
func attrsJSON(attrs map[string][]string) map[string]any {
	out := map[string]any{}
	for k, v := range attrs {
		values := []any{}
		for _, x := range v {
			values = append(values, x)
		}
		out[k] = values
	}
	return out
}

func originJSON(p diag.Position) map[string]any {
	return map[string]any{"path": p.Path, "line": float64(p.Line), "column": float64(p.Column)}
}

//uc:checks graph.snapshot-json-is-the-record
func checkSnapshotJsonIsTheRecord(t rapid.TB, content Fragment) {
	g, err := graphOf(content.Nodes, content.Edges)
	if err != nil {
		t.Fatalf("AddFragment refused nodes with distinct identities and distinct edges between them: %v", err)
	}
	var got any
	if err := json.Unmarshal(g.Encode(), &got); err != nil {
		t.Fatalf("the snapshot that encode wrote is not JSON: %v", err)
	}
	record := g.Snapshot()
	nodes, edges := []any{}, []any{}
	for _, n := range record.Nodes {
		nodes = append(nodes, map[string]any{"id": n.Id, "kind": string(n.Kind), "form": n.Form, "layer": string(n.Layer), "source": string(n.Source),
			"origin": originJSON(n.Origin), "summary": n.Summary, "salient": n.Salient, "digest": n.Digest, "attrs": attrsJSON(n.Attrs)})
	}
	for _, e := range record.Edges {
		edges = append(edges, map[string]any{"kind": string(e.Kind), "from": e.From, "to": e.To, "source": string(e.Source),
			"origin": originJSON(e.Origin), "attrs": attrsJSON(e.Attrs)})
	}
	if want := map[string]any{"nodes": nodes, "edges": edges}; !reflect.DeepEqual(got, want) {
		t.Fatalf("the JSON that encode wrote is not the snapshot record with its field names as keys and no null\nwrote: %v\nrecord: %v", got, want)
	}
}

var lowercaseHex = regexp.MustCompile(`^[0-9a-f]*$`)

//uc:checks graph.digest-is-lowercase-hex
func checkDigestIsLowercaseHex(t rapid.TB, content Fragment) {
	g, err := graphOf(content.Nodes, content.Edges)
	if err != nil {
		t.Fatalf("AddNode or AddEdge refused nodes with distinct identities and distinct edges between them: %v", err)
	}
	for _, n := range content.Nodes {
		if d := g.Digest(n.Id); !lowercaseHex.MatchString(d) {
			t.Fatalf("the digest %q of the node %q has a character that is not 0 to 9 or a to f", d, n.Id)
		}
	}
}

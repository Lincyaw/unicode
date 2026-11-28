package graph

import (
	"encoding/json"
	"maps"
	"slices"

	"pgregory.net/rapid"
)

// genEdit draws a change of base: some nodes and edges removed, some nodes
// with another content and some edges with other attrs, some items with
// another origin, source, layer or salience but the same content, and some
// new nodes and edges. A removed node may come back under its identity.
//
//uc:generates graph.compare-lists-changed-nodes.head
//uc:generates graph.compare-lists-changed-edges.head
//uc:generates graph.compare-lists-changed-attrs.head
//uc:generates graph.compare-balances-attr-values.head
//uc:generates graph.compare-is-canonical.head
//uc:generates graph.compare-lists-are-present.head
//uc:generates graph.compare-keeps-value-order.head
//uc:generates graph.compare-reverses.head
func genEdit(base Fragment) *rapid.Generator[Fragment] {
	return rapid.Custom(func(t *rapid.T) Fragment {
		var head Fragment
		ids := map[string]bool{}
		for _, n := range base.Nodes {
			n.Attrs = cloneAttrs(n.Attrs)
			switch rapid.IntRange(0, 4).Draw(t, "nodeEdit") {
			case 0:
				continue
			case 1:
				n = changeContent(t, n)
			case 2:
				n.Origin, n.Salient = genPosition(t), !n.Salient
				n.Layer = rapid.SampledFrom(layers).Draw(t, "otherLayer")
				n.Source = rapid.SampledFrom(sources).Draw(t, "otherSource")
			}
			ids[n.Id] = true
			head.Nodes = append(head.Nodes, n)
		}
		for range rapid.IntRange(0, 3).Draw(t, "newNodes") {
			id := rapid.StringOfN(rapid.SampledFrom([]rune("ab.#-")), 1, 4, -1).Draw(t, "newId")
			if !ids[id] {
				ids[id] = true
				head.Nodes = append(head.Nodes, genNodeWith(t, id))
			}
		}
		keys := map[edgeKey]bool{}
		for _, e := range base.Edges {
			if !ids[e.From] || !ids[e.To] {
				continue
			}
			e.Attrs = cloneAttrs(e.Attrs)
			switch rapid.IntRange(0, 4).Draw(t, "edgeEdit") {
			case 0:
				continue
			case 1:
				e.Attrs = changeAttrs(t, e.Attrs)
			case 2:
				e.Origin = genPosition(t)
				e.Source = rapid.SampledFrom(sources).Draw(t, "otherEdgeSource")
			}
			keys[edgeKey{e.Kind, e.From, e.To}] = true
			head.Edges = append(head.Edges, e)
		}
		if len(head.Nodes) == 0 {
			return head
		}
		for range rapid.IntRange(0, 4).Draw(t, "newEdges") {
			e := Edge{
				Kind:   rapid.SampledFrom(edgeKinds).Draw(t, "newEdgeKind"),
				From:   rapid.SampledFrom(head.Nodes).Draw(t, "newFrom").Id,
				To:     rapid.SampledFrom(head.Nodes).Draw(t, "newTo").Id,
				Source: rapid.SampledFrom(sources).Draw(t, "newEdgeSource"),
				Origin: genPosition(t),
				Attrs:  genAttrs(t, "newEdgeAttrs"),
			}
			if key := (edgeKey{e.Kind, e.From, e.To}); !keys[key] {
				keys[key] = true
				head.Edges = append(head.Edges, e)
			}
		}
		return head
	})
}

// changeContent changes one part of a node's content, its attrs as often as
// the rest: its kind, form or summary, or its attrs. The change may leave the
// content as it was.
func changeContent(t *rapid.T, n Node) Node {
	switch rapid.IntRange(0, 5).Draw(t, "contentEdit") {
	case 0:
		n.Kind = rapid.SampledFrom(nodeKinds).Draw(t, "otherKind")
	case 1:
		n.Form = genWord().Draw(t, "otherForm")
	case 2:
		n.Summary = genWord().Draw(t, "otherSummary")
	default:
		n.Attrs = changeAttrs(t, n.Attrs)
	}
	return n
}

// changeAttrs changes attrs, which the caller owns, mostly on a key it has:
// a key gets new values, loses a value, has its values reversed, turns from
// absent to empty or from nil to empty, or goes away.
func changeAttrs(t *rapid.T, attrs map[string][]string) map[string][]string {
	if attrs == nil {
		attrs = map[string][]string{}
	}
	keys := slices.Sorted(maps.Keys(attrs))
	key := genWord().Draw(t, "attrKey")
	if len(keys) > 0 && rapid.IntRange(0, 3).Draw(t, "newKey?") > 0 {
		key = rapid.SampledFrom(keys).Draw(t, "changedKey")
	}
	switch rapid.IntRange(0, 4).Draw(t, "attrEdit") {
	case 0:
		attrs[key] = append(attrs[key], rapid.SliceOfN(genWord(), 1, 2).Draw(t, "newValues")...)
	case 1:
		if v := attrs[key]; len(v) > 0 {
			attrs[key] = slices.Delete(slices.Clone(v), 0, 1)
		}
	case 2:
		attrs[key] = slices.Clone(attrs[key])
		slices.Reverse(attrs[key])
	case 3:
		if attrs[key] == nil {
			attrs[key] = []string{}
		}
	default:
		delete(attrs, key)
	}
	return attrs
}

//uc:generates graph.compare-lists-changed-nodes.gb
//uc:generates graph.compare-lists-changed-edges.gb
//uc:generates graph.compare-lists-changed-attrs.gb
//uc:generates graph.compare-balances-attr-values.gb
//uc:generates graph.compare-is-canonical.gb
//uc:generates graph.compare-reverses.gb
func deriveBaseGraph(base Fragment) (*Graph, error) { return graphOf(base.Nodes, base.Edges) }

//uc:generates graph.compare-lists-changed-nodes.gh
//uc:generates graph.compare-lists-changed-edges.gh
//uc:generates graph.compare-lists-changed-attrs.gh
//uc:generates graph.compare-balances-attr-values.gh
//uc:generates graph.compare-is-canonical.gh
//uc:generates graph.compare-reverses.gh
func deriveHeadGraph(head Fragment) (*Graph, error) { return graphOf(head.Nodes, head.Edges) }

//uc:generates graph.compare-is-canonical.rb
func deriveRotatedBase(base Fragment, rotation int) (*Graph, error) {
	return graphOf(rotate(base.Nodes, rotation), rotate(base.Edges, rotation))
}

//uc:generates graph.compare-is-canonical.rh
func deriveRotatedHead(head Fragment, rotation int) (*Graph, error) {
	return graphOf(rotate(head.Nodes, rotation), rotate(head.Edges, rotation))
}

// graphsOf builds the base and head graphs of a comparison.
func graphsOf(t rapid.TB, base, head Fragment) (*Graph, *Graph) {
	gb, err := graphOf(base.Nodes, base.Edges)
	if err != nil {
		t.Fatalf("the base fragment does not make a graph: %v", err)
	}
	gh, err := graphOf(head.Nodes, head.Edges)
	if err != nil {
		t.Fatalf("the head fragment does not make a graph: %v", err)
	}
	return gb, gh
}

//uc:checks graph.compare-lists-are-present
func checkCompareListsArePresent(t rapid.TB, base Fragment, head Fragment) {
	gb, gh := graphsOf(t, base, head)
	encoded, err := json.Marshal(Compare(gb, gh))
	if err != nil {
		t.Fatalf("the comparison does not encode as JSON: %v", err)
	}
	var c struct {
		Nodes []struct {
			Id    string                       `json:"id"`
			Attrs []map[string]json.RawMessage `json:"attrs"`
		} `json:"nodes"`
		Edges json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal(encoded, &c); err != nil {
		t.Fatalf("the comparison's JSON does not decode: %v", err)
	}
	if c.Nodes == nil || string(c.Edges) == "null" {
		t.Fatalf("the comparison writes its nodes or edges as null: %s", encoded)
	}
	for _, n := range c.Nodes {
		if n.Attrs == nil {
			t.Fatalf("the change of the node %q writes its attrs as null: %s", n.Id, encoded)
		}
		for _, a := range n.Attrs {
			if string(a["removed"]) == "null" || string(a["added"]) == "null" {
				t.Fatalf("an attrs change of the node %q writes its removed or added values as null: %s", n.Id, encoded)
			}
		}
	}
}

// isSubsequence tells whether sub is xs with some values left out.
func isSubsequence(sub, xs []string) bool {
	i := 0
	for _, x := range xs {
		if i < len(sub) && sub[i] == x {
			i++
		}
	}
	return i == len(sub)
}

//uc:checks graph.compare-keeps-value-order
func checkCompareKeepsValueOrder(t rapid.TB, base Fragment, head Fragment) {
	gb, gh := graphsOf(t, base, head)
	for _, n := range Compare(gb, gh).Nodes {
		var was, is map[string][]string
		if n.Before != nil {
			was = n.Before.Attrs
		}
		if n.After != nil {
			is = n.After.Attrs
		}
		for _, a := range n.Attrs {
			if !isSubsequence(a.Removed, was[a.Key]) || !isSubsequence(a.Added, is[a.Key]) {
				t.Fatalf("on the key %q of the node %q, the removed values %q are not in the order of the base's %q, or the added %q not in that of the head's %q",
					a.Key, n.Id, a.Removed, was[a.Key], a.Added, is[a.Key])
			}
		}
	}
}

package graph

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"pgregory.net/rapid"
)

// genEdit draws a change of base: some nodes and edges removed, some nodes
// with another content and some edges with other attrs, some items with
// another origin, source, layer or salience but the same content, and some
// new nodes and edges. A removed node may come back under its identity.
//
//uc:generates graph.compare-follows-identity.head
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

func sameNode(a, b Node) bool {
	return a.Id == b.Id && a.Kind == b.Kind && a.Form == b.Form && a.Layer == b.Layer && a.Source == b.Source &&
		a.Origin == b.Origin && a.Summary == b.Summary && a.Salient == b.Salient && sameAttrs(a.Attrs, b.Attrs)
}

func sameEdge(a, b Edge) bool {
	return a.Kind == b.Kind && a.From == b.From && a.To == b.To && a.Source == b.Source && a.Origin == b.Origin &&
		sameAttrs(a.Attrs, b.Attrs)
}

// samePresent tells whether got is nil exactly when the item is absent, and
// otherwise is the item.
func samePresent[T any](got *T, want T, present bool, same func(a, b T) bool) bool {
	if got == nil || !present {
		return got == nil && !present
	}
	return same(*got, want)
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

// counts are how often each value occurs in xs, less how often in less.
func counts(xs, less []string) map[string]int {
	c := map[string]int{}
	for _, x := range xs {
		c[x]++
	}
	for _, x := range less {
		if c[x]--; c[x] == 0 {
			delete(c, x)
		}
	}
	return c
}

// attrsMismatch returns why changes is not the change from the attrs was to
// is, or "": sorted by key, exactly the keys whose values differ, a key on
// one side only included; on each, the removed values a subsequence of
// was's, the added a subsequence of is's, no value both removed and added,
// and what remains of the two sides the same values as often.
func attrsMismatch(was, is map[string][]string, changes []AttrChange) string {
	var differ []string
	for _, k := range slices.Sorted(slices.Values(allKeys(was, is))) {
		before, inWas := was[k]
		after, inIs := is[k]
		if inWas != inIs || !slices.Equal(before, after) {
			differ = append(differ, k)
		}
	}
	if changes == nil {
		return "the attrs changes are nil instead of a list"
	}
	keys := make([]string, len(changes))
	for i, c := range changes {
		keys[i] = c.Key
	}
	if !slices.Equal(keys, differ) {
		return fmt.Sprintf("the attrs changes list the keys %q, yet the keys whose values differ, sorted, are %q", keys, differ)
	}
	for _, c := range changes {
		before, after := was[c.Key], is[c.Key]
		switch {
		case c.Removed == nil || c.Added == nil:
			return fmt.Sprintf("the change of the key %q has a nil list of removed or added values", c.Key)
		case !isSubsequence(c.Removed, before):
			return fmt.Sprintf("the values removed from the key %q, %q, are not a subsequence of the base's %q", c.Key, c.Removed, before)
		case !isSubsequence(c.Added, after):
			return fmt.Sprintf("the values added to the key %q, %q, are not a subsequence of the head's %q", c.Key, c.Added, after)
		case slices.ContainsFunc(c.Removed, func(v string) bool { return slices.Contains(c.Added, v) }):
			return fmt.Sprintf("the key %q lists a value as both removed %q and added %q", c.Key, c.Removed, c.Added)
		case !maps.Equal(counts(before, c.Removed), counts(after, c.Added)):
			return fmt.Sprintf("on the key %q, the base's values %q less the removed %q are not the head's %q less the added %q",
				c.Key, before, c.Removed, after, c.Added)
		}
	}
	return ""
}

// allKeys lists the keys that a or b has.
func allKeys[K comparable, V any](a, b map[K]V) []K {
	set := map[K]V{}
	maps.Copy(set, a)
	maps.Copy(set, b)
	return slices.Collect(maps.Keys(set))
}

func indexNodes(nodes []Node) map[string]Node {
	out := map[string]Node{}
	for _, n := range nodes {
		out[n.Id] = n
	}
	return out
}

func indexEdges(edges []Edge) map[edgeKey]Edge {
	out := map[edgeKey]Edge{}
	for _, e := range edges {
		out[edgeKey{e.Kind, e.From, e.To}] = e
	}
	return out
}

func compareKeys(a, b edgeKey) int {
	return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.from, b.from), cmp.Compare(a.to, b.to))
}

//uc:checks graph.compare-follows-identity
func checkCompareFollowsIdentity(t rapid.TB, base Fragment, head Fragment) {
	gb, gh := build(t, base.Nodes, base.Edges), build(t, head.Nodes, head.Edges)
	c := Compare(&gb, &gh)
	bn, hn := indexNodes(base.Nodes), indexNodes(head.Nodes)
	ids := slices.Sorted(slices.Values(allKeys(bn, hn)))
	var nodes []NodeChange
	for _, id := range ids {
		_, inBase := bn[id]
		_, inHead := hn[id]
		switch {
		case !inBase:
			nodes = append(nodes, NodeChange{Id: id, Change: ChangeKindAdded})
		case !inHead:
			nodes = append(nodes, NodeChange{Id: id, Change: ChangeKindRemoved})
		case gb.Digest(id) != gh.Digest(id):
			nodes = append(nodes, NodeChange{Id: id, Change: ChangeKindModified})
		}
	}
	if len(c.Nodes) != len(nodes) {
		t.Fatalf("Compare lists %d node changes, yet %d nodes are added, removed or have another content hash\ngot: %+v\nwant: %+v", len(c.Nodes), len(nodes), c.Nodes, nodes)
	}
	for i, got := range c.Nodes {
		want := nodes[i]
		before, inBase := bn[want.Id]
		after, inHead := hn[want.Id]
		switch {
		case got.Id != want.Id || got.Change != want.Change:
			t.Fatalf("node change %d is %s %q, yet aligned by identity and sorted it should be %s %q", i+1, got.Change, got.Id, want.Change, want.Id)
		case !samePresent(got.Before, before, inBase, sameNode):
			t.Fatalf("the change of the node %q gives the base's node as %+v, not %+v", want.Id, got.Before, before)
		case !samePresent(got.After, after, inHead, sameNode):
			t.Fatalf("the change of the node %q gives the head's node as %+v, not %+v", want.Id, got.After, after)
		}
		if why := attrsMismatch(before.Attrs, after.Attrs, got.Attrs); why != "" {
			t.Fatalf("the change of the node %q: %s", want.Id, why)
		}
	}
	be, he := indexEdges(base.Edges), indexEdges(head.Edges)
	keys := slices.SortedFunc(slices.Values(allKeys(be, he)), compareKeys)
	var edges []EdgeChange
	for _, k := range keys {
		before, inBase := be[k]
		after, inHead := he[k]
		change := ChangeKindModified
		switch {
		case !inBase:
			change = ChangeKindAdded
		case !inHead:
			change = ChangeKindRemoved
		case sameAttrs(before.Attrs, after.Attrs):
			continue
		}
		edges = append(edges, EdgeChange{Kind: k.kind, From: k.from, To: k.to, Change: change})
	}
	if len(c.Edges) != len(edges) {
		t.Fatalf("Compare lists %d edge changes, yet %d edges are added, removed or have other attrs\ngot: %+v\nwant: %+v", len(c.Edges), len(edges), c.Edges, edges)
	}
	for i, got := range c.Edges {
		want := edges[i]
		k := edgeKey{want.Kind, want.From, want.To}
		before, inBase := be[k]
		after, inHead := he[k]
		switch {
		case got.Kind != want.Kind || got.From != want.From || got.To != want.To || got.Change != want.Change:
			t.Fatalf("edge change %d is %s %s %q %q, yet aligned by kind, start and end and sorted it should be %s %s %q %q",
				i+1, got.Change, got.Kind, got.From, got.To, want.Change, want.Kind, want.From, want.To)
		case !samePresent(got.Before, before, inBase, sameEdge):
			t.Fatalf("the change of the edge %s %q %q gives the base's edge as %+v, not %+v", k.kind, k.from, k.to, got.Before, before)
		case !samePresent(got.After, after, inHead, sameEdge):
			t.Fatalf("the change of the edge %s %q %q gives the head's edge as %+v, not %+v", k.kind, k.from, k.to, got.After, after)
		}
	}
}

var reversed = map[ChangeKind]ChangeKind{
	ChangeKindAdded: ChangeKindRemoved, ChangeKindRemoved: ChangeKindAdded, ChangeKindModified: ChangeKindModified,
}

func samePointer[T any](a, b *T, same func(a, b T) bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return same(*a, *b)
}

//uc:checks graph.compare-reverses
func checkCompareReverses(t rapid.TB, base Fragment, head Fragment) {
	gb, gh := build(t, base.Nodes, base.Edges), build(t, head.Nodes, head.Edges)
	forward, backward := Compare(&gb, &gh), Compare(&gh, &gb)
	if len(forward.Nodes) != len(backward.Nodes) || len(forward.Edges) != len(backward.Edges) {
		t.Fatalf("compare(base, head) lists %d node and %d edge changes, compare(head, base) %d and %d",
			len(forward.Nodes), len(forward.Edges), len(backward.Nodes), len(backward.Edges))
	}
	for i, f := range forward.Nodes {
		b := backward.Nodes[i]
		switch {
		case b.Id != f.Id || b.Change != reversed[f.Change]:
			t.Fatalf("node change %d of compare(base, head) is %s %q, yet that of compare(head, base) is %s %q", i+1, f.Change, f.Id, b.Change, b.Id)
		case !samePointer(b.Before, f.After, sameNode) || !samePointer(b.After, f.Before, sameNode):
			t.Fatalf("for the node %q, compare(head, base) does not swap the before %+v and after %+v of compare(base, head)", f.Id, f.Before, f.After)
		case len(b.Attrs) != len(f.Attrs):
			t.Fatalf("for the node %q, compare(base, head) changes %d attrs keys, compare(head, base) %d", f.Id, len(f.Attrs), len(b.Attrs))
		}
		for j, fa := range f.Attrs {
			if ba := b.Attrs[j]; ba.Key != fa.Key || !slices.Equal(ba.Removed, fa.Added) || !slices.Equal(ba.Added, fa.Removed) {
				t.Fatalf("for the node %q, compare(head, base) gives the attrs change %+v, not the reverse of %+v", f.Id, ba, fa)
			}
		}
	}
	for i, f := range forward.Edges {
		b := backward.Edges[i]
		switch {
		case b.Kind != f.Kind || b.From != f.From || b.To != f.To || b.Change != reversed[f.Change]:
			t.Fatalf("edge change %d of compare(base, head) is %s %s %q %q, yet that of compare(head, base) is %s %s %q %q",
				i+1, f.Change, f.Kind, f.From, f.To, b.Change, b.Kind, b.From, b.To)
		case !samePointer(b.Before, f.After, sameEdge) || !samePointer(b.After, f.Before, sameEdge):
			t.Fatalf("for the edge %s %q %q, compare(head, base) does not swap the before %+v and after %+v of compare(base, head)", f.Kind, f.From, f.To, f.Before, f.After)
		}
	}
}

//uc:checks graph.compare-with-itself-is-empty
func checkCompareWithItselfIsEmpty(t rapid.TB, content Fragment, rotation int) {
	g := build(t, content.Nodes, content.Edges)
	rotated := build(t, rotate(content.Nodes, rotation), rotate(content.Edges, rotation))
	for _, other := range []struct {
		name  string
		graph *Graph
	}{{"itself", &g}, {"the graph of its nodes and edges added in another order", &rotated}} {
		if c := Compare(&g, other.graph); len(c.Nodes) > 0 || len(c.Edges) > 0 {
			t.Fatalf("comparing a graph with %s lists the changes %+v", other.name, c)
		}
	}
}

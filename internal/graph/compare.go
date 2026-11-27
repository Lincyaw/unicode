package graph

import (
	"cmp"
	"maps"
	"slices"
)

// ChangeKind is how an item differs between two snapshots (docs/model.md §8).
//
//uc:realizes graph.ChangeKind
type ChangeKind string

const (
	ChangeKindAdded     ChangeKind = "added"
	ChangeKindRemoved   ChangeKind = "removed"
	ChangeKindModified  ChangeKind = "modified"
	ChangeKindUnchanged ChangeKind = "unchanged"
)

// AttrChange is what a change removed from and added to the values of one
// attribute key.
//
//uc:realizes graph.AttrChange
type AttrChange struct {
	Key     string   `json:"key"`
	Removed []string `json:"removed"`
	Added   []string `json:"added"`
}

// NodeChange is the change of one node: Before is the node in the base,
// After the node in the head, and Attrs the values it lost and gained.
//
//uc:realizes graph.NodeChange
type NodeChange struct {
	Id     string       `json:"id"`
	Change ChangeKind   `json:"change"`
	Before *Node        `json:"before,omitempty"`
	After  *Node        `json:"after,omitempty"`
	Attrs  []AttrChange `json:"attrs"`
}

// EdgeChange is the change of one edge, identified by its kind, start and end.
//
//uc:realizes graph.EdgeChange
type EdgeChange struct {
	Kind   EdgeKind   `json:"kind"`
	From   string     `json:"from"`
	To     string     `json:"to"`
	Change ChangeKind `json:"change"`
	Before *Edge      `json:"before,omitempty"`
	After  *Edge      `json:"after,omitempty"`
}

// Comparison lists the nodes and edges that differ between two snapshots:
// the nodes sorted by identity, the edges by kind, start and end.
//
//uc:realizes graph.Comparison
type Comparison struct {
	Nodes []NodeChange `json:"nodes"`
	Edges []EdgeChange `json:"edges"`
}

// Compare aligns the nodes and the edges of base and head by identity
// (docs/model.md §8) and lists those that head adds, removes or modifies. A
// node is modified when its content hash differs; an edge, whose content is
// its attrs, when its attrs differ. Unchanged items are left out.
//
//uc:realizes graph.compare
func Compare(base, head *Graph) Comparison {
	c := Comparison{Nodes: []NodeChange{}, Edges: []EdgeChange{}}
	ids := unionKeys(base.nodes, head.nodes)
	slices.Sort(ids)
	for _, id := range ids {
		before, after := base.Node(id), head.Node(id)
		change := changeOf(before != nil, after != nil, base.Digest(id) != head.Digest(id))
		if change == ChangeKindUnchanged {
			continue
		}
		var was, is map[string][]string
		if before != nil {
			was = before.Attrs
		}
		if after != nil {
			is = after.Attrs
		}
		c.Nodes = append(c.Nodes, NodeChange{Id: id, Change: change, Before: before, After: after, Attrs: attrChanges(was, is)})
	}
	keys := unionKeys(base.edges, head.edges)
	slices.SortFunc(keys, func(a, b edgeKey) int {
		return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.from, b.from), cmp.Compare(a.to, b.to))
	})
	for _, k := range keys {
		before, inBase := base.edges[k]
		after, inHead := head.edges[k]
		change := changeOf(inBase, inHead, !maps.EqualFunc(before.Attrs, after.Attrs, slices.Equal[[]string]))
		if change == ChangeKindUnchanged {
			continue
		}
		ec := EdgeChange{Kind: k.kind, From: k.from, To: k.to, Change: change}
		if inBase {
			before.Attrs = cloneAttrs(before.Attrs)
			ec.Before = &before
		}
		if inHead {
			after.Attrs = cloneAttrs(after.Attrs)
			ec.After = &after
		}
		c.Edges = append(c.Edges, ec)
	}
	return c
}

// unionKeys lists the keys of a and b, each once, in no order.
func unionKeys[K comparable, V any](a, b map[K]V) []K {
	keys := slices.Collect(maps.Keys(a))
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	return keys
}

func changeOf(inBase, inHead, differs bool) ChangeKind {
	switch {
	case !inBase:
		return ChangeKindAdded
	case !inHead:
		return ChangeKindRemoved
	case differs:
		return ChangeKindModified
	}
	return ChangeKindUnchanged
}

// attrChanges lists, sorted by key, every key whose values differ between
// was and is, a key present on one side only included: the values of was
// that is lacks, and those of is that was lacks, each counted as often as
// it occurs and kept in its order.
func attrChanges(was, is map[string][]string) []AttrChange {
	keys := unionKeys(was, is)
	slices.Sort(keys)
	changes := []AttrChange{}
	for _, k := range keys {
		before, inWas := was[k]
		after, inIs := is[k]
		if inWas == inIs && slices.Equal(before, after) {
			continue
		}
		changes = append(changes, AttrChange{Key: k, Removed: without(before, after), Added: without(after, before)})
	}
	return changes
}

// without returns the values of xs left after removing, for each value of
// ys, one occurrence of it.
func without(xs, ys []string) []string {
	left := map[string]int{}
	for _, y := range ys {
		left[y]++
	}
	out := []string{}
	for _, x := range xs {
		if left[x] > 0 {
			left[x]--
			continue
		}
		out = append(out, x)
	}
	return out
}

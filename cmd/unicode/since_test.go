package main

import (
	"fmt"
	"maps"
	"slices"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// SinceSample is a pair of snapshots and an optional explicit check selection.
//
//uc:realizes cli.SinceSample
type SinceSample struct {
	Base     *graph.Graph
	Head     *graph.Graph
	Selected []string
}

//uc:generates cli.changed-checks-are-selected.sample
func genSinceSample() *rapid.Generator[SinceSample] {
	return rapid.Custom(func(t *rapid.T) SinceSample {
		var nodes []graph.Node
		var edges []graph.Edge
		addNode := func(id string, kind graph.NodeKind, layer graph.Layer, attrs map[string][]string) {
			nodes = append(nodes, graph.Node{Id: id, Kind: kind, Layer: layer, Form: string(kind), Source: graph.SourceDeclared, Salient: layer == graph.LayerDesign, Attrs: attrs})
		}
		addEdge := func(kind graph.EdgeKind, from, to string) {
			edges = append(edges, graph.Edge{Kind: kind, From: from, To: to, Source: graph.SourceDeclared})
		}
		count := rapid.IntRange(0, 4).Draw(t, "groups")
		addNode("model", graph.NodeKindContainer, graph.LayerDesign, nil)
		addNode("model.unaffected", graph.NodeKindProperty, graph.LayerDesign, nil)
		addEdge(graph.EdgeKindContains, "model", "model.unaffected")
		for i := range count {
			op, property, flow := fmt.Sprintf("model.op%d", i), fmt.Sprintf("model.check-%d", i), fmt.Sprintf("model.flow-%d", i)
			native, predicate, generator, adapter, helper, flowPredicate := sinceCode(i, "native"), sinceCode(i, "predicate"), sinceCode(i, "generator"), sinceCode(i, "observer"), sinceCode(i, "helper"), sinceCode(i, "flowPredicate")
			addNode(op, graph.NodeKindOp, graph.LayerDesign, nil)
			addNode(property, graph.NodeKindProperty, graph.LayerDesign, map[string][]string{"ensure": {"true"}})
			addNode(flow, graph.NodeKindFlow, graph.LayerDesign, nil)
			addNode(property+".state", graph.NodeKindObservation, graph.LayerDesign, map[string][]string{"type": {"int"}})
			addNode(property+".case", graph.NodeKindCase, graph.LayerDesign, map[string][]string{"values": {"value = 0"}})
			addNode(flow+".case", graph.NodeKindCase, graph.LayerDesign, map[string][]string{"values": {"value = 0"}})
			for _, id := range []string{op, property, flow} {
				addEdge(graph.EdgeKindContains, "model", id)
			}
			addEdge(graph.EdgeKindContains, property, property+".state")
			addEdge(graph.EdgeKindContains, property, property+".case")
			addEdge(graph.EdgeKindContains, flow, flow+".case")
			for _, id := range []string{native, predicate, generator, adapter, helper, flowPredicate} {
				addNode(id, graph.NodeKindOp, graph.LayerCode, map[string][]string{"body": {"old"}})
			}
			addEdge(graph.EdgeKindRealizes, native, op)
			addEdge(graph.EdgeKindChecks, predicate, property)
			addEdge(graph.EdgeKindGenerates, generator, property)
			addEdge(graph.EdgeKindObserves, adapter, property+".state")
			addEdge(graph.EdgeKindChecks, flowPredicate, flow)
			addEdge(graph.EdgeKindAbout, property, op)
			addEdge(graph.EdgeKindCalls, flowPredicate, helper)
			addEdge(graph.EdgeKindCalls, helper, native)
			addEdge(graph.EdgeKindCalls, native, helper)
			if rapid.Bool().Draw(t, "static predicate call") {
				addEdge(graph.EdgeKindCalls, predicate, native)
			}
		}
		addNode("go:sample.isolated", graph.NodeKindOp, graph.LayerCode, nil)
		base := sinceGraph(t, nodes, edges)
		for i := range count {
			op, property, flow := fmt.Sprintf("model.op%d", i), fmt.Sprintf("model.check-%d", i), fmt.Sprintf("model.flow-%d", i)
			setAttr := func(id, key, value string) {
				for j := range nodes {
					if nodes[j].Id == id {
						if nodes[j].Attrs == nil {
							nodes[j].Attrs = map[string][]string{}
						}
						nodes[j].Attrs[key] = []string{value}
					}
				}
			}
			switch rapid.IntRange(0, 17).Draw(t, "mutation") {
			case 0:
			case 1:
				setAttr(sinceCode(i, "native"), "body", "new")
			case 2:
				setAttr(property, "ensure", "false")
			case 3:
				for j := range nodes {
					if nodes[j].Id == flow {
						nodes[j].Summary = "Changed flow"
					}
				}
			case 4:
				nodes, edges = sinceRemove(nodes, edges, map[string]bool{sinceCode(i, "native"): true})
			case 5:
				id := sinceCode(i, "newNative")
				addNode(id, graph.NodeKindOp, graph.LayerCode, map[string][]string{"body": {"new"}})
				addEdge(graph.EdgeKindRealizes, id, op)
			case 6:
				setAttr(property+".case", "values", "value = 1")
			case 7:
				nodes, edges = sinceRemove(nodes, edges, map[string]bool{property + ".case": true})
			case 8:
				id := property + ".new-case"
				addNode(id, graph.NodeKindCase, graph.LayerDesign, map[string][]string{"values": {"value = 1"}})
				addEdge(graph.EdgeKindContains, property, id)
			case 9:
				setAttr(flow+".case", "values", "value = 1")
			case 10:
				setAttr(sinceCode(i, "observer"), "body", "new")
			case 11:
				setAttr(sinceCode(i, "predicate"), "body", "new")
			case 12:
				setAttr(sinceCode(i, "generator"), "body", "new")
			case 13:
				edges = slices.DeleteFunc(edges, func(e graph.Edge) bool {
					return e.Kind == graph.EdgeKindCalls && e.From == sinceCode(i, "helper") && e.To == sinceCode(i, "native")
				})
			case 14:
				for j := range edges {
					if edges[j].Kind == graph.EdgeKindRealizes && edges[j].From == sinceCode(i, "native") {
						edges[j].Attrs = map[string][]string{"relation": {"changed"}}
					}
				}
			case 15:
				nodes, edges = sinceRemove(nodes, edges, map[string]bool{property: true, property + ".state": true, property + ".case": true})
			case 16:
				for j := range nodes {
					if nodes[j].Id == sinceCode(i, "native") {
						nodes[j].Origin = diag.Position{Path: "moved.go", Line: 7, Column: 2}
						nodes[j].Source = graph.SourceProduced
						nodes[j].Salient = true
					}
				}
			case 17:
				id := property + "-new"
				addNode(id, graph.NodeKindProperty, graph.LayerDesign, map[string][]string{"ensure": {"true"}})
				addEdge(graph.EdgeKindContains, "model", id)
			}
		}
		if rapid.Bool().Draw(t, "isolated deletion") {
			nodes, edges = sinceRemove(nodes, edges, map[string]bool{"go:sample.isolated": true})
		}
		head := sinceGraph(t, nodes, edges)
		selected := []string{}
		if rapid.Bool().Draw(t, "explicit selection") {
			for _, n := range head.Nodes() {
				if n.Layer == graph.LayerDesign && (n.Kind == graph.NodeKindProperty || n.Kind == graph.NodeKindFlow) && rapid.Bool().Draw(t, "select "+n.Id) {
					selected = append(selected, n.Id)
					if rapid.Bool().Draw(t, "repeat selection") {
						selected = append(selected, n.Id)
					}
				}
			}
		}
		if rapid.IntRange(0, 9).Draw(t, "empty graph") == 0 {
			empty := graph.NewGraph()
			base, head = &empty, &empty
			selected = nil
		}
		return SinceSample{Base: base, Head: head, Selected: selected}
	})
}

func sinceCode(group int, role string) string { return fmt.Sprintf("go:sample.%s%d", role, group) }

func sinceGraph(t rapid.TB, nodes []graph.Node, edges []graph.Edge) *graph.Graph {
	g := graph.NewGraph()
	if err := g.AddFragment(graph.Fragment{Nodes: nodes, Edges: edges}); err != nil {
		t.Fatalf("the generated snapshots are not graphs: %v", err)
	}
	return &g
}

func sinceRemove(nodes []graph.Node, edges []graph.Edge, remove map[string]bool) ([]graph.Node, []graph.Edge) {
	nodes = slices.DeleteFunc(nodes, func(n graph.Node) bool { return remove[n.Id] })
	edges = slices.DeleteFunc(edges, func(e graph.Edge) bool { return remove[e.From] || remove[e.To] })
	return nodes, edges
}

type sinceEdgeKey struct {
	kind     graph.EdgeKind
	from, to string
}

//uc:checks cli.changed-checks-are-selected
func checkChangedChecksAreSelected(t rapid.TB, sample SinceSample) {
	baseNodes, headNodes := map[string]graph.Node{}, map[string]graph.Node{}
	baseEdges, headEdges := map[sinceEdgeKey]graph.Edge{}, map[sinceEdgeKey]graph.Edge{}
	for _, n := range sample.Base.Nodes() {
		baseNodes[n.Id] = n
	}
	for _, n := range sample.Head.Nodes() {
		headNodes[n.Id] = n
	}
	for _, e := range sample.Base.Edges() {
		baseEdges[sinceEdgeKey{e.Kind, e.From, e.To}] = e
	}
	for _, e := range sample.Head.Edges() {
		headEdges[sinceEdgeKey{e.Kind, e.From, e.To}] = e
	}
	changed := map[string]bool{}
	for id, before := range baseNodes {
		after, exists := headNodes[id]
		if !exists || before.Kind != after.Kind || before.Form != after.Form || before.Summary != after.Summary || !sameSinceAttrs(before.Attrs, after.Attrs) {
			changed[id] = true
		}
	}
	for id := range headNodes {
		if _, exists := baseNodes[id]; !exists {
			changed[id] = true
		}
	}
	for key, before := range baseEdges {
		after, exists := headEdges[key]
		if !exists || !sameSinceAttrs(before.Attrs, after.Attrs) {
			changed[key.from], changed[key.to] = true, true
		}
	}
	for key := range headEdges {
		if _, exists := baseEdges[key]; !exists {
			changed[key.from], changed[key.to] = true, true
		}
	}
	wanted := map[string]bool{}
	for i, nodes := range []map[string]graph.Node{baseNodes, headNodes} {
		edges := baseEdges
		if i == 1 {
			edges = headEdges
		}
		reached := map[string]bool{}
		for id := range changed {
			if _, exists := nodes[id]; exists {
				reached[id] = true
			}
		}
		for more := true; more; {
			more = false
			for _, edge := range edges {
				reverse := false
				switch string(edge.Kind) {
				case "calls", "reaches", "refers", "subscribes", "emits", "realizes", "traces", "exercises", "depends", "about", "checks", "observes", "substitutes":
					reverse = true
				}
				owner := edge.Kind == graph.EdgeKindContains && nodes[edge.From].Kind == graph.NodeKindProperty && nodes[edge.To].Kind == graph.NodeKindObservation
				if (reverse || owner) && reached[edge.To] && !reached[edge.From] {
					reached[edge.From] = true
					more = true
				}
				binding := false
				switch string(edge.Kind) {
				case "realizes", "checks", "generates", "observes", "substitutes":
					binding = true
				}
				if binding && reached[edge.From] && !reached[edge.To] {
					reached[edge.To] = true
					more = true
				}
			}
		}
		for id := range reached {
			n := nodes[id]
			if n.Layer == graph.LayerDesign && (n.Kind == graph.NodeKindProperty || n.Kind == graph.NodeKindFlow) {
				wanted[id] = true
			}
		}
		for _, edge := range edges {
			if edge.Kind == graph.EdgeKindContains && changed[edge.To] && nodes[edge.To].Kind == graph.NodeKindCase {
				parent := nodes[edge.From]
				if parent.Layer == graph.LayerDesign && (parent.Kind == graph.NodeKindProperty || parent.Kind == graph.NodeKindFlow) {
					wanted[parent.Id] = true
				}
			}
		}
	}
	want := []string{}
	for id := range wanted {
		n, exists := headNodes[id]
		if exists && n.Layer == graph.LayerDesign && (n.Kind == graph.NodeKindProperty || n.Kind == graph.NodeKindFlow) && (len(sample.Selected) == 0 || slices.Contains(sample.Selected, id)) {
			want = append(want, id)
		}
	}
	slices.Sort(want)
	beforeBase, beforeHead := sample.Base.Encode(), sample.Head.Encode()
	got := AffectedChecks(sample.Base, sample.Head, sample.Selected)
	if got == nil || !slices.Equal(got, want) {
		t.Fatalf("AffectedChecks = %v (nil=%t), want %v; selected=%v", got, got == nil, want, sample.Selected)
	}
	if !slices.Equal(beforeBase, sample.Base.Encode()) || !slices.Equal(beforeHead, sample.Head.Encode()) {
		t.Fatal("AffectedChecks changed an input snapshot")
	}
}

func sameSinceAttrs(a, b map[string][]string) bool {
	return maps.EqualFunc(a, b, func(x, y []string) bool { return slices.Equal(x, y) })
}

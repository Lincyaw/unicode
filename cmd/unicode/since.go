package main

import (
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
)

// AffectedChecks selects current checks affected by semantic changes on
// either side of a comparison, narrowed by explicit identities when given.
//
//uc:realizes cli.affectedChecks
func AffectedChecks(base, head *graph.Graph, selected []string) []string {
	changes := graph.Compare(base, head)
	seeds := map[string]bool{}
	for _, change := range changes.Nodes {
		seeds[change.Id] = true
	}
	for _, change := range changes.Edges {
		seeds[change.From], seeds[change.To] = true, true
	}
	affected := map[string]bool{}
	for _, snapshot := range []*graph.Graph{base, head} {
		caseParents := map[string][]string{}
		for _, edge := range snapshot.Edges() {
			if edge.Kind == graph.EdgeKindContains {
				if parent := snapshot.Node(edge.From); parent != nil && sinceCheck(*parent) {
					caseParents[edge.To] = append(caseParents[edge.To], parent.Id)
				}
			}
		}
		var present []string
		for id := range seeds {
			if node := snapshot.Node(id); node != nil {
				present = append(present, id)
				if sinceCheck(*node) {
					affected[id] = true
				}
				if node.Kind == graph.NodeKindCase {
					for _, parent := range caseParents[id] {
						affected[parent] = true
					}
				}
			}
		}
		impact, err := query.Impact(snapshot, present)
		if err != nil {
			// The seeds above were taken only from nodes of this snapshot.
			panic("unicode: incremental impact rejected existing nodes: " + err.Error())
		}
		for _, item := range impact {
			if sinceCheck(item.Node) {
				affected[item.Node.Id] = true
			}
		}
	}
	out := []string{}
	for id := range affected {
		if node := head.Node(id); node != nil && sinceCheck(*node) && (len(selected) == 0 || slices.Contains(selected, id)) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

func sinceCheck(node graph.Node) bool {
	return node.Layer == graph.LayerDesign && (node.Kind == graph.NodeKindProperty || node.Kind == graph.NodeKindFlow)
}

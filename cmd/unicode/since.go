package main

import (
	"maps"
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
	packageFiles := map[string]bool{}
	for _, change := range changes.Nodes {
		seeds[change.Id] = true
		if !slices.Equal(sinceGlobals(change.Before), sinceGlobals(change.After)) {
			packageFiles[change.Id] = true
		}
	}
	for _, change := range changes.Edges {
		if sinceLocationsOnly(change) {
			continue
		}
		seeds[change.From], seeds[change.To] = true, true
		if change.Kind == graph.EdgeKindImports {
			packageFiles[change.From] = true
		}
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
		presentSeeds := maps.Clone(seeds)
		for _, id := range sincePackageNodes(snapshot, packageFiles) {
			presentSeeds[id] = true
		}
		var present []string
		for id := range presentSeeds {
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

// Call and import coordinates locate evidence without changing an existing dependency.
func sinceLocationsOnly(change graph.EdgeChange) bool {
	if (change.Kind != graph.EdgeKindCalls && change.Kind != graph.EdgeKindImports) || change.Change != graph.ChangeKindModified || change.Before == nil || change.After == nil {
		return false
	}
	before, after := maps.Clone(change.Before.Attrs), maps.Clone(change.After.Attrs)
	delete(before, "sites")
	delete(after, "sites")
	return maps.EqualFunc(before, after, slices.Equal[[]string])
}

// sinceGlobals is only a code file's non-node declarations. Other metadata
// must not broaden incremental replay to an entire package.
func sinceGlobals(node *graph.Node) []string {
	if node != nil && node.Layer == graph.LayerCode && node.Kind == graph.NodeKindContainer && node.Form == "file" {
		return node.Attrs["globals"]
	}
	return nil
}

// sincePackageNodes expands files with changed globals or imports to native
// declarations in their packages and transitive importing packages.
func sincePackageNodes(snapshot *graph.Graph, files map[string]bool) []string {
	if len(files) == 0 {
		return nil
	}
	nodes := map[string]graph.Node{}
	for _, node := range snapshot.Nodes() {
		nodes[node.Id] = node
	}
	children := map[string][]string{}
	owners := map[string]string{}
	edges := snapshot.Edges()
	for _, edge := range edges {
		parent, child := nodes[edge.From], nodes[edge.To]
		if edge.Kind != graph.EdgeKindContains || parent.Layer != graph.LayerCode || child.Layer != graph.LayerCode {
			continue
		}
		children[edge.From] = append(children[edge.From], edge.To)
		if parent.Kind == graph.NodeKindContainer && parent.Form == "package" && child.Kind == graph.NodeKindContainer && child.Form == "file" {
			owners[child.Id] = parent.Id
		}
	}
	importers := map[string][]string{}
	for _, edge := range edges {
		if owner := owners[edge.From]; edge.Kind == graph.EdgeKindImports && owner != "" {
			importers[edge.To] = append(importers[edge.To], owner)
		}
	}
	var packages []string
	for id := range files {
		if owner := owners[id]; owner != "" {
			packages = append(packages, owner)
		}
	}
	seenPackages, seenNodes := map[string]bool{}, map[string]bool{}
	var out []string
	for len(packages) > 0 {
		id := packages[0]
		packages = packages[1:]
		if seenPackages[id] {
			continue
		}
		seenPackages[id] = true
		packages = append(packages, importers[id]...)
		pending := []string{id}
		for len(pending) > 0 {
			child := pending[0]
			pending = pending[1:]
			if seenNodes[child] {
				continue
			}
			seenNodes[child] = true
			if node := nodes[child]; node.Kind == graph.NodeKindOp || node.Kind == graph.NodeKindType {
				out = append(out, child)
			}
			pending = append(pending, children[child]...)
		}
	}
	return out
}

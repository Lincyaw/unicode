package review

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// Sample carries independently enumerated targets and their expected validity.
//
//uc:realizes review.Sample
type Sample struct {
	Graph   *graph.Graph
	Base    *graph.Graph
	Records []Record
	Targets []Target
	Pending []Target
}

// Malformed is a graph with a review layer, the identities of its
// review-layer nodes that carry no valid record, and the records the others
// carry.
//
//uc:realizes review.Malformed
type Malformed struct {
	Graph  *graph.Graph
	Broken []string
	Valid  []Record
}

type sampleRecord struct {
	record Record
	source *graph.Graph
}

//uc:generates review.evaluation-follows-content.sample
//uc:generates review.records-preserve-history.sample
//uc:generates review.recorded-inverts-attach.sample
func genSample() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample {
		value := rapid.IntRange(-20, 20).Draw(t, "value")
		mode := rapid.IntRange(0, 13).Draw(t, "change")
		withBase := rapid.Bool().Draw(t, "base")
		before := sampleGraph(t, value)
		nodes, edges := before.Nodes(), before.Edges()
		update := func(id string, change func(*graph.Node)) {
			for index := range nodes {
				if nodes[index].Id == id {
					change(&nodes[index])
				}
			}
		}
		switch mode {
		case 1:
			nodes = append(nodes, graph.Node{Id: "domain.added", Kind: graph.NodeKindOp, Form: "op", Layer: graph.LayerDesign, Summary: fmt.Sprint(value)})
		case 2:
			nodes = slices.DeleteFunc(nodes, func(n graph.Node) bool { return n.Id == "go:sample.Helper" })
			edges = slices.DeleteFunc(edges, func(e graph.Edge) bool { return e.From == "go:sample.Helper" || e.To == "go:sample.Helper" })
		case 3:
			update("domain.auto", func(n *graph.Node) { n.Summary += " changed" })
		case 4:
			update("go:sample.Check", func(n *graph.Node) { n.Attrs["body"] = []string{fmt.Sprint(value + 1)} })
		case 5:
			update("go:sample.Observe", func(n *graph.Node) { n.Attrs["body"] = []string{fmt.Sprint(value + 1)} })
		case 6:
			update("domain.manual.state", func(n *graph.Node) { n.Summary += " changed" })
		case 7:
			update("go:sample.Helper", func(n *graph.Node) { n.Attrs["body"] = []string{fmt.Sprint(value + 1)} })
		case 8:
			update("go:sample.Helper", func(n *graph.Node) { n.Origin.Line += 5; n.Salient = !n.Salient })
		case 9:
			edges = append(edges, graph.Edge{Kind: graph.EdgeKindUses, From: "domain", To: "domain.auto", Attrs: map[string][]string{"extra": {fmt.Sprint(value)}}})
		case 10:
			edges = slices.DeleteFunc(edges, func(e graph.Edge) bool { return e.Kind == graph.EdgeKindCalls })
		case 11:
			for index := range edges {
				if edges[index].Kind == graph.EdgeKindCalls {
					edges[index].Attrs["sites"] = []string{fmt.Sprintf("source.go:%d:1", value+30)}
				}
			}
		case 12:
			update("go:sample.Generated", func(n *graph.Node) { n.Attrs["body"] = []string{fmt.Sprint(value + 1)} })
		case 13:
			for index := range edges {
				if edges[index].Kind == graph.EdgeKindChecks && edges[index].To == "domain.manual" {
					edges[index].From = "go:sample.OtherCheck"
				}
			}
		}
		head := sampleBuild(t, nodes, edges)
		var base *graph.Graph
		if withBase {
			base = before
		}
		allBefore, allHead := oracleTargets(before), oracleTargets(head)
		selected := map[Target]bool{}
		for target := range allHead {
			if base == nil || !allBefore[target] || !reflect.DeepEqual(oracleContent(before, target), oracleContent(head, target)) {
				selected[target] = true
			}
		}
		if base != nil {
			for target := range allBefore {
				if !allHead[target] {
					target.Removed = true
					selected[target] = true
				}
			}
		}
		sample := Sample{Graph: head, Base: base, Records: []Record{}, Targets: []Target{}, Pending: []Target{}}
		var provenance []sampleRecord
		for sideIndex, source := range []*graph.Graph{before, head} {
			for _, target := range sortedOracleTargets(oracleTargets(source)) {
				if !rapid.Bool().Draw(t, fmt.Sprintf("record-%d-%s", sideIndex, targetKey(target))) {
					continue
				}
				r := sampleReview(source, target, fmt.Sprintf("2026-11-27T10:00:%02dZ", sideIndex), "approved")
				sample.Records = append(sample.Records, r)
				provenance = append(provenance, sampleRecord{r, source})
			}
		}
		for _, target := range sortedOracleTargets(selected) {
			source := head
			if target.Removed {
				source = before
			}
			if target.Removed && rapid.Bool().Draw(t, "review-removed") {
				r := sampleReview(source, target, "2026-11-27T10:00:02Z", "changes-requested")
				sample.Records = append(sample.Records, r)
				provenance = append(provenance, sampleRecord{r, source})
			}
			reviewed := false
			for _, past := range provenance {
				if past.record.Target == target && reflect.DeepEqual(oracleContent(past.source, target), oracleContent(source, target)) {
					reviewed = true
				}
			}
			sample.Targets = append(sample.Targets, target)
			if !reviewed {
				sample.Pending = append(sample.Pending, target)
			}
		}
		return sample
	})
}

func sampleReview(g *graph.Graph, target Target, at, decision string) Record {
	// The identity and digest make this a legal input record. Expected validity
	// below is computed from model content, not from these production digests.
	r := Record{Version: 1, Reviewer: "reviewer", At: at, Snapshot: graph.InputDigest(g), Target: target, Digest: indexContent(g).item(target, nil).Digest, Decision: decision, Comment: "sample review"}
	r.Id = recordId(r)
	return r
}

func sampleGraph(t rapid.TB, value int) *graph.Graph {
	nodes := []graph.Node{
		{Id: "domain", Kind: graph.NodeKindContainer, Form: "system", Layer: graph.LayerDesign},
		{Id: "domain.manual", Kind: graph.NodeKindProperty, Form: "invariant", Layer: graph.LayerDesign, Attrs: map[string][]string{"manual": {"requires an independent oracle"}}},
		{Id: "domain.auto", Kind: graph.NodeKindProperty, Form: "invariant", Layer: graph.LayerDesign},
		{Id: "domain.flow", Kind: graph.NodeKindFlow, Form: "flow", Layer: graph.LayerDesign, Attrs: map[string][]string{"manual": {"requires an environment"}}},
		{Id: "domain.manual.state", Kind: graph.NodeKindObservation, Form: "snapshot", Layer: graph.LayerDesign},
		{Id: "domain.manual.case", Kind: graph.NodeKindCase, Form: "declared", Layer: graph.LayerDesign},
		{Id: "file:source.go", Kind: graph.NodeKindContainer, Form: "file", Layer: graph.LayerCode, Attrs: map[string][]string{"path": {"source.go"}}},
		{Id: "file:generated_test.go", Kind: graph.NodeKindContainer, Form: "file", Layer: graph.LayerCode, Attrs: map[string][]string{"generated": {"true"}, "path": {"generated_test.go"}}},
		{Id: "run:old", Kind: graph.NodeKindRun, Layer: graph.LayerEvidence},
		{Id: "review:old", Kind: graph.NodeKindReview, Layer: graph.LayerReview},
	}
	for _, name := range []string{"Check", "FlowCheck", "OtherCheck", "Observe", "Helper", "Generated"} {
		nodes = append(nodes, graph.Node{Id: "go:sample." + name, Kind: graph.NodeKindOp, Form: "func", Layer: graph.LayerCode, Source: graph.SourceExtracted, Origin: diag.Position{Path: "source.go", Line: 3, Column: 1}, Attrs: map[string][]string{"body": {fmt.Sprint(value)}}})
	}
	edges := []graph.Edge{
		{Kind: graph.EdgeKindContains, From: "domain", To: "domain.manual"},
		{Kind: graph.EdgeKindContains, From: "domain", To: "domain.auto"},
		{Kind: graph.EdgeKindContains, From: "domain", To: "domain.flow"},
		{Kind: graph.EdgeKindContains, From: "domain.manual", To: "domain.manual.state"},
		{Kind: graph.EdgeKindContains, From: "domain.manual", To: "domain.manual.case"},
		{Kind: graph.EdgeKindChecks, From: "go:sample.Check", To: "domain.manual"},
		{Kind: graph.EdgeKindChecks, From: "go:sample.FlowCheck", To: "domain.flow"},
		{Kind: graph.EdgeKindObserves, From: "go:sample.Observe", To: "domain.manual.state"},
		{Kind: graph.EdgeKindCalls, From: "go:sample.Helper", To: "go:sample.Observe", Attrs: map[string][]string{"sites": {"source.go:3:1"}}},
		{Kind: graph.EdgeKindCovers, From: "domain.manual.case", To: "go:sample.Helper", Source: graph.SourceProduced},
	}
	for _, name := range []string{"Check", "FlowCheck", "OtherCheck", "Observe", "Helper", "Generated"} {
		file := "file:source.go"
		if name == "Generated" {
			file = "file:generated_test.go"
		}
		edges = append(edges, graph.Edge{Kind: graph.EdgeKindContains, From: file, To: "go:sample." + name})
	}
	return sampleBuild(t, nodes, edges)
}

func sampleBuild(t rapid.TB, nodes []graph.Node, edges []graph.Edge) *graph.Graph {
	g := graph.NewGraph()
	for _, n := range nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatal(err)
		}
	}
	return &g
}

func oracleTargets(g *graph.Graph) map[Target]bool {
	nodes := map[string]graph.Node{}
	parents := map[string]string{}
	for _, n := range g.Nodes() {
		nodes[n.Id] = n
	}
	for _, e := range g.Edges() {
		if e.Kind == graph.EdgeKindContains {
			parents[e.To] = e.From
		}
	}
	usable := func(id string) bool {
		n := nodes[id]
		if n.Layer != graph.LayerDesign && n.Layer != graph.LayerCode {
			return false
		}
		seen := map[string]bool{}
		for id != "" && !seen[id] {
			seen[id] = true
			if nodes[id].Layer == graph.LayerCode && slices.Contains(nodes[id].Attrs["generated"], "true") {
				return false
			}
			id = parents[id]
		}
		return true
	}
	out := map[Target]bool{}
	for id := range nodes {
		if usable(id) {
			out[Target{Node: id}] = true
		}
	}
	for _, e := range g.Edges() {
		if e.Kind != graph.EdgeKindCovers && usable(e.From) && usable(e.To) {
			out[Target{EdgeKind: string(e.Kind), From: e.From, To: e.To}] = true
		}
	}
	return out
}

func sortedOracleTargets(targets map[Target]bool) []Target {
	out := slices.Collect(maps.Keys(targets))
	slices.SortFunc(out, func(a, b Target) int { return strings.Compare(targetKey(a), targetKey(b)) })
	return out
}

func oracleAttrs(attrs map[string][]string) map[string][]string {
	copy := map[string][]string{}
	for key, values := range attrs {
		copy[key] = append([]string{}, values...)
	}
	return copy
}

func oracleContent(g *graph.Graph, target Target) any {
	nodeContent := func(id string) any {
		n := g.Node(id)
		if n == nil {
			return nil
		}
		return []any{n.Kind, n.Form, n.Summary, oracleAttrs(n.Attrs)}
	}
	edges := g.Edges()
	if target.Node == "" {
		for _, e := range edges {
			if string(e.Kind) == target.EdgeKind && e.From == target.From && e.To == target.To {
				return []any{target, oracleAttrs(e.Attrs)}
			}
		}
		return nil
	}
	body := []any{target, nodeContent(target.Node)}
	node := g.Node(target.Node)
	if node == nil {
		return nil
	}
	if node.Kind != graph.NodeKindProperty && node.Kind != graph.NodeKindFlow {
		return body
	}
	links := map[string]any{}
	for _, e := range edges {
		if e.Kind == graph.EdgeKindChecks && e.To == node.Id && len(node.Attrs["manual"]) > 0 {
			links["check "+e.From] = []any{nodeContent(e.From), oracleAttrs(e.Attrs)}
		}
		if e.Kind == graph.EdgeKindContains && e.From == node.Id && node.Kind == graph.NodeKindProperty {
			obs := g.Node(e.To)
			if obs == nil || obs.Kind != graph.NodeKindObservation {
				continue
			}
			links["observation "+obs.Id] = nodeContent(obs.Id)
			for _, adapter := range edges {
				if adapter.Kind == graph.EdgeKindObserves && adapter.To == obs.Id {
					links["observe "+adapter.From+" "+obs.Id] = []any{nodeContent(adapter.From), oracleAttrs(adapter.Attrs)}
				}
			}
		}
	}
	return append(body, links)
}

// withoutReviews is g without its review-layer nodes and the edges at them.
func withoutReviews(g *graph.Graph) graph.Graph {
	out := graph.NewGraph()
	for _, node := range g.Nodes() {
		if node.Layer == graph.LayerReview {
			continue
		}
		if err := out.AddNode(node); err != nil {
			panic(err)
		}
	}
	for _, edge := range g.Edges() {
		if out.Node(edge.From) == nil || out.Node(edge.To) == nil {
			continue
		}
		if err := out.AddEdge(edge); err != nil {
			panic(err)
		}
	}
	return out
}

// drawRecord draws a valid record about one of targets: marked removed now
// and then, by one of three reviewers, with either decision, at a random
// second and nanosecond of 2026-11-27, with an empty, 40- or 64-digit commit,
// either dirty flag, an empty, English or Chinese-and-emoji comment, and
// random 64-digit content and input digests.
func drawRecord(t *rapid.T, targets []Target) Record {
	hex := func(size int, label string) string {
		return rapid.StringOfN(rapid.RuneFrom([]rune("0123456789abcdef")), size, size, -1).Draw(t, label)
	}
	target := rapid.SampledFrom(targets).Draw(t, "target")
	target.Removed = rapid.IntRange(0, 3).Draw(t, "removed?") == 0
	at := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC).Add(time.Duration(rapid.IntRange(0, 86399).Draw(t, "second")) * time.Second).
		Add(time.Duration(rapid.IntRange(0, 999999999).Draw(t, "nanosecond")))
	r := Record{Version: 1, Reviewer: rapid.SampledFrom([]string{"reviewer", "张三", "review-bot"}).Draw(t, "reviewer"), At: at.Format(time.RFC3339Nano),
		Snapshot: hex(64, "snapshot"), Commit: hex(rapid.SampledFrom([]int{0, 40, 64}).Draw(t, "commitSize"), "commit"), Dirty: rapid.Bool().Draw(t, "dirty"),
		Target: target, Digest: hex(64, "digest"), Decision: rapid.SampledFrom([]string{"approved", "changes-requested"}).Draw(t, "decision"),
		Comment: rapid.SampledFrom([]string{"", "looks right", "字段名与代码一致 ✅"}).Draw(t, "comment")}
	r.Id = recordId(r)
	return r
}

// genRecords draws zero to eight valid records about the nodes and edges of
// the sample's graph.
//
//uc:generates review.recorded-inverts-attach.records
func genRecords(sample Sample) *rapid.Generator[[]Record] {
	targets := sortedOracleTargets(oracleTargets(sample.Graph))
	return rapid.Custom(func(t *rapid.T) []Record {
		records := []Record{}
		for range rapid.IntRange(0, 8).Draw(t, "records") {
			records = append(records, drawRecord(t, targets))
		}
		return records
	})
}

// genProjected is the sample's graph without its review layer, as a build
// has it before attaching, with the review layer that Attach projects from
// records.
//
//uc:generates review.recorded-inverts-attach.projected
func genProjected(sample Sample, records []Record) *rapid.Generator[*graph.Graph] {
	projected := withoutReviews(sample.Graph)
	attached := Attach(&projected, records)
	if len(attached.Diagnostics) > 0 {
		panic(fmt.Sprintf("cannot attach records that are valid by construction: %v", attached.Diagnostics))
	}
	if err := projected.AddFragment(attached.Fragment); err != nil {
		panic(err)
	}
	return rapid.Just(&projected)
}

// genMalformed draws a small graph with the review layer of one to six valid
// records, of which at least one node is damaged in one way: an attribute
// missing, an attribute with two values, version 2, a dirty or removed flag
// that is not a boolean, an identity without the review: prefix, a changed
// comment that no longer hashes to the identity, or a time that is not
// canonical UTC.
//
//uc:generates review.recorded-reports-malformed-nodes.malformed
func genMalformed() *rapid.Generator[Malformed] {
	return rapid.Custom(func(t *rapid.T) Malformed {
		g := withoutReviews(sampleGraph(t, 0))
		targets := sortedOracleTargets(oracleTargets(&g))
		byID := map[string]Record{}
		for range rapid.IntRange(1, 6).Draw(t, "records") {
			r := drawRecord(t, targets)
			byID[r.Id] = r
		}
		records := slices.Collect(maps.Values(byID))
		attached := Attach(&g, records)
		if len(attached.Diagnostics) > 0 {
			panic(fmt.Sprintf("cannot attach records that are valid by construction: %v", attached.Diagnostics))
		}
		nodes, edges := attached.Fragment.Nodes, attached.Fragment.Edges
		damaged := make([]bool, len(nodes))
		for i := range nodes {
			damaged[i] = rapid.Bool().Draw(t, "damaged?")
		}
		if !slices.Contains(damaged, true) {
			damaged[rapid.IntRange(0, len(nodes)-1).Draw(t, "damagedNode")] = true
		}
		out := Malformed{Broken: []string{}, Valid: []Record{}}
		for i, node := range nodes {
			id := strings.TrimPrefix(node.Id, "review:")
			if !damaged[i] {
				out.Valid = append(out.Valid, byID[id])
				continue
			}
			attrs := maps.Clone(node.Attrs)
			keys := slices.Sorted(maps.Keys(attrs))
			switch rapid.IntRange(0, 7).Draw(t, "damage") {
			case 0:
				delete(attrs, rapid.SampledFrom(keys).Draw(t, "missing"))
			case 1:
				key := rapid.SampledFrom(keys).Draw(t, "doubled")
				attrs[key] = append(slices.Clone(attrs[key]), "again")
			case 2:
				attrs["version"] = []string{"2"}
			case 3:
				attrs[rapid.SampledFrom([]string{"dirty", "removed"}).Draw(t, "flag")] = []string{"maybe"}
			case 4:
				renamed := "record:" + id
				for j := range edges {
					if edges[j].From == node.Id {
						edges[j].From = renamed
					}
				}
				node.Id = renamed
			case 5:
				attrs["comment"] = []string{attrs["comment"][0] + " changed"}
			default:
				attrs["at"] = []string{strings.Replace(attrs["at"][0], "Z", "+00:00", 1)}
			}
			node.Attrs = attrs
			nodes[i] = node
			out.Broken = append(out.Broken, node.Id)
		}
		if err := g.AddFragment(graph.Fragment{Nodes: nodes, Edges: edges}); err != nil {
			panic(err)
		}
		out.Graph = &g
		return out
	})
}

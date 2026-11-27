package review

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// Target identifies exactly one node or edge, possibly removed from the head.
//
//uc:realizes review.Target
type Target struct {
	Node     string `json:"node"`
	EdgeKind string `json:"edgeKind"`
	From     string `json:"from"`
	To       string `json:"to"`
	Removed  bool   `json:"removed"`
}

// Record is immutable; Id hashes the canonical JSON of all other fields.
//
//uc:realizes review.Record
type Record struct {
	Version  int    `json:"version"`
	Id       string `json:"id"`
	Reviewer string `json:"reviewer"`
	At       string `json:"at"`
	Snapshot string `json:"snapshot"`
	Commit   string `json:"commit"`
	Dirty    bool   `json:"dirty"`
	Target   Target `json:"target"`
	Digest   string `json:"digest"`
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

// Item is a current target and its latest matching review, if any.
//
//uc:realizes review.Item
type Item struct {
	Target Target  `json:"target"`
	Digest string  `json:"digest"`
	State  string  `json:"state"`
	Record *Record `json:"record"`
}

// Loaded contains immutable records and errors from their store.
//
//uc:realizes review.Loaded
type Loaded struct {
	Records     []Record          `json:"records"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// Evaluation lists the candidate targets in identity order.
//
//uc:realizes review.Evaluation
type Evaluation struct {
	Items       []Item            `json:"items"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// Request supplies review metadata and an optional expected content digest.
//
//uc:realizes review.Request
type Request struct {
	Target         Target `json:"target"`
	Reviewer       string `json:"reviewer"`
	Decision       string `json:"decision"`
	Comment        string `json:"comment"`
	ExpectedDigest string `json:"expectedDigest"`
	Commit         string `json:"commit"`
	Dirty          bool   `json:"dirty"`
}

// Written contains the stored record, or errors without a new record.
//
//uc:realizes review.Written
type Written struct {
	Record      *Record           `json:"record"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// Attached contains only produced review-layer nodes and about edges.
//
//uc:realizes review.Attached
type Attached struct {
	Fragment    graph.Fragment    `json:"fragment"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

func digest(value any) string {
	sum := sha256.Sum256(mustJSON(value))
	return hex.EncodeToString(sum[:])
}

func targetSubject(target Target) string {
	if target.Node != "" {
		return target.Node
	}
	return target.EdgeKind + " " + target.From + " " + target.To
}

func targetKey(target Target) string {
	return string(mustJSON(target))
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic("review: cannot encode internal review data: " + err.Error())
	}
	return data
}

func compareTargets(a, b Target) int {
	return cmp.Or(cmp.Compare(a.Node, b.Node), cmp.Compare(a.EdgeKind, b.EdgeKind),
		cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To), cmp.Compare(boolInt(a.Removed), boolInt(b.Removed)))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func eligible(g *graph.Graph) map[string]bool {
	out, generated := map[string]bool{}, map[string]bool{}
	for _, n := range g.Nodes() {
		if n.Layer == graph.LayerCode && slices.Contains(n.Attrs["generated"], "true") {
			generated[n.Id] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, e := range g.Edges() {
			if e.Kind == graph.EdgeKindContains && generated[e.From] && !generated[e.To] {
				generated[e.To], changed = true, true
			}
		}
	}
	for _, n := range g.Nodes() {
		if (n.Layer == graph.LayerDesign || n.Layer == graph.LayerCode) && !generated[n.Id] {
			out[n.Id] = true
		}
	}
	return out
}

func candidates(g *graph.Graph) map[string]Target {
	out := map[string]Target{}
	allowed := eligible(g)
	for id := range allowed {
		target := Target{Node: id}
		out[targetKey(target)] = target
	}
	for _, edge := range g.Edges() {
		if allowed[edge.From] && allowed[edge.To] && edge.Kind != graph.EdgeKindCovers {
			target := Target{EdgeKind: string(edge.Kind), From: edge.From, To: edge.To}
			out[targetKey(target)] = target
		}
	}
	return out
}

func edgeOf(g *graph.Graph, target Target) (graph.Edge, bool) {
	for _, edge := range g.Edges() {
		if string(edge.Kind) == target.EdgeKind && edge.From == target.From && edge.To == target.To {
			return edge, true
		}
	}
	return graph.Edge{}, false
}

func fingerprint(g *graph.Graph, target Target) string {
	if target.Node == "" {
		edge, found := edgeOf(g, target)
		if !found {
			return ""
		}
		return graph.EdgeDigest(edge)
	}
	node := g.Node(target.Node)
	if node == nil {
		return ""
	}
	own := g.Digest(node.Id)
	if node.Layer != graph.LayerDesign || node.Kind != graph.NodeKindProperty && node.Kind != graph.NodeKindFlow {
		return own
	}
	parts := map[string]string{}
	for _, edge := range g.Edges() {
		if len(node.Attrs["manual"]) > 0 && edge.Kind == graph.EdgeKindChecks && edge.To == node.Id {
			parts["predicate:"+edge.From] = g.Digest(edge.From)
			parts["binding:"+graph.EdgeDigest(edge)] = graph.EdgeDigest(edge)
		}
		if node.Kind == graph.NodeKindProperty && edge.Kind == graph.EdgeKindContains && edge.From == node.Id {
			observation := g.Node(edge.To)
			if observation == nil || observation.Kind != graph.NodeKindObservation {
				continue
			}
			parts["observation:"+observation.Id] = g.Digest(observation.Id)
			for _, binding := range g.Edges() {
				if binding.Kind == graph.EdgeKindObserves && binding.To == observation.Id {
					parts["adapter:"+binding.From] = g.Digest(binding.From)
					parts["binding:"+graph.EdgeDigest(binding)] = graph.EdgeDigest(binding)
				}
			}
		}
	}
	if len(parts) == 0 {
		return own
	}
	return digest(struct {
		Own   string            `json:"own"`
		Parts map[string]string `json:"parts"`
	}{own, parts})
}

func newer(a, b Record) bool {
	at, _ := time.Parse(time.RFC3339Nano, a.At)
	bt, _ := time.Parse(time.RFC3339Nano, b.At)
	return cmp.Or(at.Compare(bt), cmp.Compare(a.Id, b.Id)) > 0
}

func item(g *graph.Graph, target Target, records []Record) Item {
	current := Item{Target: target, Digest: fingerprint(g, target), State: "pending"}
	if target.Removed {
		copy := target
		copy.Removed = false
		current.Digest = digest(struct {
			Target Target `json:"target"`
			Before string `json:"before"`
		}{target, fingerprint(g, copy)})
	}
	for _, record := range records {
		if record.Target != target || record.Digest != current.Digest {
			continue
		}
		if current.Record == nil || newer(record, *current.Record) {
			copy := record
			current.Record, current.State = &copy, "reviewed"
		}
	}
	return current
}

// Evaluate selects current or changed targets and computes review validity.
//
//uc:realizes review.evaluate
func Evaluate(g *graph.Graph, base *graph.Graph, records []Record) Evaluation {
	out := Evaluation{Items: []Item{}, Diagnostics: []diag.Diagnostic{}}
	for _, record := range records {
		if reason := validateRecord(record); reason != "" {
			out.Diagnostics = append(out.Diagnostics, issue("review.invalid-record", recordPath(record.Id), record.Id, reason, "restore the immutable review record or remove the corrupt file"))
		}
	}
	if len(out.Diagnostics) > 0 {
		return out
	}
	current := candidates(g)
	selected := map[string]Target{}
	if base == nil {
		selected = current
	} else {
		previous := candidates(base)
		for key, target := range current {
			if _, existed := previous[key]; !existed || fingerprint(g, target) != fingerprint(base, target) {
				selected[key] = target
			}
		}
		for key, target := range previous {
			if _, exists := current[key]; exists {
				continue
			}
			missing := target.Node != "" && g.Node(target.Node) == nil
			if target.Node == "" {
				_, found := edgeOf(g, target)
				missing = !found
			}
			if missing {
				target.Removed = true
				selected[targetKey(target)] = target
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(selected)) {
		target := selected[key]
		view := g
		if target.Removed {
			view = base
		}
		out.Items = append(out.Items, item(view, target, records))
	}
	slices.SortFunc(out.Items, func(a, b Item) int { return compareTargets(a.Target, b.Target) })
	return out
}

// Attach projects immutable records into the review layer without changing g.
//
//uc:realizes review.attach
func Attach(g *graph.Graph, records []Record) Attached {
	out := Attached{Fragment: graph.Fragment{Nodes: []graph.Node{}, Edges: []graph.Edge{}}, Diagnostics: []diag.Diagnostic{}}
	seen := map[string]bool{}
	for _, record := range records {
		if reason := validateRecord(record); reason != "" {
			out.Diagnostics = append(out.Diagnostics, issue("review.invalid-record", recordPath(record.Id), record.Id, reason, "restore the immutable review record or remove the corrupt file"))
			continue
		}
		if seen[record.Id] {
			continue
		}
		seen[record.Id] = true
		attrs := map[string][]string{"version": {fmt.Sprint(record.Version)}, "reviewer": {record.Reviewer}, "at": {record.At}, "snapshot": {record.Snapshot}, "commit": {record.Commit}, "dirty": {fmt.Sprint(record.Dirty)}, "targetNode": {record.Target.Node}, "targetEdgeKind": {record.Target.EdgeKind}, "targetFrom": {record.Target.From}, "targetTo": {record.Target.To}, "removed": {fmt.Sprint(record.Target.Removed)}, "digest": {record.Digest}, "decision": {record.Decision}, "comment": {record.Comment}}
		id := "review:" + record.Id
		at := diag.Position{Path: recordPath(record.Id), Line: 1, Column: 1}
		out.Fragment.Nodes = append(out.Fragment.Nodes, graph.Node{Id: id, Kind: graph.NodeKindReview, Form: "record", Layer: graph.LayerReview, Source: graph.SourceProduced, Origin: at, Summary: record.Comment, Attrs: attrs})
		ends := []string{record.Target.Node}
		if record.Target.Node == "" {
			ends = []string{record.Target.From, record.Target.To}
		}
		for _, end := range slices.Compact(slices.Sorted(slices.Values(ends))) {
			if g.Node(end) != nil {
				out.Fragment.Edges = append(out.Fragment.Edges, graph.Edge{Kind: graph.EdgeKindAbout, From: id, To: end, Source: graph.SourceProduced, Origin: at})
			}
		}
	}
	slices.SortFunc(out.Fragment.Nodes, func(a, b graph.Node) int { return cmp.Compare(a.Id, b.Id) })
	slices.SortFunc(out.Fragment.Edges, func(a, b graph.Edge) int { return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To)) })
	return out
}

func issue(code, path, subject, cause, fix string) diag.Diagnostic {
	if path == "" {
		path = "design/system.uc"
	}
	return diag.Diagnostic{Code: code, Subject: subject, Position: diag.Position{Path: path, Line: 1, Column: 1}, Message: cause, Fix: fix}
}

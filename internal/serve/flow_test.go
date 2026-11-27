package serve

import (
	"bytes"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/review"
)

// shrunk is the graph without its last node other than the system, and
// without the edges at that node: another graph to rewrite the snapshot with.
func shrunk(f graph.Fragment) graph.Fragment {
	gone := ""
	for i := len(f.Nodes) - 1; i >= 0 && gone == ""; i-- {
		if f.Nodes[i].Form != "system" {
			gone = f.Nodes[i].Id
		}
	}
	var out graph.Fragment
	for _, n := range f.Nodes {
		if n.Id != gone {
			out.Nodes = append(out.Nodes, n)
		}
	}
	for _, e := range f.Edges {
		if e.From != gone && e.To != gone {
			out.Edges = append(out.Edges, e)
		}
	}
	return out
}

func snapshotNodes(t rapid.TB, s *Server, when string) int32 {
	return snapshotOf(t, s, when).Counts.Nodes
}

//uc:checks serve.answer-request
func checkFlowAnswerRequest(t rapid.TB, exchange Exchange) {
	s, root, g := served(t, exchange)
	defer removeAll(t, root)
	path := filepath.Join(root, ".unicode", "graph.json")
	if n := snapshotNodes(t, s, "before the snapshot changed"); int(n) != len(exchange.Fragment.Nodes) {
		t.Fatalf("/api/snapshot counts %d nodes, yet the snapshot holds %d", n, len(exchange.Fragment.Nodes))
	}
	half := len(exchange.Requests) / 2
	for _, req := range exchange.Requests[:half] {
		s.Answer(req)
	}
	other := shrunk(exchange.Fragment)
	at := time.Now().Add(time.Hour)
	rewrite(t, root, graphOf(t, other), at)
	fresh, err := Open(root, path, s.builder)
	if err != nil {
		t.Fatalf("Open refused the rewritten snapshot that Encode wrote: %v", err)
	}
	if n := snapshotNodes(t, s, "after the snapshot file changed"); int(n) != len(other.Nodes) {
		t.Fatalf("after the snapshot file was rewritten with %d nodes, /api/snapshot still counts %d", len(other.Nodes), n)
	}
	for _, req := range exchange.Requests[half:] {
		got, want := s.Answer(req), fresh.Answer(req)
		if got.Status != want.Status || !bytes.Equal(got.Body, want.Body) {
			t.Fatalf("after the snapshot file changed, %s got %d\n%s\nyet a server that read only the new snapshot answers %d\n%s",
				req, got.Status, got.Body, want.Status, want.Body)
		}
	}
	writeFile(t, path, []byte("not a snapshot"))
	resp := s.Answer(Request{Path: "/api/snapshot"})
	if p, why := problemOf(resp.Body); resp.Status != http.StatusServiceUnavailable || why != "" || p.Code != "serve.unreadable-snapshot" {
		t.Fatalf("after the snapshot file stopped being a snapshot, /api/snapshot should be refused with 503 serve.unreadable-snapshot, yet it got %d\n%s", resp.Status, resp.Body)
	}
	rewrite(t, root, g, at.Add(time.Hour))
	if n := snapshotNodes(t, s, "after the snapshot was written again"); int(n) != len(exchange.Fragment.Nodes) {
		t.Fatalf("after the snapshot was written again with %d nodes, /api/snapshot counts %d", len(exchange.Fragment.Nodes), n)
	}
	unrecorded(t, s, root, &g, exchange.Requests)
}

// unrecorded writes a review record of the system node into the review
// store of the repository s serves and checks that no answer changes: the
// server answers from the snapshot alone, and a record reaches the viewer
// only when unicode build compiles it into a snapshot.
func unrecorded(t rapid.TB, s *Server, root string, g *graph.Graph, requests []Request) {
	system := ""
	for _, n := range g.Nodes() {
		if n.Form == "system" {
			system = n.Id
		}
	}
	asked := append(slices.Clone(requests), Request{Path: "/api/reviews", Query: map[string][]string{"id": {system}, "pending": {"false"}}})
	before := make([]Response, len(asked))
	for i, req := range asked {
		before[i] = s.Answer(req)
	}
	written := review.Write(root, g, nil, review.Request{Target: review.Target{Node: system}, Reviewer: "reviewer", Decision: "approved"})
	if written.Record == nil {
		t.Fatalf("setup failed: review.Write refused a review of the system node %q: %v", system, written.Diagnostics)
	}
	for i, req := range asked {
		if got := s.Answer(req); got.Status != before[i].Status || !bytes.Equal(got.Body, before[i].Body) {
			t.Fatalf("after a review record was written to the store, with the snapshot unchanged, %s got %d\n%s\nyet before it the answer was %d\n%s",
				req, got.Status, got.Body, before[i].Status, before[i].Body)
		}
	}
}

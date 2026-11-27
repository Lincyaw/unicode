package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
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
	resp := s.Answer(Request{Path: "/api/snapshot"})
	if resp.Status != http.StatusOK {
		t.Fatalf("%s, /api/snapshot got %d\n%s", when, resp.Status, resp.Body)
	}
	var snap Snapshot
	if err := json.Unmarshal(resp.Body, &snap); err != nil {
		t.Fatalf("%s, /api/snapshot does not decode: %v\n%s", when, err, resp.Body)
	}
	return snap.Counts.Nodes
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
	fresh, err := Open(root, path)
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
}

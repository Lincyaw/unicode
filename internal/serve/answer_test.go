package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
	"github.com/lincyaw/unicode/internal/view"
)

// contract is api/openapi.yaml, loaded once, and a router over its operations.
var contract = sync.OnceValues(func() (routers.Router, error) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}
	return legacy.NewRouter(doc)
})

var problemCode = regexp.MustCompile(`^serve\.[a-z]+(-[a-z]+)*$`)

func (r Request) String() string {
	return (&url.URL{Path: r.Path, RawQuery: url.Values(r.Query).Encode()}).String()
}

// problemOf decodes a refusal: exactly the fields code, message and fix,
// each a string that is not empty, the code serve.*; "" when it is one.
func problemOf(body []byte) (Problem, string) {
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return Problem{}, fmt.Sprintf("the body is not a JSON object: %v", err)
	}
	if keys := slices.Sorted(maps.Keys(fields)); !slices.Equal(keys, []string{"code", "fix", "message"}) {
		return Problem{}, fmt.Sprintf("the body has the fields %v, not code, fix and message", keys)
	}
	var p Problem
	if err := json.Unmarshal(body, &p); err != nil || p.Message == "" || p.Fix == "" || !problemCode.MatchString(p.Code) {
		return Problem{}, "its code is not serve.*, or its message or fix is empty"
	}
	return p, ""
}

//uc:checks serve.answers-follow-contract
func checkAnswersFollowContract(t rapid.TB, exchange Exchange) {
	router, err := contract()
	if err != nil {
		t.Fatalf("setup failed: api/openapi.yaml does not load: %v", err)
	}
	s, root, _ := served(t, exchange)
	defer removeAll(t, root)
	for _, req := range exchange.Requests {
		resp := s.Answer(req)
		httpReq := httptest.NewRequest(http.MethodGet, req.String(), nil)
		route, params, err := router.FindRoute(httpReq)
		if err != nil {
			if _, why := problemOf(resp.Body); resp.Status != http.StatusNotFound || why != "" {
				t.Fatalf("%s names no endpoint of api/openapi.yaml, so it should be refused with 404 and a {code, message, fix} body, yet it got %d (%s)\n%s",
					req, resp.Status, why, resp.Body)
			}
			continue
		}
		if route.Operation.Responses.Status(resp.Status) == nil {
			if _, why := problemOf(resp.Body); why != "" || resp.Status < 400 {
				t.Fatalf("%s got the status %d, which api/openapi.yaml does not declare for %s, and its body is not a refusal: %s\n%s",
					req, resp.Status, route.Operation.OperationID, why, resp.Body)
			}
			continue
		}
		err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{Request: httpReq, PathParams: params, Route: route},
			Status:                 resp.Status,
			Header:                 http.Header{"Content-Type": {"application/json"}},
			Body:                   io.NopCloser(bytes.NewReader(resp.Body)),
			Options:                &openapi3filter.Options{IncludeResponseStatus: true},
		})
		if err != nil {
			t.Fatalf("the answer %d to %s does not match its schema in api/openapi.yaml: %v\n%s", resp.Status, req, err, resp.Body)
		}
		if _, why := problemOf(resp.Body); resp.Status >= 400 && why != "" {
			t.Fatalf("%s was refused with %d, but the body is not a serve.* refusal: %s\n%s", req, resp.Status, why, resp.Body)
		}
	}
}

// identityKeys are the fields whose string values, or the strings in whose
// lists, are node identities; details maps identities to counts.
var identityKeys = map[string]bool{"id": true, "from": true, "to": true, "focus": true, "flow": true, "parent": true,
	"select": true, "children": true, "folded": true, "refers": true}

// identitiesIn collects the node identities in a decoded answer; a node's
// or an edge's attrs hold none.
func identitiesIn(v any, key string, into map[string]bool) {
	switch v := v.(type) {
	case string:
		if identityKeys[key] {
			into[v] = true
		}
	case []any:
		for _, x := range v {
			identitiesIn(x, key, into)
		}
	case map[string]any:
		for k, x := range v {
			if k == "attrs" {
				continue
			}
			if counts, ok := x.(map[string]any); ok && k == "details" {
				for id := range counts {
					into[id] = true
				}
			}
			identitiesIn(x, k, into)
		}
	}
}

// listsNodes are the endpoints whose answer always holds at least one node.
var listsNodes = []string{"/api/snapshot", "/api/tree", "/api/entity", "/api/node", "/api/edge", "/api/slice"}

//uc:checks serve.identities-resolve
func checkIdentitiesResolve(t rapid.TB, exchange Exchange) {
	s, root, _ := served(t, exchange)
	defer removeAll(t, root)
	for _, req := range exchange.Requests {
		resp := s.Answer(req)
		if resp.Status != http.StatusOK {
			continue
		}
		var v any
		if err := json.Unmarshal(resp.Body, &v); err != nil {
			t.Fatalf("the answer to %s is not JSON: %v\n%s", req, err, resp.Body)
		}
		ids := map[string]bool{}
		identitiesIn(v, "", ids)
		if len(ids) == 0 && (slices.Contains(listsNodes, req.Path) || strings.HasPrefix(req.Path, "/api/views/")) {
			t.Fatalf("the answer to %s names no node, yet it always holds one\n%s", req, resp.Body)
		}
		for _, id := range slices.Sorted(maps.Keys(ids)) {
			if page := s.Answer(Request{Path: "/api/entity", Query: map[string][]string{"id": {id}}}); page.Status != http.StatusOK {
				t.Fatalf("the answer to %s names the node %q, whose entity page is refused with %d\n%s", req, id, page.Status, page.Body)
			}
		}
	}
}

// sameNode tells where an answer's node differs from the graph's node; "" when nowhere.
func sameNode(got Node, g *graph.Graph, id string) string {
	n := g.Node(id)
	if n == nil {
		return fmt.Sprintf("the answer lists %q, which is not a node of the graph", got.Id)
	}
	want := Node{Id: n.Id, Kind: NodeKind(n.Kind), Form: n.Form, Layer: Layer(n.Layer), Source: Source(n.Source), Origin: position(n.Origin),
		Summary: n.Summary, Salient: n.Salient, Digest: g.Digest(id), Attrs: attrs(n.Attrs)}
	got.Title, got.Status, got.Bindings, got.Step = "", nil, nil, nil
	if got.Id != id || !reflect.DeepEqual(got, want) {
		return fmt.Sprintf("the answer gives the node %q as %+v, yet the graph holds %+v", id, got, want)
	}
	return ""
}

func sameNodes(got []Node, g *graph.Graph, want []graph.Node) string {
	if len(got) != len(want) {
		return fmt.Sprintf("the answer lists %d nodes, the direct result %d", len(got), len(want))
	}
	for i := range got {
		if why := sameNode(got[i], g, want[i].Id); why != "" {
			return fmt.Sprintf("node %d: %s", i+1, why)
		}
	}
	return ""
}

func sameEdges(got []Edge, want []graph.Edge) string {
	if len(got) != len(want) {
		return fmt.Sprintf("the answer lists %d edges, the direct result %d", len(got), len(want))
	}
	for i, e := range want {
		w := Edge{Kind: EdgeKind(e.Kind), From: e.From, To: e.To, Source: Source(e.Source), Origin: position(e.Origin), Attrs: attrs(e.Attrs)}
		g := got[i]
		g.Status = nil
		if !reflect.DeepEqual(g, w) {
			return fmt.Sprintf("edge %d is %+v, yet the direct result has %+v", i+1, got[i], w)
		}
	}
	return ""
}

// refusal checks that a request was refused with the status and code.
func refusal(t rapid.TB, req Request, resp Response, status int, code string) {
	p, why := problemOf(resp.Body)
	if resp.Status != status || why != "" || p.Code != code {
		t.Fatalf("%s should be refused with %d %s, as the direct call refuses it, yet it got %d\n%s", req, status, code, resp.Status, resp.Body)
	}
}

func decode[T any](t rapid.TB, req Request, resp Response) T {
	var v T
	if resp.Status != http.StatusOK {
		t.Fatalf("%s got %d, yet the direct call answers it\n%s", req, resp.Status, resp.Body)
	}
	if err := json.Unmarshal(resp.Body, &v); err != nil {
		t.Fatalf("the answer to %s does not decode as its schema: %v\n%s", req, err, resp.Body)
	}
	return v
}

//uc:checks serve.queries-agree
func checkQueriesAgree(t rapid.TB, exchange Exchange) {
	s, root, g := served(t, exchange)
	defer removeAll(t, root)
	for _, req := range exchange.Requests {
		resp := s.Answer(req)
		q := url.Values(req.Query)
		var why string
		switch req.Path {
		case "/api/node":
			want, err := query.Get(&g, q.Get("id"))
			if errors.As(err, new(query.NotFound)) {
				refusal(t, req, resp, http.StatusNotFound, "serve.not-found")
				continue
			}
			got := decode[NodeView](t, req, resp)
			why = cmpOr(sameNode(got.Node, &g, want.Node.Id), sameEdges(got.Outgoing, want.Outgoing), sameEdges(got.Incoming, want.Incoming))
		case "/api/search":
			var kind *graph.NodeKind
			if k := q.Get("kind"); k != "" {
				if !NodeKind(k).Valid() {
					refusal(t, req, resp, http.StatusBadRequest, "serve.bad-parameter")
					continue
				}
				of := graph.NodeKind(k)
				kind = &of
			}
			want, limit := query.Search(&g, q.Get("text"), kind), 50
			if l := q.Get("limit"); l != "" {
				limit, _ = strconv.Atoi(l)
			}
			why = sameNodes(decode[[]Node](t, req, resp), &g, want[:max(0, min(limit, len(want)))])
		case "/api/slice":
			want, err := query.Slice(&g, q["id"], q["expand"])
			if errors.As(err, new(query.NotFound)) {
				refusal(t, req, resp, http.StatusNotFound, "serve.not-found")
				continue
			}
			why = sameSlice(decode[Slice](t, req, resp), &g, want)
		case "/api/impact":
			want, err := query.Impact(&g, q["id"])
			if errors.As(err, new(query.NotFound)) {
				refusal(t, req, resp, http.StatusNotFound, "serve.not-found")
				continue
			}
			got := decode[[]Affected](t, req, resp)
			if len(got) != len(want) {
				why = fmt.Sprintf("the answer lists %d affected nodes, impact %d", len(got), len(want))
			}
			for i := range min(len(got), len(want)) {
				why = cmpOr(why, sameNode(got[i].Node, &g, want[i].Node.Id), sameEdges(got[i].Path, want[i].Path))
			}
		default:
			why = sameView(t, s, req, resp, &g)
		}
		if why != "" {
			t.Fatalf("the answer to %s differs from calling query or view directly: %s\n%s", req, why, resp.Body)
		}
	}
}

func sameSlice(got Slice, g *graph.Graph, want query.Context) string {
	details := map[string]int32{}
	for id, n := range want.Details {
		details[id] = int32(n)
	}
	why := cmpOr(sameNodes(got.Focus, g, want.Focus), sameNodes(got.Ancestors, g, want.Ancestors), sameNodes(got.Extras, g, want.Extras),
		sameEdges(got.Edges, want.Edges))
	if !maps.Equal(got.Details, details) {
		why = cmpOr(why, fmt.Sprintf("the details are %v, not %v", got.Details, details))
	}
	if len(got.Neighbors) != len(want.Neighbors) {
		return cmpOr(why, fmt.Sprintf("the answer lists %d neighbors, slice %d", len(got.Neighbors), len(want.Neighbors)))
	}
	for i, n := range want.Neighbors {
		if got.Neighbors[i].Focus != n.Focus {
			why = cmpOr(why, fmt.Sprintf("neighbor %d is of %q, not of %q", i+1, got.Neighbors[i].Focus, n.Focus))
		}
		why = cmpOr(why, sameNode(got.Neighbors[i].Node, g, n.Node.Id), sameEdges([]Edge{got.Neighbors[i].Edge}, []graph.Edge{n.Edge}))
	}
	return why
}

// sameView compares a view's answer with view.Select, after the identities
// the request names are found in the graph as the command line finds them.
func sameView(t rapid.TB, s *Server, req Request, resp Response, g *graph.Graph) string {
	q := url.Values(req.Query)
	kind := strings.TrimPrefix(req.Path, "/api/views/")
	if !ViewKind(kind).Valid() {
		refusal(t, req, resp, http.StatusBadRequest, "serve.bad-parameter")
		return ""
	}
	for _, id := range viewIdentities(q.Get("input"), q["expand"]) {
		if g.Node(id) == nil {
			refusal(t, req, resp, http.StatusNotFound, "serve.not-found")
			return ""
		}
	}
	layer, level := view.ClassLayerDesign, view.C4LevelContainer
	if q.Has("layer") {
		layer = view.ClassLayer(q.Get("layer"))
	}
	if q.Has("level") {
		level = view.C4Level(q.Get("level"))
	}
	want, err := view.Select(g, view.ViewKind(kind), q.Get("input"), q["expand"], layer, level)
	if errors.As(err, new(view.NotApplicable)) {
		refusal(t, req, resp, http.StatusBadRequest, "serve.not-applicable")
		return ""
	}
	got := decode[View](t, req, resp)
	nodes := make([]Node, len(got.Nodes))
	for i, n := range got.Nodes {
		nodes[i] = Node{Id: n.Id, Kind: n.Kind, Form: n.Form, Layer: n.Layer, Source: n.Source, Origin: n.Origin, Summary: n.Summary,
			Salient: n.Salient, Digest: n.Digest, Attrs: n.Attrs}
	}
	edges := make([]Edge, len(got.Edges))
	for i, e := range got.Edges {
		edges[i] = Edge{Kind: e.Kind, From: e.From, To: e.To, Source: e.Source, Origin: e.Origin, Attrs: e.Attrs}
	}
	wantNodes := make([]graph.Node, len(want.Nodes))
	for i, n := range want.Nodes {
		wantNodes[i] = n.Node
	}
	wantEdges := make([]graph.Edge, len(want.Edges))
	for i, e := range want.Edges {
		wantEdges[i] = e.Edge
	}
	why := cmpOr(sameNodes(nodes, g, wantNodes), sameEdges(edges, wantEdges))
	if !slices.Equal(got.Folded, append([]string{}, want.Folded...)) || int(got.Hidden) != want.Hidden {
		why = cmpOr(why, fmt.Sprintf("it folds %v and hides %d, select folds %v and hides %d", got.Folded, got.Hidden, want.Folded, want.Hidden))
	}
	return why
}

func cmpOr(whys ...string) string {
	for _, w := range whys {
		if w != "" {
			return w
		}
	}
	return ""
}

//uc:checks serve.answers-are-deterministic
func checkAnswersAreDeterministic(t rapid.TB, exchange Exchange) {
	s, root, _ := served(t, exchange)
	defer removeAll(t, root)
	again, err := Open(root, s.path)
	if err != nil {
		t.Fatalf("Open refused the snapshot it read before: %v", err)
	}
	first := make([]Response, len(exchange.Requests))
	for i, req := range exchange.Requests {
		first[i] = s.Answer(req)
	}
	for i, req := range slices.Backward(exchange.Requests) {
		if got := again.Answer(req); got.Status != first[i].Status || !bytes.Equal(got.Body, first[i].Body) {
			t.Fatalf("%s, answered after other requests, got %d\n%s\nyet before them it got %d\n%s", req, got.Status, got.Body, first[i].Status, first[i].Body)
		}
	}
}

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

// htmlEscape finds in a JSON document an escape of <, > or & (\u003c,
// \u003e, \u0026), which the snapshot's JSON does not write; "" when there is none.
func htmlEscape(body []byte) string {
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' || i+1 >= len(body) {
			continue
		}
		if body[i+1] == 'u' && i+6 <= len(body) {
			if code := strings.ToLower(string(body[i+2 : i+6])); code == "003c" || code == "003e" || code == "0026" {
				return string(body[i : i+6])
			}
		}
		i++
	}
	return ""
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
			t.Fatalf("%s got the status %d, which api/openapi.yaml does not declare for %s\n%s", req, resp.Status, route.Operation.OperationID, resp.Body)
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
		if escape := htmlEscape(resp.Body); escape != "" {
			t.Fatalf("the answer to %s writes %s, an HTML escape that the snapshot's JSON does not write\n%s", req, escape, resp.Body)
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
// or an edge's attrs hold none, and a comparison's items as the base has
// them, and those it removed, are not in the snapshot.
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
		if v["change"] == "removed" {
			return
		}
		for k, x := range v {
			if k == "attrs" || k == "before" {
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
		case "/api/compare":
			if q.Get("base") != "HEAD" {
				refusal(t, req, resp, http.StatusBadRequest, "serve.bad-revision")
				continue
			}
			base := graphOf(t, baseOf(exchange.Fragment))
			why = sameComparison(decode[Comparison](t, req, resp), &base, &g)
		default:
			why = sameView(t, s, req, resp, &g)
		}
		if why != "" {
			t.Fatalf("the answer to %s differs from calling query or view directly: %s\n%s", req, why, resp.Body)
		}
	}
}

// sameComparison tells where a comparison's answer differs from
// graph.Compare of the base and the served graph: each item's change, the
// node as each side has it, and the attribute values; "" when nowhere.
func sameComparison(got Comparison, base, head *graph.Graph) string {
	want := graph.Compare(base, head)
	if len(got.Nodes) != len(want.Nodes) || len(got.Edges) != len(want.Edges) {
		return fmt.Sprintf("the answer lists %d nodes and %d edges, compare %d and %d", len(got.Nodes), len(got.Edges), len(want.Nodes), len(want.Edges))
	}
	for i, w := range want.Nodes {
		n := got.Nodes[i]
		attrs := make([]graph.AttrChange, len(n.Attrs))
		for j, a := range n.Attrs {
			attrs[j] = graph.AttrChange{Key: a.Key, Removed: a.Removed, Added: a.Added}
		}
		why := ""
		switch {
		case n.Id != w.Id || string(n.Change) != string(w.Change) || !reflect.DeepEqual(attrs, w.Attrs):
			why = fmt.Sprintf("node change %d is %s %s %v, compare gives %s %s %v", i+1, n.Change, n.Id, attrs, w.Change, w.Id, w.Attrs)
		case (n.Before == nil) != (w.Before == nil) || (n.After == nil) != (w.After == nil):
			why = fmt.Sprintf("the change of %s gives a side that compare does not, or misses one", w.Id)
		case n.Before != nil:
			why = sameNode(*n.Before, base, w.Id)
		}
		if n.After != nil {
			why = cmpOr(why, sameNode(*n.After, head, w.Id))
		}
		if why != "" {
			return why
		}
	}
	for i, w := range want.Edges {
		e := got.Edges[i]
		if string(e.Kind) != string(w.Kind) || e.From != w.From || e.To != w.To || string(e.Change) != string(w.Change) ||
			(e.Before == nil) != (w.Before == nil) || (e.After == nil) != (w.After == nil) {
			return fmt.Sprintf("edge change %d is %s %s %s %s, compare gives %s %s %s %s", i+1, e.Change, e.Kind, e.From, e.To, w.Change, w.Kind, w.From, w.To)
		}
	}
	return ""
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
	// A layer or a level goes only with the view that reads it, and names one of its values.
	if q.Has("layer") && (kind != "class" || !ClassLayer(q.Get("layer")).Valid()) ||
		q.Has("level") && (kind != "c4" || !C4Level(q.Get("level")).Valid()) {
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
	again, err := Open(root, s.path, s.builder)
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

// tabKinds are the views in the order of their tabs, as the interface writes them.
var tabKinds = func() []ViewKind {
	kinds := make([]ViewKind, len(viewKinds))
	for i, k := range viewKinds {
		kinds[i] = ViewKind(k)
	}
	return kinds
}()

// tabsAnswered are the tabs an answer gives: an entity page's or a view's; none for other answers.
func tabsAnswered(t rapid.TB, req Request, resp Response) ([]ViewTab, bool) {
	switch {
	case resp.Status != http.StatusOK:
		return nil, false
	case req.Path == "/api/entity":
		return decode[Entity](t, req, resp).Tabs, true
	case strings.HasPrefix(req.Path, "/api/views/"):
		return decode[View](t, req, resp).Tabs, true
	}
	return nil, false
}

// viewRequest is the request for the view a tab gives, as the page writes its address.
func viewRequest(r ViewRequest, system string) Request {
	q := map[string][]string{}
	if r.Input.Id != system || !slices.Contains([]ViewKind{ViewKindArchitecture, ViewKindEvents, ViewKindDdd, ViewKindC4}, r.Kind) {
		q["input"] = []string{r.Input.Id}
	}
	if len(r.Expand) > 0 {
		q["expand"] = r.Expand
	}
	if r.Layer != nil {
		q["layer"] = []string{string(*r.Layer)}
	}
	if r.Level != nil {
		q["level"] = []string{string(*r.Level)}
	}
	return Request{Path: "/api/views/" + string(r.Kind), Query: q}
}

//uc:checks serve.tabs-open-views
func checkTabsOpenViews(t rapid.TB, exchange Exchange) {
	s, root, g := served(t, exchange)
	defer removeAll(t, root)
	st := indexed(root, g)
	notations := st.nodes[st.system].Attrs["notations"]
	for _, req := range exchange.Requests {
		tabs, ok := tabsAnswered(t, req, s.Answer(req))
		if !ok {
			continue
		}
		kinds := make([]ViewKind, len(tabs))
		for i, tab := range tabs {
			kinds[i] = tab.Kind
		}
		if !slices.Equal(kinds, tabKinds) {
			t.Fatalf("the answer to %s gives tabs for the views %v, not one for each view in the tabs' order %v", req, kinds, tabKinds)
		}
		for _, tab := range tabs {
			switch {
			case (tab.View == nil) == (tab.Closed == nil):
				t.Fatalf("the answer to %s gives the %s tab %+v, which should have either a view or a reason", req, tab.Kind, tab)
			case tab.View != nil:
				view := viewRequest(*tab.View, st.system)
				if got := s.Answer(view); got.Status != http.StatusOK {
					t.Fatalf("the answer to %s leads the %s tab to %s, which is refused with %d\n%s", req, tab.Kind, view, got.Status, got.Body)
				}
			case *tab.Closed == ClosedTabUndeclaredNotation:
				if (tab.Kind != ViewKindDdd && tab.Kind != ViewKindC4) || slices.Contains(notations, string(tab.Kind)) {
					t.Fatalf("the answer to %s closes the %s tab for an undeclared notation, yet the system file declares %v", req, tab.Kind, notations)
				}
			default:
				for _, n := range g.Nodes() {
					if _, err := view.Select(&g, view.ViewKind(tab.Kind), n.Id, nil, view.ClassLayerDesign, view.C4LevelContainer); err == nil {
						t.Fatalf("the answer to %s closes the %s tab for want of an input, yet the view takes %q", req, tab.Kind, n.Id)
					}
				}
			}
		}
	}
}

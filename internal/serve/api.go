package serve

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
	"github.com/lincyaw/unicode/internal/repo"
	"github.com/lincyaw/unicode/internal/view"
)

// answers implements the operations of api/openapi.yaml over the snapshot
// that Answer puts in each request's context. A refusal is a problem error,
// which writeError writes.
type answers struct{ s *Server }

var _ StrictServerInterface = answers{}

func (answers) GetSnapshot(ctx context.Context, _ GetSnapshotRequestObject) (GetSnapshotResponseObject, error) {
	st := stateOf(ctx)
	statuses := map[string]int32{}
	for _, s := range []graph.Status{graph.StatusVerified, graph.StatusMissing, graph.StatusStub, graph.StatusContradicted} {
		statuses[string(s)] = 0
	}
	for _, s := range st.checked.Statuses {
		statuses[string(s)]++
	}
	return GetSnapshot200JSONResponse{System: st.node(st.system), Head: st.head, Path: st.relative(st.path), BuiltAt: st.builtAt,
		Counts: Counts{Nodes: int32(len(st.nodes)), Edges: int32(len(st.edges)), Diagnostics: int32(len(st.checked.Diagnostics)), Statuses: statuses}}, nil
}

func (answers) GetTree(ctx context.Context, _ GetTreeRequestObject) (GetTreeResponseObject, error) {
	st := stateOf(ctx)
	return GetTree200JSONResponse(st.tree(st.system, map[string]bool{})), nil
}

func (answers) GetView(ctx context.Context, request GetViewRequestObject) (GetViewResponseObject, error) {
	if !request.Kind.Valid() {
		return nil, refuse(http.StatusBadRequest, "serve.bad-parameter", "give one of the view kinds of api/openapi.yaml, such as architecture",
			"there is no view kind %q", request.Kind)
	}
	layer, level := view.ClassLayerDesign, view.C4LevelContainer
	if l := request.Params.Layer; l != nil {
		if err := viewOption("layer", request.Kind, ViewKindClass, *l, l.Valid(), "design or code"); err != nil {
			return nil, err
		}
		layer = view.ClassLayer(*l)
	}
	if l := request.Params.Level; l != nil {
		if err := viewOption("level", request.Kind, ViewKindC4, *l, l.Valid(), "context or container"); err != nil {
			return nil, err
		}
		level = view.C4Level(*l)
	}
	v, err := stateOf(ctx).view(view.ViewKind(request.Kind), value(request.Params.Input), value(request.Params.Expand), layer, level)
	if err != nil {
		return nil, err
	}
	return GetView200JSONResponse(v), nil
}

func (a answers) GetEntity(ctx context.Context, request GetEntityRequestObject) (GetEntityResponseObject, error) {
	st := stateOf(ctx)
	e, err := st.entity(request.Params.Id)
	if err != nil {
		return nil, err
	}
	if base := request.Params.Base; base != nil {
		c, err := a.s.compare(st, *base)
		if err != nil {
			return nil, err
		}
		changes := st.changes(request.Params.Id, c)
		e.Changes = &changes
	}
	return GetEntity200JSONResponse(e), nil
}

func (answers) GetEdge(ctx context.Context, request GetEdgeRequestObject) (GetEdgeResponseObject, error) {
	if !request.Params.Kind.Valid() {
		return nil, refuse(http.StatusBadRequest, "serve.bad-parameter", "give one of the edge kinds of api/openapi.yaml, such as calls",
			"there is no edge kind %q", request.Params.Kind)
	}
	e, err := stateOf(ctx).edgeEntity(graph.EdgeKind(request.Params.Kind), request.Params.From, request.Params.To)
	if err != nil {
		return nil, err
	}
	return GetEdge200JSONResponse(e), nil
}

func (answers) ListNodes(ctx context.Context, request ListNodesRequestObject) (ListNodesResponseObject, error) {
	st := stateOf(ctx)
	nodes := []Node{}
	for _, id := range request.Params.Id {
		if _, ok := st.nodes[id]; ok {
			nodes = append(nodes, st.node(id))
		}
	}
	return ListNodes200JSONResponse(nodes), nil
}

func (answers) GetNode(ctx context.Context, request GetNodeRequestObject) (GetNodeResponseObject, error) {
	st := stateOf(ctx)
	v, err := query.Get(&st.g, request.Params.Id)
	if err != nil {
		return nil, st.queryError(err)
	}
	return GetNode200JSONResponse{Node: st.node(v.Node.Id), Outgoing: st.edgeList(v.Outgoing), Incoming: st.edgeList(v.Incoming)}, nil
}

func (answers) Search(ctx context.Context, request SearchRequestObject) (SearchResponseObject, error) {
	st := stateOf(ctx)
	var kind *graph.NodeKind
	if k := request.Params.Kind; k != nil {
		if !k.Valid() {
			return nil, refuse(http.StatusBadRequest, "serve.bad-parameter", "give one of the node kinds of api/openapi.yaml, such as type",
				"there is no node kind %q", *k)
		}
		of := graph.NodeKind(*k)
		kind = &of
	}
	found := query.Search(&st.g, request.Params.Text, kind)
	limit := 50
	if l := request.Params.Limit; l != nil {
		limit = max(int(*l), 0)
	}
	nodes := []Node{}
	for _, n := range found[:min(limit, len(found))] {
		nodes = append(nodes, st.node(n.Id))
	}
	return Search200JSONResponse(nodes), nil
}

func (answers) GetSlice(ctx context.Context, request GetSliceRequestObject) (GetSliceResponseObject, error) {
	st := stateOf(ctx)
	c, err := query.Slice(&st.g, request.Params.Id, value(request.Params.Expand))
	if err != nil {
		return nil, st.queryError(err)
	}
	s := Slice{Focus: st.queryNodes(c.Focus), Ancestors: st.queryNodes(c.Ancestors), Neighbors: []Neighbor{}, Extras: st.queryNodes(c.Extras),
		Details: map[string]int32{}, Edges: st.edgeList(c.Edges)}
	for _, n := range c.Neighbors {
		s.Neighbors = append(s.Neighbors, Neighbor{Focus: n.Focus, Edge: st.edge(n.Edge), Node: st.node(n.Node.Id)})
	}
	for id, count := range c.Details {
		s.Details[id] = int32(count)
	}
	if withCode := request.Params.WithCode; withCode != nil && *withCode {
		code := map[string]string{}
		for _, list := range [][]graph.Node{c.Focus, c.Ancestors, c.Extras} {
			for _, n := range list {
				st.addDeclaration(code, n.Id)
			}
		}
		for _, n := range c.Neighbors {
			st.addDeclaration(code, n.Node.Id)
		}
		s.Code = &code
	}
	return GetSlice200JSONResponse(s), nil
}

func (st *state) addDeclaration(code map[string]string, id string) {
	if e, ok := st.declaration(id); ok {
		code[id] = e.Text
	}
}

func (answers) GetImpact(ctx context.Context, request GetImpactRequestObject) (GetImpactResponseObject, error) {
	st := stateOf(ctx)
	affected, err := query.Impact(&st.g, request.Params.Id)
	if err != nil {
		return nil, st.queryError(err)
	}
	out := []Affected{}
	for _, a := range affected {
		out = append(out, Affected{Node: st.node(a.Node.Id), Path: st.edgeList(a.Path)})
	}
	return GetImpact200JSONResponse(out), nil
}

func (answers) GetDiagnostics(ctx context.Context, _ GetDiagnosticsRequestObject) (GetDiagnosticsResponseObject, error) {
	st := stateOf(ctx)
	statuses := map[string]Status{}
	for id, s := range st.checked.Statuses {
		statuses[id] = Status(s)
	}
	return GetDiagnostics200JSONResponse{Diagnostics: st.diagnostics(func(string) bool { return true }), Statuses: statuses}, nil
}

func (answers) GetCode(ctx context.Context, request GetCodeRequestObject) (GetCodeResponseObject, error) {
	e, err := stateOf(ctx).code(request.Params.Path, request.Params.From, request.Params.To)
	if err != nil {
		return nil, err
	}
	return GetCode200JSONResponse(e), nil
}

func (a answers) Compare(ctx context.Context, request CompareRequestObject) (CompareResponseObject, error) {
	st := stateOf(ctx)
	c, err := a.s.compare(st, request.Params.Base)
	if err != nil {
		return nil, err
	}
	return Compare200JSONResponse(st.comparison(c)), nil
}

func (answers) ListRevisions(ctx context.Context, request ListRevisionsRequestObject) (ListRevisionsResponseObject, error) {
	limit := 20
	if l := request.Params.Limit; l != nil {
		limit = int(*l)
	}
	out := []Revision{}
	// A repository without git or without a commit has no revisions to compare with.
	rs, _ := repo.Log(stateOf(ctx).root, "HEAD", limit)
	for _, r := range rs {
		out = append(out, revision(r))
	}
	return ListRevisions200JSONResponse(out), nil
}

// queryError refuses a query given an identity that is not a node with
// serve.not-found; any other refusal is the server's failure.
func (st *state) queryError(err error) error {
	var nf query.NotFound
	if errors.As(err, &nf) {
		return st.unknown(nf.Id)
	}
	return err
}

func (st *state) queryNodes(nodes []graph.Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, st.node(n.Id))
	}
	return out
}

// view is the data of a view: view.Select's input, ancestors, nodes with
// their cards and edges with their counts and labels, each node written as
// the interface writes it, with the diagnostics of what it draws.
func (st *state) view(kind view.ViewKind, input string, expand []string, layer view.ClassLayer, level view.C4Level) (View, error) {
	for _, id := range viewIdentities(input, expand) {
		if _, ok := st.nodes[id]; !ok {
			return View{}, st.unknown(id)
		}
	}
	v, err := view.Select(&st.g, kind, input, expand, layer, level)
	var na view.NotApplicable
	switch {
	case errors.As(err, &na):
		return View{}, refuse(http.StatusBadRequest, "serve.not-applicable", "open the view from a node it takes, as an entity's views list them", "%v", err)
	case err != nil:
		return View{}, err
	}
	var ancestors []string
	for _, a := range v.Ancestors {
		ancestors = append(ancestors, a.Id)
	}
	out := View{Kind: ViewKind(kind), Input: st.node(v.Input.Id), Ancestors: st.nodeList(ancestors), Expand: append([]string{}, v.Expand...),
		Nodes: []ViewNode{}, Edges: []ViewEdge{}, Folded: append([]string{}, v.Folded...), Hidden: int32(v.Hidden)}
	if v.Layer != nil {
		l := ClassLayer(*v.Layer)
		out.Layer = &l
	}
	if v.Level != nil {
		l := C4Level(*v.Level)
		out.Level = &l
	}
	drawn := map[string]bool{}
	for _, n := range v.Nodes {
		out.Nodes = append(out.Nodes, st.viewNode(n, drawn))
	}
	for _, e := range v.Edges {
		drawn[edgeID(e.Kind, e.From, e.To)] = true
		ed := st.edge(e.Edge)
		out.Edges = append(out.Edges, ViewEdge{Kind: ed.Kind, From: ed.From, To: ed.To, Source: ed.Source, Origin: ed.Origin, Attrs: ed.Attrs, Status: ed.Status,
			Count: optional(int32(e.Count))})
		if len(e.Labels) > 0 {
			labels := make([]EdgeLabel, 0, len(e.Labels))
			for _, l := range e.Labels {
				labels = append(labels, EdgeLabel{Op: l.Op, Text: l.Text})
			}
			out.Edges[len(out.Edges)-1].Labels = &labels
		}
	}
	out.Diagnostics = st.diagnostics(func(subject string) bool { return drawn[subject] })
	out.Tabs = st.tabs(v.Input.Id)
	return out, nil
}

// viewNode writes a node of a view with its card, a code type or function
// with the file it is declared in, and records it and the nodes of its
// contents as drawn.
func (st *state) viewNode(n view.ViewNode, drawn map[string]bool) ViewNode {
	drawn[n.Id] = true
	d := st.node(n.Id)
	out := ViewNode{Id: d.Id, Kind: d.Kind, Form: d.Form, Layer: d.Layer, Source: d.Source, Origin: d.Origin, Summary: d.Summary,
		Salient: d.Salient, Digest: d.Digest, Attrs: d.Attrs, Title: d.Title, Status: d.Status, Refers: d.Refers, Bindings: d.Bindings, Step: d.Step,
		Details: optional(int32(n.Details)), Evidence: st.evidenceOf(n.Id)}
	if st.is(n.Id, graph.LayerCode, graph.NodeKindType, graph.NodeKindOp) {
		out.File = optional(st.codeFile(n.Id))
	}
	if len(n.Members) > 0 {
		members := make([]Member, 0, len(n.Members))
		for _, m := range n.Members {
			members = append(members, member(m))
		}
		out.Members = &members
	}
	if len(n.Contents) > 0 {
		contents := make([]ViewNode, 0, len(n.Contents))
		for _, c := range n.Contents {
			contents = append(contents, st.viewNode(c, drawn))
		}
		out.Contents = &contents
	}
	return out
}

// member writes a member of a type card, check's comparison of a field or
// an enum value with the code.
func member(m check.Member) Member {
	out := Member{Kind: MemberKind(m.Kind), Name: m.Name, Type: optional(m.Type), CodeType: optional(m.CodeType),
		InDesign: m.InDesign, InCode: m.InCode, Refers: append([]string{}, m.Refers...)}
	if m.Status != nil {
		s := Status(*m.Status)
		out.Status = &s
	}
	return out
}

// optional is a pointer to v, or nil when v is its type's zero value.
func optional[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

// viewOption refuses a view's layer or level, as unicode view refuses
// --layer and --level: given to another view than the one that reads it, or
// naming none of its values.
func viewOption[T ~string](name string, kind, reads ViewKind, value T, valid bool, values string) error {
	switch {
	case kind != reads:
		return refuse(http.StatusBadRequest, "serve.bad-parameter", "leave "+name+" out", "%s picks the %s of the %s view, not of the %s view", name, name, reads, kind)
	case !valid:
		return refuse(http.StatusBadRequest, "serve.bad-parameter", "give "+values, "%q is not a %s", value, name)
	}
	return nil
}

// viewIdentities are the node identities a view request names: its input,
// when given, and each expanded node or each end of an expanded reaches edge.
func viewIdentities(input string, expand []string) []string {
	var ids []string
	if input != "" {
		ids = append(ids, input)
	}
	for _, x := range expand {
		if rest, ok := strings.CutPrefix(x, "reaches "); ok {
			ids = append(ids, strings.Fields(rest)...)
		} else {
			ids = append(ids, x)
		}
	}
	return ids
}

// tree is the design tree under id: the system's children by group (core
// and support containers, external systems, persons, utility containers),
// each by name; a container's in the order they are declared. Steps and
// cases are left out.
func (st *state) tree(id string, seen map[string]bool) TreeNode {
	seen[id] = true
	out := TreeNode{Node: st.node(id), Children: []TreeNode{}}
	for _, c := range st.treeChildren(id) {
		if !seen[c] {
			out.Children = append(out.Children, st.tree(c, seen))
		}
	}
	return out
}

// treeChildren are a node's children in the design tree, in the tree's order.
func (st *state) treeChildren(id string) []string {
	kids := slices.DeleteFunc(st.children(id), func(c string) bool { return !st.inTree(c) })
	if id == st.system {
		slices.SortStableFunc(kids, func(a, b string) int {
			if d := st.group(a) - st.group(b); d != 0 {
				return d
			}
			return strings.Compare(st.title(a), st.title(b))
		})
	}
	return kids
}

// group orders the system's children in the design tree.
func (st *state) group(id string) int {
	n := st.nodes[id]
	switch {
	case n.Kind == graph.NodeKindContainer && st.attr(id, "focus") == "core":
		return 0
	case n.Kind == graph.NodeKindContainer && st.attr(id, "focus") == "support":
		return 1
	case n.Kind == graph.NodeKindActor && n.Form == "external":
		return 2
	case n.Kind == graph.NodeKindActor:
		return 3
	case n.Kind == graph.NodeKindContainer && st.attr(id, "focus") == "utility":
		return 4
	}
	return 5
}

func value[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

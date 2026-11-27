package serve

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/view"
)

// state is one snapshot as the server read it: the graph with its edges
// indexed by end, the result of checking it, and the working tree it was
// read from. It does not change once loaded.
type state struct {
	root    string
	path    string
	builtAt time.Time
	g       graph.Graph
	nodes   map[string]graph.Node
	out, in map[string][]graph.Edge
	edges   map[string]graph.Edge
	system  string
	files   map[string]string
	checked check.Checked
	head    WorkingTree
	// applies memoizes whether a view applies to an input, by "kind input".
	applies sync.Map
}

// load reads the snapshot file path of the repository at root, decodes it
// and checks it as unicode check does.
func load(root, path string, builtAt time.Time) (*state, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Unreadable{Path: path, Cause: err}
	}
	g, err := graph.Decode(data)
	if err != nil {
		return nil, Unreadable{Path: path, Cause: err}
	}
	st := &state{root: root, path: path, builtAt: builtAt.UTC(), g: g, nodes: map[string]graph.Node{},
		out: map[string][]graph.Edge{}, in: map[string][]graph.Edge{}, edges: map[string]graph.Edge{}}
	for _, n := range g.Nodes() {
		st.nodes[n.Id] = n
		if st.system == "" && n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindContainer && n.Form == "system" {
			st.system = n.Id
		}
	}
	if st.system == "" {
		return nil, Unreadable{Path: path, Cause: errors.New("the snapshot has no system container, so unicode build did not write it")}
	}
	for _, e := range g.Edges() {
		st.out[e.From] = append(st.out[e.From], e)
		st.in[e.To] = append(st.in[e.To], e)
		st.edges[edgeID(e.Kind, e.From, e.To)] = e
	}
	st.files = files(st.nodes)
	st.checked = check.Conform(&g)
	st.head = readHead(root)
	return st, nil
}

func edgeID(kind graph.EdgeKind, from, to string) string { return string(kind) + " " + from + " " + to }

// targets returns the ends of the edges of the kinds that leave id.
func (st *state) targets(id string, kinds ...graph.EdgeKind) []string {
	var ids []string
	for _, e := range st.out[id] {
		if slices.Contains(kinds, e.Kind) {
			ids = append(ids, e.To)
		}
	}
	return ids
}

// sources returns the starts of the edges of the kinds that reach id.
func (st *state) sources(id string, kinds ...graph.EdgeKind) []string {
	var ids []string
	for _, e := range st.in[id] {
		if slices.Contains(kinds, e.Kind) {
			ids = append(ids, e.From)
		}
	}
	return ids
}

// outgoing and incoming return the edges of the kinds that leave or reach id.
func (st *state) outgoing(id string, kinds ...graph.EdgeKind) []graph.Edge {
	return slices.DeleteFunc(slices.Clone(st.out[id]), func(e graph.Edge) bool { return !slices.Contains(kinds, e.Kind) })
}

func (st *state) incoming(id string, kinds ...graph.EdgeKind) []graph.Edge {
	return slices.DeleteFunc(slices.Clone(st.in[id]), func(e graph.Edge) bool { return !slices.Contains(kinds, e.Kind) })
}

// parent returns the node that contains id, the first by identity, or "".
func (st *state) parent(id string) string {
	if p := st.sources(id, graph.EdgeKindContains); len(p) > 0 {
		return p[0]
	}
	return ""
}

// ancestors returns the nodes that contain id along contains, outermost first.
func (st *state) ancestors(id string) []string {
	var chain []string
	seen := map[string]bool{id: true}
	for p := st.parent(id); p != "" && !seen[p]; p = st.parent(p) {
		seen[p] = true
		chain = append(chain, p)
	}
	slices.Reverse(chain)
	return chain
}

// children returns the nodes id contains, of the kinds when given, in the
// order they are written: by origin, then by identity.
func (st *state) children(id string, kinds ...graph.NodeKind) []string {
	var ids []string
	for _, c := range st.targets(id, graph.EdgeKindContains) {
		if len(kinds) == 0 || slices.Contains(kinds, st.nodes[c].Kind) {
			ids = append(ids, c)
		}
	}
	st.byOrigin(ids)
	return ids
}

// byOrigin sorts identities by where their nodes are written, then by identity.
func (st *state) byOrigin(ids []string) {
	slices.SortFunc(ids, func(a, b string) int {
		pa, pb := st.nodes[a].Origin, st.nodes[b].Origin
		return cmp.Or(cmp.Compare(pa.Path, pb.Path), cmp.Compare(pa.Line, pb.Line), cmp.Compare(pa.Column, pb.Column), cmp.Compare(a, b))
	})
}

// byIndex sorts step identities by their index, then by identity.
func (st *state) byIndex(ids []string) {
	slices.SortFunc(ids, func(a, b string) int { return cmp.Or(cmp.Compare(st.index(a), st.index(b)), cmp.Compare(a, b)) })
}

func (st *state) index(step string) int {
	if i, err := strconv.Atoi(st.attr(step, "index")); err == nil {
		return i
	}
	return 0
}

func (st *state) attr(id, key string) string {
	if v := st.nodes[id].Attrs[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (st *state) is(id string, layer graph.Layer, kinds ...graph.NodeKind) bool {
	n, ok := st.nodes[id]
	return ok && n.Layer == layer && slices.Contains(kinds, n.Kind)
}

// node writes the graph's node id as the interface gives it: with its
// digest, title and status; a code element with its bindings, a step with
// where it stands in its flow. The nodes these name are brief.
func (st *state) node(id string) Node {
	out := st.brief(id)
	if st.nodes[id].Layer == graph.LayerCode {
		bindings := []Neighbor{}
		for _, e := range st.outgoing(id, graph.EdgeKindRealizes, graph.EdgeKindChecks, graph.EdgeKindGenerates) {
			bindings = append(bindings, Neighbor{Focus: id, Edge: st.edge(e), Node: st.brief(e.To)})
		}
		out.Bindings = &bindings
	}
	if st.nodes[id].Kind == graph.NodeKindStep {
		out.Step = st.stepInfo(id)
	}
	return out
}

// brief writes a node with its digest, title and status only.
func (st *state) brief(id string) Node {
	n := st.nodes[id]
	return Node{Id: n.Id, Kind: NodeKind(n.Kind), Form: n.Form, Layer: Layer(n.Layer), Source: Source(n.Source),
		Origin: position(n.Origin), Summary: n.Summary, Salient: n.Salient, Digest: st.g.Digest(id),
		Attrs: attrs(n.Attrs), Title: st.title(id), Status: st.status(id)}
}

func (st *state) nodeList(ids []string) []Node {
	nodes := make([]Node, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, st.node(id))
	}
	return nodes
}

func (st *state) edge(e graph.Edge) Edge {
	return Edge{Kind: EdgeKind(e.Kind), From: e.From, To: e.To, Source: Source(e.Source), Origin: position(e.Origin),
		Attrs: attrs(e.Attrs), Status: st.status(edgeID(e.Kind, e.From, e.To))}
}

func (st *state) edgeList(edges []graph.Edge) []Edge {
	out := make([]Edge, 0, len(edges))
	for _, e := range edges {
		out = append(out, st.edge(e))
	}
	return out
}

// neighbors writes each edge with the node at its other end from focus.
func (st *state) neighbors(focus string, edges []graph.Edge) []Neighbor {
	out := make([]Neighbor, 0, len(edges))
	for _, e := range edges {
		other := e.To
		if other == focus {
			other = e.From
		}
		out = append(out, Neighbor{Focus: focus, Edge: st.edge(e), Node: st.node(other)})
	}
	return out
}

func (st *state) status(id string) *Status {
	s, ok := st.checked.Statuses[id]
	if !ok {
		return nil
	}
	status := Status(s)
	return &status
}

// title is the name the page shows for a node: a design node's last segment,
// a step's index written #n; a code element's name in its language, which is
// its identity after its package's; a file's path.
func (st *state) title(id string) string {
	n := st.nodes[id]
	switch {
	case n.Kind == graph.NodeKindStep:
		if i := strings.LastIndex(id, "#"); i >= 0 {
			return id[i:]
		}
	case n.Layer == graph.LayerDesign:
		return id[strings.LastIndex(id, ".")+1:]
	case n.Layer == graph.LayerCode && n.Kind == graph.NodeKindContainer:
		if p := st.attr(id, "path"); p != "" {
			return p
		}
	case n.Layer == graph.LayerCode:
		for _, a := range slices.Backward(st.ancestors(id)) {
			if st.nodes[a].Form == "package" && strings.HasPrefix(id, a+".") {
				return strings.TrimPrefix(id, a+".")
			}
		}
	}
	return id
}

// stepInfo is where a step stands in its flow.
func (st *state) stepInfo(id string) *StepInfo {
	info := &StepInfo{Children: st.subSteps(id)}
	for _, a := range slices.Backward(st.ancestors(id)) {
		if st.nodes[a].Kind == graph.NodeKindFlow {
			info.Flow = a
			break
		}
	}
	if p := st.parent(id); st.nodes[p].Kind == graph.NodeKindStep {
		info.Parent = &p
	}
	if s := st.targets(id, graph.EdgeKindFrom); len(s) > 0 {
		sender := st.brief(s[0])
		info.Sender = &sender
	}
	if t := st.targets(id, graph.EdgeKindCalls, graph.EdgeKindEmits, graph.EdgeKindHandles); len(t) > 0 {
		target := st.brief(t[0])
		info.Target = &target
		if owner := st.parent(t[0]); owner != "" && slices.Contains(st.targets(id, graph.EdgeKindCalls), t[0]) {
			receiver := st.brief(owner)
			info.Receiver = &receiver
		}
	}
	return info
}

// subSteps are the direct sub-steps of a step, by index.
func (st *state) subSteps(id string) []string {
	ids := []string{}
	for _, c := range st.targets(id, graph.EdgeKindContains) {
		if st.nodes[c].Kind == graph.NodeKindStep {
			ids = append(ids, c)
		}
	}
	st.byIndex(ids)
	return ids
}

// diagnostics are the checked diagnostics whose subject the keep function accepts.
func (st *state) diagnostics(keep func(subject string) bool) []Diagnostic {
	out := []Diagnostic{}
	for _, d := range st.checked.Diagnostics {
		if keep(d.Subject) {
			out = append(out, diagnostic(d))
		}
	}
	return out
}

func diagnostic(d diag.Diagnostic) Diagnostic {
	return Diagnostic{Code: d.Code, Subject: d.Subject, Path: d.Position.Path, Line: int32(d.Position.Line),
		Column: int32(d.Position.Column), Message: d.Message, Fix: d.Fix}
}

// viewApplies tells whether the view of the kind applies to the input, as
// view.Select decides; the answer is kept, as the snapshot does not change.
func (st *state) viewApplies(kind view.ViewKind, input string) bool {
	key := string(kind) + " " + input
	if known, ok := st.applies.Load(key); ok {
		return known.(bool)
	}
	_, err := view.Select(&st.g, kind, input, nil)
	st.applies.Store(key, err == nil)
	return err == nil
}

// relative writes path relative to the repository root when it is under it.
func (st *state) relative(path string) string {
	if rel, err := filepath.Rel(st.root, path); err == nil && filepath.IsLocal(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func position(p diag.Position) Position {
	return Position{Path: p.Path, Line: int32(p.Line), Column: int32(p.Column)}
}

// attrs writes attrs as the interface requires them: a map, and lists, never null.
func attrs(a map[string][]string) map[string][]string {
	out := make(map[string][]string, len(a))
	for k, v := range a {
		out[k] = append([]string{}, v...)
	}
	return out
}

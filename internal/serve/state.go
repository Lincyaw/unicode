package serve

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/repo"
	"github.com/lincyaw/unicode/internal/review"
	"github.com/lincyaw/unicode/internal/view"
)

// state is one snapshot as the server read it: the graph with its edges
// indexed by end, the result of checking it, the review records it
// carries, and the working tree it was read from. It does not change once
// loaded.
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
	// codeFiles maps a code file's path to the file's node.
	codeFiles         map[string]string
	checked           check.Checked
	head              WorkingTree
	records           []review.Record
	reviewDiagnostics []diag.Diagnostic
	// effects are the effects of the operations (docs/model.md §9 D8), and
	// meanDurations the mean durations of the design operations that the
	// current evidence records calls of, in nanoseconds.
	effects       map[string][]graph.Effect
	meanDurations map[string]int
	// prepare makes, once for the served snapshot, what every page reads:
	// inputs, the inputs each view kind takes, by identity (view.Inputs);
	// evaluated, the review evaluation without a base; typeMembers, the
	// members of every design type compared with the code (check.Members).
	inputs      map[view.ViewKind][]string
	evaluated   evaluated
	typeMembers map[string][]check.Member
	// comparisons memoizes the comparison with each base commit, and
	// baseReviews the review evaluation against each base commit.
	comparisons sync.Map
	baseReviews sync.Map
}

// load reads the snapshot file path of the repository at root, decodes it
// and builds the state that every request reads: it checks the graph as
// unicode check does, reads back the review records the snapshot carries,
// and prepares what every page reads. The snapshot is what unicode build
// compiled, evidence and reviews included; the run and review stores are
// not read.
func load(root, path string, builtAt time.Time) (*state, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Unreadable{Path: path, Cause: err}
	}
	g, err := graph.Decode(data)
	if err != nil {
		return nil, Unreadable{Path: path, Cause: err}
	}
	st := indexed(root, g)
	if st.system == "" {
		return nil, Unreadable{Path: path, Cause: errors.New("the snapshot has no system container, so unicode build did not write it")}
	}
	st.path, st.builtAt = path, builtAt.UTC()
	if st.head, err = readHead(root, filepath.Dir(path)); err != nil {
		return nil, Unreadable{Path: path, Cause: err}
	}
	st.checked = check.Conform(&st.g)
	recorded := review.Recorded(&st.g)
	st.records, st.reviewDiagnostics = recorded.Records, recorded.Diagnostics
	st.checked.Diagnostics = append(st.checked.Diagnostics, recorded.Diagnostics...)
	slices.SortFunc(st.checked.Diagnostics, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Subject, b.Subject))
	})
	st.prepare()
	return st, nil
}

// indexed indexes the graph of the repository at root, unchecked: the
// served snapshot before its check, or the graph of a base revision.
func indexed(root string, g graph.Graph) *state {
	st := &state{root: root, g: g, nodes: map[string]graph.Node{}, out: map[string][]graph.Edge{}, in: map[string][]graph.Edge{},
		edges: map[string]graph.Edge{}, checked: check.Checked{Statuses: map[string]graph.Status{}}}
	for _, n := range g.Nodes() {
		st.nodes[n.Id] = n
		if st.system == "" && n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindContainer && n.Form == "system" {
			st.system = n.Id
		}
	}
	for _, e := range g.Edges() {
		st.out[e.From] = append(st.out[e.From], e)
		st.in[e.To] = append(st.in[e.To], e)
		st.edges[edgeID(e.Kind, e.From, e.To)] = e
	}
	st.files = files(st.nodes)
	st.codeFiles = map[string]string{}
	for _, n := range st.nodes {
		if n.Layer == graph.LayerCode && n.Kind == graph.NodeKindContainer && n.Form == "file" {
			st.codeFiles[n.Origin.Path] = n.Id
		}
	}
	st.effects = g.Effects()
	st.meanDurations = evidence.MeanDurations(&g)
	return st
}

// readHead reads the working tree the snapshot was built from: from the
// record build wrote beside it in dir, the commit with what git tells of it
// and whether the working tree had changes; without a record, from git now,
// empty without git or a commit. A record that is not {commit, dirty} is an error.
func readHead(root, dir string) (WorkingTree, error) {
	record, err := repo.ReadRecord(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		w, err := repo.ReadWorkingTree(root, dir)
		if err != nil {
			return WorkingTree{}, nil
		}
		return workingTree(w), nil
	case err != nil:
		return WorkingTree{}, err
	}
	head := WorkingTree{Dirty: record.Dirty}
	if record.Commit == nil {
		return head, nil
	}
	head.Commit = *record.Commit
	if r, err := repo.ReadRevision(root, *record.Commit); err == nil && r.Commit == *record.Commit {
		head = workingTree(r.At(record.Dirty))
	}
	return head, nil
}

func workingTree(w repo.WorkingTree) WorkingTree {
	return WorkingTree{Commit: w.Commit, Short: w.Short, Branch: optional(w.Branch), Subject: w.Subject, Time: w.Time, Dirty: w.Dirty}
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
		for _, e := range st.outgoing(id, graph.EdgeKindRealizes, graph.EdgeKindChecks, graph.EdgeKindGenerates, graph.EdgeKindObserves, graph.EdgeKindSubstitutes) {
			bindings = append(bindings, Neighbor{Focus: id, Edge: st.edge(e), Node: st.brief(e.To)})
		}
		out.Bindings = &bindings
	}
	if st.nodes[id].Kind == graph.NodeKindStep {
		out.Step = st.stepInfo(id)
	}
	return out
}

// brief writes a node with its digest, title, status and the nodes it
// refers to only; an operation also with its effects and mean duration.
func (st *state) brief(id string) Node {
	n := st.nodes[id]
	return Node{Id: n.Id, Kind: NodeKind(n.Kind), Form: n.Form, Layer: Layer(n.Layer), Source: Source(n.Source),
		Origin: position(n.Origin), Summary: n.Summary, Salient: n.Salient, Digest: st.g.Digest(id),
		Attrs: attrs(n.Attrs), Title: st.title(id), Status: st.status(id), Refers: st.refers(id), Operation: st.operation(id)}
}

// operation writes what an operation node carries: its effects, unless it
// is a design operation that nothing realizes, and a design operation's mean
// duration, when the current evidence records calls of it. Other nodes have
// none.
func (st *state) operation(id string) *OperationInfo {
	if st.nodes[id].Kind != graph.NodeKindOp {
		return nil
	}
	out := &OperationInfo{}
	if kinds, ok := st.effects[id]; ok {
		effects := make([]Effect, len(kinds))
		for i, k := range kinds {
			effects[i] = Effect(k)
		}
		out.Effects = &effects
	}
	if d, ok := st.meanDurations[id]; ok {
		mean := int64(d)
		out.MeanDuration = &mean
	}
	return out
}

// refers are the nodes the refers edges of id point at, by identity, each once.
func (st *state) refers(id string) []string {
	ids := append([]string{}, st.targets(id, graph.EdgeKindRefers)...)
	slices.Sort(ids)
	return slices.Compact(ids)
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
// view.Select decides.
func (st *state) viewApplies(kind view.ViewKind, input string) bool {
	_, takes := slices.BinarySearch(st.inputsOf(kind), input)
	return takes
}

// inputsOf are the inputs the view of the kind takes, by identity
// (view.Inputs), as prepare made them.
func (st *state) inputsOf(kind view.ViewKind) []string { return st.inputs[kind] }

// prepare makes what every page of the served snapshot reads, so that no
// request pays for it: the inputs of each view kind, the review evaluation
// without a base, and the members of every design type.
func (st *state) prepare() {
	st.inputs = map[view.ViewKind][]string{}
	for _, kind := range viewKinds {
		st.inputs[kind] = view.Inputs(&st.g, kind)
	}
	st.evaluated = evaluated{Evaluation: review.Evaluate(&st.g, nil, st.records), byTarget: map[review.Target]review.Item{}}
	for _, item := range st.evaluated.Items {
		st.evaluated.byTarget[item.Target] = item
	}
	var types []string
	for _, n := range st.nodes {
		if n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindType {
			types = append(types, n.Id)
		}
	}
	st.typeMembers = check.Members(&st.g, types)
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

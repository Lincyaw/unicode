package check

import (
	"cmp"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// Checked is the result of checking a graph: the diagnostics, and the status
// of every declared item and every depends, keyed by a node's identity or an
// edge's `kind from to`.
//
//uc:realizes check.Checked
type Checked struct {
	Diagnostics []diag.Diagnostic
	Statuses    map[string]graph.Status
}

// Conform checks, on a derived graph, the constraints that need the code
// (docs/design.md §5.3): docs/model.md §10 rules 12, 14 to 20 and 22, and
// that each generated symbol has exactly one generates binding (rule 24). It
// gives every declared item its status (§6): missing where the code lacks
// it, stub where its function is a skeleton, contradicted where the code
// disagrees with it, and verified otherwise. Rules 21 and 28 wait for gen.
//
//uc:realizes check.conform
func Conform(g *graph.Graph) Checked {
	c := newChecker(g)
	c.ownership()
	c.dependencies()
	c.bindings()
	c.realizations()
	c.relations()
	c.predicates()
	c.generators()
	c.steps()
	slices.SortFunc(c.diags, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line),
			cmp.Compare(a.Position.Column, b.Position.Column), cmp.Compare(a.Code, b.Code),
			cmp.Compare(a.Subject, b.Subject), cmp.Compare(a.Message, b.Message))
	})
	return Checked{Diagnostics: c.diags, Statuses: c.statuses}
}

// checker indexes a graph's nodes and edges for the constraints, and collects
// their diagnostics and statuses.
type checker struct {
	nodes    []graph.Node
	byID     map[string]graph.Node
	edges    []graph.Edge
	out, in  map[graph.EdgeKind]map[string][]graph.Edge
	parent   map[string]string
	children map[string][]string
	reach    map[string]map[string]bool
	diags    []diag.Diagnostic
	statuses map[string]graph.Status
}

func newChecker(g *graph.Graph) *checker {
	c := &checker{nodes: g.Nodes(), edges: g.Edges(), byID: map[string]graph.Node{},
		out: map[graph.EdgeKind]map[string][]graph.Edge{}, in: map[graph.EdgeKind]map[string][]graph.Edge{},
		parent: map[string]string{}, children: map[string][]string{}, reach: map[string]map[string]bool{},
		statuses: map[string]graph.Status{}}
	for _, n := range c.nodes {
		c.byID[n.Id] = n
		if n.Source == graph.SourceDeclared {
			c.statuses[n.Id] = graph.StatusVerified
		}
	}
	for _, e := range c.edges {
		if c.out[e.Kind] == nil {
			c.out[e.Kind], c.in[e.Kind] = map[string][]graph.Edge{}, map[string][]graph.Edge{}
		}
		c.out[e.Kind][e.From] = append(c.out[e.Kind][e.From], e)
		c.in[e.Kind][e.To] = append(c.in[e.Kind][e.To], e)
		if e.Kind == graph.EdgeKindContains {
			c.parent[e.To] = e.From
			c.children[e.From] = append(c.children[e.From], e.To)
		}
		if e.Source == graph.SourceDeclared || e.Kind == graph.EdgeKindDepends {
			c.statuses[edgeID(e)] = graph.StatusVerified
		}
	}
	return c
}

func edgeID(e graph.Edge) string { return string(e.Kind) + " " + e.From + " " + e.To }

// severity orders the statuses: an item keeps the worst status any rule gives it.
var severity = map[graph.Status]int{graph.StatusVerified: 0, graph.StatusStub: 1, graph.StatusMissing: 2, graph.StatusContradicted: 3}

// mark gives a declared item a status, unless it has a worse one already.
func (c *checker) mark(id string, status graph.Status) {
	if current, ok := c.statuses[id]; ok && severity[status] > severity[current] {
		c.statuses[id] = status
	}
}

// fail reports a diagnostic about subject and gives subject the status.
func (c *checker) fail(code, subject string, status graph.Status, at diag.Position, message, fix string) {
	c.diags = append(c.diags, diag.Diagnostic{Code: code, Subject: subject, Position: at, Message: message, Fix: fix})
	c.mark(subject, status)
}

func (c *checker) is(id string, layer graph.Layer, kind graph.NodeKind, forms ...string) bool {
	n, ok := c.byID[id]
	return ok && n.Layer == layer && n.Kind == kind && (len(forms) == 0 || slices.Contains(forms, n.Form))
}

func (c *checker) has(kind graph.EdgeKind, from, to string) bool {
	return slices.ContainsFunc(c.out[kind][from], func(e graph.Edge) bool { return e.To == to })
}

func (c *checker) attr(id, key string) string {
	if v := c.byID[id].Attrs[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// ancestors lists the containment ancestors of id, nearest first, stopping
// at the first repeat of a broken graph's cycle.
func (c *checker) ancestors(id string) []string {
	var out []string
	seen := map[string]bool{id: true}
	for p, ok := c.parent[id]; ok && !seen[p]; p, ok = c.parent[p] {
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func (c *checker) isFunction(id string) bool { return c.is(id, graph.LayerCode, graph.NodeKindOp) }

// isMethod tells whether id is a code function that a code type holds: a
// method, or an abstract method of an interface.
func (c *checker) isMethod(id string) bool {
	return c.isFunction(id) && c.is(c.parent[id], graph.LayerCode, graph.NodeKindType)
}

func (c *checker) isFile(id string) bool {
	return c.is(id, graph.LayerCode, graph.NodeKindContainer, "file")
}

func (c *checker) isDesignContainer(id string) bool {
	return c.is(id, graph.LayerDesign, graph.NodeKindContainer)
}

// fileOf is the code file that holds id, or "".
func (c *checker) fileOf(id string) string {
	for _, a := range append([]string{id}, c.ancestors(id)...) {
		if c.isFile(a) {
			return a
		}
	}
	return ""
}

// owners are the design containers that own the file of id.
func (c *checker) owners(id string) []string {
	var out []string
	for _, e := range c.in[graph.EdgeKindOwns][c.fileOf(id)] {
		out = append(out, e.From)
	}
	return out
}

// home is the design container or actor that a design node belongs to.
func (c *checker) home(id string) string {
	for _, a := range c.ancestors(id) {
		if c.isDesignContainer(a) || c.is(a, graph.LayerDesign, graph.NodeKindActor) {
			return a
		}
	}
	return ""
}

// usesFrom tells whether a or one of its ancestors uses b.
func (c *checker) usesFrom(a, b string) bool {
	for _, x := range append([]string{a}, c.ancestors(a)...) {
		if c.has(graph.EdgeKindUses, x, b) {
			return true
		}
	}
	return false
}

// name is a design node's own name: the last segment of its identity.
func name(id string) string { return id[strings.LastIndex(id, ".")+1:] }

// describe names a node for a message: its kind and identity.
func (c *checker) describe(id string) string {
	n, ok := c.byID[id]
	switch {
	case !ok:
		return id
	case n.Layer == graph.LayerCode && n.Kind == graph.NodeKindOp && n.Form == "func":
		return "the function " + id
	case n.Layer == graph.LayerCode && n.Form == "abstract":
		return "the abstract method " + id
	case n.Layer == graph.LayerCode:
		return "the " + n.Form + " " + id
	case n.Kind == graph.NodeKindContainer:
		return "the " + n.Form + " " + id
	case n.Kind == graph.NodeKindOp:
		return "the operation " + id
	case n.Kind == graph.NodeKindActor:
		return "the " + n.Form + " actor " + id
	}
	return "the " + string(n.Kind) + " " + id
}

// functionsOf lists the code functions whose file a design container owns.
func (c *checker) functionsOf(container string) []string {
	var out []string
	for _, n := range c.nodes {
		if c.isFunction(n.Id) && slices.Contains(c.owners(n.Id), container) {
			out = append(out, n.Id)
		}
	}
	return out
}

// reaches tells whether the code function from reaches the code function to
// along one or more calls.
func (c *checker) reaches(from, to string) bool {
	if seen, ok := c.reach[from]; ok {
		return seen[to]
	}
	seen := map[string]bool{}
	queue := c.callees(from)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if !seen[id] {
			seen[id] = true
			queue = append(queue, c.callees(id)...)
		}
	}
	c.reach[from] = seen
	return seen[to]
}

func (c *checker) callees(id string) []string {
	var out []string
	for _, e := range c.out[graph.EdgeKindCalls][id] {
		if c.isFunction(e.From) && c.isFunction(e.To) {
			out = append(out, e.To)
		}
	}
	return out
}

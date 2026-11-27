package query

import (
	"fmt"
	"reflect"
	"slices"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// Question is a graph and a question about it: the focus identities, the
// identities to expand and a text to search for.
//
//uc:realizes query.Question
type Question struct {
	Fragment graph.Fragment
	Ids      []string
	Expand   []string
	Text     string
}

var nodeKinds = []graph.NodeKind{graph.NodeKindContainer, graph.NodeKindType, graph.NodeKindOp, graph.NodeKindActor,
	graph.NodeKindFlow, graph.NodeKindStep, graph.NodeKindProperty, graph.NodeKindObservation, graph.NodeKindCase, graph.NodeKindRun,
	graph.NodeKindResult, graph.NodeKindReview}

// children are the kinds of node that a node of each kind contains (docs/model.md §4).
var children = map[graph.NodeKind][]graph.NodeKind{
	graph.NodeKindContainer: {graph.NodeKindContainer, graph.NodeKindType, graph.NodeKindOp, graph.NodeKindFlow, graph.NodeKindProperty},
	graph.NodeKindActor:     {graph.NodeKindType, graph.NodeKindOp},
	graph.NodeKindType:      {graph.NodeKindOp},
	graph.NodeKindFlow:      {graph.NodeKindStep, graph.NodeKindCase},
	graph.NodeKindStep:      {graph.NodeKindStep},
	graph.NodeKindProperty:  {graph.NodeKindCase, graph.NodeKindObservation},
}

// edgeKinds lists every edge kind.
var edgeKinds = []graph.EdgeKind{graph.EdgeKindContains, graph.EdgeKindUses, graph.EdgeKindExtends,
	graph.EdgeKindImplements, graph.EdgeKindRefers, graph.EdgeKindSubscribes, graph.EdgeKindImports, graph.EdgeKindCalls,
	graph.EdgeKindFrom, graph.EdgeKindEmits, graph.EdgeKindHandles, graph.EdgeKindAbout, graph.EdgeKindRealizes,
	graph.EdgeKindChecks, graph.EdgeKindGenerates, graph.EdgeKindObserves, graph.EdgeKindOwns, graph.EdgeKindDepends, graph.EdgeKindTraces,
	graph.EdgeKindExercises, graph.EdgeKindReaches, graph.EdgeKindCovers}

// shapes are the edge kinds that docs/model.md §4 draws from a node of one
// kind to a node of another.
var shapes = map[[2]graph.NodeKind][]graph.EdgeKind{
	{graph.NodeKindContainer, graph.NodeKindContainer}:  {graph.EdgeKindContains, graph.EdgeKindUses, graph.EdgeKindDepends},
	{graph.NodeKindContainer, graph.NodeKindType}:       {graph.EdgeKindContains},
	{graph.NodeKindContainer, graph.NodeKindOp}:         {graph.EdgeKindContains},
	{graph.NodeKindContainer, graph.NodeKindFlow}:       {graph.EdgeKindContains},
	{graph.NodeKindContainer, graph.NodeKindProperty}:   {graph.EdgeKindContains},
	{graph.NodeKindType, graph.NodeKindOp}:              {graph.EdgeKindContains},
	{graph.NodeKindType, graph.NodeKindType}:            {graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements},
	{graph.NodeKindOp, graph.NodeKindType}:              {graph.EdgeKindRefers, graph.EdgeKindSubscribes},
	{graph.NodeKindOp, graph.NodeKindOp}:                {graph.EdgeKindCalls, graph.EdgeKindReaches, graph.EdgeKindRealizes},
	{graph.NodeKindOp, graph.NodeKindProperty}:          {graph.EdgeKindChecks, graph.EdgeKindGenerates},
	{graph.NodeKindOp, graph.NodeKindObservation}:       {graph.EdgeKindObserves},
	{graph.NodeKindProperty, graph.NodeKindType}:        {graph.EdgeKindRefers, graph.EdgeKindAbout},
	{graph.NodeKindProperty, graph.NodeKindOp}:          {graph.EdgeKindAbout},
	{graph.NodeKindProperty, graph.NodeKindCase}:        {graph.EdgeKindContains},
	{graph.NodeKindProperty, graph.NodeKindObservation}: {graph.EdgeKindContains},
	{graph.NodeKindObservation, graph.NodeKindType}:     {graph.EdgeKindRefers},
	{graph.NodeKindFlow, graph.NodeKindStep}:            {graph.EdgeKindContains},
	{graph.NodeKindFlow, graph.NodeKindType}:            {graph.EdgeKindRefers},
	{graph.NodeKindStep, graph.NodeKindStep}:            {graph.EdgeKindContains},
	{graph.NodeKindStep, graph.NodeKindOp}:              {graph.EdgeKindCalls, graph.EdgeKindTraces},
	{graph.NodeKindStep, graph.NodeKindType}:            {graph.EdgeKindEmits, graph.EdgeKindHandles, graph.EdgeKindFrom},
}

// genText draws short strings over a tiny alphabet, so that identities,
// summaries and searched texts often overlap; a wide character is among them.
func genText(min int) *rapid.Generator[string] {
	return rapid.StringOfN(rapid.SampledFrom([]rune("ab.中")), min, 4, -1)
}

// genFragment draws one to twelve nodes of any kind, salient or not, many of
// them contained in an earlier node that can contain their kind; refers
// edges from properties and types to types, as symbols and fields have;
// then edges between any nodes, mostly of a kind the model draws between
// their kinds, otherwise of any kind, loops and cycles included.
func genFragment(t *rapid.T) graph.Fragment {
	var f graph.Fragment
	seen := map[string]bool{}
	type key struct {
		kind     graph.EdgeKind
		from, to string
	}
	edges := map[key]bool{}
	for range rapid.IntRange(1, 12).Draw(t, "nodes") {
		id := genText(1).Draw(t, "id")
		if seen[id] {
			continue
		}
		seen[id] = true
		kind := rapid.SampledFrom(nodeKinds).Draw(t, "kind")
		var parents []graph.Node
		for _, n := range f.Nodes {
			if children[n.Kind] != nil {
				parents = append(parents, n)
			}
		}
		if len(parents) > 0 && rapid.IntRange(0, 3).Draw(t, "contained") > 0 {
			parent := rapid.SampledFrom(parents).Draw(t, "parent")
			kind = rapid.SampledFrom(children[parent.Kind]).Draw(t, "childKind")
			edges[key{graph.EdgeKindContains, parent.Id, id}] = true
			f.Edges = append(f.Edges, graph.Edge{Kind: graph.EdgeKindContains, From: parent.Id, To: id, Source: graph.SourceDeclared})
		}
		f.Nodes = append(f.Nodes, graph.Node{
			Id:      id,
			Kind:    kind,
			Form:    rapid.SampledFrom([]string{"component", "object", "func"}).Draw(t, "form"),
			Layer:   rapid.SampledFrom([]graph.Layer{graph.LayerDesign, graph.LayerCode}).Draw(t, "layer"),
			Source:  graph.SourceDeclared,
			Summary: genText(0).Draw(t, "summary"),
			Salient: rapid.Bool().Draw(t, "salient"),
			Attrs:   rapid.MapOfN(genText(1), rapid.SliceOfN(genText(0), 0, 2), 0, 2).Draw(t, "attrs"),
		})
	}
	var types []string
	for _, n := range f.Nodes {
		if n.Kind == graph.NodeKindType {
			types = append(types, n.Id)
		}
	}
	for _, n := range f.Nodes {
		if (n.Kind == graph.NodeKindProperty || n.Kind == graph.NodeKindObservation || n.Kind == graph.NodeKindType) && len(types) > 0 {
			for range rapid.IntRange(0, 2).Draw(t, "refers") {
				if k := (key{graph.EdgeKindRefers, n.Id, rapid.SampledFrom(types).Draw(t, "referred")}); !edges[k] {
					edges[k] = true
					f.Edges = append(f.Edges, graph.Edge{Kind: k.kind, From: k.from, To: k.to, Source: graph.SourceDeclared})
				}
			}
		}
	}
	for range rapid.IntRange(0, 24).Draw(t, "edges") {
		from, to := rapid.SampledFrom(f.Nodes).Draw(t, "from"), rapid.SampledFrom(f.Nodes).Draw(t, "to")
		kinds := edgeKinds
		if shaped := shapes[[2]graph.NodeKind{from.Kind, to.Kind}]; shaped != nil && rapid.IntRange(0, 3).Draw(t, "shaped") > 0 {
			kinds = shaped
		}
		e := graph.Edge{Kind: rapid.SampledFrom(kinds).Draw(t, "edgeKind"), From: from.Id, To: to.Id, Source: graph.SourceDerived}
		if k := (key{e.Kind, e.From, e.To}); !edges[k] {
			edges[k] = true
			f.Edges = append(f.Edges, e)
		}
	}
	for i := range len(f.Nodes) {
		property := &f.Nodes[i]
		if property.Kind != graph.NodeKindProperty || len(types) == 0 || !rapid.Bool().Draw(t, "namedObservation") {
			continue
		}
		id := fmt.Sprintf("%s.snapshot%d", property.Id, len(f.Nodes))
		adapter := id + ".adapter"
		sourceType := rapid.SampledFrom(types).Draw(t, "observedInputType")
		snapshotType := rapid.SampledFrom(types).Draw(t, "snapshotType")
		if property.Attrs == nil {
			property.Attrs = map[string][]string{}
		}
		property.Attrs["symbols"] = append(property.Attrs["symbols"], "observedInput: list<"+sourceType+">")
		f.Nodes = append(f.Nodes,
			graph.Node{Id: id, Kind: graph.NodeKindObservation, Layer: graph.LayerDesign, Salient: rapid.Bool().Draw(t, "observationSalience"), Attrs: map[string][]string{"inputs": {"observedInput"}, "type": {snapshotType}}},
			graph.Node{Id: adapter, Kind: graph.NodeKindOp, Layer: graph.LayerCode, Salient: rapid.Bool().Draw(t, "adapterSalience")})
		f.Edges = append(f.Edges,
			graph.Edge{Kind: graph.EdgeKindContains, From: property.Id, To: id},
			graph.Edge{Kind: graph.EdgeKindRefers, From: id, To: snapshotType},
			graph.Edge{Kind: graph.EdgeKindObserves, From: adapter, To: id})
	}
	return f
}

func genIds(t *rapid.T, f graph.Fragment, label string, min, max int) []string {
	var ids []string
	for range rapid.IntRange(min, max).Draw(t, label) {
		ids = append(ids, rapid.SampledFrom(f.Nodes).Draw(t, label+"Id").Id)
	}
	return ids
}

// genSliceQuestion draws a graph, one to three focus identities and up to
// three identities to expand, all taken from the graph's nodes; a focus is
// often an operation, a property or a flow, the kinds that add extras.
//
//uc:generates query.slice-is-minimal-context.question
func genSliceQuestion() *rapid.Generator[Question] {
	return rapid.Custom(func(t *rapid.T) Question {
		f := genFragment(t)
		var adding []string
		for _, n := range f.Nodes {
			if n.Kind == graph.NodeKindOp || n.Kind == graph.NodeKindProperty || n.Kind == graph.NodeKindObservation || n.Kind == graph.NodeKindFlow {
				adding = append(adding, n.Id)
			}
		}
		ids := genIds(t, f, "ids", 1, 3)
		if len(adding) > 0 && rapid.Bool().Draw(t, "adding") {
			ids[0] = rapid.SampledFrom(adding).Draw(t, "addingId")
		}
		return Question{Fragment: f, Ids: ids, Expand: genIds(t, f, "expand", 0, 3)}
	})
}

// genImpactQuestion draws a graph and one to three changed identities taken from its nodes.
//
//uc:generates query.impact-is-reverse-reachability.question
func genImpactQuestion() *rapid.Generator[Question] {
	return rapid.Custom(func(t *rapid.T) Question {
		f := genFragment(t)
		return Question{Fragment: f, Ids: genIds(t, f, "ids", 1, 3)}
	})
}

// genSearchQuestion draws a graph and a text: part of one node's identity or
// summary, or a random text that may match nothing.
//
//uc:generates query.search-matches-text.question
func genSearchQuestion() *rapid.Generator[Question] {
	return rapid.Custom(func(t *rapid.T) Question {
		f := genFragment(t)
		text := genText(0).Draw(t, "text")
		if rapid.Bool().Draw(t, "fromNode") {
			n := rapid.SampledFrom(f.Nodes).Draw(t, "node")
			source := []rune(rapid.SampledFrom([]string{n.Id, n.Summary}).Draw(t, "source"))
			start := rapid.IntRange(0, len(source)).Draw(t, "start")
			text = string(source[start:rapid.IntRange(start, len(source)).Draw(t, "end")])
		}
		return Question{Fragment: f, Text: text}
	})
}

// graphOf adds the nodes, then the edges, of a fragment whose identities are distinct.
func graphOf(t rapid.TB, f graph.Fragment) graph.Graph {
	g := graph.NewGraph()
	for _, n := range f.Nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatalf("AddNode refused the node %q, whose identity is new: %v", n.Id, err)
		}
	}
	for _, e := range f.Edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("AddEdge refused the new edge %s %q %q, whose ends are nodes of the graph: %v", e.Kind, e.From, e.To, err)
		}
	}
	return g
}

func nodesById(f graph.Fragment) map[string]graph.Node {
	nodes := map[string]graph.Node{}
	for _, n := range f.Nodes {
		nodes[n.Id] = n
	}
	return nodes
}

// sameNodes reports the first place where got differs from the nodes with
// the identities want, in that order, as the graph holds them; "" when none.
func sameNodes(got []graph.Node, want []string, nodes map[string]graph.Node) string {
	ids := make([]string, len(got))
	for i, n := range got {
		ids[i] = n.Id
	}
	if !slices.Equal(ids, want) {
		return fmt.Sprintf("the nodes are %q, not %q", ids, want)
	}
	for _, n := range got {
		if !reflect.DeepEqual(n, nodes[n.Id]) {
			return fmt.Sprintf("the node %q is %+v, yet the graph holds %+v", n.Id, n, nodes[n.Id])
		}
	}
	return ""
}

func set(ids []string) map[string]bool {
	s := map[string]bool{}
	for _, id := range ids {
		s[id] = true
	}
	return s
}

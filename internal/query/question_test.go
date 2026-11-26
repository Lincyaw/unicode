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
	graph.NodeKindFlow, graph.NodeKindStep, graph.NodeKindProperty, graph.NodeKindCase, graph.NodeKindRun,
	graph.NodeKindResult, graph.NodeKindReview}

// edgeKinds lists every edge kind; contains comes three times, so that
// chains of ancestors are common.
var edgeKinds = []graph.EdgeKind{graph.EdgeKindContains, graph.EdgeKindContains, graph.EdgeKindContains,
	graph.EdgeKindUses, graph.EdgeKindExtends, graph.EdgeKindImplements, graph.EdgeKindRefers,
	graph.EdgeKindSubscribes, graph.EdgeKindImports, graph.EdgeKindCalls, graph.EdgeKindFrom, graph.EdgeKindEmits,
	graph.EdgeKindHandles, graph.EdgeKindAbout, graph.EdgeKindRealizes, graph.EdgeKindChecks, graph.EdgeKindGenerates,
	graph.EdgeKindOwns, graph.EdgeKindDepends, graph.EdgeKindTraces, graph.EdgeKindExercises, graph.EdgeKindReaches,
	graph.EdgeKindCovers}

// genText draws short strings over a tiny alphabet, so that identities,
// summaries and searched texts often overlap; a wide character is among them.
func genText(min int) *rapid.Generator[string] {
	return rapid.StringOfN(rapid.SampledFrom([]rune("ab.中")), min, 4, -1)
}

// genFragment draws one to ten nodes of any kind, salient or not, and edges
// of any kind between them, loops and cycles included.
func genFragment(t *rapid.T) graph.Fragment {
	var f graph.Fragment
	seen := map[string]bool{}
	for range rapid.IntRange(1, 10).Draw(t, "nodes") {
		id := genText(1).Draw(t, "id")
		if seen[id] {
			continue
		}
		seen[id] = true
		f.Nodes = append(f.Nodes, graph.Node{
			Id:      id,
			Kind:    rapid.SampledFrom(nodeKinds).Draw(t, "kind"),
			Form:    rapid.SampledFrom([]string{"component", "object", "func"}).Draw(t, "form"),
			Layer:   rapid.SampledFrom([]graph.Layer{graph.LayerDesign, graph.LayerCode}).Draw(t, "layer"),
			Source:  graph.SourceDeclared,
			Summary: genText(0).Draw(t, "summary"),
			Salient: rapid.Bool().Draw(t, "salient"),
			Attrs:   rapid.MapOfN(genText(1), rapid.SliceOfN(genText(0), 0, 2), 0, 2).Draw(t, "attrs"),
		})
	}
	type key struct {
		kind     graph.EdgeKind
		from, to string
	}
	edges := map[key]bool{}
	for range rapid.IntRange(0, 20).Draw(t, "edges") {
		e := graph.Edge{
			Kind:   rapid.SampledFrom(edgeKinds).Draw(t, "edgeKind"),
			From:   rapid.SampledFrom(f.Nodes).Draw(t, "from").Id,
			To:     rapid.SampledFrom(f.Nodes).Draw(t, "to").Id,
			Source: graph.SourceDerived,
		}
		if k := (key{e.Kind, e.From, e.To}); !edges[k] {
			edges[k] = true
			f.Edges = append(f.Edges, e)
		}
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
// three identities to expand, all taken from the graph's nodes.
//
//uc:generates query.slice-is-minimal-context.question
func genSliceQuestion() *rapid.Generator[Question] {
	return rapid.Custom(func(t *rapid.T) Question {
		f := genFragment(t)
		return Question{Fragment: f, Ids: genIds(t, f, "ids", 1, 3), Expand: genIds(t, f, "expand", 0, 3)}
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

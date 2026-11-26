package query

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// genGetQuestion draws a graph and one to four identities, each either a
// node of the graph or an identity no node has.
//
//uc:generates query.get-shows-incident-edges.question
func genGetQuestion() *rapid.Generator[Question] {
	return rapid.Custom(func(t *rapid.T) Question {
		f := genFragment(t)
		var ids []string
		for range rapid.IntRange(1, 4).Draw(t, "ids") {
			id := rapid.SampledFrom(f.Nodes).Draw(t, "node").Id
			if rapid.IntRange(0, 2).Draw(t, "absent") == 0 {
				id = "?" + genText(0).Draw(t, "absentId")
			}
			ids = append(ids, id)
		}
		return Question{Fragment: f, Ids: ids}
	})
}

// sameEdges reports the first edge that got and want, a list of the graph's
// edges, do not share, or one that got holds twice; "" when none.
func sameEdges(got, want []graph.Edge) string {
	wanted := map[edgeKey]graph.Edge{}
	for _, e := range want {
		wanted[edgeKey{e.Kind, e.From, e.To}] = e
	}
	seen := map[edgeKey]bool{}
	for _, e := range got {
		k := edgeKey{e.Kind, e.From, e.To}
		switch w, ok := wanted[k]; {
		case seen[k]:
			return fmt.Sprintf("it lists %s %q %q twice", e.Kind, e.From, e.To)
		case !ok:
			return fmt.Sprintf("it lists %s %q %q, which is not one of them", e.Kind, e.From, e.To)
		case !reflect.DeepEqual(e, w):
			return fmt.Sprintf("it gives %s %q %q as %+v, yet the graph holds %+v", e.Kind, e.From, e.To, e, w)
		}
		seen[k] = true
	}
	for k := range wanted {
		if !seen[k] {
			return fmt.Sprintf("it leaves out %s %q %q", k.kind, k.from, k.to)
		}
	}
	return ""
}

//uc:checks query.get-shows-incident-edges
func checkGetShowsIncidentEdges(t rapid.TB, question Question) {
	g := graphOf(t, question.Fragment)
	nodes := nodesById(question.Fragment)
	for _, id := range question.Ids {
		view, err := Get(&g, id)
		refs, refsErr := Refs(&g, id)
		node, present := nodes[id]
		if !present {
			var missing NotFound
			if !errors.As(err, &missing) || missing.Id != id {
				t.Fatalf("Get of %q, which no node has, gave %v rather than NotFound for it", id, err)
			}
			if !errors.As(refsErr, &missing) || missing.Id != id {
				t.Fatalf("Refs of %q, which no node has, gave %v rather than NotFound for it", id, refsErr)
			}
			continue
		}
		if err != nil || refsErr != nil {
			t.Fatalf("Get or Refs refused %q, a node of the graph: %v, %v", id, err, refsErr)
		}
		if !reflect.DeepEqual(view.Node, node) {
			t.Fatalf("Get of %q gives the node %+v, yet the graph holds %+v", id, view.Node, node)
		}
		var outgoing, incoming []graph.Edge
		for _, e := range question.Fragment.Edges {
			if e.From == id {
				outgoing = append(outgoing, e)
			}
			if e.To == id {
				incoming = append(incoming, e)
			}
		}
		if problem := sameEdges(view.Outgoing, outgoing); problem != "" {
			t.Fatalf("Get of %q does not give exactly the edges that start at it as outgoing: %s", id, problem)
		}
		if problem := sameEdges(view.Incoming, incoming); problem != "" {
			t.Fatalf("Get of %q does not give exactly the edges that end at it as incoming: %s", id, problem)
		}
		if problem := sameEdges(refs, incoming); problem != "" {
			t.Fatalf("Refs of %q does not give exactly the edges that end at it: %s", id, problem)
		}
	}
}

func TestGetShowsIncidentEdges(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkGetShowsIncidentEdges(t, genGetQuestion().Draw(t, "question"))
	})
}

func FuzzGetShowsIncidentEdges(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkGetShowsIncidentEdges(t, genGetQuestion().Draw(t, "question"))
	}))
}

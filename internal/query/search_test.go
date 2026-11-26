package query

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

//uc:checks query.search-matches-text
func checkSearchMatchesText(t rapid.TB, question Question, kind *graph.NodeKind) {
	g := graphOf(t, question.Fragment)
	got := Search(&g, question.Text, kind)
	var want []string
	for _, n := range g.Nodes() {
		if (kind == nil || n.Kind == *kind) && (strings.Contains(n.Id, question.Text) || strings.Contains(n.Summary, question.Text)) {
			want = append(want, n.Id)
		}
	}
	if problem := sameNodes(got, want, nodesById(question.Fragment)); problem != "" {
		t.Fatalf("searching %q among the nodes of kind %v gives not exactly the nodes whose identity or summary contains it, sorted by identity: %s", question.Text, kindName(kind), problem)
	}
}

func kindName(kind *graph.NodeKind) string {
	if kind == nil {
		return "any"
	}
	return string(*kind)
}

func TestSearchMatchesText(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkSearchMatchesText(t, genSearchQuestion().Draw(t, "question"), rapid.Ptr(rapid.SampledFrom(nodeKinds), true).Draw(t, "kind"))
	})
}

func FuzzSearchMatchesText(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkSearchMatchesText(t, genSearchQuestion().Draw(t, "question"), rapid.Ptr(rapid.SampledFrom(nodeKinds), true).Draw(t, "kind"))
	}))
}

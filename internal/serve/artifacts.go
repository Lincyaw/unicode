package serve

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/review"
)

// inputGraph keeps source inputs and cases while removing persisted projections.
func inputGraph(source *graph.Graph) (graph.Graph, error) {
	g := graph.NewGraph()
	for _, node := range source.Nodes() {
		if node.Layer == graph.LayerEvidence || node.Layer == graph.LayerReview {
			continue
		}
		if err := g.AddNode(node); err != nil {
			return g, err
		}
	}
	for _, edge := range source.Edges() {
		if edge.Kind == graph.EdgeKindCovers || g.Node(edge.From) == nil || g.Node(edge.To) == nil {
			continue
		}
		if err := g.AddEdge(edge); err != nil {
			return g, err
		}
	}
	return g, nil
}

// liveArtifacts freezes the stores once per request, even when graph.json is unchanged.
func liveArtifacts(source *state) (*state, error) {
	g, err := inputGraph(&source.g)
	if err != nil {
		return nil, err
	}
	runs, reviews := evidence.Read(source.root), review.Read(source.root)
	measured, reviewed := evidence.Attach(&g, runs.Reports), review.Attach(&g, reviews.Records)
	for _, fragment := range []graph.Fragment{measured.Fragment, reviewed.Fragment} {
		if err := g.AddFragment(fragment); err != nil {
			return nil, err
		}
	}
	st := indexed(source.root, g)
	st.path, st.builtAt, st.head, st.reviews = source.path, source.builtAt, source.head, reviews.Records
	st.reviewDiagnostics = reviews.Diagnostics
	st.checked = check.Conform(&st.g)
	st.checked.Diagnostics = append(st.checked.Diagnostics, runs.Diagnostics...)
	st.checked.Diagnostics = append(st.checked.Diagnostics, reviews.Diagnostics...)
	st.checked.Diagnostics = append(st.checked.Diagnostics, measured.Diagnostics...)
	st.checked.Diagnostics = append(st.checked.Diagnostics, reviewed.Diagnostics...)
	slices.SortFunc(st.checked.Diagnostics, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Subject, b.Subject))
	})
	return st, nil
}

func reviewTarget(target review.Target) ReviewTarget {
	return ReviewTarget{Node: optional(target.Node), EdgeKind: optional(EdgeKind(target.EdgeKind)), From: optional(target.From), To: optional(target.To), Removed: target.Removed}
}

func reviewRecord(record review.Record) ReviewRecord {
	at, _ := time.Parse(time.RFC3339Nano, record.At)
	return ReviewRecord{Version: int32(record.Version), Id: record.Id, Reviewer: record.Reviewer, At: at, Snapshot: record.Snapshot, Commit: record.Commit, Dirty: record.Dirty, Target: reviewTarget(record.Target), Digest: record.Digest, Decision: ReviewRecordDecision(record.Decision), Comment: record.Comment}
}

func reviewedItem(item review.Item) *Review {
	target := reviewTarget(item.Target)
	out := &Review{State: ReviewState(item.State), Target: &target, Digest: &item.Digest}
	if record := item.Record; record != nil {
		at, _ := time.Parse(time.RFC3339Nano, record.At)
		out.RecordId, out.Snapshot, out.Reviewer, out.Comment, out.At = &record.Id, &record.Snapshot, &record.Reviewer, optional(record.Comment), &at
		decision := ReviewDecision(record.Decision)
		out.Decision = &decision
	}
	return out
}

func (st *state) reviewOf(target review.Target) *Review {
	if len(st.reviewDiagnostics) > 0 {
		return nil
	}
	for _, item := range review.Evaluate(&st.g, nil, st.reviews).Items {
		if item.Target == target {
			return reviewedItem(item)
		}
	}
	return nil
}

func (a answers) ListReviews(ctx context.Context, request ListReviewsRequestObject) (ListReviewsResponseObject, error) {
	st := stateOf(ctx)
	if len(st.reviewDiagnostics) > 0 {
		return nil, fmt.Errorf("cannot read review records: %v", st.reviewDiagnostics)
	}
	var baseline *graph.Graph
	if request.Params.Base != nil {
		if *request.Params.Base == "" {
			return nil, refuse(http.StatusBadRequest, "serve.bad-parameter", "give a base commit or omit base", "base is empty")
		}
		comparison, err := a.s.compare(st, *request.Params.Base)
		if err != nil {
			return nil, err
		}
		baseline = &comparison.graph.g
	}
	pending := request.Params.Pending == nil || *request.Params.Pending
	ids := value(request.Params.Id)
	result := review.Evaluate(&st.g, baseline, st.reviews)
	if len(result.Diagnostics) > 0 {
		return nil, fmt.Errorf("cannot evaluate reviews: %v", result.Diagnostics)
	}
	out := ReviewCollection{Items: []ReviewItem{}}
	for _, item := range result.Items {
		if pending && item.State != "pending" || len(ids) > 0 && !slices.Contains(ids, item.Target.Node) && !slices.Contains(ids, item.Target.From) && !slices.Contains(ids, item.Target.To) {
			continue
		}
		converted := ReviewItem{Target: reviewTarget(item.Target), Digest: item.Digest, State: ReviewState(item.State)}
		if item.Record != nil {
			record := reviewRecord(*item.Record)
			converted.Record = &record
		}
		out.Items = append(out.Items, converted)
	}
	return ListReviews200JSONResponse(out), nil
}

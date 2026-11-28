package serve

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/lincyaw/unicode/internal/review"
)

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
	if item.Record != nil {
		record := *item.Record
		at, _ := time.Parse(time.RFC3339Nano, record.At)
		out.RecordId, out.Snapshot, out.Reviewer, out.Comment, out.At = &record.Id, &record.Snapshot, &record.Reviewer, optional(record.Comment), &at
		decision := ReviewDecision(record.Decision)
		out.Decision = &decision
	}
	return out
}

// evaluated is the review evaluation of a state without a base: its items
// in order, and each item by its target.
type evaluated struct {
	review.Evaluation
	byTarget map[review.Target]review.Item
}

func (st *state) reviewOf(target review.Target) *Review {
	if len(st.reviewDiagnostics) > 0 {
		return nil
	}
	if item, ok := st.evaluated.byTarget[target]; ok {
		return reviewedItem(item)
	}
	return nil
}

// reviewsAgainst is the review evaluation of the snapshot against the base
// commit of c; it is made once per base commit and kept with the snapshot.
func (st *state) reviewsAgainst(c *compared) review.Evaluation {
	if known, ok := st.baseReviews.Load(c.base.Commit); ok {
		return known.(review.Evaluation)
	}
	evaluation, _ := st.baseReviews.LoadOrStore(c.base.Commit, review.Evaluate(&st.g, &c.graph.g, st.records))
	return evaluation.(review.Evaluation)
}

func (a answers) ListReviews(ctx context.Context, request ListReviewsRequestObject) (ListReviewsResponseObject, error) {
	st := stateOf(ctx)
	if len(st.reviewDiagnostics) > 0 {
		return nil, fmt.Errorf("cannot read review records: %v", st.reviewDiagnostics)
	}
	result := st.evaluated.Evaluation
	if request.Params.Base != nil {
		if *request.Params.Base == "" {
			return nil, refuse(http.StatusBadRequest, "serve.bad-parameter", "give a base commit or omit base", "base is empty")
		}
		comparison, err := a.s.compare(st, *request.Params.Base)
		if err != nil {
			return nil, err
		}
		result = st.reviewsAgainst(comparison)
	}
	pending := request.Params.Pending == nil || *request.Params.Pending
	ids := value(request.Params.Id)
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

import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/** Exactly one node or one edge; removed identifies a deletion relative to base. */
model ReviewTarget {
  node?: string;
  edgeKind?: EdgeKind;
  from?: string;
  to?: string;
  removed: boolean;
}

/** One immutable review record from .unicode/reviews. */
model ReviewRecord {
  version: int32;
  id: string;
  reviewer: string;
  at: utcDateTime;
  snapshot: string;
  commit: string;
  dirty: boolean;
  target: ReviewTarget;
  digest: string;
  decision: "approved" | "changes-requested";
  comment: string;
}

/** The current fingerprint and latest matching record, independently of approval. */
model ReviewItem {
  target: ReviewTarget;
  digest: string;
  state: ReviewState;
  record?: ReviewRecord;
}

model ReviewCollection {
  items: ReviewItem[];
}

/** Pending items by default; a base includes additions, modifications and deletions. */
@route("/reviews")
@get
op listReviews(
  @query pending?: boolean,
  @query base?: string,
  @query(#{ explode: true }) id?: string[],
): ReviewCollection | BadRequest | Refusals;

import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/**
 * The slice of a set of nodes, the least context that answers a question about them (docs/design.md §7.3,
 * query.Context), with the source of its code elements when asked for.
 */
model Slice {
  focus: Node[];
  ancestors: Node[];
  neighbors: Neighbor[];
  extras: Node[];

  /** For each focus node, how many detail nodes the slice counts but does not list */
  details: Record<int32>;

  /** The contains edges between its nodes, from which the ancestor chains and a flow's step tree are rebuilt */
  edges: Edge[];

  /** With `withCode`: each code type and function of the slice, by identity, to the source of its declaration */
  code?: Record<string>;
}

/**
 * A node a change reaches, with a shortest connected path to a changed node (query.Affected).
 * The path keeps the original edges; an observes edge may be followed in either direction.
 */
model Affected {
  node: Node;
  path: Edge[];
}

/** The search box: the nodes whose identity or summary contains the text, sorted by identity (`unicode node search`). */
@route("/search")
@get
op search(
  @query text: string,
  @query kind?: NodeKind,

  /** At most this many; 50 when not given */
  @query limit?: int32,
): Node[];

/** The slice of the nodes (`unicode slice`). */
@route("/slice")
@get
op getSlice(
  @query(#{ explode: true }) id: string[],
  @query(#{ explode: true }) expand?: string[],
  @query withCode?: boolean,
): Slice | NotFound;

/** The salient nodes a change of the nodes reaches, grouped by kind (`unicode impact`, docs/design.md §7.4). */
@route("/impact")
@get
op getImpact(@query(#{ explode: true }) id: string[]): Affected[] | NotFound;

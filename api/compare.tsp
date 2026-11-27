import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/** How an item differs between two snapshots, aligned by identity (docs/model.md §8): 新增、删除、修改、不变. */
enum ChangeKind {
  added,
  removed,
  modified,
  unchanged,
}

/** The values of one attribute key that a change removed and added. */
model AttrChange {
  key: string;
  removed: string[];
  added: string[];
}

/** The change of one node; modified means its content hash differs. */
model NodeChange {
  id: string;
  change: ChangeKind;

  /** The node in the base snapshot */
  before?: Node;

  /** The node in the served snapshot */
  after?: Node;

  /** The attribute values it gained and lost, such as an added field `status: OrderStatus` */
  attrs: AttrChange[];

  /** A code element: how many lines of its source differ between the two revisions */
  linesChanged?: int32;
}

/** The change of one edge; modified means its attributes differ. */
model EdgeChange {
  kind: EdgeKind;
  from: string;
  to: string;
  change: ChangeKind;
  before?: Edge;
  after?: Edge;
}

/** What differs between the snapshot of a base revision and the served snapshot (`unicode diff <rev>`). */
model Comparison {
  base: Revision;

  /** The working tree the served snapshot was read from */
  head: WorkingTree;

  /** The added, removed and modified nodes, sorted by identity */
  nodes: NodeChange[];

  /** The added, removed and modified edges, sorted by kind, start and end */
  edges: EdgeChange[];
}

/**
 * The comparison the compare button opens. The server builds the base revision's snapshot as a query's `--rev`
 * does, and keeps it while it runs.
 */
@route("/compare")
@get
op compare(@query base: string): Comparison | BadRequest;

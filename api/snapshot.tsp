import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/** A commit of the repository. */
model Revision {
  /** The full commit hash */
  commit: string;

  /** The abbreviated hash, as `git rev-parse --short` gives it */
  short: string;

  /** The branch checked out at it, when there is one */
  branch?: string;

  /** The first line of its message */
  subject: string;

  time: utcDateTime;
}

/** The working tree the snapshot was read from: the commit it is checked out at, and whether it has changes. */
model WorkingTree {
  ...Revision;

  /** The working tree differs from the commit */
  dirty: boolean;
}

/** How much the snapshot holds. */
model Counts {
  nodes: int32;
  edges: int32;

  /** The diagnostics of checking the snapshot */
  diagnostics: int32;

  /** Declared items and depends edges by status */
  statuses: Record<int32>;
}

/** The snapshot the server answers from: the top bar, the header's diagnostic count and the status bar. */
model Snapshot {
  /** The system container; its attrs give `notations` (which labelled views exist) and `roots` */
  system: Node;

  /** The repository's working tree when the snapshot was read */
  head: WorkingTree;

  /** The snapshot file, relative to the repository root */
  path: string;

  /** When the snapshot file was written */
  builtAt: utcDateTime;

  counts: Counts;
}

/** A node of the design tree, with the nodes it contains. */
model TreeNode {
  /** The design node; `attrs.focus` of a container is its effective focus, and a utility container is drawn dimmed */
  node: Node;

  children: TreeNode[];
}

/** The snapshot the server answers from. */
@route("/snapshot")
@get
op getSnapshot(): Snapshot;

/**
 * The design tree: the system, then its containers by focus (core, then support), the external systems, the persons
 * and the utility containers, each group by name; inside a container, what it declares in the order the design file
 * declares it. A type holds its operations. Steps and cases are left out: a flow's steps are on its views and page.
 */
@route("/tree")
@get
op getTree(): TreeNode;

/** The latest commits of the checked-out branch, newest first, from which the compare button picks its base. */
@route("/revisions")
@get
op listRevisions(
  /** At most this many; 20 when not given */
  @query limit?: int32,
): Revision[];

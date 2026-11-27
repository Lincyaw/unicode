namespace Unicode.Viewer;

/** A place in a file (diag.Position). Line and column start at 1; the column counts bytes. */
model Position {
  /** Relative to the repository root, with forward slashes */
  path: string;

  line: int32;
  column: int32;
}

/** The layer of a node (docs/model.md §1). */
enum Layer {
  design,
  code,
  evidence,
  review,
}

/** Where a node or an edge comes from (docs/model.md §2). */
enum Source {
  declared,
  extracted,
  derived,
  produced,
}

/** How a declared item compares with the code (docs/model.md §6): 已验证、缺失、未实现、不一致. */
enum Status {
  verified,
  missing,
  stub,
  contradicted,
}

/** The kind of a node (docs/model.md §3). */
enum NodeKind {
  container,
  `type`,
  `op`,
  actor,
  flow,
  step,
  property,
  observation,
  `case`,
  run,
  result,
  review,
}

/** The kind of an edge (docs/model.md §4). */
enum EdgeKind {
  contains,
  uses,
  `extends`,
  implements,
  refers,
  subscribes,
  imports,
  calls,
  from,
  emits,
  handles,
  about,
  realizes,
  checks,
  generates,
  observes,
  owns,
  depends,
  traces,
  exercises,
  reaches,
  covers,
}

/**
 * One node of the graph: the node of the snapshot (graph.Node with its content hash, as `unicode build` writes it),
 * and what the viewer needs to name it, colour its status and link it.
 */
model Node {
  /** The identity (docs/model.md §7) */
  id: string;

  kind: NodeKind;

  /** The form; its values depend on the kind and the layer (docs/model.md §3) */
  form: string;

  layer: Layer;
  source: Source;

  /** Where it is written: a design, corpus or code file */
  origin: Position;

  /** The summary; a property's is its statement */
  summary: string;

  /** Salient or detail (docs/model.md §9, D6) */
  salient: boolean;

  /** The content hash, SHA-256 in lower-case hex (docs/model.md §6) */
  digest: string;

  /** Everything else, as lists of strings in the canonical format; the keys are those of docs/design.md §5.2 */
  attrs: Record<string[]>;

  /**
   * The name the page shows for it. A design node: the last segment of its identity, a step's index written `#n`. A
   * code element: its name in its language, a method's qualified by its type (`Payments.charge`); a file: its path.
   */
  title: string;

  /** The status `unicode check` gives it; only declared design items have one */
  status?: Status;

  /** A code element: its realizes, checks, generates or observes edges with the design nodes they point at */
  bindings?: Neighbor[];

  /** A step: where it stands in its flow */
  step?: StepInfo;
}

/** One edge of the graph: the edge of the snapshot, and its status when it is a declared item or a depends. */
model Edge {
  kind: EdgeKind;
  from: string;
  to: string;
  source: Source;
  origin: Position;

  /** Its attributes: a calls edge's `sites`, a reaches edge's `via`, a depends edge's `imports` and `calls` */
  attrs: Record<string[]>;

  /** The status `unicode check` gives it; a contradicted depends is a dependency the design does not allow */
  status?: Status;
}

/** A node one edge away from the node `focus`, with that edge (query.Neighbor). */
model Neighbor {
  focus: string;
  edge: Edge;
  node: Node;
}

/** Where a step stands in its flow: what a one-line step such as `OrderService → payment.Payments.charge(orderId, amount)` names. */
model StepInfo {
  /** The identity of its flow */
  flow: string;

  /** The sender, along `from` */
  sender?: Node;

  /** What it calls (an operation), emits (an event) or handles (an error) */
  target?: Node;

  /** The owner of the operation it calls: a type, a container or an external system */
  receiver?: Node;

  /** The identity of the step it is nested in */
  parent?: string;

  /** The identities of its direct sub-steps, in their order */
  children: string[];
}

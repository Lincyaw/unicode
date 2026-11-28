import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/** The kind of a view (docs/design.md §7.1), one per tab: 架构、领域、类、时序、活动、事件、调用、性质、代码、C4. */
enum ViewKind {
  architecture,
  ddd,
  class,
  sequence,
  activity,
  events,
  calls,
  property,
  code,
  c4,
}

/** The layer the class view draws: the design's types, or the code types that realize them. */
enum ClassLayer {
  design,
  code,
}

/** The level the C4 view draws: the system with its persons and external systems, or the containers inside it. */
enum C4Level {
  context,
  container,
}

/** Whether a member of a type is a field or an enum value. */
enum MemberKind {
  field,
  value,
}

/**
 * A field or an enum value of a design type, compared with the code type that realizes it (docs/model.md §10, rule 18).
 * A field the code has and the design does not is a detail and is not listed; an enum value is listed either way.
 */
model Member {
  kind: MemberKind;
  name: string;

  /** A field: its type as the design writes it */
  type?: string;

  /** A field: its type in the realizing code type, normalized to the design's notation (docs/design.md §4) */
  codeType?: string;

  /** The design declares it */
  inDesign: boolean;

  /** The realizing code type has it */
  inCode: boolean;

  /** A field: the identities of the declared types its type names, which the page links */
  refers: string[];

  /**
   * verified when the code has it alike; missing when the code lacks it; contradicted when the types differ or an enum
   * value is only in the code. Absent when no code type realizes the type.
   */
  status?: Status;
}

/** A node a view draws, with what its card shows beyond the node. */
model ViewNode {
  ...Node;

  /** A type drawn as a card: its fields and enum values, compared with the code */
  members?: Member[];

  /**
   * A container drawn as a card. Expanded: the types (with their operations), flows, properties and errors it
   * declares. Folded: the types it declares, which the card lists by name.
   */
  contents?: ViewNode[];

  /** A code file or type: how many detail functions it contains that the view does not draw */
  details?: int32;

  /** M6: the latest results and coverage, for a case, a property, a flow, a step or a function */
  evidence?: Evidence;
}

/** An edge a view draws. */
model ViewEdge {
  ...Edge;

  /**
   * Set when an end is a folded container that stands for the nodes in it: how many edges of the graph this edge
   * stands for. Its `from` or `to` is then the container, so it is not an edge of the graph and has no preview.
   */
  count?: int32;

  /**
   * The operations the edge carries, sorted by their text, then by identity: for a depends, those its calls realize;
   * for an edge from a person, those the person calls in flows.
   */
  labels?: EdgeLabel[];
}

/** An operation an edge carries, as the edge's label names it. */
model EdgeLabel {
  /** The operation's identity */
  `op`: string;

  /** How the label writes it: its design name; for a call from a person, with the step's arguments, such as submit(cart) */
  text: string;
}

/** The data of one view: what it draws, folded and ordered; the layout is the page's. */
model View {
  kind: ViewKind;

  /** The input node: the system when the request names none */
  input: Node;

  /** The nodes that contain the input along contains, the system first */
  ancestors: Node[];

  /** The expansions the view applied, as the request gave them */
  expand: string[];

  layer?: ClassLayer;
  level?: C4Level;

  /** The nodes it draws, in the view's order: a flow's lifelines and steps in their order, else by identity */
  nodes: ViewNode[];

  edges: ViewEdge[];

  /** The support containers drawn folded */
  folded: string[];

  /** How many utility containers it does not draw */
  hidden: int32;

  /** The diagnostics whose subject is a node or an edge it draws, the nodes in `contents` included */
  diagnostics: Diagnostic[];

  /** Where each view's tab leads from the view's input, in the tabs' order */
  tabs: ViewTab[];
}

/**
 * Why a view's tab cannot be opened from a node (docs/design.md §7.5.1): the system file does not declare the view's
 * notation, or the graph holds no input the view takes.
 */
enum ClosedTab {
  `undeclared-notation`,
  `no-input`,
}

/**
 * Where a view's tab leads from a node (docs/design.md §7.5.1): the node's related view of the kind, else the default
 * input its scopes give (the node, its ancestors, the system); or, when there is none, why. Exactly one of `view` and
 * `closed` is given.
 */
model ViewTab {
  kind: ViewKind;

  /** The view to open; it applies, as /views/{kind} would answer it */
  view?: ViewRequest;

  /** Why the tab cannot be opened */
  closed?: ClosedTab;
}

/** A view to open from a page, as its address writes it. */
model ViewRequest {
  kind: ViewKind;
  input: Node;
  expand: string[];
  layer?: ClassLayer;
  level?: C4Level;

  /** The node to select in it, such as the step `order.submit-order#3` */
  select?: string;
}

/**
 * The data of a view (docs/design.md §7.1–§7.2). An identity in `expand` is a container, a salient function, or a
 * reaches edge written `reaches <from> <to>`. The architecture, events, DDD and C4 views take the whole system when
 * `input` is not given.
 */
@route("/views/{kind}")
@get
op getView(
  @path kind: ViewKind,
  @query input?: string,
  @query(#{ explode: true }) expand?: string[],

  /** The class view only; design when not given */
  @query layer?: ClassLayer,

  /** The C4 view only; container when not given */
  @query level?: C4Level,
): View | NotFound | BadRequest | Refusals;

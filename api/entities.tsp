import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/**
 * The page of one node: a view's right pane previews it, and the canvas shows it in full as the node's entity page.
 * The common sections come first; then exactly one section for the node's kind, present when the kind has one.
 */
model Entity {
  /** The node, with its status */
  node: Node;

  /** The nodes that contain it along contains, the system first; the header's path */
  ancestors: Node[];

  /** The diagnostics whose subject is the node, an edge at it, or a node the page lists */
  diagnostics: Diagnostic[];

  /**
   * 相关视图: the views that take the node as input; the class, DDD and architecture views of its container; the
   * sequence and activity views of the flows it takes part in; the calls view of its operations or of the function
   * that realizes it; the property view of its properties.
   */
  views: ViewRequest[];

  /** Where each view's tab leads from the node, in the tabs' order */
  tabs: ViewTab[];

  /**
   * 实现: the code elements that realize it, each with its realizes edge. For an external system, the clients that
   * realize its types and operations. A code element gives what it realizes in `node.bindings` instead.
   */
  realizedBy: Neighbor[];

  /** 相关性质: the properties about it, or about one of its operations; for a step, about its flow or what it calls */
  properties: Node[];

  /**
   * 参与的流程: the steps it takes part in, each with its StepInfo: the steps that call it or one of its operations,
   * call an operation whose signature refers to it, emit it or handle it, or that it sends as a person.
   */
  flows: Node[];

  /** 代码: the source of the code elements that realize it; a code element's own source */
  sources: CodeExcerpt[];

  /** M6: the cases whose covers edges reach it */
  coveredBy?: Node[];

  /** M6: its latest results and coverage */
  evidence?: Evidence;

  /** 变更, when the request gives a base: the change of the node and of the code elements that realize it */
  changes?: NodeChange[];

  /** M7: 评审 */
  review?: Review;

  /** A design container or an actor */
  container?: ContainerDetail;

  /** A design type */
  `type`?: TypeDetail;

  /** A design operation */
  `op`?: OpDetail;

  /** A flow */
  flow?: FlowDetail;

  /** A property */
  property?: PropertyDetail;

  /** A step */
  step?: StepDetail;

  /** A case */
  `case`?: CaseDetail;

  /** A code element: a package, a file, a type or a function */
  code?: CodeDetail;
}

/** The sections of a container's or an actor's page. */
model ContainerDetail {
  /** 包含: the nodes it declares (an external system's types and operations) */
  contents: Node[];

  /** 依赖: its uses and depends edges, each with the node at the other end; a contradicted depends is undeclared */
  dependsOn: Neighbor[];

  /** 被依赖: the uses and depends edges that reach it */
  dependedOnBy: Neighbor[];

  /** 代码: the code files it owns, along owns */
  files: Node[];
}

/** One operation of a type, as the type's page lists it. */
model OpRow {
  /** The operation; `attrs.signature` is its signature */
  `op`: Node;

  /** 实现: the functions that realize it */
  realizedBy: Neighbor[];

  /** 相关性质: the properties about it */
  properties: Node[];
}

/** The sections of a type's page. */
model TypeDetail {
  /** 字段 (or the enum values): compared with the code */
  members: Member[];

  /** 操作 */
  ops: OpRow[];

  /** 引用: the types it and its operations refer to */
  refers: Node[];

  /** 被引用: the types and operations that refer to it */
  referredBy: Node[];

  /** The types it extends or implements, with the edge */
  supertypes: Neighbor[];

  /** The types that extend or implement it, with the edge */
  subtypes: Neighbor[];

  /** An event, 发布: the steps that emit it */
  emittedBy: Node[];

  /** An event, 订阅: the operations that subscribe to it */
  subscribers: Node[];

  /** An error: the operations whose signature raises it */
  raisedBy: Node[];

  /** An error: the on steps that handle it */
  handledBy: Node[];
}

/** The sections of an operation's page. */
model OpDetail {
  /** 签名中的类型: the types its signature refers to, errors included */
  signatureTypes: Node[];

  /** 调用方: the functions that call a function realizing it, each with the calls or reaches edge */
  callers: Neighbor[];

  /** 被调用方: the salient functions a function realizing it calls or reaches, each with the edge; a reaches edge's `via` lists the detail functions between */
  callees: Neighbor[];

  /** The event it subscribes to */
  subscribes?: Node;
}

/** What checks a property or a flow. */
model Checked {
  /** 谓词: the function with the checks binding, and that edge */
  predicate?: Neighbor;

  /** The generators of its generated symbols, each with its generates edge */
  generators: Neighbor[];

  /** 用例: its declared and recorded cases, in the order of the design and then of the corpus */
  cases: CaseRow[];
}

/** One case of a property or a flow, as its table lists it. */
model CaseRow {
  /** The case; `form` is declared or recorded, `attrs.values` its values, `attrs.reason` why it was recorded */
  node: Node;

  /** M6: its latest result and its coverage gain */
  evidence?: Evidence;
}

/** The sections of a flow's page. */
model FlowDetail {
  ...Checked;

  /** Its steps in their order, each with its StepInfo */
  steps: Node[];
}

/** The sections of a property's page. Its inputs (`forall`, `where`) are `attrs.symbols` and `attrs.limits`. */
model PropertyDetail {
  ...Checked;

  /** 约束对象: what it is about; an operation's `attrs.signature` is shown beside it */
  about: Node[];
}

/** The sections of a step's page. Its own one-line form is `node.step`. */
model StepDetail {
  /** An on step: 所在调用, the call step it follows */
  call?: Node;

  /** 子步骤: its direct sub-steps, each with its StepInfo */
  children: Node[];

  /** 代码追溯 · 实现: its traces edges, each with the function that realizes what it calls */
  traces: Neighbor[];

  /**
   * 代码追溯 · 调用位置: the calls edges into a traced function from the code of the enclosing call's operation or of
   * the sender; `attrs.sites` gives where.
   */
  callSites: Neighbor[];
}

/** The sections of a case's page. Its values are `node.attrs.values`. */
model CaseDetail {
  /** M6, 覆盖的函数: its covers edges, each with the function or step it reaches; `attrs.lines` gives the lines */
  covers: Neighbor[];
}

/** The sections of a code element's page. What it realizes or checks is `node.bindings`; where it is, `node.origin`. */
model CodeDetail {
  /** A file: the design container that owns it */
  owner?: Node;

  /** A file: its types and functions; a type: its methods */
  contents: Node[];

  /** 调用方: the functions that call it, each with the calls or reaches edge */
  callers: Neighbor[];

  /** 被调用方: the salient functions it calls or reaches, each with the edge */
  callees: Neighbor[];

  /** 细节函数: the detail functions it calls directly */
  details: Node[];

  /** A type or a function: the first line of its declaration as the source writes it, the signature the page shows */
  declaration?: string;
}

/** The preview of one edge that a view draws, such as a reaches or a depends edge. */
model EdgeEntity {
  /** The edge review, evaluated against its identity and content. */
  review?: Review;

  /** The edge, with its status */
  edge: Edge;

  fromNode: Node;
  toNode: Node;

  /**
   * The nodes its attrs name: the detail functions of a reaches edge's `via` (经过的细节函数), the files and
   * functions of a depends edge's `imports` and `calls`
   */
  nodes: Node[];

  /** 相关步骤: the steps that trace to its end, each with its StepInfo */
  steps: Node[];

  /** The diagnostics whose subject is the edge */
  diagnostics: Diagnostic[];

  /** The views it opens: the view that expands it, the sequence view of each related step */
  views: ViewRequest[];
}

/** The page of a node. With `base`, `changes` compares it with that revision. */
@route("/entity")
@get
op getEntity(@query id: string, @query base?: string): Entity | NotFound | BadRequest | Refusals;

/** The preview of an edge. */
@route("/edge")
@get
op getEdge(@query kind: EdgeKind, @query from: string, @query to: string): EdgeEntity | NotFound | BadRequest | Refusals;

/**
 * Several nodes by identity, in the order asked: the entries of the navigation trail. An identity the snapshot no
 * longer has is left out, so a trail written before a rebuild still loads.
 */
@route("/nodes")
@get
op listNodes(@query(#{ explode: true }) id: string[]): Node[] | BadRequest | Refusals;

/** One node with the edges that leave it and reach it, sorted by kind, start and end (`unicode node get`). */
@route("/node")
@get
op getNode(@query id: string): NodeView | NotFound | BadRequest | Refusals;

/** A node with the edges that leave it and those that reach it (query.NodeView). */
model NodeView {
  node: Node;
  outgoing: Edge[];
  incoming: Edge[];
}

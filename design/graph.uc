component graph "图：节点与边的类型、合并、推导、摘要、序列化和比较" {
  focus core
  paths "internal/graph/**"
  uses diag
}

type Layer enum "节点所在的抽象层（model.md §1）" {
  value design
  value code
  value evidence
  value review
}

type Source enum "节点或边的来源（model.md §2）" {
  value declared
  value extracted
  value derived
  value produced
}

type NodeKind enum "节点的种类（model.md §3）" {
  value container
  value type
  value op
  value actor
  value flow
  value step
  value property
  value case
  value run
  value result
  value review
}

type EdgeKind enum "边的种类（model.md §4）" {
  value contains
  value uses
  value extends
  value implements
  value refers
  value subscribes
  value imports
  value calls
  value from
  value emits
  value handles
  value about
  value realizes
  value checks
  value generates
  value owns
  value depends
  value traces
  value exercises
  value reaches
  value covers
}

type Node record "一个节点：标识、种类、形式、层、来源、出处、摘要和属性；属性的键见 design.md §5" {
  field id: string
  field kind: NodeKind
  field form: string
  field layer: Layer
  field source: Source
  field origin: diag.Position
  field summary: string
  field attrs: map<string, list<string>>
}

type Edge record "一条边：种类、起点、终点、来源、出处和属性" {
  field kind: EdgeKind
  field from: string
  field to: string
  field source: Source
  field origin: diag.Position
  field attrs: map<string, list<string>>
}

type DuplicateNode error "图中已有这个标识的节点"

type DuplicateEdge error "图中已有这条边：种类、起点和终点都相同"

type DanglingEdge error "边的起点或终点不是图中的节点"

type Graph object "一张图：节点按标识索引，边按种类、起点和终点索引" {
  op addNode(node: Node) raises DuplicateNode "加入一个节点"
  op addEdge(edge: Edge) raises DuplicateEdge, DanglingEdge "加入一条边；它的起点和终点必须已经在图中"
  op node(id: string) -> optional<Node> "按标识取一个节点"
  op nodes() -> list<Node> "全部节点，按标识排序"
  op edges() -> list<Edge> "全部边，按种类、起点、终点排序"
  op digest(id: string) -> string "节点自身内容的 SHA-256：种类、形式、摘要和属性的规范编码，不含子节点"
  op encode() -> bytes "编码成确定的 JSON 快照（model.md §8）"
}

type Content record "一组节点和边，供性质使用" {
  field nodes: list<Node>
  field edges: list<Edge>
}

op newGraph() -> Graph "一张空图"

property encode-ignores-order relation "同一组节点和边，以任何次序加入，编码出的快照逐字节相同；快照中节点按标识、边按种类、起点、终点排序" {
  about Graph.encode
  forall content: Content generated "标识互不相同的节点，以及起点和终点都在这些节点之中、彼此互不相同的边"
  forall rotation: int
  where rotation in 0..1000
}

property digest-follows-content invariant "两个节点的摘要相同，当且仅当它们的种类、形式、摘要和属性都相同" {
  about Graph.digest
  forall first: Node
  forall second: Node
}

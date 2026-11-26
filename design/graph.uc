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

type Status enum "声明项与代码比较的状态（model.md §6）" {
  value verified
  value missing
  value stub
  value contradicted
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
  field salient: bool
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
  op addFragment(fragment: Fragment) raises DuplicateNode, DuplicateEdge, DanglingEdge "加入一组节点和边，先节点后边；终点不在图中的绑定边和方法的候选边先暂存，留给推导"
  op derive() -> list<diag.Diagnostic> "按 model.md §9 推导 owns、方法的 realizes、depends、traces、exercises、显著性和 reaches；暂存的绑定边中终点仍不存在的，报为 graph.unresolved-binding"
  op node(id: string) -> optional<Node> "按标识取一个节点"
  op nodes() -> list<Node> "全部节点，按标识排序"
  op edges() -> list<Edge> "全部边，按种类、起点、终点排序"
  op digest(id: string) -> string "节点自身内容的 SHA-256：种类、形式、摘要和属性的规范编码，不含子节点"
  op encode() -> bytes "编码成确定的 JSON 快照（model.md §8）"
}

type Fragment record "一组节点和边：一种语言抽取出的代码层，或者性质的输入" {
  field nodes: list<Node>
  field edges: list<Edge>
}

op newGraph() -> Graph "一张空图"

property encode-ignores-order relation "同一组节点和边，以任何次序加入，编码出的快照逐字节相同；快照中节点按标识、边按种类、起点、终点排序" {
  about Graph.encode
  forall content: Fragment generated "标识互不相同的节点，以及起点和终点都在这些节点之中、彼此互不相同的边"
  forall rotation: int
  where rotation in 0..1000
}

property derive-matches-definition relation "推导出的边和显著性，与按 model.md §9 的定义逐条直接计算的结果相同" {
  about Graph.derive
  forall fragment: Fragment generated "一张随机的小图：带路径模式的设计层容器、类型、操作、流程和性质，带绑定的代码层文件、类型、函数和调用"
}

property digest-follows-content invariant "两个节点的摘要相同，当且仅当它们的种类、形式、摘要和属性都相同" {
  about Graph.digest
  forall first: Node
  forall second: Node
}

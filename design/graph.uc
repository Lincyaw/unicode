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
  value observation
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
  value observes
  value substitutes
  value publishes
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

type NotASnapshot error "字节不是 encode 写出的快照：不是 JSON，字段不符，或者其中的节点和边组不成一张图"

type Graph object "一张图：节点按标识索引，边按种类、起点和终点索引" {
  codec encode, graph.decode
  op addNode(node: Node) raises DuplicateNode "加入一个节点"
  op addEdge(edge: Edge) raises DuplicateEdge, DanglingEdge "加入一条边；它的起点和终点必须已经在图中"
  op addFragment(fragment: Fragment) raises DuplicateNode, DuplicateEdge, DanglingEdge "加入一组节点和边，先节点后边；终点不在图中的绑定边和方法的候选边先暂存，留给推导"
  op derive() -> list<diag.Diagnostic> "按 model.md §9 推导 owns、方法的 realizes、depends、traces、exercises、显著性和 reaches；暂存的绑定边中终点仍不存在的，报为 graph.unresolved-binding"
  op node(id: string) -> optional<Node> pure "按标识取一个节点"
  op nodes() -> list<Node> pure "全部节点，按标识排序"
  op edges() -> list<Edge> pure "全部边，按种类、起点、终点排序"
  op digest(id: string) -> string pure "节点自身内容的 SHA-256：种类、形式、摘要和属性的规范编码，不含子节点"
  op encode() -> bytes pure "编码成确定的 JSON 快照（model.md §8）"
}

type Fragment record "一组节点和边：一种语言抽取出的代码层，或者性质的输入" {
  field nodes: list<Node>
  field edges: list<Edge>
}

type ChangeKind enum "两个快照之间一项的差别（model.md §8）" {
  value added
  value removed
  value modified
  value unchanged
}

type AttrChange record "一个属性键上，比较去掉的值和加上的值" {
  field key: string
  field removed: list<string>
  field added: list<string>
}

type NodeChange record "一个节点的差别：标识、差别的种类、它在 base 和 head 中的样子，以及属性上去掉和加上的值" {
  field id: string
  field change: ChangeKind
  field before: optional<Node>
  field after: optional<Node>
  field attrs: list<AttrChange>
}

type EdgeChange record "一条边的差别：种类、起点、终点、差别的种类，以及它在 base 和 head 中的样子" {
  field kind: EdgeKind
  field from: string
  field to: string
  field change: ChangeKind
  field before: optional<Edge>
  field after: optional<Edge>
}

type Comparison record "两个快照的差别：新增、删除和修改的节点，按标识排序；新增、删除和修改的边，按种类、起点、终点排序" {
  field nodes: list<NodeChange>
  field edges: list<EdgeChange>
}

type FingerprintSample record "同一输入图、添加运行产物的图、修改源位置的图，以及三条受控变化的边" {
  field base: Graph
  field artifacts: Graph
  field moved: Graph
  field edge: Edge
  field relocatedEdge: Edge
  field changedEdge: Edge
}

op newGraph() -> Graph "一张空图"

op edgeDigest(edge: Edge) -> string pure "边的身份与属性的规范 SHA-256，与 compare 判断边修改的含义相同，不含出处"

op inputDigest(g: Graph) -> string pure "执行输入图的 SHA-256，排除 evidence/review、全部 case、本工具生成代码及关联边；保留其余节点和边及其位置，避免把旧覆盖行套到新源码"

op compare(base: Graph, head: Graph) -> Comparison pure "按标识对齐两张图的节点和边（model.md §8），列出 head 相对 base 新增、删除和修改的项：节点的修改是内容哈希不同，边的修改是属性不同"

op decode(data: bytes) -> Graph raises NotASnapshot "encode 的逆：从快照还原出图"

property encode-ignores-order relation "同一组节点和边，以任何次序加入，编码出的快照逐字节相同；快照中节点按标识、边按种类、起点、终点排序" {
  about Graph.encode
  forall content: Fragment generated "标识互不相同的节点，以及起点和终点都在这些节点之中、彼此互不相同的边"
  forall rotation: int
  where rotation in 0..1000
  manual "需要从片段构造多张图、排列输入或核对规范编码和推导闭包；当前符号不能直接表达这些操作"
}

property decode-inverts-encode relation "解码 encode 写出的快照，再编码，得到逐字节相同的快照；解码任何字节都不会 panic，不是快照的字节得到 NotASnapshot" {
  about decode, Graph.encode
  forall content: Fragment generated "标识互不相同的节点，以及起点和终点都在这些节点之中、彼此互不相同的边"
  forall noise: bytes generated "沿用零至十二段 JSON 附近片段与任意字节的混合生成器"
  manual "需要从片段构造多张图、排列输入或核对规范编码和推导闭包；当前符号不能直接表达这些操作"
}

property derive-matches-definition relation "推导出的边和显著性，与按 model.md §9 的定义逐条直接计算的结果相同" {
  about Graph.derive
  forall fragment: Fragment generated "一张随机的小图：带路径模式的设计层容器、类型、操作、流程和性质，带绑定的代码层文件、类型、函数和调用"
  manual "需要从片段构造多张图、排列输入或核对规范编码和推导闭包；当前符号不能直接表达这些操作"
}

property digest-follows-content invariant "相同的种类、形式、摘要和属性得到相同的内容哈希；标识、层、来源、出处与显著性不参与；内容哈希是 64 位小写十六进制的 SHA-256" {
  about Graph.digest
  forall first: Node generated "沿用小字母表、短属性、空列表与随机元数据的节点生成器"
  forall second: Node generated "依赖 first，沿用独立节点、相同内容不同元数据和局部内容改动三个分支"
  manual "需要从片段构造多张图、排列输入或核对规范编码和推导闭包；当前符号不能直接表达这些操作"
}

property compare-follows-identity relation "compare 按 model.md §8 对齐节点和边，恰列出新增、删除和修改的项并按此排序，before 和 after 是该项在 base 和 head 中的内容" {
  about compare
  forall base: Fragment generated "标识互不相同的节点，以及起点和终点都在这些节点之中、彼此互不相同的边"
  forall head: Fragment generated "base 的一份改动：去掉一些节点和边，改动一些节点的内容和一些边的属性，改动一些项的出处、来源、层或显著性而不改内容，再加上一些新的节点和边"
  manual "需要从片段构造多张图、排列输入或核对规范编码和推导闭包；当前符号不能直接表达这些操作"
}

property compare-lists-changed-attrs relation "比较中每个节点的 attrs 按键排序，恰列出两边的值不同的键，每个键上去掉和加上的值分别是 base 和 head 中的值的子序列，两者没有相同的值，各自除去之后剩下的值相同" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，形式、摘要和属性取自含引号、< 和中文的短文字，可以有空的属性列表和自环，节点都不显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容或属性、五分之一只改出处、来源、层或显著性，其余不变，再加上零至三个节点和零至四条边，新节点可能用回删去的节点的标识"
  manual "需要判断去掉和加上的值是不是两边的值的子序列，表达式没有子序列的判断"
}

property compare-reverses relation "compare(head, base) 是 compare(base, head) 的反面：列出的项和次序相同，新增与删除互换，修改仍是修改，before 与 after 互换，每个属性键上去掉的值与加上的值互换" {
  about compare
  forall base: Fragment generated "标识互不相同的节点，以及起点和终点都在这些节点之中、彼此互不相同的边"
  forall head: Fragment generated "base 的一份改动：去掉一些节点和边，改动一些节点的内容和一些边的属性，改动一些项的出处、来源、层或显著性而不改内容，再加上一些新的节点和边"
  manual "需要从片段构造多张图、排列输入或核对规范编码和推导闭包；当前符号不能直接表达这些操作"
}

property compare-with-itself-is-empty postcondition "一张图与它自己比较，或者与以另一种次序加入同一组节点和边的图比较，没有新增、删除和修改" {
  about compare
  forall content: Fragment generated "沿用标识互异的节点和两端存在的互异边的完整原分布"
  forall rotation: int
  forall snapshot: Graph generated "按 content 的原始次序建图，不读取比较结果"
  forall reordered: Graph generated "按 rotation 轮换相同节点和边的插入次序建图"
  where rotation in 0..1000
  call compare(snapshot, reordered)
  ensure size(result.nodes) == 0 and size(result.edges) == 0
  ensure size(compare(snapshot, snapshot).nodes) == 0 and size(compare(snapshot, snapshot).edges) == 0
}

property node-sequence-keeps-index transition "逐次加入节点后，每个节点都能按标识取回，且声明错误不减少原有节点数量" {
  about Graph.addNode
  forall snapshot: Graph generated "沿用原有随机图生成器，包含节点、边和既有标识"
  forall steps: list<call<Graph.addNode>>
  where steps size 0..8
  ensure size(snapshot.nodes()) >= old(size(snapshot.nodes()))
  ensure all(n in snapshot.nodes(): snapshot.node(n.id) == n)
}

property fingerprints-separate-inputs-and-artifacts postcondition "运行产物不改变执行输入摘要，源位置变化改变它；边的出处不参与内容摘要，属性变化参与" {
  about inputDigest, edgeDigest
  forall sample: FingerprintSample generated "独立构造基础图、运行产物、源位置与边属性变化，不调用被测摘要计算预期"
  call inputDigest(sample.base)
  ensure size(result) == 64
  ensure result == inputDigest(sample.artifacts)
  ensure result != inputDigest(sample.moved)
  ensure edgeDigest(sample.edge) == edgeDigest(sample.relocatedEdge)
  ensure edgeDigest(sample.edge) != edgeDigest(sample.changedEdge)
}

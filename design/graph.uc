component graph "图：节点与边的类型、合并、推导、内容哈希、序列化和比较" {
  focus core
  paths "internal/graph/**"
  uses diag
}

type Layer enum "节点所在的层：设计、代码、证据或评审（model.md §1）" {
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

type Node record "一个节点：标识、种类、形式、层、来源、出处、摘要、是否显著和属性；属性的键见 design.md §5.2" {
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
  op addFragment(fragment: Fragment) raises DuplicateNode, DuplicateEdge, DanglingEdge "加入一组节点和边，先节点后边；绑定边和方法的候选边（design.md §4）的终点还不在图中时先暂存，推导时再加入"
  op derive() -> list<diag.Diagnostic> "按 model.md §9 推导边和显著性，并把终点仍不在图中的暂存绑定边报为 graph.unresolved-binding"
  op node(id: string) -> optional<Node> pure "按标识取一个节点"
  op nodes() -> list<Node> pure "全部节点，按标识排序"
  op edges() -> list<Edge> pure "全部边，按种类、起点、终点排序"
  op digest(id: string) -> string pure "节点自身内容的 SHA-256：种类、形式、摘要和属性的规范编码，不含子节点"
  op encode() -> bytes pure "编码成确定的 JSON 快照（model.md §8）"
}

type Fragment record "可以整体加入图中的一组节点和边，例如一种语言的后端抽取出的代码层" {
  field nodes: list<Node>
  field edges: list<Edge>
}

type ChangeKind enum "比较中一项的差别（model.md §8）" {
  value added
  value removed
  value modified
  value unchanged
}

type AttrChange record "一个属性键上的差别：从 base 到 head 去掉的值和加上的值" {
  field key: string
  field removed: list<string>
  field added: list<string>
}

type NodeChange record "一个节点的差别：标识、差别的种类、该节点在 base 和 head 中的内容，以及各属性键上去掉和加上的值" {
  field id: string
  field change: ChangeKind
  field before: optional<Node>
  field after: optional<Node>
  field attrs: list<AttrChange>
}

type EdgeChange record "一条边的差别：种类、起点、终点、差别的种类，以及该边在 base 和 head 中的内容" {
  field kind: EdgeKind
  field from: string
  field to: string
  field change: ChangeKind
  field before: optional<Edge>
  field after: optional<Edge>
}

type Comparison record "两张图的差别：新增、删除和修改的节点，按标识排序；新增、删除和修改的边，按种类、起点、终点排序" {
  field nodes: list<NodeChange>
  field edges: list<EdgeChange>
}

type FingerprintSample record "fingerprints-separate-inputs-and-artifacts 的输入：一张图，加上运行产物的同一张图，改了源位置的同一张图，以及一条边、只改出处的这条边和改了属性的这条边" {
  field base: Graph
  field artifacts: Graph
  field moved: Graph
  field edge: Edge
  field relocatedEdge: Edge
  field changedEdge: Edge
}

op newGraph() -> Graph "一张空图"

op edgeDigest(edge: Edge) -> string pure "边的种类、起点、终点和属性的规范编码的 SHA-256，不含出处，与 compare 判断边是否修改的依据相同"

op inputDigest(g: Graph) -> string pure "执行输入哈希：g 中决定执行的部分的 SHA-256，不含证据层、评审层、用例、生成文件中的代码及与它们相连的边，含其余节点和边及其出处，所以源码移动之后旧的覆盖行号不再适用"

op compare(base: Graph, head: Graph) -> Comparison pure "按标识对齐两张图的节点和边（model.md §8），列出 head 相对 base 新增、删除和修改的项：节点的修改是内容哈希不同，边的修改是属性不同"

op decode(data: bytes) -> Graph raises NotASnapshot "encode 的逆：从快照还原出图"

property encode-ignores-order relation "同一组节点和边以任何次序加入，编码出的快照都逐字节相同，快照中节点按标识、边按种类、起点和终点排序，逐项写出它们的字段（属性为空时也写出），节点另带与 digest 相同的内容哈希" {
  about Graph.encode
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点都不显著"
  forall rotation: int
  where rotation in 0..1000
  manual "需要把编码出的快照解析回节点和边再核对次序和字段，表达式不能解析 JSON"
}

property decode-inverts-encode relation "解码 encode 写出的快照不出错，再编码得到逐字节相同的快照，解码任何字节都不会 panic，出错时错误都是 NotASnapshot" {
  about decode, Graph.encode
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点都不显著"
  forall noise: bytes generated "零至十二段拼成的字节串：每段约六分之五是一个 JSON 记号（括号、引号、冒号、逗号、空格、0、-1、null、true，或带引号的 nodes、edges、id、attrs、origin、a），其余是一个任意字节"
  manual "需要把噪声字节插入编码出的快照再解码，表达式不能拼接字节串"
}

property derive-matches-definition relation "推导出的边及其属性、显著性和诊断，与按 model.md §9 的定义逐条直接计算的结果相同，终点不在图中的边不留在图中，诊断都带原因和修正、位于 subject 的出处" {
  about Graph.derive
  forall fragment: Fragment generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边"
  manual "需要求出调用的传递闭包和 contains 的祖先链，表达式没有递归或不定次的迭代"
}

property digest-follows-content invariant "种类、形式、摘要和属性相同的两个节点得到相同的内容哈希，不论标识、层、来源和出处是否相同，内容哈希写成 64 位小写十六进制" {
  about Graph.digest
  forall first: Node generated "任意节点：标识、形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，种类是容器、类型或操作，属性至多三个键、每键至多两个值，可以没有属性或有空的列表，层、来源和出处随机，都不显著"
  forall second: Node generated "取决于 first，三种情形各约三分之一：与 first 无关的节点，内容与 first 相同而标识、层、来源和出处另取的节点（空的属性列表有时换成 nil 列表），或在 first 上试改一处的节点：换种类、交换形式与摘要、摘要后加字、设一个属性键或给一个键加值，改后可能与 first 相同"
  manual "需要判断内容哈希的每个字符都是小写十六进制数字，表达式没有逐字符的字符串操作"
}

property compare-follows-identity relation "compare 按标识对齐节点、按种类、起点和终点对齐边，恰列出新增、删除和修改的项（节点的修改是内容哈希不同，边的修改是属性不同），节点按标识、边按种类、起点和终点排序，before 和 after 是该项在 base 和 head 中的内容" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点都不显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  manual "需要逐项核对比较结果的先后次序，表达式不能按位置取列表的元素"
}

property compare-lists-changed-attrs relation "比较中每个节点的 attrs 按键排序，恰列出值不同或只在一边出现的键，每个键上去掉和加上的值分别是 base 和 head 中的值的子序列，两者没有相同的值，各自除去之后剩下的值及其个数相同，不计次序" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点都不显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  manual "需要判断去掉和加上的值是不是两边的值的子序列，表达式没有子序列的判断"
}

property compare-reverses relation "compare(head, base) 与 compare(base, head) 列出相同的项且次序相同，其中新增与删除、before 与 after、每个属性键上去掉与加上的值都互换" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点都不显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  manual "需要按位置逐项对照两个方向的比较结果，表达式不能按位置取列表的元素"
}

property compare-with-itself-is-empty postcondition "一张图与它自己比较，或者与以另一种次序加入同一组节点和边的图比较，没有新增、删除和修改" {
  about compare
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点都不显著"
  forall rotation: int
  forall snapshot: Graph generated "按 content 中的次序把节点和边加入一张图"
  forall reordered: Graph generated "把同一组节点和边按 rotation 轮换次序后加入另一张图"
  where rotation in 0..1000
  call compare(snapshot, reordered)
  ensure size(result.nodes) == 0 and size(result.edges) == 0
  ensure size(compare(snapshot, snapshot).nodes) == 0 and size(compare(snapshot, snapshot).edges) == 0
}

property node-sequence-keeps-index transition "依次调用 addNode 时，每次调用之后（包括抛出 DuplicateNode 的调用）节点数不减少，图中的每个节点都能按标识取回" {
  about Graph.addNode
  forall snapshot: Graph generated "按片段的次序把 encode-ignores-order 的 content 那样的节点和边加入图中，不推导"
  forall steps: list<call<Graph.addNode>>
  where steps size 0..8
  ensure size(snapshot.nodes()) >= old(size(snapshot.nodes()))
  ensure all(n in snapshot.nodes(): snapshot.node(n.id) == n)
}

property fingerprints-separate-inputs-and-artifacts postcondition "执行输入哈希不随运行产物改变而随源位置改变，edgeDigest 不随出处改变而随属性改变" {
  about inputDigest, edgeDigest
  forall sample: FingerprintSample generated "一张随机的图，加上证据或评审节点的同一张图，移动了一处源位置的同一张图，以及一条边和它只改出处、只改属性的两个变体"
  call inputDigest(sample.base)
  ensure size(result) == 64
  ensure result == inputDigest(sample.artifacts)
  ensure result != inputDigest(sample.moved)
  ensure edgeDigest(sample.edge) == edgeDigest(sample.relocatedEdge)
  ensure edgeDigest(sample.edge) != edgeDigest(sample.changedEdge)
}

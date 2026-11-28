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

type Effect enum "效果的种类（model.md §3.1）：时间或随机数、文件系统、进程和网络" {
  value time
  value fs
  value process
  value network
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
  op edge(kind: EdgeKind, from: string, to: string) -> optional<Edge> pure "按种类、起点和终点取一条边"
  op digest(id: string) -> string pure "节点自身内容的 SHA-256：种类、形式、摘要和属性的规范编码，不含子节点"
  op snapshot() -> Snapshot pure "快照的记录形式：节点按标识、边按种类、起点、终点排序，各字段取自节点和边，节点另带 digest 给出的内容哈希"
  op encode() -> bytes pure "把 snapshot 编码成确定的 JSON 快照（model.md §8）"
  op effects() -> map<string, OperationEffects> pure "按 model.md §9 D8 求出的效果：每个代码层操作和每个有实现的设计层操作的标识，对应它的直接效果、后继、效果和每个效果种类的来历，种类按 time、fs、process、network 的次序"
  op dispatch(id: string) -> list<string> pure "调用接口的抽象方法 id 时可能执行的方法：实现该接口的每个代码层类型自己的同名方法，没有时取它经嵌入得到的最近的同名方法，按标识排序；其他操作没有"
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

type EffectWitness record "一个操作的一个效果种类的来历（model.md §9 D8）：via 为空、rank 为零时这个种类是操作自己的直接效果，否则它来自后继 via，via 的这个种类的 rank 更小" {
  field kind: Effect
  field via: string
  field rank: int
}

type OperationEffects record "effects 给一个操作的结果：直接效果、后继（代码层操作是它调用的代码层操作和接口分派的目标，设计层操作是实现它的代码层操作）、效果，以及每个效果种类的来历" {
  field direct: list<Effect>
  field successors: list<string>
  field effects: list<Effect>
  field witnesses: list<EffectWitness>
}

type EffectSample record "effects-follow-calls、effects-read-the-graph 和 dispatch-finds-implementations 的输入：一张推导过的图，生成它时给每个代码层操作写下的直接效果，以及其中的一个抽象方法" {
  field snapshot: Graph
  field direct: map<string, list<Effect>>
  field abstract: string
}

type SnapshotNode record "快照中的一个节点：Node 的各字段，另加 digest 给出的内容哈希；编码成 JSON 时键名就是字段名" {
  field id: string
  field kind: NodeKind
  field form: string
  field layer: Layer
  field source: Source
  field origin: diag.Position
  field summary: string
  field salient: bool
  field digest: string
  field attrs: map<string, list<string>>
}

type SnapshotEdge record "快照中的一条边：Edge 的各字段；编码成 JSON 时键名就是字段名" {
  field kind: EdgeKind
  field from: string
  field to: string
  field source: Source
  field origin: diag.Position
  field attrs: map<string, list<string>>
}

type Snapshot record "快照的记录形式：节点和边；编码成 JSON 时键名就是字段名" {
  field nodes: list<SnapshotNode>
  field edges: list<SnapshotEdge>
}

type Usage record "DeriveSample 中的一条函数间的调用边或导入边：种类、起点、终点，以及 depends 的属性中列出它的写法" {
  field kind: EdgeKind
  field from: string
  field to: string
  field label: string
}

type DeriveSample record "derive 各性质的输入：一个片段，以及生成它时就定下的事实：各代码文件被哪些设计层容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边" {
  field fragment: Fragment
  field matches: map<string, list<string>>
  field depth: map<string, int>
  field badPatterns: list<string>
  field dangling: map<string, diag.Position>
  field files: map<string, string>
  field sourceFiles: map<string, string>
  field ancestors: map<string, list<string>>
  field externalOps: map<string, string>
  field usages: list<Usage>
}

type FingerprintSample record "fingerprints-separate-inputs-and-artifacts 的输入：一张图，加上运行产物的同一张图，移动了一处源位置的同一张图，以及一条边、改了出处和来源的这条边和改了属性的这条边" {
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

op decode(data: bytes) -> Graph pure raises NotASnapshot "encode 的逆：从快照还原出图"

property encode-ignores-order relation "同一组节点和边以原来的、轮换后的和打乱后的次序分别加入三张图，三张图编码出的快照逐字节相同" {
  about Graph.encode
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall rotation: int
  forall inOrder: Graph generated "按 content 中的次序把节点和边加入一张新图"
  forall rotated: Graph generated "把 content 的节点和边各按 rotation 轮换次序后加入一张新图"
  forall shuffled: Graph generated "把 content 的节点和边各以 rotation 为种子打乱次序后加入一张新图"
  where rotation in 0..1000
  ensure inOrder.encode() == rotated.encode() and inOrder.encode() == shuffled.encode()
}

property snapshot-copies-content postcondition "snapshot 恰列出 content 的每个节点和每条边各一次，各字段与 content 中的相同，节点另带 digest 给出的内容哈希" {
  about Graph.snapshot
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall g: Graph generated "按 content 中的次序把节点和边加入一张新图"
  call g.snapshot()
  ensure size(result.nodes) == size(content.nodes) and size(result.edges) == size(content.edges)
  ensure all(n in content.nodes: any(r in result.nodes: r.id == n.id and r.kind == n.kind and r.form == n.form and r.layer == n.layer and r.source == n.source and r.origin == n.origin and r.summary == n.summary and r.salient == n.salient and r.attrs == n.attrs and r.digest == g.digest(n.id)))
  ensure all(e in content.edges: any(r in result.edges: r.kind == e.kind and r.from == e.from and r.to == e.to and r.source == e.source and r.origin == e.origin and r.attrs == e.attrs))
}

property snapshot-json-is-the-record invariant "encode 写出的 JSON 恰有 nodes 和 edges 两个键，各节点和各边的键恰是 SnapshotNode 和 SnapshotEdge 的字段名，值与 snapshot 给出的相同，属性为空时写作 {}，没有 null" {
  about Graph.encode, Graph.snapshot
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  manual "需要把编码出的字节解析成 JSON 再逐键核对，表达式不能解析 JSON"
}

property decode-inverts-encode relation "解码 encode 写出的快照，得到的图的节点和边与原图的相同，再编码得到逐字节相同的快照" {
  about decode, Graph.encode
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall g: Graph generated "按 content 中的次序把节点和边加入一张新图"
  ensure decode(g.encode()).nodes() == g.nodes() and decode(g.encode()).edges() == g.edges()
  ensure decode(g.encode()).encode() == g.encode()
}

property decode-is-total postcondition "解码 data 抛出的错误只有 NotASnapshot；不抛出时，得到的图编码之后再解码，编码不变" {
  about decode
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall noise: bytes generated "零至十二段拼成的字节串：每段约六分之五是一个 JSON 记号（括号、引号、冒号、逗号、空格、0、-1、null、true，或带引号的 nodes、edges、id、attrs、origin、a），其余是一个任意字节"
  forall alone: bool
  forall data: bytes generated "alone 为真时是 noise；否则把 noise 插进按 content 中的次序建出的图编码出的快照，位置是 noise 的字节数除以快照的字节数加一的余数"
  call decode(data)
  ensure raised(NotASnapshot) or decode(result.encode()).encode() == result.encode()
}

property derive-keeps-resolved-edges postcondition "推导之后，图中来源不是 derived 的边恰是片段中终点是图中节点的、不是方法候选边的边，与片段中的相同" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(e in sample.fragment.edges: not (e.kind == EdgeKind.realizes and e.source == Source.derived) implies (snapshot.node(e.to) != none implies snapshot.edge(e.kind, e.from, e.to) == e) and (snapshot.node(e.to) == none implies snapshot.edge(e.kind, e.from, e.to) == none))
  ensure all(x in snapshot.edges(): x.source != Source.derived implies any(e in sample.fragment.edges: e.kind == x.kind and e.from == x.from and e.to == x.to))
}

property derive-reports-unresolved-bindings postcondition "推导对片段中终点不是节点的每条绑定边恰给出一条 graph.unresolved-binding 诊断，对象是 sample.dangling 中这条边的种类、起点和终点，位置是它的出处，此外没有这个码的诊断" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(s in sample.dangling: count(d in result: d.code == "graph.unresolved-binding" and d.subject == s and d.position == sample.dangling[s]) == 1)
  ensure count(d in result: d.code == "graph.unresolved-binding") == size(sample.dangling)
}

property derive-reports-invalid-patterns postcondition "推导对每个不合写法的路径模式给出一条 graph.bad-path-pattern 诊断，对象是带这个模式的设计层容器，位置是容器的出处，此外没有这个码的诊断" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(c in sample.badPatterns: count(d in result: d.code == "graph.bad-path-pattern" and d.subject == c and d.position == snapshot.node(c).origin) == count(x in sample.badPatterns: x == c))
  ensure count(d in result: d.code == "graph.bad-path-pattern") == size(sample.badPatterns)
}

property derive-diagnostics-are-described postcondition "推导给出的诊断码只有 graph.unresolved-binding 和 graph.bad-path-pattern，每条诊断都有原因和修正" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(d in result: (d.code == "graph.unresolved-binding" or d.code == "graph.bad-path-pattern") and size(d.message) > 0 and size(d.fix) > 0)
}

property owns-follows-deepest-pattern postcondition "推导之后，代码文件 f 与设计层容器 c 之间有 owns 边，当且仅当 c 有合乎写法的模式匹配 f 的路径，且这样的容器中没有比 c 更深的" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(e in snapshot.edges(): e.kind == EdgeKind.owns implies e.to in sample.matches and e.from in sample.matches[e.to])
  ensure all(f in sample.matches: all(c in sample.matches[f]: (snapshot.edge(EdgeKind.owns, c, f) != none) == all(o in sample.matches[f]: sample.depth[o] <= sample.depth[c])))
}

property method-realizes-follows-candidates postcondition "推导之后，图中来源为 derived 的 realizes 边恰是这样的方法候选边 (m, o)：o 是设计层的操作，包含 o 的设计层类型被包含 m 的代码层类型实现" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(c in sample.fragment.edges: c.kind == EdgeKind.realizes and c.source == Source.derived implies (snapshot.edge(EdgeKind.realizes, c.from, c.to) != none) == (snapshot.node(c.to) != none and snapshot.node(c.to).layer == Layer.design and snapshot.node(c.to).kind == NodeKind.op and any(t in sample.ancestors[c.from]: snapshot.node(t).kind == NodeKind.type and any(p in sample.fragment.edges: p.kind == EdgeKind.contains and p.to == c.to and snapshot.node(p.from).layer == Layer.design and snapshot.node(p.from).kind == NodeKind.type and snapshot.edge(EdgeKind.realizes, t, p.from) != none))))
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.realizes and x.source == Source.derived implies any(c in sample.fragment.edges: c.kind == EdgeKind.realizes and c.source == Source.derived and c.from == x.from and c.to == x.to))
}

property depends-lists-calls postcondition "推导之后，对设计层容器 A ≠ B，A 拥有的函数调用 B 拥有的函数或外部系统 B 的客户端时，有 depends(A, B)，它的 calls 属性列出这条调用，拥有按 sample.matches 中最深的容器算" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(u in sample.usages: u.kind == EdgeKind.calls implies all(a in sample.matches[sample.files[u.from]]: all(b in sample.matches[sample.files[u.to]]: a != b and all(o in sample.matches[sample.files[u.from]]: sample.depth[o] <= sample.depth[a]) and all(o in sample.matches[sample.files[u.to]]: sample.depth[o] <= sample.depth[b]) implies snapshot.edge(EdgeKind.depends, a, b) != none and "calls" in snapshot.edge(EdgeKind.depends, a, b).attrs and u.label in snapshot.edge(EdgeKind.depends, a, b).attrs["calls"])))
  ensure all(u in sample.usages: u.kind == EdgeKind.calls implies all(r in sample.fragment.edges: r.kind == EdgeKind.realizes and r.from == u.to and r.to in sample.externalOps and snapshot.edge(EdgeKind.realizes, r.from, r.to) != none implies all(a in sample.matches[sample.files[u.from]]: all(o in sample.matches[sample.files[u.from]]: sample.depth[o] <= sample.depth[a]) implies snapshot.edge(EdgeKind.depends, a, sample.externalOps[r.to]) != none and "calls" in snapshot.edge(EdgeKind.depends, a, sample.externalOps[r.to]).attrs and u.label in snapshot.edge(EdgeKind.depends, a, sample.externalOps[r.to]).attrs["calls"])))
}

property depends-lists-imports postcondition "推导之后，对设计层容器 A ≠ B，A 拥有的文件导入含 B 拥有的文件的包时，有 depends(A, B)，它的 imports 属性列出这条导入，拥有按 sample.matches 中最深的容器算" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(u in sample.usages: u.kind == EdgeKind.imports implies all(g in sample.fragment.edges: g.kind == EdgeKind.contains and g.from == u.to and g.to in sample.matches implies all(a in sample.matches[u.from]: all(b in sample.matches[g.to]: a != b and all(o in sample.matches[u.from]: sample.depth[o] <= sample.depth[a]) and all(o in sample.matches[g.to]: sample.depth[o] <= sample.depth[b]) implies snapshot.edge(EdgeKind.depends, a, b) != none and "imports" in snapshot.edge(EdgeKind.depends, a, b).attrs and u.label in snapshot.edge(EdgeKind.depends, a, b).attrs["imports"]))))
}

property depends-attrs-are-distinct-usages postcondition "推导之后，每条 depends(A, B) 的 A ≠ B，属性只有 calls 和 imports 两个键，每个键至少有一个值，每个值只出现一次" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.depends implies x.from != x.to and size(x.attrs) > 0 and all(k in x.attrs: (k == "calls" or k == "imports") and size(x.attrs[k]) > 0 and all(v in x.attrs[k]: count(w in x.attrs[k]: w == v) == 1)))
}

property depends-values-are-usages postcondition "推导之后，每条 depends(A, B) 的 calls 的每个值是 A 拥有的函数调用 B 拥有的函数或外部系统 B 的客户端的一条调用，imports 的每个值是 A 拥有的文件导入含 B 拥有的文件的包的一条导入，拥有按 sample.matches 中最深的容器算" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.depends and "calls" in x.attrs implies all(v in x.attrs["calls"]: any(u in sample.usages: u.kind == EdgeKind.calls and u.label == v and x.from in sample.matches[sample.files[u.from]] and all(o in sample.matches[sample.files[u.from]]: sample.depth[o] <= sample.depth[x.from]) and (x.to in sample.matches[sample.files[u.to]] and all(o in sample.matches[sample.files[u.to]]: sample.depth[o] <= sample.depth[x.to]) or any(r in sample.fragment.edges: r.kind == EdgeKind.realizes and r.from == u.to and r.to in sample.externalOps and sample.externalOps[r.to] == x.to and snapshot.edge(EdgeKind.realizes, r.from, r.to) != none)))))
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.depends and "imports" in x.attrs implies all(v in x.attrs["imports"]: any(u in sample.usages: u.kind == EdgeKind.imports and u.label == v and x.from in sample.matches[u.from] and all(o in sample.matches[u.from]: sample.depth[o] <= sample.depth[x.from]) and any(g in sample.fragment.edges: g.kind == EdgeKind.contains and g.from == u.to and g.to in sample.matches and x.to in sample.matches[g.to] and all(o in sample.matches[g.to]: sample.depth[o] <= sample.depth[x.to])))))
}

property traces-follow-realizations postcondition "推导之后，traces(s, f) 在图中，当且仅当 s 是形式为 call 或 send 的步骤，f 是代码层的操作，s 调用的某个操作由 f 实现" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(t in snapshot.edges(): t.kind == EdgeKind.traces implies snapshot.node(t.from).kind == NodeKind.step and (snapshot.node(t.from).form == "call" or snapshot.node(t.from).form == "send") and snapshot.node(t.to).layer == Layer.code and snapshot.node(t.to).kind == NodeKind.op and any(c in sample.fragment.edges: c.kind == EdgeKind.calls and c.from == t.from and snapshot.edge(EdgeKind.realizes, t.to, c.to) != none))
  ensure all(c in sample.fragment.edges: c.kind == EdgeKind.calls and snapshot.node(c.from).kind == NodeKind.step and (snapshot.node(c.from).form == "call" or snapshot.node(c.from).form == "send") implies all(r in sample.fragment.edges: r.kind == EdgeKind.realizes and r.to == c.to and snapshot.edge(EdgeKind.realizes, r.from, r.to) != none and snapshot.node(r.from).layer == Layer.code and snapshot.node(r.from).kind == NodeKind.op implies snapshot.edge(EdgeKind.traces, c.from, r.from) != none))
}

property exercises-close-over-calls postcondition "推导之后，exercises 边都从带解析了的 checks 绑定的谓词出发、指向代码层的操作，谓词直接调用的和从它执行到的函数所调用的代码层操作都由它执行到" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.exercises implies snapshot.node(x.to).layer == Layer.code and snapshot.node(x.to).kind == NodeKind.op and any(k in sample.fragment.edges: k.kind == EdgeKind.checks and k.from == x.from and snapshot.node(k.to) != none))
  ensure all(c in sample.fragment.edges: c.kind == EdgeKind.calls and snapshot.node(c.to).layer == Layer.code and snapshot.node(c.to).kind == NodeKind.op implies all(k in sample.fragment.edges: k.kind == EdgeKind.checks and snapshot.node(k.to) != none and (k.from == c.from or snapshot.edge(EdgeKind.exercises, k.from, c.from) != none) implies snapshot.edge(EdgeKind.exercises, k.from, c.to) != none))
}

property exercises-paths-lead-to-predicates postcondition "推导之后，每条 exercises(p, g) 的 path 列出的函数都由 p 执行到，path 为空时 p 直接调用 g，否则 path 中有调用 g 的函数 h，exercises(p, h) 的 path 比它少一项" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.exercises implies all(h in x.attrs["path"]: snapshot.edge(EdgeKind.exercises, x.from, h) != none) and (size(x.attrs["path"]) == 0 and snapshot.edge(EdgeKind.calls, x.from, x.to) != none or any(h in x.attrs["path"]: snapshot.edge(EdgeKind.calls, h, x.to) != none and size(snapshot.edge(EdgeKind.exercises, x.from, h).attrs["path"]) + 1 == size(x.attrs["path"]))))
}

property design-nodes-are-salient postcondition "推导之后，设计层的节点都显著" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(n in snapshot.nodes(): n.layer == Layer.design implies n.salient)
}

property code-salience-follows-bindings postcondition "推导之后，代码层的类型和操作显著，当且仅当它有解析了的 realizes、checks、generates、observes、substitutes 或 publishes 边，且它的源文件不是生成文件" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(n in snapshot.nodes(): n.layer == Layer.code and (n.kind == NodeKind.type or n.kind == NodeKind.op) implies n.salient == (any(e in sample.fragment.edges: e.from == n.id and (e.kind == EdgeKind.realizes or e.kind == EdgeKind.checks or e.kind == EdgeKind.generates or e.kind == EdgeKind.observes or e.kind == EdgeKind.substitutes or e.kind == EdgeKind.publishes) and snapshot.edge(e.kind, e.from, e.to) != none) and not ("generated" in snapshot.node(sample.sourceFiles[n.id]).attrs and "true" in snapshot.node(sample.sourceFiles[n.id]).attrs["generated"])))
}

property container-salience-follows-elements postcondition "推导之后，代码层的文件和包显著，当且仅当它不是生成文件，并且是某个显著的类型或操作的源文件或包含它" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(c in snapshot.nodes(): c.layer == Layer.code and c.kind == NodeKind.container implies c.salient == (not ("generated" in c.attrs and "true" in c.attrs["generated"]) and any(e in snapshot.nodes(): e.salient and e.layer == Layer.code and (e.kind == NodeKind.type or e.kind == NodeKind.op) and (c.id in sample.ancestors[e.id] or sample.sourceFiles[e.id] == c.id))))
}

property reaches-joins-salient-functions postcondition "推导之后，每条 reaches(f, g) 的 f 和 g 是不同的显著的代码层操作，f 不直接调用 g，via 非空，其中的函数各出现一次，都是代码层的细节操作，各由 f 或 via 中的函数调用，各调用 g 或 via 中的函数" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  call snapshot.derive()
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.reaches implies x.from != x.to and snapshot.node(x.from).salient and snapshot.node(x.to).salient and snapshot.node(x.from).layer == Layer.code and snapshot.node(x.from).kind == NodeKind.op and snapshot.node(x.to).layer == Layer.code and snapshot.node(x.to).kind == NodeKind.op and snapshot.edge(EdgeKind.calls, x.from, x.to) == none and size(x.attrs["via"]) > 0)
  ensure all(x in snapshot.edges(): x.kind == EdgeKind.reaches implies all(h in x.attrs["via"]: count(w in x.attrs["via"]: w == h) == 1 and snapshot.node(h).layer == Layer.code and snapshot.node(h).kind == NodeKind.op and not snapshot.node(h).salient and (snapshot.edge(EdgeKind.calls, x.from, h) != none or any(w in x.attrs["via"]: snapshot.edge(EdgeKind.calls, w, h) != none)) and (snapshot.edge(EdgeKind.calls, h, x.to) != none or any(w in x.attrs["via"]: snapshot.edge(EdgeKind.calls, h, w) != none))))
}

property reaches-covers-detail-chains invariant "f 和 g 是不同的显著的函数、f 不直接调用 g、f 经一个或多个细节函数调用 g 时，推导得出 reaches(f, g)，via 恰列出这些调用链上的每个细节函数" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  manual "需要求出细节函数之间调用的传递闭包，表达式没有递归或不定次的迭代"
}

property derive-ignores-order postcondition "同一个片段的节点和边以原来的和轮换后的次序加入两张图，两张图推导之后编码出的快照相同" {
  about Graph.derive
  forall sample: DeriveSample generated "一张随机的小图：系统下零至四个可嵌套、各带零至二个路径模式（其中有不合写法的）的组件，有时有外部系统，组件和外部系统下有类型和操作，组件有时带含嵌套步骤的流程和带具名观察的性质；代码层有包、文件（有的是生成文件）、结构体和接口类型及其方法、函数、调用和导入，类型、方法和函数的绑定有的指向不存在或种类不符的节点，类型的绑定附带方法的候选边，没有绑定的类型有时也带指向某个设计层类型的操作的候选边，有外部系统时约一半另有一个实现它的操作的函数及对这个函数的一次调用，约四分之三另有一条从一个函数经一至两个新的没有绑定的函数到另一个函数的调用链（两端尽量取带绑定的函数）且约一半另有两端之间的直接调用，有的代码类型和函数的出处另指一个文件，并附上构造时就定下的事实：各代码文件被哪些容器的合乎写法的模式匹配，各设计层容器和外部系统的深度，每个不合写法的模式所在的容器，终点不在片段中的绑定边的诊断对象及其出处，各代码类型和函数所在的文件、源文件和包含它的各级容器，外部系统的各操作所属的外部系统，以及每条函数间的调用边和导入边连同它在 depends 属性中的写法（起点、一个空格、终点）"
  forall rotation: int
  forall snapshot: Graph generated "把 sample.fragment 加入一张新图，不推导"
  forall reordered: Graph generated "把 sample.fragment 的节点和边各按 rotation 轮换次序后加入一张新图，再推导"
  where rotation in 0..1000
  call snapshot.derive()
  ensure snapshot.encode() == reordered.encode()
}

property digest-follows-content invariant "种类、形式、摘要和属性相同的两个节点得到相同的内容哈希，不论标识、层、来源和出处是否相同" {
  about Graph.digest
  forall first: Node generated "任意节点：标识、形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，种类是容器、类型或操作，属性至多三个键、每键至多两个值，可以没有属性或有空的列表，层、来源、出处和是否显著随机"
  forall second: Node generated "取决于 first，三种情形各约三分之一：与 first 无关的节点，内容与 first 相同而标识、层、来源、出处和是否显著另取的节点（空的属性列表有时换成 nil 列表），或在 first 上试改一处的节点：换种类、交换形式与摘要、摘要后加字、设一个属性键或给一个键加值，改后可能与 first 相同"
  forall a: Graph generated "只含 first 的一张新图"
  forall b: Graph generated "只含 second 的一张新图"
  ensure first.kind == second.kind and first.form == second.form and first.summary == second.summary and first.attrs == second.attrs implies a.digest(first.id) == b.digest(second.id)
}

property digest-has-sha256-length invariant "图中每个节点的内容哈希都有 64 个字符" {
  about Graph.digest
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall g: Graph generated "按 content 中的次序把节点和边加入一张新图"
  ensure all(n in content.nodes: size(g.digest(n.id)) == 64)
}

property digest-is-lowercase-hex invariant "图中每个节点的内容哈希都只由 0 至 9 和 a 至 f 组成" {
  about Graph.digest
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  manual "需要逐个字符判断内容哈希的字符，表达式没有逐字符的字符串操作"
}

property compare-lists-changed-nodes postcondition "compare 列出的节点恰是只在 head 中的（新增）、只在 base 中的（删除）和两边都有而内容哈希不同的（修改），每个只列一次，before 和 after 是它在 base 和 head 中的节点" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  forall gb: Graph generated "按 base 中的次序把节点和边加入一张新图"
  forall gh: Graph generated "按 head 中的次序把节点和边加入一张新图"
  call compare(gb, gh)
  ensure all(c in result.nodes: count(x in result.nodes: x.id == c.id) == 1 and c.before == gb.node(c.id) and c.after == gh.node(c.id))
  ensure all(c in result.nodes: (c.change == ChangeKind.added) == (c.before == none and c.after != none) and (c.change == ChangeKind.removed) == (c.before != none and c.after == none) and (c.change == ChangeKind.modified) == (c.before != none and c.after != none))
  ensure all(c in result.nodes: c.change == ChangeKind.modified implies gb.digest(c.id) != gh.digest(c.id))
  ensure all(n in head.nodes: gb.node(n.id) == none implies any(c in result.nodes: c.id == n.id))
  ensure all(n in base.nodes: gh.node(n.id) == none implies any(c in result.nodes: c.id == n.id))
  ensure all(n in head.nodes: gb.node(n.id) != none and gb.digest(n.id) != gh.digest(n.id) implies any(c in result.nodes: c.id == n.id))
}

property compare-lists-changed-edges postcondition "compare 列出的边恰是只在 head 中的（新增）、只在 base 中的（删除）和两边都有而属性不同的（修改），每条只列一次，before 和 after 是它在 base 和 head 中的边" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  forall gb: Graph generated "按 base 中的次序把节点和边加入一张新图"
  forall gh: Graph generated "按 head 中的次序把节点和边加入一张新图"
  call compare(gb, gh)
  ensure all(c in result.edges: count(x in result.edges: x.kind == c.kind and x.from == c.from and x.to == c.to) == 1 and c.before == gb.edge(c.kind, c.from, c.to) and c.after == gh.edge(c.kind, c.from, c.to))
  ensure all(c in result.edges: (c.change == ChangeKind.added) == (c.before == none and c.after != none) and (c.change == ChangeKind.removed) == (c.before != none and c.after == none) and (c.change == ChangeKind.modified) == (c.before != none and c.after != none))
  ensure all(c in result.edges: c.change == ChangeKind.modified implies c.before.attrs != c.after.attrs)
  ensure all(e in head.edges: gb.edge(e.kind, e.from, e.to) == none implies any(c in result.edges: c.kind == e.kind and c.from == e.from and c.to == e.to))
  ensure all(e in base.edges: gh.edge(e.kind, e.from, e.to) == none implies any(c in result.edges: c.kind == e.kind and c.from == e.from and c.to == e.to))
  ensure all(e in head.edges: gb.edge(e.kind, e.from, e.to) != none and gb.edge(e.kind, e.from, e.to).attrs != e.attrs implies any(c in result.edges: c.kind == e.kind and c.from == e.from and c.to == e.to))
}

property compare-lists-changed-attrs postcondition "比较中每个节点的 attrs 每个键只列一次，恰列出在 base 和 head 中值不同或只在一边出现的键" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  forall gb: Graph generated "按 base 中的次序把节点和边加入一张新图"
  forall gh: Graph generated "按 head 中的次序把节点和边加入一张新图"
  call compare(gb, gh)
  ensure all(c in result.nodes: all(a in c.attrs: (c.before != none and a.key in c.before.attrs or c.after != none and a.key in c.after.attrs) and not (c.before != none and c.after != none and a.key in c.before.attrs and a.key in c.after.attrs and c.before.attrs[a.key] == c.after.attrs[a.key])))
  ensure all(c in result.nodes: c.before != none implies all(k in c.before.attrs: c.after == none or not k in c.after.attrs or c.after.attrs[k] != c.before.attrs[k] implies any(a in c.attrs: a.key == k)))
  ensure all(c in result.nodes: c.after != none implies all(k in c.after.attrs: c.before == none or not k in c.before.attrs or c.before.attrs[k] != c.after.attrs[k] implies any(a in c.attrs: a.key == k)))
  ensure all(c in result.nodes: all(a in c.attrs: count(b in c.attrs: b.key == a.key) == 1))
}

property compare-balances-attr-values postcondition "比较中每个键上去掉的值不多于 base 中的（计个数，键不在 base 中时没有），加上的值不多于 head 中的，同一个值不既去掉又加上，两边除去去掉和加上的值之后剩下的值及其个数相同" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  forall gb: Graph generated "按 base 中的次序把节点和边加入一张新图"
  forall gh: Graph generated "按 head 中的次序把节点和边加入一张新图"
  call compare(gb, gh)
  ensure all(c in result.nodes: all(a in c.attrs: all(v in a.removed: not v in a.added)))
  ensure all(c in result.nodes: all(a in c.attrs: (c.before != none and a.key in c.before.attrs implies all(v in a.removed: count(x in a.removed: x == v) <= count(x in c.before.attrs[a.key]: x == v))) and (not (c.before != none and a.key in c.before.attrs) implies size(a.removed) == 0)))
  ensure all(c in result.nodes: all(a in c.attrs: (c.after != none and a.key in c.after.attrs implies all(v in a.added: count(x in a.added: x == v) <= count(x in c.after.attrs[a.key]: x == v))) and (not (c.after != none and a.key in c.after.attrs) implies size(a.added) == 0)))
  ensure all(c in result.nodes: all(a in c.attrs: c.before != none and c.after != none and a.key in c.before.attrs and a.key in c.after.attrs implies all(v in c.before.attrs[a.key]: count(x in c.before.attrs[a.key]: x == v) - count(x in a.removed: x == v) == count(x in c.after.attrs[a.key]: x == v) - count(x in a.added: x == v)) and all(v in c.after.attrs[a.key]: count(x in c.before.attrs[a.key]: x == v) - count(x in a.removed: x == v) == count(x in c.after.attrs[a.key]: x == v) - count(x in a.added: x == v))))
  ensure all(c in result.nodes: all(a in c.attrs: (c.before != none and a.key in c.before.attrs and not (c.after != none and a.key in c.after.attrs) implies all(v in c.before.attrs[a.key]: count(x in a.removed: x == v) == count(x in c.before.attrs[a.key]: x == v))) and (c.after != none and a.key in c.after.attrs and not (c.before != none and a.key in c.before.attrs) implies all(v in c.after.attrs[a.key]: count(x in a.added: x == v) == count(x in c.after.attrs[a.key]: x == v)))))
}

property compare-lists-are-present invariant "compare 的结果中 nodes、edges、每个节点变化的 attrs 以及每个属性变化的 removed 和 added 都不是 nil 列表" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  manual "需要区分 nil 列表和空列表，模型值的列表不区分两者"
}

property compare-is-canonical relation "两张图的节点和边各按 rotation 轮换次序后加入两张新图，compare 给出的比较与原来的逐项相同，次序也相同" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  forall rotation: int
  forall gb: Graph generated "按 base 中的次序把节点和边加入一张新图"
  forall gh: Graph generated "按 head 中的次序把节点和边加入一张新图"
  forall rb: Graph generated "把 base 的节点和边各按 rotation 轮换次序后加入一张新图"
  forall rh: Graph generated "把 head 的节点和边各按 rotation 轮换次序后加入一张新图"
  where rotation in 0..1000
  ensure compare(gb, gh) == compare(rb, rh)
}

property compare-reverses postcondition "compare(head, base) 与 compare(base, head) 列出的节点和边一一对应，新增与删除互换、修改仍是修改，before 与 after 互换，每个属性键上去掉与加上的值互换" {
  about compare
  forall base: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
  forall head: Fragment generated "base 的一份改动：每个节点和边约五分之一删去、五分之一改内容（边只改属性，改动可能落空）、五分之一重取出处和来源（节点另重取层并翻转显著性），其余不变，端点被删去的边一并删去，再加上零至三个新节点和零至四条新边，新节点可能用回删去的节点的标识"
  forall gb: Graph generated "按 base 中的次序把节点和边加入一张新图"
  forall gh: Graph generated "按 head 中的次序把节点和边加入一张新图"
  call compare(gh, gb)
  ensure size(result.nodes) == size(compare(gb, gh).nodes) and size(result.edges) == size(compare(gb, gh).edges)
  ensure all(f in compare(gb, gh).nodes: any(b in result.nodes: b.id == f.id and b.before == f.after and b.after == f.before and (f.change == ChangeKind.added) == (b.change == ChangeKind.removed) and (f.change == ChangeKind.removed) == (b.change == ChangeKind.added) and (f.change == ChangeKind.modified) == (b.change == ChangeKind.modified) and size(b.attrs) == size(f.attrs) and all(fa in f.attrs: any(ba in b.attrs: ba.key == fa.key and ba.removed == fa.added and ba.added == fa.removed))))
  ensure all(f in compare(gb, gh).edges: any(b in result.edges: b.kind == f.kind and b.from == f.from and b.to == f.to and b.before == f.after and b.after == f.before and (f.change == ChangeKind.added) == (b.change == ChangeKind.removed) and (f.change == ChangeKind.removed) == (b.change == ChangeKind.added) and (f.change == ChangeKind.modified) == (b.change == ChangeKind.modified)))
}

property compare-with-itself-is-empty postcondition "一张图与它自己比较，或者与以另一种次序加入同一组节点和边的图比较，没有新增、删除和修改" {
  about compare
  forall content: Fragment generated "零至八个标识互不相同的节点，以及零至十二条两端都在其中、彼此不同的边：标识是 a、b、.、#、- 组成的一至四个字符，种类、层和来源任取，出处取自三个路径（含空路径）和零至三的行列，形式、摘要以及属性的键和值都是由 a、b、英文双引号、< 和汉字“中”组成的零至三个字符，属性至多三个键、每键至多两个值，可以没有属性、有空的属性列表和自环，节点各以一半机会显著"
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

property fingerprints-separate-inputs-and-artifacts postcondition "执行输入哈希有 64 个字符，图中加入证据、评审、录制用例、生成文件中的代码及与它们相连的边时不变，移动一处源位置时改变，edgeDigest 不随出处和来源改变而随属性改变" {
  about inputDigest, edgeDigest
  forall sample: FingerprintSample generated "固定形状的图：一个系统和一条性质、一个源文件中的一至五个函数及第一个函数的自调用边，摘要和函数体是至多 30 个字母、数字、<、> 或空格，行号在 1 至 1000，加上录制用例、运行、结果、评审节点和生成文件中的代码的同一张图，第一个函数后移一至十行的同一张图，以及这条调用边、改了出处和来源的变体与 labels 多一个值的变体"
  call inputDigest(sample.base)
  ensure size(result) == 64
  ensure result == inputDigest(sample.artifacts)
  ensure result != inputDigest(sample.moved)
  ensure edgeDigest(sample.edge) == edgeDigest(sample.relocatedEdge)
  ensure edgeDigest(sample.edge) != edgeDigest(sample.changedEdge)
}

property effects-follow-calls postcondition "effects 给出的结果合乎 D8 的定义：每个操作的效果含它的直接效果和它的每个后继的效果，每个效果种类恰有一个来历，来历或者是 rank 为零的直接效果，或者是有这个种类且 rank 更小的后继" {
  about Graph.effects
  forall sample: EffectSample generated "derive-keeps-resolved-edges 的 sample 那样的随机图另加零至六条边（各有一半机会是从结构体到接口的实现接口边或结构体之间的继承边），每个具体函数和方法约一半带随机的非空直接效果，推导过，direct 记下每个代码层操作带的直接效果（没有时为空），abstract 是图中随机的一个抽象方法，没有时为空串"
  call sample.snapshot.effects()
  ensure all(id in result: all(k in result[id].direct: k in result[id].effects))
  ensure all(id in result: all(s in result[id].successors: s in result implies all(k in result[s].effects: k in result[id].effects)))
  ensure all(id in result: size(result[id].witnesses) == size(result[id].effects) and all(k in result[id].effects: count(w in result[id].witnesses: w.kind == k) == 1))
  ensure all(id in result: all(w in result[id].witnesses: w.via == "" and w.rank == 0 and w.kind in result[id].direct or w.via in result[id].successors and w.via in result and any(v in result[w.via].witnesses: v.kind == w.kind and v.rank < w.rank)))
}

property effects-read-the-graph postcondition "effects 给每个代码层操作的直接效果是生成图时写下的，给设计层操作的直接效果为空，后继含图中操作之间每条 calls 边的终点、每条 realizes 边的起点和 dispatch 给抽象方法的方法，每个后继都来自这样的边或者是抽象方法的同名方法" {
  about Graph.effects, Graph.dispatch
  forall sample: EffectSample generated "与 effects-follow-calls 的 sample 相同"
  call sample.snapshot.effects()
  ensure all(id in sample.direct: id in result and result[id].direct == sample.direct[id])
  ensure all(id in result: id in sample.direct or size(result[id].direct) == 0)
  ensure all(e in sample.snapshot.edges(): e.kind == EdgeKind.calls and e.from in result and e.to in result implies e.to in result[e.from].successors)
  ensure all(e in sample.snapshot.edges(): e.kind == EdgeKind.realizes and e.from in result and e.to in result implies e.from in result[e.to].successors)
  ensure all(id in result: all(m in sample.snapshot.dispatch(id): m in result[id].successors))
  ensure all(id in result: all(s in result[id].successors: any(e in sample.snapshot.edges(): e.kind == EdgeKind.calls and e.from == id and e.to == s) or any(e in sample.snapshot.edges(): e.kind == EdgeKind.realizes and e.from == s and e.to == id) or s in sample.snapshot.dispatch(id)))
}

property dispatch-finds-implementations postcondition "dispatch 给抽象方法的每个方法都与它同名，实现该接口的类型自己有同名方法时这个方法在其中" {
  about Graph.dispatch
  forall sample: EffectSample generated "与 effects-follow-calls 的 sample 相同"
  call sample.snapshot.dispatch(sample.abstract)
  ensure all(m in result: sample.snapshot.node(m).attrs["name"] == sample.snapshot.node(sample.abstract).attrs["name"])
  ensure all(i in sample.snapshot.edges(): i.kind == EdgeKind.implements and any(c in sample.snapshot.edges(): c.kind == EdgeKind.contains and c.from == i.to and c.to == sample.abstract) implies all(c in sample.snapshot.edges(): c.kind == EdgeKind.contains and c.from == i.from and sample.snapshot.node(c.to).kind == NodeKind.op and sample.snapshot.node(c.to).attrs["name"] == sample.snapshot.node(sample.abstract).attrs["name"] implies c.to in result))
}

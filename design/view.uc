component view "视图的选取与折叠" {
  focus support
  paths "internal/view/**"
  uses graph, check
}

type ViewKind enum "视图的种类（design.md §7.1）" {
  value architecture
  value class
  value sequence
  value activity
  value events
  value calls
  value property
  value code
  value ddd
  value c4
}

type ClassLayer enum "类视图画出的层：设计中的类型，或实现它们的代码类型" {
  value design
  value code
}

type C4Level enum "C4 视图的层级：系统上下文（人、系统、外部系统），或系统中带 container 标签的容器" {
  value context
  value container
}

type NotApplicable error "这种视图不适用于这个节点，或者系统文件没有声明这种记法"

type ViewNode record "视图画出的一个节点，加上卡片上显示的内容：类型的成员比较 members、容器中的内容 contents，以及未画出的细节函数的个数 details" {
  extends graph.Node
  field members: list<check.Member>
  field contents: list<ViewNode>
  field details: int
}

type ViewEdge record "视图画出的一条边；一端是折叠的容器或 C4 元素时，这条边合并了图中的 count 条边，本身不是图中的边，labels 是经过它的调用所实现的操作" {
  extends graph.Edge
  field count: int
  field labels: list<string>
}

type View record "一个视图的数据：种类、输入节点及其祖先、请求的展开、类视图的层和 C4 视图的层级，按视图规定的次序选取的节点和边，折叠的节点，以及未画出的 utility 容器的个数" {
  field kind: ViewKind
  field input: graph.Node
  field ancestors: list<graph.Node>
  field expand: list<string>
  field layer: optional<ClassLayer>
  field level: optional<C4Level>
  field nodes: list<ViewNode>
  field edges: list<ViewEdge>
  field folded: list<string>
  field hidden: int
}

type Request record "view 各条性质的输入：一张图和一次视图请求，含视图的种类、输入节点的标识、要展开的标识、类视图的层和 C4 视图的层级" {
  field fragment: graph.Fragment
  field kind: ViewKind
  field id: string
  field expand: list<string>
  field layer: ClassLayer
  field level: C4Level
}

op select(g: graph.Graph, kind: ViewKind, id: string, expand: list<string>, layer: ClassLayer, level: C4Level) -> View raises NotApplicable "按视图的种类选取子图并折叠；层只用于类视图，层级只用于 C4 视图"

op inputs(g: graph.Graph, kind: ViewKind) -> list<string> "kind 这种视图接受的全部输入：作为输入时 select 不抛出 NotApplicable 的节点，按标识排序"

property view-is-folded-subgraph invariant "视图的节点、边、合并的边数以及折叠和隐藏的节点，与按 design.md §7.1、§7.2 的规则直接算出的相同" {
  about select
  forall request: Request generated "一张随机的图，带各种关注度的容器、显著和细节的函数，以及一次视图请求：输入约六分之五是该视图适用的节点，展开的标识约四分之三是视图中能展开的项"
  manual "需要按 design.md §7.1、§7.2 的规则另行算出视图再比较，表达式不能遍历图的边"
}

property sequence-follows-steps invariant "时序视图和活动视图中的步骤，按它们在流程中的序号排列；生命线按首次出现的次序排列" {
  about select
  forall request: Request generated "一张随机的图，其中有带嵌套步骤的流程，以及一次对某个流程的时序或活动视图请求"
  manual "需要按步骤的序号和生命线首次出现的次序排列，表达式不能排序"
}

property view-echoes-its-request postcondition "视图给出请求的种类、展开、类视图的层和 C4 视图的层级，请求给出节点时输入就是这个节点，不给时只有架构、事件、DDD 和 C4 视图适用且输入是系统" {
  about select
  forall g: graph.Graph generated "一张随机的图，带各种关注度的容器、类型、操作、流程、性质和代码层"
  forall kind: ViewKind
  forall id: string generated "约十二分之一是空串，约一半是生成的图中存在的标识，其余是不存在的标识"
  forall expand: list<string>
  forall layer: ClassLayer
  forall level: C4Level
  where expand size 0..3
  observe none
  changes none
  call select(g, kind, id, expand, layer, level)
  ensure raised(NotApplicable) or result.kind == kind and result.expand == expand
  ensure raised(NotApplicable) or (result.layer != none) == (kind == ViewKind.class) and (result.level != none) == (kind == ViewKind.c4)
  ensure raised(NotApplicable) or kind != ViewKind.class or result.layer == layer
  ensure raised(NotApplicable) or kind != ViewKind.c4 or result.level == level
  ensure raised(NotApplicable) or id == "" or result.input.id == id
  ensure raised(NotApplicable) or id != "" or result.input.form == "system" and (kind == ViewKind.architecture or kind == ViewKind.events or kind == ViewKind.ddd or kind == ViewKind.c4)
}

property inputs-are-what-select-takes relation "inputs(g, kind) 恰是图中作为输入、不带展开时 select 不抛出 NotApplicable 的节点，按标识排序" {
  about inputs, select
  forall g: graph.Graph generated "一张随机的图，带各种关注度的容器、类型、操作、流程、性质和代码层"
  forall kind: ViewKind
  manual "需要对图中的每个节点调用 select 并与 inputs 的结果比较，表达式不能遍历图的节点"
}

property view-names-input-ancestors invariant "视图的输入是图中标识为请求 id 的节点，它的祖先是沿 contains 包含它的各层节点，系统在前" {
  about select
  forall request: Request generated "一张随机的图，以及一次输入取自图中节点的视图请求"
  manual "需要在图中沿 contains 逐层找出输入的祖先并比较整个节点，表达式不能遍历图的边"
}

property view-cards-show-contents invariant "视图中卡片附带的成员、容器的内容、未画出的细节函数的个数和边上的操作名称，与按 design.md §7.5.1 的定义直接算出的相同" {
  about select, check.members
  forall request: Request generated "一张随机的图，类型带字段或枚举值，实现它们的代码类型有的字段相同、有的缺少或不同，依赖带调用，以及一次输入取自图中节点的视图请求"
  manual "需要按定义另行算出每张卡片的内容再比较，表达式不能遍历图的边"
}

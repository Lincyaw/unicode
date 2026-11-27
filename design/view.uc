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

type ClassLayer enum "类视图画出的层：设计中的类型，或落实它们的代码类型" {
  value design
  value code
}

type C4Level enum "C4 视图的层级：系统上下文（人、系统、外部系统），或系统中带 container 标签的容器" {
  value context
  value container
}

type NotApplicable error "这种视图不适用于这个节点，或者系统文件没有声明这种记法"

type ViewNode record "视图画出的一个节点，以及它的卡片在节点之外显示的内容" {
  extends graph.Node
  field members: list<check.Member>
  field contents: list<ViewNode>
  field details: int
}

type ViewEdge record "视图画出的一条边；一端是代表其中节点的折叠容器或 C4 元素时，它代表 count 条图中的边，不是图中的边" {
  extends graph.Edge
  field count: int
  field labels: list<string>
}

type View record "一个视图的数据：输入节点及其祖先、请求的展开、类视图的层和 C4 视图的层级；选取的节点和边，按视图规定的次序；折叠的节点，以及未画出的工具与基础服务的个数" {
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

type Request record "一张图和一次视图请求：视图的种类、输入节点、要展开的标识、类视图的层和 C4 视图的层级，供性质使用" {
  field fragment: graph.Fragment
  field kind: ViewKind
  field id: string
  field expand: list<string>
  field layer: ClassLayer
  field level: C4Level
}

op select(g: graph.Graph, kind: ViewKind, id: string, expand: list<string>, layer: ClassLayer, level: C4Level) -> View raises NotApplicable "按视图的种类选取子图并折叠；层只用于类视图，层级只用于 C4 视图"

property view-is-folded-subgraph invariant "视图的节点都在图中；不带 count 的边是图中两端都画出的边，带 count 的边代表 count 条同种类的图中的边，它们的两端各是这条边的一端或由它代表；不展开时没有细节节点；架构、类、调用、代码、DDD 和 C4 视图中，support 容器折叠成一个节点，utility 容器和指向它的依赖都不出现，类视图中输入之外的其他容器也折叠；时序、活动和事件视图不折叠；各视图照 design.md §7.1 选取，展开的标识照 §7.2 展开" {
  about select
  forall request: Request generated "一张随机的图，带各种关注度的容器、显著和细节的函数，以及一次输入取自图中节点的视图请求"
  manual "需要按视图规则独立计算折叠、排列与首次出现次序，当前表达式没有这些构造"
}

property sequence-follows-steps invariant "时序视图和活动视图中的步骤，按它们在流程中的序号排列；生命线按首次出现的次序排列" {
  about select
  forall request: Request generated "一张随机的图，其中有带嵌套步骤的流程，以及一次对某个流程的时序或活动视图请求"
  manual "需要按视图规则独立计算折叠、排列与首次出现次序，当前表达式没有这些构造"
}

property view-echoes-its-request postcondition "视图给出请求的种类和展开；给出输入时，输入就是它；不给输入时，只有架构、事件、DDD 和 C4 视图适用，输入是系统；类视图给出请求的层，C4 视图给出请求的层级，其余视图两者都不给" {
  about select
  forall g: graph.Graph generated "一张随机的图，带各种关注度的容器、类型、操作、流程、性质和代码层"
  forall kind: ViewKind
  forall id: string generated "空串，或生成的图常有的一个标识，偶尔是不存在的标识"
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

property view-names-input-ancestors invariant "视图的输入是图中的那个节点，它的祖先是沿 contains 包含它的各层节点，系统在前" {
  about select
  forall request: Request generated "一张随机的图，以及一次输入取自图中节点的视图请求"
  manual "需要在图中沿 contains 逐层找出输入的祖先并比较整个节点，表达式不能遍历图的边"
}

property view-cards-show-contents invariant "视图画出的设计类型，和落实设计类型的代码类型，带该设计类型与落实它的代码类型逐项比较的成员；架构视图中展开的设计容器带其中直接声明的类型（各带成员和操作）、操作、流程和性质，任何视图中折叠的容器带其中的类型；代码文件和代码类型带其中未画出的细节函数的个数；依赖边带它所代表的调用落实的操作，从人出发的调用边带人在流程中调用的操作" {
  about select, check.members
  forall request: Request generated "一张随机的图，类型带字段或枚举值，落实它们的代码类型有的字段相同、有的缺少或不同，依赖带调用，以及一次输入取自图中节点的视图请求"
  manual "需要按定义独立比较字段与枚举值、找出容器声明的元素、数出未画出的细节函数、沿调用找出落实的操作，表达式不能遍历图的边"
}

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

type Request record "view-is-folded-subgraph、sequence-follows-steps、view-names-input-ancestors 和 view-cards-show-contents 的输入：一张图和一次视图请求，含视图的种类、输入节点的标识、要展开的标识、类视图的层和 C4 视图的层级" {
  field fragment: graph.Fragment
  field kind: ViewKind
  field id: string
  field expand: list<string>
  field layer: ClassLayer
  field level: C4Level
}

op select(g: graph.Graph, kind: ViewKind, id: string, expand: list<string>, layer: ClassLayer, level: C4Level) -> View raises NotApplicable "按视图的种类选取子图并折叠；层只用于类视图，层级只用于 C4 视图"

op inputs(g: graph.Graph, kind: ViewKind) -> list<string> "kind 这种视图接受的全部输入：作为输入时 select 不抛出 NotApplicable 的节点，按标识排序"

property view-is-folded-subgraph invariant "视图适用于输入时，视图的节点、边、合并的边数、折叠的节点和未画出的 utility 容器的个数与按 design.md §7.1、§7.2 的规则直接算出的相同，不适用时 select 抛出 NotApplicable" {
  about select
  forall request: Request generated "一张随机的图（各种关注度的容器、带字段或枚举值的类型、成员与之有同有缺有异有多的代码类型、显著和细节的函数、带调用的依赖），以及一次任意种类的视图请求：输入约六分之五是该视图适用的节点，其余是任意节点或空串，零至四个展开的标识约四分之三是视图中能展开的项，其余是图中的容器、函数或不存在的名称"
  manual "需要按 design.md §7.1、§7.2 的规则另行算出视图，其中的包含和经由细节函数的调用要沿边追溯任意多层，表达式没有递归"
}

property sequence-follows-steps invariant "流程的时序视图和活动视图恰画出流程中的全部步骤并按序号排列，步骤的发起方、被调用操作的所属者和发布的事件都画成生命线，按首次出现的次序排列" {
  about select
  forall request: Request generated "一张至少有一个流程的随机图，每个流程有一至十二个步骤，两步以上时第二步嵌套在第一步中，其余随机嵌套，以及一次对其中一个流程的时序或活动视图请求，带零至四个展开的标识"
  manual "需要按步骤的序号和生命线首次出现的次序排列，表达式不能排序"
}

property view-echoes-its-request postcondition "视图给出请求的种类和展开，类视图只另给请求的层，C4 视图只另给请求的层级，其余视图两者都不给，请求给出节点时输入就是这个节点，不给时只有架构、事件、DDD 和 C4 视图适用且输入是系统" {
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

property inputs-are-what-select-takes relation "对图中每个节点，不带展开时以它为输入的 select 成功当且仅当它在 inputs(g, kind) 中，失败时只抛出 NotApplicable，inputs(g, kind) 按标识排序" {
  about inputs, select
  forall g: graph.Graph generated "一张随机的图，带各种关注度的容器、类型、操作、流程、性质和代码层"
  forall kind: ViewKind
  manual "需要对图中每个节点各调用一次 select 再与 inputs 的结果比较，表达式中只能调用纯操作，而 select 不是纯操作"
}

property view-names-input-ancestors invariant "视图适用时，输入是图中标识为请求 id 的节点，不给 id 时是系统，输入的祖先是沿 contains 包含输入的各层节点，最外层在前" {
  about select
  forall request: Request generated "一张随机的图（各种关注度的容器、带字段或枚举值的类型、成员与之有同有缺有异有多的代码类型、显著和细节的函数、带调用的依赖），以及一次任意种类的视图请求：输入约六分之五是该视图适用的节点，其余是任意节点或空串，零至四个展开的标识约四分之三是视图中能展开的项，其余是图中的容器、函数或不存在的名称"
  manual "需要沿 contains 逐层向上找出输入的全部祖先，表达式没有递归，不能沿边追溯任意多层"
}

property view-cards-show-contents invariant "视图中卡片附带的成员、容器的内容、未画出的细节函数的个数和边上的操作名称，与按 design.md §7.5.1 的定义直接算出的相同" {
  about select, check.members
  forall request: Request generated "一张随机的图（各种关注度的容器、带字段或枚举值的类型、成员与之有同有缺有异有多的代码类型、显著和细节的函数、带调用的依赖），以及一次任意种类的视图请求：输入约六分之五是该视图适用的节点，其余是任意节点或空串，零至四个展开的标识约四分之三是视图中能展开的项，其余是图中的容器、函数或不存在的名称"
  manual "需要把字段的文本拆成名称和类型再与代码类型的字段逐项比较，表达式不能切分字符串"
}

component view "视图的选取与折叠" {
  focus support
  paths "internal/view/**"
  uses graph
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

type NotApplicable error "这种视图不适用于这个节点，或者系统文件没有声明这种记法"

type View record "一个视图的数据：选取的节点和边，按视图规定的次序；折叠的节点，以及未画出的工具与基础服务的个数" {
  field kind: ViewKind
  field nodes: list<graph.Node>
  field edges: list<graph.Edge>
  field folded: list<string>
  field hidden: int
}

type Request record "一张图和一次视图请求：视图的种类、输入节点和要展开的标识，供性质使用" {
  field fragment: graph.Fragment
  field kind: ViewKind
  field id: string
  field expand: list<string>
}

op select(g: graph.Graph, kind: ViewKind, id: string, expand: list<string>) -> View raises NotApplicable "按视图的种类选取子图并折叠"

property view-is-folded-subgraph invariant "视图的节点和边都在图中，边的两端都在视图之中；不展开时没有细节节点，support 容器折叠成一个节点，utility 容器和指向它的依赖都不出现；展开的标识照 design.md §7.2 展开" {
  about select
  forall request: Request generated "一张随机的图，带各种关注度的容器、显著和细节的函数，以及一次输入取自图中节点的视图请求"
}

property sequence-follows-steps invariant "时序视图和活动视图中的步骤，按它们在流程中的序号排列；生命线按首次出现的次序排列" {
  about select
  forall request: Request generated "一张随机的图，其中有带嵌套步骤的流程，以及一次对某个流程的时序或活动视图请求"
}

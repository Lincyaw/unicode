component query "节点、引用、搜索、切片和影响" {
  focus support
  paths "internal/query/**"
  uses graph
}

type NotFound error "图中没有这个标识的节点"

type NodeView record "一个节点，以及从它出发和指向它的边" {
  field node: graph.Node
  field outgoing: list<graph.Edge>
  field incoming: list<graph.Edge>
}

type Neighbor record "从焦点一步可达的一个显著节点，以及经过的那条边" {
  field focus: string
  field edge: graph.Edge
  field node: graph.Node
}

type Context record "回答一个问题所需的最小上下文（design.md §7.3）：焦点、祖先、一步可达的显著节点、按种类补充的节点、每个焦点未列出的细节节点的个数，以及这些节点之间的 contains 边" {
  field focus: list<graph.Node>
  field ancestors: list<graph.Node>
  field neighbors: list<Neighbor>
  field extras: list<graph.Node>
  field details: map<string, int>
  field edges: list<graph.Edge>
}

type Affected record "一个受影响的节点，以及说明原因的一条最短路径" {
  field node: graph.Node
  field path: list<graph.Edge>
}

type Question record "一张图和对它提出的问题：焦点的标识、要展开的标识和搜索的文字，供性质使用" {
  field fragment: graph.Fragment
  field ids: list<string>
  field expand: list<string>
  field text: string
}

op get(g: graph.Graph, id: string) -> NodeView raises NotFound "一个节点及其边"

op refs(g: graph.Graph, id: string) -> list<graph.Edge> raises NotFound "指向这个节点的边"

op search(g: graph.Graph, text: string, kind: optional<graph.NodeKind>) -> list<graph.Node> "标识或摘要包含这段文字的节点，按标识排序"

op slice(g: graph.Graph, ids: list<string>, expand: list<string>) -> Context raises NotFound "一组节点的切片"

op impact(g: graph.Graph, ids: list<string>) -> list<Affected> raises NotFound "一组变更的节点影响到的显著节点"

property slice-is-minimal-context invariant "切片恰好含有焦点、焦点的祖先链、焦点一步可达的显著节点，以及按种类补充的显著节点：操作所属的类型和该类型的其余操作，性质沿 refers 经由类型能到达的类型，流程的整个步骤树和每一步 traces 到的函数；细节节点只计入个数，展开之后才列出；切片的 edges 恰是两端都在切片列出的节点之中的 contains 边，由此能还原祖先链和流程的步骤树；切片中的边都是图中的边" {
  about slice
  forall question: Question generated "一张随机的图，焦点和展开的标识取自图中的节点"
}

property impact-is-reverse-reachability relation "影响恰是沿 design.md §7.4 的边反向能到达焦点的显著节点；每条路径都是图中的路径，并且最短" {
  about impact
  forall question: Question generated "一张随机的图，焦点取自图中的节点"
}

property get-shows-incident-edges invariant "get 给出的出边恰是起点为该节点的边，入边恰是终点为它的边；refs 恰是终点为它的边；不存在的标识得到 NotFound" {
  about get, refs
  forall question: Question generated "一张随机的图，焦点有的取自图中的节点，有的是不存在的标识"
}

property search-matches-text invariant "搜索的结果恰是标识或摘要包含这段文字、种类相符的节点" {
  about search
  forall question: Question generated "一张随机的图，以及一段取自其中某个标识或摘要的文字，或者随机的文字"
  forall kind: optional<graph.NodeKind>
}

component query "在图上回答查询：取节点及其边、反查引用、按文字搜索、求切片和影响" {
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

type Neighbor record "从输入节点 focus 一步可达的一个显著节点 node，以及经过的边 edge" {
  field focus: string
  field edge: graph.Edge
  field node: graph.Node
}

type Context record "回答一个问题所需的最小上下文（design.md §7.3）：输入节点、祖先、一步可达的显著节点、按种类补充的节点、每个输入节点未列出的细节节点的个数，以及这些节点之间的 contains 边" {
  field focus: list<graph.Node>
  field ancestors: list<graph.Node>
  field neighbors: list<Neighbor>
  field extras: list<graph.Node>
  field details: map<string, int>
  field edges: list<graph.Edge>
}

type Affected record "一个受影响的节点，以及说明它为什么受影响的一条最短路径，路径中的边保持图中的原方向（design.md §7.4）" {
  field node: graph.Node
  field path: list<graph.Edge>
}

type Question record "query 各条性质的输入：一张图，以及输入节点的标识、要展开的标识和搜索的文字" {
  field fragment: graph.Fragment
  field ids: list<string>
  field expand: list<string>
  field text: string
}

op get(g: graph.Graph, id: string) -> NodeView raises NotFound "一个节点及其边"

op refs(g: graph.Graph, id: string) -> list<graph.Edge> raises NotFound "指向这个节点的边"

op search(g: graph.Graph, text: string, kind: optional<graph.NodeKind>) -> list<graph.Node> "标识或摘要包含 text、且种类为 kind（给出时）的节点，按标识排序"

op slice(g: graph.Graph, ids: list<string>, expand: list<string>) -> Context raises NotFound "ids 中节点的切片（design.md §7.3），expand 中的标识展开对应的细节节点"

op impact(g: graph.Graph, ids: list<string>) -> list<Affected> raises NotFound "ids 中变更的节点影响到的显著节点（design.md §7.4）"

property slice-is-minimal-context invariant "切片与按 design.md §7.3 的定义直接算出的相同：输入节点、祖先链、一步可达的显著节点、按种类补充的节点、细节节点的个数和 contains 边" {
  about slice
  forall question: Question generated "一张随机的图，输入节点和展开的标识取自图中的节点，约一半的第一个输入节点是操作、性质、观察或流程"
  manual "需要按定义沿图的边另行算出切片再比较，表达式不能遍历图的边"
}

property impact-is-reverse-reachability relation "影响恰是按 design.md §7.4 的传播规则能到达输入节点的显著节点，每个节点附一条由图中原边首尾相接组成的最短路径" {
  about impact
  forall question: Question generated "一张随机的图，一至三个输入节点取自图中的节点"
  manual "需要逐层遍历图算出最短距离，表达式不能遍历图的边"
}

property get-shows-incident-edges invariant "get 给出的出边和入边恰是起点和终点为该节点的边，refs 恰是终点为它的边，不存在的标识得到 NotFound" {
  about get, refs
  forall question: Question generated "一张随机的图，一至四个标识，约三分之一换成图中没有的标识"
  manual "需要按起点和终点筛选图中的全部边，表达式不能遍历图的边"
}

property search-matches-text invariant "搜索的结果恰是标识或摘要包含这段文字、种类与 kind 相符（给出时）的节点" {
  about search
  forall question: Question generated "一张随机的图，以及零至四个字符的文字，约一半取自某个节点的标识或摘要"
  forall kind: optional<graph.NodeKind> generated "一半没有种类，另一半是十二种节点种类之一"
  manual "需要判断一段文字是否包含另一段，表达式没有字符串的子串操作"
}

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

property slice-is-minimal-context invariant "切片与按 design.md §7.3 的定义直接算出的相同：输入节点、祖先链、一步可达的显著或展开的节点、按种类补充的显著或展开的节点、未展开的细节节点的个数，以及两端都在切片中的 contains 边" {
  about slice
  forall question: Question generated "一张随机的图（一至十二个任意种类、约一半显著的节点，标识是 a、b、. 和汉字“中”组成的一至四个字符，约四分之三挂在可以包含其种类的先前节点下，另有指向类型的 refers 边和零至二十四条多取模型中相应种类的任意边，可有自环和环，部分性质带具名观察和适配器），一至三个输入标识和零至三个展开的标识，都取自图中的节点、可以重复，图中有操作、性质、观察或流程时约一半把第一个输入标识换成其中之一"
  manual "需要沿 contains 和 refers 边反复扩展到不再增加，表达式没有递归或不定次的迭代"
}

property impact-is-reverse-reachability relation "影响恰是按 design.md §7.4 的传播规则（另沿 publishes 双向传播）能到达输入节点的、输入节点之外的显著节点，按种类分组、组内按标识排序，每个节点附一条由图中原边首尾相接、通到某个输入节点的最短路径" {
  about impact
  forall question: Question generated "一张随机的图（一至十二个任意种类、约一半显著的节点，标识是 a、b、. 和汉字“中”组成的一至四个字符，约四分之三挂在可以包含其种类的先前节点下，另有指向类型的 refers 边和零至二十四条多取模型中相应种类的任意边，可有自环和环，部分性质带具名观察和适配器），以及取自图中节点、可以重复的一至三个输入标识"
  manual "需要逐层扩展求出到输入节点的最短距离，表达式没有递归或不定次的迭代"
}

property get-shows-incident-edges invariant "get 按标识给出图中的节点，出边和入边恰是以这个节点为起点和为终点的边，refs 恰是以它为终点的边，get 和 refs 对图中没有的标识都抛出带这个标识的 NotFound" {
  about get, refs
  forall question: Question generated "一张随机的图（一至十二个任意种类、约一半显著的节点，标识是 a、b、. 和汉字“中”组成的一至四个字符，约四分之三挂在可以包含其种类的先前节点下，另有指向类型的 refers 边和零至二十四条多取模型中相应种类的任意边，可有自环和环，部分性质带具名观察和适配器），以及一至四个标识，各约三分之一换成以 ? 开头、图中没有的标识"
  manual "需要对多个标识各调用一次不纯的 get 和 refs，表达式只有一次被测调用"
}

property search-matches-text invariant "search 的结果恰是标识或摘要包含所给文字、种类与 kind 相符（给出时）的节点，按标识排序" {
  about search
  forall question: Question generated "一张随机的图（一至十二个任意种类、约一半显著的节点，标识是 a、b、. 和汉字“中”组成的一至四个字符，约四分之三挂在可以包含其种类的先前节点下，另有指向类型的 refers 边和零至二十四条多取模型中相应种类的任意边，可有自环和环，部分性质带具名观察和适配器），以及一段文字：约一半是某个节点的标识或摘要中连续的一段（可以为空），其余是 a、b、. 和汉字“中”组成的零至四个字符"
  forall kind: optional<graph.NodeKind> generated "一半没有种类，另一半是十二种节点种类之一"
  manual "需要判断一段文字是否包含另一段，表达式没有字符串的子串操作"
}

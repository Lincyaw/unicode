component check "归属、依赖、绑定、可执行、生成与录制这几组约束" {
  focus core
  paths "internal/check/**"
  uses graph, diag
}

type Checked record "检查的结果：诊断，以及每个声明项的状态" {
  field diagnostics: list<diag.Diagnostic>
  field statuses: map<string, graph.Status>
}

type Sample record "一张合乎约束的图：设计层和代码层，尚未推导，供性质使用" {
  field fragment: graph.Fragment
}

type Broken record "一张合乎约束的图做了一处违反某条约束的改动：图、改动的节点或边，以及应当得到的诊断码" {
  field fragment: graph.Fragment
  field subject: string
  field code: string
}

op conform(graph: graph.Graph) -> Checked "在推导过的图上，检查需要代码才能判断的约束（design.md §5.3）"

property accepts-conforming-graph invariant "合乎约束的图推导、检查之后没有诊断，每个声明项的状态都是 verified" {
  about conform
  forall sample: Sample generated "一张随机的、合乎 design.md §5.3 全部约束的图"
}

property rejects-broken-graph invariant "合乎约束的图做一处违反某条约束的改动之后，检查给出这条约束的诊断，对象是改动的节点或边；改动涉及的声明项，状态不再是 verified" {
  about conform
  forall broken: Broken generated "一张随机的合乎约束的图，在随机的一处违反了 design.md §5.3 中的某条约束"
}

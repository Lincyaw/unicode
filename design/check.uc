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

op conform(g: graph.Graph) -> Checked "在推导过的图上，检查需要代码才能判断的约束（design.md §5.3）"

property accepts-conforming-graph invariant "合乎约束的图推导、检查之后没有诊断，每个声明项的状态都是 verified" {
  about conform
  forall sample: Sample generated "一张随机的、合乎 design.md §5.3 全部约束的图"
  manual "需要先构造并推导带特定约束改动的图，再核对完整诊断和状态"
}

property rejects-broken-graph invariant "合乎约束的图做一处违反某条约束的改动之后，检查给出这条约束的诊断，对象是改动的节点或边；改动涉及的声明项，状态不再是 verified" {
  about conform
  forall broken: Broken generated "一张随机的合乎约束的图，在随机的一处违反了 design.md §5.3 中的某条约束"
  manual "需要先构造并推导带特定约束改动的图，再核对完整诊断和状态"
}

property conforming-result-contract postcondition "合乎约束且已推导的图没有诊断，全部状态为 verified" {
  about conform
  forall snapshot: graph.Graph generated "由合乎全部约束的图样本构造并完成推导的图"
  observe none
  changes none
  call conform(snapshot)
  ensure size(result.diagnostics) == 0
  ensure all(id in result.statuses: result.statuses[id] == graph.Status.verified)
}

property stubs-stay-incomplete invariant "绑定的谓词或生成器还是未实现骨架时，性质与流程为 stub；补齐其余完全一致的实现后恢复 verified，不把未执行的生成器当成已验证" {
  about conform
  forall sample: Sample generated "沿用完整一致图的随机生成器，逐个选择实际绑定的谓词与生成器并只改变骨架标记"
  manual "需要在随机图上改变绑定代码的骨架属性，独立核对诊断为空及设计节点的状态"
}

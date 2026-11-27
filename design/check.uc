component check "在推导过的图上检查需要代码才能判断的约束（design.md §5.3），给出诊断和每个声明项的状态" {
  focus core
  paths "internal/check/**"
  uses graph, diag
}

type Checked record "检查的结果：诊断，以及每个声明项的状态" {
  field diagnostics: list<diag.Diagnostic>
  field statuses: map<string, graph.Status>
}

type Sample record "accepts-conforming-graph 和 stubs-stay-incomplete 的输入：一张合乎约束、尚未推导的图，含设计层和代码层" {
  field fragment: graph.Fragment
}

type Broken record "rejects-broken-graph 的输入：一张合乎约束的图做了一处违反某条约束的改动，改动的节点或边，以及应当得到的诊断码" {
  field fragment: graph.Fragment
  field subject: string
  field code: string
}

type MemberKind enum "类型的成员是字段还是枚举值" {
  value field
  value value
}

type Member record "设计类型的一个成员与落实该类型的代码类型的比较（model.md §10 第 18 条）：种类、名称、设计中和代码中的类型、两边各有没有它、类型中引用的设计类型，以及比较的状态" {
  field kind: MemberKind
  field name: string
  field type: string
  field codeType: string
  field inDesign: bool
  field inCode: bool
  field refers: list<string>
  field status: optional<graph.Status>
}

op conform(g: graph.Graph) -> Checked "在推导过的图上，检查需要代码才能判断的约束（design.md §5.3）"

op members(g: graph.Graph, ids: list<string>) -> map<string, list<Member>> "ids 中每个设计类型的成员：设计中写出的字段按书写次序，枚举的值另加只在代码中出现的值；类型恰被一个代码类型落实时，各成员带比较的状态"

property accepts-conforming-graph postcondition "合乎约束的图推导、检查之后没有诊断，每个声明项和依赖恰有一个 verified 状态" {
  about conform
  forall sample: Sample generated "一张随机的、合乎 design.md §5.3 全部约束的图：二至四个组件，各有类型、操作、性质、具名观察、流程和落实它们的 Go 代码，有时带外部系统、接口的实现和继承"
  forall snapshot: graph.Graph generated "由 sample.fragment 建图并推导，保留原始输入"
  forall declaredIds: list<string> generated "从 snapshot 的声明项与依赖提取完整标识集合，不读取检查结果"
  call conform(snapshot)
  ensure size(result.diagnostics) == 0
  ensure size(declaredIds) > 0
  ensure size(result.statuses) == size(declaredIds)
  ensure all(id in declaredIds: id in result.statuses and result.statuses[id] == graph.Status.verified)
}

property rejects-broken-graph invariant "合乎约束的图做一处违反某条约束的改动之后，检查给出这条约束的诊断，诊断的 subject 是改动的节点或边，改动涉及的声明项的状态不再是 verified" {
  about conform
  forall broken: Broken generated "一张随机的合乎约束的图，在随机的一处违反了 design.md §5.3 中的某条约束"
  manual "需要把改动过的片段加入图并推导之后再检查，表达式不能构造图"
}

property stubs-stay-incomplete invariant "性质或流程绑定的谓词或生成器是骨架时，这条性质或这个流程的状态是 stub，其余声明项仍是 verified，它不再是骨架时全部为 verified" {
  about conform
  forall sample: Sample generated "与 accepts-conforming-graph 相同的随机图，其中每个函数的 stub 属性都为假"
  manual "需要逐个把谓词和生成器的 stub 属性改为真再改回，每次都推导和检查，表达式不能改动图"
}

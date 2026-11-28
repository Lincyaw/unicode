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

type Member record "设计类型的一个成员与实现该类型的代码类型的比较（model.md §10 第 18 条）：种类、名称、设计中和代码中的类型、两边各有没有它、类型中引用的设计类型，以及比较的状态" {
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

op members(g: graph.Graph, ids: list<string>) -> map<string, list<Member>> "ids 中每个设计类型的成员：设计中写出的字段按书写次序，枚举的值另加只在代码中出现的值；类型恰被一个代码类型实现时，各成员带比较的状态"

property accepts-conforming-graph postcondition "合乎约束的图推导、检查之后没有诊断，每个声明项和依赖恰有一个 verified 状态" {
  about conform
  forall sample: Sample generated "一张随机的、合乎 design.md §5.3 全部约束、尚未推导的图：二至四个组件（后两个可以嵌套），各有对象类型及其操作（约三分之一的操作声明为纯，代码都没有直接效果），有时另有记录、事件、错误、枚举或接口及实现接口的类型，各有一条带具名观察（有时带生成器）的性质和一个含调用、发送、发布和分支步骤的流程，以及实现它们的 Go 代码、生成文件中的测试和一个 Python 测试文件，第二个组件另有发布事件的函数和方法，有时带外部系统和类型之间的实现接口与继承"
  forall snapshot: graph.Graph generated "把 sample.fragment 加入一张新图并推导"
  forall declaredIds: list<string> generated "snapshot 中全部声明项和 depends 边的标识，不经 conform 取得"
  call conform(snapshot)
  ensure size(result.diagnostics) == 0
  ensure size(declaredIds) > 0
  ensure size(result.statuses) == size(declaredIds)
  ensure all(id in declaredIds: id in result.statuses and result.statuses[id] == graph.Status.verified)
}

property rejects-broken-graph invariant "合乎约束的图做一处违反某条约束的改动之后，检查给出这条约束的、subject 为改动所在的节点或边的诊断，每条诊断都有 check. 开头的诊断码、subject、原因、修正和位置，这个 subject 有状态时不是 verified" {
  about conform
  forall broken: Broken generated "与 accepts-conforming-graph 相同的随机图，在随机的一处做 44 种改动之一，各种改动机会相等，合起来涵盖 check 除运行证据之外的每个诊断码"
  manual "需要判断每条诊断的码以 check. 开头，表达式没有字符串的前缀判断"
}

property stubs-stay-incomplete invariant "性质或流程绑定的谓词或生成器是骨架时，检查没有诊断，这条性质或这个流程的状态是 stub、其余声明项仍是 verified，谓词或生成器不是骨架时检查没有诊断、全部声明项都是 verified" {
  about conform
  forall sample: Sample generated "与 accepts-conforming-graph 相同的随机图，其中每个函数的 stub 属性都为假"
  manual "需要逐个把谓词和生成器的 stub 属性改为真再改回，每次都推导和检查，表达式不能改动图"
}

property reports-effects-in-pure-ops postcondition "conform 报出 check.effect-in-pure-op 的 subject 恰是声明为纯、而 effects 给出非空效果的设计层操作，这些操作的状态是 contradicted" {
  about conform
  forall snapshot: graph.Graph generated "与 accepts-conforming-graph 相同的随机图，每个具体函数和方法约四分之一带随机的非空直接效果，推导过"
  call conform(snapshot)
  ensure all(id in snapshot.effects(): "pure" in snapshot.node(id).attrs and size(snapshot.effects()[id]) > 0 implies any(d in result.diagnostics: d.code == "check.effect-in-pure-op" and d.subject == id) and result.statuses[id] == graph.Status.contradicted)
  ensure all(d in result.diagnostics: d.code == "check.effect-in-pure-op" implies d.subject in snapshot.effects() and size(snapshot.effects()[d.subject]) > 0 and "pure" in snapshot.node(d.subject).attrs)
}

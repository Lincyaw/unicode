component evidence "按覆盖挑选和缩减用例，缩小反例，录制用例，并把运行结果作为证据并入图" {
  focus core
  paths "internal/evidence/**"
  uses graph, lang, syntax, diag
}

type Reduction record "reduce 的结果：缩减后留下的用例，它们保持原有覆盖的并集、全部声明的用例和反例，以及诊断" {
  field cases: list<lang.CaseResult>
  field diagnostics: list<diag.Diagnostic>
}

type Shrunk record "shrink 的结果：缩小后仍能复现的反例，以及缩小过程中的检查错误；预算耗尽时是已知最小的反例" {
  field result: lang.CaseResult
  field diagnostics: list<diag.Diagnostic>
}

type Loaded record "read 的结果：.unicode/runs 中不可变的运行记录和诊断；没有执行输入哈希的旧结果不作为当前证据" {
  field reports: list<lang.RunReport>
  field diagnostics: list<diag.Diagnostic>
}

type Attached record "attach 的结果：证据层的节点，它们与性质、流程和用例之间的边，当前有效的 covers 边，以及诊断" {
  field fragment: graph.Fragment
  field diagnostics: list<diag.Diagnostic>
}

type Recording record "recordings 的结果：要写的录制文件、要删除的录制文件和诊断，其中不改写声明的用例" {
  field edits: list<lang.Edit>
  field removals: list<string>
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "evidence 各性质的输入：一组用例的结果及其覆盖、录制用例的上限，以及必须留下的用例标识" {
  field cases: list<lang.CaseResult>
  field limit: int
  field required: list<string>
}

type AttachmentSample record "attachment-preserves-input-identity 的输入：一张图、一组运行报告，以及应判为当前的用例数、当前的用例和以反例失败的性质或流程" {
  field graph: graph.Graph
  field reports: list<lang.RunReport>
  field freshCount: int
  field currentCases: list<string>
  field failedChecks: list<string>
}

op reduce(cases: list<lang.CaseResult>, limit: int) -> Reduction pure "保留全部声明的用例和反例，其余按新增覆盖最多、同分时按输入哈希的次序贪心挑选，再删去冗余的用例；录制用例超过 limit 而不能保持覆盖时给出诊断"

op shrink(initial: lang.CaseResult, runner: lang.CandidateRunner, budget: duration) -> Shrunk "在 budget 内反复把 initial 缩小成仍合乎类型和前提、仍以反例而不是检查错误失败的更小输入，直到不能再缩小，返回最后的反例，不含中途的尝试"

op read(root: string) -> Loaded "读取 .unicode/runs 下的不可变运行记录和兼容的 results.json，损坏的文件和符号链接是错误，已失效的结果保留为历史"

op save(root: string, report: lang.RunReport) -> list<diag.Diagnostic> "把 report 保存成不可变的运行记录，并写出最新的 results.json，同一份记录再保存不改变文件，写入失败时给出诊断"

op attach(g: graph.Graph, reports: list<lang.RunReport>) -> Attached pure "按执行输入哈希判定哪些结果是当前证据，为它们生成 run 和 result 节点及 about、contains 和 covers 边；旧结果的行号不套用到新的源码上"

op recordings(root: string, g: graph.Graph, cases: list<lang.CaseResult>) -> Recording "按规范格式给出 corpus 下要写的录制文件，用例名是输入哈希的前八位；名称碰撞、输入不能回放或超过上限时不写入"

op applyRecordings(root: string, recording: Recording) -> list<diag.Diagnostic> "先检查 recording 中全部要写和要删的文件，再逐个文件安全写入，只写规范的录制文件，不跟随符号链接，不改写声明的用例，多个文件之间不保证同时成功"

property reduction-preserves-evidence postcondition "reduce 没有诊断时，留下的用例含全部声明的用例和反例，覆盖原有的全部覆盖点，录制用例不超过 limit" {
  about reduce
  forall sample: Sample generated "声明的用例、反例和覆盖用例的组合，覆盖点互相重叠，上限有时不足以保持覆盖"
  call reduce(sample.cases, sample.limit)
  ensure size(result.diagnostics) == 0 implies all(original in sample.cases: original.form == "declared" or original.reason == "counterexample" implies any(retained in result.cases: retained.id == original.id))
  ensure size(result.diagnostics) == 0 implies all(original in sample.cases: all(point in original.coverage: any(retained in result.cases: any(covered in retained.coverage: point == covered))))
  ensure size(result.diagnostics) == 0 implies count(retained in result.cases: retained.form != "declared") <= sample.limit
}

property attachment-preserves-input-identity postcondition "attach 只把输入和执行输入哈希都与当前相符的用例判为当前证据，只把以反例失败的性质或流程标为失败（只有检查错误的不算），covers 边只从当前的用例出发" {
  about attach
  forall sample: AttachmentSample generated "十种情形之一：当前通过的报告，执行输入哈希已过期的报告，改了名而输入相同的用例，录制的反例，观察失败，反例和观察失败并存，没有录制的反例，超出 within，表达式求值错误，以及没有有效执行；有时另有一份当前通过的报告"
  call attach(sample.graph, sample.reports)
  ensure size(result.diagnostics) == 0
  ensure count(node in result.fragment.nodes: node.form == "case" and "true" in node.attrs["fresh"]) == sample.freshCount
  ensure count(node in result.fragment.nodes: node.form == "check" and "true" in node.attrs["fresh"] and "true" in node.attrs["businessFailed"]) == size(sample.failedChecks)
  ensure all(id in sample.failedChecks: any(node in result.fragment.nodes: node.form == "check" and id in node.attrs["check"] and "true" in node.attrs["fresh"] and "true" in node.attrs["businessFailed"]))
  ensure all(edge in result.fragment.edges: edge.kind == graph.EdgeKind.covers implies edge.from in sample.currentCases)
}

property recordings-are-replayable invariant "录制的用例回放时得到录制时的同一输入：模型值在调用前冻结，编码往返不丢状态，录制文件重新解析后给出同一输入，失败或损坏时不写出空结果" {
  about recordings, applyRecordings, shrink, save, read, attach
  forall sample: Sample generated "含整数、集合、记录、经 codec 编码的对象和替身脚本的输入，其中有的输入有错，以及当前的和陈旧的结果"
  manual "需要在临时目录中写出并读回录制文件和运行记录，表达式不能读写文件"
}

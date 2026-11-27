component evidence "覆盖选择、候选缩减、模型用例录制和运行证据；不调度 agent" {
  focus core
  paths "internal/evidence/**"
  uses graph, lang, syntax, diag
}

type Reduction record "保持覆盖并集、声明用例和全部反例的确定性缩减结果" {
  field cases: list<lang.CaseResult>
  field diagnostics: list<diag.Diagnostic>
}

type Shrunk record "最终仍能复现的反例及缩减边界的检查错误；预算耗尽保留已知反例" {
  field result: lang.CaseResult
  field diagnostics: list<diag.Diagnostic>
}

type Loaded record "本地不可变运行记录；旧版无输入摘要的结果不作为当前证据" {
  field reports: list<lang.RunReport>
  field diagnostics: list<diag.Diagnostic>
}

type Attached record "证据层节点、与检查和用例的关系，以及当前有效的 covers 边" {
  field fragment: graph.Fragment
  field diagnostics: list<diag.Diagnostic>
}

type Recording record "录制文件的完整生成计划；不会改写声明用例" {
  field edits: list<lang.Edit>
  field removals: list<string>
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "独立给出用例和覆盖集合，包含声明、反例、重复覆盖及超过上限的变体" {
  field cases: list<lang.CaseResult>
  field limit: int
  field required: list<string>
}

type AttachmentSample record "当前图、真实报告及独立标记的有效输入和业务反例；包含旧摘要、环境错误和同输入别名" {
  field graph: graph.Graph
  field reports: list<lang.RunReport>
  field freshCount: int
  field currentCases: list<string>
  field failedChecks: list<string>
}

op reduce(cases: list<lang.CaseResult>, limit: int) -> Reduction pure "保留全部声明与反例，按新增覆盖最多、输入摘要破同分贪心挑选，再删除冗余；超过上限而不能保持保证时给出诊断"

op shrink(initial: lang.CaseResult, runner: lang.CandidateRunner, budget: duration) -> Shrunk "只接受仍失败且不是环境错误的更小模型值；保留前提与类型，预算内缩到候选缩减的固定点；返回最终反例，不混入缩减尝试"

op read(root: string) -> Loaded "读取 .unicode/runs 下的不可变运行记录和兼容的 results.json；损坏与符号链接是错误，失效结果保留为历史"

op save(root: string, report: lang.RunReport) -> list<diag.Diagnostic> "保存不可变运行记录和最新 results.json；相同记录幂等，定位报告写入错误"

op attach(g: graph.Graph, reports: list<lang.RunReport>) -> Attached pure "按执行输入摘要判定当前证据；生成 run/result、about、contains 和 covers，不把旧结果的行号套到新源码"

op recordings(root: string, g: graph.Graph, cases: list<lang.CaseResult>) -> Recording "以规范格式规划 corpus 文件；录制名是输入摘要前八位，碰撞、不可回放输入或上限冲突均禁止写入"

op applyRecordings(root: string, recording: Recording) -> list<diag.Diagnostic> "先预检全部写入与移除项，只发布本工具拥有的规范 corpus 文件，不跟随符号链接或改写声明用例；每文件安全写入，不承诺跨文件事务"

property reduction-preserves-evidence postcondition "缩减保留声明与反例及全部覆盖，不因重复用例或顺序丢失证据" {
  about reduce
  forall sample: Sample generated "独立覆盖集合与声明、反例、覆盖用例组合；上限有时不足"
  call reduce(sample.cases, sample.limit)
  ensure size(result.diagnostics) == 0 implies all(original in sample.cases: original.form == "declared" or original.reason == "counterexample" implies any(retained in result.cases: retained.id == original.id))
  ensure size(result.diagnostics) == 0 implies all(original in sample.cases: all(point in original.coverage: any(retained in result.cases: any(covered in retained.coverage: point == covered))))
  ensure size(result.diagnostics) == 0 implies count(retained in result.cases: retained.form != "declared") <= sample.limit
}

property attachment-preserves-input-identity postcondition "有效性取自实际执行输入与当前摘要；环境错误不伪装为业务失败，覆盖只连接当前同输入用例" {
  about attach
  forall sample: AttachmentSample generated "当前与陈旧报告、重命名的同输入用例、未录制业务反例，以及只有环境错误但曾有有效执行的报告"
  call attach(sample.graph, sample.reports)
  ensure size(result.diagnostics) == 0
  ensure count(node in result.fragment.nodes: node.form == "case" and "true" in node.attrs["fresh"]) == sample.freshCount
  ensure count(node in result.fragment.nodes: node.form == "check" and "true" in node.attrs["fresh"] and "true" in node.attrs["businessFailed"]) == size(sample.failedChecks)
  ensure all(id in sample.failedChecks: any(node in result.fragment.nodes: node.form == "check" and id in node.attrs["check"] and "true" in node.attrs["fresh"] and "true" in node.attrs["businessFailed"]))
  ensure all(edge in result.fragment.edges: edge.kind == graph.EdgeKind.covers implies edge.from in sample.currentCases)
}

property recordings-are-replayable invariant "录制前冻结模型值，编码往返保持完整状态；规范文件可重新解析并生成同一输入；失败和损坏不写空结果" {
  about recordings, applyRecordings, shrink, save, read, attach
  forall sample: Sample generated "整数、集合、记录、codec、替身脚本和错误输入，以及当前与陈旧结果"
  manual "准备模型编解码和真实文件边界，逐字节核对独立输入、失败复现与证据图，不用最终布尔值代替原始数据"
}

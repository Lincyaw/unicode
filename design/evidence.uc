component evidence "按覆盖挑选和缩减用例，缩小反例，录制用例，并把运行结果作为证据并入图" {
  focus core
  paths "internal/evidence/**"
  uses graph, lang, syntax, diag, monitor
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

type Sample record "reduction-preserves-evidence 和 recordings-are-replayable 的输入：一组用例的结果及其覆盖、录制用例的上限，以及必须保持的覆盖点" {
  field cases: list<lang.CaseResult>
  field limit: int
  field required: list<string>
}

type InvalidTraffic error "采集的调用的文件不是 monitor 写出的 JSON：不是 JSON，字段不符，或者值不合规范编码"

type Traffic record "readTraffic 的结果：.unicode/traffic 中采集的调用，以及不合规范编码、指向不存在的操作或不合该操作签名的文件的诊断" {
  field collected: list<monitor.Collected>
  field diagnostics: list<diag.Diagnostic>
}

type TrafficSample record "traffic-gives-inputs 的输入：一张图、一组采集的调用，以及按 language.md E29 直接算出的每条在运行时判定的单次调用契约的输入" {
  field graph: graph.Graph
  field collected: list<monitor.Collected>
  field expected: map<string, list<lang.Input>>
}

type VenueSample record "runtime-evidence-keeps-its-venue 的输入：一张图，以及一组执行场合为测试或运行时的运行报告" {
  field graph: graph.Graph
  field reports: list<lang.RunReport>
}

type AttachmentSample record "attachment-preserves-input-identity 的输入：一张图、一组运行报告，以及应判为当前的用例数、当前的用例和以反例失败的性质或流程" {
  field graph: graph.Graph
  field reports: list<lang.RunReport>
  field freshCount: int
  field currentCases: list<string>
  field failedChecks: list<string>
}

type DurationSample record "mean-durations-follow-current-evidence 的输入：一张图、一组运行报告，以及按 model.md §3.5 的定义从报告直接算出的每个设计层操作的平均耗时（纳秒）" {
  field graph: graph.Graph
  field reports: list<lang.RunReport>
  field expected: map<string, int>
}

op reduce(cases: list<lang.CaseResult>, limit: int) -> Reduction pure "保留全部声明的用例和反例，其余按新增覆盖最多、同分时按输入哈希的次序贪心挑选，再删去冗余的用例；录制用例超过 limit 而不能保持覆盖时给出诊断"

op shrink(initial: lang.CaseResult, runner: lang.CandidateRunner, budget: duration) -> Shrunk "在 budget 内反复把 initial 缩小成仍合乎类型和前提、仍以反例而不是检查错误失败的更小输入，直到不能再缩小，返回最后的反例，不含中途的尝试"

op read(root: string) -> Loaded "读取 .unicode/runs 下的不可变运行记录和兼容的 results.json，损坏的文件和符号链接是错误，已失效的结果保留为历史"

op save(root: string, report: lang.RunReport) -> list<diag.Diagnostic> "把 report 保存成不可变的运行记录，并写出最新的 results.json，同一份记录再保存不改变文件，写入失败时给出诊断"

op attach(g: graph.Graph, reports: list<lang.RunReport>) -> Attached pure "按执行输入哈希判定哪些结果是当前证据，为它们生成 run 和 result 节点及 about、contains 和 covers 边；旧结果的行号不套用到新的源码上"

op meanDurations(g: graph.Graph) -> map<string, int> pure "g 的证据层中当前证据记下的设计层操作的平均耗时（纳秒，向零取整）：每条性质和流程取它最近一次的当前运行，按操作平均其中用例记下的调用的耗时，没有这样的调用的操作不在其中"

op recordings(root: string, g: graph.Graph, cases: list<lang.CaseResult>) -> Recording "按规范格式给出 corpus 下要写的录制文件，用例名是输入哈希的前八位；名称碰撞、输入不能回放或超过上限时不写入"

op applyRecordings(root: string, recording: Recording) -> list<diag.Diagnostic> "先检查 recording 中全部要写和要删的文件，再逐个文件安全写入，只写规范的录制文件，不跟随符号链接，不改写声明的用例，多个文件之间不保证同时成功"

op decodeCollected(data: bytes) -> monitor.Collected pure raises InvalidTraffic "monitor.encodeCollected 的逆：从 traffic/ 中的一个文件读出采集的调用"

op readTraffic(root: string, g: graph.Graph) -> Traffic "读取 root 下 .unicode/traffic/<操作的标识>/<输入哈希>.json 中采集的调用，按路径排序；目录不存在时没有采集的调用，文件不合规范编码、指向 g 中不存在的操作、实参与该操作的参数不符或是符号链接时给出 evidence.invalid-traffic"

op trafficInputs(g: graph.Graph, collected: list<monitor.Collected>) -> map<string, list<lang.Input>> pure "按 language.md E29 把采集的调用对应成 g 中在运行时判定的单次调用契约（lang.monitoredCalls）的输入：每个采集的调用对其操作上的每条契约给出一个输入，接收者的值给出接收者的符号，调用值的各实参给出同一位置的实参的符号"

property reduction-preserves-evidence postcondition "reduce 没有诊断时，留下的用例含全部声明的用例和反例，覆盖原有的全部覆盖点，录制用例不超过 limit" {
  about reduce
  forall sample: Sample generated "一至八个用例，约一半是声明的用例，约四分之一是反例，其余是覆盖用例，约一半重复出现一次，共一至六个覆盖点，每个用例各以一半机会覆盖其中每一个，上限 0 至 9，有时不足以保持覆盖，输入是十四种模型值之一"
  call reduce(sample.cases, sample.limit)
  ensure size(result.diagnostics) == 0 implies all(original in sample.cases: original.form == "declared" or original.reason == "counterexample" implies any(retained in result.cases: retained.id == original.id))
  ensure size(result.diagnostics) == 0 implies all(original in sample.cases: all(point in original.coverage: any(retained in result.cases: any(covered in retained.coverage: point == covered))))
  ensure size(result.diagnostics) == 0 implies count(retained in result.cases: retained.form != "declared") <= sample.limit
}

property attachment-preserves-input-identity postcondition "attach 没有诊断，标为当前的用例结果数恰等于执行输入哈希与当前相符的报告中的用例数，标为失败的当前性质或流程恰是有反例或超出 within 的那些（只有检查错误的不算），covers 边只从输入与当前结果相同的用例出发" {
  about attach
  forall sample: AttachmentSample generated "整数输入 -50 至 50，以及十种情形之一：当前通过的报告，执行输入哈希已过期的失败报告，改了名而输入相同的失败用例，图中没有对应用例的录制反例，观察失败，反例和观察失败并存，输入与声明的用例不同的反例，超出 within，报告中的表达式求值错误，以及没有有效执行；约一半另有一份执行输入哈希与前一份相同、用例通过的报告"
  call attach(sample.graph, sample.reports)
  ensure size(result.diagnostics) == 0
  ensure count(node in result.fragment.nodes: node.form == "case" and "true" in node.attrs["fresh"]) == sample.freshCount
  ensure count(node in result.fragment.nodes: node.form == "check" and "true" in node.attrs["fresh"] and "true" in node.attrs["businessFailed"]) == size(sample.failedChecks)
  ensure all(id in sample.failedChecks: any(node in result.fragment.nodes: node.form == "check" and id in node.attrs["check"] and "true" in node.attrs["fresh"] and "true" in node.attrs["businessFailed"]))
  ensure all(edge in result.fragment.edges: edge.kind == graph.EdgeKind.covers implies edge.from in sample.currentCases)
}

property mean-durations-follow-current-evidence postcondition "在 sample.graph 加上 attach 为 sample.reports 给出的证据层的图上，meanDurations 给出的平均耗时与 sample.expected 相同" {
  about meanDurations, attach
  forall sample: DurationSample generated "一张有操作 domain.a、b、c 和性质 domain.p0、p1 的图，一至四份时间各不相同（有的只差零点五秒）、约四分之三的执行输入哈希与图相符的运行报告，每份各有约四分之三的机会带两条性质的结论（没有 domain.p0 的结论时总带 domain.p1 的），每条结论零至三个通过的用例，各耗时 1 至 1000000 纳秒，调用零至三次 domain.a、b、c 或不是操作的 domain.gone，每次的耗时不超过用例的耗时，以及按每条性质最近一次相符的报告中的调用、逐个操作求出的向零取整的平均耗时"
  forall snapshot: graph.Graph generated "sample.graph 加上 attach(sample.graph, sample.reports) 给出的节点和边"
  call meanDurations(snapshot)
  ensure result == sample.expected
}

property recordings-are-replayable invariant "录制文件重新解析后给出录制时的同一输入，再次录制不改动文件，不能写成录制文件的输入、越界路径、损坏的录制文件和符号链接给出诊断而不改动文件，孤立的录制文件被删除，运行记录保存后读出不变且不改写历史，缩小把反例缩到最小的失败输入，缩小中断时保留原反例" {
  about recordings, applyRecordings, shrink, save, read, attach
  forall sample: Sample generated "一至八个用例，同一样本的输入都取十四种模型值中的同一种：整数、列表、集合、记录、经 codec 编码的记录、映射、调用值、应答脚本、字节串、时间、时长、浮点数、枚举或含接口替身的记录，其中经 codec 编码的记录和映射含非 UTF-8 的字符串，字节串含控制字节，三者都不能直接写成录制文件；用例约一半是声明的，约四分之一是反例，上限 0 至 9"
  manual "需要在临时目录中写出并读回录制文件和运行记录，表达式不能读写文件"
}

property collected-calls-round-trip invariant "decodeCollected 读回 monitor.encodeCollected 写出的采集的调用，得到原来的 collected" {
  about decodeCollected, monitor.encodeCollected
  forall collected: monitor.Collected generated "操作的标识是一至三段以点分隔的小写名称，约一半带接收者，接收者和调用值是至多三层的规范编码的模型值（种类取自 int、string、bytes、bool、list、set、map、record、none 和 redacted），提交是空串或四十位十六进制，时间是整秒的 UTC 时间"
  ensure decodeCollected(monitor.encodeCollected(collected)) == collected
}

property traffic-gives-inputs postcondition "trafficInputs 给出的输入与 sample.expected 相同：每个采集的调用对其操作上每条在运行时判定的单次调用契约恰给出一个输入，接收者的值给出接收者的符号，调用值的各实参给出同一位置的实参的符号，不对应任何契约的采集的调用不给出输入" {
  about trafficInputs, lang.monitoredCalls
  forall sample: TrafficSample generated "一张有类型 domain.Box（操作 put(key: string, count: int)）、容器直接提供的操作 domain.scale(factor: int) 和 domain.other() 的图，其上零至三条以 put 或 scale 为被测调用的契约（实参都是符号）、一条实参是字段访问的契约和一条人工性质，零至六个采集的调用（操作取自 domain.Box.put、domain.scale 和 domain.other），以及逐个契约按参数的位置算出的输入"
  call trafficInputs(sample.graph, sample.collected)
  ensure result == sample.expected
}

property runtime-evidence-keeps-its-venue postcondition "attach 给每个运行节点和结论节点记下恰一个执行场合，取自报告的 venue，没有时为 test，运行时的结论节点另列出报告中不成立的每条限定或前提和判定过的每个子句，执行场合为运行时的结论节点是当前证据当且仅当它的执行输入哈希与图的相同且它的性质或流程在图中" {
  about attach
  forall sample: VenueSample generated "attachment-preserves-input-identity 那样的图，以及一至三份报告，执行场合为 test、runtime 或缺省，约一半的执行输入哈希与图的相同，运行时的结论带一至两条不成立的限定或前提、一至两个子句的判定次数、0 至 1 秒的第 95 百分位耗时和零至一个反例"
  call attach(sample.graph, sample.reports)
  ensure all(n in result.fragment.nodes: n.kind == graph.NodeKind.run or n.kind == graph.NodeKind.result implies size(n.attrs["venue"]) == 1 and ("test" in n.attrs["venue"] or "runtime" in n.attrs["venue"]))
  ensure all(r in sample.reports: all(x in r.results: any(n in result.fragment.nodes: n.form == "check" and x.id in n.attrs["check"] and (r.venue == "runtime") == ("runtime" in n.attrs["venue"]))))
  ensure all(r in sample.reports: r.venue == "runtime" implies all(x in r.results: all(o in x.outsideDomain: any(n in result.fragment.nodes: n.form == "check" and x.id in n.attrs["check"] and "runtime" in n.attrs["venue"] and o.clause in n.attrs["outsideDomain"]))))
  ensure all(r in sample.reports: r.venue == "runtime" implies all(x in r.results: all(c in x.clauses: any(n in result.fragment.nodes: n.form == "check" and x.id in n.attrs["check"] and "runtime" in n.attrs["venue"] and c.clause in n.attrs["clauses"]))))
  ensure all(n in result.fragment.nodes: n.form == "check" and "runtime" in n.attrs["venue"] implies ("true" in n.attrs["fresh"]) == (graph.inputDigest(sample.graph) in n.attrs["snapshot"] and any(m in sample.graph.nodes(): m.id in n.attrs["check"] and (m.kind == graph.NodeKind.property or m.kind == graph.NodeKind.flow))))
}

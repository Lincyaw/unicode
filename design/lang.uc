component lang "语言后端的接口和名称映射，以及后端生成和运行的输入、输出类型" {
  focus support
  paths "internal/lang/**"
  uses graph, diag, monitor
}

type NameRole enum "名称映射作用的名称的种类（design.md §4）" {
  value type
  value op
  value field
  value enumValue
  value predicate
  value flowPredicate
}

type ExtractFailed error "代码无法抽取：源码根目录不存在，或者这种语言的工具链不可用"

type Extracted record "一次抽取的结果：代码层的节点和边，以及绑定的写法不合格式之处" {
  field fragment: graph.Fragment
  field diagnostics: list<diag.Diagnostic>
}

type Backend interface "一种语言的后端：静态抽取代码层，给出名称映射" {
  op language() -> string "语言的名称，写法与系统文件中的 language 相同"
  op extract(root: string, srcRoots: list<string>) -> Extracted raises ExtractFailed "抽取源码根目录下的代码层：节点及其属性（包括每个函数的直接效果）、边，以及方法的候选边（design.md §4）"
  op mapName(role: NameRole, designName: string) -> string "设计中的名称在这种语言中的写法"
}

type Edit record "生成计划中的一个文件：相对仓库根目录的路径、内容，以及是否只在文件不存在时创建（createOnly，骨架为真）" {
  field path: string
  field content: bytes
  field createOnly: bool
}

type Generation record "一种语言的生成计划：要写的文件和诊断；有诊断时 gen 不写入其中任何文件" {
  field edits: list<Edit>
  field diagnostics: list<diag.Diagnostic>
}

type GenPlan record "一种语言的生成输入：仓库根目录、源码根目录和已链接的图" {
  field root: string
  field roots: list<string>
  field graph: graph.Graph
}

type RunPlan record "一种语言的运行输入：仓库根目录、源码根目录、图、要运行的性质和流程（ids 为空时全部）、探索时长 explore 和探索的候选输入数 explorations（两者都为零时只回放）、是否写回录制文件 record，以及 unicode test --traffic 时按性质或流程给出的采集的调用的输入 traffic" {
  field root: string
  field roots: list<string>
  field graph: graph.Graph
  field ids: list<string>
  field explore: duration
  field explorations: int
  field record: bool
  field traffic: map<string, list<monitor.Input>>
}

type Coverage record "一个覆盖点：代码块在源码中的位置，或者流程中走到的步骤、分支，或者抛出的声明的错误" {
  field kind: string
  field node: string
  field path: string
  field line: int
  field column: int
  field endLine: int
  field endColumn: int
  field label: string
}

type TraceEvent record "由 Go 运行轨迹得到的函数进入、退出、发布与错误，保留真实 goroutine 和创建关系" {
  field kind: string
  field node: string
  field goroutine: int
  field parent: int
  field time: int
  field detail: string
}

type CallTime record "用例中对一个设计层操作的一次调用：被测调用或调用序列中的一次调用，以及它的耗时（纳秒）" {
  field op: string
  field duration: int
}

type CaseResult record "一个输入的最终结果：所属的性质或流程、用例的形式和录制原因、冻结的输入及其哈希、是否通过或被拒绝、覆盖、轨迹、观察、范围外的差异、诊断、执行耗时（纳秒）、其中每次调用的操作和耗时，以及不成立的子句、限定或前提 clause 和运行时的反例的时间 at；缩小反例时的尝试不算作结果" {
  field id: string
  field check: string
  field form: string
  field reason: string
  field input: monitor.Input
  field inputHash: string
  field passed: bool
  field rejected: bool
  field coverage: list<Coverage>
  field trace: list<TraceEvent>
  field observations: list<string>
  field differences: list<string>
  field diagnostics: list<diag.Diagnostic>
  field duration: int
  field calls: list<CallTime>
  field clause: string
  field at: string
}

type CandidateEvaluation record "一个候选输入的执行结果：预算耗尽时没有结果，检查错误列在 diagnostics 中，与反例分开" {
  field result: optional<CaseResult>
  field exhausted: bool
  field diagnostics: list<diag.Diagnostic>
}

type CandidateRunner interface "编译好的驱动，每次重新构造输入，在单独的进程中执行一个候选输入" {
  op evaluate(input: monitor.Input) -> CandidateEvaluation "执行 input 这一个候选输入，返回它自己的覆盖和轨迹，不含此前的候选累计的覆盖"
}

type RunResult record "一条性质或一个流程的运行结论：是否通过、有效执行数（运行时为适用的调用数）、拒绝的候选输入数、观察范围、范围外的差异、ensure 不成立或有范围外的差异时的诊断、各用例的结果、代码、分支和错误覆盖点的总数，以及运行时的域外调用数、各子句的判定次数、第 95 百分位耗时（纳秒）和核对的轨迹数" {
  field id: string
  field passed: bool
  field accepted: int
  field rejected: int
  field observations: list<string>
  field differences: list<string>
  field diagnostics: list<diag.Diagnostic>
  field cases: list<CaseResult>
  field codeTotal: int
  field branchTotal: int
  field errorTotal: int
  field outsideDomain: list<monitor.ClauseCalls>
  field clauses: list<monitor.ClauseJudged>
  field p95: int
  field traces: int
}

type RunReport record "一次运行的报告：各性质和流程的结论，不属于任何一条结论的诊断（工具失败和检查错误），以及格式版本、执行输入哈希、提交、工作区是否有改动、时间、运行方式和执行场合 venue（test 或 runtime），运行时的记录另有记录周期的开始时间 from" {
  field results: list<RunResult>
  field diagnostics: list<diag.Diagnostic>
  field version: int
  field snapshot: string
  field commit: string
  field dirty: bool
  field at: string
  field mode: string
  field venue: string
  field from: string
}

type Monitored record "一条在运行时判定的单次调用契约（model.md §3.4）的被测调用：被调用的设计操作、接收者的符号（容器直接提供的操作为空串），以及按参数次序的各实参的符号" {
  field op: string
  field receiver: string
  field args: list<string>
}

type Generator interface "生成能力：后端可以另外实现的接口，不实现它的后端仍是完整的 Backend" {
  op language() -> string "系统文件中的语言名称"
  op generate(plan: GenPlan) -> Generation "给出这种语言的生成计划：骨架和生成文件；不写仓库，不支持的目标给出诊断"
}

type GenerationExtractor interface "计算生成计划时可用的抽取能力：把旧生成测试读成只有包声明的文件，它们中的类型错误（例如接口改名之后）因此不妨碍重新生成" {
  op language() -> string "系统文件中的语言名称"
  op extractForGeneration(root: string, srcRoots: list<string>) -> Extracted raises ExtractFailed "像 extract 一样抽取，但在内存中把带生成标记的测试文件读成只有包声明的文件，不改动磁盘；手写源码的错误照常报出"
}

type Runner interface "运行能力：后端可以另外实现的接口，不实现它的后端仍是完整的 Backend" {
  op language() -> string "系统文件中的语言名称"
  op run(plan: RunPlan) -> RunReport "核对生成文件后回放，或在 explore 时长内探索，返回每条性质和流程的结论"
}

type RootsSample record "roots-follow-system-file 的输入：一张图，以及写进其中系统节点的 roots 属性的各语言的源码根目录" {
  field graph: graph.Graph
  field roots: map<string, list<string>>
}

op monitoredCalls(g: graph.Graph) -> map<string, Monitored> pure "g 中在运行时判定的每条单次调用契约（model.md §3.4）的标识到它的被测调用：带 call 子句、不是人工性质的 postcondition，被测调用的接收者和各实参是互不相同的符号，性质的每个符号都是其中之一，被调用的操作经接收者的类型及其 extends 的祖先或容器解析得到"

op languageRoots(g: graph.Graph) -> map<string, list<string>> pure "系统节点的 roots 属性中每项写作“语言: 目录”，给出各语言的源码根目录，同一语言的目录按书写的次序"

property roots-follow-system-file postcondition "languageRoots 恰给出写进系统节点的各语言的源码根目录，同一语言的目录按写入的次序" {
  about languageRoots
  forall sample: RootsSample generated "go、python、rust 各以一半机会有一至三个源码根目录，目录是一至三个小写字母或 . 与 / 组成的串，可以重复，各语言的项交错写进系统节点的 roots 属性；图中另有一个也带 roots 属性的组件节点，它的项不算"
  call languageRoots(sample.graph)
  ensure result == sample.roots
}

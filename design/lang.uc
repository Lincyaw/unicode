component lang "语言后端的接口和名称映射，以及后端生成和运行的输入、输出类型" {
  focus support
  paths "internal/lang/**"
  uses graph, diag
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
  op extract(root: string, srcRoots: list<string>) -> Extracted raises ExtractFailed "抽取源码根目录下的代码层：节点、边，以及方法的候选边（design.md §4）"
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

type RunPlan record "一种语言的运行输入：仓库根目录、源码根目录、图、要运行的性质和流程（ids 为空时全部）和探索时长（explore 为零时只回放）" {
  field root: string
  field roots: list<string>
  field graph: graph.Graph
  field ids: list<string>
  field explore: duration
  field record: bool
}

type Value record "可回放模型值；kind 决定 text、items 或 entries，编码值的 items 恰含其声明表示" {
  field kind: string
  field text: string
  field items: list<Value>
  field entries: list<ValueEntry>
}

type ValueEntry record "记录、映射、调用或应答脚本中的键和值" {
  field key: Value
  field value: Value
}

type Input record "调用前冻结的完整输入；对象的私有状态经声明的 codec 保存，替身保存原始脚本" {
  field values: map<string, Value>
}

type Coverage record "一个真实覆盖点：代码块的原始位置，或设计步骤、分支、声明错误" {
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

type CaseResult record "一次输入的最终结果；缩减尝试不累计成额外业务执行，只有真实结果产生覆盖" {
  field id: string
  field check: string
  field form: string
  field reason: string
  field input: Input
  field inputHash: string
  field passed: bool
  field rejected: bool
  field coverage: list<Coverage>
  field trace: list<TraceEvent>
  field observations: list<string>
  field differences: list<string>
  field diagnostics: list<diag.Diagnostic>
}

type CandidateEvaluation record "一次候选请求；预算耗尽没有结果，环境诊断与业务反例分开" {
  field result: optional<CaseResult>
  field exhausted: bool
  field diagnostics: list<diag.Diagnostic>
}

type CandidateRunner interface "已经编译的语言执行边界，每次重建输入，在独立边界执行一个候选" {
  op evaluate(input: Input) -> CandidateEvaluation "真实执行并返回覆盖与轨迹，不能以累计覆盖替代当前候选"
}

type RunResult record "一条性质或一个流程的运行结论：是否通过、有效执行数、拒绝的候选输入数、观察范围、范围外的差异，以及 ensure 不成立或有范围外的差异时的诊断" {
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
}

type RunReport record "一次运行中各性质和流程的结论，以及不属于任何一条结论的诊断：工具失败和检查错误（观察失败、前提求值错误、域外输入、没有有效执行）" {
  field results: list<RunResult>
  field diagnostics: list<diag.Diagnostic>
  field version: int
  field snapshot: string
  field commit: string
  field dirty: bool
  field at: string
  field mode: string
}

type Generator interface "后端可以另外实现的生成能力；不实现它的后端仍是完整的 Backend" {
  op language() -> string "系统文件中的语言名称"
  op generate(plan: GenPlan) -> Generation "给出这种语言的生成计划：骨架和生成文件；不写仓库，不支持的目标给出诊断"
}

type GenerationExtractor interface "计算生成计划时可用的抽取能力：把旧生成测试读成只有包声明的文件，它们中的类型错误（例如接口改名之后）因此不妨碍重新生成" {
  op language() -> string "系统文件中的语言名称"
  op extractForGeneration(root: string, srcRoots: list<string>) -> Extracted raises ExtractFailed "像 extract 一样抽取，但在内存中把带生成标记的测试文件读成只有包声明的文件，不改动磁盘；手写源码的错误照常报出"
}

type Runner interface "后端可选的运行能力" {
  op language() -> string "系统文件中的语言名称"
  op run(plan: RunPlan) -> RunReport "核对生成文件后回放，或在 explore 时长内探索，返回每条性质和流程的结论"
}

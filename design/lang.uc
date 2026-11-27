component lang "语言后端的接口、名称映射、生成计划和运行计划" {
  focus support
  paths "internal/lang/**"
  uses graph, diag
}

type NameRole enum "名称映射作用的对象（design.md §4）" {
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
  op extract(root: string, srcRoots: list<string>) -> Extracted raises ExtractFailed "抽取源码根目录下的代码层：节点、边，以及方法按反向名称映射落实的操作"
  op mapName(role: NameRole, designName: string) -> string "设计中的名称在这种语言中的写法"
}

type Edit record "生成计划中的一个文件，路径相对于仓库根目录" {
  field path: string
  field content: bytes
}

type Generation record "生成的文件及诊断；有诊断时不应用计划" {
  field edits: list<Edit>
  field diagnostics: list<diag.Diagnostic>
}

type GenPlan record "一种语言的生成输入：仓库、源码根目录与已链接的图" {
  field root: string
  field roots: list<string>
  field graph: graph.Graph
}

type RunPlan record "一种语言的运行输入；ids 为空时运行全部可执行性质" {
  field root: string
  field roots: list<string>
  field graph: graph.Graph
  field ids: list<string>
  field explore: duration
}

type RunResult record "一条性质的运行结论：有效执行、拒绝候选、观察边界、范围外差异与业务断言的定位诊断" {
  field id: string
  field passed: bool
  field accepted: int
  field rejected: int
  field observations: list<string>
  field differences: list<string>
  field diagnostics: list<diag.Diagnostic>
}

type RunReport record "一次运行中各性质的结果；顶层诊断是工具、观察、前提、输入域或零有效执行等检查环境错误" {
  field results: list<RunResult>
  field diagnostics: list<diag.Diagnostic>
}

type Generator interface "后端可选的生成能力；不扩充只读 Backend 的必需方法" {
  op language() -> string "系统文件中的语言名称"
  op generate(plan: GenPlan) -> Generation "生成语言文件；不写仓库，未支持的目标给出诊断"
}

type Runner interface "后端可选的运行能力" {
  op language() -> string "系统文件中的语言名称"
  op run(plan: RunPlan) -> RunReport "核对已生成文件，执行回放或预算内探索，返回每条性质的结论"
}

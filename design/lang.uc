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

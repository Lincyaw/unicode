component review "评审记录：本地持久化、按内容判断有效性、列出当前及比较中的待审项" {
  focus core
  paths "internal/review/**"
  uses graph, diag
}

type Target record "恰指向一个节点或一条边；removed 为比较中已经删除的项" {
  field node: string
  field edgeKind: string
  field from: string
  field to: string
  field removed: bool
}

type Record record "不可变的评审记录，id 是除 id 外全部字段规范 JSON 的 SHA-256" {
  field version: int
  field id: string
  field reviewer: string
  field at: string
  field snapshot: string
  field commit: string
  field dirty: bool
  field target: Target
  field digest: string
  field decision: string
  field comment: string
}

type Item record "一个目标当前的内容指纹和评审状态；record 是与当前指纹相符的最新记录" {
  field target: Target
  field digest: string
  field state: string
  field record: optional<Record>
}

type Loaded record "读取评审存储的记录和定位诊断" {
  field records: list<Record>
  field diagnostics: list<diag.Diagnostic>
}

type Evaluation record "按目标身份排序的全部候选及诊断；不把评过等同于批准" {
  field items: list<Item>
  field diagnostics: list<diag.Diagnostic>
}

type Request record "记录评审的输入；expectedDigest 非空时拒绝已经变化的目标" {
  field target: Target
  field reviewer: string
  field decision: string
  field comment: string
  field expectedDigest: string
  field commit: string
  field dirty: bool
}

type Written record "写出的记录或定位诊断；有诊断时没有新记录" {
  field record: optional<Record>
  field diagnostics: list<diag.Diagnostic>
}

type Attached record "评审层节点及其所指目标的 about 边；不改动输入图" {
  field fragment: graph.Fragment
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "独立构造节点、边、修改和记录，并给出完整候选与待审目标" {
  field graph: graph.Graph
  field base: optional<graph.Graph>
  field records: list<Record>
  field targets: list<Target>
  field pending: list<Target>
}

op read(root: string) -> Loaded "读取 .unicode/reviews 下不可变 JSON；缺失为空，损坏、非法版本或符号链接为错误"

op evaluate(g: graph.Graph, base: optional<graph.Graph>, records: list<Record>) -> Evaluation pure "没有基准时列出当前设计和非生成代码的节点及边；有基准时列出新增、修改、删除和因人工谓词或观察适配器变化失效的性质"

op write(root: string, g: graph.Graph, base: optional<graph.Graph>, request: Request) -> Written "校验目标与预期指纹，写入不可变记录；approved 和 changes-requested 都表示该内容已经评过，结论独立保存"

op attach(g: graph.Graph, records: list<Record>) -> Attached pure "生成 review 层节点；节点目标用 about 指向该节点，边目标用属性保存完整身份并指向仍存在的端点"

property evaluation-follows-content postcondition "候选和待审集合与独立构造的内容变化相符；已评项持有相同指纹的有效记录" {
  about evaluate
  forall sample: Sample generated "节点和边的新增、修改、删除、恢复，人工谓词与观察适配器变化，以及普通无关函数变化；独立保留目标与待审集合"
  call evaluate(sample.graph, sample.base, sample.records)
  ensure size(result.diagnostics) == 0
  ensure size(result.items) == size(sample.targets)
  ensure all(target in sample.targets: any(item in result.items: item.target == target))
  ensure count(item in result.items: item.state == "pending") == size(sample.pending)
  ensure all(target in sample.pending: any(item in result.items: item.target == target and item.state == "pending"))
  ensure all(item in result.items: item.state == "reviewed" implies item.record != none and item.record.digest == item.digest)
}

property records-preserve-history invariant "相同记录重复读取不改变内容，不同记录不覆盖；缺失存储为空，损坏记录与符号链接有完整定位诊断" {
  about read, write
  forall sample: Sample generated "合法评审记录、重复记录及损坏、非法路径、符号链接等文件树变体"
  manual "准备并读取实际文件树，独立比较全部原始字节和记录，不把文件系统判定包装为布尔操作"
}

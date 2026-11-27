component review "评审记录：保存在本地，按内容判断是否有效，列出当前和比较中待审的项" {
  focus core
  paths "internal/review/**"
  uses graph, diag
}

type Target record "评审的目标：恰是一个节点 node 或一条边（edgeKind、from、to），removed 表示它是比较中已经删除的项" {
  field node: string
  field edgeKind: string
  field from: string
  field to: string
  field removed: bool
}

type Record record "一条不可变的评审记录：格式版本、评审者、时间、执行输入哈希、提交、工作区是否有改动、目标、内容指纹、结论和意见，id 是其余字段的规范 JSON 的 SHA-256" {
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

type Loaded record "read 的结果：评审存储中的记录和诊断" {
  field records: list<Record>
  field diagnostics: list<diag.Diagnostic>
}

type Evaluation record "evaluate 的结果：按目标排序的全部候选项及其评审状态，以及诊断；评过不等于批准" {
  field items: list<Item>
  field diagnostics: list<diag.Diagnostic>
}

type Request record "write 的请求：目标、评审者、结论、意见、提交和工作区是否有改动；expectedDigest 不为空而目标的指纹已变时拒绝写入" {
  field target: Target
  field reviewer: string
  field decision: string
  field comment: string
  field expectedDigest: string
  field commit: string
  field dirty: bool
}

type Written record "write 的结果：写出的记录或者诊断，有诊断时没有新记录" {
  field record: optional<Record>
  field diagnostics: list<diag.Diagnostic>
}

type Attached record "attach 的结果：评审层的节点和指向目标的 about 边，输入的图不改动" {
  field fragment: graph.Fragment
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "review 各性质的输入：一张图、可选的基准图、一组评审记录，以及应有的全部候选目标和待审目标" {
  field graph: graph.Graph
  field base: optional<graph.Graph>
  field records: list<Record>
  field targets: list<Target>
  field pending: list<Target>
}

op read(root: string) -> Loaded "读取 .unicode/reviews 下的不可变记录，目录不存在时为空，损坏的记录、不认识的版本或符号链接是错误"

op evaluate(g: graph.Graph, base: optional<graph.Graph>, records: list<Record>) -> Evaluation pure "没有 base 时列出当前设计和生成文件之外的代码的节点和边，有 base 时列出新增、修改和删除的项，以及因手写谓词或观察适配器变化而须重新评审的性质"

op write(root: string, g: graph.Graph, base: optional<graph.Graph>, request: Request) -> Written "核对目标和 expectedDigest 后写入一条不可变记录；结论 approved 和 changes-requested 都表示这份内容已经评过"

op attach(g: graph.Graph, records: list<Record>) -> Attached pure "为记录生成评审层的节点：目标是节点时用 about 边指向它，目标是边时在属性中保存边的完整标识，并用 about 边指向仍存在的端点"

op recorded(g: graph.Graph) -> Loaded pure "attach 的逆：从 g 的评审层节点还原出评审记录并按记录的标识排序，节点的属性不全或还原出的不是合法记录时给出这个节点的诊断"

property evaluation-follows-content postcondition "evaluate 没有诊断，列出的候选项恰是 sample.targets，待审项恰是 sample.pending，状态为 reviewed 的项带指纹相同的记录" {
  about evaluate
  forall sample: Sample generated "整数 -20 至 20 构造的一张固定的小图，约一半附基准图，以及十四种改动之一：不改、新增节点、删除节点及其边、改性质或观察的摘要、改手写谓词、观察适配器或无关函数的函数体、只移动无关函数的位置、新增、删除或改动边、改生成文件中的函数、把谓词绑定移到另一个函数；改动前和改动后的图中每个目标各以一半机会带按所在图的内容写的 approved 记录，已删除的项另以一半机会带 changes-requested 记录，并附应有的候选和待审目标"
  call evaluate(sample.graph, sample.base, sample.records)
  ensure size(result.diagnostics) == 0
  ensure size(result.items) == size(sample.targets)
  ensure all(target in sample.targets: any(item in result.items: item.target == target))
  ensure count(item in result.items: item.state == "pending") == size(sample.pending)
  ensure all(target in sample.pending: any(item in result.items: item.target == target and item.state == "pending"))
  ensure all(item in result.items: item.state == "reviewed" implies item.record != none and item.record.digest == item.digest)
}

property records-preserve-history invariant "写过的记录再读出时内容不变，同一请求再写不产生新记录，不同的记录（含并发写入的）都保留、互不改写，存储不存在时读出为空，损坏的记录、不认识的版本和符号链接给出诊断" {
  about read, write
  forall sample: Sample generated "整数 -20 至 20 构造的一张固定的小图，约一半附基准图，以及十四种改动之一：不改、新增节点、删除节点及其边、改性质或观察的摘要、改手写谓词、观察适配器或无关函数的函数体、只移动无关函数的位置、新增、删除或改动边、改生成文件中的函数、把谓词绑定移到另一个函数；改动前和改动后的图中每个目标各以一半机会带按所在图的内容写的 approved 记录，已删除的项另以一半机会带 changes-requested 记录，并附应有的候选和待审目标"
  manual "需要在临时目录中写出并读回文件树，表达式不能读写文件"
}

property recorded-inverts-attach postcondition "把 sample.records 附到 sample.graph 上之后，recorded 从评审层还原出的记录恰是 sample.records 中的记录，没有诊断" {
  about attach, recorded
  forall sample: Sample generated "整数 -20 至 20 构造的一张固定的小图，约一半附基准图，以及十四种改动之一：不改、新增节点、删除节点及其边、改性质或观察的摘要、改手写谓词、观察适配器或无关函数的函数体、只移动无关函数的位置、新增、删除或改动边、改生成文件中的函数、把谓词绑定移到另一个函数；改动前和改动后的图中每个目标各以一半机会带按所在图的内容写的 approved 记录，已删除的项另以一半机会带 changes-requested 记录，并附应有的候选和待审目标"
  forall projected: graph.Graph generated "sample.graph 去掉其中的评审层节点及其边，再加上 attach 对它和 sample.records 生成的评审层节点和边"
  call recorded(projected)
  ensure size(result.diagnostics) == 0
  ensure all(record in sample.records: record in result.records)
  ensure all(record in result.records: record in sample.records)
}

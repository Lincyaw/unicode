component review "评审记录：保存在本地，按内容判断是否有效，列出当前和比较中待审的项" {
  focus core
  paths "internal/review/**"
  uses graph, diag, repo
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

type Malformed record "recorded-reports-malformed-nodes 的输入：一张带评审层的图、其中不是合法记录的评审层节点的标识，以及其余评审层节点带的合法记录" {
  field graph: graph.Graph
  field broken: list<string>
  field valid: list<Record>
}

type WriteSample record "write 各性质的输入：一张图、可选的基准图、先写入存储的请求、之后要写的请求、它是否与同一目标最后一个先写入的请求相同，以及它应得的诊断码" {
  field graph: graph.Graph
  field base: optional<graph.Graph>
  field history: list<Request>
  field request: Request
  field repeat: bool
  field code: string
}

type LatestSample record "evaluation-picks-latest 的输入：一张图、不给的基准图、一组评审记录，以及按构造定下的每个有记录的目标的最新记录" {
  field graph: graph.Graph
  field base: optional<graph.Graph>
  field records: list<Record>
  field latest: list<Record>
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

property read-is-empty-without-store postcondition "仓库中没有评审存储时，read 没有记录，也没有诊断" {
  about read
  forall tree: repo.Tree generated "空树，只有 .unicode 目录的树，或者只有 design/system.uc 的树，三者机会相等"
  forall dir: string generated "materialize 把 tree 写成的新目录"
  call read(dir)
  ensure size(result.records) == 0 and size(result.diagnostics) == 0
}

property write-records-the-request postcondition "write 一个合法的请求，得到版本为 1 的记录，评审者、结论、意见、目标、提交和工作区是否有改动都与请求相同，执行输入哈希是 graph.inputDigest 给图的，evaluate 列出这个目标时，记录的内容指纹是它列出的" {
  about write
  forall sample: WriteSample generated "evaluation-follows-content 的 sample 那样的图和基准图，零至四个先写入的请求和一个之后要写的请求：目标取自图中当前的候选项和比较中删除的项，评审者取 reviewer、张三、review-bot 之一，结论取 approved 或 changes-requested，意见取空、英文或含中文和表情符号的文字，提交为空或 40 位十六进制，工作区是否有改动随机，不给 expectedDigest；repeat 表示之后要写的请求与同一目标最后一个先写入的请求相同，code 为空"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  observe stored: Loaded from dir
  changes stored
  call write(dir, sample.graph, sample.base, sample.request)
  ensure size(result.diagnostics) == 0 and result.record != none
  ensure result.record.version == 1 and result.record.reviewer == sample.request.reviewer and result.record.decision == sample.request.decision and result.record.comment == sample.request.comment and result.record.target == sample.request.target and result.record.commit == sample.request.commit and result.record.dirty == sample.request.dirty
  ensure result.record.snapshot == graph.inputDigest(sample.graph)
  ensure all(item in evaluate(sample.graph, sample.base, stored.records).items: item.target == sample.request.target implies item.digest == result.record.digest)
}

property write-stores-the-record postcondition "write 一个合法的请求之后，read 读出的记录中有 write 返回的记录，没有诊断" {
  about write, read
  forall sample: WriteSample generated "evaluation-follows-content 的 sample 那样的图和基准图，零至四个先写入的请求和一个之后要写的请求：目标取自图中当前的候选项和比较中删除的项，评审者取 reviewer、张三、review-bot 之一，结论取 approved 或 changes-requested，意见取空、英文或含中文和表情符号的文字，提交为空或 40 位十六进制，工作区是否有改动随机，不给 expectedDigest；repeat 表示之后要写的请求与同一目标最后一个先写入的请求相同，code 为空"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  observe stored: Loaded from dir
  changes stored
  call write(dir, sample.graph, sample.base, sample.request)
  ensure size(stored.diagnostics) == 0 and result.record in stored.records
}

property write-keeps-history postcondition "write 不改动目录中已有的条目，至多新加一个文件" {
  about write
  forall sample: WriteSample generated "evaluation-follows-content 的 sample 那样的图和基准图，零至四个先写入的请求和一个之后要写的请求：目标取自图中当前的候选项和比较中删除的项，评审者取 reviewer、张三、review-bot 之一，结论取 approved 或 changes-requested，意见取空、英文或含中文和表情符号的文字，提交为空或 40 位十六进制，工作区是否有改动随机，不给 expectedDigest；repeat 表示之后要写的请求与同一目标最后一个先写入的请求相同，code 为空"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  observe files: repo.Tree from dir
  changes files
  call write(dir, sample.graph, sample.base, sample.request)
  ensure all(p in old(files).entries: p in files.entries and files.entries[p] == old(files).entries[p])
  ensure count(p in files.entries: not p in old(files).entries and files.entries[p].kind == repo.EntryKind.file) <= 1
}

property write-adds-a-new-record postcondition "请求与同一目标最后一个先写入的请求不同时，write 加一条记录，它的标识不同于已有的记录，时间不同于同一目标已有的记录" {
  about write
  forall sample: WriteSample generated "evaluation-follows-content 的 sample 那样的图和基准图，零至四个先写入的请求和一个之后要写的请求：目标取自图中当前的候选项和比较中删除的项，评审者取 reviewer、张三、review-bot 之一，结论取 approved 或 changes-requested，意见取空、英文或含中文和表情符号的文字，提交为空或 40 位十六进制，工作区是否有改动随机，不给 expectedDigest；repeat 表示之后要写的请求与同一目标最后一个先写入的请求相同，code 为空"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  observe stored: Loaded from dir
  changes stored
  call write(dir, sample.graph, sample.base, sample.request)
  ensure not sample.repeat implies size(stored.records) == size(old(stored).records) + 1 and not any(r in old(stored).records: r.id == result.record.id)
  ensure not sample.repeat implies all(r in old(stored).records: r.target == sample.request.target implies r.at != result.record.at)
}

property write-is-idempotent postcondition "把同一目标最后写入的请求再写一次，write 没有诊断，返回存储中已有的记录，不改动目录" {
  about write
  forall sample: WriteSample generated "与 write-records-the-request 的 sample 相同地取图和请求，但至少有一个先写入的请求，之后要写的请求就是最后一个先写入的请求，repeat 为真"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  observe stored: Loaded from dir
  observe files: repo.Tree from dir
  changes none
  call write(dir, sample.graph, sample.base, sample.request)
  ensure size(result.diagnostics) == 0 and result.record in stored.records
}

property read-is-repeatable postcondition "read 不改动目录，读出的结果与之前一次 read 的相同" {
  about read
  forall sample: WriteSample generated "evaluation-follows-content 的 sample 那样的图和基准图，零至四个先写入的请求和一个之后要写的请求：目标取自图中当前的候选项和比较中删除的项，评审者取 reviewer、张三、review-bot 之一，结论取 approved 或 changes-requested，意见取空、英文或含中文和表情符号的文字，提交为空或 40 位十六进制，工作区是否有改动随机，不给 expectedDigest；repeat 表示之后要写的请求与同一目标最后一个先写入的请求相同，code 为空"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  observe stored: Loaded from dir
  observe files: repo.Tree from dir
  changes none
  call read(dir)
  ensure result == old(stored)
}

property write-rejects-invalid-requests postcondition "请求不合法或 expectedDigest 已过时时，write 给出 sample.code 的诊断，不返回记录，不改动目录" {
  about write
  forall sample: WriteSample generated "与 write-records-the-request 的 sample 相同地取图和先写入的请求，之后要写的请求做六种改动之一，code 是应得的诊断码：评审者只有一个空格（review.invalid-reviewer），结论是 unknown（review.invalid-decision），目标是不存在的节点（review.unknown-target），目标同时给出节点和边（review.invalid-target），expectedDigest 不是 64 位十六进制（review.invalid-record），或者 expectedDigest 是 64 个 0（review.stale-target）"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  observe files: repo.Tree from dir
  changes none
  call write(dir, sample.graph, sample.base, sample.request)
  ensure any(d in result.diagnostics: d.code == sample.code) and result.record == none
}

property write-diagnostics-are-described postcondition "write 给出的每条诊断都有位置的路径、从 1 起的行和列、原因和修正" {
  about write
  forall sample: WriteSample generated "与 write-records-the-request 的 sample 相同地取图和先写入的请求，之后要写的请求做六种改动之一，code 是应得的诊断码：评审者只有一个空格（review.invalid-reviewer），结论是 unknown（review.invalid-decision），目标是不存在的节点（review.unknown-target），目标同时给出节点和边（review.invalid-target），expectedDigest 不是 64 位十六进制（review.invalid-record），或者 expectedDigest 是 64 个 0（review.stale-target）"
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求"
  call write(dir, sample.graph, sample.base, sample.request)
  ensure all(d in result.diagnostics: size(d.position.path) > 0 and d.position.line >= 1 and d.position.column >= 1 and size(d.message) > 0 and size(d.fix) > 0)
}

property read-rejects-corrupt-records postcondition "存储中有版本不对、多出字段或有重复键的记录时，read 给出 review.invalid-record，不改动目录" {
  about read
  forall sample: WriteSample generated "与 write-records-the-request 的 sample 相同地取图和请求，但至少有一个先写入的请求，之后要写的请求就是最后一个先写入的请求，repeat 为真"
  forall damage: int
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求，再按 damage 改坏存储：0 时另加一个名为 64 个 0 加 .json、内容是 {\"version\":42} 的文件，1 时给名字最小的记录文件的 JSON 对象另加一个字段，2 时在它的 JSON 对象开头另加一个 comment 键"
  where damage in 0..2
  observe files: repo.Tree from dir
  changes none
  call read(dir)
  ensure any(d in result.diagnostics: d.code == "review.invalid-record")
}

property read-rejects-symlinks postcondition "评审存储中的记录文件或仓库的 .unicode 是符号链接时，read 给出 review.symlink，不改动目录" {
  about read
  forall sample: WriteSample generated "与 write-records-the-request 的 sample 相同地取图和请求，但至少有一个先写入的请求，之后要写的请求就是最后一个先写入的请求，repeat 为真"
  forall linkRecord: bool
  forall dir: string generated "一个新的空目录，其中 store 下依次用 write 写入了 sample.history 中的请求；linkRecord 为真时 store 的 .unicode/reviews 下另有一个名为 64 个 1 加 .json、指向名字最小的记录文件的符号链接，否则另有 repo 目录，其中的 .unicode 是指向 ../store/.unicode 的符号链接"
  forall root: string generated "linkRecord 为真时是 dir 下的 store，否则是 dir 下的 repo"
  observe files: repo.Tree from dir
  changes none
  call read(root)
  ensure any(d in result.diagnostics: d.code == "review.symlink")
}

property write-rejects-symlinks postcondition "评审存储中的记录文件或仓库的 .unicode 是符号链接时，write 给出 review.symlink，不返回记录，不改动目录" {
  about write
  forall sample: WriteSample generated "与 write-records-the-request 的 sample 相同地取图和请求，但至少有一个先写入的请求，之后要写的请求就是最后一个先写入的请求，repeat 为真"
  forall linkRecord: bool
  forall dir: string generated "一个新的空目录，其中 store 下依次用 write 写入了 sample.history 中的请求；linkRecord 为真时 store 的 .unicode/reviews 下另有一个名为 64 个 1 加 .json、指向名字最小的记录文件的符号链接，否则另有 repo 目录，其中的 .unicode 是指向 ../store/.unicode 的符号链接"
  forall root: string generated "linkRecord 为真时是 dir 下的 store，否则是 dir 下的 repo"
  observe files: repo.Tree from dir
  changes none
  call write(root, sample.graph, sample.base, sample.request)
  ensure any(d in result.diagnostics: d.code == "review.symlink") and result.record == none
}

property read-diagnostics-are-described postcondition "read 对损坏的存储给出的每条诊断都有位置的路径、从 1 起的行和列、原因和修正" {
  about read
  forall sample: WriteSample generated "与 write-records-the-request 的 sample 相同地取图和请求，但至少有一个先写入的请求，之后要写的请求就是最后一个先写入的请求，repeat 为真"
  forall damage: int
  forall dir: string generated "一个新的空目录，其中依次用 write 写入了 sample.history 中的请求，再按 damage 改坏存储：0 时另加一个名为 64 个 0 加 .json、内容是 {\"version\":42} 的文件，1 时给名字最小的记录文件的 JSON 对象另加一个字段，2 时在它的 JSON 对象开头另加一个 comment 键"
  where damage in 0..2
  call read(dir)
  ensure all(d in result.diagnostics: size(d.position.path) > 0 and d.position.line >= 1 and d.position.column >= 1 and size(d.message) > 0 and size(d.fix) > 0)
}

property attach-keeps-input-digest postcondition "attach 给 records 中的每条记录一个评审层节点，把它们加进图之后，graph.inputDigest 不变" {
  about attach
  forall sample: Sample generated "整数 -20 至 20 构造的一张固定的小图，约一半附基准图，以及十四种改动之一：不改、新增节点、删除节点及其边、改性质或观察的摘要、改手写谓词、观察适配器或无关函数的函数体、只移动无关函数的位置、新增、删除或改动边、改生成文件中的函数、把谓词绑定移到另一个函数；改动前和改动后的图中每个目标各以一半机会带按所在图的内容写的 approved 记录，已删除的项另以一半机会带 changes-requested 记录，并附应有的候选和待审目标"
  forall records: list<Record> generated "零至八条合法记录：目标是 sample.graph 中的节点或边，约四分之一标为已删除，评审者是 reviewer、中文名或带连字符的名称，结论是 approved 或 changes-requested，时间是 2026-11-27 当天随机的秒并带随机的纳秒，commit 为空或是 40 位或 64 位十六进制，dirty 为真或假，意见为空、英文或含中文和表情符号的文字，内容指纹和执行输入哈希是随机的 64 位十六进制"
  forall bare: graph.Graph generated "sample.graph 去掉其中的评审层节点及其边"
  forall projected: graph.Graph generated "sample.graph 去掉其中的评审层节点及其边，再加上 attach 对它和 records 生成的评审层节点和边"
  call attach(bare, records)
  ensure count(n in result.fragment.nodes: n.layer == graph.Layer.review) == size(records)
  ensure graph.inputDigest(projected) == graph.inputDigest(bare)
}

property evaluation-picks-latest postcondition "没有基准图时，evaluate 给 sample.latest 中每条记录的目标列出的项带的就是这条记录" {
  about evaluate
  forall sample: LatestSample generated "整数 -20 至 20 构造的一张固定的小图中的一至三个当前候选项，各有一至四条按它当前的内容写的合法记录，时间取自 2026-11-27 的三个时刻（可以相同），评审者、结论和意见随机，不给基准图；latest 是每个目标的记录中时间最晚、同一时刻时标识最大的那条"
  call evaluate(sample.graph, sample.base, sample.records)
  ensure all(r in sample.latest: any(item in result.items: item.target == r.target and item.record == r))
}

property recorded-inverts-attach postcondition "把 records 附到 sample.graph 上之后，recorded 从评审层还原出的记录恰是 records 中的记录，没有诊断" {
  about attach, recorded
  forall sample: Sample generated "整数 -20 至 20 构造的一张固定的小图，约一半附基准图，以及十四种改动之一：不改、新增节点、删除节点及其边、改性质或观察的摘要、改手写谓词、观察适配器或无关函数的函数体、只移动无关函数的位置、新增、删除或改动边、改生成文件中的函数、把谓词绑定移到另一个函数；改动前和改动后的图中每个目标各以一半机会带按所在图的内容写的 approved 记录，已删除的项另以一半机会带 changes-requested 记录，并附应有的候选和待审目标"
  forall records: list<Record> generated "零至八条合法记录：目标是 sample.graph 中的节点或边，约四分之一标为已删除，评审者是 reviewer、中文名或带连字符的名称，结论是 approved 或 changes-requested，时间是 2026-11-27 当天随机的秒并带随机的纳秒，commit 为空或是 40 位或 64 位十六进制，dirty 为真或假，意见为空、英文或含中文和表情符号的文字，内容指纹和执行输入哈希是随机的 64 位十六进制"
  forall projected: graph.Graph generated "sample.graph 去掉其中的评审层节点及其边，再加上 attach 对它和 records 生成的评审层节点和边"
  call recorded(projected)
  ensure size(result.diagnostics) == 0
  ensure all(record in records: record in result.records)
  ensure all(record in result.records: record in records)
}

property recorded-reports-malformed-nodes postcondition "recorded 对 malformed.broken 中的每个评审层节点恰给出一条以它为对象的 review.invalid-record 诊断，没有别的诊断，还原出的记录恰是 malformed.valid 中的记录" {
  about recorded
  forall malformed: Malformed generated "一张固定的小图加上 attach 对一至六条合法记录生成的评审层，其中一至全部评审层节点各有一处损坏：缺一个属性、一个属性有两个值、version 为 2、dirty 或 removed 不是布尔值、节点标识不以 review: 开头、改动意见使记录标识与内容不符，或者时间不是规范的 UTC 写法"
  call recorded(malformed.graph)
  ensure size(result.diagnostics) == size(malformed.broken)
  ensure all(id in malformed.broken: count(d in result.diagnostics: d.subject == id and d.code == "review.invalid-record") == 1)
  ensure size(result.records) == size(malformed.valid)
  ensure all(record in malformed.valid: record in result.records)
}

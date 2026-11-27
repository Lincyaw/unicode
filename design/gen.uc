component gen "对每种已配置的语言调用有生成能力的后端，合并成一份生成计划，检查路径和生成标记后逐个文件原子写入" {
  focus core
  paths "internal/gen/**"
  uses graph, lang, diag
}

type WriteSample record "write-preserves-unowned-files 和 check-files-are-current 的输入：已有的文件树、符号链接，以及一份生成计划的文件" {
  field files: map<string, string>
  field links: map<string, string>
  field edits: list<lang.Edit>
}

type PlanSample record "plan-follows-language-roots 的输入：一张图，以及各语言后端替身预先给出的生成计划" {
  field graph: graph.Graph
  field generations: map<string, lang.Generation>
}

op plan(root: string, g: graph.Graph, backends: list<lang.Backend>) -> lang.Generation "对 g 的系统文件中配置的每种语言，调用 backends 中有生成能力的后端，把生成计划按路径排序合并；缺少落实的代码或谓词不妨碍生成"

op write(root: string, edits: list<lang.Edit>) -> list<diag.Diagnostic> "先检查 edits 的全部路径、重复目标、符号链接和生成标记，再逐个文件原子写入：只改写带生成标记的文件，createOnly 的文件只在不存在时创建，多个文件之间不保证同时成功"

op checkFiles(root: string, edits: list<lang.Edit>) -> list<diag.Diagnostic> "只读地检查生成计划的路径和生成标记，逐字节核对全部文件，缺失或内容不同时给出诊断，不跟随符号链接，也不创建或改写文件"

property write-preserves-unowned-files invariant "write 只改写生成计划中带生成标记的文件：任何一个目标不安全时不写任何文件，同一份生成计划写两次结果不变，其中 createOnly 的文件第二次报冲突且不被改动" {
  about write
  forall sample: WriteSample generated "一棵含一个带生成标记的文件和一个不带的文件的树，以及新建一个文件的生成计划，十三种情形之一：三种安全（只新建，另改写带生成标记的文件，新建的文件为 createOnly），十种冲突（改写不带生成标记的文件、越出仓库、同一目标两次、经过文件或目录的符号链接、含 . 段、目标是另一目标的父目录、空路径、对已有的文件 createOnly）"
  manual "需要在临时目录中写出文件树和符号链接，表达式不能读写文件"
}

property plan-follows-language-roots invariant "plan 只调用系统文件中配置的语言的后端，各一次，传入该语言的源码根目录和同一张图；同一语言有两个后端、后端没有生成能力或两份生成计划写同一路径时给出诊断" {
  about plan
  forall sample: PlanSample generated "只有系统节点的图，go 和 python 各有一半机会配置一至三个源码根目录，以及三种语言（含一种未配置的）的后端替身，各预先给出零至三个文件，一半另带一条诊断"
  manual "需要用记录调用的后端替身观察 plan 调用了哪些后端和传入的参数，表达式不能记录调用"
}

property check-files-are-current invariant "checkFiles 对 write 拒绝的生成计划同样报错，全部目标存在且内容逐字节相同时通过，缺失或不同时报 gen.stale-file，检查前后文件、目录和链接都不变" {
  about checkFiles
  forall sample: WriteSample generated "与 write-preserves-unowned-files 相同的文件树和生成计划"
  manual "需要在临时目录中写出文件树，比较检查前后的内容、权限、修改时间和链接，表达式不能读写文件"
}

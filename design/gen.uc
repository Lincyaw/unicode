component gen "按语言根目录编排生成计划，预检路径与归属后原子写入生成文件" {
  focus core
  paths "internal/gen/**"
  uses graph, lang, diag
}

type WriteSample record "生成文件写入的输入：已有文件树、符号链接和候选文件" {
  field files: map<string, string>
  field links: map<string, string>
  field edits: list<lang.Edit>
}

type PlanSample record "按语言根目录调度生成后端的样本" {
  field graph: graph.Graph
  field generations: map<string, lang.Generation>
}

op plan(root: string, g: graph.Graph, backends: list<lang.Backend>) -> lang.Generation "按系统 roots 选取可选 Generator，按路径排序合并结果；不以缺少实现或谓词阻止生成"

op write(root: string, edits: list<lang.Edit>) -> list<diag.Diagnostic> "预检全部路径、重复目标、符号链接和生成标记，再逐文件原子写入；只覆盖工具生成文件，不承诺跨文件事务"

op checkFiles(root: string, edits: list<lang.Edit>) -> list<diag.Diagnostic> "只读检查计划的路径和归属，逐字节核对全部生成文件；缺失或内容陈旧时给出诊断，不跟随符号链接，不创建或改写文件"

property write-preserves-unowned-files invariant "写入只改计划中的工具生成文件；路径非法、重复、符号链接或已有文件没有生成标记时预检失败且不写任何文件；相同计划重复应用保持内容不变" {
  about write
  forall sample: WriteSample generated "临时仓库中的普通文件、工具生成文件、符号链接和合法或带一处冲突的生成计划"
  manual "完整声明涉及文件系统的归属、符号链接和原子替换，由手写谓词在临时目录观察"
}

property plan-follows-language-roots invariant "只调用系统配置语言的生成后端，传入准确的源码根目录和图；重复后端、缺少生成能力或冲突路径给出定位诊断；有诊断不写文件" {
  about plan
  forall sample: PlanSample generated "带多种语言 roots 的图，以及记录调用和预设生成结果的后端替身"
  manual "需要观察后端调度及调用参数，不属于值之间的关系"
}

property check-files-are-current invariant "只读核对拒绝不安全或冲突的计划；全部目标存在且内容逐字节相等时通过，缺失或不同时报错；文件、目录和链接不因检查改变" {
  about checkFiles
  forall sample: WriteSample generated "原有文件树和生成计划，有时相同、有时缺失、内容变化或带路径与归属冲突"
  manual "需要核对实际文件、目录、权限和符号链接，并观察检查前后的文件树"
}

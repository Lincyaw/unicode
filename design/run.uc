component run "按语言根目录编排回放与探索，记录黑盒契约的结果；覆盖录制在 M6 完成" {
  focus core
  paths "internal/run/**"
  uses graph, lang, syntax, diag
}

type ExecuteSample record "运行编排的输入：系统配置、目标标识和后端报告" {
  field graph: graph.Graph
  field ids: list<string>
  field reports: map<string, lang.RunReport>
}

op execute(root: string, g: graph.Graph, backends: list<lang.Backend>, ids: list<string>, explore: duration) -> lang.RunReport "按系统 roots 和目标性质分语言调度 Runner，合并结果并原子写出 .unicode/results.json；拒绝候选和观察失败单独报告"

property execute-follows-language-roots invariant "只运行系统配置语言的目标性质，传入准确 roots、ids 和预算；未知目标、缺少运行能力或重复后端给出诊断；结果与后端报告一致并保存到 results.json" {
  about execute
  forall sample: ExecuteSample generated "带语言 roots 与性质的图，以及记录调用和预设报告的后端替身；目标有时不存在"
  manual "需要观察后端调度、运行边界和结果文件，由手写谓词在临时目录检查"
}

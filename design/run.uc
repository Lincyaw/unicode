component run "对每种已配置的语言调用有运行能力的后端，回放或探索性质和流程，把结论写到 .unicode/results.json；按覆盖挑选并录制输入属于 design.md §9 的 M6" {
  focus core
  paths "internal/run/**"
  uses graph, lang, syntax, diag, evidence, repo, monitor
}

type ExecuteSample record "execute-follows-language-roots 的输入：一张图（系统节点给出语言和源码根目录）、要运行的目标标识、各目标的采集的调用的输入，以及各语言后端替身预先给出的报告" {
  field graph: graph.Graph
  field ids: list<string>
  field traffic: map<string, list<monitor.Input>>
  field reports: map<string, lang.RunReport>
}

op execute(plan: lang.RunPlan, backends: list<lang.Backend>) -> lang.RunReport "对 plan.graph 的系统文件中配置的每种语言，调用 backends 中有运行能力的后端运行 plan.ids 中的性质和流程（为空时运行全部）：传入 plan 的仓库根目录、图、explore 和 record，这种语言的源码根目录、选中的目标，以及 plan.traffic 中这些目标的采集的调用的输入（explore 为零时只回放、否则在这段时间内探索，record 为真时写回录制文件），合并逐输入的结论并附上执行输入哈希和仓库的提交信息，交给 evidence 保存成不可变的运行记录和最新的 results.json"

property execute-follows-language-roots invariant "execute 只为有选中目标的已配置语言各调用一次后端，传入仓库根目录、同一张图、这种语言的源码根目录、排好序的目标标识、explore 和 traffic 中恰是这些目标的输入，按标识合并的结论与后端报告一致，返回的报告原样写入 results.json；有未知目标时给出诊断且不调用后端，缺少运行能力、重复的后端或后端漏报结论时也给出诊断" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个源码根目录，各有一至三条性质，后端替身预先给出通过或不通过的结论；每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识，约一半的性质另有一至两个整数符号的采集的调用的输入"
  manual "需要在临时目录中运行 execute 再读回 results.json，表达式不能读写文件"
}

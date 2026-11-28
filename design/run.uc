component run "对每种已配置的语言调用有运行能力的后端，回放或探索性质和流程，把结论写到 .unicode/results.json；按覆盖挑选并录制输入属于 design.md §9 的 M6" {
  focus core
  paths "internal/run/**"
  uses graph, lang, syntax, diag, evidence, repo, monitor
}

type ExecuteSample record "execute 各性质的输入：一张图（系统节点给出语言和源码根目录）、要运行的目标标识、各目标的采集的调用的输入、各语言后端替身预先给出的报告、各语言选中的性质，以及选中的性质应得的结论" {
  field graph: graph.Graph
  field ids: list<string>
  field traffic: map<string, list<monitor.Input>>
  field reports: map<string, lang.RunReport>
  field selected: map<string, list<string>>
  field expected: list<lang.RunResult>
}

type RunReceived record "execute 的后端替身收到的一次运行输入：仓库根目录、源码根目录、图编码出的快照、要运行的标识、探索时长、是否录制，以及按标识给出的采集的调用的输入" {
  field root: string
  field roots: list<string>
  field snapshot: bytes
  field ids: list<string>
  field explore: duration
  field record: bool
  field traffic: map<string, list<monitor.Input>>
}

op execute(plan: lang.RunPlan, backends: list<lang.Backend>) -> lang.RunReport "对 plan.graph 的系统文件中配置的每种语言，调用 backends 中有运行能力的后端运行 plan.ids 中的性质和流程（为空时运行全部）：传入 plan 的仓库根目录、图、explore 和 record，这种语言的源码根目录、选中的目标，以及 plan.traffic 中这些目标的采集的调用的输入（explore 为零时只回放、否则在这段时间内探索，record 为真时写回录制文件），合并逐输入的结论并附上执行输入哈希和仓库的提交信息，交给 evidence 保存成不可变的运行记录和最新的 results.json"

property execute-dispatches-selected-checks postcondition "目标都存在时，execute 恰给有选中的性质的每种已配置语言的后端一次运行输入，传入 plan 的仓库根目录、图、explore 和 record，这种语言的源码根目录、排好序的选中标识，以及 plan.traffic 中恰是这些标识的输入，不调用其余的后端" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个由一至七个小写字母组成的源码根目录，各有一至三条性质，谓词在这种语言的测试文件中，后端替身预先给出的结论各以一半机会通过（不通过的带一条诊断），有效执行数 1 至 50，拒绝数 0 至 10，每条性质一半有一至两个采集的调用的输入（符号 n 取 -9 至 9 的整数），每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识；selected 是各语言选中的性质按标识排序，expected 是选中的性质的结论按标识排序"
  forall explore: duration generated "零至三秒的整秒"
  forall record: bool generated "真或假"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall plan: lang.RunPlan generated "以 dir 为仓库根目录，运行 sample.graph 中 sample.ids 的性质，探索时长 explore，录制与否 record，采集的调用的输入 sample.traffic，没有源码根目录，探索的候选输入数为零"
  forall backends: list<lang.Backend> generated "go、python 和 unused 各一个后端替身，记下收到的每次运行输入，按报告中相反的次序返回 sample.reports 中这种语言的报告里选中的标识的结论，以及报告的诊断"
  observe received: map<string, list<RunReceived>> from backends
  changes received
  call execute(plan, backends)
  ensure not any(id in sample.ids: sample.graph.node(id) == none) implies all(l in received: (l in sample.selected and size(sample.selected[l]) > 0) == (size(received[l]) == 1) and size(received[l]) <= 1)
  ensure not any(id in sample.ids: sample.graph.node(id) == none) implies all(l in sample.selected: all(p in received[l]: p.root == plan.root and p.roots == lang.languageRoots(sample.graph)[l] and p.snapshot == sample.graph.encode() and p.ids == sample.selected[l] and p.explore == plan.explore and p.record == plan.record))
  ensure not any(id in sample.ids: sample.graph.node(id) == none) implies all(l in sample.selected: all(p in received[l]: all(id in p.traffic: id in p.ids and id in plan.traffic and p.traffic[id] == plan.traffic[id]) and all(id in p.ids: id in plan.traffic implies id in p.traffic)))
}

property execute-merges-results postcondition "目标都存在时，execute 没有诊断，结论恰是后端为选中的性质给出的结论，按标识排序" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个由一至七个小写字母组成的源码根目录，各有一至三条性质，谓词在这种语言的测试文件中，后端替身预先给出的结论各以一半机会通过（不通过的带一条诊断），有效执行数 1 至 50，拒绝数 0 至 10，每条性质一半有一至两个采集的调用的输入（符号 n 取 -9 至 9 的整数），每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识；selected 是各语言选中的性质按标识排序，expected 是选中的性质的结论按标识排序"
  forall explore: duration generated "零至三秒的整秒"
  forall record: bool generated "真或假"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall plan: lang.RunPlan generated "以 dir 为仓库根目录，运行 sample.graph 中 sample.ids 的性质，探索时长 explore，录制与否 record，采集的调用的输入 sample.traffic，没有源码根目录，探索的候选输入数为零"
  forall backends: list<lang.Backend> generated "go、python 和 unused 各一个后端替身，记下收到的每次运行输入，按报告中相反的次序返回 sample.reports 中这种语言的报告里选中的标识的结论，以及报告的诊断"
  call execute(plan, backends)
  ensure not any(id in sample.ids: sample.graph.node(id) == none) implies size(result.diagnostics) == 0 and result.results == sample.expected
}

property execute-rejects-unknown-checks postcondition "有不存在的目标时，execute 给出 run.unknown-check，不调用任何后端" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个由一至七个小写字母组成的源码根目录，各有一至三条性质，谓词在这种语言的测试文件中，后端替身预先给出的结论各以一半机会通过（不通过的带一条诊断），有效执行数 1 至 50，拒绝数 0 至 10，每条性质一半有一至两个采集的调用的输入（符号 n 取 -9 至 9 的整数），每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识；selected 是各语言选中的性质按标识排序，expected 是选中的性质的结论按标识排序"
  forall explore: duration generated "零至三秒的整秒"
  forall record: bool generated "真或假"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall plan: lang.RunPlan generated "以 dir 为仓库根目录，运行 sample.graph 中 sample.ids 的性质，探索时长 explore，录制与否 record，采集的调用的输入 sample.traffic，没有源码根目录，探索的候选输入数为零"
  forall backends: list<lang.Backend> generated "go、python 和 unused 各一个后端替身，记下收到的每次运行输入，按报告中相反的次序返回 sample.reports 中这种语言的报告里选中的标识的结论，以及报告的诊断"
  observe received: map<string, list<RunReceived>> from backends
  changes received
  call execute(plan, backends)
  ensure any(id in sample.ids: sample.graph.node(id) == none) implies any(d in result.diagnostics: d.code == "run.unknown-check") and all(l in received: size(received[l]) == 0)
}

property execute-saves-its-report postcondition "execute 把返回的报告写进仓库的 .unicode/results.json" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个由一至七个小写字母组成的源码根目录，各有一至三条性质，谓词在这种语言的测试文件中，后端替身预先给出的结论各以一半机会通过（不通过的带一条诊断），有效执行数 1 至 50，拒绝数 0 至 10，每条性质一半有一至两个采集的调用的输入（符号 n 取 -9 至 9 的整数），每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识；selected 是各语言选中的性质按标识排序，expected 是选中的性质的结论按标识排序"
  forall explore: duration generated "零至三秒的整秒"
  forall record: bool generated "真或假"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall plan: lang.RunPlan generated "以 dir 为仓库根目录，运行 sample.graph 中 sample.ids 的性质，探索时长 explore，录制与否 record，采集的调用的输入 sample.traffic，没有源码根目录，探索的候选输入数为零"
  forall backends: list<lang.Backend> generated "go、python 和 unused 各一个后端替身，记下收到的每次运行输入，按报告中相反的次序返回 sample.reports 中这种语言的报告里选中的标识的结论，以及报告的诊断"
  observe stored: optional<lang.RunReport> from dir
  changes stored
  call execute(plan, backends)
  ensure stored == result
}

property execute-rejects-duplicate-backends postcondition "目标都存在而同一语言有两个后端时，execute 给出 run.duplicate-backend" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个由一至七个小写字母组成的源码根目录，各有一至三条性质，谓词在这种语言的测试文件中，后端替身预先给出的结论各以一半机会通过（不通过的带一条诊断），有效执行数 1 至 50，拒绝数 0 至 10，每条性质一半有一至两个采集的调用的输入（符号 n 取 -9 至 9 的整数），每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识；selected 是各语言选中的性质按标识排序，expected 是选中的性质的结论按标识排序"
  forall explore: duration generated "零至三秒的整秒"
  forall record: bool generated "真或假"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall plan: lang.RunPlan generated "以 dir 为仓库根目录，运行 sample.graph 中 sample.ids 的性质，探索时长 explore，录制与否 record，采集的调用的输入 sample.traffic，没有源码根目录，探索的候选输入数为零"
  forall backends: list<lang.Backend> generated "与 execute-dispatches-selected-checks 的 backends 相同的三个后端替身，再加一次 go 的"
  call execute(plan, backends)
  ensure not any(id in sample.ids: sample.graph.node(id) == none) implies any(d in result.diagnostics: d.code == "run.duplicate-backend")
}

property execute-rejects-backends-without-runner postcondition "目标都存在而后端都没有运行能力时，execute 给有选中的性质的每种语言一条 run.unsupported-language" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个由一至七个小写字母组成的源码根目录，各有一至三条性质，谓词在这种语言的测试文件中，后端替身预先给出的结论各以一半机会通过（不通过的带一条诊断），有效执行数 1 至 50，拒绝数 0 至 10，每条性质一半有一至两个采集的调用的输入（符号 n 取 -9 至 9 的整数），每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识；selected 是各语言选中的性质按标识排序，expected 是选中的性质的结论按标识排序"
  forall explore: duration generated "零至三秒的整秒"
  forall record: bool generated "真或假"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall plan: lang.RunPlan generated "以 dir 为仓库根目录，运行 sample.graph 中 sample.ids 的性质，探索时长 explore，录制与否 record，采集的调用的输入 sample.traffic，没有源码根目录，探索的候选输入数为零"
  forall backends: list<lang.Backend> generated "go 和 python 各一个没有运行能力的后端替身"
  call execute(plan, backends)
  ensure not any(id in sample.ids: sample.graph.node(id) == none) implies count(d in result.diagnostics: d.code == "run.unsupported-language") == count(l in sample.selected: size(sample.selected[l]) > 0)
}

property execute-rejects-omitted-results postcondition "目标都存在而后端漏报选中的性质的结论时，execute 给出 run.invalid-report" {
  about execute
  forall sample: ExecuteSample generated "go 和 python 都配置了一至三个由一至七个小写字母组成的源码根目录，各有一至三条性质，谓词在这种语言的测试文件中，后端替身预先给出的结论各以一半机会通过（不通过的带一条诊断），有效执行数 1 至 50，拒绝数 0 至 10，每条性质一半有一至两个采集的调用的输入（符号 n 取 -9 至 9 的整数），每条性质一半被选中，都不选时表示全部，约四分之一另加一个不存在的标识；selected 是各语言选中的性质按标识排序，expected 是选中的性质的结论按标识排序"
  forall explore: duration generated "零至三秒的整秒"
  forall record: bool generated "真或假"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall plan: lang.RunPlan generated "以 dir 为仓库根目录，运行 sample.graph 中 sample.ids 的性质，探索时长 explore，录制与否 record，采集的调用的输入 sample.traffic，没有源码根目录，探索的候选输入数为零"
  forall backends: list<lang.Backend> generated "go、python 和 unused 各一个后端替身，返回 sample.reports 中这种语言的报告的诊断，不返回结论"
  call execute(plan, backends)
  ensure not any(id in sample.ids: sample.graph.node(id) == none) implies any(d in result.diagnostics: d.code == "run.invalid-report")
}

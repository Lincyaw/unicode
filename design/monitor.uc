component monitor "运行时包：组合根经 Wrap 和 Entry 接入监控文件，受监控的调用按比例采样，判定的结论汇总成运行时的运行记录并经 OpenTelemetry 报告，抽中的调用经脱敏后采集（design.md §6.6）；随工具链发布，编进使用者的程序，只依赖标准库和 OpenTelemetry 的 API" {
  focus support
  paths "monitor/**"
}

type Value record "monitor 写出的模型值，即 design.md §6.4 的规范编码：kind 决定取 text、items 还是 entries，脱敏的值的 kind 为 redacted" {
  field kind: string
  field text: string
  field items: list<Value>
  field entries: list<ValueEntry>
}

type ValueEntry record "记录、映射或调用值中的一项：键和值" {
  field key: Value
  field value: Value
}

type Input record "一次调用冻结的输入：符号的名称到模型值" {
  field values: map<string, Value>
}

type ModelType record "redact 读取的一个命名类型：形式、各字段的类型（含继承的字段）、codec 的表示类型（没有 codec 时为空串），以及是否带标签 sensitive" {
  field form: string
  field fields: map<string, string>
  field codec: string
  field sensitive: bool
}

type Schema record "redact 读取的类型信息：命名类型的标识到它的形式、字段和标签，操作的标识到它的参数名和参数类型" {
  field types: map<string, ModelType>
  field operations: map<string, map<string, string>>
}

type Rates record "各操作的采样比例：缺省比例 fallback，以及按操作的标识给出的比例 ops" {
  field fallback: float
  field ops: map<string, float>
}

type BadRates error "UNICODE_MONITOR_RATE 的文本不合 默认比例[,操作的标识=比例]... 的写法，或者其中的比例不在 0 到 1 之间"

type Verdict enum "一条性质或流程在一次受监控的调用上的结论（language.md E26）：通过、反例、域外调用或检查错误" {
  value passed
  value counterexample
  value outsideDomain
  value checkError
}

type Judgment record "一条性质或流程在一次受监控的调用上的判定：标识 check、结论、不成立的子句、限定或前提（如 ensure#2、where#1、requires#1）、判定了的子句、反例的冻结输入及其输入哈希、检查错误的原因，以及 within 的时长（没有 within 时为零）" {
  field check: string
  field verdict: Verdict
  field clause: string
  field judged: list<string>
  field input: map<string, Value>
  field inputHash: string
  field message: string
  field within: duration
}

type JudgedCall record "一个记录周期中的一次受监控的调用：被调用的设计操作、调用开始的时间、从开始到返回的耗时，以及抽中时判定它的各条性质或流程的结论" {
  field op: string
  field at: time
  field duration: duration
  field judgments: list<Judgment>
}

type Build record "受监控的程序构建所在的提交（没有时为空串）和工作区是否有改动，取自 Go 写进程序的构建信息" {
  field commit: string
  field dirty: bool
}

type ClauseCalls record "运行时的结论中一条不成立的限定或前提，以及它不成立的调用数" {
  field clause: string
  field calls: int
}

type ClauseJudged record "运行时的结论中一个子句，以及它被判定的次数" {
  field clause: string
  field judged: int
}

type Diagnostic record "运行记录中的一条诊断，JSON 与 diag.Diagnostic 相同：诊断码、对象、原因和修正" {
  field code: string
  field subject: string
  field message: string
  field fix: string
}

type Case record "运行时的一个反例，JSON 与 lang.CaseResult 相同：所属的性质或流程、冻结并脱敏的输入及其输入哈希、不成立的子句和调用开始的时间" {
  field id: string
  field check: string
  field input: Input
  field inputHash: string
  field clause: string
  field at: time
}

type Result record "一条性质或流程在一个记录周期中的运行时的结论，JSON 与 lang.RunResult 相同：是否通过、适用的调用数、域外调用数及不成立的限定或前提、各子句的判定次数、反例、带 within 时适用的调用耗时的第 95 百分位，以及核对的轨迹数" {
  field id: string
  field passed: bool
  field accepted: int
  field outsideDomain: list<ClauseCalls>
  field clauses: list<ClauseJudged>
  field cases: list<Case>
  field p95: duration
  field traces: int
  field diagnostics: list<Diagnostic>
}

type Record record "一个记录周期的运行时的运行记录，JSON 与 lang.RunReport 相同：版本、程序构建所在的提交和工作区是否有改动、周期的起止时间、执行场合 runtime、各条结论，以及不属于任何一条结论的检查错误" {
  field version: int
  field commit: string
  field dirty: bool
  field from: time
  field at: time
  field venue: string
  field results: list<Result>
  field diagnostics: list<Diagnostic>
}

type Collected record "一个采集的调用（design.md §6.6）：被调用的操作的标识、接收者的模型值（类型的操作才有）、调用值、程序构建所在的提交和采集的时间，值都已脱敏" {
  field op: string
  field receiver: optional<Value>
  field call: Value
  field commit: string
  field at: time
}

type RatesText record "rates-read-their-text 的输入：一组采样比例，以及把它写成 UNICODE_MONITOR_RATE 的文本" {
  field rates: Rates
  field text: string
}

type SummarySample record "summary-counts-judgments 的输入：一个记录周期中的受监控的调用、程序的构建信息、周期的起止时间，以及按 language.md E16 直接算出的每条性质或流程的第 95 百分位耗时和带 within 的性质或流程的时长" {
  field calls: list<JudgedCall>
  field build: Build
  field from: time
  field to: time
  field p95: map<string, duration>
  field within: map<string, duration>
}

type RedactionSample record "redaction-replaces-sensitive-values 的输入：一个模型值及其类型、类型信息、密钥，以及按 design.md §6.6 直接算出的脱敏结果" {
  field value: Value
  field model: string
  field schema: Schema
  field key: bytes
  field expected: Value
}

op parseRates(text: string) -> Rates pure raises BadRates "读出 UNICODE_MONITOR_RATE 的文本 默认比例[,操作的标识=比例]...：第一项是缺省比例，其余各项是一个操作的比例"

op sampled(rates: Rates, op: string, draw: float) -> bool pure "以 [0, 1) 中均匀抽取的 draw 决定 op 的一次调用是否抽中：draw 小于 op 的比例时抽中，op 在 rates.ops 中时比例取 rates.ops 的值，否则取 rates.fallback"

op summarize(calls: list<JudgedCall>, build: Build, from: time, to: time) -> Record pure "把一个记录周期中受监控的调用的判定汇总成执行场合为 runtime 的运行记录：每条被判定过的性质或流程一个结论，反例带输入，通过的调用不留输入，检查错误列在外层的诊断中"

op redact(value: Value, model: string, schema: Schema, key: bytes) -> Value pure "把类型为 model 的模型值 value 中类型带 sensitive 标签的每个值换成种类为 redacted 的值，其 text 是原值的规范编码以 key 为密钥的 HMAC-SHA-256，写作小写十六进制"

op encodeCollected(collected: Collected) -> bytes pure "把一个采集的调用编码成写进 traffic/ 的 JSON"

property sampling-follows-rates postcondition "sampled 为真当且仅当 draw 小于 op 的比例，op 在 rates.ops 中时比例取 rates.ops[op]，否则取 rates.fallback" {
  about sampled
  forall rates: Rates
  forall op: string generated "约一半是 rates.ops 的一个键，其余是不超过 8 个字符的任意文本"
  forall draw: float
  where rates.fallback in 0.0..1.0
  where rates.ops in 0.0..1.0
  where draw in 0.0..1.0
  call sampled(rates, op, draw)
  ensure result == (op in rates.ops and draw < rates.ops[op] or not op in rates.ops and draw < rates.fallback)
}

property rates-read-their-text postcondition "parseRates 从 sample.text 读出的缺省比例和各操作的比例与 sample.rates 相同" {
  about parseRates
  forall sample: RatesText generated "rates 的缺省比例和零至三个操作的比例都在 0 到 1 之间，操作的标识是一至三段以点分隔的小写名称，text 把它写成 默认比例[,操作的标识=比例]...，操作的次序随机，比例写成能精确读回的最短十进制"
  call parseRates(sample.text)
  ensure result == sample.rates
}

property malformed-rates-are-refused postcondition "parseRates 对不合 默认比例[,操作的标识=比例]... 写法或比例不在 0 到 1 之间的文本抛出 BadRates" {
  about parseRates
  forall text: string generated "不合写法的文本：空串，不是数的缺省比例，小于 0 或大于 1 的比例，缺少等号、标识为空或比例不是数的一项，或者末尾多一个逗号"
  call parseRates(text)
  ensure raised(BadRates)
}

property summary-counts-judgments postcondition "summarize 的运行记录的执行场合为 runtime，提交、工作区是否有改动和起止时间取自输入，每条在 calls 中被判定过的性质或流程恰有一个结论，其适用的调用数是通过和反例的判定数，各条限定或前提的域外调用数、各子句的判定次数和第 95 百分位耗时与 calls 中的相同，反例恰是结论为反例的判定并带其输入、输入哈希、子句和调用的时间，不通过恰在有反例或第 95 百分位超过 within 时，外层的诊断恰有每个检查错误一条" {
  about summarize
  forall sample: SummarySample generated "零至八次调用，操作取自三个标识，时间在一个周期之内，耗时 1 微秒至 1 秒，每次带零至三个判定，性质取自四个标识，其中两个带 within，结论在通过、反例、域外调用和检查错误中均匀地取，子句取自 ensure#1、ensure#2、where#1、requires#1，反例带一至两个符号的输入，以及按 E16 算出的每条性质的第 95 百分位耗时"
  call summarize(sample.calls, sample.build, sample.from, sample.to)
  ensure result.venue == "runtime" and result.version == 1 and result.commit == sample.build.commit and result.dirty == sample.build.dirty and result.from == sample.from and result.at == sample.to
  ensure all(c in sample.calls: all(j in c.judgments: count(r in result.results: r.id == j.check) == 1))
  ensure all(r in result.results: any(c in sample.calls: any(j in c.judgments: j.check == r.id)))
  ensure all(r in result.results: r.accepted == sum(c in sample.calls: count(j in c.judgments: j.check == r.id and (j.verdict == Verdict.passed or j.verdict == Verdict.counterexample))))
  ensure all(c in sample.calls: all(j in c.judgments: j.verdict == Verdict.outsideDomain implies any(r in result.results: r.id == j.check and any(o in r.outsideDomain: o.clause == j.clause))))
  ensure all(r in result.results: all(o in r.outsideDomain: o.calls == sum(c in sample.calls: count(j in c.judgments: j.check == r.id and j.verdict == Verdict.outsideDomain and j.clause == o.clause))))
  ensure all(r in result.results: all(x in r.clauses: x.judged == sum(c in sample.calls: count(j in c.judgments: j.check == r.id and x.clause in j.judged))))
  ensure all(c in sample.calls: all(j in c.judgments: all(x in j.judged: any(r in result.results: r.id == j.check and any(y in r.clauses: y.clause == x)))))
  ensure all(r in result.results: size(r.cases) == sum(c in sample.calls: count(j in c.judgments: j.check == r.id and j.verdict == Verdict.counterexample)))
  ensure all(r in result.results: all(k in r.cases: k.check == r.id and any(c in sample.calls: c.at == k.at and any(j in c.judgments: j.check == r.id and j.verdict == Verdict.counterexample and j.inputHash == k.inputHash and j.clause == k.clause and j.input == k.input.values))))
  ensure all(r in result.results: r.p95 == sample.p95[r.id])
  ensure all(r in result.results: r.passed == (size(r.cases) == 0 and (not r.id in sample.within or r.p95 <= sample.within[r.id])))
  ensure size(result.diagnostics) == sum(c in sample.calls: count(j in c.judgments: j.verdict == Verdict.checkError))
  ensure all(d in result.diagnostics: d.code == "monitor.check-failed" and any(c in sample.calls: any(j in c.judgments: j.verdict == Verdict.checkError and j.check == d.subject)))
}

property redaction-replaces-sensitive-values postcondition "redact 的结果与 sample.expected 相同，即 sample.value 中类型带 sensitive 标签的每个值，无论是值本身、字段、列表或集合的元素、映射的键或值、可选值的内容、codec 的表示还是调用值的实参，都换成种类为 redacted、text 为该值规范编码以 sample.key 为密钥的 HMAC-SHA-256 小写十六进制的值，其余部分不变" {
  about redact
  forall sample: RedactionSample generated "由四个命名类型组成的类型信息：带 sensitive 标签的记录 Secret（字段 text: string）和枚举 Token、不带标签的记录 Holder（字段 secret: Secret、names: list<string>、tokens: map<string, Token>、maybe: optional<Secret>）和带 codec 的 Coded（表示为 list<Secret>），一个操作 store(secret: Secret, count: int)，以及从这些类型、list、set、map、optional 和该操作的调用值中随机取的类型及其值，深度至多四层，密钥一至十六字节，和按定义逐层算出的脱敏结果"
  call redact(sample.value, sample.model, sample.schema, sample.key)
  ensure result == sample.expected
}

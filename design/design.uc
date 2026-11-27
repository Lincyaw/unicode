component design "把语法树解析成设计层的节点和边；检查设计内部、流程、符号与用例的约束" {
  focus core
  paths "internal/design/**"
  uses syntax, graph, diag
}

type Loaded record "载入的结果：设计层的图和诊断；有诊断时，图可能不完整" {
  field graph: graph.Graph
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "一组合法的设计文件和录制文件，以及它们声明的节点和边，供性质使用" {
  field files: map<string, string>
  field nodes: list<string>
  field edges: list<string>
  field attrs: map<string, map<string, list<string>>>
  field edgeAttrs: map<string, map<string, list<string>>>
}

type Broken record "一组合法的文件做了一处违反某条约束的改动：文件、改动所在的路径和行，以及应当得到的诊断码" {
  field files: map<string, string>
  field path: string
  field line: int
  field code: string
}

type ReplySample record "应答脚本类型检查的样本：脚本长度及合法或单处非法变体" {
  field count: int
  field variant: int
}

op load(files: list<syntax.File>) -> Loaded "解析引用，建出设计层的节点和边，检查只需设计就能判断的约束"

property accepts-valid-design invariant "合法的设计载入之后没有诊断，图中恰有它声明的节点和边，标识按 model.md §7 写出；attrs 和 edgeAttrs 中列出的节点与边，属性恰是列出的那些；边的键为种类、起点、终点以空格连接" {
  about load
  forall sample: Sample generated "一组随机的合法文件：系统文件、若干层组件文件和录制文件，引用都能解析，满足只需设计就能判断的全部约束；节点和边写成标识，边写作 种类 起点 终点"
  manual "需要从源文件建立预期图并比较完整的引用、标识与诊断；表达式没有文件解析和图构造操作"
}

property rejects-broken-design invariant "合法的设计做一处违反某条约束的改动之后，载入在改动所在的文件和行给出这条约束的诊断" {
  about load
  forall broken: Broken generated "一组随机的合法文件，在随机的一处做了违反 model.md §10 或 language.md 中某条只需设计就能判断的规则的改动"
  manual "需要从源文件建立预期图并比较完整的引用、标识与诊断；表达式没有文件解析和图构造操作"
}

property load-ignores-file-order relation "文件以任何次序给出，载入得到的快照和诊断都相同" {
  about load
  forall files: map<string, string> generated "一组随机的文件，有的合法，有的带违反约束的改动"
  forall rotation: int
  where rotation in 0..1000
  manual "需要从源文件建立预期图并比较完整的引用、标识与诊断；表达式没有文件解析和图构造操作"
}

property contract-graph-is-declared invariant "合法的黑盒契约投影成恰当的节点、边、契约属性和引用边属性，保留前提、观察和允许变化的路径" {
  about load
  forall sample: Sample generated "沿用完整契约样本，随机域名和金额上下界，并独立构造完整预期图"
  manual "需要从源文件建立预期图并比较完整的引用、标识与属性，沿用合法设计的独立判定"
}

property returned-value-contract-is-declared invariant "返回对象的字段和纯操作可在 result 上访问，投影保留被测调用和完整引用属性" {
  about load
  forall sample: Sample generated "随机域名的返回值契约，保留 result.amount() 与 result.value 的完整预期投影"
  manual "需要从源文件建立预期图并比较完整的引用、标识与属性，沿用合法设计的独立判定"
}

property contract-rejects-invalid-relations invariant "黑盒契约的引用、类型、前提和状态路径关系有一处非法时，在改动位置拒绝" {
  about load
  forall broken: Broken generated "沿用非法契约关系的十三个替换变体及其预期诊断和调用行重定位"
  manual "需要构造非法设计并核对准确的诊断与源位置，沿用错误设计的独立判定"
}

property reply-scripts-are-typed invariant "成功与声明错误的应答、接口操作及继承字段逐一匹配类型；拒绝空脚本、重复目标、错误字段、未声明错误及不是外部直接操作的 replies 目标" {
  about load
  forall sample: ReplySample generated "一至五个应答，以及合法脚本或只改一处类型、目标、字段、基数与归属的变体"
  manual "需要生成完整设计程序并比较载入诊断与位置，表达式没有源文本构造与编译操作"
}

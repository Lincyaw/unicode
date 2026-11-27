component design "把设计文件和录制文件的语法树载入成设计层的节点和边，并检查只需设计就能判断的约束（design.md §5.1）" {
  focus core
  paths "internal/design/**"
  uses syntax, graph, diag
}

type Loaded record "载入的结果：设计层的图和诊断；有诊断时，图中缺少出错之处的节点或边" {
  field graph: graph.Graph
  field diagnostics: list<diag.Diagnostic>
}

type Explored record "loadForExploration 的结果：载入的设计层，丢弃的不再合乎设计的录制用例，以及指向的性质或流程已不存在的录制文件" {
  field loaded: Loaded
  field discarded: list<string>
  field orphaned: list<string>
}

type Sample record "design 中三条接受合法设计的性质的输入：一组合法的设计文件和录制文件，载入后应得的节点标识和边，以及部分节点和边应有的属性；边写成种类、起点和终点以空格相连" {
  field files: map<string, string>
  field nodes: list<string>
  field edges: list<string>
  field attrs: map<string, map<string, list<string>>>
  field edgeAttrs: map<string, map<string, list<string>>>
}

type Broken record "rejects-broken-design 和 contract-rejects-invalid-relations 的输入：一组做了一处违规改动的合法文件，改动所在的路径和行，以及应当得到的诊断码" {
  field files: map<string, string>
  field path: string
  field line: int
  field code: string
}

type ReplySample record "reply-scripts-are-typed 的输入：默认应答脚本的项数 count，以及在固定的设计上做哪一种改动 variant" {
  field count: int
  field variant: int
}

type InheritanceSample record "inheritance-members-are-resolved 的输入：父类型字段的值 value，以及子类型是否另声明同名的字段和操作来遮蔽父类型的 overridden" {
  field value: int
  field overridden: bool
}

op loadForExploration(files: list<syntax.File>) -> Explored "供 unicode test --explore 载入设计：丢弃不再合乎符号的类型和限定的录制用例，忽略目标已不存在的录制文件，其余错误照常给出诊断"

op load(files: list<syntax.File>) -> Loaded "把 files 中的语法树载入成设计层：建出节点和边，解析引用，并检查只需设计就能判断的约束"

property accepts-valid-design invariant "合法的设计载入后没有诊断，图中的节点和边恰是 sample 列出的那些，标识按 model.md §7 写出，sample 列出属性的节点和边也恰有这些属性" {
  about load
  forall sample: Sample generated "一组随机的合法文件：约三分之一是 contract-graph-is-declared 或 returned-value-contract-is-declared 的黑盒契约，其余是系统文件、至多三层的组件文件和录制文件，含各种形式的类型、操作、流程、步骤、性质、限定和用例，名称约四分之一取关键字，满足只需设计就能判断的全部约束"
  manual "需要把源文本解析成语法树再载入，表达式不能解析文本"
}

property rejects-broken-design invariant "合法的设计做一处违反某条约束的改动之后，载入在改动所在的文件和行给出这条约束的诊断" {
  about load
  forall broken: Broken generated "一组随机的合法文件，在随机的一处做了违反 model.md §10 或 language.md 中某条只需设计就能判断的规则的改动；约三分之一是 contract-rejects-invalid-relations 的改动"
  manual "需要把源文本解析成语法树再载入，表达式不能解析文本"
}

property load-ignores-file-order relation "同一组文件以任何次序给出，载入得到的设计层的图和诊断都相同" {
  about load
  forall files: map<string, string> generated "一组随机的文件：一半合法，含黑盒契约，一半带一处违反约束的改动"
  forall rotation: int
  where rotation in 0..1000
  manual "需要把同一组文件排成两种次序的列表各载入一次，表达式不能构造列表的排列"
}

property contract-graph-is-declared invariant "合法的黑盒契约载入后，性质带 symbols、limits、observe、requires、changes、call 和 ensure 属性，具名观察成为带 type 和 inputs 的 observation 节点，子句引用的类型和操作成为带 clauses 和 paths 属性的 refers 边" {
  about load
  forall sample: Sample generated "一个含完整黑盒契约的组件文件：组件名称和金额区间的上下界随机，契约有直接观察路径、具名观察、两条前提、两条变化路径和六条 ensure，附预期的节点、边和属性"
  manual "谓词复用 accepts-valid-design 的手写谓词，它需要把源文本解析成语法树，表达式不能解析文本"
}

property returned-value-contract-is-declared invariant "ensure 中可以访问被测调用返回值 result 的字段和纯操作，载入后性质的 call 属性保留这次调用，result 上访问的类型和操作成为 refers 边" {
  about load
  forall sample: Sample generated "一个组件名称随机的组件文件，其中的黑盒契约调用 make，并在 ensure 中访问 result.amount() 和 result.value，附预期的节点、边和属性"
  manual "谓词复用 accepts-valid-design 的手写谓词，它需要把源文本解析成语法树，表达式不能解析文本"
}

property contract-rejects-invalid-relations invariant "黑盒契约的前提、变化路径、具名观察、about、调用序列或表达式有一处不合规则时，载入在出错的行给出对应的诊断" {
  about load
  forall broken: Broken generated "一条固定的黑盒契约做十三种文本替换之一：前提类型不符或用到 result，changes 改成观察范围之外的路径，具名观察的 from 缺失、重复或类型是调用，about 不含被测的操作，多一个调用序列，被测调用或 ensure 的类型不符或误用 old；附应得的诊断码和行，about 与调用序列的改动落在 call 一行"
  manual "谓词复用 rejects-broken-design 的手写谓词，它需要把源文本解析成语法树，表达式不能解析文本"
}

property reply-scripts-are-typed invariant "应答脚本合乎 model.md §10 第 23、26 条时载入没有诊断，有一处不合时载入给出 design.invalid-replies-type 或 design.value-type-mismatch" {
  about load
  forall sample: ReplySample generated "一至五项应答的脚本，在固定的设计上做十三种改动之一：不改或改用子类型的错误（这两种合法），或者目标不是外部系统的操作、脚本为空、返回值或错误不合、缺字段、同一目标两个脚本、replies 嵌在 list 或操作签名中、接口字段缺操作或应答类型不合、一项同时有 returns 和 raises"
  manual "需要按改动拼出设计文件的文本再解析和载入，表达式不能构造或解析文本"
}

property inheritance-members-are-resolved invariant "继承的字段和操作按 language.md §4.4 的查找规则解析：在表达式、限定、用例和调用序列中都能访问，子类型的同名成员遮蔽父类型的，调用序列以引用的具体类型为接收者，用例给出记录的值时含继承的字段" {
  about load
  forall sample: InheritanceSample generated "父类型字段的值取 -20 至 20 的任意整数，overridden 决定子类型是否另声明同名的字段和操作来遮蔽父类型的"
  manual "需要按 sample 拼出设计文件的文本再解析和载入，表达式不能构造或解析文本"
}

property codecs-are-typed invariant "codec 投影两个绝对操作和引用，encoded 按表示类型检查；错误签名、重复、接口、无声明、表示依赖循环或隐式继承均被拒绝，静态载入不调用解码器" {
  about load
  forall value: int
  forall variant: int
  where value in -1000..1000
  where variant in 0..14
  manual "构造 codec 源程序并核对独立的操作引用、类型、诊断及位置，载入边界不执行原生解码"
}

property exploration-prunes-recorded-inputs invariant "只有显式探索能剔除过期录制输入及无目标的录制文件；合法记录保留，声明错误和非域错误仍阻断，普通载入保持严格" {
  about load, loadForExploration
  forall sample: Sample generated "合法设计和录制样本，分别加入合法、过期符号或类型、域外、无目标与错误路径的录制变体"
  manual "在真实设计语法树和独立记录身份上比较两种载入模式，保留全部非录制域错误"
}

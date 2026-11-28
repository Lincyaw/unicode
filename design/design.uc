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

type Sample record "accepts-valid-design、contract-graph-is-declared、returned-value-contract-is-declared 和 exploration-prunes-recorded-inputs 的输入：一组合法的设计文件和录制文件，载入后应得的节点标识和边，以及部分节点和边应有的属性；边写成种类、起点和终点以空格相连" {
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

op loadSources(sources: list<syntax.Source>) -> Loaded pure "解析 sources 中的每个源文件，把没有诊断的语法树交给 load；诊断是各文件的解析诊断和载入诊断，与 sources 的次序无关"

op loadSourcesForExploration(sources: list<syntax.Source>) -> Explored pure "与 loadSources 一样解析 sources 中的每个源文件，再把没有诊断的语法树交给 loadForExploration；诊断是各文件的解析诊断和载入诊断"

property accepts-valid-design invariant "合法的设计载入后没有诊断，图中的节点和边恰是 sample 列出的那些，标识按 model.md §7 写出，sample 列出属性的节点和边也恰有这些属性" {
  about load
  forall sample: Sample generated "一组随机的合法文件：约三分之一是 contract-graph-is-declared 或 returned-value-contract-is-declared 的黑盒契约，其余是系统文件、至多三层的组件文件和录制文件，含各种形式的类型、操作、流程、步骤、性质、限定和用例，名称约四分之一取关键字，满足只需设计就能判断的全部约束"
  manual "需要把源文本解析成语法树再载入，表达式不能解析文本"
}

property rejects-broken-design invariant "合法的设计做一处违反某条约束的改动之后，载入在改动所在的文件和行给出这条约束的诊断，并且每条诊断都有 design.<名称> 形式的诊断码、subject、原因和修正，除 design.missing-system-file 外都位于给出的文件之内" {
  about load
  forall broken: Broken generated "一组含固定骨架的随机合法文件，等概率取三十四个诊断码之一，多在骨架上做一处违反对应约束的改动，附改动所在的路径、行和诊断码；约三分之一改用 contract-rejects-invalid-relations 的改动"
  manual "需要把源文本解析成语法树再载入，表达式不能解析文本"
}

property load-ignores-file-order relation "同一组文件按路径排序和把排序结果轮转 rotation 位之后各载入一次，得到的设计层的图编码相同、诊断相同" {
  about load
  forall files: map<string, string> generated "一组随机的文件：一半是 accepts-valid-design 取的合法设计，一半是 rejects-broken-design 取的带一处违规改动的设计，两者都约有三分之一是黑盒契约"
  forall rotation: int
  where rotation in 0..1000
  manual "需要把同一组文件排成两种次序的列表各载入一次，表达式不能构造列表的排列"
}

property contract-graph-is-declared invariant "合法的黑盒契约载入后没有诊断，节点和边恰是列出的那些，性质的 symbols、limits、observe、requires、changes、call 和 ensure 属性逐条保留子句，具名观察成为带 type 和 inputs 的 observation 节点，子句引用的类型和操作成为带 clauses 属性的 refers 边，子句访问了字段时边另带 paths 属性" {
  about load
  forall sample: Sample generated "一个固定的系统文件和一个含完整黑盒契约的组件文件：组件名称取 domain0 至 domain9999，金额区间的下界取 1 至 100、上界取下界至 1000，契约有直接观察路径、具名观察、两条前提、两条变化路径和六条 ensure，附预期的节点、边和属性"
  manual "谓词复用 accepts-valid-design 的手写谓词，它需要把源文本解析成语法树，表达式不能解析文本"
}

property returned-value-contract-is-declared invariant "ensure 访问被测调用返回值 result 的字段和纯操作的黑盒契约载入后没有诊断，节点和边恰是列出的那些，性质的 symbols、call 和 ensure 属性保留子句，result 上访问的类型和操作成为带 clauses 属性的 refers 边，访问的字段记在 paths 属性中" {
  about load
  forall sample: Sample generated "一个固定的系统文件和一个组件文件，组件名称取 result0 至 result9999，其中的黑盒契约调用 make，并在 ensure 中访问 result.amount() 和 result.value，附预期的节点、边和属性"
  manual "谓词复用 accepts-valid-design 的手写谓词，它需要把源文本解析成语法树，表达式不能解析文本"
}

property contract-rejects-invalid-relations invariant "黑盒契约的前提、变化路径、具名观察、about、调用序列或表达式有一处不合规则时，载入在出错的文件和行给出对应的诊断，并且每条诊断都有 design.<名称> 形式的诊断码、subject、原因、修正和文件内的位置" {
  about load
  forall broken: Broken generated "一条固定的黑盒契约做十三种文本替换之一：前提类型不符或用到 result，changes 改成观察范围之外的路径，具名观察的 from 列出未声明或重复的符号、观察类型是调用，about 不含被测的操作，多一个调用序列，被测调用的实参类型不符，ensure 调用非纯操作、对整数取下标、误用 old 或约束变量与符号同名；附应得的诊断码和行，about 与调用序列的改动落在 call 一行"
  manual "谓词复用 rejects-broken-design 的手写谓词，它需要把源文本解析成语法树，表达式不能解析文本"
}

property reply-scripts-are-typed invariant "应答脚本合乎 model.md §10 第 23、26 条时载入没有诊断，有一处不合时载入给出 design.invalid-replies-type 或 design.value-type-mismatch，每条诊断都有 design.<名称> 形式的诊断码、subject、原因、修正和文件内的位置" {
  about load
  forall sample: ReplySample generated "一至五项应答的脚本，在固定的设计上做十三种改动之一：不改或改用子类型的错误（这两种合法），或者目标不是外部系统的操作、脚本为空、返回值或错误不合、缺字段、同一目标两个脚本、replies 嵌在 list 或操作签名中、接口字段缺操作或应答类型不合、一项同时有 returns 和 raises"
  manual "需要按改动拼出设计文件的文本再解析和载入，表达式不能构造或解析文本"
}

property inheritance-members-are-resolved invariant "继承的字段和操作按 language.md §4.4 的查找规则解析：在表达式、限定、用例和调用序列中都能访问，子类型的同名成员遮蔽父类型的，调用序列引用类型时以这个类型为接收者、引用操作时以声明操作的类型为接收者，用例给出记录的值时必须含继承的字段，继承对象类型的枚举和接口不暴露字段，图中类型的 fields 属性只列自己声明的字段，每条诊断都有 design.<名称> 形式的诊断码、subject、原因、修正和文件内的位置" {
  about load
  forall sample: InheritanceSample generated "父类型字段的值取 -20 至 20 的任意整数，overridden 决定子类型是否另声明同名的字段和操作来遮蔽父类型的"
  manual "需要按 sample 拼出设计文件的文本再解析和载入，表达式不能构造或解析文本"
}

property codecs-are-typed invariant "codec 载入成 codec 属性中编码和解码操作的引用、codecType 属性中的表示类型和指向这两个操作的 refers 边，经 optional、list、set、map 或另一个 codec 终止的表示被接受，encoded(v) 按表示类型检查，签名不对、重复、用于接口、操作没有声明、没有 codec 却用 encoded、表示类型经必填字段或 codec 循环依赖、靠继承得到 codec 时载入给出诊断，解码后的限定不在载入时检查，每条诊断都有 design.<名称> 形式的诊断码、subject、原因、修正和文件内的位置" {
  about load
  forall value: int
  forall variant: int
  where value in -1000..1000
  where variant in 0..14
  manual "需要按 value 和 variant 拼出含 codec 的设计文件再载入，表达式不能构造或解析文本"
}

property exploration-prunes-recorded-inputs invariant "loadForExploration 丢弃不再合乎设计的录制用例、忽略目标不存在或不是性质或流程的录制文件并保留合法的录制用例，所得的图与不含这些录制时 load 的图相同，声明的用例、重复的录制用例名、放错路径的录制文件和其他设计错误照常给出诊断，而 load 对丢弃的录制用例和忽略的文件仍报错，两种载入都不改动语法树，每条诊断都有 design.<名称> 形式的诊断码、subject、原因、修正和文件内的位置" {
  about load, loadForExploration
  forall sample: Sample generated "与 accepts-valid-design 的 sample 取值相同：一组随机的合法文件，约三分之一是黑盒契约，其余是系统文件、至多三层的组件文件和录制文件，录制用例都合乎设计"
  manual "需要在语法树上增删和改写录制用例，再打印成文件载入，表达式不能构造或打印语法树"
}

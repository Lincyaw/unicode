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

type EdgeRef record "accepts-valid-design、contract-graph-is-declared、returned-value-contract-is-declared、inheritance-members-are-resolved 和 valid-codecs-load 的输入：一条边的种类、起点和终点" {
  field kind: graph.EdgeKind
  field from: string
  field to: string
}

type EdgeAttrs record "accepts-valid-design、contract-graph-is-declared 和 returned-value-contract-is-declared 的输入：一条边的种类、起点、终点和它应有的全部属性" {
  field kind: graph.EdgeKind
  field from: string
  field to: string
  field attrs: map<string, list<string>>
}

type Sample record "accepts-valid-design、contract-graph-is-declared 和 returned-value-contract-is-declared 的输入：一组合法的源文件，载入后应得的全部节点标识和边，以及部分节点和边应有的全部属性" {
  field sources: list<syntax.Source>
  field nodes: list<string>
  field edges: list<EdgeRef>
  field attrs: map<string, map<string, list<string>>>
  field edgeAttrs: list<EdgeAttrs>
}

type Expected record "inheritance-members-are-resolved 和 valid-codecs-load 的输入：一组合法的源文件，部分节点的部分属性应有的值，以及图中必须有的边" {
  field sources: list<syntax.Source>
  field attrs: map<string, map<string, list<string>>>
  field edges: list<EdgeRef>
}

type Broken record "rejects-broken-design、contract-rejects-invalid-relations、reply-scripts-are-typed、inheritance-rejects-invalid-members、codecs-are-typed 和 diagnostics-are-described 的输入：一组做了一处违规改动的源文件，各文件的行数，应当得到的诊断码和它们所在的文件，给出时须相符的行和 subject，以及节点不应有的属性" {
  field sources: list<syntax.Source>
  field lines: map<string, int>
  field codes: list<string>
  field path: string
  field line: optional<int>
  field subject: optional<string>
  field absent: map<string, list<string>>
}

type Exploration record "exploration-keeps-valid-recordings、exploration-discards-stale-recordings、exploration-isolates-orphaned-corpus 和 exploration-keeps-corpus-errors 的输入：一组源文件 sources，加入一个录制用例或录制文件 target 之前的源文件 base，以及载入 sources 时应报的诊断码 code" {
  field sources: list<syntax.Source>
  field base: list<syntax.Source>
  field target: string
  field code: string
}

op loadForExploration(files: list<syntax.File>) -> Explored "供 unicode test --explore 载入设计：丢弃不再合乎符号的类型和限定的录制用例，忽略目标已不存在的录制文件，其余错误照常给出诊断"

op load(files: list<syntax.File>) -> Loaded "把 files 中的语法树载入成设计层：建出节点和边，解析引用，并检查只需设计就能判断的约束"

op loadSources(sources: list<syntax.Source>) -> Loaded pure "解析 sources 中的每个源文件，把没有诊断的语法树交给 load；诊断是各文件的解析诊断和载入诊断，与 sources 的次序无关"

op loadSourcesForExploration(sources: list<syntax.Source>) -> Explored pure "与 loadSources 一样解析 sources 中的每个源文件，再把没有诊断的语法树交给 loadForExploration；诊断是各文件的解析诊断和载入诊断"

property accepts-valid-design postcondition "合法的设计载入后没有诊断，图中的节点恰是 sample.nodes 列出的标识，边恰是 sample.edges 不重复地列出的种类、起点和终点，sample.attrs 和 sample.edgeAttrs 列出的节点和边恰有列出的属性" {
  about loadSources
  forall sample: Sample generated "一组随机的合法源文件：约三分之一是 contract-graph-is-declared 或 returned-value-contract-is-declared 的黑盒契约，其余是系统文件、至多三层的组件文件和录制文件，含各种形式的类型、操作、流程、步骤、性质、限定和用例，名称约四分之一取关键字，满足只需设计就能判断的全部约束；附全部节点和边，以及系统和每个流程的属性，契约另附性质、具名观察和子句引用的边的属性"
  call loadSources(sample.sources)
  ensure size(result.diagnostics) == 0
  ensure all(id in sample.nodes: result.graph.node(id) != none)
  ensure all(n in result.graph.nodes(): n.id in sample.nodes)
  ensure all(x in sample.edges: count(y in sample.edges: y == x) == 1)
  ensure size(result.graph.edges()) == size(sample.edges)
  ensure all(e in result.graph.edges(): any(x in sample.edges: x.kind == e.kind and x.from == e.from and x.to == e.to))
  ensure all(id in sample.attrs: result.graph.node(id).attrs == sample.attrs[id])
  ensure all(x in sample.edgeAttrs: any(e in result.graph.edges(): e.kind == x.kind and e.from == x.from and e.to == x.to and e.attrs == x.attrs))
}

property rejects-broken-design postcondition "合法的设计做一处违反某条约束的改动之后，载入在改动所在的文件给出这条约束的诊断码，给出行或 subject 时诊断的行或 subject 也相符" {
  about loadSources
  forall broken: Broken generated "一组含固定骨架的随机合法源文件，取三十四个诊断码之一，在骨架上做一处违反对应约束的改动；附各文件的行数，以及改动所在的文件和行"
  call loadSources(broken.sources)
  ensure all(c in broken.codes: any(d in result.diagnostics: d.code == c and d.position.path == broken.path and (broken.line == none or d.position.line == broken.line) and (broken.subject == none or d.subject == broken.subject)))
}

property diagnostics-are-described postcondition "载入做了一处违规改动的设计时，loadSources 和 loadSourcesForExploration 给出的每条诊断都有 subject、原因和修正，行列不小于 1，除 design.missing-system-file 外都位于给出的某个文件中、行不超过这个文件的行数" {
  about loadSources, loadSourcesForExploration
  forall broken: Broken generated "rejects-broken-design、contract-rejects-invalid-relations、reply-scripts-are-typed、inheritance-rejects-invalid-members 和 codecs-are-typed 五者之一的输入，或者 exploration-discards-stale-recordings、exploration-isolates-orphaned-corpus、exploration-keeps-corpus-errors 和 exploration-keeps-design-errors 四者之一取的源文件，附各文件的行数，不给诊断码"
  call loadSources(broken.sources)
  ensure all(d in result.diagnostics: size(d.subject) > 0 and size(d.message) > 0 and size(d.fix) > 0 and d.position.line >= 1 and d.position.column >= 1 and (d.code == "design.missing-system-file" or d.position.path in broken.lines and d.position.line <= broken.lines[d.position.path]))
  ensure all(d in loadSourcesForExploration(broken.sources).loaded.diagnostics: size(d.subject) > 0 and size(d.message) > 0 and size(d.fix) > 0 and d.position.line >= 1 and d.position.column >= 1 and (d.code == "design.missing-system-file" or d.position.path in broken.lines and d.position.line <= broken.lines[d.position.path]))
}

property load-ignores-file-order postcondition "同一组源文件换一种次序载入，得到的图编码和诊断与按原次序载入的相同" {
  about loadSources
  forall sources: list<syntax.Source> generated "一组随机的源文件：accepts-valid-design 取的合法设计，rejects-broken-design 或 contract-rejects-invalid-relations 取的带一处违规改动的设计，在一至三个文件的随机一行之前插入 @ 而不能解析的合法设计，或者把一个组件文件复制到 design/copy.uc（已有时在 copy 后补 x）的合法设计，没有组件文件的合法设计总是插入 @"
  forall reordered: list<syntax.Source> generated "sources 的一个随机排列"
  call loadSources(sources)
  ensure loadSources(reordered).graph.encode() == result.graph.encode()
  ensure loadSources(reordered).diagnostics == result.diagnostics
}

property contract-graph-is-declared postcondition "合法的黑盒契约载入后没有诊断，节点恰是列出的标识，边恰是不重复地列出的种类、起点和终点，性质的 symbols、limits、observe、requires、changes、call 和 ensure 属性逐条保留子句，具名观察成为带 type 和 inputs 的 observation 节点，子句引用的类型和操作成为带 clauses 属性的 refers 边，子句访问了字段时边另带 paths 属性" {
  about loadSources
  forall sample: Sample generated "一个系统文件和一个含黑盒契约的组件文件：组件名称取 domain0 至 domain9999，类型、字段、操作、符号、具名观察和性质的名称各从三四个候选中取，金额区间的下界取 1 至 100、上界取下界至 1000，两条前提和六条 ensure 各自或留或去（ensure 至少留一条）、次序随机；附按留下的子句写出的节点、边和属性"
  call loadSources(sample.sources)
  ensure size(result.diagnostics) == 0
  ensure all(id in sample.nodes: result.graph.node(id) != none)
  ensure all(n in result.graph.nodes(): n.id in sample.nodes)
  ensure all(x in sample.edges: count(y in sample.edges: y == x) == 1)
  ensure size(result.graph.edges()) == size(sample.edges)
  ensure all(e in result.graph.edges(): any(x in sample.edges: x.kind == e.kind and x.from == e.from and x.to == e.to))
  ensure all(id in sample.attrs: result.graph.node(id).attrs == sample.attrs[id])
  ensure all(x in sample.edgeAttrs: any(e in result.graph.edges(): e.kind == x.kind and e.from == x.from and e.to == x.to and e.attrs == x.attrs))
}

property returned-value-contract-is-declared postcondition "ensure 访问被测调用返回值 result 的字段和纯操作的黑盒契约载入后没有诊断，节点恰是列出的标识，边恰是不重复地列出的种类、起点和终点，性质的 symbols、call 和 ensure 属性保留子句，result 上访问的类型和操作成为带 clauses 属性的 refers 边，访问的字段记在 paths 属性中" {
  about loadSources
  forall sample: Sample generated "一个系统文件和一个组件文件：组件名称取 result0 至 result9999，类型、字段、纯操作、被测操作、符号和性质的名称各从三个候选中取，黑盒契约调用被测操作，访问 result 上的纯操作和访问 result 的字段的两条 ensure 各自或留或去（至少留一条）、次序随机；附预期的节点、边和属性"
  call loadSources(sample.sources)
  ensure size(result.diagnostics) == 0
  ensure all(id in sample.nodes: result.graph.node(id) != none)
  ensure all(n in result.graph.nodes(): n.id in sample.nodes)
  ensure all(x in sample.edges: count(y in sample.edges: y == x) == 1)
  ensure size(result.graph.edges()) == size(sample.edges)
  ensure all(e in result.graph.edges(): any(x in sample.edges: x.kind == e.kind and x.from == e.from and x.to == e.to))
  ensure all(id in sample.attrs: result.graph.node(id).attrs == sample.attrs[id])
  ensure all(x in sample.edgeAttrs: any(e in result.graph.edges(): e.kind == x.kind and e.from == x.from and e.to == x.to and e.attrs == x.attrs))
}

property contract-rejects-invalid-relations postcondition "黑盒契约的前提、变化路径、具名观察、about、调用序列或表达式有一处不合规则时，载入在出错的文件给出对应的诊断码，给出行或 subject 时诊断的行或 subject 也相符" {
  about loadSources
  forall broken: Broken generated "与 contract-graph-is-declared 取法相同、留下第一条前提和 ensure result 等于金额并把它们排在最前的黑盒契约，做十三种改动之一：前提类型不符或用到 result，changes 改成观察范围之外的路径，具名观察的 from 列出未声明或重复的符号、观察类型是调用，about 不含被测的操作，多一个调用序列，被测调用的实参类型不符，ensure 调用非纯操作、对整数取下标、误用 old 或约束变量与符号同名；附组件文件、应得的诊断码和行，about 与调用序列的改动落在 call 一行"
  call loadSources(broken.sources)
  ensure all(c in broken.codes: any(d in result.diagnostics: d.code == c and d.position.path == broken.path and (broken.line == none or d.position.line == broken.line) and (broken.subject == none or d.subject == broken.subject)))
}

property valid-reply-scripts-load postcondition "成功应答和声明错误的子类型的应答、接口字段各操作的应答都合乎类型时，载入没有诊断" {
  about loadSources
  forall sources: list<syntax.Source> generated "一个系统文件（外部系统的操作、错误和错误的子类型）和一个组件文件（接口、带接口字段的记录和一条带用例的性质），用例给出一至五项成功应答的脚本，或一项错误的子类型带字段的应答，组件、类型、操作、符号、字段和用例的名称各从三个候选中取"
  call loadSources(sources)
  ensure size(result.diagnostics) == 0
}

property reply-scripts-are-typed postcondition "应答脚本有一处不合 model.md §10 第 23、26 条时，载入在出错的文件给出 design.invalid-replies-type 或 design.value-type-mismatch，给出行或 subject 时诊断的行或 subject 也相符" {
  about loadSources
  forall broken: Broken generated "与 valid-reply-scripts-load 取法相同的设计做十一种改动之一：目标不是外部系统的操作，脚本为空，返回值类型不合，抛出未声明的错误，错误缺字段，同一目标两个脚本，replies 嵌在 list 中或用作操作的参数，接口字段某个操作的应答类型不合或缺少，一项同时有 returns 和 raises；附组件文件和应得的诊断码，不给行和 subject"
  call loadSources(broken.sources)
  ensure all(c in broken.codes: any(d in result.diagnostics: d.code == c and d.position.path == broken.path and (broken.line == none or d.position.line == broken.line) and (broken.subject == none or d.subject == broken.subject)))
}

property inheritance-members-are-resolved postcondition "继承的字段和操作在表达式、限定、用例和调用序列中都能访问时载入没有诊断，类型的 fields 属性只列自己声明的字段，类型引用的调用序列在 symbols 属性中保留这个类型，被测调用的 refers 边指向实际声明的操作，子类型的同名操作遮蔽父类型的" {
  about loadSources
  forall expected: Expected generated "一个系统文件和一个组件文件：子类型继承父类型的字段和操作，父类型字段的值取 -20 至 20 的整数，子类型或另声明同名的字段和操作来遮蔽父类型的、或不声明，两条性质在表达式、限定、用例和调用序列中访问继承的成员，组件、类型、字段、操作、符号、用例和性质的名称各从三个候选中取；附两个类型的 fields 属性、调用序列性质的 symbols 属性，以及被测调用指向实际声明的操作的 refers 边"
  call loadSources(expected.sources)
  ensure size(result.diagnostics) == 0
  ensure all(id in expected.attrs: all(k in expected.attrs[id]: result.graph.node(id).attrs[k] == expected.attrs[id][k]))
  ensure all(x in expected.edges: any(e in result.graph.edges(): e.kind == x.kind and e.from == x.from and e.to == x.to))
}

property inheritance-rejects-invalid-members postcondition "用例给出的记录缺继承的字段、继承对象类型的枚举或接口的字段被访问、操作引用的调用序列以子类型为接收者时，载入在出错的文件给出对应的全部诊断码，给出行或 subject 时诊断的行或 subject 也相符" {
  about loadSources
  forall broken: Broken generated "与 inheritance-members-are-resolved 取法相同的设计做三种改动之一：记录用例去掉一个继承的字段，应得 design.value-type-mismatch，或者加一个继承父类型的枚举或接口并在一条新性质的限定和 ensure 中访问它继承的字段，应得 subject 为这条性质的 design.invalid-limit-path 和 design.expression-type，或者调用序列改为引用父类型的操作，应得 design.invalid-call-sequence；附组件文件，不给行"
  call loadSources(broken.sources)
  ensure all(c in broken.codes: any(d in result.diagnostics: d.code == c and d.position.path == broken.path and (broken.line == none or d.position.line == broken.line) and (broken.subject == none or d.subject == broken.subject)))
}

property valid-codecs-load postcondition "codec 的签名合乎 language.md §5.3.1、表示经 optional、list、set、map 或另一个 codec 终止时载入没有诊断，类型的 codec 属性列出编码和解码操作，codecType 属性是表示类型，类型有指向这两个操作的 refers 边" {
  about loadSources
  forall expected: Expected generated "一个系统文件和一个组件文件，其中的类型声明合法的 codec：或者编码成 int（值为奇数时）或记录（值为偶数时）并带 encoded 用例，值取 -1000 至 1000，或者编码成一个记录，它的字段经 optional、list、set、map 或另一个 codec 回到自身，组件、类型、操作、字段和用例的名称各从三个候选中取；附类型的 codec、codecType 属性和指向编码、解码操作的两条 refers 边"
  call loadSources(expected.sources)
  ensure size(result.diagnostics) == 0
  ensure all(id in expected.attrs: all(k in expected.attrs[id]: result.graph.node(id).attrs[k] == expected.attrs[id][k]))
  ensure all(x in expected.edges: any(e in result.graph.edges(): e.kind == x.kind and e.from == x.from and e.to == x.to))
}

property codecs-are-typed postcondition "codec 的签名不对、重复、用于接口、操作没有声明、没有 codec 却用 encoded、表示类型经必填字段或另一个 codec 回到自身、靠继承得到 codec 时，载入在出错的文件给出对应的诊断码，给出行或 subject 时诊断的行或 subject 也相符，broken.absent 列出的节点属性不存在或为空" {
  about loadSources
  forall broken: Broken generated "与 valid-codecs-load 取法相同的设计做一处改动：十四种之一（codec 重复、用于接口、编码操作没有声明、带参数、不是纯操作或声明了错误、解码操作的参数或返回类型不符、解码操作是方法、编码成自身、没有 codec 却用 encoded、encoded 的值类型不符、子类型靠继承得到 codec、经另一个 codec 回到自身），或者表示类型经必填字段直接或间接回到自身；附组件文件和应得的诊断码，后一种另要求类型没有 codecType 属性"
  call loadSources(broken.sources)
  ensure all(c in broken.codes: any(d in result.diagnostics: d.code == c and d.position.path == broken.path and (broken.line == none or d.position.line == broken.line) and (broken.subject == none or d.subject == broken.subject)))
  ensure all(id in broken.absent: all(k in broken.absent[id]: not k in result.graph.node(id).attrs or size(result.graph.node(id).attrs[k]) == 0))
}

property exploration-keeps-valid-recordings postcondition "录制用例都合乎设计时，探索载入和 loadSources 都没有诊断，探索载入不丢弃录制用例、不列出孤立的录制文件，两者的图编码相同，图中有加入的录制用例 target" {
  about loadSourcesForExploration, loadSources
  forall sample: Exploration generated "accepts-valid-design 取的合法设计，加一个录制边界组件（一个类型、一个操作和一条整数符号限定在 0 至 10、带一个声明的用例的性质）和它的录制文件（一个录制用例），用例的值取 0 至 10；录制文件再加一个合法的录制用例作为 target，base 是加入 target 之前的源文件"
  call loadSourcesForExploration(sample.sources)
  ensure size(result.loaded.diagnostics) == 0 and size(result.discarded) == 0 and size(result.orphaned) == 0
  ensure size(loadSources(sample.sources).diagnostics) == 0
  ensure result.loaded.graph.encode() == loadSources(sample.sources).graph.encode()
  ensure result.loaded.graph.node(sample.target) != none
}

property exploration-discards-stale-recordings postcondition "加入的录制用例 target 不再合乎设计时，loadSources 对它给出诊断码 code，探索载入没有诊断和孤立的录制文件，恰丢弃 target，图中没有 target，图编码与载入 base 的相同" {
  about loadSourcesForExploration, loadSources
  forall sample: Exploration generated "与 exploration-keeps-valid-recordings 相同地加入录制边界，再加一个不合设计的录制用例作为 target：多一个未知符号、缺值、重复的值、字符串值，或 -1000 至 -1、11 至 1000 的越界值；code 是对应的诊断码，base 是加入 target 之前的源文件"
  call loadSourcesForExploration(sample.sources)
  ensure any(d in loadSources(sample.sources).diagnostics: d.code == sample.code and d.subject == sample.target)
  ensure size(result.loaded.diagnostics) == 0 and size(result.orphaned) == 0
  ensure size(result.discarded) == 1 and sample.target in result.discarded
  ensure result.loaded.graph.node(sample.target) == none
  ensure result.loaded.graph.encode() == loadSources(sample.base).graph.encode()
}

property exploration-isolates-orphaned-corpus postcondition "录制文件 target 指向的性质或流程不存在时，loadSources 有诊断，探索载入没有诊断，不丢弃录制用例，恰把 target 列为孤立的录制文件，图编码与载入 base 的相同" {
  about loadSourcesForExploration, loadSources
  forall sample: Exploration generated "与 exploration-keeps-valid-recordings 相同地加入录制边界，再按指向的标识在新路径 target 加一个录制文件，它指向不存在的标识，或者设计中（含录制边界组件）的一个组件、类型或操作；base 是加入它之前的源文件"
  call loadSourcesForExploration(sample.sources)
  ensure size(loadSources(sample.sources).diagnostics) > 0
  ensure size(result.loaded.diagnostics) == 0 and size(result.discarded) == 0
  ensure size(result.orphaned) == 1 and sample.target in result.orphaned
  ensure result.loaded.graph.encode() == loadSources(sample.base).graph.encode()
}

property exploration-keeps-corpus-errors postcondition "录制文件放错路径或录制用例重名时，探索载入照常给出诊断码 code，不丢弃录制用例，也不列出孤立的录制文件" {
  about loadSourcesForExploration
  forall sample: Exploration generated "与 exploration-keeps-valid-recordings 相同地加入录制边界，再在 corpus/wrong-location.uc 加一个指向不存在的标识或者一个组件、类型或操作的录制文件（code 为 design.corpus-path-mismatch），或在录制文件中重复已有的录制用例（code 为 design.duplicate-name）"
  call loadSourcesForExploration(sample.sources)
  ensure any(d in result.loaded.diagnostics: d.code == sample.code)
  ensure size(result.discarded) == 0 and size(result.orphaned) == 0
}

property exploration-keeps-design-errors postcondition "设计中声明的用例超出限定或 about 引用不存在时，loadSources 和探索载入都有诊断，探索载入不丢弃录制用例，也不列出孤立的录制文件" {
  about loadSourcesForExploration, loadSources
  forall sources: list<syntax.Source> generated "与 exploration-keeps-valid-recordings 相同地加入录制边界，再把声明的用例的值改为限定之外的 11，或在性质的 about 中加一个不存在的操作"
  call loadSourcesForExploration(sources)
  ensure size(loadSources(sources).diagnostics) > 0
  ensure size(result.loaded.diagnostics) > 0
  ensure size(result.discarded) == 0 and size(result.orphaned) == 0
}

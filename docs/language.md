# unicode 设计语言

本文是 unicode 设计语言（下称本语言）的参考。本语言书写一个系统的设计：系统由哪些组件构成，各组件有哪些类型和操作，组件之间按什么流程协作，系统必须满足哪些性质。本语言的程序编译成图的设计层（[model.md](model.md)）；工具链再并入从代码抽取的代码层，检查两者是否一致。

本语言的源文件有两种：设计文件和录制文件。此外，代码中的**绑定**声明一个代码元素落实哪个设计节点。三者都只有一种写法，工具链不接受其他写法。

本文依次定义源文件（§1）、词法（§2）、语法（§3）、语义（§4）、编译（§5）和绑定（§6），最后给出一个完整的例子（§7）。

## 1 文件

规则按前缀分组：F（源文件）、L（词法）、G（语法）、P（排版）、R（引用）。违反 F、L、G、R 规则是错误，`unicode fmt` 不改动这样的文件；违反 P 规则的文件，`unicode fmt` 改写成规范格式，`unicode check` 报 `cli.not-canonical`。理由见附录 A。

- **F1** 设计文件在仓库根目录的 `design/` 中，扩展名为 `.uc`；`design/` 中没有其他文件。
- **F2** `design/system.uc` 是系统文件，声明系统、记法、各语言的源码根目录和参与者。其余每个设计文件声明一个组件，路径由组件的标识决定：组件 `order` 在 `design/order.uc`，组件 `order.pricing` 在 `design/order/pricing.uc`。顶层组件不能叫 `system`。
- **F3** 录制文件在仓库根目录的 `corpus/` 中，扩展名为 `.uc`。每个文件存放一条性质或一个流程的录制用例，路径由它的标识决定：`order.total-is-sum` 的录制用例在 `corpus/order/total-is-sum.uc`。录制文件只由 `unicode test --record` 写入；`corpus/` 中没有其他文件。
- **F4** 文件的内容与 `unicode fmt` 的输出逐字节相同。

## 2 词法

- **L1** 文件是 UTF-8 文本。空白只有空格（U+0020）和 LF；制表符、CR、其他控制字符和非 UTF-8 的字节都是错误。记号之间的空格个数、缩进、行尾空格和空行属于排版（P 规则）。
- **L2** 注释独占一行，以 `//` 开头；`//` 之后的文字原样保留，只去掉行尾的空格。
- **L3** 字符串用双引号括起，写在一行之内。转义只有 `\"`、`\\`、`\n`（换行）和 `\t`（制表符）四种。
- **L4** 摘要字符串非空，首尾没有空白（按 Unicode 判断），不含换行和制表符。
- **L5** 数是十进制：`Int` 为 `-?[0-9]+`，`Number` 为 `-?[0-9]+(\.[0-9]+)?`。规范写法中，整数部分没有多余的前导零，没有 `-0`；小数部分去掉末尾的零，至少留一位。例如 `007` 写作 `7`，`1.50` 写作 `1.5`，`1.0` 不变。
- **L6** 名称有三种写法：

  | 写法 | 形式 | 用于 |
  | --- | --- | --- |
  | 短横线 | `[a-z][a-z0-9]*(-[a-z0-9]+)*` | 系统、组件的一段、参与者、流程、性质、用例、标签 |
  | 大驼峰 | `[A-Z][A-Za-z0-9]*` | 类型 |
  | 小驼峰 | `[a-z][A-Za-z0-9]*` | 操作、字段、参数、枚举值、符号 |

- **L7** `true`、`false` 和 `none` 是保留字，不能用作小驼峰的名称；其余关键字都可以用作名称。

## 3 语法

声明一律按"关键字、名称、形式、摘要"的次序书写，例如 `type Order object "…"`、`property total-is-sum invariant "…"`。

```ebnf
SystemFile    = SystemDecl { NotationDecl } { LanguageDecl } { ActorDecl } .
SystemDecl    = "system" Kebab String NL .
NotationDecl  = "notation" ( "ddd" | "c4" ) NL .
LanguageDecl  = "language" ( "go" | "python" ) "{" NL Root { Root } "}" NL .
Root          = "root" String NL .
ActorDecl     = "actor" Kebab "person" String NL
              | "actor" Kebab "external" String [ "{" NL { TypeDecl } { OpDecl } "}" ] NL .

ComponentFile = ComponentDecl { TypeDecl } { OpDecl } { FlowDecl } { PropertyDecl } .
ComponentDecl = "component" Path String "{" NL
                { Label }
                [ "focus" ( "core" | "support" | "utility" ) NL ]
                "paths" String { "," String } NL
                [ "uses" Ref { "," Ref } NL ]
                "}" NL .
Label         = "label" Kebab NL .

TypeDecl      = "type" Upper TypeForm String [ "{" NL { Label } { Member } "}" ] NL .
TypeForm      = "object" | "record" | "interface" | "enum" | "error" | "event" .
Member        = "extends" TypeRef NL | "implements" TypeRef NL
              | "field" Lower ":" TypeExpr NL | "value" Lower NL | OpDecl .
OpDecl        = "op" Lower "(" [ Param { "," Param } ] ")" [ "->" TypeExpr ]
                [ "raises" TypeRef { "," TypeRef } ] [ "handles" TypeRef ] [ String ] NL .
Param         = Lower ":" TypeExpr .
TypeExpr      = Builtin | TypeRef
              | ( "list" | "set" | "optional" ) "<" TypeExpr ">"
              | "map" "<" TypeExpr "," TypeExpr ">"
              | "call" "<" Ref { "," Ref } ">" .
Builtin       = "bool" | "int" | "float" | "string" | "bytes" | "time" | "duration" .

FlowDecl      = "flow" Kebab String "{" NL { Symbol } { Limit } { Step } { CaseDecl } "}" NL .
Step          = Call { OnBlock } | Send | Emit | Async | Fragment .
Call          = Participant "->" OpRef Args [ "{" NL { Step } "}" ] NL .
Send          = Participant "->>" OpRef Args NL .
Emit          = Participant "emits" TypeRef Args NL .
Args          = "(" [ Lower { "," Lower } ] ")" .
OnBlock       = "on" TypeRef "{" NL { Step } "}" NL .
Async         = "async" OpRef "{" NL { Step } "}" NL .
Fragment      = "alt" String "{" NL { Step } { "}" "else" String "{" NL { Step } } "}" NL
              | ( "opt" | "loop" ) String "{" NL { Step } "}" NL .

PropertyDecl  = "property" Kebab PropertyForm String "{" NL
                "about" Ref { "," Ref } NL
                { Symbol } { Limit } { CaseDecl }
                "}" NL .
PropertyForm  = "invariant" | "postcondition" | "relation" | "transition" .
Symbol        = "forall" Lower ":" TypeExpr [ "generated" String ] NL .
Limit         = "where" SymbolPath ( "in" Number ".." Number
                                   | "in" "{" Lower { "," Lower } "}"
                                   | "size" Int ".." Int ) NL .
SymbolPath    = Lower { "." Lower } .
CaseDecl      = "case" Kebab [ String ] "{" NL { Lower "=" Value NL } "}" NL .
Value         = Number | String | "true" | "false" | "none" | Lower
              | Lower "(" [ Lower ":" Value { "," Lower ":" Value } [ "," ] ] ")"
              | "[" [ Value { "," Value } [ "," ] ] "]"
              | "{" [ Entry { "," Entry } [ "," ] ] "}" .
Entry         = ( Lower | String | Number ) ":" Value .

CorpusFile    = "corpus" Ref NL { RecordedCase } .
RecordedCase  = "case" Kebab ( "coverage" | "counterexample" ) "{" NL { Lower "=" Value NL } "}" NL .

Path          = Kebab { "." Kebab } .
TypeRef       = { Kebab "." } Upper .
OpRef         = { Kebab "." } [ Upper "." ] Lower .
Participant   = { Kebab "." } ( Kebab | Upper ) .
Ref           = Name { "." Name } .
Name          = Kebab | Upper | Lower .
NL            = 换行 { 换行 } .
```

**次序与结构**

- **G1** 组件文件中的声明按组件、类型、操作、流程、性质分组，各组内部保持作者写的次序。
- **G2** 类型的成员按 `label`、`extends`、`implements`、`field` 或 `value`、`op` 的次序排列。`value` 只出现在 `enum` 中；`field` 不出现在 `enum` 和 `interface` 中。
- **G3** 流程的成员按 `forall`、`where`、步骤、`case` 的次序排列；性质的成员按 `about`、`forall`、`where`、`case` 的次序排列。
- **G4** `{` 总在行尾，`}` 总在行首；同一行中 `{` 之后或 `}` 之前不能有别的内容。`alt` 的分支写作一行 `} else "标签" {`。
- **G5** 在流程中，一行的第一个词之后是 `->`、`->>` 或 `.`，或是 `emits` 再接一个大驼峰的词或带 `.` 的引用时，这一行是一个步骤；否则第一个词是关键字。
- **G6** 调用后面的 `{ … }` 是这次调用的子步骤。`on` 紧跟在一个调用的 `}` 之后，调用与 `on` 之间的注释属于 `on`。

**必需项与取值**

- **G7** 系统、参与者、组件、类型、流程和性质必须有摘要；操作和用例的摘要可以省略。`generated` 之后的取值域说明，以及 `alt`、`opt`、`loop`、`else` 的标签，都遵守 L4；`paths` 和 `root` 的字符串是普通字符串。
- **G8** 顶层组件必须写 `focus`；子组件省略 `focus` 时，取父组件的关注度。
- **G9** `uses` 只指向容器和外部系统，不指向人。
- **G10** 区间的下界不大于上界。
- **G11** 一个值从 `=` 所在的行开始，只在开括号之后、逗号前后和闭括号之前换行；值中没有注释。枚举子集 `in {a, b}` 写在一行之内。结尾的逗号只出现在值中。

标签的约束见 [model.md](model.md) §10 第 5 条。

**排版**

- **P1** `NL` 是一个或多个换行；空行属于排版。
- **P2** 顶层声明之间恰好空一行，块内不空行，文件末尾恰有一个换行。
- **P3** 逗号之后一个空格；`:` 之前没有空格、之后一个空格；`->`、`->>`、`=`、`emits` 两边各一个空格；`<` `>` `(` `)` `[` `]` 和值的 `{` `}` 内侧没有空格；区间写作 `1..10`。
- **P4** 可选的块为空时（类型、外部系统、调用的子步骤）不写花括号；必需的块为空时写成 `{` 和 `}` 两行。
- **P5** 注释的归属：一组注释（其间可以有空行）属于紧随其后的声明、成员或步骤，紧贴在它之上，缩进与它相同；`}` 之前的注释留在块的末尾；`} else` 之前的注释属于前一个分支；最后一个声明之后的注释，空一行之后打印。
- **P6** 值的宽度按显示宽度计（东亚的宽字符算两列），包括缩进、前面的 `name = ` 或 `key: `，以及结尾的逗号。值能在 100 列之内写完时写在一行，元素之间用逗号分隔，最后一项之后没有逗号；写不完时，每个元素、字段或参数各占一行，缩进一级，每一项之后都有逗号。这条规则逐层适用。不能再拆的值写不完时，仍写在一行。

### 3.1 值

用例中的值按符号的类型解读：

| 类型 | 写法 |
| --- | --- |
| `bool` | `true`、`false` |
| `int`、`float` | 数 |
| `string`、`bytes` | 字符串；`bytes` 取字符串的 UTF-8 编码 |
| `time` | RFC 3339 的字符串，例如 `"2026-11-26T08:00:00Z"` |
| `duration` | 字符串，例如 `"1h30m"` |
| `enum` | 枚举值的名称，例如 `draft` |
| `list<T>`、`set<T>` | `[v1, v2]` |
| `map<K, V>` | `{"k": v}`，键按 K 的写法 |
| `optional<T>` | `none` 或 T 的值 |
| `record`、`object`、`event` | `{field: v, …}`，给出全部字段 |
| `call<…>` | `操作名(参数: v, …)`，给出全部参数，例如 `reserve(items: [])` |

## 4 语义

本语言的一个**程序**是一个仓库在一个提交上 `design/` 与 `corpus/` 中的全部文件。程序的含义是图的设计层：一组节点和边，其种类和形式见 [model.md](model.md) §3–§4，标识见 [model.md](model.md) §7。所有文件共享一个命名空间，没有导入语句：引用按 §4.1 的规则解析到任一文件中声明的节点。文件的次序不影响程序的含义。

### 4.1 引用

- **R1** 以小写段开头、多于一段的引用，是从根写起的绝对引用，例如 `inventory.Stock`、`order.pricing.Price`、`payment-gateway.charge`。
- **R2** 以类型开头的引用在当前组件中查找，例如 `Order`、`Order.total`。
- **R3** 只有一段的小写引用，先在当前组件中查找，再在根下查找，例如操作 `parseCart`、参与者 `customer`。

### 4.2 声明与节点

每条声明产生一个节点，并由一条 `contains` 边挂在包含它的声明之下。组件文件中的顶层声明属于该组件；外部系统中的类型和操作属于该外部系统，例如 `payment-gateway.charge`。

| 声明 | 节点的种类 / 形式 | 标识（以组件 `order` 为例） |
| --- | --- | --- |
| `system shop` | `container` / `system` | `shop` |
| `actor customer person` | `actor` / `person` | `customer` |
| `actor payment-gateway external` | `actor` / `external` | `payment-gateway` |
| `component order` | `container` / `component` | `order` |
| `type Order object` | `type` / 声明中的形式 | `order.Order` |
| 类型中的 `op total` | `op` / `op` | `order.Order.total` |
| 组件中的 `op parseCart` | `op` / `op` | `order.parseCart` |
| `flow submit-order` | `flow` / `flow` | `order.submit-order` |
| 流程中的步骤 | `step` / `call`、`send`、`emit`、`async`、`alt`、`opt`、`loop`、`on` | 流程的标识后接 `#` 和按文本先后的序号，如 `order.submit-order#1` |
| `property total-is-sum invariant` | `property` / 声明中的形式 | `order.total-is-sum` |
| 设计文件中的 `case empty` | `case` / `declared` | `order.total-is-sum.empty` |
| 录制文件中的 `case r-3f9a2c1d coverage` | `case` / `recorded` | `order.total-is-sum.r-3f9a2c1d` |

其余构造不产生节点，而是成为节点的属性：

- `notation` 和 `language`：系统允许的记法，以及各语言的源码根目录；
- `label`、`focus`、`paths`：容器或类型的标签、关注度和拥有的路径；
- `field`、`value`：类型的字段和枚举值；
- `forall`、`where`：性质或流程的符号和限定；
- 用例中的赋值：用例对各符号的取值，按 §3.1 解读，并且必须落在限定之内；
- 摘要：节点的摘要。性质的摘要就是它的陈述。

字符串，即摘要、性质的陈述、生成符号的取值域说明、`alt`、`opt`、`loop` 和 `else` 的标签，不参与约束的检查。它们计入节点的内容哈希，所以改动会出现在快照的比较和评审中（[model.md](model.md) §8）。性质的可执行含义由它的谓词给出；陈述与谓词是否一致由评审判断。

### 4.3 子句与边

| 构造 | 产生的边 |
| --- | --- |
| 组件中的 `uses R` | `uses` 组件 → R |
| `extends T`、`implements T` | `extends`、`implements` 类型 → T |
| `field x: E` | E 中每个命名类型 T：`refers` 类型 → T |
| 操作的参数、返回值和 `raises` | 其中每个命名类型 T：`refers` 操作 → T |
| `handles E` | `subscribes` 操作 → E |
| `forall x: E` | E 中每个命名类型 T：`refers` 性质或流程 → T |
| `about R` | `about` 性质 → R |
| `P -> R(…)`、`P ->> R(…)` | `call`、`send` 步骤；`calls` 步骤 → R，`from` 步骤 → P |
| `P emits E(…)` | `emit` 步骤；`emits` 步骤 → E，`from` 步骤 → P |
| `async R { … }` | `async` 步骤；`calls` 步骤 → R |
| `on E { … }` | `on` 步骤；`handles` 步骤 → E |

`owns`、`depends`、`traces`、`reaches` 等边不由本语言写出，在编译的链接阶段推导得到（§5.4）。

## 5 编译

编译把一个程序变成图的设计层，再与代码层链接成完整的图。前端（§5.1–§5.3）只读设计文件和录制文件；链接（§5.4）还读代码。

```
路径   ─分类─▶ 文件种类
源文本 ─词法分析─▶ 记号 ─语法分析─▶ 语法树 ─静态规则─▶ 合法的语法树 ─打印─▶ 规范文本
全部合法的语法树 ─载入─▶ 设计层 ─链接─▶ 图、诊断与状态
```

| 阶段 | 实现 | 输入 → 输出 | 诊断 |
| --- | --- | --- | --- |
| 分类 | `syntax.KindOf` | 路径 → 文件种类 | `cli.not-a-design-file` |
| 词法分析 | `internal/syntax/lex.go` | 字节 → 记号 | `syntax.tab`、`syntax.control-character`、`syntax.invalid-escape` 等 |
| 语法分析 | `syntax.Parse`，基于 participle | 记号 → 语法树 `syntax.File` | `syntax.unexpected-token` 等，每个文件至多一条 |
| 静态规则 | `internal/syntax/check.go` | 语法树 → 合法的语法树 | `syntax.declaration-order`、`syntax.invalid-name` 等 |
| 规范格式 | `internal/syntax/print.go` | 语法树 → 规范文本 | `cli.not-canonical` |
| 载入 | `design.Load` | 全部合法的语法树 → 设计层 | `design.*` |
| 链接 | 语言后端、`graph`、`check` | 设计层与代码 → 图与状态 | `cli.extract-failed`、`graph.*`、`check.*` |

### 5.1 分类、词法分析与语法分析

- **分类**：路径决定文件的种类，种类决定使用哪个起始符号。`design/system.uc` 是系统文件，`design/` 中的其他 `.uc` 文件是组件文件，`corpus/` 中的 `.uc` 文件是录制文件；其他路径不是本语言的源文件。
- **词法分析**：先拒绝 §2 禁止的字符，即制表符、CR、其他控制字符和非 UTF-8 的字节，每处一条诊断；再切分记号：注释、字符串、数、名称、`->>`、`->`、`..` 和标点。字符串和注释各自整体作为一个记号，因此不会被当作关键字。
- **语法分析**：§3 的文法以 participle 的结构体标签写在 `internal/syntax/grammar.go` 中，与 §3 的产生式逐条对应。分析得到语法树 `syntax.File`；遇到第一个不合文法的记号即停止，报告一条诊断。

### 5.2 静态规则与规范格式

- **静态规则**：文法表达不了的 L、G 规则在语法树上检查，例如声明和成员的次序、名称的写法、保留字、摘要的形式、同一块中重复的成员。违反的规则全部报告。语法树只有在没有任何诊断时才进入下一阶段。
- **规范格式**：打印器把语法树写成规范文本，它实现 P 规则。`unicode fmt` 用它改写文件；`unicode check` 把它与源文本逐字节比较，不同时报 `cli.not-canonical`。规范文本再经分析和打印，结果不变。

### 5.3 载入

`design.Load` 接收全部合法的语法树，产出设计层和诊断（`design.Loaded`），分三步：

1. **声明**：按固定次序读取声明，为每条声明建一个节点并给出标识。次序是系统文件、由浅到深的组件文件、组件中的成员、录制文件，同组之内按路径。两条声明得到同一标识时，后读到的报 `design.duplicate-name`；读取次序固定，所以哪一条算重复与文件次序无关。
2. **解析引用**：全部节点建好之后，按依赖分批解析引用：先是组件的 `uses`；再是类型和操作的签名、符号的类型；再是符号的取值域，它要读类型的字段；最后是步骤和用例，它们要读前面的全部结果。解析成功即按 §4.3 建边；失败时报 `design.unresolved-reference` 或 `design.wrong-reference-kind`。
3. **设计约束**：检查只需设计就能判断的约束（[design.md](design.md) §5.1），包括用例的值是否属于符号的类型并落在限定之内。

有语法错误的文件不参与载入；其余文件照常载入，指向缺失声明的引用报为未解析。

### 5.4 链接

链接把设计层与代码层合成一张图：每种已配置语言的后端抽取代码层，代码中的绑定（§6）成为 `realizes`、`checks` 和 `generates` 边；合并之后按 [model.md](model.md) §9 推导其余的边和显著性，再按 [model.md](model.md) §10 检查约束，得到诊断和每个设计节点的状态。步骤、缓存和快照见 [design.md](design.md) §5。相同的程序和代码总是得到逐字节相同的快照。

### 5.5 诊断

每条诊断有诊断码、位置、原因和修正办法。诊断码写作 `<阶段>.<短横线名称>`，阶段为 `cli`、`syntax`、`design`、`graph` 或 `check`。诊断都是错误，没有警告；输出按路径、行、列和诊断码排序。

## 6 绑定

绑定写在代码中声明的正上方，参数都是字面量，语言后端不执行代码也能读出。绑定只有三种：

- **`realizes <标识>`**：写在类型或自由函数上，表示它落实这个类型，或容器、外部系统直接提供的这个操作。落实外部系统的操作的，是调用该外部系统的客户端函数。方法上不写绑定；方法按名称映射对应到它所在类型落实的那个类型的操作（[design.md](design.md) §4）。
- **`checks <标识>`**：写在谓词上，表示它检查这条性质或这个流程。谓词的参数就是性质或流程的符号；驱动由 `gen` 生成（[design.md](design.md) §6.2）。一个函数至多带一个 `checks`。
- **`generates <标识>`**：写在生成器函数上，标识是性质或流程的标识后接 `.` 和一个生成的符号的名称，表示它给出这个符号的生成器。同一个生成器可以用于几个符号，每个符号写一行 `generates`。

### 6.1 Go

绑定是声明的文档注释中的指令，独占一行，写作 `//uc:` 后接动词、一个空格和标识：

```go
//uc:realizes order.OrderService
type OrderService struct {
	stock    inventory.Stock
	payments payment.Payments
	orders   OrderRepository
}

//uc:realizes payment-gateway.charge
func Charge(amount int) (string, error) {
	// 调用外部支付网关
}

//uc:checks order.total-is-sum
func checkTotalIsSum(t rapid.TB, order Order) {
	// 断言 order.Total() 等于各行数量乘单价之和
}

//uc:generates syntax.format-is-stable.sample
func genSample() *rapid.Generator[Sample] {
	// 生成合法的语法树，再以随意的空白打印成文本
}
```

### 6.2 Python

绑定是 `uc` 包的装饰器。模块只能写作 `import uc`，不能取别名，也不能用 `from uc import`。装饰器只能写作 `@uc.realizes("<标识>")`、`@uc.checks("<标识>")` 或 `@uc.generates("<标识>")`，只有一个字符串字面量参数：

```python
import uc

@uc.realizes("order.OrderService")
class OrderService:
    def submit(self, cart: Cart) -> Order: ...
    def on_paid(self, event: Paid) -> None: ...

@uc.checks("order.total-is-sum")
def check_total_is_sum(order: Order) -> None:
    expected = sum(line.quantity * line.unit_price for line in order.lines)
    assert order.total() == expected, "订单总额不等于各行数量乘单价之和"
```

`uc` 包只依赖标准库。它的装饰器原样返回被装饰的对象，只在对象上记下标识，供运行时核对流程的调用顺序时使用。

## 7 例子

一个网上商店，下面是它的系统文件、库存、支付和订单三个组件，以及一个录制文件。

```
// design/system.uc
system shop "网上商店：浏览商品、下单、支付"

notation ddd

language python {
  root "src"
}

actor customer person "在商店下单的人"

actor payment-gateway external "外部支付网关" {
  type Rejected error "网关拒绝扣款"
  op charge(amount: int) -> string raises Rejected "扣款，返回网关的流水号"
}
```

```
// design/inventory.uc
component inventory "库存：各商品的可用数量与预留" {
  label bounded-context
  focus support
  paths "src/shop/inventory/**"
}

type StockItem record "一种商品及其数量" {
  label value
  field sku: string
  field quantity: int
}

type OutOfStock error "某种商品的可用数量不足"

type Stock object "各商品的可用数量" {
  label aggregate
  field available: map<string, int>
  op reserve(items: list<StockItem>) raises OutOfStock "预留数量；任何一项不足时都不预留"
  op release(items: list<StockItem>) "释放此前预留的数量"
}

property available-never-negative invariant "任意一串预留和释放之后，每种商品的可用数量都不小于 0" {
  about Stock.reserve, Stock.release
  forall stock: Stock
  forall steps: list<call<Stock>>
  where stock.available size 0..5
  where stock.available in 0..100
  where steps size 0..30
  where steps.reserve.items.quantity in 1..10
  where steps.release.items.quantity in 1..10
  case reserve-then-release {
    stock = {available: {"a": 5}}
    steps = [reserve(items: [{sku: "a", quantity: 3}]), release(items: [{sku: "a", quantity: 3}])]
  }
}
```

```
// design/payment.uc
component payment "支付：经外部网关扣款" {
  focus support
  paths "src/shop/payment/**"
  uses payment-gateway
}

type Declined error "扣款被拒"

type Paid event "一笔订单已扣款" {
  field orderId: string
  field amount: int
}

type Payments object "扣款" {
  op charge(orderId: string, amount: int) raises Declined "经网关扣款；成功后发布 Paid"
}
```

```
// design/order.uc
component order "订单：从购物车生成订单，并管理订单的状态" {
  label bounded-context
  focus core
  paths "src/shop/order/**"
  uses inventory, payment
}

type OrderLine record "订单中的一行：商品、数量和单价" {
  label value
  field sku: string
  field quantity: int
  field unitPrice: int
}

type OrderStatus enum "订单的状态" {
  value draft
  value paid
  value canceled
}

type Order object "一笔订单：行项目和状态，总额由行项目算出" {
  label aggregate
  field id: string
  field lines: list<OrderLine>
  field status: OrderStatus
  op total() -> int "各行数量乘单价之和，单位为分"
}

type Cart record "顾客提交的购物车" {
  label value
  field lines: list<OrderLine>
}

type EmptyCart error "购物车为空"

type OrderRepository interface "订单的持久化" {
  label repository
  op save(order: Order)
}

type OrderService object "下单：预留库存、保存订单、扣款" {
  label domain-service
  op submit(cart: Cart) -> Order raises EmptyCart, inventory.OutOfStock, payment.Declined "从购物车生成订单"
  op onPaid(event: payment.Paid) handles payment.Paid "扣款成功后把订单记为已支付"
}

flow submit-order "顾客提交购物车：预留库存、保存订单并扣款；扣款成功后订单记为已支付" {
  forall cart: Cart
  where cart.lines size 1..10
  where cart.lines.quantity in 1..5
  where cart.lines.unitPrice in 1..100000
  customer -> OrderService.submit(cart) {
    OrderService -> inventory.Stock.reserve(items)
    OrderService -> OrderRepository.save(order)
    OrderService -> payment.Payments.charge(orderId, amount) {
      payment.Payments -> payment-gateway.charge(amount)
      payment.Payments emits payment.Paid(orderId, amount)
    }
    on payment.Declined {
      OrderService -> inventory.Stock.release(items)
    }
  }
  async OrderService.onPaid {
    OrderService -> OrderRepository.save(order)
  }
}

property total-is-sum invariant "订单总额等于各行数量乘单价之和" {
  about Order.total
  forall order: Order
  where order.lines size 0..20
  where order.lines.quantity in 1..99
  where order.lines.unitPrice in 0..1000000
  case empty "没有行项目时总额为 0" {
    order = {id: "o1", lines: [], status: draft}
  }
  case two-lines {
    order = {
      id: "o2",
      lines: [{sku: "a", quantity: 2, unitPrice: 300}, {sku: "b", quantity: 1, unitPrice: 50}],
      status: draft,
    }
  }
}

property declined-keeps-stock postcondition "扣款被拒时，提交前后各商品的可用数量相同" {
  about OrderService.submit
  forall cart: Cart
  where cart.lines size 1..10
}
```

```
// corpus/order/total-is-sum.uc
corpus order.total-is-sum

case r-3f9a2c1d coverage {
  order = {id: "", lines: [{sku: "", quantity: 99, unitPrice: 1000000}], status: canceled}
}
```

从这些文件，各种记法分别取其中的一部分（视图的定义见 [design.md](design.md) §7）：

- **DDD**：核心的 `order` 展开，支撑的 `inventory` 和 `payment` 默认各折叠成一个节点，点开后才显示其中的类型。`order` 中的聚合 `Order` 由值对象 `OrderLine` 组成，领域服务 `OrderService` 和仓储 `OrderRepository` 围绕着它；`Paid` 是领域事件。
- **UML 类图**：各个类型及其字段和操作。`refers` 边来自字段类型（`Order` → `OrderLine`、`OrderStatus`）和操作签名。类图不需要标签。
- **时序图和活动图**：同一个流程 `submit-order` 的两种画法。时序图中，`submit` 的激活里嵌着 `charge` 的激活，`charge` 里又有对外部系统 `payment-gateway` 的调用和对 `Paid` 的发布；之后 `onPaid` 异步地被激活。活动图把 `on payment.Declined` 画成扣款之后的一个分支。
- **事件图**：`Paid` 由 `Payments` 发布，被 `OrderService.onPaid` 订阅。
- **性质视图**：`total-is-sum` 约束 `Order.total`，带一个符号 `order`、三条限定、两个声明的用例和一个录制的用例。它的检查是谓词 `check_total_is_sum`，驱动由 `gen` 生成，视图中还显示这些用例覆盖到的代码。

写好代码之后，`unicode check` 检查的内容包括：

- `src/shop/order/` 下的每个文件归属于 `order`；`order` 的代码只导入 `inventory` 和 `payment` 的包；只有 `payment` 调用落实 `payment-gateway.charge` 的客户端。
- `OrderService` 由一个带绑定的类落实，它的 `submit` 和 `on_paid` 方法签名与设计一致。
- 流程中的每次调用都能在调用图中找到：`submit` 的方法能到达 `reserve`、`save` 和 `charge`，`charge` 的方法能到达网关的客户端；`charge` 的方法引用了落实 `Paid` 的类型。
- 每条性质和每个流程都有一个谓词；声明的和录制的用例都符合符号的类型和限定；代码中没有手写的测试；生成文件与当前的设计一致。

## 附录 A 理由

- **只有一种写法（F4）。** 规范格式让同一个设计只有一种文本，评审时的差异就是设计的差异；`fmt` 只改排版（P 规则），不替人改次序和结构（G 规则），因为次序是作者的阅读顺序。
- **路径由标识决定（F2、F3）。** 从标识能直接找到文件，反之亦然；顶层组件因此不能与系统文件同名。
- **字符串在一行之内（L3）。** 每行恰是一个声明、成员或步骤，按行比较即可看出改动；多行文本用 `\n` 写在一行之内。
- **名称的写法由种类决定（L6）。** 看到名称就知道种类，也让名称能与各语言的代码名称双向映射（[model.md](model.md) §10 第 30 条）。
- **关键字可以用作名称（L7、G5）。** 步骤由行首的形状识别，参与者和符号因此可以与关键字同名；只有 `true`、`false`、`none` 会与值混淆，所以保留。
- **组内保持作者的次序（G1）。** 分组固定了查找的位置，组内的次序留给作者安排阅读顺序。

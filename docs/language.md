# unicode 设计语言

本文是 unicode 设计语言（下称本语言）的参考。本语言书写一个系统的设计：系统由哪些组件构成，各组件有哪些类型和操作，组件之间按什么流程协作，系统必须满足哪些性质。本语言的程序编译成图的设计层（[model.md](model.md)）；工具链再并入从代码抽取的代码层，检查两者是否一致。

本语言的源文件有两种：设计文件和录制文件。此外，代码中的**绑定**声明一个代码元素落实哪个设计节点。三者都只有一种写法，工具链不接受其他写法。

本文依次定义源文件（§1）、词法（§2）、语法（§3）、语义（§4）、编译（§5）和绑定（§6），最后给出一个完整的例子（§7）。

## 1 文件

规则按前缀分组：F（源文件）、L（词法）、G（语法）、P（排版）、R（引用）、E（表达式）、B（绑定）。违反 F、L、G、R、B 规则是错误，`unicode fmt` 不改动这样的文件；违反 P 规则的文件，`unicode fmt` 改写成规范格式，`unicode check` 报 `cli.not-canonical`。E 规则中，名称和类型的规则在载入时检查，求值和检查的规则在运行谓词时适用。理由见附录 A。

- **F1** 设计文件在仓库根目录的 `design/` 中，扩展名为 `.uc`；`design/` 中没有其他文件。
- **F2** `design/system.uc` 是系统文件，声明系统、记法、各语言的源码根目录和参与者。其余每个设计文件声明一个组件，路径由组件的标识决定：组件 `order` 在 `design/order.uc`，组件 `order.pricing` 在 `design/order/pricing.uc`。顶层组件不能叫 `system`。
- **F3** 录制文件在仓库根目录的 `corpus/` 中，扩展名为 `.uc`。每个文件存放一条性质或一个流程的录制用例，路径由它的标识决定：`order.total-is-sum` 的录制用例在 `corpus/order/total-is-sum.uc`。录制文件只由 `unicode test --record` 写入；`corpus/` 中没有其他文件。
- **F4** 文件的内容与 `unicode fmt` 的输出逐字节相同。

## 2 词法

- **L1** 文件是 UTF-8 文本。空白只有空格（U+0020）和 LF；制表符、CR、其他控制字符、非 UTF-8 的字节，以及不属于任何记号（§5.1）的字符，都是错误。记号之间的空格个数、缩进、行尾空格和空行属于排版（P 规则）。
- **L2** 注释独占一行，以 `//` 开头；`//` 之后的文字原样保留，只去掉行尾的空格。
- **L3** 字符串用双引号括起，写在一行之内。转义只有 `\"`、`\\`、`\n`（换行）和 `\t`（制表符）四种。
- **L4** 摘要字符串非空，首尾没有空白（按 Unicode 判断），不含换行和制表符。
- **L5** 数是十进制：`Int` 为 `-?[0-9]+`，`Number` 为 `-?[0-9]+(\.[0-9]+)?`。规范写法中，整数部分没有多余的前导零，没有 `-0`；小数部分去掉末尾的零，至少留一位。例如 `007` 写作 `7`，`1.50` 写作 `1.5`，`1.0` 不变。
- **L6** 名称有三种写法：

  | 写法 | 形式 | 用于 |
  | --- | --- | --- |
  | 短横线 | `[a-z][a-z0-9]*(-[a-z0-9]+)*` | 系统、组件的一段、参与者、流程、性质、用例、标签 |
  | 大驼峰 | `[A-Z][A-Za-z0-9]*` | 类型 |
  | 小驼峰 | `[a-z][A-Za-z0-9]*` | 操作、字段、参数、枚举值、符号、具名观察 |

- **L7** `true`、`false` 和 `none` 是保留字，不能用作小驼峰的名称；其余关键字都可以用作名称，G15 列出的除外。

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
              | "codec" OpRef "," OpRef NL
              | "field" Lower ":" TypeExpr NL | "value" Lower NL | OpDecl .
OpDecl        = "op" Lower "(" [ Param { "," Param } ] ")" [ "->" TypeExpr ] [ "pure" ]
                [ "raises" TypeRef { "," TypeRef } ] [ "handles" TypeRef ] [ String ] NL .
Param         = Lower ":" TypeExpr .
TypeExpr      = Builtin | TypeRef
              | ( "list" | "set" | "optional" ) "<" TypeExpr ">"
              | "map" "<" TypeExpr "," TypeExpr ">"
              | "call" "<" Ref { "," Ref } ">"
              | "replies" "<" OpRef ">" .
Builtin       = "bool" | "int" | "float" | "string" | "bytes" | "time" | "duration" .

FlowDecl      = "flow" Kebab String "{" NL { Symbol } { Limit } { Step } [ Within ] { Ensure } [ Manual ] { CaseDecl } "}" NL .
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
                { Symbol } { Limit } { Observe } { Requires } [ Changes ] [ Act ]
                ( Within { Ensure } | Ensure { Ensure } | Manual ) { CaseDecl }
                "}" NL .
Observe       = "observe" ( "none" | ObservePath { "," ObservePath }
                          | Lower ":" TypeExpr "from" Lower { "," Lower } ) NL .
ObservePath   = Lower { "." Lower } .
Requires      = "requires" Expr NL .
Changes       = "changes" ( "none" | StatePath { "," StatePath } ) NL .
StatePath     = Lower { "." Lower | "[" Expr "]" } .
Act           = "call" Expr NL .
Within        = "within" String NL .
Ensure        = "ensure" Expr NL .
Manual        = "manual" String NL .
PropertyForm  = "invariant" | "postcondition" | "relation" | "transition" .
Symbol        = "forall" Lower ":" TypeExpr [ "generated" String ] NL .
Limit         = "where" SymbolPath ( "in" Number ".." Number
                                   | "in" "{" Lower { "," Lower } "}"
                                   | "size" Int ".." Int ) NL .
SymbolPath    = Lower { "." Lower } .
CaseDecl      = "case" Kebab [ String ] "{" NL { Lower "=" Value NL } "}" NL .
Value         = Number | String | "true" | "false" | "none" | Lower
              | "encoded" "(" Value ")"
              | Lower "(" [ Lower ":" Value { "," Lower ":" Value } [ "," ] ] ")"
              | "[" [ Value { "," Value } [ "," ] ] "]"
              | "{" [ Entry { "," Entry } [ "," ] ] "}" .
Entry         = ( Lower | String | Number ) ":" Value .

Expr          = Or [ "implies" Expr ] .
Or            = And { "or" And } .
And           = Not { "and" Not } .
Not           = "not" Not | Comparison .
Comparison    = Sum [ ( "==" | "!=" | "<" | "<=" | ">" | ">=" | "in" ) Sum ] .
Sum           = Product { ( "+" | "-" ) Product } .
Product       = Postfix { ( "*" | "/" ) Postfix } .
Postfix       = Atom { "." Name [ Arguments ] | "[" Expr "]" } .
Arguments     = "(" [ Expr { "," Expr } ] ")" .
Atom          = Number | String | "true" | "false" | "none" | "result"
              | Name [ Arguments ]
              | "old" "(" Expr ")" | "raised" "(" TypeRef ")" | "failure" "(" TypeRef ")"
              | Each "(" Lower "in" Expr ":" Expr ")"
              | "(" Expr ")" .
Each          = "all" | "any" | "count" | "sum" .

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

- **G0** 每个文件合乎 §3 中它的种类的产生式，包括产生式规定的声明次序、块的成员和只出现一次的子句。
- **G1** 组件文件中的声明按组件、类型、操作、流程、性质分组，各组内部保持作者写的次序。
- **G2** 类型的成员按 `label`、`extends`、`implements`、`codec`、`field` 或 `value`、`op` 的次序排列。`value` 只出现在 `enum` 中；`field` 不出现在 `enum` 和 `interface` 中。
- **G3** 流程的成员按 `forall`、`where`、步骤、`within`、`ensure` 或有理由的 `manual`、`case` 的次序排列；性质的成员按 `about`、`forall`、`where`、`observe`、`requires`、`changes`、`call`、`within`、`ensure` 或 `manual`、`case` 的次序排列；流程的 `within` 和 `ensure` 在步骤之后、`case` 之前。
- **G4** `{` 总在行尾，`}` 总在行首；同一行中 `{` 之后或 `}` 之前不能有别的内容。`alt` 的分支写作一行 `} else "标签" {`。
- **G5** 在流程中，一行的第一个词之后是 `->`、`->>` 或 `.`，或是 `emits` 再接一个大驼峰的词或带 `.` 的引用时，这一行是一个步骤；否则第一个词是关键字。
- **G6** 调用后面的 `{ … }` 是这次调用的子步骤。`on` 紧跟在一个调用之后，调用带子步骤时在它的 `}` 之后；一个调用之后可以有几个 `on`。调用与 `on` 之间的注释属于 `on`。

**必需项与取值**

- **G7** 系统、参与者、组件、类型、流程和性质必须有摘要；操作和用例的摘要可以省略。`generated` 之后的取值域说明，以及 `alt`、`opt`、`loop`、`else` 的标签，都遵守 L4；`paths` 和 `root` 的字符串是普通字符串。
- **G8** 顶层组件必须写 `focus`；子组件省略 `focus` 时，取父组件的关注度。
- **G9** `uses` 只指向容器和外部系统，不指向人。
- **G10** 区间的下界不大于上界。
- **G11** 一个值从 `=` 所在的行开始，只在开括号之后、逗号前后和闭括号之前换行；值中没有注释。枚举子集 `in {a, b}` 写在一行之内。结尾的逗号只出现在值中。

**性质的陈述**

- **G12** `manual` 的字符串写明这条性质或流程为什么需要手写谓词，并遵守 L4。
- **G13** 形式为 `postcondition` 且带 `ensure` 或 `within` 的性质恰有一条 `call` 子句，它调用的操作在 `about` 中；其他形式的性质没有 `call` 子句。`within` 只出现在带 `call` 子句的性质和流程中，它的字符串是一个 `duration` 的值（§3.1）。
- **G14** 带 `ensure` 的性质至多有一个类型为 `list<call<R1, …, Rk>>` 的符号。有这样的符号时，R1, …, Rk 中的操作都属于同一个类型 T，性质恰有一个类型为 T 的符号，并且没有 `call` 子句。形式为 `transition` 且带 `ensure` 的性质必须有这样的符号。
- **G15** 表达式的关键字（`implies`、`or`、`and`、`not`、`in`、`old`、`raised`、`failure`、`result`、`all`、`any`、`count`、`sum`、`size`）不能用作符号、具名观察、约束变量，以及容器或外部系统直接提供的操作的名称。
- **G16** 表达式中二元运算符的两边各有一个空格。`a-b` 是一个短横线名称，`a -1` 是一个名称之后跟一个负数，都不是减法。
- **G17** 自动流程（[model.md](model.md) §3.3）的第一个步骤是同步调用；人工流程保留步骤，由手写的谓词执行。
- **G18** `observe` 和 `changes` 成对出现，只用于带 `call`、不带 `manual` 的 `postcondition`。`observe none` 独占观察组，且对应 `changes none`；省略这两组时只检查原有陈述，不产生范围保护的结论。
- **G19** 直接观察路径从输入符号出发，只访问字段；具名观察的 `from` 恰列出适配器接收的输入符号，次序固定，名称不重复。具名观察不依赖其他观察，名称与本性质的符号、观察和用例互不相同。
- **G20** `requires` 的类型是 `bool`，只引用输入符号、调用前观察值、枚举和纯操作，不使用 `old`、`result`、`raised` 或 `failure`。多条前提按文本次序求值，前一条为假就停止。
- **G21** 每条变化路径（E19）位于某个观察根之内，只访问字段或按键访问 `map`。下标只使用调用前可求值的表达式，不使用 `old`、`result`、`raised` 或 `failure`。路径的定位、观察值与差异的规则见 §4.5。

标签的约束见 [model.md](model.md) §10 第 5 条。

**排版**

- **P1** `NL` 是一个或多个换行；空行属于排版。
- **P2** 顶层声明之间恰好空一行，块内不空行，文件末尾恰有一个换行。
- **P3** 逗号之后一个空格；`:` 之前没有空格、之后一个空格；`->`、`->>`、`=`、`emits` 两边各一个空格；类型的 `<` `>`，以及 `(` `)` `[` `]` 和值的 `{` `}`，内侧没有空格；区间写作 `1..10`。
- **P4** 可选的块为空时（类型、外部系统、调用的子步骤）不写花括号；必需的块为空时写成 `{` 和 `}` 两行。
- **P5** 注释属于哪一项：一组注释（其间可以有空行）属于紧随其后的声明、成员或步骤，紧贴在它之上，缩进与它相同；`}` 之前的注释留在块的末尾；`} else` 之前的注释属于前一个分支；最后一个声明之后的注释，空一行之后打印。
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
| `replies<外部操作>` | 非空应答列表，每项恰为 `{returns: 值}` 或 `{raises: "绝对错误标识", fields: {字段…}}`；无返回值成功写 `none` |
| 接口字段 | `{操作名: 应答列表, …}`，给出包括继承操作在内的全部非空应答脚本 |

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
| `observe state: BankSnapshot from bank` | `observation` / `snapshot` | 所在性质的标识后接 `.state` |
| 设计文件中的 `case empty` | `case` / `declared` | `order.total-is-sum.empty` |
| 录制文件中的 `case r-3f9a2c1d coverage` | `case` / `recorded` | `order.total-is-sum.r-3f9a2c1d` |

其余构造不产生节点，而是成为节点的属性：

- `notation` 和 `language`：系统允许的记法，以及各语言的源码根目录；
- `label`、`focus`、`paths`：容器或类型的标签、关注度和拥有的路径；
- `field`、`value`：类型的字段和枚举值；
- `forall`、`where`：性质或流程的符号和限定；
- `requires`、直接路径形式的 `observe` 和 `changes`：性质的前提、观察范围和允许变化的范围；具名观察另产生节点，其名称也可用于表达式；
- 用例中的赋值：用例对各符号的取值，按 §3.1 解读，并且必须落在限定之内；
- 摘要：节点的摘要。性质的摘要用一句话说明它的陈述；
- `pure`：操作是纯操作（[model.md](model.md) §3.3），可以在表达式中调用；
- `call`、`within`、`ensure`：性质或流程的陈述；`manual`：人工性质的原因。

字符串，即摘要、生成符号的取值域说明、`alt`、`opt`、`loop` 和 `else` 的标签，以及 `manual` 的原因，不参与约束的检查。它们计入节点的内容哈希，所以改动会出现在快照的比较和评审中（[model.md](model.md) §8）。

摘要、取值域说明和 `manual` 的原因在查看器中单独显示，按下列规则书写：

- **摘要**是一句话，只说一件事，单独显示时也能读懂："这""该""它"只指本条字符串中出现过的名词。摘要提到声明的关键字段或参数，不提声明中没有的东西，不用自造的缩略语。性质的摘要说明它的陈述。只作为性质或流程输入的类型，摘要以"X 的输入："开头，X 是用到它的性质、流程或组件。
- **取值域说明**写取哪些值：范围、组成，以及特意加入的边界情形；不写生成器的来历。
- **`manual` 的原因**写这条性质或这个流程需要、而表达式没有的那一项构造。它针对这一条，不是它所在的一组。
- 两条声明的陈述相近时，各自的摘要写出两者的区别。
- 文档定义过的术语只用定义的意思；英文标识只用来指确切的字段、命令或代码。

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
| `call`、`requires`、`ensure` 引用的操作、枚举和错误类型 | `refers` 性质或流程 → 被引用的操作、类型 |
| `observe`、`changes` 中访问的字段所属类型和下标引用 | `refers` 性质 → 被引用的类型、操作，字段路径和子句序号写在边的属性中 |
| 具名观察的观察类型 | `refers` 观察 → 其中每个命名类型 |
| `P -> R(…)`、`P ->> R(…)` | `call`、`send` 步骤；`calls` 步骤 → R，`from` 步骤 → P |
| `P emits E(…)` | `emit` 步骤；`emits` 步骤 → E，`from` 步骤 → P |
| `async R { … }` | `async` 步骤；`calls` 步骤 → R |
| `on E { … }` | `on` 步骤；`handles` 步骤 → E |

`owns`、`depends`、`traces`、`reaches` 等边不由本语言写出，在编译的链接阶段推导得到（§5.4）。

### 4.4 表达式

性质的 `requires`、`call`、`ensure` 和变化路径的下标，以及流程的 `ensure` 中写表达式。表达式的求值不改变任何值（`call` 子句除外），表达式的构造本身必然终止。

**名称。**

- **E1** 链 `a.s1.….sk` 中，`a` 是符号、具名观察或约束变量时，带实参表的段是对前一个值的类型的操作的调用，其余的段是字段访问。
- **E2** `a` 不是符号或约束变量时，取链中按 R1–R3 能解析为枚举值或操作的最长前缀作为引用，其后的段按 E1 读。`size` 是唯一的内置函数。
- **E3** 量化式 `all(x in c: p)`、`any`、`count`、`sum` 中的 x 称为**约束变量**，作用域是冒号之后的表达式；约束变量与所在性质或流程的符号、具名观察、外层的约束变量互不同名。

**类型。** 每个表达式有一个类型，记作 e : T。未列出的组合都是类型错误：

| 表达式 | 条件 | 类型 |
| --- | --- | --- |
| 符号 x | — | x 的类型 |
| 具名观察 x | 前提中取调用前观察值，后置陈述中取调用后观察值 | 声明的观察类型 |
| 约束变量 x | 见量化式的行 | 见该行 |
| 不带小数点的数；带小数点的数；字符串；`true`、`false` | — | `int`；`float`；`string`；`bool` |
| `none` | 与 `optional<T>` 的值比较 | `optional<T>` |
| `T.v` | T 是 `enum`，v 是它的枚举值 | T |
| `e.f` | e : R，R 是 `record`、`object`、`event` 或 `error`，f 是 R 本身或经 `extends` 继承的字段 | f 的类型 |
| `e.o(a1, …)` | e : R，o 是 R 的操作，声明为 `pure`，有返回值，实参的类型与参数一一相同 | o 的返回类型 |
| `o(a1, …)` | o 是容器或外部系统直接提供的操作，其余同上 | o 的返回类型 |
| `size(e)` | e 为 `list`、`set`、`map`、`string` 或 `bytes` | `int` |
| `e[k]` | e : `map<K, V>`，k : K | V |
| `e1 + e2`、`e1 - e2`、`e1 * e2`、`e1 / e2` | 操作数同为 `int` 或同为 `float` | 操作数的类型 |
| `e1 == e2`、`e1 != e2` | 操作数同一类型，不是 `call<…>` | `bool` |
| `e1 < e2`、`<=`、`>`、`>=` | 操作数同为 `int`、`float`、`string`、`time` 或 `duration` | `bool` |
| `e1 in e2` | e2 : `list<T>` 或 `set<T>` 且 e1 : T；或 e2 : `map<K, V>` 且 e1 : K | `bool` |
| `not e`；`e1 and e2`、`e1 or e2`、`e1 implies e2` | 操作数都是 `bool` | `bool` |
| `all(x in c: p)`、`any(x in c: p)` | c : `list<T>` 或 `set<T>` 时 x : T，c : `map<K, V>` 时 x : K；p : `bool` | `bool` |
| `count(x in c: p)` | 同上 | `int` |
| `sum(x in c: e)` | x 同上；e : `int` 或 `float` | e 的类型 |
| `old(e)` | 有被测的调用或调用序列；e 中没有约束变量、`old`、`result`、`raised` 和 `failure` | e 的类型 |
| `result` | 被测的调用有返回值 | 返回类型 |
| `raised(E)` | E 在被测调用的操作的 `raises` 中 | `bool` |
| `failure(E)` | 与 `raised(E)` 相同；仅匹配的错误返回时有值 | E，只暴露 E 声明的字段 |

类型为 `optional<T>` 的表达式可以用在需要 T 的地方。`int` 与 `float` 不互相转换，与 `float` 比较的常数写作 `0.0`。`ensure` 的类型为 `bool`。

字段与操作先在本类型查找，再沿 `extends` 按父类型的声明次序深度优先查找；本类型的同名成员遮蔽父成员，同一祖先在菱形继承中只访问一次。用例给出记录的值时，给出按这条规则可见的全部字段；限定和表达式使用同一查找规则。`call<T>` 包含 T 的可见操作，继承操作仍调用其实际落实；这类类型引用的序列以 T 为接收者类型，直接操作引用则以其声明的所属类型为接收者类型。

**被测的调用**是性质的 `call` 子句，或流程的第一个步骤。`call` 子句写作 `e.o(a1, …)` 或 `o(a1, …)`：o 可以不是纯操作，可以没有返回值；e 是输入符号，或从输入符号出发的字段访问；接收者和实参不依赖具名观察，也不使用 `old`、`result`、`raised` 或 `failure`。

**求值。**

- **E4** `int` 是数学上的整数，没有溢出；`int` 的 `/` 向零截断。`float` 是有限的 IEEE 754 双精度数，运算得到 NaN 或无穷是求值错误；`float` 的 `sum` 是各项的精确和舍入到最近的双精度数，与次序无关。
- **E5** `==` 是结构相等：`record`、`object`、`event` 和错误视图逐个比较设计中写出的字段，`list` 按次序比较，`set` 和 `map` 不计次序。
- **E6** `and`、`or` 和 `implies` 只求值需要的操作数；其余运算先求值全部操作数。
- **E7** 量化式对 c 的每个元素都求值，不短路。c 为空时，`all` 为真，`any` 为假，`count` 为 `0`，`sum` 为 `0` 或 `0.0`。
- **E8** `size` 对 `string` 计码位的个数，对 `bytes` 计字节数；`string` 的 `<` 按码位的字典序比较。
- **E9** `old(e)` 的值是被测调用之前 e 的值的副本，此后的调用不改变它。
- **E10** 操作的 `raises` 中列出的错误称为它**声明的错误**。`raised(E)` 为真，当且仅当被测调用抛出的错误是 E，或经 `extends` 以 E 为祖先。
- **E11** 以下是求值错误：除以零；`map` 中不存在的键；用在需要 T 的地方的 `optional<T>` 的值为 `none`；纯操作在表达式中抛出错误；被测调用抛出错误时求值 `result`；没有匹配错误时求值 `failure(E)`。

**检查。** 一条带陈述的性质或流程，对每个用例（声明的、录制的和生成的）：

- **E12** 有被测的调用时，先取得调用前观察值并检查全部前提；只有前提成立，才固定变化路径、求值全部 `old(e)` 和被测调用的接收者与实参，再执行一次调用，冻结返回值或错误的数据，取得调用后观察值，最后检查全部 `ensure`，以及没有范围外的差异。拒绝输入时不执行调用，也不预先求值 `old(e)`。被测调用抛出不在其操作的 `raises` 中的错误时，这个用例是反例。流程的被测调用由流程的驱动执行，M4 执行入口并检查后置陈述，M6 再核对内部步骤的调用先后（[design.md](design.md) §6.3）。自动流程入口实参必须是已声明且类型匹配的输入符号；方法入口须有唯一的所属类型输入作为接收者。需要额外环境准备的流程声明 `manual` 理由。
- **E13** 有调用序列的符号时（G14），先检查前提，初态的 `old(e)` 取初态副本，求值一次全部 `ensure`；再把调用依次作用到类型为 T 的符号上，每次调用之前求值 `old(e)`，之后求值全部 `ensure`。调用抛出它的 `raises` 中的错误时，接着执行下一个调用；抛出其他错误时，这个用例是反例。
- **E14** 其余的性质，前提成立时求值全部 `ensure`；人工性质的前提成立时调用手写谓词。
- **E15** 任一 `ensure` 为假、有范围外的差异，或陈述出现求值错误，这个用例就是反例。前提求值错误、域外输入、观察失败和没有有效执行（E23）称为**检查错误**：它们分别报告，不算反例。外部系统的应答和类型为接口的字段由替身给出，也是生成的输入（[design.md](design.md) §6.2）。
- **E16** 带 `within` 的性质或流程，记下一次运行中每个用例的被测调用的耗时；这些耗时的第 95 百分位超过 `within` 的时长时，这条性质或流程不通过。

人工性质没有可求值的陈述，由手写的谓词检查（[model.md](model.md) §10 第 20 条）。

### 4.5 黑盒契约的范围与观察

- **E17** `requires` 在调用前按文本次序求值，只有全部为真才适用。前提求值错误是检查错误；为假时的处理见 E22。前提只限定这条性质的适用输入，不替操作声明拒绝非法输入的行为；要求某种输入返回错误时，应把该输入纳入适用域，并以 `ensure raised(E)` 陈述。
- **E18** 观察值是有限的模型值：内置类型、枚举、记录、对象、事件及其集合或可选组合，只包含设计声明的字段。接口、调用值、不可读字段和循环引用不能直接作为观察值；需要具名观察的适配器给出可观察的数据。观察值是独立副本，此后的修改，包括经共享对象的修改，都不改变调用前观察值。只读适配器接收 `from` 列出的输入，调用前后各读取一次；它不接受其他观察值。捕获消息或写入的替身须在第一次读取前安装，调用结束时读取它的实际记录，并在拒绝或结束后清理。读取失败、不可序列化或没有记录能力时是观察失败，不能用默认值代替。
- **E19** `changes` 列出的每条路径称为**变化路径**。变化路径从观察根出发，字段与下标逐段解析。下标在前提成立后、调用前求值一次，其键值固定。允许改变某个值，就允许改变该值的整个子树；允许改变 `map` 的一个条目，包含新增和删除该条目；只允许改变条目的字段时，条目本身必须保留。缺失的最终条目可以作为允许新增的目标，缺失的父值则是路径求值错误。`list`、`set` 只能整体列入变化范围，本轮不增加位置访问。
- **E20** 正常返回和错误返回都按 E5 比较观察前后的值，允许变化的子树之外必须相同。比较包括字段和集合成员的存在性；每条观察路径独立适用，别名不能扩大另一条路径的允许范围。`changes none` 没有例外。检查只涉及声明的观察范围及其最终值；要约束实际写入或消息发送，须把对应记录列入观察。
- **E21** `failure(E)` 返回本次调用实际产生的、与 E 匹配的错误数据的 E 类型视图，匹配规则同 E10；实际错误是 E 的子类型时也只暴露 E 的字段。错误内容在调用返回时复制，不受之后的观察或断言影响。
- **E22** 声明或回放的用例不满足前提时是域外输入错误，不执行被测调用。探索生成的候选输入不满足前提时被拒绝，称为**拒绝的候选输入**：它只计数，探索继续生成；前提求值错误仍然报错。类型和 `where` 的检查仍在载入时进行，依赖纯操作或观察的前提只在运行时判断；`unicode check` 不执行适配器或业务操作。模型整数传给实际语言的整数参数时，必须保留整数值；超出实际参数的有符号或无符号范围是域外输入错误，不能截断或环绕后调用。
- **E23** 前提成立、并且执行了被测调用（人工性质则调用了手写的谓词）的一次执行，称为**有效执行**。每条被选中的性质或流程须有有效执行，才有执行通过的结论。回放没有适用用例，或探索预算耗尽而没有有效执行时，报告没有有效输入；拒绝的候选输入数单独计入结果，不算覆盖或有效执行。输入域是否为空不能由拒绝采样的结果推断。

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
| 链接 | 语言后端、`graph`、`check` | 设计层与代码 → 图与状态 | `cli.extract-failed`、`lang.<语言>.*`、`graph.*`、`check.*` |

### 5.1 分类、词法分析与语法分析

- **分类**：路径决定文件的种类，种类决定使用哪个起始符号。`design/system.uc` 是系统文件，`design/` 中的其他 `.uc` 文件是组件文件，`corpus/` 中的 `.uc` 文件是录制文件；其他路径不是本语言的源文件，路径中有 `.`、`..` 或空的段，或者文件名只是 `.uc` 的，也不是。
- **词法分析**：先拒绝 §2 禁止的字符，即制表符、CR、其他控制字符和非 UTF-8 的字节，每处一条诊断；再按最长匹配切分记号：注释、字符串、数、名称、`->>`、`->`、`..`、`==`、`!=`、`<=`、`>=`，以及标点 `{ } ( ) [ ] < > , : . = + - * /`。关键字与名称是同一种记号，按所在的位置区分。字符串和注释各自整体作为一个记号，因此不会被当作关键字。
- **语法分析**：§3 的文法以 participle 的结构体标签写在 `internal/syntax/grammar.go` 中，与 §3 的产生式逐条对应。分析得到语法树 `syntax.File`；遇到第一个不合文法的记号即停止，报告一条诊断。

### 5.2 静态规则与规范格式

- **静态规则**：文法表达不了的 L、G 规则在语法树上检查，例如声明和成员的次序、名称的写法、保留字、摘要的形式、同一块中重复的成员。违反的规则全部报告。语法树只有在没有任何诊断时才进入下一阶段。
- **规范格式**：打印器把语法树写成规范文本，它实现 P 规则。`unicode fmt` 用它改写文件；`unicode check` 把它与源文本逐字节比较，不同时报 `cli.not-canonical`。规范文本再经分析和打印，结果不变。

### 5.3 载入

`design.Load` 接收全部合法的语法树，产出设计层和诊断（`design.Loaded`），分三步：

1. **声明**：按固定次序读取声明，为每条声明建一个节点并给出标识。次序是系统文件、由浅到深的组件文件、组件中的成员、录制文件，同组之内按路径。两条声明得到同一标识时，后读到的报 `design.duplicate-name`；读取次序固定，所以哪一条算重复与文件次序无关。
2. **解析引用**：全部节点建好之后，按依赖分批解析引用：先是组件的 `uses`；再是类型和操作的签名、符号和观察的类型；再是符号的取值域与观察输入；最后是步骤、用例和全部契约子句。解析成功即按 §4.3 建边；失败时报 `design.unresolved-reference` 或 `design.wrong-reference-kind`。
3. **设计约束**：检查只需设计就能判断的约束（[design.md](design.md) §5.1），包括表达式的类型（§4.4），以及用例的值是否属于符号的类型并落在限定之内。

有语法错误的文件不参与载入；其余文件照常载入，指向缺失声明的引用报为未解析。

### 5.3.1 可回放表示

命名类型可在成员中声明一次 `codec 编码操作, 解码操作`。编码操作是该类型的纯方法，无参数、无声明错误、恰返回一个有限可序列化模型类型（E18 的内置值、枚举、记录或集合，排除 interface、error、call 与 replies；可经其他 codec 得到有限表示）；解码操作是自由操作，恰接受这个表示类型，返回原类型，可声明解码错误。codec 不随继承隐式传给子类型，表示依赖不能循环；接口使用既有应答脚本，不声明 codec。

用例值 `encoded(v)` 表示按该类型的 codec 解码表示值 v。v 在设计载入时按编码操作的返回类型检查；解码后的类型、限定和前提在运行前检查，不能静态执行解码操作来假装判定。没有 codec、表示类型不符或解码失败均为错误。运行时编码后解码再编码，须得到同一个规范表示；这是表示的等价关系，不要求缓存、指针或 nil/空集合等原生细节逐位相同；未建模的 Go 私有状态既不能以空记录录制，也不能以随机种子冒充模型值。`graph.Graph` 用既有 `encode` 和 `graph.decode`，因而图的完整内容可进入 corpus。普通字段值和替身脚本保持原写法。

### 5.4 链接

链接把设计层与代码层合成一张图：每种已配置语言的后端抽取代码层，代码中的绑定（§6）成为 `realizes`、`checks`、`generates` 和 `observes` 边；合并之后按 [model.md](model.md) §9 推导其余的边和显著性，再按 [model.md](model.md) §10 检查约束，得到诊断和每个设计节点的状态。步骤、缓存和快照见 [design.md](design.md) §5。相同的程序和代码总是得到逐字节相同的快照。

### 5.5 诊断

每条诊断有诊断码、位置、原因和修正办法。诊断码写作 `<组件>.<短横线名称>`，组件是报出它的那个：`cli`、`syntax`、`design`、`graph`、`check`，或语言后端，例如 `lang.golang`。诊断都是错误，没有警告；输出按路径、行、列和诊断码排序。

## 6 绑定

绑定写在代码中声明的正上方，参数都是字面量，语言后端不执行代码也能读出。绑定有六种：

- **`realizes <标识>`**：写在类型或自由函数上，表示它落实这个类型，或容器、外部系统直接提供的这个操作。落实外部系统的操作的，是调用该外部系统的客户端函数。方法上不写绑定；方法按名称映射对应到它所在类型落实的那个类型的操作（[design.md](design.md) §4）。
- **`checks <标识>`**：写在谓词上，表示它检查这条性质或这个流程。谓词的参数就是性质或流程的符号；驱动由 `gen` 生成（[design.md](design.md) §6.2）。一个函数至多带一个 `checks`。
- **`generates <标识>`**：写在生成器函数上，标识是性质或流程的标识后接 `.` 和一个生成的符号的名称，表示它给出这个符号的生成器。同一个生成器可以用于几个符号，每个符号写一行 `generates`。
- **`observes <标识>`**：写在只读的观察适配器自由函数上，指向性质内的具名观察。参数按 `from` 的次序，返回观察类型的值；Go 可以另带末尾的 `error`，Python 抛出的异常也属于观察失败。这些失败是检查错误（E15），不是被测操作声明的错误。

- **`substitutes <标识>`**：写在安装器上（[model.md](model.md) §4），安装器是自由函数。目标是外部系统直接提供的操作；Go 参数是与真实客户端完全同签名的函数，唯一返回值是清理函数 `func()`。脚本通过 `forall answers: replies<外部操作>` 提供；`replies` 只作符号的直接类型，同一检查每个目标至多一个脚本；接口字段按操作名给出同形脚本。每项应答是 `{returns: 值}` 或 `{raises: "绝对错误标识", fields: {…}}`，无返回值的成功值为 `none`；脚本非空，末项重复。应答的类型、声明的错误和字段都在设计载入时检查。

- **`publishes <事件标识>`**：实际发布边界的函数或方法，恰接收该事件的一个参数且没有返回值；多个发布边界可以指向同一事件，每个函数只声明一个。正常完成该边界产生一次发布轨迹，panic 或未完成不当作已发布。它是运行观测边界，不把构造事件或进入订阅者当成发布。

- **B1** 绑定的动词只有 `realizes`、`checks`、`generates`、`observes`、`substitutes` 和 `publishes`，后接一个空格和一个设计标识；`generates` 指向生成的符号，`observes` 指向具名观察。
- **B2** 绑定写在类型或函数的文档注释中，独占一行；`checks`、`generates` 只写在函数上，`observes` 和 `substitutes` 只写在自由函数上；`publishes` 只写在具体自由函数或方法上，不能写在接口的抽象操作上。

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
	// 由 gen 从 ensure 生成，在生成文件中：断言 order.Total() 等于各行数量乘单价之和
}

//uc:generates syntax.format-is-stable.sample
func genSample() *rapid.Generator[Sample] {
	// 生成合法的语法树，再以随意的空白打印成文本
}
```

### 6.2 Python

绑定是 `uc` 包的装饰器。模块只能写作 `import uc`，不能取别名，也不能用 `from uc import`。装饰器是 `@uc.realizes("<标识>")`、`@uc.checks("<标识>")`、`@uc.generates("<标识>")` 或 `@uc.observes("<标识>")`，只有一个字符串字面量参数：

```python
import uc

@uc.realizes("order.OrderService")
class OrderService:
    def submit(self, cart: Cart) -> Order: ...
    def on_paid(self, event: Paid) -> None: ...

# 由 gen 从 ensure 生成，在生成文件中
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

notation c4

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
  label container
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
  ensure all(sku in stock.available: stock.available[sku] >= 0)
  case reserve-then-release {
    stock = {available: {"a": 5}}
    steps = [reserve(items: [{sku: "a", quantity: 3}]), release(items: [{sku: "a", quantity: 3}])]
  }
}
```

```
// design/payment.uc
component payment "支付：经外部网关扣款" {
  label bounded-context
  label container
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
  label container
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
  op total() -> int pure "各行数量乘单价之和，单位为分"
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
  field stock: inventory.Stock
  field payments: payment.Payments
  field orders: OrderRepository
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
  within "500ms"
}

property total-is-sum invariant "订单总额等于各行数量乘单价之和" {
  about Order.total
  forall order: Order
  where order.lines size 0..20
  where order.lines.quantity in 1..99
  where order.lines.unitPrice in 0..1000000
  ensure order.total() == sum(l in order.lines: l.quantity * l.unitPrice)
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
  forall service: OrderService
  forall cart: Cart
  where cart.lines size 1..10
  call service.submit(cart)
  ensure raised(payment.Declined) implies service.stock.available == old(service.stock.available)
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
- **性质视图**：`total-is-sum` 约束 `Order.total`，带一个符号 `order`、三条限定、两个声明的用例和一个录制的用例。它的谓词 `check_total_is_sum` 和驱动都由 `gen` 从 `ensure` 生成，视图中还显示这些用例覆盖到的代码。

写好代码之后，`unicode check` 检查的内容包括：

- `src/shop/order/` 下的每个文件归属于 `order`；`order` 的代码只导入 `inventory` 和 `payment` 的包；只有 `payment` 调用落实 `payment-gateway.charge` 的客户端。
- `OrderService` 由一个带绑定的类落实，它的 `submit` 和 `on_paid` 方法签名与设计一致。
- 流程中的每次调用都能在调用图中找到：`submit` 的方法能到达 `reserve`、`save` 和 `charge`，`charge` 的方法能到达网关的客户端；`charge` 的方法引用了落实 `Paid` 的类型。
- 每条性质和每个流程都有一个谓词；声明的和录制的用例都符合符号的类型和限定；代码中没有手写的测试；生成文件与当前的设计一致。

### 7.1 单次转账的黑盒契约

`bank` 组件的操作实现、凭据和错误都由代码绑定；具名观察的适配器另带 `//uc:observes bank.declined-keeps-state.state`，接收 `Bank`，返回 `BankSnapshot` 或 `(BankSnapshot, error)`。

```uc
component bank "账户转账" {
  focus core
  paths "src/bank/**"
}

type Receipt record "转账凭据" {
  field amount: int
}

type InsufficientFunds error "转出账户余额不足" {
  field accountId: string
  field available: int
}

type BankSnapshot record "观察适配器取得的账户余额" {
  field balances: map<string, int>
}

type Bank object "按账户标识保存的余额" {
  field balances: map<string, int>
  op transfer(fromId: string, toId: string, amount: int) -> Receipt raises InsufficientFunds
}

property transfer-moves-money postcondition "转账金额与双方余额正确，其他账户保持不变" {
  about Bank.transfer
  forall bank: Bank
  forall fromId: string
  forall toId: string
  forall amount: int
  where amount in 1..100000
  observe bank
  requires fromId != toId
  requires fromId in bank.balances and toId in bank.balances
  requires amount <= bank.balances[fromId]
  changes bank.balances[fromId], bank.balances[toId]
  call bank.transfer(fromId, toId, amount)
  ensure not raised(InsufficientFunds)
  ensure result.amount == amount
  ensure bank.balances[fromId] == old(bank.balances[fromId]) - amount
  ensure bank.balances[toId] == old(bank.balances[toId]) + amount
  case three-accounts {
    bank = {balances: {"a": 10, "b": 5, "c": 7}}
    fromId = "a"
    toId = "b"
    amount = 3
  }
}

property declined-keeps-state postcondition "余额不足时指出账户与可用余额，观察到的余额全部保持不变" {
  about Bank.transfer
  forall bank: Bank
  forall fromId: string
  forall toId: string
  forall amount: int
  where amount in 1..100000
  observe state: BankSnapshot from bank
  requires fromId != toId
  requires fromId in state.balances and toId in state.balances
  requires amount > state.balances[fromId]
  changes none
  call bank.transfer(fromId, toId, amount)
  ensure raised(InsufficientFunds)
  ensure raised(InsufficientFunds) implies failure(InsufficientFunds).accountId == fromId
  ensure raised(InsufficientFunds) implies failure(InsufficientFunds).available == old(state.balances[fromId])
  case insufficient {
    bank = {balances: {"a": 2, "b": 5}}
    fromId = "a"
    toId = "b"
    amount = 3
  }
}
```

## 附录 A 理由

- **只有一种写法（F4）。** 规范格式让同一个设计只有一种文本，评审时的差异就是设计的差异；`fmt` 只改排版（P 规则），不替人改次序和结构（G 规则），因为次序是作者的阅读顺序。
- **路径由标识决定（F2、F3）。** 从标识能直接找到文件，反之亦然；顶层组件因此不能与系统文件同名。
- **字符串在一行之内（L3）。** 每行恰是一个声明、成员或步骤，按行比较即可看出改动；多行文本用 `\n` 写在一行之内。
- **名称的写法由种类决定（L6）。** 看到名称就知道种类，也让名称能与各语言的代码名称双向映射（[model.md](model.md) §10 第 30 条）。
- **关键字可以用作名称（L7、G5）。** 步骤由行首的形状识别，参与者和符号因此可以与关键字同名；只有 `true`、`false`、`none` 会与值混淆，所以保留。
- **组内保持作者的次序（G1）。** 分组固定了查找的位置，组内的次序留给作者安排阅读顺序。
- **性质用表达式书写（§4.4）。** 陈述写成表达式，工具链才能检查它的类型并生成执行真实操作的谓词。表达式只引用声明过的类型、字段、观察和纯操作，因此陈述用到的数据必须先进入设计。
- **表达式保持最小（§4.4）。** 本语言做的是抽象层面的声明，不描述具体的计算，所以表达式只有陈述关系所需的构造，唯一的内置函数是 `size`。按键取值、计数和可选值的使用是原子的：没有它们，按键存放的值、唯一性和可选字段上的性质都写不出来，拆分也绕不过去。
- **写不成表达式的性质要声明出来（G12）。** 一条性质写不成表达式时，先拆成几条只陈述一件事的性质；仍写不成的才写 `manual`，并写明原因。它的含义只能靠评审保证，所以谓词一变就列为待审（[model.md](model.md) §10 第 33 条）。
- **整数没有溢出（E4）。** 陈述描述的是数学上的结果；代码中的溢出因此表现为反例，而不会被陈述本身掩盖。
- **没有一元的负号（G16）。** 负数是一个记号；`-` 只有减法一种读法，排版也就不会改变表达式的含义。
- **流程也可以写陈述（G17）。** 流程的结果是端到端的陈述；它复用流程的符号，以第一次调用作为被测的调用。
- **耗时是陈述的一种（E16）。** 耗时的预算复用性质的符号、生成器和被测的调用，测量与正确性检查用同一批输入；取第 95 百分位，偶然一次的慢不会判为不通过。

## 附录 B 规则与诊断码

每条规则一行，列出违反它时报出的诊断码，以及生成器违反它的性质或流程（`design/*.uc` 中的标识）。P 规则由打印器实现，违反时报 `cli.not-canonical`。不属于任何规则的诊断码列在 [design.md](design.md) §8。`make rule-check`（`scripts/rulecodes.py`）检查本表、[model.md](model.md) 附录 B 和 [design.md](design.md) §8 合起来恰好列出代码中的全部诊断码。

| 规则 | 诊断码 | 验证的性质 |
| --- | --- | --- |
| F1 | `cli.not-a-design-file` | `syntax.kind-of-follows-path`、`cli.format-files`、`cli.check-design` |
| F2 | `syntax.missing-header`、`syntax.unknown-declaration`、`syntax.duplicate-member`、`syntax.missing-member`、`design.missing-system-file`、`design.component-path-mismatch` | `syntax.rejects-broken-text`、`design.rejects-broken-design` |
| F3 | `cli.not-a-design-file`、`syntax.missing-header`、`syntax.unknown-declaration`、`syntax.duplicate-member`、`design.corpus-path-mismatch`；"只由 `unicode test --record` 写入"未实现：M6 | `cli.format-files`、`syntax.rejects-broken-text`、`design.rejects-broken-design` |
| F4 | `cli.not-canonical` | `syntax.format-is-stable`、`cli.format-files`、`cli.check-design` |
| L1 | `syntax.invalid-utf8`、`syntax.tab`、`syntax.carriage-return`、`syntax.control-character`、`syntax.unexpected-character` | `syntax.rejects-broken-text` |
| L2 | `syntax.trailing-comment` | `syntax.rejects-broken-text` |
| L3 | `syntax.unterminated-string`、`syntax.invalid-escape` | `syntax.rejects-broken-text` |
| L4 | `syntax.empty-summary`、`syntax.summary-tab-or-newline`、`syntax.summary-surrounding-space` | `syntax.rejects-broken-text` |
| L5 | `syntax.not-an-integer`、`cli.not-canonical` | `syntax.rejects-broken-text`、`syntax.format-is-stable` |
| L6 | `syntax.invalid-name`、`syntax.invalid-reference` | `syntax.rejects-broken-text` |
| L7 | `syntax.reserved-name` | `syntax.rejects-broken-text` |
| G0 | `syntax.unexpected-token`、`syntax.unexpected-end`、`syntax.missing-member`、`syntax.declaration-order`、`syntax.missing-claim`、`syntax.mixed-claim` | `syntax.rejects-broken-text` |
| G1 | `syntax.declaration-order` | `syntax.rejects-broken-text` |
| G2 | `syntax.member-order`、`syntax.value-outside-enum`、`syntax.field-not-allowed` | `syntax.rejects-broken-text` |
| G3 | `syntax.member-order`、`syntax.missing-member`、`syntax.duplicate-member` | `syntax.rejects-broken-text` |
| G4 | `syntax.brace-position`、`syntax.misplaced-else`、`syntax.unexpected-token` | `syntax.rejects-broken-text` |
| G5 | `syntax.unexpected-token` | `syntax.rejects-broken-text` |
| G6 | `syntax.on-without-call` | `syntax.rejects-broken-text` |
| G7 | `syntax.unexpected-token`、`syntax.empty-summary`、`syntax.summary-tab-or-newline`、`syntax.summary-surrounding-space` | `syntax.rejects-broken-text` |
| G8 | `design.missing-focus` | `design.rejects-broken-design` |
| G9 | `design.uses-person`、`design.wrong-reference-kind` | `design.rejects-broken-design` |
| G10 | `design.empty-interval` | `design.rejects-broken-design` |
| G11 | `syntax.comment-in-value`、`syntax.unexpected-token` | `syntax.rejects-broken-text` |
| G12 | `syntax.empty-summary`、`syntax.summary-tab-or-newline`、`syntax.summary-surrounding-space` | `syntax.rejects-broken-text` |
| G13 | `syntax.invalid-tested-call`、`syntax.invalid-time-budget`、`design.call-not-about` | `syntax.rejects-broken-text`、`design.rejects-broken-design` |
| G14 | `design.invalid-call-sequence` | `design.rejects-broken-design` |
| G15 | `syntax.expression-keyword-name` | `syntax.rejects-broken-text` |
| G16 | `syntax.expression-spacing` | `syntax.rejects-broken-text` |
| G17 | `syntax.invalid-flow-call` | `syntax.rejects-broken-text` |
| G18 | `syntax.invalid-observation-scope` | `syntax.rejects-broken-text` |
| G19 | `design.invalid-observation`、`design.duplicate-name` | `design.rejects-broken-design` |
| G20 | `design.expression-type` | `design.rejects-broken-design` |
| G21 | `design.invalid-change-path`、`design.expression-type` | `design.rejects-broken-design` |
| P1 | `cli.not-canonical` | `syntax.format-is-stable` |
| P2 | `cli.not-canonical` | `syntax.format-is-stable` |
| P3 | `cli.not-canonical` | `syntax.format-is-stable` |
| P4 | `cli.not-canonical` | `syntax.format-is-stable` |
| P5 | `cli.not-canonical` | `syntax.format-is-stable` |
| P6 | `cli.not-canonical` | `syntax.format-is-stable` |
| R1 | `design.unresolved-reference` | `design.rejects-broken-design` |
| R2 | `design.unresolved-reference` | `design.rejects-broken-design` |
| R3 | `design.unresolved-reference` | `design.rejects-broken-design` |
| B1 | `lang.golang.unknown-binding-verb`、`lang.golang.missing-binding-id`、`lang.golang.invalid-binding-id` | `lang.golang.extracts-generated-module` |
| B2 | `lang.golang.misplaced-binding` | `lang.golang.extracts-generated-module` |
| E1 | `design.expression-type` | `design.rejects-broken-design` |
| E2 | `design.expression-type`、`design.unresolved-reference`、`design.wrong-reference-kind` | `design.rejects-broken-design` |
| E3 | `design.expression-type` | `design.rejects-broken-design` |
| E4 | `lang.golang.expression-failed`、`lang.golang.ensure-failed` | `lang.golang.executes-contracts` |
| E5 | `lang.golang.ensure-failed`、`lang.golang.frame-violation` | `lang.golang.executes-contracts` |
| E6 | `lang.golang.expression-failed`、`lang.golang.ensure-failed` | `lang.golang.executes-contracts` |
| E7 | `lang.golang.expression-failed`、`lang.golang.ensure-failed` | `lang.golang.executes-contracts` |
| E8 | `lang.golang.ensure-failed` | `lang.golang.executes-contracts` |
| E9 | `lang.golang.observation-failed`、`lang.golang.unsupported-contract`、`lang.golang.ensure-failed` | `lang.golang.executes-contracts` |
| E10 | `lang.golang.undeclared-error`、`lang.golang.ensure-failed` | `lang.golang.executes-contracts` |
| E11 | `lang.golang.expression-failed`、`lang.golang.invalid-result`、`lang.golang.call-panicked` | `lang.golang.executes-contracts` |
| E12 | `lang.golang.undeclared-error`、`lang.golang.ensure-failed`、`lang.golang.expression-failed`、`lang.golang.flow-mismatch`、`lang.golang.trace-missing`、`lang.golang.missing-publisher`；流程内部顺序按 M6 真实轨迹核对 | `lang.golang.executes-contracts` |
| E13 | `lang.golang.undeclared-error`、`lang.golang.ensure-failed`、`lang.golang.expression-failed` | `lang.golang.executes-contracts` |
| E14 | `lang.golang.ensure-failed`、`lang.golang.predicate-failed` | `lang.golang.executes-contracts` |
| E15 | `lang.golang.missing-substitute`、`lang.golang.substitute-failed`、`lang.golang.ensure-failed`、`lang.golang.frame-violation`、`lang.golang.expression-failed`、`lang.golang.predicate-failed`；接口和外部客户端使用输入中的应答脚本 | `lang.golang.executes-contracts` |
| E16 | `lang.golang.within-exceeded` | `lang.golang.executes-contracts` |
| E17 | `lang.golang.input-outside-domain`、`lang.golang.expression-failed` | `lang.golang.executes-contracts` |
| E18 | `lang.golang.observation-failed`、`lang.golang.adapter-failed`、`lang.golang.unsupported-contract` | `lang.golang.executes-contracts` |
| E19 | `lang.golang.expression-failed`、`lang.golang.frame-violation` | `lang.golang.executes-contracts` |
| E20 | `lang.golang.frame-violation` | `lang.golang.executes-contracts` |
| E21 | `lang.golang.expression-failed`、`lang.golang.observation-failed` | `lang.golang.executes-contracts` |
| E22 | `lang.golang.input-outside-domain`、`lang.golang.expression-failed` | `lang.golang.executes-contracts` |
| E23 | `lang.golang.no-effective-input` | `lang.golang.executes-contracts` |

# 图模型

本文定义 unicode 的图：快照的结构（§1–§6）、标识（§7）、快照的比较（§8）、推导规则（§9）和约束（§10）。正文只写定义和规则，理由见附录 A。设计文件的写法见 [language.md](language.md)，工具链如何建图、生成代码和运行检查见 [design.md](design.md)。

## 1 统一结构

**快照。** 一个仓库在一个提交上的全部知识是一张有类型的有向图 G = (N, E)，称为该提交的快照。

- N 是节点的有限集。标识 id : N → Id 是单射（§7）。
- E ⊆ EdgeKind × Id × Id 是边的有限集。边 e = (k, a, b) 的两端 a、b 都是 N 中节点的标识；同一个 (k, a, b) 至多一条边。

**层。** 每个节点恰属于一层，layer(n) ∈ {design, code, evidence, review}：

| 层 | 内容 |
| --- | --- |
| design | 设计文件和录制文件声明的意图 |
| code | 从代码抽取的事实 |
| evidence | 运行检查的结果（§3.5） |
| review | 评审记录（§3.5） |

**元素。** kind(n) ∈ {container, type, op, actor} 的节点称为元素。设计层和代码层使用同一组元素种类；`realizes` 边把代码层的元素对应到同一种类的设计层元素上。

**记法。** 记法从 G 中选取一部分，按自己的规则画出（§5）。记法不增加节点和边，不改变约束。

**记号。** 下文中，未注明层的容器、类型和操作都指设计层的；container(x) 是包含 x 的最近的设计层容器；allowed(A, B) 见第 13 条。

## 2 来源

每个节点和每条边恰有一种来源，source ∈ {declared, extracted, derived, produced}：

| 来源 | 产生者 |
| --- | --- |
| declared（声明） | 设计文件、录制文件，或代码中的绑定 |
| extracted（抽取） | 语言后端静态读取代码，不执行代码 |
| derived（推导） | §9 的规则 |
| produced（运行） | 执行检查，包括录制的用例 |

## 3 节点

每个节点有种类 kind(n) 和形式 form(n)；形式的取值由种类和层决定：

| 种类 | 设计层的形式 | 代码层的形式 | 属性 |
| --- | --- | --- | --- |
| `container` | `system`（根，恰有一个）、`component` | `package`、`file` | 名称、摘要、关注度、标签；设计层另有拥有的路径，代码层另有语言和路径 |
| `type` | `object`、`record`、`interface`、`enum`、`error`、`event` | 各语言的命名类型，例如 `class`、`struct`、`interface` | 名称、摘要、标签、字段、枚举值 |
| `op` | `op` | `func`、`method`、`abstract`（接口中声明的方法） | 名称、摘要、签名、订阅的事件；设计层另有是否为纯操作，代码层另有测试类别、是否为骨架 |
| `actor` | `person`、`external` | — | 名称、摘要 |
| `flow` | `flow` | — | 名称、摘要、符号、限定；陈述（`within`、`ensure`） |
| `step` | `call`、`send`、`emit`、`async`、`alt`、`opt`、`loop`、`on` | — | 序号、分支标签、实参 |
| `property` | `invariant`、`postcondition`、`relation`、`transition` | — | 名称、摘要、符号、限定、前提、观察范围和允许变化的范围；陈述（`call`、`within`、`ensure`），或人工性质的原因（`manual`） |
| `observation` | `snapshot` | — | 具名观察的名称、快照类型和输入符号；由观察适配器取得值 |
| `case` | `declared`、`recorded` | — | 名称、摘要、每个符号的取值；录制的用例另有录制原因 |

### 3.1 元素

- 设计层 `type` 的形式：

  | 形式 | 定义 |
  | --- | --- |
  | `object` | 有字段和操作，操作可以改变字段的值 |
  | `record` | 只有字段 |
  | `interface` | 只有操作，由其他类型实现 |
  | `enum` | 只有枚举值 |
  | `error` | 可以出现在操作的 `raises` 中 |
  | `event` | 可以被发布和订阅的数据 |

- 字段和枚举值是 `type` 的属性，不是节点。代码层只细到函数：变量和局部定义不是节点。
- `op` 至多订阅一个事件：该事件每发布一次，它就被调用一次。
- `actor`：`person` 是人，只能发起调用；`external` 是外部系统，可以包含 `type` 和 `op`，描述本系统用到的接口。外部系统的类型和操作由本仓库中的客户端代码落实。
- 关注度 focus(c) ∈ {core, support, utility}，是设计层 `container` 的属性，只影响视图（[design.md](design.md) §7.2）。
- 代码层 `op`：签名是语言无关的归一化形式（[design.md](design.md) §4）；测试类别 testKind ∈ {none, example, property, fuzz}，由后端按该语言的测试约定判定；骨架指函数体恰为 `gen` 生成的未实现标记。

### 3.2 流程

流程的步骤由 `contains` 边构成一棵树。步骤的形式：

| 形式 | 定义 |
| --- | --- |
| `call` | 同步调用，发起方等待返回；可以带子步骤，即接收方在这次调用中做的事 |
| `send` | 异步调用，发起方不等待 |
| `emit` | 发布一个事件；接收方由订阅关系决定 |
| `async` | 订阅者收到此前发布的事件或此前发送的调用之后做的事 |
| `alt`、`opt`、`loop` | 分支、可选和循环片段 |
| `on` | 前一次调用抛出某个错误时的分支 |

流程可以像性质一样声明符号和限定，它们是流程的输入。

### 3.3 性质、符号与用例

- 性质的形式：

  | 形式 | 陈述成立的范围 |
  | --- | --- |
  | `invariant` | 每个取值 |
  | `postcondition` | 操作返回或抛出错误之后 |
  | `relation` | 几次调用的结果之间，例如往返、幂等、变形关系 |
  | `transition` | 值的每次变化，只按允许的转移 |

- **符号** x 有名称和设计中的类型 type(x)。性质的陈述对符号的每一组取值都成立。
- type(x) = `call<R1, …, Rk>` 时，x 的一个值是对 R1, …, Rk 中某个操作的一次调用，带全部实参；Ri 是类型时指它的全部操作。关于一串调用的性质写成 `forall steps: list<call<Stock>>`；检查时先检查一次陈述，再依次执行这些调用，每一步之后检查陈述（[language.md](language.md) §4.4 E13）。
- **陈述**是性质或流程的全部 `ensure` 表达式，连同性质的 `call` 子句和 `within` 的时长；类型规则和检查步骤见 [language.md](language.md) §4.4。流程的被测调用是它的第一个步骤。不能写成表达式的性质改带 `manual`，称为**人工性质**，由手写的谓词检查。需要环境准备的流程同样显式写 `manual` 理由。
- `replies<R>` 是外部系统操作 R 的非空应答脚本，符号的类型确定安装目标；同一性质或流程对每个目标至多一个这样的符号。接口字段的值以同样的应答列表描述全部操作。成功和声明错误的值形状见 [design.md](design.md) §6.2；末项重复，应答不会在运行中随机生成。
- **纯操作**是声明为 `pure` 的操作：结果只由接收者和实参决定，调用不改变接收者、实参或其他任何值。
- **限定**缩小符号的取值。x 的**取值域** Dom(x) 是 type(x) 的全部取值中满足 x 的全部限定的部分。
- **生成的符号**的取值域不由类型和限定给出，而由 `generates` 绑定的生成器给出（[language.md](language.md) §6）。生成的符号附一句取值域的说明，不带限定。
- **用例** c 对每个符号 x 给出一个值 c(x) ∈ Dom(x)，只给输入，不给期望结果。form(c) = `declared` 的用例写在设计文件中；form(c) = `recorded` 的用例由探索选出，写在录制文件中（[design.md](design.md) §6.3），另有录制原因 ∈ {coverage, counterexample}。

### 3.4 黑盒契约与观察

- 带 `call` 的 `postcondition` 约束一次操作的可观测行为，称为**黑盒契约**。其输入是符号，被测调用给出返回值或错误，`ensure` 声明结果与调用前后值的关系；操作内部的算法不进入契约。
- **前提**是全部 `requires` 表达式的合取，在调用之前、取得调用前观察值之后求值。各符号满足类型和限定，且前提成立时，这组输入适用于该性质。前提可以关联多个符号和调用前观察值；它不改变各符号单独的 Dom(x)。
- **观察范围**由 `observe` 给出，包括直接读取的输入字段，以及由适配器取得的具名快照。直接观察路径是性质的属性；只有具名观察产生 `observation` 节点。观察值不是生成的输入，不增加谓词的参数，也不写进用例赋值。
- **允许变化的范围**由 `changes` 给出，路径在调用前固定。正常返回和错误返回都要求：观察范围中，允许变化的部分之外的值，前后结构相等。范围按模型路径判定；不同路径即使对应共享对象，也各自受约束。
- 范围检查只约束设计中声明的可观测值的最终变化。数据库记录、发出的消息等由只读的观察适配器或测试边界的实际记录提供；没有取得的状态不作为已检查的内容。内部曾经写入、随后恢复的行为，须通过写入记录另行观察。
- 错误的数据由 `failure(E)` 取得，类型是 E；错误匹配规则与 `raised(E)` 相同。求值、快照、范围和执行步骤见 [language.md](language.md) §4.4–§4.5。

### 3.5 证据与评审记录

- `run` 是在一个提交上执行一组检查的一次运行；`result` 是一个检查在一次运行中的结论。
- 黑盒契约的结果另记录有效执行数、拒绝的候选输入数、观察范围，以及失败子句的位置或范围外的实际差异。适配失败是检查环境的错误，不伪造成空快照或业务反例。声明的允许变化范围属于设计，实际差异属于证据。
- 运行中由生成器产生、但没有录制的输入不进入快照。
- 用例执行到的代码和步骤记为 `covers` 边。
- `review`（M7）记录评审者、快照、被评审的节点或边、当时的内容哈希、结论和意见。评审记录不在 Git 中，存放位置在 M7 决定。

## 4 边

| 种类 | 从 → 到 | 来源 | 含义 |
| --- | --- | --- | --- |
| `contains` | container → container、type、op、flow、property；system → actor；actor → type、op；type → op；flow → step；step → step；property、flow → case；property → observation | 声明；抽取；运行 | 包含，每层各构成一棵树 |
| `uses` | container → container、actor | 声明 | 允许的依赖 |
| `extends` | type → type | 声明；抽取 | 继承或嵌入 |
| `implements` | type → type | 声明；抽取 | 实现接口 |
| `refers` | type → type；op → type；property、flow → type、op；observation → type | 声明；抽取 | 字段、参数、返回值、错误、符号和观察引用的类型；契约子句引用的类型与操作；代码层的 op 另有函数体中构造或使用的类型。子句位置和字段路径写在边的属性中 |
| `subscribes` | op → type | 声明 | 操作订阅的事件 |
| `imports` | container → container | 抽取 | 文件导入包 |
| `calls` | op → op | 抽取 | 静态解析到的调用；属性列出每个调用位置 |
| `calls` | step → op | 声明 | `call`、`send` 和 `async` 步骤指向的操作；接收方是该操作所属的类型、容器或外部系统 |
| `from` | step → actor、container、type | 声明 | 步骤的发起方 |
| `emits` | step → type | 声明 | `emit` 步骤发布的事件 |
| `handles` | step → type | 声明 | `on` 片段处理的错误 |
| `about` | property → container、type、op、flow | 声明 | 性质约束的对象 |
| `realizes` | 代码层 type → 设计层 type；代码层 op → 设计层 op | 声明（代码中的绑定）；推导 | 代码元素落实设计元素 |
| `checks` | 代码层 op → property、flow | 声明（代码中的绑定） | 该谓词检查这条性质或这个流程 |
| `generates` | 代码层 op → property、flow | 声明（代码中的绑定） | 该函数给出一个生成的符号的生成器；属性写明符号的名称 |
| `observes` | 代码层 op → observation | 声明（代码中的绑定） | 只读适配器取得这项观察的快照 |
| `substitutes` | 代码层 op → 外部系统直接提供的设计层 op | 声明（代码中的绑定） | 安装该客户端的确定性应答替身，返回清理函数 |
| `owns` | 设计层 container → 代码层 file | 推导 | 文件归属的容器 |
| `depends` | 设计层 container → 设计层 container、actor | 推导 | 实际的依赖 |
| `traces` | step → 代码层 op | 推导 | 落实该步骤所调用操作的函数 |
| `exercises` | 代码层 op → 代码层 op | 推导 | 谓词沿静态调用可达的函数 |
| `reaches` | 代码层 op → 代码层 op | 推导 | 两个显著函数之间只经过细节函数的调用；属性列出经过的函数 |
| `covers` | case → 代码层 op；case → step | 运行 | 用例执行到的函数（属性列出行）和走到的步骤 |

## 5 记法与标签

**投影**只用图的结构：

| 记法 | 选取 |
| --- | --- |
| 架构图 | 容器和它们之间的依赖 |
| UML 类图 | 类型、字段、操作，以及 `refers`、`extends`、`implements` |
| 时序图 | 一个流程，按发起方和接收方分出生命线 |
| 活动图 | 一个流程，按步骤的控制结构 |
| 事件图 | 事件、发布它的步骤和订阅它的操作 |
| 调用图 | 函数之间的 `calls` 和 `reaches` |

**带标签的记法**为元素加上记法自己的词汇。词汇由工具链内置：

| 记法 | 标签 | 适用的种类 |
| --- | --- | --- |
| `ddd` | `bounded-context` | container |
| `ddd` | `aggregate`、`entity`、`value`、`domain-service`、`repository`、`factory` | type |
| `c4` | `software-system`、`container`、`component` | container |

- 元素的标签必须属于系统文件 `notation` 声明的记法，并适用于该元素的种类（第 5 条）。
- 形式为 `event` 的类型就是领域事件，不需要标签。
- 标签只影响视图，不影响约束。各记法的视图见 [design.md](design.md) §7.1。

## 6 共有属性

每个节点和每条边都有下列属性：

| 属性 | 定义 |
| --- | --- |
| 标识 | 见 §7 |
| 种类、形式、层 | 见 §1、§3、§4 |
| 来源 | 见 §2 |
| 出处 | 写出它的位置：设计文件、录制文件或代码文件的路径和行号。推导项的出处是推出它的那些边 |
| 内容哈希 | 自身内容的 SHA-256，不含子节点。节点的内容是种类、形式、摘要和属性（[design.md](design.md) §5.2）的确定编码；代码层 `op` 以 `attrs.body` 含归一化的函数体，代码层 `type` 以 `attrs.declaration` 含不含方法体的声明 |
| 显著性 | 显著或细节，见 §9 |
| 状态 | 只有设计层的声明项和 `depends` 边才有，status ∈ {verified, missing, stub, contradicted}，由 §10 的约束算出 |

状态的含义：`verified` 与代码一致；`missing` 代码中没有对应项；`stub` 对应的函数还是骨架；`contradicted` 代码与声明不符。

## 7 身份

- **设计节点**的标识按 `contains` 树从上到下逐段写出，段之间用 `.` 连接：

  | 节点 | 标识 | 例子 |
  | --- | --- | --- |
  | 系统 | 系统的名称 | `shop` |
  | 参与者、顶层容器 | 名称 | `customer`、`order` |
  | 子容器、类型、操作、流程、性质、用例 | 父节点的标识 . 名称 | `order.pricing`、`order.Order`、`order.Order.total`、`order.submit-order`、`order.total-is-sum.empty` |
  | 容器或外部系统直接提供的操作 | 容器或外部系统的标识 . 名称 | `order.parseCart`、`payment-gateway.charge` |
  | 步骤 | 流程的标识 # 序号 | `order.submit-order#3` |
  | 符号 | 性质或流程的标识 . 名称 | `order.total-is-sum.order`，只用于 `generates` 绑定 |
  | 具名观察 | 性质的标识 . 名称 | `bank.transfer-moves-money.state` |
  | 录制的用例 | 父节点的标识 . `r-` 后接取值的哈希的前 8 位十六进制 | `order.total-is-sum.r-3f9a2c1d` |

  步骤的序号是它在流程中按文本先后的位置，从 1 起，子步骤和片段中的步骤也计入。参与者和顶层容器因此不能与系统同名。
- **代码节点**的标识带语言前缀：

  | 节点 | 例子 |
  | --- | --- |
  | Go 的包、类型、方法 | `go:github.com/x/shop/order`、`go:github.com/x/shop/order.Order`、`go:github.com/x/shop/order.Order.Total` |
  | Python 的方法 | `py:shop.order.Order.total` |
  | 文件 | `file:src/shop/order.py` |

- **边**的标识是 (种类, 起点, 终点)。同一对函数之间有多处调用时，只有一条 `calls` 边，调用位置都列在它的属性中。
- 改名或移动等于删除旧节点、新增新节点。工具可以提示可能的改名，但不会把两者认作同一个节点。

## 8 快照与比较

- **序列化**：节点按标识排序，边按 (种类, 起点, 终点) 排序。相同的输入总是产生逐字节相同的输出。
- **比较**：比较两个快照时，按标识对齐节点和边，每一项归为新增、删除、修改（内容哈希不同）或不变。
- **待审**：一项从未审过，或审过之后内容哈希变了，就是待审。

## 9 推导规则

以下各条定义推导出的边和显著性。

- **D1 `owns`**：owns(c, f) 当且仅当 f 是代码层的文件，c 是路径模式匹配 f 的设计层容器中最深的一个。
- **D2 方法的 `realizes`**：若 realizes(t′, t)，t 的操作为 o，t′ 中有名为 map(o) 的方法 m（名称映射见 [design.md](design.md) §4），则 realizes(m, o)。接口中声明的方法也是 `op` 节点，形式为 `abstract`；经由接口的调用，其 `calls` 边指向它。
- **D3 `depends`**：对设计层容器 A ≠ B，depends(A, B) 当且仅当 A 拥有的某个文件导入了 B 拥有其文件的包，或 A 拥有的某个函数调用了 B 拥有的某个函数。对外部系统 X，depends(A, X) 当且仅当 A 拥有的某个函数调用了落实 X 的某个操作的客户端。边的属性列出这些导入和调用。
- **D4 `traces`**：若步骤 s 的形式为 `call` 或 `send`，calls(s, o)，且 realizes(f, o)，则 traces(s, f)。
- **D5 `exercises`**：若 checks(p, x)，函数 g 从 p 沿 `calls` 可达，则 exercises(p, g)。
- **D6 显著性**：
  1. 设计层的节点都是显著的。
  2. 代码层的类型和函数是显著的，当且仅当它落实设计节点（包括按 D2 落实操作的方法），或带 `checks`、`generates`、`observes` 绑定；但 `gen` 产生的生成文件中的元素都是细节。
  3. 代码层的文件和包是显著的，当且仅当它含有显著的元素。
- **D7 `reaches`**：对显著的函数 f 和 g，reaches(f, g) 当且仅当存在 f → h1 → … → hk → g 的 `calls` 路径，k ≥ 1，且 h1, …, hk 都是细节。属性列出每条这样的路径上的 h1, …, hk。f 直接调用 g 时只有 `calls`，没有 `reaches`。

## 10 约束

违反一条约束得到一条诊断，诊断都是错误。标【状态】的约束同时决定相关声明项的状态；标【评审】的约束决定哪些项列为待审（§8）。

**设计内部**

1. 每个引用都解析到一个声明的节点，节点的种类与引用的位置相符。
2. 名称互不相同：同一父节点的各子节点；参与者与顶层容器；同一条性质或同一个流程中的符号、具名观察与用例。
3. 设计层的 `contains` 构成以 `system` 为根的树。
4. 跨容器的引用，即字段、参数、返回值、错误、订阅的事件、`extends`、`implements`、符号和观察的类型所引用的类型，以及契约子句中引用的操作、枚举和错误类型 t，满足 allowed(container(x), container(t))，其中 x 是引用所在的节点。t 属于外部系统时，用该外部系统代替 container(t)。
5. 元素的每个标签都属于系统文件声明的记法，并适用于该元素的种类（§5）。
6. subscribes(o, E) 时，form(E) = `event`，并且 o 恰有一个参数，其类型为 E。

**流程**

7. 对每个 `call` 和 `send` 步骤 s，设 calls(s, o)，from(s, P)，o 所在的容器或外部系统为 Q：
   1. P 是参与者、容器或类型；P 是人时，s 是最外层的步骤；
   2. Q = container(P)，或 allowed(container(P), Q)；
   3. s 的实参个数等于 o 的参数个数。
8. 调用的每个子步骤，发起方都是该调用的接收方。
9. 对每个 `emit` 步骤 s，设 emits(s, E)，from(s, P)：P 是容器或类型；form(E) = `event`；s 的实参个数等于 E 的字段个数。
10. `on E` 紧跟在一个 `call` 步骤 s 之后，且 E 属于 s 所调用操作的 `raises`。
11. 对每个 `async` 步骤 a，设 calls(a, o)，o 的接收方为 X：o 订阅了流程中先于 a 发布的某个事件，或 o 被先于 a 的某个 `send` 步骤调用；a 的子步骤的发起方都是 X。

**归属与依赖**

12. 容器的路径模式都合乎 doublestar 的写法。每种已配置的语言，其源码根目录下的每个源文件恰被一个容器拥有；互不嵌套的两个容器，路径模式不匹配同一个文件。
13. allowed(A, B) 当且仅当 B 是 A 的子容器，或 A 或 A 的某个祖先 `uses` B；B 可以是外部系统。allowed(A, B) 不延及 B 的子容器。
14. 【状态】每条 depends(A, B) 都满足 allowed(A, B)；否则该 `depends` 为 `contradicted`。

**绑定**

15. 绑定指向存在的设计节点，种类相符：
    1. 代码层类型的 `realizes` 指向类型；
    2. 不属于任何类型的代码层操作，其 `realizes` 指向容器或外部系统直接提供的操作；
    3. `checks` 指向 `property` 或 `flow`，一个函数至多带一个 `checks`；
    4. `generates` 指向生成的符号；
    5. 方法不带 `realizes`；
    6. `observes` 指向具名观察，每个观察适配器是自由函数，恰带一个 `observes` 绑定；
    7. `substitutes` 的安装器是自由函数，恰指向一个外部直接操作；每个目标至多一个安装器，归属与其客户端相同的允许容器。Go 安装器接收与客户端完全同签名的函数，返回清理函数。
16. 【状态】每个类型和每个操作，包括外部系统的，都恰被一个代码层元素落实。没有时为 `missing`；落实操作的函数是骨架时为 `stub`。
17. 若 realizes(x, n)，则 x 所在的文件归属于 container(n)。n 属于外部系统 X 时，x 所在的文件归属于某个 `uses` X 的容器。
18. 【状态】若 realizes(x, n)，则下列各项都成立，否则 n 为 `contradicted`：
    1. x 的名称是 n 的名称的映射（[design.md](design.md) §4）；
    2. 归一化后的签名一致：参数类型的序列、返回值和 `raises` 相同，参数名不比较；
    3. n 的每个字段在 x 中都存在，类型一致；x 中其他的字段和方法不受限制；
    4. n 是 `enum` 时，x 的枚举值集合等于 n 的枚举值集合。
19. 若 implements(t₁, t₂) 或 extends(t₁, t₂)，则落实 t₁ 和 t₂ 的代码层类型之间有同一种边。

**可执行**

20. 每条 `property` 和每个 `flow` 都恰有一个 `checks` 绑定。绑定的函数称为**谓词**；除语言要求的测试句柄（[design.md](design.md) §4）外，谓词的参数恰为它的符号（可以没有），按名称映射一一对应，类型一致。具名观察在谓词内部取得，不增加参数。带陈述的性质和全部流程，谓词在生成文件中，由 `gen` 产生；人工性质的谓词是手写的。
21. 生成文件之外的代码中没有测试函数：其中每个代码层 `op` 的 testKind 都是 `none`。归属按实际声明文件的生成标记判断。
22. 【状态】对每个 `call` 和 `send` 步骤 s，存在 traces(s, f)，并且 f 可达：
    1. s 是调用 s₀ 的子步骤时，从落实 s₀ 所调用操作的函数沿 `calls` 能到达 f；
    2. s 在最外层、发起方是类型或容器时，从落实发起方的代码（类型的方法，或容器拥有的函数）中的某个函数能到达 f；
    3. 发起方是人时，不检查。

    s₀ 调用的是接口的操作时，第 1 项的起点改为实现该接口的代码层类型中的同名方法；发起方是接口类型时，第 2 项的起点改为实现它的各代码层类型的方法；有一个能到达即可。

    对每个 `emit` 步骤 s，设 emits(s, E)：落实发起方的代码 `refers` 落实 E 的代码层类型。

    不满足时，s 为 `missing`。
23. 不是生成的符号 x，其类型能够生成：type(x) 不是 `interface` 或 `error`，不经由必填字段递归引用自身；type(x) 为 `call<…>` 时，其中各操作的参数都能生成，操作名互不相同。`replies<…>` 的返回值和错误字段同样能够生成。类型为 `interface` 的字段不受此限，由替身填充（[design.md](design.md) §6.2）。
24. 每个生成的符号恰有一个 `generates` 绑定，并且不带限定；不是生成的符号没有 `generates` 绑定。
25. 限定的路径从一个符号出发逐段解析：经过 `list`、`set` 时作用于每个元素，经过 `map` 时作用于每个值，经过 `call` 时下一段是操作名、再下一段是参数名。区间只用于 `int` 和 `float`；枚举子集只用于 `enum`；大小只用于 `list`、`set`、`map`、`string`、`bytes` 和 `replies`。
26. 每个用例 c，包括声明的和录制的，对每个符号 x 恰给出一个值，且 c(x) ∈ Dom(x)：记录给出全部字段，枚举值存在，只有 `optional` 取 `none`，调用给出该操作的全部参数，应答脚本非空且全部返回值和错误字段类型相符，并满足全部限定。用例不对具名观察赋值；运行时的前提检查见 [language.md](language.md) E22。
27. 声明的用例的名称不匹配 `r-[0-9a-f]{8}`。

**生成与录制**

28. 每个生成文件（[design.md](design.md) §6.2）都与 `unicode gen` 按当前设计产生的内容逐字节相同。
29. 每个录制文件都指向一个存在的性质或流程，内容是规范格式。

**名称映射**

30. 对每种已配置的语言，设 map 为它的名称映射（[design.md](design.md) §4）：
    1. 同一个容器中，类型、操作、谓词和生成器的 map 名称互不相同；同一个类型中，字段与操作的 map 名称互不相同；
    2. map 可逆，即反向映射得到原来的名称。因此性质的名称不以 `flow-` 开头，性质和流程的名称中短横线之后不紧跟数字。

**表达式**

31. 每个表达式都合乎 [language.md](language.md) §4.4 的名称与类型规则。
32. 【状态】纯操作由驱动在运行时核对：同样的实参调用两次，结果相同，接收者和实参不变；不满足时，该操作为 `contradicted`。
33. 【评审】人工性质的谓词的内容哈希变了，这条性质也列为待审。
34. 每个具名观察恰有一个 `observes` 绑定。适配器的参数是 `from` 按次序列出的输入符号，类型一致，返回该观察的快照类型；适配器不接受测试句柄。Go 可带末尾的 `error`，Python 的异常也按观察失败处理，它们不是业务 `raises`。适配器的文件属于性质所在容器。签名和归属不符时是错误。
35. 观察与允许变化的范围满足 [language.md](language.md) §4.5：快照类型可观察，允许变化的路径属于观察范围，下标的类型正确；它们描述设计约束，不推导实际写入边。
36. 【评审】观察适配器的内容哈希变了，它所绑定观察的父性质列为待审。

## 附录 A 理由

- **来源。** 声明是意图，抽取是事实，推导把二者连起来。§10 的约束比较意图与事实，结果写在声明项的状态上。
- **记法不进入结构。** DDD、UML、时序图、事件风暴和 C4 都只是从同一张图中选取并绘制，因此换一种记法不改设计文件，也不改约束。
- **显著性由设计决定（D6）。** 人在设计中写下的就是他要关注的，不需要另外标注。
- **外部系统由客户端落实（第 16、17 条）。** 外部系统在本仓库中没有自己的代码，本仓库能检查的是调用它的客户端。
- **签名不比较参数名（第 18 条）。** 参数名是给人读的说明；各语言的习惯常要求换名字，例如 Go 中参数不能遮住包名。契约是参数的类型、次序、返回值和错误。
- **枚举值必须相等（第 18 条）。** 字段多出来是细节；枚举多出一个值，就是设计没有声明的状态。
- **按分派检查接口（第 22 条）。** 接口中的 `abstract` 方法没有函数体，本身不调用任何函数，可达性只能从实现它的类型出发。
- **运行时核对的部分（第 22 条）。** 静态可达只说明调用可能发生。调用的先后和事件的投递，由流程的驱动在运行时核对（[design.md](design.md) §6.3）。
- **名称映射可逆（第 30 条）。** 谓词的名称由性质或流程的名称映射而来；`flow-` 开头的性质会与流程的谓词同名。Go 的映射会丢掉短横线，短横线后紧跟数字时无法还原。例如在 Go 中，类型 `Slice` 和操作 `slice` 都映射成 `Slice`，不能放在同一个容器中。
- **陈述是表达式（§3.3）。** 陈述写成表达式，工具链才能检查它的类型并生成执行真实操作的谓词。写不成表达式的性质先拆成原子性质；剩下的声明为人工性质，靠评审保证含义（第 33 条）。
- **观察与变化范围分开（§3.4）。** 观察给出检查边界，允许变化的范围给出边界内的例外，避免把没有观察的值当作保持不变。具名观察独立成节点，是因为它需要绑定和评审适配器；普通字段路径仍是属性。

## 附录 B 规则与诊断码

每条推导规则和约束一行，列出违反它时报出的诊断码，以及生成器违反它的性质或流程（`design/*.uc` 中的标识）。推导规则没有诊断码，由性质把推导的结果与 §9 的定义逐条比较。不属于任何规则的诊断码列在 [design.md](design.md) §8。`make rule-check`（`scripts/rulecodes.py`）检查本表与代码中的诊断码一致。

| 规则 | 诊断码 | 验证的性质 |
| --- | --- | --- |
| D1 | 由性质验证 | `graph.derive-matches-definition` |
| D2 | 由性质验证 | `graph.derive-matches-definition`、`lang.golang.extracts-generated-module` |
| D3 | 由性质验证 | `graph.derive-matches-definition` |
| D4 | 由性质验证 | `graph.derive-matches-definition` |
| D5 | 由性质验证 | `graph.derive-matches-definition` |
| D6 | 由性质验证 | `graph.derive-matches-definition`、`lang.golang.extracts-generated-module` |
| D7 | 由性质验证 | `graph.derive-matches-definition` |
| 1 | `design.unresolved-reference`、`design.wrong-reference-kind` | `design.rejects-broken-design` |
| 2 | `design.duplicate-name` | `design.rejects-broken-design` |
| 3 | `design.missing-system-file`、`design.missing-parent` | `design.rejects-broken-design` |
| 4 | `design.undeclared-dependency` | `design.rejects-broken-design` |
| 5 | `design.unknown-label`、`design.undeclared-notation`、`design.label-not-allowed` | `design.rejects-broken-design` |
| 6 | `design.not-an-event`、`design.invalid-subscription` | `design.rejects-broken-design` |
| 7 | `design.wrong-reference-kind`、`design.nested-person-sender`、`design.undeclared-dependency`、`design.argument-count` | `design.rejects-broken-design` |
| 8 | `design.sender-not-receiver` | `design.rejects-broken-design` |
| 9 | `design.wrong-reference-kind`、`design.not-an-event`、`design.argument-count` | `design.rejects-broken-design` |
| 10 | `syntax.on-without-call`、`design.unraised-error` | `syntax.rejects-broken-text`、`design.rejects-broken-design` |
| 11 | `design.async-without-trigger`、`design.sender-not-receiver` | `design.rejects-broken-design` |
| 12 | `graph.bad-path-pattern`、`check.unowned-file`、`check.overlapping-paths` | `graph.derive-matches-definition`、`check.rejects-broken-graph` |
| 13 | `design.undeclared-dependency`、`check.undeclared-dependency` | `design.rejects-broken-design`、`check.rejects-broken-graph` |
| 14 | `check.undeclared-dependency` | `check.rejects-broken-graph` |
| 15 | `lang.golang.missing-binding-id`、`lang.golang.invalid-binding-id`、`lang.golang.misplaced-binding`、`lang.golang.duplicate-binding`、`graph.unresolved-binding`、`check.wrong-binding-kind`、`check.multiple-checks`、`check.multiple-observes`、`check.not-generated-symbol`、`check.method-binding` | `lang.golang.extracts-generated-module`、`graph.derive-matches-definition`、`check.rejects-broken-graph` |
| 16 | `check.unrealized`、`check.multiple-realizations` | `check.rejects-broken-graph` |
| 17 | `check.realization-outside-container` | `check.rejects-broken-graph` |
| 18 | `check.name-mismatch`、`check.signature-mismatch`、`check.missing-field`、`check.field-type-mismatch`、`check.missing-value`、`check.extra-value` | `check.rejects-broken-graph` |
| 19 | `check.missing-implements`、`check.missing-extends` | `check.rejects-broken-graph` |
| 20 | `check.missing-predicate`、`check.multiple-predicates`、`check.predicate-signature`；Go 生成自动性质和流程入口谓词 | `check.rejects-broken-graph`、`lang.golang.executes-contracts` |
| 21 | `check.handwritten-test`；Go 已启用，Python 未实现：M5 | `check.rejects-broken-graph` |
| 22 | `check.untraced-step`、`check.unreachable-step`、`check.unreferenced-event` | `check.rejects-broken-graph` |
| 23 | `design.ungenerable-symbol` | `design.rejects-broken-design` |
| 24 | `design.limited-generated-symbol`、`lang.golang.duplicate-binding`、`check.missing-generator`、`check.multiple-generators`、`check.not-generated-symbol` | `design.rejects-broken-design`、`lang.golang.extracts-generated-module`、`check.rejects-broken-graph` |
| 25 | `design.invalid-limit-path`、`design.invalid-limit` | `design.rejects-broken-design` |
| 26 | `design.unknown-symbol`、`design.duplicate-value`、`design.missing-value`、`design.value-type-mismatch`、`design.value-out-of-limit` | `design.rejects-broken-design` |
| 27 | `design.recorded-case-name` | `design.rejects-broken-design` |
| 28 | `gen.stale-file`、`lang.golang.stale-contracts` | `gen.check-files-are-current`、`lang.golang.executes-contracts` |
| 29 | `design.unresolved-reference`、`design.wrong-reference-kind`、`cli.not-canonical` | `design.rejects-broken-design`、`cli.check-design` |
| 30 | `design.name-collision`、`design.irreversible-name`；Python 的名称映射未实现：M5；生成器的 map 名称未实现：[design.md](design.md) §4 没有给出生成器的名称映射 | `design.rejects-broken-design` |
| 31 | `design.expression-type` | `design.rejects-broken-design` |
| 32 | `lang.golang.purity-violation`，核对表达式和被测调用的纯性；证据并图后的状态传播属于 M6 | `lang.golang.executes-contracts` |
| 33 | 未实现：M7，评审 | — |
| 34 | `check.missing-observer`、`check.multiple-observers`、`check.observer-signature`、`check.observer-outside-container` | `check.rejects-broken-graph` |
| 35 | `design.invalid-observation`、`design.invalid-change-path`、`lang.golang.expression-failed`、`lang.golang.frame-violation`、`lang.golang.observation-failed` | `design.rejects-broken-design`、`lang.golang.executes-contracts` |
| 36 | 未实现：M7，观察适配器的评审 | — |

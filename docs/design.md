# unicode 工具链设计

unicode 把一个仓库的设计意图、代码和证据组织成一张图（[model.md](model.md)），设计意图写在设计文件中（[language.md](language.md)）。本文写工具链本身：用法、组件、语言后端、建图、生成与运行、视图与切片、命令行、计划，以及工具链对自己的验证。设计理由集中在附录 A，待定的事项列在附录 B。

## 1 目标

- **管理人的上下文**：人只看核心，也就是架构、流程、核心函数，以及每条性质和它的证据；细节在点开时才出现。
- **知识与代码一致**：结构从代码静态抽取，意图由设计声明，二者由检查对齐。没有对齐的知识会明确标出来。
- **设计先行**：先写设计，再按设计生成骨架、生成器和测试驱动，最后写函数体，以及人工性质的谓词。
- **检查都可执行**：每条性质和每个流程都有一个谓词。测试驱动全部由工具生成，按覆盖录制下来的输入构成可以快速回放的回归集。
- **黑盒契约**：输入、前提、输出、观察值和允许变化的范围由设计声明；驱动调用真实实现并核对，不展开函数内部算法。具名观察通过只读适配器接入。此路径不依赖定理证明器或 SMT 求解器。
- **结构统一，记法可换**：图只有一套结构（[model.md](model.md) §1），DDD、UML、时序图、事件风暴、C4 都是它的视图。

## 2 使用方式

工具链的使用者是设计者兼评审者，可以是人，也可以是 Claude Code 的主会话。实现由设计者派给 agent 完成。一轮工作分六步：

1. 设计者编写或修改设计文件：组件、类型、流程，以及带符号和用例的性质，直到 `unicode check --design-only` 通过。
2. 设计者运行 `unicode gen`，为缺少的设计节点生成骨架，并重写生成器、由陈述生成的谓词和驱动。
3. 设计者把要实现的设计节点派给 agent，任务附上这些节点的切片（`unicode slice`，§7.3）。
4. agent 写函数体和人工性质的谓词，自己运行 `unicode check` 和 `unicode test`（快路径），然后交回。
5. 设计者用 `unicode check`、`unicode test`、`unicode diff` 和视图评审交回的结果。通过就合入；不通过就带着诊断再派一次。
6. 慢路径 `unicode test --explore` 按时间预算另外运行，例如每晚一次。它把增加覆盖的输入和找到的反例录制下来，经评审后合入。

工具链不调度 agent，也不保存任务。它只提供设计、生成、检查、运行和查询这几类能力。

## 3 组件

工具链是一个 Go 程序，module 为 `github.com/lincyaw/unicode`，命令为 `unicode`。每个包是一个组件，M1 时写进工具链自己的设计文件。

| 包 | 职责 | 依赖 |
| --- | --- | --- |
| `internal/diag` | 诊断：位置、诊断码、原因与修正 | — |
| `internal/graph` | 图的类型、合并、推导、内容哈希、序列化和比较 | diag |
| `internal/syntax` | 设计文件和录制文件的词法、语法、语法树和规范格式的打印 | diag |
| `internal/design` | 把语法树解析成设计层的节点和边；检查设计内部、流程、符号与用例的约束 | syntax、graph、diag |
| `internal/lang` | 语言后端的接口、名称映射、生成计划和运行计划的类型 | graph、diag |
| `internal/lang/golang` | Go 后端：抽取、名称映射、契约生成与执行 | lang、graph、diag、syntax |
| `internal/lang/python` | Python 后端，嵌入抽取脚本 | lang、graph、diag |
| `internal/check` | 归属、依赖、绑定、可执行、生成与录制这几组约束 | graph、diag |
| `internal/query` | 节点、引用、搜索、切片和影响 | graph |
| `internal/view` | 视图的选取与折叠 | graph |
| `internal/serve` | 查看器的后端：读取快照，载入时按 `check` 的约束检查，回答 §7.5.1 的接口 | graph、diag、check、query、view、lang.golang |
| `internal/gen` | 生成计划：缺少的骨架，以及应有的生成文件 | graph、lang、diag |
| `internal/run` | 回放与探索：经后端运行驱动，按覆盖挑选输入，缩减并写出录制文件 | graph、lang、syntax、diag |
| `cmd/unicode` | 解析参数，调用其余组件，输出结果和退出码 | 全部 |
| `python/uc` | Python 运行时包 `uc`（装饰器），随工具链发布 | — |

`graph` 在最底层：设计层由 `design` 写入图，代码层由语言后端写入图，证据由 `run` 写入图，其余组件只读图。

凡是有维护中的外部库能满足需要的地方，都用外部库，只有没有维护中的库、或者库满足不了需要时才自己写。选用的库：

| 用途 | 库 |
| --- | --- |
| 设计文件和录制文件的词法与语法：[language.md](language.md) §3 的语法写成它的语法结构体 | `github.com/alecthomas/participle/v2` |
| 命令行的命令、参数和帮助 | `github.com/spf13/cobra` |
| 路径模式中的 `**` | `github.com/bmatcuk/doublestar/v4` |
| 值的显示宽度（规范格式的 100 列） | `github.com/mattn/go-runewidth` |
| 原子地改写文件，保留权限 | `github.com/google/renameio/v2` |
| 深度验证中的变异测试 | `github.com/go-gremlins/gremlins` |
| 生成器、性质测试与 fuzz 的驱动 | `pgregory.net/rapid` 与 Go 原生的 fuzz |
| Go 代码的抽取（M2） | `golang.org/x/tools/go/packages` 与 `go/types` |
| 骨架的 module 路径读取 | `golang.org/x/mod/modfile`，沿用 Go 工具链依赖的版本 |
| 查看器的接口：用 TypeSpec 写，生成 OpenAPI 3 文档（§7.5.1） | `@typespec/compiler`、`@typespec/http`、`@typespec/openapi`、`@typespec/openapi3`，1.18.0 |
| `serve` 的接口类型和路由：由 OpenAPI 文档生成 Go 代码 | `github.com/oapi-codegen/oapi-codegen/v2` v2.8.2，生成的代码依赖 `github.com/oapi-codegen/runtime` v1.9.0 |
| `serve` 的性质按 OpenAPI 文档校验每个应答 | `github.com/getkin/kin-openapi` v0.142.0（`openapi3filter` 与 `routers/legacy`），即 oapi-codegen 所依赖的版本 |
| 前端的接口类型和请求：由 OpenAPI 文档生成 TypeScript 类型，按类型发出请求 | `openapi-typescript` 7.13.0 与 `openapi-fetch` 0.17.0 |

规范格式的打印没有现成的库，由 `syntax` 自己完成。

表达式前端由 `syntax.parseExpr` 和 `syntax.exprText` 提供，供设计载入和 Go 生成共用。表达式属性保存规范的源引用拼写，解析后的语义引用另存为 `refers`；不能直接把局部引用替换成绝对名称，否则可能与输入符号的名称冲突。

查看器的接口定义在 `api/` 中：TypeSpec 源文件 `api/*.tsp`、编译配置 `api/tspconfig.yaml`，以及由它们生成并提交的 `api/openapi.yaml`。`api/` 用 pnpm 管理 TypeSpec 的版本，不属于任何 Go 组件。

## 4 语言后端

每种语言有一个后端，实现以下接口（草案）：

```go
type Backend interface {
	Language() string
	Extract(root string, srcRoots []string) (Extracted, error)
	MapName(role NameRole, designName string) string
}

type Generator interface {
	Language() string
	Generate(plan GenPlan) Generation
}

type Runner interface {
	Language() string
	Run(plan RunPlan) RunReport
}
```

- **抽取**：后端读出 `container`、`type`、`op` 节点，以及 `contains`、`imports`、`calls`、`refers`、`extends`、`implements` 边。它同时读出代码中的绑定，写成 `realizes`、`checks`、`generates` 和 `observes` 边；绑定的写法不合格式时，给出诊断。带 `realizes` 绑定的类型，它的每个方法按反向的名称映射（例如 Go 的 `Submit` 对应 `submit`）得到一条到 `<类型的标识>.<操作名>` 的 `realizes` 边，来源为推导；推导时，终点不存在的这类边被丢掉，那些方法是细节。代码层 `op` 的 `signature` 属性按下面的类型映射归一化成设计的写法，引用写成从根起的标识，与设计层的签名直接按字符串比较。后端产出的是代码层的图片段，格式与快照相同。
- **Go 后端**：在进程内用 `golang.org/x/tools/go/packages` 和 `go/types` 解析源码和类型。调用按静态类型解析，接口方法的调用解析到接口的方法。`implements` 按 Go 方法集的类型身份判断，参数名和返回值名不参与比较；包的测试变体仍按同一包路径识别类型，有符号性、整数宽度与可变参数等真实类型差异保留。抽取包括 `_test.go` 文件：谓词、生成器和只供它们使用的类型都在其中，它们的绑定照常读取。
- **Python 后端**：工具链内嵌一个只用标准库的抽取脚本，以 `python3 -I` 运行。脚本用 `ast` 解析源码，把图片段以 JSON 写到标准输出。调用按名称和导入关系解析，解析不到的调用不成边，只计数。生成在 Go 中按模板完成。
- **运行**：后端用该语言的测试工具运行驱动，在进程内逐个输入记录覆盖：Go 用 `-cover` 构建，并用 `runtime/coverage` 在每个输入前后清零和读出计数；Python 用标准库的 `sys.monitoring`。覆盖只统计谓词沿 `exercises` 可达的函数，不含生成文件和谓词本身。
- **测试类别**：Go 中，`func FuzzX(f *testing.F)` 为 `fuzz`，调用了 `rapid.Check` 的 `TestX` 为 `property`，其余 `TestX` 为 `example`。Python 中，带 `hypothesis.given` 的测试为 `property`，用 `atheris` 的为 `fuzz`。后端据此找出生成文件之外的测试函数（[model.md](model.md) §10 第 21 条）。
- **名称映射**：

  | 设计 | Go | Python |
  | --- | --- | --- |
  | 类型 `Order` | `Order` | `Order` |
  | 操作 `unitPrice` | 方法或函数 `UnitPrice` | `unit_price` |
  | 字段 `unitPrice` | 字段 `UnitPrice` | 属性 `unit_price` |
  | 枚举值 `draft` | 常量 `OrderStatusDraft` | 成员 `DRAFT` |
  | 性质 `total-is-sum` 的谓词 | `checkTotalIsSum` | `check_total_is_sum` |
  | 流程 `submit-order` 的谓词 | `checkFlowSubmitOrder` | `check_flow_submit_order` |

- **谓词的签名**：参数依次是该性质或流程的符号，按名称映射，类型一致。Go 的谓词另有第一个参数作为测试句柄，类型为 `rapid.TB`；Python 的谓词没有测试句柄，用 `assert` 断言。
- **类型映射**：归一化签名用设计的类型写法，各语言的写法与它一一对应。例如：
  - `int` 对应 Go 的 `int` 和 Python 的 `int`；
  - `list<T>` 对应 `[]T` 和 `list[T]`；
  - `optional<T>` 对应 `*T` 和 `T | None`；`object` 类型在 Go 中按指针传递，`T` 和 `optional<T>` 都写作 `*T`；
  - `bytes` 对应 `[]byte`，`set<T>` 对应 `map[T]struct{}`，`map<K, V>` 对应 `map[K]V`，`float` 对应 `float64`，`time` 和 `duration` 对应 `time.Time` 和 `time.Duration`；
  - 落实设计类型的代码类型，写作该设计类型的标识；其余的代码类型写作它的代码标识，与设计的比较因此不相等；
  - `call<…>` 和 `replies<…>` 对应生成文件中按完整模型类型稳定命名的载体，以 `model-type` 元数据保留精确类型。

  - `enum` 对应 Go 的 `type X string` 和常量 `X<值>`，常量的值是设计中枚举值的名称；对应 Python 的 `enum.Enum`，成员的值也是设计中的名称。

  操作只有一个返回值；要返回几个结果时，写成一个记录。在 Go 中，`raises` 对应最后一个返回值 `error`，这是多返回值唯一的用法；在 Python 中，错误不出现在签名里。Python 中落实设计节点的函数和谓词，每个参数和返回值都必须有类型标注。
- **生成器的签名**：带 `generates` 绑定的函数，参数是在该符号之前声明的符号中它用到的那些（按声明的次序，可以没有），返回该符号类型的生成器：Go 为 `func genX() *rapid.Generator[T]`，Python 为返回 Hypothesis strategy 的函数。
- **观察适配器的签名**：参数是观察的 `from` 按次序列出的输入符号，类型一致，返回快照类型；Go 允许末尾另有 `error`，Python 的异常也按观察失败处理。它不接收测试句柄，不使用谓词的名称映射。一个适配器恰绑定一项观察，文件属于性质所在容器。
- **可选能力**：`lang.Backend` 只要求抽取与名称映射；后端可以另实现 `lang.Generator` 和 `lang.Runner`，两者均有 `language`。`generate(GenPlan)` 返回 `Generation`，不直接写仓库；`run(RunPlan)` 返回逐性质的 `RunReport`。计划、生成文件和报告的字段以 `design/lang.uc` 为准。`gen.plan` 按系统的语言根目录调度，`gen.write` 预检全部路径、重复目标、符号链接和生成标记后逐文件原子写入；不承诺跨文件事务。`run.execute` 按目标标识调度后端，并原子写出 `.unicode/results.json`。

## 5 建图

1. `syntax` 解析 `design/` 下的设计文件和 `corpus/` 下的录制文件。`design` 把它们解析成设计层的节点和边，并检查用例的值是否符合符号的类型和限定。
2. 每种已配置的语言，由它的后端抽取源码根目录下的代码。
3. `graph` 合并设计层和代码层，再按 [model.md](model.md) §9 推导出 `owns`、方法的 `realizes`、`depends`、`traces`、`exercises`、显著性和 `reaches`。最近一次运行的结果（`.unicode/results.json`）作为证据并入。
4. `check` 检查 [model.md](model.md) §10 的约束，把结果写成诊断和声明项的状态。
5. `build` 把快照写到 `.unicode/graph.json`，搜索索引写到 `.unicode/index.json`，仓库所在的提交和工作区是否有改动写到 `.unicode/build.json`。快照中不含提交，相同的输入因此总是得到相同的快照。`.unicode/` 不进 Git。

相同的输入总是得到逐字节相同的输出。M3 起，抽取结果按文件缓存，键为文件内容的 SHA-256 和后端的版本，只重新抽取变了的文件。

### 5.1 只需设计就能判断的约束

`design` 载入设计文件和录制文件时检查以下各项，诊断码写作 `design.<短横线名称>`。`unicode check --design-only` 报出的就是它们，加上语法和规范格式的诊断。

- [language.md](language.md) 中的语义规则：
  - `design/system.uc` 恰有一个，`design/` 中其余每个文件声明的组件，其标识与文件路径相符；录制文件的路径与它指向的性质或流程的标识相符；
  - 引用按 [language.md](language.md) §4.1 的规则解析；
  - 顶层组件写了 `focus`；
  - 标签属于 `notation` 声明过的记法，并且能用在这种元素上；
  - 性质的陈述：G14；表达式的名称与类型（[language.md](language.md) §4.4 的 E1 至 E3 和类型表）。
- [model.md](model.md) §10：
  - 设计内部的第 1 至 6 条，流程的第 7 至 11 条；
  - 符号与用例的第 23、25、26、27 条，以及第 24 条中"生成的符号不带限定"的一半；
  - 第 29 条，以及名称映射的第 30 条（撞名为 `design.name-collision`，映射不能往返为 `design.irreversible-name`）；
  - 表达式的第 31 条。

其余约束要看代码，M2 由 `check` 检查。

### 5.2 节点的属性

图中每个节点的摘要字段是它的摘要，其余内容写在属性中。属性的值都是字符串的列表，类型和值都按规范格式写出。类型和节点引用写成绝对标识；表达式与状态路径保留规范源拼写，并由 `refers` 保存已解析的引用：

| 种类 | 属性的键 |
| --- | --- |
| container | `focus`（取实际生效的关注度，子组件继承父组件的）、`labels`、`paths`（设计层）；`notations`、`roots`（只有 `system` 才有，`roots` 每项写作 `语言: 目录`）；`language`、`path`（代码层） |
| type | `labels`、`fields`（每项 `名称: 类型`）、`values` |
| op | `signature`（`(参数: 类型, …) -> 类型 raises 错误, …`）、`pure`（纯操作为 `true`，其余不写出） |
| flow、property | `symbols`（每项 `名称: 类型`，生成的符号另加 ` generated`）、`limits`（每项是一条限定的规范写法）、`call`（`call` 子句的规范写法）、`within`（时长）、`ensure`（每项一条表达式的规范写法，按书写次序）、`manual`（人工性质的原因） |
| property 的黑盒契约 | `requires`（按次序的前提表达式）、`observe`（直接观察路径、具名观察声明或 `none`）、`changes`（允许变化的路径或 `none`）；`call` 只保存调用表达式，不带关键字 |
| observation（snapshot） | `type`（快照类型）、`inputs`（`from` 列出的输入符号，保持次序） |
| step | `index`、`label`、`args` |
| case | `values`（每项 `符号 = 值`）、`reason`（录制的用例） |

代码层的节点由语言后端写出，属性的键如下。凡是名称，都先按反向的名称映射换回设计的写法（例如 Go 的 `UnitPrice` 写作 `unitPrice`），`check` 因此不必知道名称映射，直接与设计比较：

| 种类 | 属性的键 |
| --- | --- |
| container（`package`、`file`） | `language`、`path`（文件的路径，或包的导入路径）；工具生成的文件另带 `generated: true` |
| type | `name`、`fields`（每项 `名称: 类型`；未导出的字段是细节，不列出）、`declaration`（规范类型声明）、`values`（以字符串为底层类型、带常量的类型，每项是常量名按反向映射得到的枚举值，例如 `OrderStatusDraft` 写作 `draft`） |
| op（`func`、`method`、`abstract`） | `name`（带 `checks` 绑定的谓词，写作性质或流程的名称）、`signature`、`testKind`（`none`、`example`、`property` 或 `fuzz`）、`stub`（`true` 或 `false`） |

`calls` 边的属性 `sites` 列出每个调用位置，写作 `路径:行:列`；`generates` 边的属性 `symbols` 列出符号的名称。Go 签名的写法：指针 `*T` 写作 `optional<T>`，`check` 比较时把设计中 `object` 类型的 `optional<X>` 与 `X` 视为相同；末尾的 `error` 写作 ` raises error`，与设计中非空的 `raises` 相符；谓词的第一个参数（`rapid.TB`）保留在签名中，比较符号时跳过。绑定边的终点是设计层的标识。`Graph.AddFragment` 先暂存终点不在图中的绑定边和方法的候选边；推导时加入能解析的，绑定边中终点仍不存在的报为 `graph.unresolved-binding`，即 [model.md](model.md) §10 第 15 条中"目标存在"的一半；其余端点缺失的边仍是 `DanglingEdge`。

`alt` 中的步骤属于哪个分支，写在从 `alt` 到该步骤的 `contains` 边的属性 `branch` 上。空的列表不写出。

契约的 `refers` 边以 `clauses` 列出引用所在的子句，如 `requires#1`、`ensure#2`，以 `paths` 列出该引用访问的规范字段路径；同一对端点的引用合并，属性去重并排序。具名观察通过自身的 `refers` 引用快照类型。子句的具体源位置由生成器重新解析原文件、按性质标识和子句序号取得，写入生成的检查代码，不把行列号加入语义属性。

`extends`、`implements`、`uses`、订阅、调用、发起方和 `about` 都是边，不是属性。快照是 `{"nodes": [...], "edges": [...]}`：节点按标识排序，边按种类、起点、终点排序，属性的键按字典序排列。

### 5.3 需要代码才能判断的约束（M2）

`check` 在推导过的图上检查以下各项，诊断码写作 `check.<短横线名称>`，并给出每个声明项的状态：[model.md](model.md) §10 的第 12、14 至 20、22、34 条，以及第 24 条中"每个生成的符号恰有一个 `generates` 绑定"的一半。第 21 条对 Go 启用：实际声明文件没有本工具的生成标记时，测试入口报 `check.handwritten-test`；谓词和生成器不属于测试入口。Python 的驱动生成与本项检查在 M5 交付。第 28 条由 `unicode gen --check` 核对生成计划，并进入 `make check`；运行前仍核对将执行的生成文件。推导由 `graph` 完成，路径模式用 doublestar 匹配。

M2 的验收：`make check` 中加入 `unicode check`，在工具链自身上通过。

代码层 `op` 的 `attrs.body` 是去掉注释和出处的规范 Go AST 函数体；包括函数体中的闭包，接口抽象操作没有此属性。代码层 `type` 的 `attrs.declaration` 是同形规范类型声明，不含方法体。以新的空 `token.FileSet` 对 AST 节点作 `go/format` 打印，排版和位置不进入值。比较只改函数体的工作区时也必须改变操作内容哈希；顶层变量的赋值跟踪仍按现有模型，不扩大为运行证据。

## 6 生成与运行

`unicode gen` 产生两类文件：骨架和生成文件。`unicode test` 运行生成的驱动。

M4 包括 Go 骨架、单次黑盒契约、调用序列、流程入口、接口与外部客户端替身，以及统一的回放、性质测试和原生 fuzz 驱动。覆盖挑选、缩减录制、流程内部轨迹核对和运行证据并图属于 M6；本节的快路径与预算内随机探索不依赖这些能力。生成或执行失败给出定位诊断，不生成空检查或报告通过。

开发验证保留默认 rapid 次数。普通 `go test` 先回放声明与录制用例，再完整执行 rapid 的配置次数，不按墙钟时间跳过剩余次数。只有显式 `unicode test --explore DURATION` 使用每条性质的时间预算；快路径 `unicode test` 只回放。`make check` 的 Go 包级超时为 20 分钟，容纳 CLI 性质反复载入临时模块的累计耗时，不增加探索次数或启动 `make deep`。

经单独授权的 `make deep` 可配置 `RAPID_CHECKS`、`FUZZ_TIME` 与 `DEEP_TIMEOUT`；最后一项默认 `0`，避免高次数运行被普通包级超时截断。配置这项能力不启动深测。

每条可执行性质与流程另有一个原生入口 `FuzzUnicode_<完整ID的SHA-256小写十六进制>`，交给 `rapid.MakeFuzz` 从字节构造输入。名称只取决于完整设计标识，与声明次序无关，长度固定并保存 Go 原生 corpus 的归属；入口注释写明设计标识。入口与普通测试共用按声明次序生成输入和调用谓词的逻辑；随机测试、探索和 fuzz 向人工谓词传入真正的 `*rapid.T`，声明用例逐个以 `*testing.T` 回放一次。拒绝候选由 fuzz 跳过，不当作域外回放。fuzz 核对单个输入的关系与范围，`within` 的第 95 百分位由批次性质测试、回放或探索核对。每次执行结束或失败都恢复驱动的上下文，报告互不串用。生成声明与手写代码或另一项生成声明冲突时给出诊断。

旧驱动迁移逐条核对谓词、符号次序、自定义生成器及额外断言。原驱动使用的非通用分布以 `generated` 和 `generates` 明确绑定；专门的输入子域声明独立性质，保留各自的完整测试次数。迁移后删除手写 `TestX/FuzzX` 入口，手写谓词、生成器与独立预期值仍保留。`gen` 只改写工具拥有的文件；本次手写入口由开发迁移删除，生成器不猜测、改写任意手写断言。Go 的普通测试、fuzz 与新增性质入口此后统一随 `unicode gen` 更新，`check` 拒绝生成文件之外的 Go 测试入口。

`unicode gen --check` 以同一生成计划逐字节核对全部目标，缺失或陈旧时报错，不创建目录、改写文件或跟随符号链接。它与 `--dry-run` 互斥，JSON 保留完整生成计划与诊断，成功的文本输出为空。`make check` 在运行 Go 测试之前检查格式、生成内容、自身绑定、规则码和 API 产物，避免在已知陈旧的代码上重复测试。

生成规划使用后端可选的 `lang.GenerationExtractor`：Go 后端以只读的 `go/packages` 内存覆盖把本工具拥有的旧生成测试替换为空包文件，再抽取手写代码和绑定。只匹配源码根目录内、常规文件且带本工具精确生成标记的测试文件，不跟随符号链接，不忽略手写源码错误，也不修改磁盘。Go 生成器自身加载代码时使用同一规则。普通 `check/build/test` 仍严格读取实际生成文件；生成不能被陈旧输出的类型错误阻止，否则接口改名后无法自我修复。

Go 的旧生成测试若已不再承载任何性质，仍进入生成计划并改写为空包，保留生成标记和构建条件，清除陈旧谓词与入口。这样删除或移动性质后，旧驱动不会继续编译和运行；只读检查同样核对这些文件。手写文件与其他工具的生成文件不参与这项清理。

单次契约按 [language.md](language.md) E12、E17–E23 执行。生成文件首行使用 `// Code generated by unicode. DO NOT EDIT.`；Go 检查与被测代码放在同一包的生成测试文件中，测试时用包路径和固定入口选择运行，输入在每次运行内重新构造。生成器使用真实 Go 签名构造调用与值，不从归一化签名猜测指针。执行结果包含有效执行数、拒绝候选数、观察边界、范围外差异和子句诊断；观察错误不记为业务反例。未获得有效执行不能报告通过。表达式中的纯操作会重复调用并比较结果、接收者与实参；被测操作声明为纯时，也重复调用；冻结第一次的返回值或声明错误，比较两次结果与错误数据，以及调用前后接收者和实参的模型值，包括无返回值的操作。第二次调用用于纯性检查，后置陈述读取第一次结果，计时也只计第一次。纯性诊断指向实际操作并保留子句位置；运行结论并入快照后的状态传播属于 M6。

### 6.1 骨架

骨架只在缺少时生成一次，之后归人或 agent 所有。`gen` 只新增，从不改写已有的代码元素。计划中的 `Edit.createOnly` 为真时，即使目标带生成标记也不能覆盖；预检全部目标后以独占创建写入。骨架不带工具生成标记。再次建图运行 `gen` 时已有绑定不再产生该骨架；重放旧的只新增计划须报覆盖冲突。新声明以稳定的逐节点文件写出，已有类型缺少的方法写进新文件，不改用户文件。空组件使用其路径模式中唯一确定的目录，已有包沿用包名，新包按目录和 module 确定；不能唯一确定时诊断，不猜路径。生成前通过只读内存覆盖加载骨架并解析绑定，所以一次计划同时包含骨架和引用它们的驱动；目录尚不存在也能规划，普通抽取仍然严格。`--dry-run` 和 `--check` 均不写骨架。它为以下各项生成骨架：

- 每个没有落实的类型：带 `realizes` 绑定的类型，字段和操作按名称映射，方法体是未实现标记。未实现标记在 Go 中是 `panic("unicode: unimplemented")`，在 Python 中是 `raise NotImplementedError("unicode: unimplemented")`，函数体恰好是这一句时就是骨架。
- 每个没有落实的、容器或外部系统直接提供的操作：带 `realizes` 绑定的自由函数。外部系统的操作，生成在 `uses` 它的容器中，作为客户端。
- 每条没有谓词的人工性质或人工流程：带 `checks` 绑定的谓词，参数是它的符号。
- `replies` 输入需要但缺少的外部安装器：带 `substitutes` 绑定的自由函数骨架。先加载真实客户端与客户端骨架，再用其精确原生签名生成安装参数，返回 `func()` 清理函数；默认名称为 `install` 加外部系统和操作名称的大驼峰形式，再加 `Double`。函数体仍是未实现标记，依赖安装器的目标保持 `stub`。
- 每个缺少自定义生成器的 `generated` 符号：带 `generates` 绑定的骨架，接受全部前置符号并返回该符号的 rapid 生成器；用户可实现后收窄未用的依赖。默认名称为 `gen` 加性质或流程名称的大驼峰形式，再加符号的首字母大写形式，流程在名称前加 `Flow`；同一包撞名时诊断，绝不覆盖已有函数。已有自定义生成器的名称由绑定确定，可服务多个符号，不强制改名。

### 6.2 生成文件

生成文件归 `gen` 所有，每次运行都按当前的设计整体重写，人不编辑。文件开头标明它是生成文件。`unicode check` 确认它们与 `gen` 的输出逐字节相同（[model.md](model.md) §10 第 28 条）。生成文件包括：

- **生成器**：每个符号一个，按它的类型递归构造：
  - 内置类型取该语言的生成器，区间和大小作为边界；
  - `record`、`object` 和 `event` 由各字段的生成器组合而成；
  - `enum` 从枚举值或限定的子集中取；
  - `list`、`set`、`map` 按大小的限定生成；
  - `optional` 在 `none` 和 T 的值之间取；
  - 字段的类型为 `interface` 时，取该接口的替身；
  - `call<…>` 先选一个操作，再按它的参数类型和限定生成实参。

  生成的符号不按类型构造，用 `generates` 绑定的生成器。其余符号的取值域（[model.md](model.md) §3.3）只写在设计中一处，生成器和用例的检查都由它推出。
- **谓词**：每条带陈述的性质和每个流程一个，按 [language.md](language.md) §4.4 的检查步骤把 `call`、`within` 和 `ensure` 翻译成该语言的代码，带 `checks` 绑定；自动流程的谓词执行流程的第一个同步步骤。该步骤的实参必须是流程的输入符号且类型逐一匹配；类型方法须有唯一的该类型输入作为接收者。步骤树中的内部调用不由驱动重复执行，实际操作自行执行它们；内部顺序、事件和分支的运行核对属于 M6。需要文件系统、HTTP 等环境准备或独立关系断言的流程写 `manual` 理由并绑定手写谓词，生成驱动保留相同输入与独立预期值。`int` 用该语言的任意精度整数，例如 Go 的 `math/big`。人工性质没有生成的谓词。
- **调用的执行**：每个 `call<…>` 类型有一个执行器，把一个调用值作用到目标上，并返回该操作的结果或错误。有状态的谓词先检查初态，再逐步保存 `old`、调用和检查全部陈述；声明错误之后继续下一步，未声明错误则失败。Go 的调用值及应答脚本用生成的具名载体保存；工具拥有的类型声明上的 `//uc:model-type 类型表达式` 只用于将载体反映射到精确的 `call<…>` 或 `replies<…>`，不产生落实边。注释只在工具生成文件有效，生成规划的覆盖保留这些不依赖运行时的类型声明，以供手写生成器引用。
- **替身**：接口字段以实现完整原生接口的具体类型填充；每个声明操作的应答脚本是输入值的一部分，在调用之前生成，不在业务执行中抽随机数。接口值写成操作名到非空应答列表的记录。成功应答恰写 `{returns: 值}`，无返回值写 `none`；失败应答恰写 `{raises: "绝对错误标识", fields: {字段…}}`，错误须在该操作声明的错误或其后代中。每次调用取下一项，末项重复，输入有限而行为确定。

  原生接口中用于该边界的方法须有模型声明，未建模的方法不能以默认值假装应答。

  外部自由函数不能直接替换。它通过显式的 `substitutes` 安装器接入：参数恰为与真实客户端原生 Go 签名完全相同的函数，返回恰为 `func()` 清理函数；不修改客户端源码或机器码。自动契约沿静态调用可达的每个外部操作须有唯一安装器，以及一个 `replies<该操作>` 符号提供非空应答脚本。安装先于观察和前提，清理覆盖拒绝、失败和 panic；安装失败或空清理函数是环境错误。普通调用仍使用真实客户端。
- **驱动**：每个谓词一个，以三种方式把输入交给谓词：
  - **回放**：把声明的用例和录制的用例转写成该语言的值，逐个交给谓词；
  - **探索**：由生成器产生输入，失败时把输入缩小到最小的反例；
  - **fuzz**：把 fuzz 引擎给出的字节，经生成器解码成各符号的取值。

  驱动每调用一次纯操作，就再调用一次，比较两次的结果、接收者和实参（[model.md](model.md) §10 第 32 条）；有 `within` 时，记下被测调用的耗时（[language.md](language.md) §4.4 E16）。
- **语言的对应**：

  | | Go | Python |
  | --- | --- | --- |
  | 生成器 | `rapid.Custom` | Hypothesis 的 strategy |
  | 探索 | `rapid.Check` | `@given` |
  | fuzz | `FuzzX` 中的 `rapid.MakeFuzz` | Hypothesis 的 `fuzz_one_input` 接到 atheris 上 |
  | 回放 | 普通的 `TestX`，逐个调用谓词 | 逐个调用谓词 |

  Go 后端在落实代码所在包生成 `unicode_contracts_gen_test.go`，一个包一个文件，普通入口是 `TestUnicodeContracts`，各性质按标识作为子测试；原生 fuzz 入口按完整标识稳定命名。自动谓词带 `checks` 绑定，人工谓词直接复用；人工流程的 `within` 计量整个手写谓词边界，准备更细的边界时须拆分声明。所有模式共享输入与谓词，执行语义见本节开头；覆盖引导的模型用例录制仍按后续阶段交付。

### 6.3 回放与探索

每个谓词的输入有两个来源：声明的用例（人写的）和录制的用例（探索挑选的）。运行分快、慢两条路径。

- **快路径** `unicode test`：只回放声明的和录制的用例，结果是确定的。它是每次修改之后都跑的回归。加 `--since REV` 时，比较基准与工作区的节点及边，同时沿两张图的 §7.4 影响传播，选择当前仍存在的性质与流程；变化的用例归到其检查，删除的依赖仍从基准图传播。基准建图有诊断时阻断运行，不能据不完整图漏选。显式未知标识先报错，合法标识与受影响集合取交集。没有受影响检查是成功的无工作结果，保留上次真实运行的 results.json，不能把空集合当成全部运行；被选中的检查没有有效执行仍报错。
- **慢路径** `unicode test --explore DURATION`：在给定的时间内，按性质和流程分配预算，由生成器产生新的输入：
  1. 开始时，先丢弃已经不符合当前设计（[model.md](model.md) §10 第 26 条）的录制用例，再回放其余的用例，得到已有覆盖的并集 U。
  2. 每产生一个输入，就运行谓词并记录它的覆盖 C。C 中有 U 没有的部分时，这个输入以 `coverage` 为原因保留，U 并入 C。
  3. 谓词失败时，把输入缩小成最小的反例，以 `counterexample` 为原因保留，然后结束这条性质的探索。
  4. 探索结束后做缩减：声明的用例和反例都保留，再按贪心的集合覆盖，从录制的 `coverage` 用例中选出能保持 U 的最小子集。每条性质或流程至多保留 64 个录制用例。
  5. 加 `--record` 时，把结果写回 `corpus/` 下的录制文件，规范格式，按用例名排序。
- **覆盖**分两种，都写成 `covers` 边：
  - **代码覆盖**：谓词沿 `exercises` 可达的函数中执行到的行；
  - **设计覆盖**：流程中走到的步骤和分支，以及被调用操作声明的错误中实际抛出过的那些。
- **测试好不好**，用覆盖来衡量：一组用例的代码覆盖率、流程分支的覆盖率和错误的覆盖率，都显示在性质视图中。没有带来新覆盖的输入不录制，所以录制集保持小而有效。
- **流程的核对**：流程的驱动在运行时记录落实设计操作的函数被进入的次序和发布的事件，与流程的步骤树比较。次序不符时，谓词失败。
- 结果写到 `.unicode/results.json`，下一次建图时作为证据并入。

## 7 视图、切片与查询

### 7.1 视图

视图由三部分组成：从图中选取哪些节点和边、默认怎样折叠、元素按什么次序排列。视图只给出结构，不给出坐标，布局由前端完成。

**投影**只用结构，对任何项目都可用：

| 视图 | 输入 | 选取 |
| --- | --- | --- |
| 架构 `architecture` | 容器，缺省为整个系统 | 子容器、外部系统、`uses`、`depends` 及其状态；展开的容器另带其中的类型、操作、流程、性质和错误，类型带字段和枚举值与代码的逐项比较 |
| 类 `class` | 容器或类型 | 类型及其字段和操作、类型之间的边，以及被引用的其他容器中的类型（所在的容器折叠）；可以在设计层和落实它们的代码层类型之间切换 |
| 时序 `sequence` | 流程 | 步骤树、按首次出现排列的生命线、嵌套的激活、发送和发布，以及每一步的 `traces`、状态和设计覆盖 |
| 活动 `activity` | 流程 | 同一个步骤树，按控制结构画出 |
| 事件 `events` | 容器，缺省为整个系统 | 事件、发布它的步骤和订阅它的操作；错误、抛出它的调用步骤和处理它的 `on` 片段 |
| 调用 `calls` | 操作或函数 | 显著函数之间的 `calls` 和 `reaches` |
| 性质 `property` | 性质或容器 | 性质、`about`、符号、限定、用例（声明的和录制的）、谓词，以及覆盖率和最近的结果 |
| 代码 `code` | 文件或类型 | `contains`、进出的调用，以及覆盖到它的用例 |

**带标签的记法**只在系统文件声明了该记法时可用：

| 视图 | 输入 | 选取 |
| --- | --- | --- |
| DDD `ddd` | 容器，缺省为整个系统 | 带 `bounded-context` 的容器，其中带 DDD 标签的类型和事件，以及它们之间的 `refers`、`extends`、`implements` |
| C4 `c4` | 容器，缺省为整个系统 | 按 C4 的标签分两层：系统上下文（人、系统、外部系统）和容器（带 `container` 标签的容器）；边是依赖，标上经过它调用的操作 |

时序视图和调用视图合起来回答"这个流程怎样走、经过哪些核心函数"。

### 7.2 折叠与展开

视图默认只显示显著的节点（[model.md](model.md) §9）。架构、类、调用、代码、DDD 和 C4 视图还按容器的关注度折叠；时序、活动、事件和性质视图不折叠，流程的每个参与者、被调用的操作、发布的事件，以及性质约束的对象，都画出来，因为它们是设计者写进流程或性质的。折叠的规则：

- `core` 的容器展开。
- `support` 的容器折叠成一个节点，只显示名称、摘要和进出它的依赖。
- `utility` 的容器不画出来，指向它的依赖也不画，视图边上只给出"另有 n 个工具与基础服务"。
- 细节函数不显示。显著函数之间经过细节函数的调用，画成一条 `reaches` 边，标明经过了几个细节函数。

视图的输入 `expand` 列出要展开的标识：

- 展开一个折叠的或未画出的容器，显示它的内容；
- 展开一条 `reaches` 边，显示路径上经过的细节函数；
- 展开一个显著函数，显示它直接调用的细节函数。

视图的输入节点、包含它的容器和展开的标识都不折叠。展开一条 `reaches` 边时，标识写作 `reaches <起点> <终点>`。输入是细节节点或不存在的标识时，视图不适用；命令行先检查标识是否存在，不存在时退出码为 3。

指向折叠的容器的边合并成一条，标明合并了几条；合并的边没有单独的预览。

展开的状态只存在于视图的输入里，工具链不保存它；前端把它写在页面地址中。

### 7.3 切片

切片是回答一个问题所需的最小上下文。人在详情中看到的，和派给 agent 的任务所附的，是同一种切片。一组节点 N 的切片包含：

- N 中每个节点的完整声明、状态和出处；
- 每个节点到 `system` 的祖先链，只给摘要；
- 每个节点沿各种边一步可达的显著节点，只给摘要；细节节点只给个数，展开之后才列出；
- 按种类补充的内容：
  - 操作：所属类型的字段和各操作的签名、订阅的事件、约束它的性质、调用它的流程步骤、落实它的函数的位置；
  - 性质：它约束的对象的签名、各符号的类型及其字段、限定、前提、观察与允许变化的范围、具名观察的快照类型和适配器、用例，以及谓词的位置；
  - 具名观察：父性质、快照类型及其字段、输入符号的类型，以及适配器的位置；
  - 流程：符号、步骤树和每一步的 `traces`；
  - 容器：子容器、`uses`、带状态的 `depends`，以及类型的摘要。

按种类补充的内容放在切片的 `extras` 中，焦点节点本身不重复列出，其他部分之间允许重复（例如操作所属的类型既是祖先，也在 `extras` 中）。性质的符号类型沿 `refers` 只经过类型逐层展开；具名观察另从父性质的 `symbols` 解析 `inputs` 对应的输入类型，展开其字段类型及快照类型，并包含 `observes` 的适配器；流程给出整棵步骤树和每一步 `traces` 的终点；容器要补充的内容都在一步之内，没有 `extras`。`get` 的边按种类、起点、终点排序。

切片的 `edges` 是两端都在切片列出的节点之中的 `contains` 边，由它还原祖先链和流程的步骤树。

代码只给位置（路径和行号）；加 `--with-code` 时附上源码，JSON 中为 `code`：标识到源码，源码是包含该节点的顶层声明及其文档注释所占的各行，由 Go 后端的 `declaration` 给出，查看器的源码片段也用它。目前只有 Go 后端提供，Python 到来时成为后端接口的一个操作。

### 7.4 影响与索引

- **影响**：对一组变更的节点 C，影响是沿 `calls`、`reaches`、`refers`、`subscribes`、`emits`、`realizes`、`traces`、`exercises`、`depends`、`about`、`checks`、`observes`、`substitutes` 反向能到达 C 的节点。结果按种类分组，每一项附上一条最短的路径，说明它为什么受影响。默认只列显著的节点。
- **绑定的影响**：代码的实际变化还沿 `realizes`、`checks`、`generates`、`observes` 和 `substitutes` 正向到绑定的设计节点，因此只改函数体、谓词或生成器也能选中相关检查。另从变更的适配器沿 `observes` 正向到观察，再沿该观察的 `contains` 反向到父性质；不沿其他 `contains` 传播。观察类型变化仍沿 `refers` 和 `observes` 反向影响适配器。说明路径保留图中边的原始方向，观察绑定可以逆向读出，因此路径是一串首尾相接的原图边，不要求每一条都按 `from` 到 `to` 遍历。
- **索引**：可以按标识、名称、种类和摘要文字查找节点，并按边反查，例如谁调用了它、谁订阅了这个事件、哪些性质约束它。

### 7.5 查看器

查看器只渲染图，不另存数据。视觉规范是 Figma 原型（文件 `inTTk3ypjVqoB9tHq0ErO4`），样式取自 aegis-bench-site；界面的改动先画进原型，再实现。

查看器由工具链提供：`unicode serve` 启动一个小型后端，读取本地 `unicode build` 编译出的数据，提供查询与交互的接口，并提供前端页面。接口用 TypeSpec 定义，由各视图需要显示的内容推出（§7.5.1）；前端的源码在 `web/`，只通过这些接口取数据。

- **视图是入口，实体页是落点。** 顶部的视图标签是探索的入口；类型、操作、流程、性质等节点各有实体页。图上的节点、设计树的条目、链接都能进入实体页，实体页中的引用又能继续进入下一页。右栏在视图中预览选中的节点，在实体页中显示导航路径。视图、选中项和导航路径都记在地址里。
- **页面只放数据。** 说明性的文字，例如小节的说明、图例、读法和计数口径，收进标题旁的 ⓘ，悬停时显示；页面上不出现"点击……""另有……"之类的提示句。
- **能用图标的用图标。** 节点种类、操作、状态和链接用图标表示，不写成文字标签，也不用手打的符号。图标取自 Lucide（前端为 `lucide-react`），16px，笔画 1.4，颜色随文字。状态用圆点：绿色为已验证，黄色为缺失，空心为未实现，红色为不一致。
- **类型卡片显示内容。** 类型卡片列出设计中写出的字段、枚举值和操作签名，字段最多 6 个，其余折叠为"+n"；代码中多出的字段算细节，不列出。折叠的组件列出其中的类型名称。
- **单击预览，双击进入。** 单击节点在右栏预览；双击节点，或点右栏标题旁的 ↗，进入实体页。
- **用词正式**，统一按下面的术语表。

| 种类 | 图标 |
| --- | --- |
| 系统、组件、外部系统、参与者 | `network`、`box`、`cloud`、`user` |
| 类型（object、record）、枚举、接口、错误、事件 | `braces`、`list`、`puzzle`、`octagon-x`、`zap` |
| 操作、细节函数 | `square-function`、`code` |
| 流程、步骤、性质、用例 | `workflow`、`corner-down-right`、`shield-check`、`flask-conical` |
| 进入实体页、展开、比较、诊断 | `arrow-up-right`、`chevron-right`、`git-compare-arrows`、`triangle-alert` |
| 回放、探索、模糊测试、分支、异步 | `play`、`dices`、`repeat`、`split`、`radio` |
| 提交、文件、隐藏的节点、说明 | `git-commit-horizontal`、`file-code`、`eye-off`、`info` |

| 含义 | 页面用词 |
| --- | --- |
| 状态 verified、missing、stub、contradicted | 已验证、缺失、未实现、不一致 |
| `realizes` | 实现 |
| `calls` 的两端 | 调用方、被调用方 |
| `refers` 的两端 | 被引用、引用 |
| 节点出现在其中的流程 | 参与的流程 |
| 以节点为 `about` 的性质；性质的 `about` | 相关性质；约束对象 |
| 性质的 `forall` 与 `where` | 输入 |
| 用例的来源 | 声明、录制、反例 |
| 覆盖的三种度量；三者合称 | 代码覆盖率、分支覆盖率、错误覆盖率；测试充分性 |
| 一个用例新增的覆盖 | 覆盖增量 |
| 步骤落到的代码 | 代码追溯：实现、调用位置 |
| 关注度 `focus` | 关注级别 |
| 与早先提交的差别 | 变更 |
| 评审记录 | 评审：未评审、已评审 |
| 实体页的来路 | 导航路径 |

#### 7.5.1 接口

**数据来源。** `unicode serve` 读取 `unicode build` 写出的快照 `.unicode/graph.json`，载入时按 `unicode check` 的约束检查一遍，得到诊断和每个声明项的状态。快照文件改变后（按修改时间和大小判断），下一次请求时重新读入；每个请求只从一个快照回答。工作区所在的提交、分支和是否有改动，在读入快照时由 git 给出。比较的基准由后端按查询的 `--rev` 的方式取出该提交并建图，在进程内保留。

**约定。** 接口只读：视图、输入、展开、选中项和导航路径都记在页面地址中，服务端不保存状态。标识中含有 `.`、`#`、`:` 和 `/`，一律放在查询参数里，不放在路径中。接口的源文件是 `api/*.tsp`，`api/openapi.yaml` 由它生成，`make api-check` 确认二者一致；各字段的含义和取法以源文件中的说明为准。证据（运行、结果、覆盖，M6）和评审记录（M7）的字段是可选的，图中有了这些数据之后才给出。

**形状。** 已有的形状都沿用，字段名与命令行的 JSON 相同：

- 节点和边是快照中的节点和边（§5.2、[model.md](model.md) §8），节点带内容哈希 `digest`。节点另有 `title`（页面上的名称）、`status`（`check` 给出的状态）、代码元素的 `bindings`（绑定边及其指向的设计节点）、步骤的 `step`（所在流程、序号、发起方、目标、接收方、外层步骤和子步骤）；边另有 `status`。
- 诊断与 `unicode check --format json` 的相同。一条边加另一端的节点沿用 `query.Neighbor`，切片沿用 `query.Context`，影响沿用 `query.Affected`，单个节点沿用 `query.NodeView`。
- 视图在 `view.View`（`kind`、`nodes`、`edges`、`folded`、`hidden`）之上，加上输入节点、它的祖先和视图中各项的诊断。类型卡片带 `members`，即字段和枚举值与代码的逐项比较；容器卡片带 `contents`，展开时是其中的类型、操作、流程、性质和错误，折叠时是其中类型的名称；代码文件和代码类型带 `details`，即未画出的细节函数的个数。边的一端是折叠的容器时，`count` 是它代表的边数；`labels` 是边承载的操作。
- 拒绝的请求返回 `{code, message, fix}`：标识、边、文件或接口不存在为 404 `serve.not-found`；视图不适用于输入为 400 `serve.not-applicable`，提交读不出为 400 `serve.bad-revision`，路径不是设计文件、录制文件或快照中的代码文件为 400 `serve.bad-path`，参数缺失或不合类型（包括不存在的视图、节点或边的种类）为 400 `serve.bad-parameter`。快照文件在运行中变得读不出时，请求得到 503 `serve.unreadable-snapshot`，文件恢复后照常回答；比较要等 `graph.Compare`，在此之前为 501 `serve.not-implemented`；服务端自身的失败为 500 `serve.internal-error`。

**接口。** 路径都在 `/api` 之下：

| 接口 | 内容 | 对应的命令 |
| --- | --- | --- |
| `GET /snapshot` | 快照的元数据：系统节点、读入时工作区所在的提交、快照文件及其写出时间，以及节点、边、诊断和各状态的计数 | — |
| `GET /tree` | 设计树：设计层的节点按 `contains` 组成的树，不含步骤和用例 | — |
| `GET /views/{kind}` | 十种视图之一，参数 `input`、可重复的 `expand`，类视图另有 `layer`（设计层或代码层），C4 视图另有 `level`（系统上下文或容器） | `unicode view` |
| `GET /entity?id=` | 实体页：共有的各部分，以及按节点种类的一部分；带 `base` 时附上变更 | — |
| `GET /edge?kind=&from=&to=` | 边的预览 | — |
| `GET /nodes?id=…` | 按标识取一组节点，用于导航路径 | — |
| `GET /node?id=` | 一个节点及其出边和入边 | `unicode node get` |
| `GET /search?text=` | 搜索，可按 `kind` 过滤 | `unicode node search` |
| `GET /slice?id=…` | 切片，参数 `expand` 和 `withCode` | `unicode slice` |
| `GET /impact?id=…` | 影响 | `unicode impact` |
| `GET /diagnostics` | 全部诊断和状态 | `unicode check` |
| `GET /code?path=&from=&to=` | 一个文件中的若干行 | — |
| `GET /compare?base=` | 与基准提交的快照比较，列出新增、删除和修改的节点和边 | `unicode diff <rev>` |
| `GET /revisions` | 当前分支最近的提交，供比较按钮选择基准 | — |

架构、事件、领域和 C4 视图不给 `input` 时，输入是整个系统。实体页的共有部分是：节点及其祖先、诊断、相关视图、实现、相关性质、参与的流程、代码、覆盖它的用例（M6）、证据（M6）、变更和评审（M7）；按种类的部分是容器（也用于参与者）、类型、操作、流程、性质、步骤、用例和代码元素各一种。参与的流程是这样一些步骤：调用该节点或它的操作、所调用操作的签名引用该节点、发布或处理该节点，或者由作为人的该节点发起。

**界面元素与字段。** 每个原型中的元素由下表的接口和字段提供；接口一栏省略 `/api`，同一行的多个元素用顿号分开：

| 界面元素 | 接口 | 字段 |
| --- | --- | --- |
| 顶栏的系统名 | `/snapshot` | `system.title` |
| 顶栏的搜索 | `/search` | 节点的 `title`、`kind`、`form`、`summary` |
| 标题区的路径和标题 | `/views/{kind}`、`/entity` | `ancestors`、`input.title`；`ancestors`、`node.title` |
| 视图标签 | `/snapshot`、`/entity` | 领域和 C4 是否可用看 `system.attrs.notations`；切换后的输入取自选中项的 `views` |
| 标题区和状态栏的诊断计数 | `/snapshot`、`/diagnostics` | `counts.diagnostics`；`diagnostics` |
| 比较按钮 | `/revisions`、`/compare` | 提交列表；`base`、`nodes`、`edges` |
| 状态栏的分支和提交 | `/snapshot` | `head.branch`、`head.short`、`head.dirty` |
| 设计树 | `/tree` | `node.title`、`node.status`、`node.attrs.focus`、`children` |
| 右栏的预览 | `/entity`、`/edge` | 选中的节点或边，各部分同下文的实体页 |
| 右栏的导航路径 | `/nodes` | `title`、`kind`、`form` |
| 架构：组件卡片、类型卡片（01） | `/views/architecture` | 组件的 `attrs.focus`、`attrs.labels`、`attrs.paths`；`contents`；类型的 `members`，操作的 `attrs.signature` |
| 架构：折叠的组件、隐藏的工具组件（01、12） | `/views/architecture` | `folded`、`contents`；`hidden` |
| 架构：依赖边及其诊断（01、07、12） | `/views/architecture` | 边的 `kind`、`status`、`attrs.imports`、`attrs.calls`；`diagnostics` |
| 领域（07） | `/views/ddd` | 类型的 `attrs.labels`；refers、extends、implements 边 |
| 类：层的切换、类型卡片（08） | `/views/class` | `layer`；`members`、操作的 `attrs.signature` |
| 类：其他组件中被引用的类型（08） | `/views/class` | 折叠的组件及其 `contents`；边的 `count` |
| 时序：生命线、步骤、激活、片段（02） | `/views/sequence` | `nodes` 的次序；步骤的 `step`、`attrs.args`、`attrs.label`；from、calls、emits、handles 边，contains 边的 `attrs.branch` |
| 时序：步骤的状态和诊断卡片（02） | `/views/sequence` | 步骤的 `status`；`diagnostics` 中 `subject` 为该步骤的各条 |
| 活动（09） | `/views/activity` | 同时序 |
| 事件：发布、事件、订阅；抛出、错误、处理（10） | `/views/events` | 步骤的 `step`（发起方、目标、子步骤）；emits、subscribes、handles 边；`diagnostics` |
| 调用：函数、reaches 边、折叠的组件（04） | `/views/calls` | 函数的 `bindings`（设计名称与状态）；calls、reaches 边，reaches 的 `attrs.via`；边的 `count` |
| 调用：展开与收起（04） | `/views/calls` | 请求的 `expand` |
| 性质：陈述、约束对象、输入（03） | `/views/property` | 性质的 `summary`、`attrs.symbols`、`attrs.limits`；about 边及其另一端操作的 `attrs.signature` |
| 性质：谓词、驱动（03） | `/views/property` | checks 边的起点及其 `origin`；`evidence.drivers`（M4） |
| 性质：用例表、测试充分性（03） | `/views/property` | 用例的 `form`、`attrs.values`、`attrs.reason`；`evidence.outcome`、`evidence.coverageGain`、`evidence.adequacy`（M6） |
| 代码：文件中的类型和函数（11） | `/views/code` | 代码元素的 `bindings`；`details` |
| 代码：进出的调用（11） | `/views/code` | calls、reaches 边；边的 `count`；被调用方按 `origin.path` 分组 |
| 覆盖到它的用例（11 的画布，02、09 的右栏） | `/views/code`、`/entity` | covers 边及其起点的用例；`coveredBy`、`evidence.branches`（M6） |
| C4：层级切换、人、组件、外部系统、带操作名的边（12） | `/views/c4` | `level`；`nodes`；边的 `labels` |
| 实体页：标题、标签、出处（05、06） | `/entity` | `node` 的 `title`、`status`、`attrs.labels`、`origin`；`realizedBy` 中代码元素的 `origin` |
| 实体页：字段表（05） | `/entity` | `type.members` 的 `type`、`codeType`、`refers`、`status` |
| 实体页：操作表（05） | `/entity` | `type.ops` 的 `op`、`realizedBy`、`properties` |
| 实体页：被引用、引用（05、07、08） | `/entity` | `type.referredBy`、`type.refers` |
| 实体页：参与的流程（05、06、08、10、12） | `/entity` | `flows`，各步骤的 `step` |
| 实体页：相关性质（06、07、08、09） | `/entity` | `properties`；性质的 `status`，诊断见 `diagnostics` |
| 实体页：代码（05、06） | `/entity` | `sources` |
| 实体页：相关视图（全部） | `/entity` | `views` |
| 实体页：变更、评审（05、06） | `/entity?base=` | `changes` 的 `attrs`、`linesChanged`；`review`（M7） |
| 操作页：签名、签名中的类型（06） | `/entity` | `node.attrs.signature`；`op.signatureTypes` |
| 操作页：调用方、被调用方（06） | `/entity` | `op.callers`、`op.callees`，reaches 边的 `attrs.via` |
| 组件预览：属性、依赖、包含、代码（01） | `/entity` | `node.attrs`；`container.dependsOn`、`container.dependedOnBy`、`container.contents`、`container.files` |
| 外部系统预览：包含、签名、被依赖、实现（12） | `/entity` | `container.contents` 及其操作的 `attrs.signature`；`container.dependedOnBy`；`realizedBy` |
| 事件预览：字段、发布、订阅（10） | `/entity` | `type.members`、`type.emittedBy`、`type.subscribers`；`diagnostics` |
| 步骤预览：发起方与目标、签名、调用方、被调用方、实参（02、09） | `/entity` | `node.step` 的 `sender`、`target`、`receiver`，目标的 `attrs.signature`；`node.attrs.args` |
| 步骤预览：子步骤、所在调用、处理的错误（02、09） | `/entity` | `step.children`、`step.call`、`node.step.target` |
| 步骤预览：代码追溯（02、09） | `/entity` | `step.traces`；`step.callSites` 中 calls 边的 `attrs.sites` |
| 用例预览：取值、来源、历史、覆盖的函数（03） | `/entity` | `node.attrs.values`、`node.attrs.reason`；`evidence.history`、`case.covers`（M6） |
| 函数预览：实现、位置、签名、调用方、被调用方、细节函数（11） | `/entity` | `node.bindings`、`node.origin`；`code.declaration`、`code.callers`、`code.callees`、`code.details` |
| reaches 预览：经过的细节函数、终点、相关步骤、展开（04） | `/edge` | `nodes`、`toNode`、`steps`、`views` |

## 8 命令行

命令行的使用者既是人，也是 agent，约定遵守 cli-design 的规则：标准输出只放数据，诊断和日志写到标准错误；每个命令都支持 `--format text|json`；不读标准输入，不提示确认。

| 命令 | 作用 | 写入 |
| --- | --- | --- |
| `unicode fmt [--check] [path...]` | 把设计文件和录制文件改写成规范格式；带 `--check` 时只报告，不改写 | 设计文件、录制文件 |
| `unicode check [--design-only]` | 建图，检查全部约束，输出诊断 | 无 |
| `unicode build [--out DIR]` | 建图，写出快照和索引 | `.unicode/` |
| `unicode gen [--dry-run / --check]` | 重写生成文件、只读输出计划，或只读核对当前文件；包含只新增的骨架 | 生成计划、文件或诊断 |
| `unicode test [--since REV] [id...]` | 快路径：回放声明的和录制的用例 | `.unicode/results.json` |
| `unicode test --explore DURATION [--record] [id...]` | 慢路径：在预算内探索，按覆盖挑选；带 `--record` 时写回录制文件 | `.unicode/results.json`、录制文件 |
| `unicode diff <rev> [--to REV]` | 比较两个快照：`<rev>` 的与工作区的，带 `--to` 时与 `REV` 的 | 无 |
| `unicode node get <id>` | 给出一个节点及其边 | 无 |
| `unicode node search [--kind K] [--] <text>` | 按名称和摘要搜索；以 `-` 开头的文字写在 `--` 之后，参数写在它之前 | 无 |
| `unicode node refs <id>` | 给出指向它的边 | 无 |
| `unicode slice <id>... [--expand ID]... [--with-code]` | 切片 | 无 |
| `unicode impact <id>...` | 影响 | 无 |
| `unicode view <kind> <id> [--expand ID]...` | 视图的数据 | 无 |
| `unicode schema` | 输出命令、参数和各命令输出的 JSON Schema | 无 |
| `unicode serve [--snapshot FILE] [--addr HOST:PORT] [--web DIR]` | 启动查看器：读取快照，回答 §7.5.1 的接口，并提供前端页面 | 无 |

- **全局参数**：`--root DIR` 指定仓库根目录，缺省为向上找到的第一个含 `design/system.uc` 的目录；`--rev REV` 让查询读一个提交的快照，缺省读工作区。
- **退出码**：0 成功；1 未知错误；2 参数错误；3 标识不存在；5 冲突（`gen` 要写的位置已有不同的内容）；7 环境错误（找不到 `python3` 或 Go 工具链）；10 存在诊断；11 有谓词未通过。
- **`check`**：建图并检查全部约束；设计有错时，各阶段照样运行，诊断全部报出。JSON 为 `{"diagnostics": [...], "statuses": {...}}`。代码无法抽取时给出 `cli.extract-failed`，位置在系统文件中该语言的块，退出码为 10；找不到语言的工具链时退出码为 7；尚无后端的语言给出 `cli.unsupported-language` 并跳过。
- **`build`**：建图有诊断时（不含 `check` 的约束）不写快照，退出码为 10；JSON 为 `{"diagnostics": [...], "snapshot": "路径"}`。快照之外，`build` 在同一目录写出 `build.json`，即 `{"commit": "…", "dirty": false}`：`commit` 是仓库所在提交的完整哈希，`dirty` 表示根目录下有没有未提交的改动或未跟踪的文件，`build` 自己写出的 `graph.json` 和 `build.json` 不算。不在 Git 的工作区中、还没有提交或找不到 `git` 时，写出 `{"commit": null, "dirty": true}`。`--out DIR` 把这两个文件写到 `DIR` 中。搜索索引 `index.json` 等查看器确定格式后再写。
- **查询**：`node`、`slice`、`impact` 和 `view` 每次都按工作区重新建图；`--rev REV` 只用于这些查询，以 `git archive` 取出该提交的文件到临时目录。查询中的标识（包括 `--expand` 的）不存在时退出码为 3；视图不适用于输入时退出码为 2。`view architecture` 和 `view events` 可以不给标识，表示整个系统。JSON 的字段名是设计中的名称（小驼峰）。
- **`diff`**：`unicode diff <rev>` 像查询的 `--rev` 一样取出 `<rev>` 并建图，与按工作区建出的图比较；带 `--to REV` 时，与 `REV` 的图比较。两张图按 [model.md](model.md) §8 比较（`graph.Compare`）：节点按标识对齐，内容哈希不同为修改；边按种类、起点、终点对齐，属性不同为修改；只列出新增、删除和修改的项。与查询一样，建图的诊断由 `check` 报出，`diff` 不报。JSON 与 `api/compare.tsp` 的 `Comparison` 相同：`{"base": 提交, "head": 提交, "nodes": [...], "edges": [...]}`。提交是 `{commit, short, branch, subject, time}`，`branch` 只在该提交正是检出的分支所在时给出，`time` 是提交时间（UTC）；`head` 另有 `dirty`，与工作区比较时同 `build.json` 的 `dirty`，与提交比较时为 `false`。`nodes` 中每项是 `{id, change, before, after, attrs}`，`change` 为 `added`、`removed` 或 `modified`，`before`、`after` 是两边的节点（与 `node get` 的节点相同），只在该边有它时给出；`attrs` 按键排序，列出两边的值不同的键，每项 `{key, removed, added}` 是该键上去掉和加上的值，按各自原来的次序，重复的值按出现次数计。`edges` 中每项是 `{kind, from, to, change, before, after}`。文本先写两端的提交，再在 `nodes` 和 `edges` 两节中每项一行，修改的节点之后每个变了的键一行。`<rev>` 或 `REV` 不是提交时退出码为 2，找不到 `git` 时为 7。`diff` 不接受 `--rev`。
- **`serve`**：`--snapshot` 缺省为根目录下的 `.unicode/graph.json`，`--addr` 缺省为 `127.0.0.1:8765`。`serve` 在 `/` 下提供页面，在 `/api` 下提供接口。页面是 `--web DIR` 给出的目录中的文件，缺省为根目录下的 `web/dist`（存在时）；地址不是其中的文件时给出 `index.html`。没有页面目录时只提供接口，并在标准错误中说明。前端由 `web/` 构建到 `web/dist`；前端定型之后，构建结果用 Go 的 `embed` 嵌入程序，作为没有页面目录时的页面。`--web` 给出的不是目录时是参数错误。开始监听后，标准输出写出页面的地址（JSON 为 `{"url": …}`），每个请求的日志写到标准错误；收到中断信号时退出，退出码为 0。快照不存在或读不出时退出码为 7，提示先运行 `unicode build`；地址无法监听时退出码也为 7。`serve` 不接受 `--rev`，比较的基准由接口的 `base` 参数给出。
- **`check --design-only`**：解析 `design/` 和 `corpus/` 下的全部文件，报出语法、规范格式和 §5.1 的诊断；JSON 为 `{"diagnostics": [...]}`。M2 之前，不带 `--design-only` 的 `check` 是参数错误。
- **路径参数**：按工作目录解析，必须是根目录下 `design/` 或 `corpus/` 之内的文件或目录。路径不存在、不在这两个目录之内，或者找不到缺省的根目录，都是参数错误。目录会被遍历。`design/` 或 `corpus/` 中 `KindOf` 拒绝的文件给出诊断 `cli.not-a-design-file`，读不出的文件给出 `cli.unreadable-file`，符号链接给出诊断 `cli.symlink`，既不跟随读取，也不改写。全局参数也可以写在命令之后。
- **改写与输出**：改写是原子的，并保留文件的权限，失败时不留下临时文件；写入或输出失败时退出码为 1。`cli.not-canonical` 的位置是与规范格式第一处不同的字节。`--help` 写到标准输出，退出码为 0。JSON 缩进两个空格。带 `--format json` 时，诊断是数据，写在标准输出的 JSON 中，标准错误只放日志；文本模式下，诊断写到标准错误，标准输出只列出改写了的或不是规范格式的文件。
- **诊断的 JSON**：`{code, subject, path, line, column, message, fix}`。`code` 是稳定的诊断码，写作 `<组件>.<短横线名称>`，例如 `syntax.unexpected-token`；每条约束有一个或几个，修正办法不同的子规则各用一个。`syntax` 和 `cli` 的诊断，对象是文件的路径。
- **生成的 JSON**：与 `lang.Generation` 同形，为 `{edits: [{path, content, createOnly}], diagnostics: [...]}`；`content` 是 UTF-8 源码字符串，不是字节的 base64。`--dry-run` 返回同样的计划，不写文件。普通生成遇到诊断时不应用计划，文本模式只列生成文件的路径。已有文件缺少生成标记属于覆盖冲突，退出码为 5。
- **运行的 JSON**：与 `lang.RunReport` 同形，为 `{results: [{id, passed, accepted, rejected, observations, differences, diagnostics}], diagnostics}`，数组为空时也输出 `[]`。顶层诊断表示工具、观察、前提、域外输入、无有效执行或结果写入错误，退出码为 10；工具链不存在仍为 7。没有顶层诊断且有 `passed = false` 的性质时退出码为 11，其内部诊断定位 `ensure`、范围或后置表达式的失败。适配错误可另关联到该性质，但顶层必须保留。命令输出和 `.unicode/results.json` 均使用小驼峰字段，所有层级的诊断都使用上面的扁平 JSON 形状；`diag.Diagnostic` 的 JSON 编解码统一实现此形状。
- **诊断码与规则**：违反规则的诊断码按规则列在 [language.md](language.md) 和 [model.md](model.md) 的附录 B 中。不属于任何规则的诊断码如下：

  | 诊断码 | 原因 |
  | --- | --- |
  | `cli.unreadable-file` | 文件读不出，属于输入输出 |
  | `cli.symlink` | 符号链接既不跟随读取，也不改写，属于文件系统 |
  | `cli.extract-failed` | 代码无法加载，或源码根目录不存在，属于环境 |
  | `cli.unsupported-language` | 系统文件配置的语言尚无后端，属于工具链的能力 |
  | `serve.not-found` | 查看器请求的标识、边、文件或接口不存在，属于请求 |
  | `serve.not-applicable` | 视图不适用于请求的输入，属于请求 |
  | `serve.bad-revision` | 请求的基准提交读不出，属于请求 |
  | `serve.bad-path` | 请求的路径不是快照描述的文件，属于请求 |
  | `serve.bad-parameter` | 请求的参数缺失或不合类型，属于请求 |
  | `serve.unreadable-snapshot` | 快照文件在运行中读不出，属于环境 |
  | `serve.not-implemented` | 查看器的比较尚未实现，属于工具链的能力 |
  | `serve.internal-error` | 服务端未能回答请求，属于工具链自身 |
  | `gen.duplicate-backend` | 同一种语言注册了多个生成后端 |
  | `gen.unsupported-language` | 配置的语言没有可用生成能力 |
  | `gen.duplicate-path` | 生成计划多次写入同一个路径 |
  | `gen.invalid-path` | 生成路径不规范、越出仓库或与其他目标存在父子冲突 |
  | `gen.symlink` | 生成目标或其父路径是符号链接 |
  | `gen.unowned-file` | 目标已有非工具生成的文件，属于覆盖冲突 |
  | `gen.write-failed` | 预检或原子写入生成文件失败 |
  | `gen.read-failed` | 只读核对无法读取生成路径，属于环境，退出码为 7 |
  | `lang.golang.driver-name-collision` | 生成的声明与手写代码或另一项生成声明重名 |
  | `run.duplicate-backend` | 同一种语言注册了多个运行后端 |
  | `run.unsupported-language` | 目标语言没有可用运行能力 |
  | `run.unknown-check` | 目标不是已绑定到配置语言的性质或流程 |
  | `run.results-write-failed` | 结果文件不可安全写入 |
  | `run.invalid-report` | 后端报告缺失、重复、包含未选目标，或无有效执行却报告通过 |
  | `run.no-checks` | 所选范围没有可执行性质或流程 |
  | `lang.golang.invalid-plan` | Go 生成计划没有图 |
  | `lang.golang.generate-failed` | 生成所需的 Go 包不能载入，或生成源码不能解析 |
  | `lang.golang.missing-contract-package` | 性质没有可定位的 Go 落实包 |
  | `lang.golang.missing-generator` | 生成符号缺少唯一的 Go 生成器绑定 |
  | `lang.golang.generator-dependency` | 输入生成器依赖未声明或尚未生成的符号 |
  | `lang.golang.unsupported-case` | 声明用例不能构造成受支持的 Go 值 |
  | `lang.golang.run-failed` | Go 测试进程不能执行、编译或返回有效报告 |
  | `lang.golang.driver-failed` | Go 测试驱动在契约报告之外失败 |

## 9 计划

| 里程碑 | 内容 | 完成的标志 |
| --- | --- | --- |
| M0 设计 | 本文、[model.md](model.md)、[language.md](language.md) | 设计评审通过 |
| M1 设计文件 | `syntax`、`fmt`、`design`、`check --design-only`，包括流程、符号、限定、用例和录制文件的检查；用格式写出工具链自己的设计 | 工具链自己的 `design/` 通过 `fmt --check` 和 `check --design-only` |
| M2 Go 后端 | 抽取、绑定、建图、全部静态约束 | 工具链的代码带上绑定，`unicode check` 在工具链自身上通过（2026-11-26 完成：0 条诊断，611 个声明项全部一致） |
| M3 查询与视图 | `node`、`slice`、`impact`、`view`、`build` 和抽取缓存；查看器的雏形 | 此后给 agent 派任务都附上切片 |
| M4 Go 生成与回放 | Go 骨架、黑盒契约、调用序列、流程入口、接口及外部客户端替身、纯性检查；统一生成测试驱动；`unicode test` 快路径及 `--since` | 此后工具链的新组件和检查都由 `gen` 起头，工具链自己的性质由生成的驱动检查 |
| M5 Python | 抽取脚本、`uc` 包、Python 的生成和回放；把 [language.md](language.md) §7 的商店例子做成样例仓库 | 样例仓库通过 `unicode check` 和 `unicode test` |
| M6 探索与证据 | 慢路径：按覆盖挑选、缩小、缩减和录制；设计覆盖；运行时核对流程；结果作为证据并入快照 | 性质视图显示覆盖率和录制的用例 |
| M7 评审 | 评审记录与评审状态，按比较结果评审 | 评审只列出待审的项 |

从 M1 起，工具链用自己的格式描述自己；从 M2 起，用自己检查自己。M1 写出工具链的设计之后，本文、[model.md](model.md) 和 [language.md](language.md) 中能写成性质的条目，都移到设计文件里成为 `property`，文档只留下理由和设计文件表达不了的内容。

## 10 工具链对自己的验证

- **静态检查**：`gofmt`、`go vet`、`golangci-lint`，以及 `unicode fmt --check` 和 `unicode check`。`make rule-check`（`scripts/rulecodes.py`）检查每条规则在 [language.md](language.md) 或 [model.md](model.md) 的附录 B 中有一行，这两张表与 §8 的表合起来恰好列出代码中的全部诊断码。M2 之前，组件之间的导入边界由 golangci-lint 的 depguard 检查；M2 起改由 `unicode check` 按设计中的 `uses` 检查。
- **性质**（M1 时写成带符号的 `property`，M4 起由工具链自己生成驱动）：
  1. `fmt` 幂等。
  2. 规范格式打印之后再解析，得到同一棵语法树。
  3. 解析任何字节都不会 panic；出错时给出带位置的诊断。
  4. 相同的输入得到逐字节相同的快照，与文件的枚举顺序无关。
  5. 诊断的集合与文件的枚举顺序无关。
  6. `gen` 对骨架只新增：`gen` 之前已有的每个函数体，`gen` 之后逐字节不变；`gen` 两次与一次的结果相同；`gen` 之后不再有 `missing`，只剩 `stub`。
  7. 生成器产生的每个值，都通过与声明的用例相同的类型和限定检查。
  8. 回放录制文件，得到的覆盖与录制时相同；缩减之后的覆盖等于缩减之前的。
  9. `diff(a, a)` 为空；`diff(a, b)` 与 `diff(b, a)` 互为相反。
  10. 推导出的 `depends`、`traces`、`reaches` 与按 [model.md](model.md) §9 的定义直接计算的结果相同；输入是随机生成的小图。
- M4 之前，这些性质的驱动手写，用 `pgregory.net/rapid` 和 Go 原生的 fuzz；M4 起改为生成，手写的驱动随之删除。
- **两个阶段**：开发时只跑 `make check`，随机检查用缺省的次数，几分钟之内结束，通过就提交。深度验证 `make deep` 只在用户同意之后运行：一部分开发完成后，主会话向用户汇报并提议运行，用户同意了才由单独的 agent 在自己的工作区中跑，或者手动触发 CI；随机检查用大的次数，每个 fuzz 目标跑一段时间，再用 gremlins 做变异测试。它不阻塞开发；存活的变异和找到的失败，都成为之后的修复任务。gremlins 的结果要这样读：它只跑被变异的包自己的测试，用缺省的随机检查次数，所以存活的变异要用大的次数再确认一次；编译不过的变异被它算作杀死，要另外剔除；`switch` 的 `case` 表达式没有覆盖的块，其中的变异它不会运行。M6 起，`unicode test --explore` 接替其中的随机检查和 fuzz。

## 附录 A 设计理由

1. **一套结构，多种记法**：DDD、UML、时序图、事件风暴各自的元素，都能归为容器、类型、操作、参与者、流程和性质。把它们统一成一套结构，同一个对象在各种图中自然就是同一个节点；换一种记法只是换一种视图，不用改设计。
2. **两层用同一套种类**：代码层的包、类型和函数，与设计层的容器、类型和操作种类相同。`realizes` 因此是同种类节点之间的对应，比较和视图都可以在两层之间切换。
3. **消息分三种，外部系统只描述接口**：各种通行的记法都区分同步调用、异步消息和事件。UML 时序图用实心箭头和开放箭头区分前两种；BPMN 有消息流和抛出、捕获事件；事件风暴用"每当某事件发生就执行某命令"的策略把事件接到下一个动作；AsyncAPI 用通道上的发送和接收描述事件。外部系统在 C4 中是边界外的软件系统，在 BPMN 中是黑盒的池。我们取它们的共同部分：`call`、`send`、`emit` 加上 `subscribes`，外部系统只写它对我们提供的类型和操作。调用的子步骤对应 UML 的嵌套激活，让静态检查能从具体的那个方法出发去找调用。
4. **有状态的性质用 `call<…>`，不另设状态机语法**：一串调用只是一个列表类型的符号，生成、缩小、用例和录制都沿用同一套机制；`apply` 由生成得到，谓词仍然只是谓词。
5. **检查都是谓词，测试全部生成**：输入都由符号描述，回放、探索、fuzz 和录制才能统一处理；具体的例子写成用例，不另写固定例子的测试。
6. **按覆盖录制，分快慢两条路径**：快路径只回放，确定、迅速，每次修改都能跑；慢路径在预算内扩展输入空间。覆盖给出了"测试好不好"的可度量标准：没有带来新覆盖的输入不保留，反例永远保留。
7. **录制文件进 Git，但不在 `design/` 中**：它们是检查的输入，和种子一样随代码的版本走，任何克隆都能跑同样的快路径。它们由工具写出，放在 `corpus/` 中，不打扰人读设计。
8. **自定义格式，而不是 YAML、JSON、HCL 或 CUE**：签名、流程的步骤和限定各写成一行；只有一种规范写法，diff 干净；语法错误能精确到位置，并说明怎样修正；不接受别的写法，校验才有意义。HCL 和 CUE 都现成带有解析和格式化，CUE 还能校验值与类型和限定是否相符；但流程在 HCL 中要写成层层的块，在 CUE 中是嵌套的结构体列表，同一个意思又有多种写法。用 participle 之后，自己写的只剩语法定义、语法之外的规则和打印，代价小于流程变得难读。
9. **只有错误，没有警告**：警告会被忽略。只要有一处违反留下来，agent 就会把它当成惯例去复制。
10. **代码只细到函数**：字段和变量太多，人记不住，设计也不需要一一对应到它们。
11. **绑定只写在类型和自由函数上**：方法按固定的名称映射对应到操作，不必在每个方法上再写一遍标识；生成和检查都按同一套映射进行。
12. **静态抽取**：不运行代码就能建图，又快又确定，也不需要运行环境。运行时的证据由 `unicode test` 产生，作为证据并入。
13. **显著性由设计决定**：人写进设计的就是他要关注的。不另设一套标注，就不会出现标注与设计不一致。关注度只用来把整个支撑容器或工具容器折叠起来。
14. **取值域只写一处**：符号的类型和限定写在设计中，生成器、fuzz 的解码和用例的检查都由它推出。用例只给输入，结果由谓词判断，所以用例是性质的实例，而不是另一份测试。
15. **骨架与生成文件分开**：骨架生成之后归人所有，`gen` 不改写它，人写的函数体因此不会丢失；生成器和驱动完全由设计决定，归 `gen` 所有，每次重写，并由检查确认与设计一致。
16. **先做 Go 后端**：工具链用 Go 写，Go 后端做好后，工具链立刻能检查自己，每一步都有真实的反馈。
17. **快照不进 Git**：快照是生成物，随时可以从提交重建；Git 只保存设计文件、录制文件和代码。
18. **工具链不调度 agent**：调度由使用工具链的设计者负责。工具链只做设计、生成、检查、运行和查询，接口因此简单而确定。
19. **优先用维护中的外部库**：库的代码有人维护，我们只维护怎样用它，评审时也只需看用法；精力放在性质所描述的整体行为上，而不是细节的打磨。
20. **规则对照表的检查用 Python 写**：仓库中的每个 Go 文件都归属于设计中的某个组件，`scripts/rulecodes.py` 只读文档、设计文件和源码，不属于任何组件，所以不用 Go 写；它只用标准库。

## 附录 B 待定

1. Python 抽取脚本输出的格式与快照共用一份 schema。schema 用什么语言写：直接手写 JSON Schema，还是用 TypeSpec 生成。
2. M6 将 M4 的应答脚本按同一模型值格式写入录制文件；Go 的安装边界和应答形状见 §6.2。
3. Go 中怎样在运行时记录落实设计操作的函数被进入的次序（Python 由 `uc` 的装饰器在测试时包装方法）。
4. 错误的完整性：是否要求一次调用可能抛出的每个错误，要么有 `on` 分支处理，要么出现在发起方操作的 `raises` 中。
5. 复杂关系已由 `requires` 表达；满足这些前提的输入生成先使用类型生成、前置符号依赖和拒绝采样，生成预算与拒绝计数的调优留待实际使用。
6. 并发的性质：并行的调用、线性一致性。
7. 录制用例的数量上限（暂定每条 64 个）和探索预算在各性质之间的分配方式。
8. 评审记录存放在哪里（M7）。
9. `observe` 能否省略：不涉及设计之外的状态时，观察的通常就是 `changes` 路径的起点；可考虑只写 `changes` 时自动观察这些起点，只有读取外部状态时才写 `observe`，从而去掉 G18 的成对要求。
10. 通用的 Go 规则放进 engineering-conventions skill 的 `reference/go.md`；本仓库的 AGENTS.md 只留本仓库特有的规则。
11. 设计中的泛型类型（类型参数）：M1 不支持。
12. 记法的词汇是否允许项目扩展。目前内置、固定，以保证校验。

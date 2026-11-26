# 决定记录

每条只记决定和理由，并指向它所改动的章节。当前的设计以各文档为准，不必回放本记录。

## 2026-11-26

1. **做成命令行工具链加静态查看器，工具链不调度 agent。** 使用者是设计者兼评审者，由他派 agent 去实现，再用工具链评审结果。见 [design.md](design.md) §2。
2. **图采用一套统一的结构：元素、关系、流程、性质。** 设计层和代码层用同一套种类；DDD、UML 类图、时序图、活动图和 C4 都是记法，即这张图的视图。见 [model.md](model.md) §1、§5。
3. **代码的结构由语言后端静态抽取，只细到函数。** 抽取不运行代码，又快又确定；函数以下的符号太多，人记不住。见 [model.md](model.md) §2、§3。
4. **设计文件用自定义格式，只有一种规范写法，其他写法一律不收；代码与设计的对应以绑定写在代码中：Go 用注释指令，Python 用装饰器。** 写法统一，校验才能强制。见 [format.md](format.md)。
5. **性质必须能执行。性质可以声明符号和限定，用例用这些符号写成；生成器和驱动由 `gen` 从取值域推出。** 取值域只写在一处，用例只给输入，结果由谓词判断。见 [model.md](model.md) §3、§10，[design.md](design.md) §6。
6. **显著性由设计决定，关注度只用来折叠容器。** 人写进设计的就是他要关注的；工具函数和基础服务默认收起，点开才展开。见 [model.md](model.md) §9，[design.md](design.md) §7.2。
7. **设计先行：`gen` 对骨架只新增，生成文件归 `gen` 所有。** 人写的函数体不会被覆盖；生成文件由检查确认与设计一致。见 [design.md](design.md) §6。
8. **工具链用 Go 写，先做 Go 后端，再做 Python 后端。** 工具链能立刻检查自己。见 [design.md](design.md) §3、§9。
9. **项目名和命令为 `unicode`，设计文件扩展名为 `.uc`，绑定的前缀为 `uc`。** 扩展名和前缀取缩写，写起来短。见 [format.md](format.md) §1、§4。
10. **工具链用自己的格式描述自己，并检查自己：M1 起有自己的设计文件，M2 起自检。** 自己先用起来，每一步都有真实的反馈。见 [design.md](design.md) §9。
11. **`relation` 用符号加谓词表达；有状态的性质用 `call<…>` 写成一串调用。** 不另设状态机语法，生成、缩小、用例和录制沿用同一套机制。见 [model.md](model.md) §3.3，[design.md](design.md) 附录 A-4。
12. **流程支持异步发送、事件的发布与订阅、外部系统，调用可以带子步骤。** 取 UML、BPMN、事件风暴、AsyncAPI 和 C4 的共同部分。见 [model.md](model.md) §3.2、§10，[format.md](format.md) §3，[design.md](design.md) 附录 A-3。
13. **强制：每条性质和每个流程恰有一个谓词，代码中不能有手写的测试，驱动全部生成。** 输入都由符号描述，回放、探索、fuzz 和录制才能统一。见 [model.md](model.md) §10 第 20、21 条。
14. **按覆盖录制用例：快路径只回放，慢路径按时间预算探索；增加覆盖的输入和反例写进 `corpus/` 并进 Git。** 覆盖给出测试好坏的标准，回放给出快速而确定的回归。见 [design.md](design.md) §6.3。
15. **确认扩展名 `.uc` 和绑定前缀 `uc`。** 见第 9 条。
16. **写工具链自己的设计时发现的四处缺口，补进模型和格式。**
    - 有的取值域写不成类型和限定，例如一段合法的设计文件文本：这样的符号标为 `generated`，生成器由代码中的第三种绑定 `generates` 给出。见 [model.md](model.md) §3.3、§10 第 24 条，[format.md](format.md) §4。
    - 用例要写多行文本：字符串增加 `\n` 和 `\t` 转义。见 [format.md](format.md) §2。
    - 操作只有一个返回值，几个结果写成记录；Go 的多返回值只用于 `raises` 的 `error`。见 [design.md](design.md) §4。
    - 规范格式的空白与空行此前没有写明，打印器无从实现。见 [format.md](format.md) §3。
17. **原则写进 `CLAUDE.md`，`AGENTS.md` 是指向它的链接。** 两种 agent 读同一份规则。
18. **优先用维护中的外部库，只有没有合适的库时才自己写：解析用 participle，命令行用 cobra，路径模式用 doublestar。** 维护和评审的成本都更低，精力放在整体行为上。见 [design.md](design.md) §3、附录 A-19，`CLAUDE.md` 原则 8。
19. **第一轮实现提出的 40 处理解写进规格；其中改变行为的几处这样定：**
    - 数有唯一的规范写法：`fmt` 去掉多余的前导零和小数末尾的零，`-0` 写作 `0`。
    - `true`、`false`、`none` 是保留名，否则枚举值与字面量会撞名。
    - 花括号的位置错了是错误，`fmt` 不修；取值域说明和片段的标签遵守摘要的规则。
    - 值的 100 列按显示宽度计，用 `go-runewidth`。
    - 代码层类型可以有设计之外的字段和方法；Go 的抽取读 `_test.go`。

    见 [format.md](format.md) §2、§3、§3.1，[design.md](design.md) §4、§8，[model.md](model.md) §10 第 18 条。
20. **保留自定义格式加 participle，不换成 CUE 或 HCL。** 换了能省下打印，CUE 还能省下用例与类型、限定的校验；但流程会变得难读，而流程和签名一行写一个是设计文件好读、diff 好审的关键。见 [design.md](design.md) 附录 A-8。
21. **第一次评审后补上一条性质：违反规则的文本会被拒绝。** 原有的性质只验证合法的文本和诊断的格式，把解析后的检查整个关掉也照样通过。见 `design/syntax.uc` 的 `rejects-broken-text`。
22. **命令行的两处理解：JSON 模式下诊断是数据，写到标准输出；路径参数必须在 `design/` 或 `corpus/` 之内。改写文件用 renameio。** 见 [design.md](design.md) §3、§8。
23. **工具链的界面文字统一用英文，文档仍用中文。** 诊断、帮助和命令行消息与 participle、cobra、pflag 和操作系统的英文提示一致，不必翻译；读库的报错时取它的结构化字段，不拆它的文字。见 `CLAUDE.md` 原则 7。
24. **流程 `format-files` 的输入带上文件的权限位（`cli.RepoFile`）。** 谓词只能从符号取输入，权限要改写后保持不变，就必须是输入的一部分，而不是从路径的哈希推出来。
25. **第二次评审后补强两条性质的输入。** `rejects-broken-text` 的输入带上改动所在的行，诊断必须落在这一行；`format-files` 增加路径参数 `targets`，验证路径参数的规则。原来的输入让多数改错的实现仍能通过。见 `design/syntax.uc`、`design/cli.uc`。
26. **`design/` 和 `corpus/` 中的符号链接给出诊断 `cli.symlink`，不跟随，也不改写。** 见 [design.md](design.md) §8。
27. **大批量的验证是合并之后的异步阶段，不阻塞开发。** 开发分支只跑一次 `make check` 就提交；变异测试、长时间的 fuzz 和大量的随机检查归 `make deep`，由单独的 agent 或定时的 CI 跑，结果成为修复任务。见 [design.md](design.md) §10，`CLAUDE.md` 原则 9。
28. **M1 的第二部分：`graph` 与 `design` 的设计。** 节点的属性都是字符串的列表，类型、签名和值按规范格式写出，引用写成从根起的标识；M2 比较代码与设计时，把代码的签名归一化成同样的写法，直接比较字符串。节点的摘要按它自身内容的规范编码计算。只需设计就能判断的约束列在 [design.md](design.md) §5.1。
29. **M2 的设计：`lang` 的接口、Go 后端、`graph` 的合并与推导、`check` 的代码约束、`check-code` 流程。** 方法到操作的落实由后端按反向的名称映射给出候选边，推导时丢掉终点不存在的，`graph` 因此不依赖 `lang`。代码的签名归一化成设计的写法，与设计层直接比较字符串。`check` 的性质在随机的图上验证，Go 后端的性质在随机生成的 Go 模块上验证，二者合起来由 `check-code` 流程和工具链自身的 `unicode check` 验证。Go 中对象类型按指针传递，`T` 和 `optional<T>` 都写作 `*T`。见 [design.md](design.md) §4、§5.3。
30. **M3 的设计：`query`（节点、引用、搜索、切片、影响）和 `view`（选取与折叠）。** 二者只读图，性质在随机的图上验证，可以与 M2 并行开发；命令行的接线（`build`、`node`、`slice`、`impact`、`view`、`check`）放在最后一条开发线中统一完成。见 `design/query.uc`、`design/view.uc`。
31. **名称映射之后不能撞名（[model.md](model.md) §10 第 30 条）。** M3 的实现中发现类型 `Slice` 与操作 `slice` 在 Go 中都映射成 `Slice`；查询的两个记录改名为 `Context` 和 `Affected`，同类的撞名今后在载入设计时就报出。
32. **代码层契约的细节（Go 后端与 `check` 之间）：** 指针写作 `optional`，由 `check` 按设计中的形式放宽；末尾的 `error` 写作 `raises error`；接口中声明的方法也是 `op` 节点（形式 `abstract`），经由接口的调用因此能被追溯；`AddFragment` 不因绑定的目标不存在而失败，这类边留给 `check` 报告。见 [design.md](design.md) §5.2、[model.md](model.md) §3、§9。
33. **第一次深度验证之后：** `make deep` 的 gremlins 改为在根目录下不带路径、不用缓存、放宽超时运行；`kindOf` 增加性质 `kind-of-follows-path`；语法诊断按位置排序，并且落在改动的列上；`format-files` 的输入增加工作目录、是否给出 `--root`，以及读不出的文件。27 个改变行为的变异原先没有被任何性质抓到。
34. **昂贵的验证只在用户同意之后运行。** `make deep`（变异测试、长时间的 fuzz、大量的随机检查）不由任何开发线、评审、主会话或定时任务自行启动；一部分开发完成后，主会话汇报并提议，用户同意了再跑。CI 中的 `deep` 只保留手动触发。见 `CLAUDE.md` 原则 9，[design.md](design.md) §10。
35. **合并 m1-design 时定下的理解：** 系统节点的标识是系统的名称；顶层组件不能叫 `system`；`system` 节点带上 `notations` 和 `roots`；`alt` 的分支写在 `contains` 边的 `branch` 上；`uses` 不能指向人；区间的下界不能大于上界；一条约束可以有几个诊断码。`make check` 从此包含工具链对自身设计的 `check --design-only`。见 [model.md](model.md) §7、[format.md](format.md) §1、§3，[design.md](design.md) §5.2、§8。

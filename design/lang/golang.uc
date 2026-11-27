component lang.golang "Go 后端：抽取、名称映射、生成与运行" {
  paths "internal/lang/golang/**"
  uses lang, syntax
}

type Backend object "Go 的后端：用 go/packages 和 go/types 静态抽取" {
  implements lang.Backend
  implements lang.Generator
  implements lang.GenerationExtractor
  implements lang.Runner
}

type ContractSample record "黑盒运行时的转账样本，含正常、错误、拒绝和越界变化" {
  field balances: map<string, int>
  field fromId: string
  field toId: string
  field amount: int
  field foreignDelta: int
  field raiseError: bool
}

type DriverSample record "驱动编译的样本：稳定的完整性质标识、输入个数及谓词种类" {
  field name: string
  field symbols: int
  field manual: bool
}

type Module record "一个随机生成的 Go 模块：文件，以及抽取应当得到的节点、边和诊断，供性质使用" {
  field files: map<string, string>
  field nodes: list<string>
  field edges: list<string>
  field diagnostics: list<string>
}

type Lines record "文件中相连的几行：首行和末行，都从 1 起，含末行" {
  field from: int
  field to: int
}

type SkeletonSample record "只新增骨架的样本：原有类型、方法及外部客户端" {
  field name: string
  field existingType: bool
  field existingMethod: bool
  field external: bool
}

type ExecutionSample record "调用序列、流程入口及纯性核对的有状态样本" {
  field initial: int
  field deltas: list<int>
  field failAt: int
  field impure: bool
}

type DoubleSample record "确定性替身的输入：应答数量及声明错误" {
  field count: int
  field fail: bool
}

type BodySample record "操作内容哈希的样本：原有常数、变化与排版差异" {
  field value: int
  field changed: bool
  field spaced: bool
}

op newBackend() -> Backend "一个 Go 后端"

op declaration(src: bytes, line: int, column: int) -> optional<Lines> "Go 源码中包含这一位置的顶层声明所占的行，从它的文档注释起；分组的类型声明中，取包含这一位置的那个类型；源码解析不了，或者这一位置不在任何声明之中时没有"

property extracts-generated-module invariant "随机生成的 Go 模块，抽取得到的节点、边和诊断恰是生成时记下的那些，签名按 design.md §4 归一化；诊断写作 诊断码 路径:行" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：几个包和文件，类型、函数、方法、调用、导入、嵌入、接口的实现、测试文件，以及 //uc: 绑定，其中有的写法不合格式"
  manual "需要写入并抽取临时 Go 模块，比较源码位置、归一化签名和诊断，表达式不执行文件系统操作"
}

property executes-contracts invariant "前提不成立时不调用被测函数；成立后按调用前键值检查余额与范围，正常和错误返回都保护范围外的值；观察失败和零有效执行单独报告；共享对象不改变调用前副本" {
  about Backend
  forall sample: ContractSample generated "随机余额、账户标识、金额、外部账户改动和错误结果，含缺失键、重复标识及共享对象"
  manual "需要构造实际函数与观察适配器并记录是否调用，独立谓词逐项核对运行时与生成文件"
}

property driver-modes-preserve-claims invariant "普通测试回放后完整执行 rapid 次数，只有显式探索截断时间；稳定命名的原生 fuzz 与测试共用有序输入和谓词，保留独立输入及上下文，拒绝不成为伪反例" {
  about Backend
  forall sample: DriverSample generated "合法的完整性质标识，零到三个整数符号，以及自动或人工谓词；覆盖名称和符号次序的改变"
  manual "需要编译驱动并检查测试入口、原生 fuzz、输入调度和运行上下文，模型值的关系不能直接表达 Go 代码生成"
}

property generation-extraction-is-read-only invariant "普通抽取严格检查生成测试；生成模式仅排除本工具拥有的常规生成测试，手写错误和符号链接不作为可修复输出；两种抽取均不改变磁盘文件" {
  about Backend
  forall repairable: bool
  forall linked: bool
  forall sourceName: string generated "合法且非关键字的 Go 标识符，用于构造已改名的函数及旧引用"
  manual "需要建立实际 Go 模块、旧生成文件和符号链接，调用两种抽取并逐字节比较磁盘内容"
}

property skeletons-preserve-source invariant "一次规划缺少的类型、操作和人工谓词及其驱动，骨架只新增且有绑定；重建图后再次生成不再产生骨架，已有函数体保持逐字节相同；旧只新增计划拒绝覆盖" {
  about Backend
  forall sample: SkeletonSample generated "合法名称及已有类型、方法、外部操作的组合，含空包和不存在的源码目录"
  manual "需要规划和载入实际 Go 模块，核对文件树、绑定、编译结果与函数体字节"
}

property executes-sequences-and-flows invariant "序列检查初态及每个声明调用后的关系，逐步刷新 old 并在声明错误后继续；自动流程只调用入口；纯操作重复核对冻结的结果、错误及接收者与实参" {
  about Backend
  forall sample: ExecutionSample generated "初始计数、零到八次增减、声明失败位置与纯性破坏方式"
  manual "需要执行实际 Go 方法并记录调用次序，与独立状态演算、返回和错误快照比较"
}

property scripted-doubles-follow-inputs invariant "接口及外部客户端的每次应答来自输入脚本，末项重复，声明错误保留字段；安装及清理覆盖拒绝、失败和 panic，不调用真实外部服务" {
  about Backend
  forall sample: DoubleSample generated "一到五个成功或声明错误的应答，包含多个接口方法及客户端安装边界"
  manual "需要构造实际原生接口和客户端安装器，记录调用及清理并比较脚本的独立预期值"
}

property implementation-content-is-normalized invariant "函数体与类型声明的规范内容忽略排版、注释及源码位置；只改实际函数体包括闭包时，操作的内容哈希改变，依赖它的检查可以被增量选择" {
  about Backend
  forall sample: BodySample generated "有符号整数与是否改变函数返回表达式、注释及空行的组合"
  manual "需要解析和抽取实际 Go AST，比较规范内容与图的哈希，而非手写预期格式"
}

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

op newBackend() -> Backend "一个 Go 后端"

op declaration(src: bytes, line: int, column: int) -> optional<Lines> "Go 源码中包含这一位置的顶层声明所占的行，从它的文档注释起；分组的类型声明中，取包含这一位置的那个类型；源码解析不了，或者这一位置不在任何声明之中时没有"

property extracts-generated-module invariant "随机生成的 Go 模块，抽取得到的节点、边和诊断恰是生成时记下的那些，签名按 design.md §4 归一化，两个函数体、两个类型声明或两个文件中不是节点的顶层声明的哈希相同当且仅当它们的归一化形式相同，包级变量的初始值中闭包里的调用和存放的函数引用记在引用该变量的函数上；诊断写作 诊断码 路径:行" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：几个包和文件，类型、函数、方法、调用、导入、嵌入、接口的实现、测试文件，以及 //uc: 绑定，其中有的写法不合格式；有的函数体、结构体和文件中的包级声明只在格式和注释上与另一个不同，包级声明包括变量、常量、类型别名、init 和名为 _ 的函数；包级变量的初始值中有闭包、函数和方法的引用或初始化时的调用，函数引用这些变量"
  manual "需要写入并抽取临时 Go 模块，比较源码位置、归一化签名、诊断，以及函数体 body、类型声明 declaration 和文件中包级声明 globals 的哈希按归一化形式划分的等价类；表达式不执行文件系统操作"
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

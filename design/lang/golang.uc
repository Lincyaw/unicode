component lang.golang "Go 后端：抽取、名称映射、生成与运行" {
  paths "internal/lang/golang/**"
  uses lang, syntax, evidence
}

type Backend object "Go 的后端：用 go/packages 和 go/types 静态抽取，并为 Go 生成和运行骨架、谓词和驱动" {
  implements lang.Backend
  implements lang.Generator
  implements lang.GenerationExtractor
  implements lang.Runner
}

type ContractSample record "executes-contracts 的输入：一次转账的余额表、转出和转入的账户、金额，对其他值的改动量 foreignDelta，以及被测函数是否返回错误" {
  field balances: map<string, int>
  field fromId: string
  field toId: string
  field amount: int
  field foreignDelta: int
  field raiseError: bool
}

type DriverSample record "driver-modes-preserve-claims 的输入：一个性质标识、符号的个数，以及谓词是否手写" {
  field name: string
  field symbols: int
  field manual: bool
}

type Module record "extracts-generated-module 的输入：一个随机 Go 模块的文件，以及抽取应当得到的节点、边和诊断" {
  field files: map<string, string>
  field nodes: list<string>
  field edges: list<string>
  field diagnostics: list<string>
}

type Lines record "文件中相连的几行：首行和末行，都从 1 起，含末行" {
  field from: int
  field to: int
}

type SkeletonSample record "skeletons-preserve-source 的输入：一个名称，仓库中是否已有对应的类型和方法，以及是否有外部系统的操作" {
  field name: string
  field existingType: bool
  field existingMethod: bool
  field external: bool
}

type ExecutionSample record "executes-sequences-and-flows 的输入：计数器的初值、一串增减量、在第几步抛出声明的错误，以及纯操作是否被改成会改动实参" {
  field initial: int
  field deltas: list<int>
  field failAt: int
  field impure: bool
}

type DoubleSample record "scripted-doubles-follow-inputs 的输入：应答的项数，以及是否有一半应答是声明的错误" {
  field count: int
  field fail: bool
}

type BodySample record "implementation-content-is-normalized 的输入：函数返回的常数、是否改动函数体，以及是否只改排版和注释" {
  field value: int
  field changed: bool
  field spaced: bool
}

type MeasurementSample record "可重建临时仓库的模型数据：数值、输入域、codec 状态及同步异步轨迹变体，不保存临时绝对路径" {
  field value: int
  field variant: int
}

op newBackend() -> Backend "创建 Go 后端"

op declaration(src: bytes, line: int, column: int) -> optional<Lines> "src 中包含第 line 行第 column 列的顶层声明所占的行，从文档注释起，分组的类型声明中只取包含该位置的类型；源码解析不了或该位置不在声明中时为空"

property extracts-generated-module invariant "抽取随机 Go 模块得到的节点、边和诊断，与按 design.md §4、§5.2 的规则从生成模块时的模型算出的相同" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：一至三个包，含测试文件、类型、函数、方法、调用、导入、嵌入、接口的实现、包级变量和 //uc: 绑定，部分绑定和生成标记故意写错，部分声明只在排版和注释上与另一个不同"
  manual "需要写出临时 Go 模块并用 go/packages 抽取，表达式不能读写文件"
}

property executes-contracts invariant "Go 驱动按 language.md E4、E12、E17 至 E22 执行黑盒契约：前提不成立时不调用被测函数，调用后检查 ensure 和范围外的差异，观察失败报为检查错误，调用前观察值不随共享对象改变，整数既不溢出也不截断" {
  about Backend
  forall sample: ContractSample generated "a、b、c 三个账户各有一半机会存在、余额 0 至 200 的余额表，从 a、b 和不存在的账户中取的转出和转入账户，金额 1 至 100，对其他值 -2 至 2 的改动量，以及是否返回错误；约一半保证前提成立"
  manual "需要经反射调用实际的 Go 函数并记录调用次数，表达式不能调用代码"
}

property driver-modes-preserve-claims invariant "生成的驱动先回放用例再执行 rapid 配置的次数，只在 unicode test --explore 时按时长停止；原生 fuzz 入口以性质标识的 SHA-256 命名，与普通测试共用输入构造和谓词，被前提拒绝的输入不报告为反例" {
  about Backend
  forall sample: DriverSample generated "由 a、bank、same-name、longer、x9 组成的二至五段性质标识，零至三个整数符号，以及谓词是否手写；符号、生成器和谓词的名称按标识的长度与驱动的局部名称和另一个包的名称相撞，回放用例一至三个"
  manual "需要生成驱动的 Go 源码，再用 go/types 检查并读取它的语法树，表达式不能处理 Go 源码"
}

property measured-execution-keeps-real-evidence invariant "通过真实生成和隔离 worker 运行模型输入；探索拒绝继续计数，完整保留有效输入，基本块映射到原始源码；codec 保持表示，别名错误不静默复制；同步与异步轨迹须恰符合流程，额外调用不被忽略" {
  about Backend
  forall sample: MeasurementSample generated "可重建的合法回放、输入域拒绝、codec 私有状态、共享别名，以及同步、发布和额外异步调用的原始变体"
  manual "从模型数据重建临时 Go 仓库并运行真实编译器和隔离进程，读取原始 profile、轨迹及报告逐项比较；这些环境生命周期尚不在表达式中，不提交手写 Test/Fuzz 或固定例子"
}

property generation-extraction-is-read-only invariant "普通抽取遇到旧生成测试中的类型错误照常失败，供生成用的抽取只把源码根目录中带生成标记、不是符号链接的测试文件读成只有包声明的文件，两种抽取都不改动磁盘" {
  about Backend
  forall repairable: bool
  forall linked: bool
  forall sourceName: string generated "1 至 13 个字符、不是 Go 关键字的标识符，作为函数改名前后的名称的前缀"
  manual "需要写出带旧生成测试和符号链接的临时 Go 模块，调用两种抽取后逐字节比较磁盘，表达式不能读写文件"
}

property skeletons-preserve-source invariant "一份生成计划同时含缺少的类型、操作、手写谓词和生成器的骨架（createOnly、带绑定）和引用它们的驱动，写入后已有文件逐字节不变，重新建图再生成不再产生骨架" {
  about Backend
  forall sample: SkeletonSample generated "一至六个字符的名称，以及是否已有对应的类型和方法、是否有外部系统的操作的组合；没有已有类型时源码目录不存在，仓库中还有指向仓库之外的符号链接"
  manual "需要在临时 Go 模块中生成、写入、抽取并运行代码，表达式不能读写文件"
}

property executes-sequences-and-flows invariant "调用序列先检查初态，每次调用前重新求 old、调用后检查 ensure，抛出声明的错误时继续下一个调用；纯操作被调用两次并比较结果，自动流程的谓词调用流程的第一个步骤" {
  about Backend
  forall sample: ExecutionSample generated "计数器的初值 -20 至 20，零至八个 -5 至 5 的增减量，在第几步抛出声明的错误（-1 至 8，超出范围时不抛出），以及纯操作是否被改成会改动实参"
  manual "需要经反射调用实际的 Go 方法并记录调用次数，表达式不能调用代码"
}

property scripted-doubles-follow-inputs invariant "接口和外部客户端的替身按输入中的应答脚本依次应答，末项重复，错误应答带上脚本中的字段，被拒绝、失败或 panic 时都执行清理，不调用真实的外部客户端" {
  about Backend
  forall sample: DoubleSample generated "一至五项应答，fail 为真时偶数项是带字段的声明的错误，其余是返回值"
  manual "需要经真实的安装器替换客户端，并记录安装、清理和真实客户端的调用，表达式不能调用代码"
}

property implementation-content-is-normalized invariant "函数体、类型声明和原生声明头的内容哈希不随排版、注释和源码位置改变，只改函数体（含闭包）、接收者或原生声明头时，操作的内容哈希改变" {
  about Backend
  forall sample: BodySample generated "函数返回的常数 -1000 至 1000，以及是否改动函数体、是否只改排版和注释"
  manual "需要写出临时 Go 模块并抽取，再与 go/format 给出的规范文本比较，表达式不能读写文件"
}

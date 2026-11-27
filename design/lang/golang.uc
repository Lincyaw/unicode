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

type DoubleSample record "scripted-doubles-follow-inputs 的输入：应答的项数 count，以及第一、三、五项应答是否抛出声明的错误 fail" {
  field count: int
  field fail: bool
}

type BodySample record "implementation-content-is-normalized 的输入：函数返回的常数、是否改动函数体，以及是否只改排版和注释" {
  field value: int
  field changed: bool
  field spaced: bool
}

type MeasurementSample record "measured-execution-keeps-real-evidence 的输入：一个数值 value 和一种情形 variant，谓词据此重建临时的 Go 仓库" {
  field value: int
  field variant: int
}

op newBackend() -> Backend "创建 Go 后端"

op declaration(src: bytes, line: int, column: int) -> optional<Lines> "src 中包含第 line 行第 column 列的顶层声明所占的行，从文档注释起，分组的类型声明中只取包含该位置的类型；源码解析不了或该位置不在声明中时为空"

property extracts-generated-module invariant "抽取随机 Go 模块不出错，得到的节点和边与按 design.md §4、§5.2 的规则从生成模块时的模型算出的相同，其中内容哈希是 SHA-256 并把节点分成同样的类，诊断的诊断码、文件和行也相同" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：一至三个包，各有一至两个源码文件，可有内部测试文件和外部测试包，含零至六个类型及其方法、零至五个函数、按测试命名的函数、调用、作为值引用的函数和方法（函数名、方法值和方法表达式，包括接口的方法）、导入、嵌入、实现接口的类型、零至三个包级变量和 //uc: 绑定；部分绑定和生成标记故意写错，部分方法的签名与接口只差一个类型，部分声明只在排版和注释上与另一个不同，另有不成节点的声明"
  manual "需要写出临时 Go 模块并用 go/packages 抽取，表达式不能读写文件"
}

property executes-contracts invariant "Go 驱动按 language.md E4、E12、E17 至 E22 执行转账的黑盒契约：前提不成立时不调用被测函数（回放时报域外输入），成立时恰调用一次，ensure 都成立且只改 changes 中的余额时没有差异，改动其他值或经另一条观察的别名写入时有范围外的差异，观察失败报为检查错误，old 和调用前观察值不随之后的写入改变，整数既不溢出也不截断" {
  about Backend
  forall sample: ContractSample generated "a、b、c 三个账户各有一半机会存在、余额 0 至 200 的余额表，从 a、b 和不存在的账户中取的转出和转入账户，金额 1 至 100，对其他值 -2 至 2 的改动量，以及是否返回错误；约一半保证前提成立"
  manual "需要经反射调用实际的 Go 函数并记录调用次数，表达式不能调用代码"
}

property driver-modes-preserve-claims invariant "生成的驱动和手写谓词一起通过 go/types 检查，先逐个回放用例，回放模式之外再执行 rapid 配置的次数，只在 unicode test --explore 时按时长停止；原生 fuzz 入口以性质标识的 SHA-256 命名，与普通测试共用输入构造和谓词，名称相撞时不生成而给出诊断，被前提拒绝或中止的调用报告没有有效输入而不是反例" {
  about Backend
  forall sample: DriverSample generated "以 driver 为首段、其后一至四段各取 a、bank、same-name、longer 或 x9 的性质标识，零至三个整数符号，以及谓词是否手写；符号、生成器和谓词的名称按标识的长度与驱动的局部名称和另一个包的名称相撞，回放用例一至三个"
  manual "需要生成驱动的 Go 源码，再用 go/types 检查并读取它的语法树，表达式不能处理 Go 源码"
}

property measured-execution-keeps-real-evidence invariant "Go 后端在隔离的进程中执行生成的驱动且不改动文件：拒绝的候选输入只计数，有效执行全部保留且输入哈希与规范值相符，基本块覆盖只映射到原始源码，codec 保持私有状态，共享别名的输入报错，轨迹只含被测调用及其后代并恰合同步和异步流程的步骤和分支，多出的异步调用报为流程不合" {
  about Backend
  forall sample: MeasurementSample generated "value 取 -100 至 100，variant 取 0 至 7，选出八种情形之一：两个声明用例的回放、被前提拒绝的探索输入、codec 编码的私有状态、共享的别名、含同步调用的流程、含发布的流程、多出异步调用的流程，以及反复得到同一有效输入的探索"
  manual "需要按 sample 重建临时的 Go 仓库，编译并在隔离的进程中运行，表达式不能读写文件或运行程序"
}

property generation-extraction-is-read-only invariant "普通抽取遇到旧生成测试中的类型错误照常失败，供生成用的抽取恰把源码根目录中带生成标记、不是符号链接的测试文件读成只有包声明的文件并保留源码中的绑定，两种抽取都不改动磁盘" {
  about Backend
  forall repairable: bool
  forall linked: bool
  forall sourceName: string generated "1 至 13 个字符、不是 Go 关键字的标识符，作为函数改名前后的名称的前缀"
  manual "需要写出带旧生成测试和符号链接的临时 Go 模块，调用两种抽取后逐字节比较磁盘，表达式不能读写文件"
}

property skeletons-preserve-source invariant "一份生成计划同时含缺少的类型、操作、手写谓词和生成器的骨架（createOnly、带绑定、不带生成标记）和引用它们的驱动，规划和写入都不改动已有文件和仓库之外的文件，写入后抽取没有诊断，重新建图再生成不再产生骨架且生成文件逐字节不变" {
  about Backend
  forall sample: SkeletonSample generated "一至六个字符的名称，以及是否已有对应的类型和方法、是否有外部系统的操作的组合；没有已有类型时源码目录不存在，仓库中还有指向仓库之外的符号链接"
  manual "需要在临时 Go 模块中生成、写入、抽取并运行代码，表达式不能读写文件"
}

property executes-sequences-and-flows invariant "调用序列先检查初态，不成立时不调用，每次调用前重新求 old、调用后检查 ensure，抛出声明的错误时继续下一个调用；纯操作被调用两次，结果、声明的错误或实参不同时报纯性违规，自动流程生成按名称映射命名的谓词" {
  about Backend
  forall sample: ExecutionSample generated "计数器的初值 -20 至 20，零至八个 -5 至 5 的增减量，在第几步抛出声明的错误（-1 至 8，超出范围时不抛出），以及纯操作是否被改成会改动实参"
  manual "需要经反射调用实际的 Go 方法并记录调用次数，表达式不能调用代码"
}

property scripted-doubles-follow-inputs invariant "接口和外部客户端的替身在执行前安装一次，按输入中的应答脚本依次应答，接口的各方法各自计位，末项重复，错误应答带上脚本中的字段且调用方改动不影响脚本，被拒绝、失败或 panic 时都清理并恢复真实的客户端，替身在位时不调用真实的外部客户端" {
  about Backend
  forall sample: DoubleSample generated "一至五项应答，fail 为真时第一、三、五项是带字段的声明的错误，其余是返回值"
  manual "需要经真实的安装器替换客户端，并记录安装、清理和真实客户端的调用，表达式不能调用代码"
}

property implementation-content-is-normalized invariant "函数体、类型声明和原生声明头的内容哈希恰在去掉注释后经 go/format 排版的文本不同时不同，不随排版、注释、源码位置和类型自身的名称改变，改函数体（含闭包）、接收者或泛型约束时操作的内容哈希改变，改方法体不改变所属类型的内容" {
  about Backend
  forall sample: BodySample generated "函数返回的常数 -1000 至 1000，以及是否改动函数体、是否只改排版和注释"
  manual "需要写出临时 Go 模块并抽取，再与 go/format 给出的规范文本比较，表达式不能读写文件"
}

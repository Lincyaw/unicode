component lang.golang "Go 后端：抽取、名称映射、生成与运行" {
  paths "internal/lang/golang/**"
  uses lang, syntax, evidence, monitor
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

type CacheChange record "extraction-cache-follows-changes 和 planned-check-cache-follows-changes 的输入：改动的种类 kind、选出改动所在的包的序号 pick，以及写进改动的常数 value" {
  field kind: int
  field pick: int
  field value: int
}

type DerivedOutcome enum "derived-state-is-removed 的输入：一个用例的结局" {
  value passes
  value fails
  value rejected
  value panics
  value underivable
}

type BuiltState record "derived-state-is-removed 和 derived-inputs-are-recomputed 的观察：派生函数记下的目录数、其中仍然存在的目录数，以及它每次被调用时的 mode 的十进制文本" {
  field recorded: int
  field remaining: int
  field modes: list<string>
}

type EffectModule record "extracts-direct-effects 的输入：一个 Go 模块和它依赖的一个模块的文件、源码根目录，以及模块中每个函数和方法应有的直接效果" {
  field files: map<string, string>
  field roots: list<string>
  field expected: map<string, list<string>>
}

type MonitoredRun record "受监控的模块的输入：模块的一个副本的目录，以 unicode_monitor 和竞态检测构建的程序以抽样比例 1、打开采集运行之后写出的运行时的运行记录（没有判定时为空的报告），对 levels 中每个值经入口包装和直接调用 demo.score 各自得到的结果，程序写出的运行记录、采集的调用和导出的 span 中的全部单词，monitor.Wrap 是否给 demo.Counter 包装而对没有登记的接口和函数原样返回，包装传给实现的 context 是否带着这次调用的 span 且以调用方的 span 为父，以及竞态检测是否报出竞态" {
  field root: string
  field record: lang.RunReport
  field wrapped: list<string>
  field direct: list<string>
  field words: list<string>
  field wraps: bool
  field keepsUnregistered: bool
  field contextLinked: bool
  field raced: bool
}

op newBackend() -> Backend "创建 Go 后端"

op testFile(path: string) -> bool pure "path 是不是 Go 的测试文件，即文件名以 _test.go 结尾、只有 go test 才编译的文件"

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

property extraction-cache-hits-moved-module invariant "随机 Go 模块经内容缓存抽取之后，写到另一个目录的同一模块按同一种抽取（普通的或供生成用的）命中缓存，得到的节点、边和诊断与在那个目录不经缓存抽取的完全相同" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：一至三个包，各有一至两个源码文件，可有内部测试文件和外部测试包，含零至六个类型及其方法、零至五个函数、按测试命名的函数、调用、导入、嵌入、实现接口的类型、零至三个包级变量和 //uc: 绑定；部分绑定和生成标记故意写错，部分方法的签名与接口只差一个类型，部分声明只在排版和注释上与另一个不同，另有不成节点的声明"
  forall forGeneration: bool
  manual "需要在两个临时目录写出同一个 Go 模块，经缓存和不经缓存各用 go/packages 抽取，表达式不能读写文件"
}

property extraction-cache-follows-changes invariant "随机 Go 模块经内容缓存做一次普通的或供生成用的抽取，做 change 所说的改动之后再经缓存做同一种抽取，它失败当且仅当不经缓存的同一种抽取失败，成功时节点、边和诊断与之完全相同" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：一至三个包，各有一至两个源码文件，可有内部测试文件和外部测试包，含零至六个类型及其方法、零至五个函数、按测试命名的函数、调用、作为值引用的函数和方法（函数名、方法值和方法表达式，包括接口的方法）、导入、嵌入、实现接口的类型、零至三个包级变量和 //uc: 绑定；部分绑定和生成标记故意写错，部分方法的签名与接口只差一个类型，部分声明只在排版和注释上与另一个不同，另有不成节点的声明"
  forall change: CacheChange generated "kind 取 0 至 9，依次是在源码文件末尾加函数、在包中新增源码文件、删除测试文件（模块没有测试文件时不改动）、改写 go.mod 的 go 版本行、删除包嵌入的文件、给放在根目录旁经 replace 引入的 rapid 替身中的 TB 改名、给 go.mod 之下根目录旁的兄弟包的函数改名、给经根目录中的目录链接引入的 rapid 替身中的 TB 改名、设 GOFLAGS=-tags=uccacheprobe 引入有类型错误的文件，以及设 GOARCH=386 引入有类型错误的文件；pick 和 value 各取 0 至 1000，pick 按源码文件的个数取余选出改动所在的包，value 写进新函数的名称和返回值，也按 3 取余选出 go 版本 1.24、1.25 或 1.26"
  forall forGeneration: bool
  manual "需要写出临时 Go 模块、改动其中的文件或 go 命令的环境，再经缓存和不经缓存各用 go/packages 抽取，表达式不能读写文件"
}

property planned-check-cache-follows-changes invariant "随机 Go 模块经内容缓存检查一个计划写入的源码文件能否载入，做 change 所说的改动之后再经缓存检查同一个文件，它报错当且仅当不经缓存载入计划的代码报错" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：一至三个包，各有一至两个源码文件，可有内部测试文件和外部测试包，含零至六个类型及其方法、零至五个函数、按测试命名的函数、调用、作为值引用的函数和方法（函数名、方法值和方法表达式，包括接口的方法）、导入、嵌入、实现接口的类型、零至三个包级变量和 //uc: 绑定；部分绑定和生成标记故意写错，部分方法的签名与接口只差一个类型，部分声明只在排版和注释上与另一个不同，另有不成节点的声明"
  forall change: CacheChange generated "kind 取 0 至 9，依次是在源码文件末尾加函数、在包中新增源码文件、删除测试文件（模块没有测试文件时不改动）、改写 go.mod 的 go 版本行、删除包嵌入的文件、给放在根目录旁经 replace 引入的 rapid 替身中的 TB 改名、给 go.mod 之下根目录旁的兄弟包的函数改名、给经根目录中的目录链接引入的 rapid 替身中的 TB 改名、设 GOFLAGS=-tags=uccacheprobe 引入有类型错误的文件，以及设 GOARCH=386 引入有类型错误的文件；pick 和 value 各取 0 至 1000，pick 按源码文件的个数取余选出改动所在的包，value 写进新函数的名称和返回值，也按 3 取余选出 go 版本 1.24、1.25 或 1.26"
  manual "需要写出临时 Go 模块、改动其中的文件或 go 命令的环境，再经缓存和不经缓存各载入计划的代码，表达式不能读写文件"
}

property split-runs-cover-the-same-cases invariant "普通 go test 下，生成的驱动把一条性质的 N 个随机用例分给 8 与 N 中较小的那个数的子进程，各用不同的种子，各子进程通过的用例数之和是 N，与在一个进程中执行时通过的用例数相同" {
  about Backend
  forall checks: int
  where checks in 1..40
  manual "需要生成并编译一个 Go 模块的驱动，再拆分和不拆分各运行一次它的测试二进制并读取输出，表达式不能运行程序"
}

property split-runs-shrink-alike invariant "一条性质在奇数输入上失败时，30 至 40 个随机用例拆分到子进程执行和在一个进程中执行都失败，每个失败都缩小成 n = 1 或 n = -1" {
  about Backend
  forall checks: int
  where checks in 30..40
  manual "需要生成并编译一个 Go 模块的驱动，再拆分和不拆分各运行一次它的测试二进制并读取输出，表达式不能运行程序"
}

property split-failures-name-their-process invariant "一条性质在奇数输入上失败时，拆分执行中每个失败的子进程的报告写明性质、它是 8 与用例数中较小的那个数中的第几个、它的用例数和种子，在一个进程中从这个种子执行同样多的用例得到同一个缩小后的反例" {
  about Backend
  forall checks: int
  where checks in 30..40
  manual "需要生成并编译一个 Go 模块的驱动，再拆分执行并按报告中的种子重放它的测试二进制，读取输出，表达式不能运行程序"
}

property split-failures-replay-by-seed invariant "一条性质在奇数输入上失败时，拆分执行中每个失败的子进程报出 rapid 给出的重放种子，在一个进程中从这个种子只执行一个用例就得到同一个缩小后的反例" {
  about Backend
  forall checks: int
  where checks in 30..40
  manual "需要生成并编译一个 Go 模块的驱动，再拆分执行并按 rapid 给出的种子重放它的测试二进制，读取输出，表达式不能运行程序"
}

property derived-state-is-removed postcondition "Go 后端回放或探索一条派生函数建出目录的性质之后，派生函数记下了目录，记下的目录都已删除，不论用例通过、失败、被前提拒绝、panic 还是派生函数出错" {
  about lang.Runner.run
  forall backend: Backend
  forall outcomes: list<DerivedOutcome>
  forall explore: bool
  forall plan: lang.RunPlan generated "把一个 Go 模块写进新目录、生成驱动并建图得到的运行输入：模块中的一条性质有整数符号 mode 和派生输入 dir，派生函数新建一个目录并把它的路径记在模块根目录的 built.txt 中，outcomes 的每一项是这条性质的一个声明的用例，按其结局使 mode 为 0 时通过、为 1 时失败、为 2 时被前提拒绝、为 3 时在被测调用中 panic、为 4 时派生函数记下目录之后返回错误，explore 为真时在回放之后探索 200 毫秒"
  where outcomes size 1..5
  observe built: BuiltState from plan
  changes built
  call backend.run(plan)
  ensure built.recorded > 0 and built.remaining == 0
}

property derived-inputs-are-recomputed postcondition "Go 后端回放或探索一条带派生输入的性质时，用例冻结的输入中没有派生输入，每个用例的 mode 都在这次运行中调用过派生函数（同一次运行中 mode 相同的用例可以复制它建出的模板），派生函数出错的用例使运行报告的诊断中有 lang.golang.derivation-failed" {
  about lang.Runner.run
  forall backend: Backend
  forall outcomes: list<DerivedOutcome>
  forall explore: bool
  forall plan: lang.RunPlan generated "与 derived-state-is-removed 的 plan 相同"
  where outcomes size 1..5
  observe built: BuiltState from plan
  changes built
  call backend.run(plan)
  ensure all(r in result.results: all(c in r.cases: not "dir" in c.input.values))
  ensure all(r in result.results: all(c in r.cases: "mode" in c.input.values implies c.input.values["mode"].text in built.modes))
  ensure DerivedOutcome.underivable in outcomes implies any(d in result.diagnostics: d.code == "lang.golang.derivation-failed")
}

property derived-template-matches-fresh-derivation invariant "同一次运行中，派生输入以同样的前置符号第二次算出时不再调用派生函数而是得到模板的副本，派生值在新目录中的相对位置与第一次相同，副本的目录、文件、链接、权限位和内容与重新调用派生函数建出的相同" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：一至三个包，各有一至两个源码文件，可有内部测试文件和外部测试包，含零至六个类型及其方法、零至五个函数、按测试命名的函数、调用、作为值引用的函数和方法（函数名、方法值和方法表达式，包括接口的方法）、导入、嵌入、实现接口的类型、零至三个包级变量和 //uc: 绑定；部分绑定和生成标记故意写错，部分方法的签名与接口只差一个类型，部分声明只在排版和注释上与另一个不同，另有不成节点的声明"
  manual "需要经生成驱动的派生运行时两次算出建出目录的派生输入，再与重新建出的目录逐项比较，表达式不能读写文件"
}

property derived-template-skips-self-naming-state invariant "派生函数建出的状态中有文件写着它自己的目录时，同一次运行中以同样的前置符号第二次算出派生输入仍调用派生函数，不用模板" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：一至三个包，各有一至两个源码文件，可有内部测试文件和外部测试包，含零至六个类型及其方法、零至五个函数、按测试命名的函数、调用、作为值引用的函数和方法（函数名、方法值和方法表达式，包括接口的方法）、导入、嵌入、实现接口的类型、零至三个包级变量和 //uc: 绑定；部分绑定和生成标记故意写错，部分方法的签名与接口只差一个类型，部分声明只在排版和注释上与另一个不同，另有不成节点的声明"
  manual "需要经生成驱动的派生运行时两次算出建出目录的派生输入并数派生函数被调用的次数，表达式不能读写文件"
}

property extracts-direct-effects postcondition "抽取 root 中的模块没有诊断，抽取出的操作恰是 module.expected 中的那些，每个操作带 directEffects 当且仅当 module.expected 给它非空的效果，带的效果种类与 module.expected 中的相同" {
  about lang.Backend.extract
  forall backend: Backend
  forall module: EffectModule generated "位于根目录、源码根目录恰为 . 、依赖有版本的模块 golang.org/x/mod 的模块 example.test/m，以及一半机会另有它依赖并以目录 dep 替换的模块 example.test/dep 的文件：依赖模块有未导出的辅助函数、保存函数字面量的包级变量和一至三个导出函数（各调用零至两个标准库，可能调用辅助函数、该变量的函数和更早的导出函数），本模块有接口 Doer、类型 Impl 的方法 Do、保存函数字面量的包级变量和一至四个函数（各在函数体和其中的函数字面量中调用零至两个标准库，可能调用 semver.IsValid、依赖模块的一个导出函数、该变量的函数、本模块的一个函数和经 Doer 调用的 Do），辅助函数、变量的函数字面量和 Do 也各调用零至两个标准库；对标准库的调用取自 design.md §4 效果表中每个有效果的包、其中的例外、没有效果的包、经 net.Conn 调用的方法和表中列出的变量（单独用到或传给没有效果的函数），以及按这些调用逐个函数算出的、含抽象方法 Doer.Do 在内的每个操作的直接效果"
  forall root: string generated "把 module.files 和仓库的 go.sum 写进的新目录的路径"
  call backend.extract(root, module.roots)
  ensure size(result.diagnostics) == 0
  ensure all(id in module.expected: any(n in result.fragment.nodes: n.id == id))
  ensure all(n in result.fragment.nodes: n.kind == graph.NodeKind.op implies n.id in module.expected and (size(module.expected[n.id]) > 0) == ("directEffects" in n.attrs))
  ensure all(n in result.fragment.nodes: "directEffects" in n.attrs implies n.id in module.expected and n.attrs["directEffects"] == module.expected[n.id])
}

property records-call-durations postcondition "Go 后端回放一条性质时，每个通过的用例记下正的执行耗时（纳秒）和恰好一次调用，即被测调用 demo.probe 及其不小于零、不大于执行耗时的耗时，outcomes 中有通过的用例时报告中有通过的用例" {
  about lang.Runner.run
  forall backend: Backend
  forall outcomes: list<DerivedOutcome>
  forall explore: bool
  forall plan: lang.RunPlan generated "与 derived-state-is-removed 的 plan 相同"
  where outcomes size 1..5
  call backend.run(plan)
  ensure all(r in result.results: all(c in r.cases: c.passed implies c.duration > 0 and size(c.calls) == 1 and all(t in c.calls: t.op == "demo.probe" and t.duration >= 0 and t.duration <= c.duration)))
  ensure DerivedOutcome.passes in outcomes implies any(r in result.results: any(c in r.cases: c.passed))
}

property monitored-judgments-match-replay postcondition "回放 run.root 中采集的调用时，demo.bounded 的以反例失败的回放用例的输入哈希恰是 run.record 中 demo.bounded 的反例的输入哈希，回放中不成立的限定或前提恰是 run.record 中 demo.bounded 的域外调用的子句" {
  about Backend.run
  forall backend: Backend
  forall levels: list<int>
  forall secrets: list<string> generated "一至三个口令，各是 zq 之后八个小写字母"
  forall run: MonitoredRun generated "把受监控的模块复制到新目录再运行得到的目录、运行记录和结果：模块中纯操作 demo.score 在 3 上返回 2、在 4 上 panic、其余返回 level + 1，纯操作 demo.ratio 在 -3 上 panic，性质 demo.bounded 限定 level in -5..5、前提 level != 0、ensure result >= level 和 ratio(level) == level，纯操作 demo.unlock 接收带 sensitive 标签的 Token 和经 extends 继承它的 Badge、按口令前缀 panic-、undeclared-、denied- 分别 panic、返回未声明的错误或返回声明的错误 Denied（说明里都带着口令），接口 demo.Counter 的方法以 context.Context 为第一个参数、由 demo.Tally 实现，性质 demo.totals 在 total 上读接收者；程序以 unicode_monitor 和竞态检测构建，以抽样比例 1、打开采集运行，对 levels 中每个值经 monitor.Entry 和直接各调用一次 score，对 secrets 中每个口令以四种前缀经 monitor.Entry 调用 unlock，经 monitor.Wrap 包装一个 Tally、以带 span 的 context 调用它，再以八个 goroutine 并发调用它的 add 和 total"
  forall plan: lang.RunPlan generated "以 run.root 的设计和代码建出的图回放 demo.bounded 的运行输入，traffic 是按 language.md E29 把 run.root 中采集的调用对应成的 demo.bounded 的输入"
  where levels size 1..4
  where levels in -8..8
  call backend.run(plan)
  ensure all(r in result.results: all(c in r.cases: c.form == "traffic" and not c.passed and not c.rejected implies any(x in run.record.results: any(k in x.cases: k.inputHash == c.inputHash))))
  ensure all(x in run.record.results: x.id == "demo.bounded" implies all(k in x.cases: any(r in result.results: any(c in r.cases: c.form == "traffic" and not c.passed and not c.rejected and c.inputHash == k.inputHash))))
  ensure all(x in run.record.results: x.id == "demo.bounded" implies all(o in x.outsideDomain: any(r in result.results: any(p in r.outsideDomain: p.clause == o.clause))))
  ensure all(r in result.results: all(p in r.outsideDomain: any(x in run.record.results: any(o in x.outsideDomain: o.clause == p.clause))))
}

property monitoring-keeps-calls invariant "受监控的程序经入口包装对 levels 中的每个值调用 demo.score 得到的结果与直接调用的相同，即返回值相同、判定中 demo.ratio 的 panic 不改变调用的结果、score 的 panic 以同一个值继续，并且每个值都调用了一次" {
  about Backend
  forall levels: list<int>
  forall secrets: list<string> generated "一至三个口令，各是 zq 之后八个小写字母"
  forall run: MonitoredRun generated "与 monitored-judgments-match-replay 的 run 相同"
  where levels size 1..4
  where levels in -8..8
  ensure size(run.wrapped) == size(levels) and run.wrapped == run.direct
}

property monitoring-hides-sensitive-values invariant "受监控的程序写出的运行记录、采集的调用和导出的 span 中没有一个单词是 secrets 中的口令，尽管口令出现在敏感类型和继承它的类型的值、未声明的错误、声明的错误和 panic 的说明中" {
  about Backend
  forall levels: list<int>
  forall secrets: list<string> generated "一至三个口令，各是 zq 之后八个小写字母"
  forall run: MonitoredRun generated "与 monitored-judgments-match-replay 的 run 相同"
  where levels size 1..4
  where levels in -8..8
  ensure all(s in secrets: not s in run.words)
}

property monitoring-wraps-and-links-contexts invariant "受监控的程序中 monitor.Wrap 给登记了包装的接口 demo.Counter 一个包装，对没有登记的接口和函数原样返回，包装传给实现的 context 带着这次调用的 span，它的父 span 是调用方的" {
  about Backend
  forall levels: list<int>
  forall secrets: list<string> generated "一至三个口令，各是 zq 之后八个小写字母"
  forall run: MonitoredRun generated "与 monitored-judgments-match-replay 的 run 相同"
  where levels size 1..4
  where levels in -8..8
  ensure run.wraps and run.keepsUnregistered and run.contextLinked
}

property monitoring-reads-receivers-without-races invariant "八个 goroutine 并发调用同一个 demo.Tally 的受监控的 add 和 total 时，竞态检测没有报出竞态，demo.totals 有判定过的调用而没有反例" {
  about Backend
  forall levels: list<int>
  forall secrets: list<string> generated "一至三个口令，各是 zq 之后八个小写字母"
  forall run: MonitoredRun generated "与 monitored-judgments-match-replay 的 run 相同"
  where levels size 1..4
  where levels in -8..8
  ensure not run.raced
  ensure any(x in run.record.results: x.id == "demo.totals" and x.accepted > 0 and size(x.cases) == 0)
}

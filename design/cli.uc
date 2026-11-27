component cli "命令行：解析参数，调用其余组件，输出结果和退出码" {
  focus support
  paths "cmd/unicode/**"
  uses diag, graph, syntax, design, lang, lang.golang, lang.python, check, query, view, gen, run, serve, repo, review, evidence
}

type RepoFile record "仓库中的一个文件：内容和权限位" {
  field content: string
  field mode: int
}

type SinceSample record "changed-checks-are-selected 的输入：基准图、当前图，以及显式给出的性质或流程的标识" {
  field base: graph.Graph
  field head: graph.Graph
  field selected: list<string>
}

op affectedChecks(base: graph.Graph, head: graph.Graph, selected: list<string>) -> list<string> "供 unicode test --since 使用：比较 base 和 head，按 design.md §6.3 从变化的节点和边传播影响，返回仍存在且受影响的性质和流程，再与 selected 中显式给出的标识取交集"

op main(args: list<string>) -> int "执行一条命令，返回退出码"

flow format-files "用户运行 unicode fmt：按 design.md §8 把给出的路径（缺省为 design/ 和 corpus/）下每个合法的设计文件和录制文件改写成规范格式并保留权限位，带 --check 时只报告不改写，符号链接、不是设计文件的路径、读不出和不合法的文件只报诊断，路径参数在 design/ 和 corpus/ 之外或不存在、在仓库之外运行又不给 --root 时以退出码 2 退出且不改动仓库" {
  forall files: map<string, RepoFile> generated "一个仓库中 design/ 和 corpus/ 下的文件及权限位：总有 design/system.uc，另有至多六个文件，.uc 文件三分之一是规范格式，三分之一合法而排版随意，三分之一不合法，另有 .txt 文件、约五分之一读不出的文件，以及指向其他文件、所在目录、上一级目录或不存在的路径的符号链接"
  forall check: bool
  forall targets: list<string> generated "fmt 的路径参数：零至三个已有的文件或目录，约四分之一混入不存在的路径或 design/ 和 corpus/ 之外的路径"
  forall at: string generated "运行命令的工作目录：约三分之二是仓库之内的目录，其余是仓库之外的目录"
  forall explicitRoot: bool
  user -> main(args) {
    cli -> syntax.kindOf(path)
    cli -> syntax.parse(path, kind, src)
    cli -> syntax.File.print()
  }
  manual "需要在临时目录中写出带权限位和符号链接的仓库，切换工作目录后运行命令，表达式不能读写文件"
}

flow check-design "用户运行 unicode check：带 --design-only 时恰报出符号链接、不是设计文件的路径、语法、规范格式和只需设计就能判断的约束的诊断，不带时报出的诊断包含这些诊断并另给出 statuses，两种情形都不改动仓库" {
  forall files: map<string, RepoFile> generated "一个仓库中 design/ 和 corpus/ 下的文件：约四分之三是一至三个组件的合法设计再做零至两处改动（语法错误、排版、注释掉 focus、越界的值、未声明的 uses、移动或删除文件、多余的文件），其余是 format-files 那样的随机文件但都能读出，有时带符号链接"
  forall designOnly: bool generated "约五分之四为真"
  user -> main(args) {
    cli -> syntax.kindOf(path)
    cli -> syntax.parse(path, kind, src)
    cli -> syntax.File.print()
    cli -> design.load(files)
  }
  manual "需要在临时目录中写出仓库再运行命令，表达式不能读写文件"
}

flow check-code "用户运行 unicode check：载入设计，抽取每种已配置语言的代码，合并、推导之后检查全部约束，输出的诊断和状态与直接调用这些组件得到的相同，没有后端的语言和抽取失败的代码各报一条诊断，仓库不变" {
  forall files: map<string, RepoFile> generated "一个建图没有诊断的小仓库（一至三个组件的设计文件、Go 源码和谓词），约四分之三在一个组件处做一处改动：语法错误、排版、违反设计、代码或绑定的一条约束、让 Go 代码编译不过、配置没有后端的语言或改错源码根目录"
  user -> main(args) {
    cli -> design.load(files)
    cli -> lang.golang.newBackend()
    cli -> lang.Backend.extract(root, srcRoots)
    cli -> graph.Graph.addFragment(fragment)
    cli -> graph.Graph.derive()
    cli -> check.conform(graph)
  }
  manual "需要在临时目录中写出能由 go/packages 载入的 Go 模块，表达式不能读写文件"
}

flow build-snapshot "用户运行 unicode build：按 design.md §8 把快照写到 .unicode/graph.json 或 --out 给出的目录，快照与直接建出的图的编码逐字节相同且不随仓库所在的位置改变，旁边的 build.json 记下 git 的提交和是否有改动（不在 git 中时为 null 和有改动），仓库中的其他文件不变" {
  forall files: map<string, RepoFile> generated "一个建图没有诊断的小仓库：一至三个组件的设计文件、Go 源码和谓词，有的组件另有录制文件"
  forall committed: bool
  forall edited: bool
  user -> main(args) {
    cli -> design.load(files)
    cli -> lang.Backend.extract(root, srcRoots)
    cli -> graph.Graph.addFragment(fragment)
    cli -> graph.Graph.derive()
    cli -> graph.Graph.encode()
  }
  manual "需要在临时目录中用 git 提交仓库再运行命令，表达式不能读写文件或运行 git"
}

flow query-graph "用户运行 node get、node search、node refs、slice、impact 或 view，可带 --rev HEAD：JSON 输出与直接调用 query 和 view 的结果相同，文本输出含其中的每个标识，slice --with-code 另附各代码元素的声明，标识不存在时退出码为 3，视图不适用或给了它不读的 --layer、--level 时退出码为 2" {
  forall files: map<string, RepoFile> generated "一个建图没有诊断的小仓库：一至三个组件的设计文件、Go 源码和谓词，有的组件另有录制文件"
  forall command: list<string> generated "六种查询命令之一及其参数：标识取自这个仓库的设计和 Go 源码，约十分之一不存在，搜索的文本是一个标识的一段，视图的输入约五分之四是该视图接受的节点，--layer 和 --level 各约四分之一给出而不论视图，约五分之一带 --rev HEAD"
  user -> main(args) {
    cli -> design.load(files)
    cli -> graph.Graph.derive()
    cli -> query.slice(g, ids, expand)
    cli -> view.select(g, kind, id, expand, layer, level)
  }
  manual "需要在临时目录中写出仓库，带 --rev 时还要用 git 提交，再运行命令，表达式不能读写文件"
}

flow diff-snapshots "用户运行 unicode diff：按 design.md §8 比较一个提交与工作区或另一个提交的图，JSON 的 nodes 和 edges 与直接调用 graph.compare 的结果相同，base 和 head 与 git 给出的提交相符，文本输出从基准提交写起并列出每个变化的节点和边，提交读不出时退出码为 2" {
  forall base: map<string, RepoFile> generated "一个建图没有诊断的小仓库：一至三个组件的设计文件、Go 源码和谓词，有的组件另有录制文件"
  forall head: map<string, RepoFile> generated "一个建图没有诊断的小仓库：一至三个组件的设计文件、Go 源码和谓词，有的组件另有录制文件"
  forall toCommit: bool
  forall jsonOutput: bool
  forall readable: bool
  user -> main(args) {
    cli -> design.load(files)
    cli -> lang.Backend.extract(root, srcRoots)
    cli -> graph.Graph.addFragment(fragment)
    cli -> graph.Graph.derive()
    cli -> graph.compare(base, head)
  }
  manual "需要用 git 提交基准再改写工作区，表达式不能读写文件或运行 git"
}

flow serve-viewer "用户运行 unicode serve：按 design.md §8 读入快照并开始监听，写出页面的地址，GET /api/snapshot 得到快照的节点数，POST 得到 405，每个请求记在标准错误中，收到中断信号后以退出码 0 退出，还没有快照时以退出码 7 退出并提示运行 unicode build" {
  forall files: map<string, RepoFile> generated "一个建图没有诊断的小仓库：一至三个组件的设计文件、Go 源码和谓词，有的组件另有录制文件"
  forall built: bool
  user -> main(args) {
    cli -> serve.open(root, snapshot, builder)
    cli -> serve.Server.answer(request)
  }
  manual "需要启动 HTTP 服务、发出请求并向进程发送中断信号，表达式不能做这些"
}

property changed-checks-are-selected invariant "affectedChecks 返回的恰是当前图中仍存在、按 design.md §6.3 的规则从两张图的变化传播到的性质和流程，按标识排序，显式给出标识时只保留这些标识中受影响的，两张图在调用后不变" {
  about affectedChecks
  forall sample: SinceSample generated "两张小图：一半是带包级声明和导入关系（有时成环）的四个代码包，在一处改动包级声明、文件或导入，或只改位置，或不改，另一半是零至四组性质、流程、观察、用例和代码，每组不改或改动函数体、ensure、摘要、用例、调用、绑定、增删性质或只改位置，约十分之一两张图都为空；约一半另有显式的选择，有时重复同一标识"
  manual "需要在两张图上反复传播影响直到不再变化，表达式没有迭代到不动点的构造"
}

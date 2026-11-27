component cli "命令行：解析参数，调用其余组件，输出结果和退出码" {
  focus support
  paths "cmd/unicode/**"
  uses diag, graph, syntax, design, lang, lang.golang, lang.python, check, query, view, gen, run, serve, repo
}

type RepoFile record "仓库中的一个文件：内容和权限位" {
  field content: string
  field mode: int
}

op main(args: list<string>) -> int "执行一条命令，返回退出码"

flow format-files "用户运行 unicode fmt：把给出的路径之下、缺省为 design/ 和 corpus/ 下的每个文件改写成规范格式；带 --check 时只报告，不改写；路径不在这两个目录之内或不存在时是参数错误；不给出 --root 时，从工作目录向上找含 design/system.uc 的目录，找不到时是参数错误；读不出的文件给出 cli.unreadable-file" {
  forall files: map<string, RepoFile> generated "一个仓库中 design/ 和 corpus/ 下的文件，路径到内容和权限位；有的合法，有的不合法，有的不是规范格式，有的读不出"
  forall check: bool
  forall targets: list<string> generated "fmt 的路径参数：没有，或 design/ 和 corpus/ 中的文件和目录，有时混入不存在的路径或这两个目录之外的路径"
  forall at: string generated "运行命令的工作目录：仓库的根目录、它的某个子目录，或者仓库之外的目录"
  forall explicitRoot: bool
  user -> main(args) {
    cli -> syntax.kindOf(path)
    cli -> syntax.parse(path, kind, src)
    cli -> syntax.File.print()
  }
}

flow check-design "用户运行 unicode check --design-only：解析 design/ 和 corpus/ 下的全部文件，报出语法、规范格式和只需设计就能判断的约束的诊断；不带 --design-only 时，报出的诊断包含 --design-only 的全部诊断，并给出各声明项的状态" {
  forall files: map<string, RepoFile> generated "一个仓库中 design/ 和 corpus/ 下的文件，有的合法，有的带语法错误、不是规范格式或违反约束"
  forall designOnly: bool generated "沿用原 IntRange(0,4) 大于零的取值方式与边界偏置"
  user -> main(args) {
    cli -> syntax.kindOf(path)
    cli -> syntax.parse(path, kind, src)
    cli -> syntax.File.print()
    cli -> design.load(files)
  }
}

flow check-code "用户运行 unicode check：载入设计，抽取每种已配置语言的代码，合并、推导之后检查全部约束，输出诊断" {
  forall files: map<string, RepoFile> generated "一个小仓库：设计文件和 Go 源码，有的处处合乎设计，有的在一处违反了某条约束"
  user -> main(args) {
    cli -> design.load(files)
    cli -> lang.golang.newBackend()
    cli -> lang.Backend.extract(root, srcRoots)
    cli -> graph.Graph.addFragment(fragment)
    cli -> graph.Graph.derive()
    cli -> check.conform(graph)
  }
}

flow build-snapshot "用户运行 unicode build：建出图，把快照写到 .unicode/graph.json，把仓库所在的提交和工作区是否有改动写到 .unicode/build.json；不在 Git 的工作区中、或者还没有提交时，提交为 null，工作区算作有改动；build 自己写出的这两个文件不算改动；相同的仓库总是写出逐字节相同的快照" {
  forall files: map<string, RepoFile> generated "一个合法的小仓库：设计文件、录制文件和 Go 源码"
  forall committed: bool
  forall edited: bool
  user -> main(args) {
    cli -> design.load(files)
    cli -> lang.Backend.extract(root, srcRoots)
    cli -> graph.Graph.addFragment(fragment)
    cli -> graph.Graph.derive()
    cli -> graph.Graph.encode()
  }
}

flow query-graph "用户运行 node get、node search、node refs、slice、impact 或 view：JSON 输出与直接调用 query 和 view 得到的结果相同；标识不存在时退出码为 3" {
  forall files: map<string, RepoFile> generated "一个合法的小仓库：设计文件、录制文件和 Go 源码"
  forall command: list<string> generated "一条查询命令及其参数，标识取自这个仓库的图，有时是不存在的标识"
  user -> main(args) {
    cli -> design.load(files)
    cli -> graph.Graph.derive()
    cli -> query.slice(g, ids, expand)
    cli -> view.select(g, kind, id, expand, layer, level)
  }
}

flow diff-snapshots "用户运行 unicode diff <rev>：像查询的 --rev 一样取出该提交并建图，与工作区的图比较；带 --to <rev> 时，与另一个提交的图比较；JSON 的 nodes 和 edges 与直接调用 graph.compare 得到的相同，base 和 head 给出两端的提交，head 另给出它是否有未提交的改动；文本列出每个变化的节点和边；提交读不出时是参数错误" {
  forall base: map<string, RepoFile> generated "一个合法的小仓库：设计文件、录制文件和 Go 源码，提交为比较的基准"
  forall head: map<string, RepoFile> generated "另一个合法的小仓库，写在基准的工作区中：留在工作区，或者再提交一次"
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
}

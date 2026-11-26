component cli "命令行：解析参数，调用其余组件，输出结果和退出码" {
  focus support
  paths "cmd/unicode/**"
  uses diag, graph, syntax, design, lang, lang.golang, lang.python, check, query, view, gen, run
}

type RepoFile record "仓库中的一个文件：内容和权限位" {
  field content: string
  field mode: int
}

op main(args: list<string>) -> int "执行一条命令，返回退出码"

flow format-files "用户运行 unicode fmt：把 design/ 和 corpus/ 下的每个文件改写成规范格式；带 --check 时只报告，不改写" {
  forall files: map<string, RepoFile> generated "一个仓库中 design/ 和 corpus/ 下的文件，路径到内容和权限位；有的合法，有的不合法，有的不是规范格式"
  forall check: bool
  user -> main(args) {
    cli -> syntax.kindOf(path)
    cli -> syntax.parse(path, kind, src)
    cli -> syntax.File.print()
  }
}

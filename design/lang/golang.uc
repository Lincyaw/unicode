component lang.golang "Go 后端：抽取、名称映射、生成与运行" {
  paths "internal/lang/golang/**"
  uses lang
}

type Backend object "Go 的后端：用 go/packages 和 go/types 静态抽取" {
  implements lang.Backend
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

property extracts-generated-module invariant "随机生成的 Go 模块，抽取得到的节点、边和诊断恰是生成时记下的那些，签名按 design.md §4 归一化；诊断写作 诊断码 路径:行" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：几个包和文件，类型、函数、方法、调用、导入、嵌入、接口的实现、测试文件，以及 //uc: 绑定，其中有的写法不合格式"
}

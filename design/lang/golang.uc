component lang.golang "Go 后端：抽取、名称映射、生成与运行" {
  paths "internal/lang/golang/**"
  uses lang
}

type Backend object "Go 的后端：用 go/packages 和 go/types 静态抽取" {
  implements lang.Backend
}

type Module record "一个随机生成的 Go 模块：文件，以及抽取应当得到的节点和边，供性质使用" {
  field files: map<string, string>
  field nodes: list<string>
  field edges: list<string>
}

op newBackend() -> Backend "一个 Go 后端"

property extracts-generated-module invariant "随机生成的 Go 模块，抽取得到的节点和边恰是生成时记下的那些，签名按 design.md §4 归一化；不合格式的绑定恰好各得到一条诊断" {
  about Backend
  forall module: Module generated "一个随机的 Go 模块：几个包和文件，类型、函数、方法、调用、导入、嵌入、接口的实现、测试文件，以及 //uc: 绑定，其中有的写法不合格式"
}

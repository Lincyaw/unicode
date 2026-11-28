component serve "查看器的后端：读取 unicode build 写出的快照，载入时按 unicode check 的约束检查，回答 api/openapi.yaml 的只读接口；前端的构建结果嵌入在这个组件中" {
  focus support
  paths "internal/serve/**"
  uses graph, diag, check, query, view, lang, lang.golang, repo, review
}

type Unreadable error "快照文件不存在、读不出、不是 unicode build 写出的快照，或者旁边的 build 记录不合格式"

type Unbuildable error "比较的基准提交取不出文件，或者建不出图"

type Request record "一次接口请求：以 /api 开头的路径和查询参数" {
  field path: string
  field query: map<string, list<string>>
}

type Response record "一次接口请求的应答：状态码和 JSON 正文" {
  field status: int
  field body: bytes
}

type Server object "查看器的后端：持有读入的快照及其检查结果，每个快照只检查一次；快照文件改变后，下一次请求时重新读入" {
  op answer(request: Request) -> Response "按 api/openapi.yaml 回答一次接口请求；拒绝的请求得到 {code, message, fix}，诊断码为 serve.*"
}

type Builder interface "建出仓库在一个提交上的图，像查询的 --rev 那样取出该提交的文件再建图；由命令行提供" {
  op build(root: string, commit: string) -> graph.Graph raises Unbuildable
}

type Exchange record "serve 的性质和流程 answer-request 的输入：一个仓库（快照中的图和工作区中的文件），以及查看器依次发出的一组接口请求" {
  field fragment: graph.Fragment
  field files: map<string, string>
  field requests: list<Request>
}

op open(root: string, snapshot: string, builder: Builder) -> Server raises Unreadable "读入仓库 root 的快照文件 snapshot，按 unicode check 的约束检查一遍；比较的基准由 builder 建图"

flow answer-request "查看器发来一次接口请求时，快照文件自上次读入后改变过就先重新读入、解码并检查，之后的应答与只读入新快照的服务给出的逐字节相同，新快照读不出时 /api/snapshot 得到 503 serve.unreadable-snapshot，文件恢复后照常回答" {
  forall exchange: Exchange generated "一个随机小仓库的图和文件（系统、人、外部系统、一至三个组件及其类型、操作、流程、性质、用例和 Go 代码，以及零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对各个接口和一个不是接口的路径的一至六个请求：标识约七分之一不存在，参数有时不合格式，约五分之一重复前面的请求"
  user -> Server.answer(request) {
    opt "快照文件改变过" {
      Server -> graph.decode(data)
      Server -> check.conform(g)
      Server -> review.recorded(g)
    }
  }
  manual "需要在两批请求之间改写磁盘上的快照文件，表达式不能读写文件"
}

property answers-follow-contract invariant "每个应答都合乎 api/openapi.yaml：状态码是该接口声明的，正文合乎声明的模式，拒绝的正文是诊断码为 serve.* 的 {code, message, fix}，不是接口的路径得到 404" {
  about Server.answer
  forall exchange: Exchange generated "一个随机小仓库的图和文件（系统、人、外部系统、一至三个组件及其类型、操作、流程、性质、用例和 Go 代码，以及零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对各个接口和一个不是接口的路径的一至六个请求：标识约七分之一不存在，参数有时不合格式，约五分之一重复前面的请求"
  manual "需要按 api/openapi.yaml 校验每个应答，表达式不能读取和校验 OpenAPI 文档"
}

property answers-do-not-escape-html invariant "每个应答的 JSON 正文都把 <、> 和 & 原样写出，不写成 Unicode 转义序列" {
  about Server.answer
  forall exchange: Exchange generated "一个随机小仓库的图和文件（系统、人、外部系统、一至三个组件及其类型、操作、流程、性质、用例和 Go 代码，以及零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对各个接口和一个不是接口的路径的一至六个请求：标识约七分之一不存在，参数有时不合格式，约五分之一重复前面的请求"
  manual "需要在每个应答的原始正文中查找 <、> 和 & 的转义序列，表达式不能在字节中查找子串"
}

property identities-resolve invariant "成功应答中在属性之外作为节点标识出现的每个值都能经 /api/entity 取到实体页，只在基准图中的项除外（比较中删除的项，以及各项在基准中的内容），snapshot、tree、entity、node、edge、slice 和视图的成功应答至少含一个这样的值" {
  about Server.answer
  forall exchange: Exchange generated "一个随机小仓库的图和文件（系统、人、外部系统、一至三个组件及其类型、操作、流程、性质、用例和 Go 代码，以及零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对各个接口和一个不是接口的路径的一至六个请求：标识约七分之一不存在，参数有时不合格式，约五分之一重复前面的请求"
  manual "需要在应答的 JSON 中找出全部节点标识再逐个请求，表达式不能解析 JSON"
}

property queries-agree relation "node、search、slice、impact、views、compare 和 reviews 七个接口的应答与直接调用 query、view、graph.compare 和 review.evaluate（记录取自快照的评审层）的结果有同样的项且次序相同（节点和边，视图另有同样的折叠和隐藏，评审项另有同样的状态、指纹和记录），实体页的评审与 review.evaluate 对该节点给出的相同，标识不存在、参数不合、视图不适用或基准提交读不出时应答是 design.md §7.5.1 规定的拒绝" {
  about Server.answer, query.get, query.search, query.slice, query.impact, view.select, graph.compare, review.evaluate, review.recorded
  forall exchange: Exchange generated "一个随机小仓库的图和文件（带零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对 node、search、slice、impact、views、compare、reviews 和 entity 的一至六个请求：标识约七分之一不存在，参数有时不合，比较的基准约五分之二是 HEAD，其余是读不出的提交，reviews 带零至三个标识、pending 为真、为假或不给，约三分之一带基准 HEAD，约五分之一重复前面的请求"
  manual "需要解码 JSON 应答再与直接调用的结果比较，表达式不能解析 JSON"
}

property tabs-open-views invariant "实体页和视图的 tabs 按标签的次序每种视图恰好一项，各项给出能由 /api/views 回答的请求或者关闭的原因，undeclared-notation 只在 DDD 或 C4 视图的记法未声明时出现，no-input 只在图中没有该视图能接受的输入时出现" {
  about Server.answer
  forall exchange: Exchange generated "一个随机小仓库的图和文件（带零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对 /api/entity 和 /api/views 的一至六个请求：标识约七分之一不存在，实体页约六分之一带基准 HEAD，视图的种类、层和层级有时不合，约五分之一重复前面的请求"
  manual "需要对图中每个节点调用 view.select 判断视图是否适用，表达式中只能调用纯操作，而 view.select 不是纯操作"
}

property code-tab-ranks-files invariant "组件的实体页没有代码的相关视图、而组件拥有代码视图接受的文件时，代码标签指向其中排在最前的文件，不是测试文件的在前，落实设计节点的代码元素多的其次，最后按路径" {
  about Server.answer
  forall exchange: Exchange generated "一个随机小仓库的图和文件，每个组件有一至三个代码文件，可有第二个文件和一个 _test.go 文件，三者路径的次序与这一排序相反，各文件随机分到组件的类型、函数和谓词，以及对 /api/entity 的一至六个请求：标识约七分之一不存在，约六分之一带基准 HEAD，约五分之一重复前面的请求"
  manual "需要对组件拥有的每个文件调用 view.select 判断代码视图是否接受它，表达式中只能调用纯操作，而 view.select 不是纯操作"
}

property answers-are-deterministic invariant "同一个快照上，同一个请求的应答逐字节相同，与它之前回答过哪些请求无关" {
  about Server.answer
  forall exchange: Exchange generated "一个随机小仓库的图和文件（系统、人、外部系统、一至三个组件及其类型、操作、流程、性质、用例和 Go 代码，以及零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对各个接口和一个不是接口的路径的一至六个请求：标识约七分之一不存在，参数有时不合格式，约五分之一重复前面的请求"
  manual "需要以相反的次序把同一组请求再回答一遍，表达式不能重排列表"
}

property snapshot-head-follows-record invariant "/api/snapshot 的 head 按 design.md §7.5.1 取自 build 记录，git 读得出记录的提交时另带缩写、说明、时间和分支，没有 build 记录时取自 git 给出的工作区" {
  about Server.answer
  forall exchange: Exchange generated "一个随机小仓库的图和文件（系统、人、外部系统、一至三个组件及其类型、操作、流程、性质、用例和 Go 代码，以及零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对各个接口和一个不是接口的路径的一至六个请求：标识约七分之一不存在，参数有时不合格式，约五分之一重复前面的请求"
  forall record: optional<repo.Head> generated "三种情形各约三分之一：没有 build 记录，提交为 null 且有改动的 build 记录，或者记有一个 git 读不出的随机四十位十六进制提交和随机的是否有改动的 build 记录"
  manual "需要写出 build 记录并用 git 提交仓库，表达式不能读写文件或运行 git"
}

property open-refuses-malformed-record invariant "快照旁的 build 记录不是 {commit, dirty} 时，open 报 Unreadable" {
  about open
  forall exchange: Exchange generated "一个随机小仓库的图和文件（系统、人、外部系统、一至三个组件及其类型、操作、流程、性质、用例和 Go 代码，以及零至三条评审记录的评审层，约三分之一的被评节点在评审之后改了摘要），以及对各个接口和一个不是接口的路径的一至六个请求：标识约七分之一不存在，参数有时不合格式，约五分之一重复前面的请求"
  forall record: string generated "不合 {commit, dirty} 的 build 记录文本：不是 JSON 的文本，不是对象的 JSON 值，缺 commit 或 dirty 的对象，多一个字段的对象，或者 commit 或 dirty 的类型不对的对象"
  manual "需要在快照旁写出 record 作为 build 记录再调用 open，表达式不能读写文件"
}

property pages-answer-within-budget postcondition "对一个与 unicode 自身同等规模的生成仓库，查看器以 200 应答页面的请求，应答耗时的第 95 百分位不超过 within 的时长" {
  about Server.answer
  forall seed: int
  forall server: Server generated "以 seed 为种子构造的一个仓库用 git 提交、写成快照后打开的查看器后端，基准提交建出的图是快照去掉最后一个非系统节点并改动第一个非系统节点的图，种子相同的用例共用一个：仓库有 18 个组件，各有 7 个类型、4 个操作和 4 条性质（一条人工性质、一条带用例），其中 9 个组件另有一个 4 步的流程，每个组件一个 12 个文件的 Go 包，共 4104 个函数和 1080 个类型，各包按组件的次序分层调用，共 5771 个节点和约三万六千条边，边的种类、出处和显著性的构成与 unicode 自身的图相近"
  forall request: Request generated "seed 的仓库上的页面请求，在 /api/snapshot、/api/tree、/api/diagnostics、/api/entity、/api/nodes、/api/edge、/api/search 和 /api/reviews 中均匀地取：实体页的标识和节点组的一至三个标识取自图中的节点，边取自图中的边，搜索词是某个节点标识中非空的一段，/api/reviews 列出图中一个节点的评审项，pending 取真或假，约四分之三带基准提交 HEAD"
  where seed in 1..2
  call server.answer(request)
  within "100ms"
  ensure result.status == 200
}

property queries-answer-within-budget postcondition "对一个与 unicode 自身同等规模的生成仓库，查看器以 200 应答查询和视图的请求，应答耗时的第 95 百分位不超过 within 的时长" {
  about Server.answer
  forall seed: int
  forall server: Server generated "以 seed 为种子构造的一个仓库用 git 提交、写成快照后打开的查看器后端，基准提交建出的图是快照去掉最后一个非系统节点并改动第一个非系统节点的图，种子相同的用例共用一个：仓库有 18 个组件，各有 7 个类型、4 个操作和 4 条性质（一条人工性质、一条带用例），其中 9 个组件另有一个 4 步的流程，每个组件一个 12 个文件的 Go 包，共 4104 个函数和 1080 个类型，各包按组件的次序分层调用，共 5771 个节点和约三万六千条边，边的种类、出处和显著性的构成与 unicode 自身的图相近"
  forall request: Request generated "seed 的仓库上的查询和视图请求，在 /api/node、/api/slice、/api/impact 和视图中均匀地取：前三者的标识取自图中的节点，视图在十种中均匀地取，输入取自 view.inputs 给出的这种视图的输入"
  where seed in 1..2
  call server.answer(request)
  within "500ms"
  ensure result.status == 200
}

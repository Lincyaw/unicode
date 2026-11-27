component serve "查看器的后端：读取 unicode build 写出的快照，载入时按 unicode check 的约束检查，回答 api/openapi.yaml 的只读接口；前端的构建结果嵌入在这个组件中" {
  focus support
  paths "internal/serve/**"
  uses graph, diag, check, query, view, lang, lang.golang, repo, review, evidence
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

type Server object "查看器的后端：持有读入的快照及其检查结果；快照文件改变后，下一次请求时重新读入" {
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

flow answer-request "查看器发来一次接口请求时，快照文件自上次读入后改变过就先重新读入、解码并检查，此后的应答都来自新的快照" {
  forall exchange: Exchange generated "一张随机的图写成快照，以及对各个接口的一至六个请求，其中约五分之一重复前面的请求"
  user -> Server.answer(request) {
    opt "快照文件改变过" {
      Server -> graph.decode(data)
      Server -> check.conform(g)
    }
  }
  manual "需要在两批请求之间改写磁盘上的快照文件，表达式不能读写文件"
}

property answers-follow-contract invariant "每个应答都合乎 api/openapi.yaml：状态码是该接口声明的，正文合乎声明的模式，拒绝的正文是诊断码为 serve.* 的 {code, message, fix}，不是接口的路径得到 404" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对各个接口的请求：标识约七分之一不存在，参数有时不合格式，也有不是接口的路径"
  manual "需要按 api/openapi.yaml 校验每个应答，表达式不能读取和校验 OpenAPI 文档"
}

property answers-do-not-escape-html invariant "应答的 JSON 与快照的写法一样，不把 <、> 和 & 写成转义序列" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对各个接口的请求：标识约七分之一不存在，参数有时不合格式，也有不是接口的路径"
  manual "需要在每个应答的原始正文中查找 <、> 和 & 的转义序列，表达式不能在字节中查找子串"
}

property identities-resolve invariant "应答中作为节点标识出现的每个值，都能经 /api/entity 取到实体页，只在基准图中的项除外（比较中删除的项，以及各项在基准中的内容）" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对各个接口的请求，标识多取自图中的节点"
  manual "需要在应答的 JSON 中找出全部节点标识再逐个请求，表达式不能解析 JSON"
}

property queries-agree relation "node、search、slice、impact、views 和 compare 六个接口的应答，与直接调用 query、view 和 graph.compare 的结果有同样的节点和边且次序相同，直接调用出错时应答是 design.md §7.5.1 规定的拒绝" {
  about Server.answer, query.get, query.search, query.slice, query.impact, view.select, graph.compare
  forall exchange: Exchange generated "一张随机的图，以及对 node、search、slice、impact、views 和 compare 的请求：标识约七分之一不存在，参数有时不合，比较的基准约五分之二是 HEAD，其余是读不出的提交，基准的图比当前的图少一个节点并改了 user 节点"
  manual "需要解码 JSON 应答再与直接调用的结果比较，表达式不能解析 JSON"
}

property tabs-open-views invariant "实体页和视图的 tabs 按 design.md §7.5.1 给出：每种视图恰好一项，给出的请求能由 /api/views 回答，关闭的原因只在视图的记法未声明或图中没有它能接受的输入时出现" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对实体页和视图的请求，标识多取自图中的节点"
  manual "需要对图中每个节点调用 view.select 判断视图是否适用，表达式不能遍历图的节点"
}

property answers-are-deterministic invariant "同一个快照上，同一个请求的应答逐字节相同，与它之前回答过哪些请求无关" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对各个接口的一至六个请求，其中约五分之一重复前面的请求"
  manual "需要以相反的次序把同一组请求再回答一遍，表达式不能重排列表"
}

property snapshot-head-follows-record invariant "/api/snapshot 的 head 按 design.md §7.5.1 取自 build 记录，git 读得出记录的提交时另带缩写、说明、时间和分支，没有 build 记录时取自 git 给出的工作区" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图和它的文件"
  forall record: optional<repo.Head> generated "三种情形各约三分之一：没有 build 记录，提交为空的 build 记录，或者记有一个 git 读不出的随机提交和随机的是否有改动"
  manual "需要写出 build 记录并用 git 提交仓库，表达式不能读写文件或运行 git"
}

property open-refuses-malformed-record invariant "快照旁的 build 记录不是 {commit, dirty} 时，open 报 Unreadable" {
  about open
  forall exchange: Exchange generated "一张随机的图和它的文件"
  forall record: string generated "不合 {commit, dirty} 的 build 记录文本：不是 JSON 的文本，不是对象的 JSON 值，缺 commit 或 dirty 的对象，多一个字段的对象，或者 commit 或 dirty 的类型不对的对象"
  manual "需要在快照旁写出 record 作为 build 记录再调用 open，表达式不能读写文件"
}

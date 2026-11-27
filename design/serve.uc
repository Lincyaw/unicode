component serve "查看器的后端：读取 unicode build 写出的快照，载入时按 unicode check 的约束检查，回答 api/openapi.yaml 的只读接口；前端的构建结果嵌入在这个组件中" {
  focus support
  paths "internal/serve/**"
  uses graph, diag, check, query, view, lang.golang, repo
}

type Unreadable error "快照文件不存在、读不出，或者不是 unicode build 写出的快照"

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

type Exchange record "一个仓库和对它的查看器的一组接口请求：快照中的图、工作区中的文件和请求，供性质使用" {
  field fragment: graph.Fragment
  field files: map<string, string>
  field requests: list<Request>
}

op open(root: string, snapshot: string, builder: Builder) -> Server raises Unreadable "读入仓库 root 的快照文件 snapshot，按 unicode check 的约束检查一遍；比较的基准由 builder 建图"

flow answer-request "查看器发来一次接口请求：快照文件自上次读入后改变过时，先重新读入、解码，并按 unicode check 的约束检查，再回答；此后的应答都来自新的快照" {
  forall exchange: Exchange generated "一张随机的图写成快照，以及对它的各个接口的请求；请求之间快照文件有时被改写成另一张图"
  user -> Server.answer(request) {
    opt "快照文件改变过" {
      Server -> graph.decode(data)
      Server -> check.conform(g)
    }
  }
}

property answers-follow-contract invariant "每个应答都合乎 api/openapi.yaml：状态码是该接口声明的，正文合乎声明的模式；拒绝的正文是诊断码为 serve.* 的 {code, message, fix}；不是接口的路径得到 404；JSON 与快照的写法一样，不把 <、> 和 & 转义" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对各个接口的请求：标识多取自图中的节点，有时不存在，参数有时不合格式"
  manual "需要启动服务并检查 HTTP、OpenAPI、请求顺序和 JSON 引用，表达式没有这些环境操作"
}

property identities-resolve invariant "应答中作为节点标识出现的每个值，经 /api/entity 都能取到实体页；比较中各项在基准中的样子和删除的项不在快照中，除外" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对各个接口的请求，标识多取自图中的节点"
  manual "需要启动服务并检查 HTTP、OpenAPI、请求顺序和 JSON 引用，表达式没有这些环境操作"
}

property queries-agree relation "/api/node、/api/search、/api/slice、/api/impact、/api/views 和 /api/compare 的应答，与直接调用 query、view 和 graph.compare 的结果有同样的节点和边，次序相同；标识不存在时为 404 serve.not-found，视图不适用于输入时为 400 serve.not-applicable，基准不是提交时为 400 serve.bad-revision，layer 给了类视图之外的视图、level 给了 C4 视图之外的视图，或者不是它们的取值时，为 400 serve.bad-parameter" {
  about Server.answer, query.get, query.search, query.slice, query.impact, view.select, graph.compare
  forall exchange: Exchange generated "一张随机的图，以及对这六个接口的请求，标识多取自图中的节点，有时不存在；比较的基准是另一张图，它的提交有时读不出"
  manual "需要启动服务并检查 HTTP、OpenAPI、请求顺序和 JSON 引用，表达式没有这些环境操作"
}

property answers-are-deterministic invariant "同一个快照上，同一个请求的应答逐字节相同，与它之前回答过哪些请求无关" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图，以及对各个接口的请求，有的重复出现"
  manual "需要启动服务并检查 HTTP、OpenAPI、请求顺序和 JSON 引用，表达式没有这些环境操作"
}

property snapshot-head-follows-record invariant "/api/snapshot 的 head 取自快照旁的 build.json：提交和是否有改动与记录相同，git 读得出这个提交时另带它的缩写、说明、时间和检出在它上面的分支；记录的提交为空时其余各项为空；没有记录时取自读入快照时 git 给出的工作区；记录不是 {commit, dirty} 时快照读不出" {
  about Server.answer
  forall exchange: Exchange generated "一张随机的图和它的文件"
  forall record: optional<repo.Head> generated "没有记录；没有提交的记录；或者记有一个随机的提交哈希和是否有改动的记录"
  manual "需要写出仓库、提交和记录文件，再经 HTTP 读出快照的元数据，表达式没有这些环境操作"
}

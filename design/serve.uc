component serve "查看器的后端：读取 unicode build 写出的快照，载入时按 unicode check 的约束检查，回答 api/openapi.yaml 的只读接口" {
  focus support
  paths "internal/serve/**"
  uses graph, diag, check, query, view, lang.golang
}

type Unreadable error "快照文件不存在、读不出，或者不是 unicode build 写出的快照"

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

op open(root: string, snapshot: string) -> Server raises Unreadable "读入仓库 root 的快照文件 snapshot，按 unicode check 的约束检查一遍"

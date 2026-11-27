component diag "诊断：位置、诊断码、原因与修正" {
  focus utility
  paths "internal/diag/**"
}

type Position record "文件中的一个位置；行和列都从 1 起，列按字节计" {
  field path: string
  field line: int
  field column: int
}

type Diagnostic record "一条诊断：违反某条规则或约束的一处，含诊断码、出错的节点、边或文件（subject）、位置、原因和修正办法" {
  field code: string
  field subject: string
  field position: Position
  field message: string
  field fix: string
}

property diagnostic-json-is-flat invariant "诊断编码成的 JSON 恰有 code、subject、path、line、column、message、fix 七个顶层字段，各取诊断中对应的值，其中 path、line、column 取自位置，解码后得到原来的诊断" {
  about Diagnostic
  forall diagnostic: Diagnostic generated "任意诊断：各字符串字段取任意 UTF-8 文本，包括空串，行和列取任意整数，包括 0、负数和很大的数"
  manual "需要把诊断编码成 JSON 再解码，表达式不能编码或解析 JSON"
}

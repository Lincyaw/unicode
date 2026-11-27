component diag "诊断：位置、诊断码、原因与修正" {
  focus utility
  paths "internal/diag/**"
}

type Position record "文件中的一个位置；行和列都从 1 起，列按字节计" {
  field path: string
  field line: int
  field column: int
}

type Diagnostic record "违反一条约束的一处：诊断码、对象、位置、原因与修正" {
  field code: string
  field subject: string
  field position: Position
  field message: string
  field fix: string
}

property diagnostic-json-is-flat invariant "诊断的 JSON 恰有 code、subject、path、line、column、message、fix 七个字段，位置平铺；序列化后反序列化保留原诊断全部字段" {
  about Diagnostic
  forall diagnostic: Diagnostic
  manual "JSON 编码与字段集合不能在当前表达式中直接检查，手写谓词解码后逐字段比较"
}

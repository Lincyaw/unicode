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

type Flat record "诊断的线上形式：诊断码、subject、位置的路径、行和列、原因和修正各是一个字段，编码成 JSON 时键名就是字段名" {
  field code: string
  field subject: string
  field path: string
  field line: int
  field column: int
  field message: string
  field fix: string
}

op flatten(diagnostic: Diagnostic) -> Flat pure "诊断的线上形式：位置拆成路径、行和列三个字段，其余字段照抄"

op unflatten(flat: Flat) -> Diagnostic pure "由线上形式还原诊断：路径、行和列合成位置，其余字段照抄"

property flatten-copies-fields postcondition "flatten 的结果中，code、subject、message 和 fix 与诊断的相同，path、line 和 column 与诊断的位置的相同" {
  about flatten
  forall diagnostic: Diagnostic generated "任意诊断：各字符串字段取任意 UTF-8 文本，包括空串，行和列取任意整数，包括 0、负数和很大的数"
  call flatten(diagnostic)
  ensure result.code == diagnostic.code and result.subject == diagnostic.subject and result.message == diagnostic.message and result.fix == diagnostic.fix
  ensure result.path == diagnostic.position.path and result.line == diagnostic.position.line and result.column == diagnostic.position.column
}

property unflatten-inverts-flatten relation "unflatten 还原 flatten 的结果，得到原来的诊断" {
  about flatten, unflatten
  forall diagnostic: Diagnostic generated "任意诊断：各字符串字段取任意 UTF-8 文本，包括空串，行和列取任意整数，包括 0、负数和很大的数"
  ensure unflatten(flatten(diagnostic)) == diagnostic
}

property diagnostic-json-is-flat invariant "诊断编码成的 JSON 恰有 code、subject、path、line、column、message、fix 七个顶层键，各取诊断中对应的值，其中 path、line、column 取自位置，解码后得到原来的诊断" {
  about Diagnostic
  forall diagnostic: Diagnostic generated "任意诊断：各字符串字段取任意 UTF-8 文本，包括空串，行和列取任意整数，包括 0、负数和很大的数"
  manual "需要把诊断编码成 JSON 再按键读回，表达式不能编码或解析 JSON"
}

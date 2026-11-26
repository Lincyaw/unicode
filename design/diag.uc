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

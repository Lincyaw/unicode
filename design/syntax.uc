component syntax "设计文件和录制文件的词法、语法、语法树和规范格式的打印" {
  focus core
  paths "internal/syntax/**"
  uses diag
}

type FileKind enum "文件的种类，决定按哪一种文件的语法解析" {
  value system
  value component
  value corpus
}

type NotADesignFile error "路径既不是 design/ 下的 .uc 文件，也不是 corpus/ 下的 .uc 文件"

type File object "一个文件的语法树，带注释和各节点的位置；节点的类型是细节" {
  field path: string
  field kind: FileKind
  op print() -> bytes "按规范格式打印"
}

type Parsed record "解析的结果；有诊断时，语法树可能不完整，也可能没有" {
  field file: optional<File>
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "一个合法文件的种类和文本，供性质使用" {
  field kind: FileKind
  field text: string
}

type Broken record "一个合法文件做了一处违反某条规则的改动：种类、文本、改动所在的行和列，以及这处改动应当得到的诊断码" {
  field kind: FileKind
  field text: string
  field line: int
  field column: int
  field code: string
}

op kindOf(path: string) -> FileKind raises NotADesignFile "由仓库中的相对路径判断文件的种类"

op parse(path: string, kind: FileKind, src: bytes) -> Parsed "按 language.md 的词法和语法解析一个文件"

property kind-of-follows-path invariant "design/system.uc 是系统文件；design/ 下其余文件名为 <名称>.uc、名称不为空的整洁路径是组件文件；corpus/ 下同样的路径是录制文件；其余路径都被拒绝，包括只叫 .uc 的文件和不整洁的路径（含 .、.. 或连续的 /）" {
  about kindOf
  forall path: string generated "形如 design/…、corpus/… 和其他目录下的相对路径，扩展名有的是 .uc，有的不是，有的文件名只是 .uc，有的路径不整洁"
}

property parse-is-total invariant "解析任何字节都会返回，不会崩溃；每条诊断的位置都在文件之内，写明修正的办法；诊断按行、列、诊断码排序" {
  about parse
  forall path: string
  forall kind: FileKind
  forall src: bytes
  where path size 1..64
  where src size 0..4096
}

property format-is-stable relation "合法的文本 s 解析后打印得到 t：t 是规范格式，t 解析出的语法树与 s 的相同，t 再解析再打印仍是 t" {
  about parse, File.print
  forall sample: Sample generated "一个合法的文件：随机的语法树，以随意而合法的空白和空行写成文本"
}

property rejects-broken-text invariant "合法的文件做一处违反某条规则的改动之后，解析在改动所在的行和列给出这条规则的诊断，并且不给出语法树" {
  about parse
  forall broken: Broken generated "一个随机的合法文件，在随机的一处做了违反 language.md 某条规则的改动，附上改动所在的行和应当得到的诊断码"
}

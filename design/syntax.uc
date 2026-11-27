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

type Expr object "表达式语法树；节点的内部结构由 syntax 管理，供载入与后端生成使用"

type ExprParsed record "单条表达式的解析结果与诊断" {
  field expr: optional<Expr>
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

op parse(path: string, kind: FileKind, src: bytes) -> Parsed pure "按 language.md 的词法和语法解析一个文件，包括黑盒契约与具名观察"

op parseExpr(path: string, text: string) -> ExprParsed pure "解析单条表达式，位置从给定路径的首行首列起"

op exprText(expr: Expr) -> string pure "表达式的规范文本，保留源中的引用拼写，避免引入符号遮蔽"

flow parse-entry-describes-outcome "语法解析入口返回文件或定位诊断，成功时保留输入路径与种类" {
  forall path: string
  forall kind: FileKind
  forall src: bytes
  where path size 1..64
  where src size 0..512
  user -> parse(path, kind, src)
  ensure size(result.diagnostics) == 0 implies result.file != none and result.file.path == path and result.file.kind == kind
  ensure all(d in result.diagnostics: d.position.path == path and d.position.line >= 1 and d.position.column >= 1)
  case empty-component {
    path = "design/example.uc"
    kind = component
    src = ""
  }
}

property kind-of-follows-path invariant "design/system.uc 是系统文件；design/ 下其余文件名为 <名称>.uc、名称不为空的整洁路径是组件文件；corpus/ 下同样的路径是录制文件；其余路径都被拒绝，包括只叫 .uc 的文件和不整洁的路径（含 .、.. 或连续的 /）" {
  about kindOf
  forall path: string generated "形如 design/…、corpus/… 和其他目录下的相对路径，扩展名有的是 .uc，有的不是，有的文件名只是 .uc，有的路径不整洁"
  manual "需要比较完整语法树、路径分段或诊断的源位置与排序；这些结构不由表达式语言直接提供"
  case system-file {
    path = "design/system.uc"
  }
  case nested-component {
    path = "design/order/pricing.uc"
  }
  case recorded-input {
    path = "corpus/order/total-is-sum.uc"
  }
  case outside-design {
    path = "src/order.go"
  }
  case missing-name {
    path = "design/.uc"
  }
  case unclean-path {
    path = "design/order/../pricing.uc"
  }
}

property parse-is-total invariant "解析任何字节都会返回，不会崩溃；每条诊断的位置都在文件之内，写明修正的办法；诊断按行、列、诊断码排序" {
  about parse
  forall path: string generated "沿用路径生成器，长度一至六十四，保留原测试的字符与边界分布"
  forall kind: FileKind generated "从三种文件种类中取值"
  forall src: bytes generated "沿用不超过 4096 字节的原始字节、片段拼接、合法文件编辑和截断混合生成器"
  manual "需要比较完整语法树、路径分段或诊断的源位置与排序；这些结构不由表达式语言直接提供"
}

property format-is-stable relation "合法的文本 s 解析后打印得到 t：t 是规范格式，t 解析出的语法树与 s 的相同，t 再解析再打印仍是 t" {
  about parse, File.print
  forall sample: Sample generated "一个合法的文件：随机的语法树，以随意而合法的空白和空行写成文本"
  manual "需要比较完整语法树、路径分段或诊断的源位置与排序；这些结构不由表达式语言直接提供"
}

property rejects-broken-text invariant "合法的文件做一处违反某条规则的改动之后，解析在改动所在的行和列给出这条规则的诊断，并且不给出语法树" {
  about parse
  forall broken: Broken generated "一个随机的合法文件，在随机的一处做了违反 language.md 某条规则的改动，附上改动所在的行和应当得到的诊断码"
  manual "需要比较完整语法树、路径分段或诊断的源位置与排序；这些结构不由表达式语言直接提供"
}

property expression-format-is-stable relation "深度三的表达式在复合项均有括号时解析、规范打印并再次解析，语法树与规范文本均保持稳定" {
  about parse, File.print
  forall sample: Sample generated "独立的表达式输入域，覆盖运算符、量词、old 与错误形式，保留每轮完整的测试次数"
  manual "需要比较忽略位置的完整语法树、布局和规范文本，沿用格式性质的独立判定"
}

property contract-syntax-rejects-invalid-claims invariant "契约在陈述、调用、前提、观察、变化范围或关键字上有一处非法写法时，在正确位置拒绝" {
  about parse
  forall broken: Broken generated "沿用非法契约的十一个变体及其预期诊断和位置"
  manual "需要构造非法源文本并比较诊断的位置与代码，沿用语法错误性质的独立判定"
}

property observation-syntax-rejects-malformed-paths invariant "直接观察路径的名称、大小写或分段不合文法时，在正确位置拒绝" {
  about parse
  forall broken: Broken generated "一至四条观察路径，每条一至五段，保留非法名称变体和预期位置"
  manual "需要构造非法源文本并比较诊断的位置与代码，沿用语法错误性质的独立判定"
}

property parse-result-is-described postcondition "解析成功时返回路径和种类正确的文件；每条诊断有相同路径、正的位置以及原因和修正" {
  about parse
  forall path: string
  forall kind: FileKind
  forall src: bytes
  where path size 1..64
  where src size 0..4096
  call parse(path, kind, src)
  ensure size(result.diagnostics) == 0 implies result.file != none and result.file.path == path and result.file.kind == kind
  ensure all(d in result.diagnostics: d.position.path == path and d.position.line >= 1 and d.position.column >= 1 and size(d.message) > 0 and size(d.fix) > 0)
  case empty-component {
    path = "design/sample.uc"
    kind = component
    src = ""
  }
}

property expression-text-is-stable postcondition "表达式解析成功后，规范文本再次解析和打印得到相同文本；错误时有定位诊断" {
  about parseExpr, exprText
  forall path: string
  forall text: string
  where path size 1..64
  where text size 0..1024
  call parseExpr(path, text)
  ensure size(result.diagnostics) == 0 implies result.expr != none and exprText(parseExpr(path, exprText(result.expr)).expr) == exprText(result.expr)
  ensure all(d in result.diagnostics: d.position.path == path and d.position.line >= 1 and d.position.column >= 1 and size(d.message) > 0 and size(d.fix) > 0)
  case precedence {
    path = "expression.uc"
    text = "1 + 2 * 3 == 7"
  }
  case short-circuit {
    path = "expression.uc"
    text = "false implies 1 / 0 == 0"
  }
  case quantifier {
    path = "expression.uc"
    text = "all(x in items: x.quantity > 0)"
  }
}

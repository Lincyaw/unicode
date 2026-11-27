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

type NotADesignFile error "路径按 language.md §5.1 的分类不是本语言的源文件"

type File object "一个设计文件或录制文件的语法树：路径、种类，以及带位置的语法成分和注释；语法成分的类型不在设计中声明" {
  field path: string
  field kind: FileKind
  op print() -> bytes "按规范格式打印"
}

type Parsed record "解析的结果：没有诊断时有语法树，有诊断时只有诊断" {
  field file: optional<File>
  field diagnostics: list<diag.Diagnostic>
}

type Expr object "一条表达式的语法树；内部结构不在设计中声明，供 design 载入和 Go 后端生成代码时读取"

type ExprParsed record "单条表达式的解析结果：没有诊断时有语法树，有诊断时只有诊断" {
  field expr: optional<Expr>
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "format-is-stable 和 expression-format-is-stable 的输入：一个合法的设计文件或录制文件的种类和文本" {
  field kind: FileKind
  field text: string
}

type Broken record "syntax 中三条拒绝非法文本的性质的输入：做了一处违规改动的合法文件的种类和文本，改动所在的行和列，以及应当得到的诊断码" {
  field kind: FileKind
  field text: string
  field line: int
  field column: int
  field code: string
}

op kindOf(path: string) -> FileKind raises NotADesignFile "由仓库中的相对路径判断文件的种类"

op parse(path: string, kind: FileKind, src: bytes) -> Parsed pure "以 kind 的文法，按 language.md 的词法、语法和静态规则解析 path 处的源文本 src，不合规则时只给出诊断"

op parseExpr(path: string, text: string) -> ExprParsed pure "解析单条表达式 text，诊断的位置记在 path 上，表达式的第一个字符是第 1 行第 1 列"

op exprText(expr: Expr) -> string pure "表达式的规范文本，其中的引用保持源文本的写法，不改写成绝对标识，以免与输入符号同名而改变含义"

flow parse-entry-describes-outcome "用户直接调用 parse：没有诊断时语法树的路径和种类与输入相同，每条诊断都在输入路径上、行列不小于 1；与性质 parse-result-is-described 不同，本流程不要求原因和修正不为空" {
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

property kind-of-follows-path invariant "kindOf 按 language.md §5.1 的分类由路径给出文件的种类，不是本语言源文件的路径得到 NotADesignFile" {
  about kindOf
  forall path: string generated "路径：约四分之一取自固定的边界情形（不在 design/ 下或嵌套的 system.uc、只有目录、只叫 .uc 的文件、扩展名大小写不同或相近、含 .、.. 或空段、以 / 开头或结尾），其余由 design、corpus 或其他首段和零至两段目录拼成，约六分之五再接带各种扩展名的文件名，其中约四分之一另插入 .、.. 或空段"
  manual "需要按 / 切分路径并检查每一段，表达式没有字符串切分"
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

property parse-is-total invariant "解析任何字节都正常返回而不 panic：没有诊断时给出路径和种类与输入相同的语法树，有诊断时每条的诊断码形如 syntax.<名称>、原因和修正不为空、位置在输入路径的文件之内，并且诊断按行、列和诊断码排序" {
  about parse
  forall path: string generated "1 至 64 个任意字符、不超过 64 字节的路径"
  forall kind: FileKind generated "三种文件种类之一"
  forall src: bytes generated "不超过 4096 字节的输入，来自四种来源之一：任意字节，语法片段（含制表符、CR、NUL 和不完整的 UTF-8）的拼接，对合法文件做一至四处编辑（复制、删除、交换或插入行，或替换几个字节），或合法文件的截断"
  manual "需要按换行切分源字节，得出行数和每行的长度来判断诊断的位置在文件之内，表达式没有字节切分"
}

property format-is-stable relation "合法的文本 s 解析后打印得到 t：t 是规范格式，t 解析出的语法树与 s 的相同，t 再解析再打印仍是 t" {
  about parse, File.print
  forall sample: Sample generated "一个合法的文件：约三分之一是 expression-format-is-stable 的含随机表达式的组件文件，其余是随机种类、带随机注释的随机语法树，写成文本时随意取缩进、记号间和行尾的空格、空行、零至两个结尾换行、空的可选块是否写花括号、值是否在项之间换行和带结尾逗号，数的写法可带多余的前导零、小数末尾的零或写成 -0"
  manual "需要比较忽略位置的两棵语法树，表达式不能构造或比较语法树"
}

property rejects-broken-text postcondition "合法的文件做一处违反某条规则的改动之后，解析在改动所在的行和列给出这条规则的诊断，并且不给出语法树" {
  about parse
  forall broken: Broken generated "一个随机的合法文件，在随机的一处做了违反 language.md 某条规则的改动，附改动所在的行、列和应当得到的诊断码；约三分之一是 contract-syntax-rejects-invalid-claims 的改动"
  forall path: string generated "与 broken.kind 相符的一条固定路径"
  forall src: bytes generated "broken.text 的 UTF-8 字节"
  call parse(path, broken.kind, src)
  ensure result.file == none
  ensure any(d in result.diagnostics: d.code == broken.code and d.position.line == broken.line and d.position.column == broken.column)
}

property expression-format-is-stable relation "ensure 是随机表达式的组件文件解析后打印得到 t：t 是规范格式，t 解析出的语法树与原文件的相同，t 再解析再打印仍是 t；与 expression-text-is-stable 不同，表达式经整份文件解析，不经 parseExpr" {
  about parse, File.print
  forall sample: Sample generated "一份固定的组件文件，其中一条性质的 ensure 是至多四层嵌套的随机表达式，用到全部二元运算符、not、字段和下标访问、两个实参的调用、量化式、old、raised 和 failure，每个复合子表达式都带括号，表达式不必类型正确"
  manual "需要比较忽略位置的两棵语法树，表达式不能构造或比较语法树；谓词复用 format-is-stable 的手写谓词"
  case container-named-in {
    sample = {
      kind: component,
      text: "component sample \"Expression fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty expression invariant \"Expression normalization\" {\n  about compute\n  ensure raised(in.A)\n}\n",
    }
  }
}

property contract-syntax-rejects-invalid-claims postcondition "黑盒契约或流程有一处违反 language.md 的 G0、G13、G15 至 G18 或 L6 时，解析不给出语法树，并在出错处的行列给出所违反规则的诊断" {
  about parse
  forall broken: Broken generated "一条固定的黑盒契约，性质名随机，做十一种改动之一：删去 ensure 或 call，加上 manual、changes none、不是时长的 within 或带运算的 changes，符号改名为 size，== 两边去掉空格，改成以 alt 开头或没有步骤的流程，或改用 observation-syntax-rejects-malformed-paths 的一处改动；附应得的诊断码和行列"
  forall path: string generated "与 broken.kind 相符的一条固定路径"
  forall src: bytes generated "broken.text 的 UTF-8 字节"
  call parse(path, broken.kind, src)
  ensure result.file == none
  ensure any(d in result.diagnostics: d.code == broken.code and d.position.line == broken.line and d.position.column == broken.column)
}

property observation-syntax-rejects-malformed-paths postcondition "直接观察路径中有一段名称写成大驼峰或带短横线时，解析不给出语法树，并在这一段的行列给出 syntax.invalid-name" {
  about parse
  forall broken: Broken generated "一份固定的组件文件，其中黑盒契约的 observe 列出一至四条直接观察路径，每条一至五段小驼峰名称，其中一段换成大驼峰名称或带短横线的名称，附这一段的行列和诊断码 syntax.invalid-name"
  forall path: string generated "与 broken.kind 相符的一条固定路径"
  forall src: bytes generated "broken.text 的 UTF-8 字节"
  call parse(path, broken.kind, src)
  ensure result.file == none
  ensure any(d in result.diagnostics: d.code == broken.code and d.position.line == broken.line and d.position.column == broken.column)
}

property parse-result-is-described postcondition "parse 的结果合乎输入：没有诊断时语法树的路径和种类与输入相同，每条诊断都在输入路径上、行列不小于 1、原因和修正都不为空" {
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

property expression-text-is-stable postcondition "parseExpr 解析成功时，exprText 的结果再经 parseExpr 和 exprText 得到同一段文本，失败时每条诊断都在输入路径上、行列不小于 1、原因和修正都不为空" {
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

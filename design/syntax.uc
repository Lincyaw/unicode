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
  op print() -> bytes pure "按规范格式打印"
}

type Source record "一个源文件：仓库中的相对路径、按路径得到的种类和原始字节" {
  field path: string
  field kind: FileKind
  field text: bytes
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

type KindSample record "kind-of-follows-path 的输入：一个路径，以及按 language.md §5.1 的分类它应得的种类；kind 为 none 表示这个路径不是本语言的源文件" {
  field path: string
  field kind: optional<FileKind>
}

type RawText record "parse-is-total 的输入：一段源字节 src，以及 src 按换行切成的各行的字节数 widths，键是从 1 起的行号" {
  field src: bytes
  field widths: map<int, int>
}

type Sample record "format-is-stable、format-ignores-layout、format-keeps-tree 和 format-follows-layout-rules 的输入：一个合法的设计文件或录制文件的种类和路径，以及同一棵语法树的两份随意排版的文本" {
  field kind: FileKind
  field path: string
  field text: bytes
  field relaid: bytes
}

type Broken record "syntax 中三条拒绝非法文本的性质的输入：做了一处违规改动的合法文件的种类、路径和字节，改动所在的行和列，以及应当得到的诊断码" {
  field kind: FileKind
  field path: string
  field text: bytes
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

property kind-of-follows-path postcondition "kindOf 对 sample.kind 不为 none 的路径给出 sample.kind，对 sample.kind 为 none 的路径抛出 NotADesignFile" {
  about kindOf
  forall sample: KindSample generated "等概率取九类之一再按这一类构造路径，三类合法的是系统文件 design/system.uc，以及 design/ 或 corpus/ 下零至两层目录中的组件文件或录制文件，文件名是 .、..、system 或其他不为空的名称接 .uc，目录名可以含 .，kind 是这一类的种类；六类不合法的 kind 为 none，分别是首段不是 design 或 corpus 的 .uc 路径或只有一段的 .uc 文件名、扩展名是 .UC、.ucx、.txt 等而不是 .uc、没有扩展名或名称以不带点的 uc、-uc、_uc 结尾的文件、文件名只是 .uc 的文件、只有 design 或 corpus 和零至两层目录而没有文件的路径、在合法的路径中某处插入一段 . 或 ..，以及在合法的路径中某处插入一段空的段，即开头、中间或结尾多一个 /"
  call kindOf(sample.path)
  ensure sample.kind == none implies raised(NotADesignFile)
  ensure sample.kind != none implies not raised(NotADesignFile) and result == sample.kind
  case system-file {
    sample = {path: "design/system.uc", kind: system}
  }
  case nested-component {
    sample = {path: "design/order/pricing.uc", kind: component}
  }
  case nested-system-name {
    sample = {path: "design/order/system.uc", kind: component}
  }
  case dotted-name {
    sample = {path: "design/..uc", kind: component}
  }
  case recorded-input {
    sample = {path: "corpus/order/total-is-sum.uc", kind: corpus}
  }
  case outside-design {
    sample = {path: "src/order.go", kind: none}
  }
  case missing-name {
    sample = {path: "design/.uc", kind: none}
  }
  case unclean-path {
    sample = {path: "design/order/../pricing.uc", kind: none}
  }
  case trailing-slash {
    sample = {path: "design/order.uc/", kind: none}
  }
}

property parse-is-total postcondition "解析任何字节都正常返回：没有诊断时语法树的路径和种类与输入相同，每条诊断都在输入路径上、原因和修正不为空，行是 raw.src 的一行，列在 1 与这一行的字节数加一之间；与 parse-result-is-described 不同，位置还不超出 raw.src 的末行和所在行的末尾" {
  about parse
  forall path: string generated "1 至 64 个任意字符、不超过 64 字节的路径"
  forall kind: FileKind generated "三种文件种类之一"
  forall raw: RawText generated "src 不超过 4096 字节，来自四种来源之一：任意字节，语法片段（含制表符、CR、NUL 和不完整的 UTF-8）的拼接，对 format-is-stable 取的合法文件的文本做一至四处编辑（复制、删除、交换或插入行，或替换几个字节），或这样的文本的截断；widths 是 src 按换行切成的各行的字节数"
  call parse(path, kind, raw.src)
  ensure size(result.diagnostics) == 0 implies result.file != none and result.file.path == path and result.file.kind == kind
  ensure all(d in result.diagnostics: d.position.path == path and size(d.message) > 0 and size(d.fix) > 0)
  ensure all(d in result.diagnostics: d.position.line in raw.widths and d.position.column >= 1 and d.position.column <= raw.widths[d.position.line] + 1)
}

property format-is-stable postcondition "合法的文本 sample.text 解析没有诊断，打印得到的文本解析也没有诊断，再打印仍是同一段文本" {
  about parse, File.print
  forall sample: Sample generated "一个合法的文件，路径与种类相符（系统文件为 design/system.uc，其余为 design/ 或 corpus/ 下零至两层目录中的随机名称）：约三分之一是一份组件文件，其中一条性质的 ensure 是 expression-format-is-stable 取的随机表达式，relaid 在这条表达式外再加一层括号；其余是随机种类、带随机注释的随机语法树，text 和 relaid 各自随意取缩进、记号间和行尾的空格、空行、零至两个结尾换行、空的可选块是否写花括号、值是否在项之间换行和带结尾逗号，数的写法可带多余的前导零、小数末尾的零或写成 -0"
  call parse(sample.path, sample.kind, sample.text)
  ensure size(result.diagnostics) == 0
  ensure size(parse(sample.path, sample.kind, result.file.print()).diagnostics) == 0
  ensure parse(sample.path, sample.kind, result.file.print()).file.print() == result.file.print()
  case container-named-in {
    sample = {
      kind: component,
      path: "design/sample.uc",
      text: "component sample \"Expression fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty expression invariant \"Expression normalization\" {\n  about compute\n  ensure raised(in.A)\n}\n",
      relaid: "component sample \"Expression fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty expression invariant \"Expression normalization\" {\n  about compute\n  ensure (raised(in.A))\n}\n",
    }
  }
  case container-named-in-call {
    sample = {
      kind: component,
      path: "design/sample.uc",
      text: "component sample \"Expression fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty expression invariant \"Expression normalization\" {\n  about compute\n  ensure size(in.items) == 1\n}\n",
      relaid: "component sample \"Expression fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty expression invariant \"Expression normalization\" {\n  about compute\n  ensure (size(in.items) == 1)\n}\n",
    }
  }
}

property format-ignores-layout postcondition "同一棵语法树的两份随意排版的文本 sample.text 和 sample.relaid 解析后打印得到同样的字节" {
  about parse, File.print
  forall sample: Sample generated "一个合法的文件，路径与种类相符（系统文件为 design/system.uc，其余为 design/ 或 corpus/ 下零至两层目录中的随机名称）：约三分之一是一份组件文件，其中一条性质的 ensure 是 expression-format-is-stable 取的随机表达式，relaid 在这条表达式外再加一层括号；其余是随机种类、带随机注释的随机语法树，text 和 relaid 各自随意取缩进、记号间和行尾的空格、空行、零至两个结尾换行、空的可选块是否写花括号、值是否在项之间换行和带结尾逗号，数的写法可带多余的前导零、小数末尾的零或写成 -0"
  call parse(sample.path, sample.kind, sample.text)
  ensure result.file.print() == parse(sample.path, sample.kind, sample.relaid).file.print()
}

property format-keeps-tree invariant "合法的文本 sample.text 解析后打印得到的文本再解析，得到的语法树除位置之外与 sample.text 的相同，表达式按去掉多余括号之后的结构比较" {
  about parse, File.print
  forall sample: Sample generated "一个合法的文件，路径与种类相符（系统文件为 design/system.uc，其余为 design/ 或 corpus/ 下零至两层目录中的随机名称）：约三分之一是一份组件文件，其中一条性质的 ensure 是 expression-format-is-stable 取的随机表达式，relaid 在这条表达式外再加一层括号；其余是随机种类、带随机注释的随机语法树，text 和 relaid 各自随意取缩进、记号间和行尾的空格、空行、零至两个结尾换行、空的可选块是否写花括号、值是否在项之间换行和带结尾逗号，数的写法可带多余的前导零、小数末尾的零或写成 -0"
  manual "需要比较两棵语法树在位置之外是否相同，语法树是不透明的对象，表达式不能比较它们的结构"
}

property format-follows-layout-rules invariant "合法的文本 sample.text 解析后打印得到的文本合乎 language.md 的 P1 至 P6：没有制表符、CR 和行尾空格，恰以一个换行结尾，每层块或值缩进两个空格，块内不空行，顶层的项之间恰空一行，注释紧贴它之后的项，记号之间的空格合乎 P3，数是规范写法，值按 P6 在 100 列之内写成一行或逐项断开" {
  about parse, File.print
  forall sample: Sample generated "一个合法的文件，路径与种类相符（系统文件为 design/system.uc，其余为 design/ 或 corpus/ 下零至两层目录中的随机名称）：约三分之一是一份组件文件，其中一条性质的 ensure 是 expression-format-is-stable 取的随机表达式，relaid 在这条表达式外再加一层括号；其余是随机种类、带随机注释的随机语法树，text 和 relaid 各自随意取缩进、记号间和行尾的空格、空行、零至两个结尾换行、空的可选块是否写花括号、值是否在项之间换行和带结尾逗号，数的写法可带多余的前导零、小数末尾的零或写成 -0"
  manual "需要把打印出的字节切成行和记号来判断版面，表达式没有字节和字符串的切分"
}

property rejects-broken-text postcondition "合法的文件做一处违反某条规则的改动之后，解析不给出语法树，并在改动所在的文件、行和列给出这条规则的诊断" {
  about parse
  forall broken: Broken generated "一个随机的合法文件，在随机的一处做了违反 language.md 某条规则的改动，附与种类相符的随机路径、改动所在的行和列以及应当得到的诊断码；约三分之一是 contract-syntax-rejects-invalid-claims 的改动"
  call parse(broken.path, broken.kind, broken.text)
  ensure result.file == none
  ensure any(d in result.diagnostics: d.code == broken.code and d.position.path == broken.path and d.position.line == broken.line and d.position.column == broken.column)
}

property expression-format-is-stable postcondition "复合子表达式都带括号的合法表达式 text 经 parseExpr 没有诊断，exprText 的结果再经 parseExpr 也没有诊断，再经 exprText 仍是同一段文本；与 expression-text-is-stable 不同，text 都是合法的表达式，并且要求没有诊断" {
  about parseExpr, exprText
  forall text: string generated "一棵至多四层的随机表达式树写成的文本，每个复合子表达式都带括号：最后一层只有原子，即数、字符串、名称，或 true、false、none、result 之一，四类各约四分之一，之上每一层约五分之一是这样的原子，其余是复合的，其中约三分之一是二元运算（十四种运算符各约十四分之一），not、字段访问、下标访问、两个实参的调用、量化式（all、any、count 或 sum）各约九分之一，余下约九分之一中 old 与 raised 或 failure 各半；表达式不必类型正确"
  call parseExpr("expression.uc", text)
  ensure size(result.diagnostics) == 0
  ensure size(parseExpr("expression.uc", exprText(result.expr)).diagnostics) == 0
  ensure exprText(parseExpr("expression.uc", exprText(result.expr)).expr) == exprText(result.expr)
  case container-named-in {
    text = "raised(in.A)"
  }
  case redundant-brackets {
    text = "((a + b) + (c * d))"
  }
}

property contract-syntax-rejects-invalid-claims postcondition "黑盒契约或流程有一处违反 language.md 的 G0、G13、G15 至 G18 或 L6 时，解析不给出语法树，并在出错处的文件、行和列给出所违反规则的诊断" {
  about parse
  forall broken: Broken generated "一个只含一条黑盒契约的组件文件：组件、性质、about 和 call 中的类型与操作的名称随机，一至三个内置类型的符号，第一个传给被测调用，一至三条 ensure 各比较 result、符号或零至 999 的数中的两个；再等概率做十一种改动之一：删去全部 ensure 或 call，加上 manual 或 changes none 或不是时长的 within，把一个符号改名为表达式的关键字，一条 ensure 的比较运算符两边去掉空格，加上带运算的 changes，改成以 alt 开头或没有步骤的流程，或改用 observation-syntax-rejects-malformed-paths 的一处改动，并附 design/ 下的随机路径、应得的诊断码和行列"
  call parse(broken.path, broken.kind, broken.text)
  ensure result.file == none
  ensure any(d in result.diagnostics: d.code == broken.code and d.position.path == broken.path and d.position.line == broken.line and d.position.column == broken.column)
}

property observation-syntax-rejects-malformed-paths postcondition "直接观察路径中有一段名称写成大驼峰或带短横线时，解析不给出语法树，并在这一段的文件、行和列给出 syntax.invalid-name" {
  about parse
  forall broken: Broken generated "一份组件文件，其中名称随机的黑盒契约的 observe 列出一至四条直接观察路径，每条一至五段小驼峰名称，其中一段换成大驼峰名称或带短横线的名称，附 design/ 下的随机路径、这一段的行列和诊断码 syntax.invalid-name"
  call parse(broken.path, broken.kind, broken.text)
  ensure result.file == none
  ensure any(d in result.diagnostics: d.code == broken.code and d.position.path == broken.path and d.position.line == broken.line and d.position.column == broken.column)
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

component design "把语法树解析成设计层的节点和边；检查设计内部、流程、符号与用例的约束" {
  focus core
  paths "internal/design/**"
  uses syntax, graph, diag
}

type Loaded record "载入的结果：设计层的图和诊断；有诊断时，图可能不完整" {
  field graph: graph.Graph
  field diagnostics: list<diag.Diagnostic>
}

type Sample record "一组合法的设计文件和录制文件，以及它们声明的节点和边，供性质使用" {
  field files: map<string, string>
  field nodes: list<string>
  field edges: list<string>
  field attrs: map<string, map<string, list<string>>>
}

type Broken record "一组合法的文件做了一处违反某条约束的改动：文件、改动所在的路径和行，以及应当得到的诊断码" {
  field files: map<string, string>
  field path: string
  field line: int
  field code: string
}

op load(files: list<syntax.File>) -> Loaded "解析引用，建出设计层的节点和边，检查只需设计就能判断的约束"

property accepts-valid-design invariant "合法的设计载入之后没有诊断，图中恰有它声明的节点和边，标识按 model.md §7 写出；attrs 中列出的节点，属性恰是列出的那些" {
  about load
  forall sample: Sample generated "一组随机的合法文件：系统文件、若干层组件文件和录制文件，引用都能解析，满足只需设计就能判断的全部约束；节点和边写成标识，边写作 种类 起点 终点"
}

property rejects-broken-design invariant "合法的设计做一处违反某条约束的改动之后，载入在改动所在的文件和行给出这条约束的诊断" {
  about load
  forall broken: Broken generated "一组随机的合法文件，在随机的一处做了违反 model.md §10 或 format.md 中某条只需设计就能判断的规则的改动"
}

property load-ignores-file-order relation "文件以任何次序给出，载入得到的快照和诊断都相同" {
  about load
  forall files: map<string, string> generated "一组随机的文件，有的合法，有的带违反约束的改动"
  forall rotation: int
  where rotation in 0..1000
}

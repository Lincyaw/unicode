component gen "对每种已配置的语言调用有生成能力的后端，合并成一份生成计划，检查路径和生成标记后逐个文件原子写入" {
  focus core
  paths "internal/gen/**"
  uses graph, lang, diag, repo
}

type WriteSample record "write 和 checkFiles 各性质的输入：一棵目录树（repo 下是仓库，outside 下是仓库之外的文件），一份生成计划，计划中各路径在树中的路径，以及按构造定下的这份计划是否安全" {
  field tree: repo.Tree
  field edits: list<lang.Edit>
  field keys: map<string, string>
  field safe: bool
}

type PlanSample record "plan 各性质的输入：一张图，go、python 和 unused 各自的后端替身预先给出的生成计划，以及其中已配置的语言的生成计划" {
  field graph: graph.Graph
  field generations: map<string, lang.Generation>
  field configured: list<lang.Generation>
}

type Received record "plan-dispatches-configured-languages 中后端替身收到的一次生成输入：仓库根目录、源码根目录和图编码出的快照" {
  field root: string
  field roots: list<string>
  field snapshot: bytes
}

op plan(root: string, g: graph.Graph, backends: list<lang.Backend>) -> lang.Generation "对 g 的系统文件中配置的每种语言，调用 backends 中有生成能力的后端，用 merge 合并它们的生成计划和后端重复、缺少生成能力的诊断；缺少实现的代码或谓词不妨碍生成"

op merge(generations: list<lang.Generation>) -> lang.Generation pure "把几份生成计划合并成一份，结果与它们的次序无关，同一路径每多一个文件另报一条 gen.duplicate-path"

op write(root: string, edits: list<lang.Edit>) -> list<diag.Diagnostic> "先检查 edits 的全部路径、重复目标、符号链接和生成标记，再逐个文件原子写入：只改写带生成标记的文件，createOnly 的文件只在不存在时创建，多个文件之间不保证同时成功"

op checkFiles(root: string, edits: list<lang.Edit>) -> list<diag.Diagnostic> "只读地检查生成计划的路径和生成标记，逐字节核对全部文件，缺失或内容不同时给出诊断，不跟随符号链接，也不创建或改写文件"

property write-applies-safe-plans postcondition "生成计划安全时，write 没有诊断，每个目标都是普通文件，内容是计划中的" {
  about write
  forall sample: WriteSample generated "一棵目录树：repo 下是仓库，有一个带生成标记的 owned.go 和一个不带的 user.go（权限位都是 640），outside 下是仓库之外的 user.go，生成计划新建 repo 中一至八个小写字母的目录下的 new.go，内容是生成标记和任意文本，再按十三种情形之一改动，三种安全（只新建，另改写 owned.go，新建的文件为 createOnly），十种不安全（改写 user.go、越出仓库到 ../outside/user.go、同一目标两次、目标是指向 user.go 的符号链接、经过指向 ../outside 的目录链接、路径含 . 段、目标是另一目标的上一级目录、空路径、对 owned.go 或 user.go 用 createOnly）；keys 给计划中的每个路径前加 repo/，safe 是按情形定下的是否安全"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录"
  forall root: string generated "dir 下的 repo 目录"
  observe files: repo.Tree from dir
  changes files
  call write(root, sample.edits)
  ensure sample.safe implies size(result) == 0 and all(e in sample.edits: sample.keys[e.path] in files.entries and files.entries[sample.keys[e.path]].kind == repo.EntryKind.file and files.entries[sample.keys[e.path]].content == e.content)
}

property write-rejects-unsafe-plans postcondition "生成计划不安全时，write 至少给出一条诊断，仓库内外的条目都不变" {
  about write
  forall sample: WriteSample generated "一棵目录树：repo 下是仓库，有一个带生成标记的 owned.go 和一个不带的 user.go（权限位都是 640），outside 下是仓库之外的 user.go，生成计划新建 repo 中一至八个小写字母的目录下的 new.go，内容是生成标记和任意文本，再按十三种情形之一改动，三种安全（只新建，另改写 owned.go，新建的文件为 createOnly），十种不安全（改写 user.go、越出仓库到 ../outside/user.go、同一目标两次、目标是指向 user.go 的符号链接、经过指向 ../outside 的目录链接、路径含 . 段、目标是另一目标的上一级目录、空路径、对 owned.go 或 user.go 用 createOnly）；keys 给计划中的每个路径前加 repo/，safe 是按情形定下的是否安全"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录"
  forall root: string generated "dir 下的 repo 目录"
  observe files: repo.Tree from dir
  changes files
  call write(root, sample.edits)
  ensure not sample.safe implies size(result) > 0 and files == old(files)
}

property write-keeps-other-entries postcondition "write 之后，计划的目标之外原有的条目（仓库之外的也在内）都不变，新出现的条目只有目标和目录" {
  about write
  forall sample: WriteSample generated "一棵目录树：repo 下是仓库，有一个带生成标记的 owned.go 和一个不带的 user.go（权限位都是 640），outside 下是仓库之外的 user.go，生成计划新建 repo 中一至八个小写字母的目录下的 new.go，内容是生成标记和任意文本，再按十三种情形之一改动，三种安全（只新建，另改写 owned.go，新建的文件为 createOnly），十种不安全（改写 user.go、越出仓库到 ../outside/user.go、同一目标两次、目标是指向 user.go 的符号链接、经过指向 ../outside 的目录链接、路径含 . 段、目标是另一目标的上一级目录、空路径、对 owned.go 或 user.go 用 createOnly）；keys 给计划中的每个路径前加 repo/，safe 是按情形定下的是否安全"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录"
  forall root: string generated "dir 下的 repo 目录"
  observe files: repo.Tree from dir
  changes files
  call write(root, sample.edits)
  ensure all(p in old(files).entries: not any(e in sample.edits: sample.keys[e.path] == p) implies p in files.entries and files.entries[p] == old(files).entries[p])
  ensure all(p in files.entries: p in old(files).entries or any(e in sample.edits: sample.keys[e.path] == p) or files.entries[p].kind == repo.EntryKind.dir)
}

property write-keeps-modes postcondition "write 没有诊断时，改写的原有目标保留原来的权限位" {
  about write
  forall sample: WriteSample generated "一棵目录树：repo 下是仓库，有一个带生成标记的 owned.go 和一个不带的 user.go（权限位都是 640），outside 下是仓库之外的 user.go，生成计划新建 repo 中一至八个小写字母的目录下的 new.go，内容是生成标记和任意文本，再按十三种情形之一改动，三种安全（只新建，另改写 owned.go，新建的文件为 createOnly），十种不安全（改写 user.go、越出仓库到 ../outside/user.go、同一目标两次、目标是指向 user.go 的符号链接、经过指向 ../outside 的目录链接、路径含 . 段、目标是另一目标的上一级目录、空路径、对 owned.go 或 user.go 用 createOnly）；keys 给计划中的每个路径前加 repo/，safe 是按情形定下的是否安全"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录"
  forall root: string generated "dir 下的 repo 目录"
  observe files: repo.Tree from dir
  changes files
  call write(root, sample.edits)
  ensure size(result) == 0 implies all(e in sample.edits: sample.keys[e.path] in old(files).entries implies files.entries[sample.keys[e.path]].mode == old(files).entries[sample.keys[e.path]].mode)
}

property write-diagnostics-are-described postcondition "write 给出的每条诊断都有 subject、位置的路径、从 1 起的行和列、原因和修正" {
  about write
  forall sample: WriteSample generated "一棵目录树：repo 下是仓库，有一个带生成标记的 owned.go 和一个不带的 user.go（权限位都是 640），outside 下是仓库之外的 user.go，生成计划新建 repo 中一至八个小写字母的目录下的 new.go，内容是生成标记和任意文本，再按十三种情形之一改动，三种安全（只新建，另改写 owned.go，新建的文件为 createOnly），十种不安全（改写 user.go、越出仓库到 ../outside/user.go、同一目标两次、目标是指向 user.go 的符号链接、经过指向 ../outside 的目录链接、路径含 . 段、目标是另一目标的上一级目录、空路径、对 owned.go 或 user.go 用 createOnly）；keys 给计划中的每个路径前加 repo/，safe 是按情形定下的是否安全"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录"
  forall root: string generated "dir 下的 repo 目录"
  call write(root, sample.edits)
  ensure all(d in result: size(d.subject) > 0 and size(d.position.path) > 0 and d.position.line >= 1 and d.position.column >= 1 and size(d.message) > 0 and size(d.fix) > 0)
}

property write-is-idempotent postcondition "把安全的生成计划再写进已写入它的树，计划中有 createOnly 的文件时 write 给出诊断，否则没有诊断，仓库内外的条目都不变" {
  about write
  forall sample: WriteSample generated "与 write-rejects-unsafe-plans 的 sample 相同，只取三种安全的情形"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录，再用 write 把 sample.edits 写进其中的 repo"
  forall root: string generated "dir 下的 repo 目录"
  observe files: repo.Tree from dir
  changes none
  call write(root, sample.edits)
  ensure any(e in sample.edits: e.createOnly) == (size(result) > 0)
}

property check-files-agrees-with-write postcondition "生成计划不安全时，checkFiles 给出与 write 相同的诊断" {
  about checkFiles, write
  forall sample: WriteSample generated "一棵目录树：repo 下是仓库，有一个带生成标记的 owned.go 和一个不带的 user.go（权限位都是 640），outside 下是仓库之外的 user.go，生成计划新建 repo 中一至八个小写字母的目录下的 new.go，内容是生成标记和任意文本，再按十三种情形之一改动，三种安全（只新建，另改写 owned.go，新建的文件为 createOnly），十种不安全（改写 user.go、越出仓库到 ../outside/user.go、同一目标两次、目标是指向 user.go 的符号链接、经过指向 ../outside 的目录链接、路径含 . 段、目标是另一目标的上一级目录、空路径、对 owned.go 或 user.go 用 createOnly）；keys 给计划中的每个路径前加 repo/，safe 是按情形定下的是否安全"
  forall rejection: list<diag.Diagnostic> generated "write 把 sample.edits 写进 sample.tree 的另一份副本时给出的诊断"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录"
  forall root: string generated "dir 下的 repo 目录"
  call checkFiles(root, sample.edits)
  ensure not sample.safe implies result == rejection
}

property check-files-reports-stale-targets postcondition "生成计划安全、没有已存在的 createOnly 目标时，checkFiles 恰给每个缺失或内容不同的目标报一条 subject 为它的路径的 gen.stale-file，没有别的诊断，仓库内外的条目和修改时间都不变" {
  about checkFiles
  forall sample: WriteSample generated "与 write-rejects-unsafe-plans 的 sample 相同，只取三种安全的情形"
  forall stage: int
  forall which: int
  forall dir: string generated "materialize 把 sample.tree 写成的新目录；stage 为 1 至 3 时再用 write 把 sample.edits 写进其中的 repo，stage 为 2 时另在第 which 除以目标数的余数个目标的内容末尾加一个 !，为 3 时删去这个目标"
  forall root: string generated "dir 下的 repo 目录"
  where stage in 0..3
  where which in 0..1
  observe files: repo.Tree from dir
  observe stamps: map<string, time> from dir
  requires not any(e in sample.edits: e.createOnly and sample.keys[e.path] in files.entries)
  changes none
  call checkFiles(root, sample.edits)
  ensure all(e in sample.edits: (not sample.keys[e.path] in files.entries or files.entries[sample.keys[e.path]].content != e.content) == any(d in result: d.code == "gen.stale-file" and d.subject == e.path))
  ensure all(d in result: d.code == "gen.stale-file" and count(x in result: x.subject == d.subject) == 1)
}

property check-files-rejects-written-create-only postcondition "安全的生成计划写入之后，计划中有 createOnly 的文件时 checkFiles 给出诊断，否则没有诊断，仓库内外的条目和修改时间都不变" {
  about checkFiles
  forall sample: WriteSample generated "与 write-rejects-unsafe-plans 的 sample 相同，只取三种安全的情形"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录，再用 write 把 sample.edits 写进其中的 repo"
  forall root: string generated "dir 下的 repo 目录"
  observe files: repo.Tree from dir
  observe stamps: map<string, time> from dir
  changes none
  call checkFiles(root, sample.edits)
  ensure any(e in sample.edits: e.createOnly) == (size(result) > 0)
}

property check-files-diagnostics-are-described postcondition "checkFiles 给出的每条诊断都有 subject、位置的路径、从 1 起的行和列、原因和修正，仓库内外的条目和修改时间都不变" {
  about checkFiles
  forall sample: WriteSample generated "一棵目录树：repo 下是仓库，有一个带生成标记的 owned.go 和一个不带的 user.go（权限位都是 640），outside 下是仓库之外的 user.go，生成计划新建 repo 中一至八个小写字母的目录下的 new.go，内容是生成标记和任意文本，再按十三种情形之一改动，三种安全（只新建，另改写 owned.go，新建的文件为 createOnly），十种不安全（改写 user.go、越出仓库到 ../outside/user.go、同一目标两次、目标是指向 user.go 的符号链接、经过指向 ../outside 的目录链接、路径含 . 段、目标是另一目标的上一级目录、空路径、对 owned.go 或 user.go 用 createOnly）；keys 给计划中的每个路径前加 repo/，safe 是按情形定下的是否安全"
  forall dir: string generated "materialize 把 sample.tree 写成的新目录"
  forall root: string generated "dir 下的 repo 目录"
  observe files: repo.Tree from dir
  observe stamps: map<string, time> from dir
  changes none
  call checkFiles(root, sample.edits)
  ensure all(d in result: size(d.subject) > 0 and size(d.position.path) > 0 and d.position.line >= 1 and d.position.column >= 1 and size(d.message) > 0 and size(d.fix) > 0)
}

property plan-dispatches-configured-languages postcondition "plan 给每种已配置的语言的后端恰一次生成输入，传入仓库根目录、这种语言的源码根目录和同一张图，不调用未配置的语言的后端，返回 merge 对已配置的语言的生成计划给出的结果，不改动仓库" {
  about plan
  forall sample: PlanSample generated "只有系统节点的图，go 和 python 各有一半机会配置一至三个由一至六个小写字母组成的源码根目录，go、python 和未配置的 unused 各自的生成计划：零至三个文件，路径是语言名下的序号加 .go，内容任意，一半另带一条诊断；configured 是已配置的语言的生成计划"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall backends: list<lang.Backend> generated "go、python 和 unused 各一个后端替身，记下收到的每次生成输入，返回 sample.generations 中这种语言的生成计划"
  observe received: map<string, list<Received>> from backends
  observe files: repo.Tree from dir
  changes received
  call plan(dir, sample.graph, backends)
  ensure all(l in lang.languageRoots(sample.graph): size(received[l]) == 1 and all(p in received[l]: p.root == dir and p.roots == lang.languageRoots(sample.graph)[l] and p.snapshot == sample.graph.encode()))
  ensure all(l in received: not l in lang.languageRoots(sample.graph) implies size(received[l]) == 0)
  ensure result == merge(sample.configured)
}

property plan-rejects-duplicate-backends postcondition "同一语言有两个后端时，plan 给出 gen.duplicate-backend" {
  about plan
  forall sample: PlanSample generated "只有系统节点的图，go 和 python 各有一半机会配置一至三个由一至六个小写字母组成的源码根目录，go、python 和未配置的 unused 各自的生成计划：零至三个文件，路径是语言名下的序号加 .go，内容任意，一半另带一条诊断；configured 是已配置的语言的生成计划"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall backends: list<lang.Backend> generated "与 plan-dispatches-configured-languages 的 backends 相同的三个后端替身，再加一次 go 的"
  call plan(dir, sample.graph, backends)
  ensure any(d in result.diagnostics: d.code == "gen.duplicate-backend")
}

property plan-rejects-backends-without-generation postcondition "已配置的语言的后端都没有生成能力时，plan 给每种已配置的语言一条 gen.unsupported-language" {
  about plan
  forall sample: PlanSample generated "只有系统节点的图，go 和 python 各有一半机会配置一至三个由一至六个小写字母组成的源码根目录，go、python 和未配置的 unused 各自的生成计划：零至三个文件，路径是语言名下的序号加 .go，内容任意，一半另带一条诊断；configured 是已配置的语言的生成计划"
  forall dir: string generated "materialize 把空树写成的新目录"
  forall backends: list<lang.Backend> generated "go 和 python 各一个没有生成能力的后端替身"
  call plan(dir, sample.graph, backends)
  ensure count(d in result.diagnostics: d.code == "gen.unsupported-language") == size(lang.languageRoots(sample.graph))
}

property merge-keeps-every-file postcondition "merge 的文件恰是各份生成计划的文件，个数相同" {
  about merge
  forall generations: list<lang.Generation> generated "一至四份生成计划：各零至三个文件，路径取自 a.go、b.go、c/d.go 三个，内容取自 x、y 两个，createOnly 各一半，约一半另带一条诊断：诊断码取自 gen.stale-file 和 gen.write-failed，位置取自 a.go 和 design/system.uc，原因取自两种"
  call merge(generations)
  ensure size(result.edits) == sum(g in generations: size(g.edits))
  ensure all(g in generations: all(e in g.edits: count(x in result.edits: x == e) == sum(h in generations: count(x in h.edits: x == e))))
}

property merge-reports-duplicate-paths postcondition "merge 的诊断恰是各份生成计划的诊断，个数相同，另加每条路径比它的文件数少一条 subject 为这条路径的 gen.duplicate-path" {
  about merge
  forall generations: list<lang.Generation> generated "一至四份生成计划：各零至三个文件，路径取自 a.go、b.go、c/d.go 三个，内容取自 x、y 两个，createOnly 各一半，约一半另带一条诊断：诊断码取自 gen.stale-file 和 gen.write-failed，位置取自 a.go 和 design/system.uc，原因取自两种"
  call merge(generations)
  ensure all(g in generations: all(d in g.diagnostics: count(x in result.diagnostics: x == d) == sum(h in generations: count(x in h.diagnostics: x == d))))
  ensure all(g in generations: all(e in g.edits: count(d in result.diagnostics: d.code == "gen.duplicate-path" and d.subject == e.path) == sum(h in generations: count(x in h.edits: x.path == e.path)) - 1))
  ensure all(d in result.diagnostics: d.code == "gen.duplicate-path" or any(g in generations: d in g.diagnostics))
}

property merge-ignores-order relation "各份生成计划按 rotation 轮换次序之后，merge 的结果不变" {
  about merge
  forall generations: list<lang.Generation> generated "一至四份生成计划：各零至三个文件，路径取自 a.go、b.go、c/d.go 三个，内容取自 x、y 两个，createOnly 各一半，约一半另带一条诊断：诊断码取自 gen.stale-file 和 gen.write-failed，位置取自 a.go 和 design/system.uc，原因取自两种"
  forall rotation: int
  forall rotated: list<lang.Generation> generated "generations 按 rotation 轮换次序"
  where rotation in 0..1000
  ensure merge(generations) == merge(rotated)
}

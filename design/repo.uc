component repo "仓库：经 git 读出提交、工作区和提交中的文件，以及 build 记录" {
  focus utility
  paths "internal/repo/**"
}

type NoGit error "找不到命令 git"

type BadRevision error "git 读不出这个提交"

type GitFailed error "git 的命令失败，原因不是提交读不出"

type BadRecord error "build 记录不存在、读不出，或者不是 {commit, dirty}"

type Revision record "一个提交：完整的哈希、缩写的哈希、检出在它上面的分支（没有时为空）、提交说明的第一行和提交时间（UTC）" {
  field commit: string
  field short: string
  field branch: string
  field subject: string
  field time: time
}

type WorkingTree record "工作区：它所在的提交，各字段同 Revision，以及其中的文件是否与提交不同" {
  field commit: string
  field short: string
  field branch: string
  field subject: string
  field time: time
  field dirty: bool
}

type Head record "build 记录 build.json：仓库所在的提交（不在 Git 工作区中或还没有提交时为空），以及工作区是否有改动" {
  field commit: optional<string>
  field dirty: bool
}

type EntryKind enum "目录条目的种类：普通文件、目录或符号链接" {
  value file
  value dir
  value link
}

type Entry record "目录树中的一个条目：种类、普通文件的内容、符号链接的目标，以及权限位（链接为 0）" {
  field kind: EntryKind
  field content: bytes
  field target: string
  field mode: int
}

type Tree record "一棵目录树：以 / 分隔的相对路径到条目，目录和链接也是条目，不跟随链接" {
  field entries: map<string, Entry>
}

type BadTree error "树写不出：有条目的路径不是干净的相对路径，有条目的上一级不是树中的目录，或者写入失败"

type Unreadable error "目录或其中的条目读不出，或者有条目不是目录、普通文件或符号链接"

op readRevision(root: string, rev: string) -> Revision raises NoGit, BadRevision "读出 rev 指的提交；它是检出的提交时带上分支"

op log(root: string, rev: string, limit: int) -> list<Revision> raises NoGit, BadRevision "rev 的历史中最近的至多 limit 个提交，新的在前"

op show(root: string, commit: string, path: string) -> bytes raises NoGit, BadRevision "提交中一个文件的内容"

op readWorkingTree(root: string, out: string) -> WorkingTree raises NoGit, BadRevision, GitFailed "工作区所在的提交，以及工作区是否有改动；build 写到目录 out 中的快照和 build 记录不算改动"

op readHead(root: string, out: string) -> Head raises GitFailed "build 要记下的仓库状态：没有 git 或者没有提交时，提交为空，算作有改动"

op readRecord(dir: string) -> Head raises BadRecord "读出 build 写在目录 dir 中的 build 记录"

op linesChanged(before: string, after: string) -> int raises NoGit, GitFailed "两段文本之间的行差别，按 git diff 计：增加的行数加删除的行数"

op materialize(tree: Tree) -> string raises BadTree "新建一个临时目录，按 tree 在其中写出每个目录、普通文件和符号链接及其权限位，返回这个目录的路径"

op readTree(dir: string) -> Tree raises Unreadable "读出 dir 下的每个条目，不跟随符号链接，不改动任何条目"

property read-tree-inverts-materialize postcondition "readTree 读回 materialize 写出的目录，得到写入的 tree" {
  about readTree, materialize
  forall tree: Tree generated "零至十个条目，路径是 a、b、c 中的一至三段以 / 连成，每个条目的上一级是树中的目录；条目约三分之一是目录，权限位取 700、750、755（八进制）之一，约三分之一是普通文件，内容是零至十六个任意字节，权限位取 600、640、644、700、755 之一，其余是符号链接，目标取 a、../x、b/c、/nonexistent 之一，权限位为 0"
  forall dir: string generated "materialize 把 tree 写成的新目录"
  call readTree(dir)
  ensure result == tree
}

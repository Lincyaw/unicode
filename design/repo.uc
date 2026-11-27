component repo "仓库：经 git 读出提交、工作区和提交中的文件，以及 build 写在快照旁边的记录" {
  focus utility
  paths "internal/repo/**"
}

type NoGit error "找不到命令 git"

type BadRevision error "git 读不出这个提交"

type GitFailed error "git 的命令失败，原因不是提交读不出"

type BadRecord error "build 写出的记录不存在、读不出，或者不是 {commit, dirty}"

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

type Head record "build 写在快照旁边的记录 build.json：仓库所在的提交，不在 Git 的工作区中或者还没有提交时没有；工作区是否有改动" {
  field commit: optional<string>
  field dirty: bool
}

op readRevision(root: string, rev: string) -> Revision raises NoGit, BadRevision "读出 rev 指的提交；它是检出的提交时带上分支"

op log(root: string, rev: string, limit: int) -> list<Revision> raises NoGit, BadRevision "rev 的历史中最近的至多 limit 个提交，新的在前"

op show(root: string, commit: string, path: string) -> bytes raises NoGit, BadRevision "提交中一个文件的内容"

op readWorkingTree(root: string, out: string) -> WorkingTree raises NoGit, BadRevision, GitFailed "工作区所在的提交，以及工作区是否有改动；build 写到目录 out 中的快照和记录不算改动"

op readHead(root: string, out: string) -> Head raises GitFailed "build 要记下的仓库状态：没有 git 或者没有提交时，提交为空，算作有改动"

op readRecord(dir: string) -> Head raises BadRecord "读出 build 写在目录 dir 中的记录"

op linesChanged(before: string, after: string) -> int raises NoGit, GitFailed "两段文本之间的行差别，按 git diff 计：增加的行数加删除的行数"

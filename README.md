# unicode

unicode 把一个仓库的设计意图、代码和证据组织成一张图。设计写在 `design/` 下的 `.uc` 文件中；代码的结构由语言后端静态抽取；二者由代码中的绑定对应起来。DDD、UML 类图、时序图和 C4 都是这张图的视图。性质可以声明输入、前提、被测调用、观察范围、允许变化的位置和结果关系；Go 后端把这些黑盒契约生成可执行的谓词和驱动。

当前 Go 工具链提供 M4 的生成与回放、M6 的覆盖引导探索与证据、M7 的内容评审。探索记录真实基本块与流程轨迹，保留增加覆盖的输入并缩减反例；运行结果和评审状态在查看器中显示。Python 的生成与回放仍属于 M5。

本仓库用自己的设计做检查：

```sh
go run ./cmd/unicode gen --dry-run
go run ./cmd/unicode gen
go run ./cmd/unicode gen --check
go run ./cmd/unicode check
go run ./cmd/unicode test syntax.kind-of-follows-path syntax.parse-result-is-described syntax.expression-text-is-stable
go run ./cmd/unicode review list
make check
```

`gen --dry-run` 输出生成计划；`gen` 更新本工具拥有的生成文件；`gen --check` 只读检查生成内容是否陈旧。普通 `go test` 回放用例并完整执行默认 rapid 次数，原生 `FuzzUnicode_…` 入口也由 `gen` 维护。`test` 只回放所选性质的用例，可用 `--explore 1s --record` 探索并录制到 `corpus/`，随后自动更新驱动。无法用当前值语法无损表示的输入会报错，并保留旧录制文件。没有有效执行会报错。

可表达的性质用 `ensure` 自动生成谓词；自定义生成器负责输入分布，复杂环境与独立判定保留有理由的 `manual` 谓词。`review list` 列出待审内容，`review record` 对节点或边记录评审人、结论和注释；目标内容或关联的人工谓词、观察适配器变化后重新待审。详细语义和诊断见下列文档。

| 文档 | 内容 |
| --- | --- |
| [docs/model.md](docs/model.md) | 图模型：统一结构、节点、边、记法、推导和约束 |
| [docs/language.md](docs/language.md) | 设计语言：源文件、词法、语法、语义、编译与代码中的绑定 |
| [docs/design.md](docs/design.md) | 工具链：组件、语言后端、生成、视图、命令行和计划 |
| [docs/decisions.md](docs/decisions.md) | 决定记录 |

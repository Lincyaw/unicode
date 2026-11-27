# unicode

unicode 把一个仓库的设计意图、代码和证据组织成一张图。设计写在 `design/` 下的 `.uc` 文件中；代码的结构由语言后端静态抽取；二者由代码中的绑定对应起来。DDD、UML 类图、时序图和 C4 都是这张图的视图。性质可以声明输入、前提、被测调用、观察范围、允许变化的位置和结果关系；Go 后端把这些黑盒契约生成可执行的谓词和驱动。

当前实现到 M4 第一阶段：设计与代码的一致性检查、图查询和影响分析、单次调用契约、声明用例回放，以及按预算生成输入的探索。复杂性质保留有理由的 `manual` 谓词；调用序列、完整流程驱动、接口替身和覆盖引导录制按后续阶段交付。

本仓库用自己的设计做检查：

```sh
go run ./cmd/unicode gen --dry-run
go run ./cmd/unicode gen
go run ./cmd/unicode gen --check
go run ./cmd/unicode check
go run ./cmd/unicode test syntax.kind-of-follows-path syntax.parse-result-is-described syntax.expression-text-is-stable
make check
```

`gen --dry-run` 输出生成计划；`gen` 更新本工具拥有的生成文件；`gen --check` 只读检查生成内容是否陈旧。普通 `go test` 回放用例并完整执行默认 rapid 次数，原生 `FuzzUnicode_…` 入口也由 `gen` 维护。`test` 只回放所选性质的用例，可用 `--explore 1s` 做限时探索。没有有效执行会报错。手写谓词和自定义生成器保留，重复的测试入口由生成驱动接替。详细语义和诊断见下列文档。

| 文档 | 内容 |
| --- | --- |
| [docs/model.md](docs/model.md) | 图模型：统一结构、节点、边、记法、推导和约束 |
| [docs/language.md](docs/language.md) | 设计语言：源文件、词法、语法、语义、编译与代码中的绑定 |
| [docs/design.md](docs/design.md) | 工具链：组件、语言后端、生成、视图、命令行和计划 |
| [docs/decisions.md](docs/decisions.md) | 决定记录 |

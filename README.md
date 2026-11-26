# unicode

unicode 把一个仓库的设计意图、代码和证据组织成一张图。设计写在 `design/` 下的 `.uc` 文件中；代码的结构由语言后端静态抽取；二者由代码中的绑定对应起来。DDD、UML 类图、时序图和 C4 都是这张图的视图。性质必须能执行：性质可以声明符号和用例，生成器和测试驱动由工具链生成。

当前处于 M0（设计）。

| 文档 | 内容 |
| --- | --- |
| [docs/model.md](docs/model.md) | 图模型：统一结构、节点、边、记法、推导和约束 |
| [docs/format.md](docs/format.md) | 设计文件的格式与代码中的绑定 |
| [docs/design.md](docs/design.md) | 工具链：组件、语言后端、生成、视图、命令行和计划 |
| [docs/decisions.md](docs/decisions.md) | 决定记录 |

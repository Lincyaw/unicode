# AGENTS

This project's engineering conventions are the general rules of the `engineering-conventions` skill, with the rules of this file on top. Load that skill before writing, refactoring or reviewing code, designing components or interfaces, writing verification, or editing documents.

## Project

- unicode is a toolchain that builds one graph of a repository's design intent, code and evidence; notations (DDD, UML class and sequence diagrams, activity diagrams, C4) are views of that graph. `docs/model.md` defines the graph, `docs/format.md` the design file format and the code bindings, and `docs/design.md` the toolchain itself. `docs/decisions.md` logs each decision and its reason.
- Design comes first. A change to the graph, the format or the toolchain's structure edits those documents, and from M1 the toolchain's own design files in `design/`, before any code changes.
- The toolchain describes itself in its own format from M1 and checks itself from M2; from then on `unicode fmt --check` and `unicode check` run with the other checks.

## Roles

- The main session designs and reviews. It dispatches implementation to subagents. Each task carries the slice of the design nodes it implements (`unicode slice` from M3). The main session reviews the result with the toolchain and by reading the code. The toolchain itself does not schedule agents.

## Go

- Go 1.26, module `github.com/lincyaw/unicode`. Code is formatted with gofmt and passes `go vet` and `golangci-lint`.
- Each package under `internal/` is one component of the design. Its imports follow the design's `uses`, checked by golangci-lint's depguard until M2 and by `unicode check` from M2.
- Verification is by properties, with `pgregory.net/rapid` and native fuzzing. Concrete examples go in as cases or seeds; no test pins one example.
- General Go rules belong in the conventions skill as `reference/go.md`. Until that reference exists, this section holds them.

## Python

- The Python backend's extraction script and the `uc` package use only the standard library. The skill's Python reference applies to them.

## Documents

- `docs/` holds documents only. Each subject has one current document, edited in place on each decision; `docs/decisions.md` records only the decision, its reason and the section it changed.

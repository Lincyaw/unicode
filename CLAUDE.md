# CLAUDE.md

This project's engineering conventions are the general rules of the `engineering-conventions` skill, with the rules of this file on top. Load that skill before writing, refactoring or reviewing code, designing components or interfaces, writing verification, or editing documents. `AGENTS.md` is a symlink to this file, so Codex reads the same rules.

## Project

- unicode is a toolchain that builds one graph of a repository's design intent, code and evidence; notations (DDD, UML class and sequence diagrams, activity diagrams, event storming, C4) are views of that graph. `docs/model.md` defines the graph, `docs/format.md` the design file format, the recorded-case files and the code bindings, and `docs/design.md` the toolchain itself. `docs/decisions.md` logs each decision and its reason.
- The toolchain's own design is in `design/*.uc`, written in its own format. Each Go package is one component there, and code element names follow the design through the name mapping of `docs/design.md` §4.

## Principles

1. **Design first.** A change to the graph, the format or the toolchain's structure edits the documents and the design files before any code. The documents state the current design; `docs/decisions.md` records only what was decided and why.
2. **Requirements are found by using the toolchain on itself.** When the toolchain's own design cannot say something it needs, the model or the format changes (with a decision entry); the design file and the code never work around the gap.
3. **The specification decides, not the implementer.** Where `docs/model.md` or `docs/format.md` is ambiguous, the implementer picks the reading closest to the text, states the ambiguity and the reading in its report, and the main session fixes the document.
4. **Strict by construction.** Every diagnostic is an error: there are no warnings. Each one carries a stable code (`<component>.<kebab-name>`), a position, the cause and the fix. Nothing is suppressed without a one-line reason, and every suppression is listed in review.
5. **Every check is a predicate.** Each property and flow has exactly one predicate whose parameters are its symbols. Tests are only the drivers that call predicates. Until `unicode gen` writes drivers (M4), they are written by hand in `*_test.go` with `pgregory.net/rapid` and native fuzzing, and are deleted when generation replaces them. No test pins one example; concrete examples are declared cases.
6. **Bindings from the first line.** Code that realizes a design node carries its `//uc:realizes` directive, predicates carry `//uc:checks`, generators of generated symbols carry `//uc:generates`, even before the checks that read them exist.
7. **The toolchain speaks English; the documents are Chinese.** Diagnostics, help text, CLI messages, identifiers, code comments, test messages and commit messages are English, so library messages (participle, cobra, pflag, the OS) fit in without translation. `docs/` is written in Chinese.
8. **Prefer maintained libraries; care about behavior, not details.** Use a maintained external library wherever one meets the need, and build our own only when none is maintained or none fits. Library code is cheaper to maintain and easier to review than our own. Each library choice is a design decision, listed in `docs/design.md` §3 and logged in `docs/decisions.md`. Effort goes into the overall behavior that the properties state, not into hand-tuning details.
9. **Expensive verification runs only with the user's approval.** A lane runs `make check` once, with the default rapid settings, and commits. Mutation testing (gremlins), long fuzzing and high rapid counts are `make deep`. Nobody starts it on their own: not a lane, not a review, not the main session, not a scheduled job. When a piece of development is done, the main session reports it to the user and proposes a deep run, and runs it only after the user agrees. Each surviving mutant or failure it finds becomes a fix task for a later lane.
10. **The viewer shows data, quietly and formally.** A page shows data only: every explanation (a section's note, a legend, how to read a view, how a number is counted) sits behind a small ⓘ that shows it on hover, or another small interaction such as expanding in place. Node kinds, actions, statuses and links are icons from one maintained set (Lucide; `lucide-react` in the frontend), never word labels or typed ASCII/Unicode symbols. The page's wording is formal Chinese, taken from the term table in `docs/design.md` §7.5; nothing colloquial. The Figma prototype is the viewer's visual specification: a UI change is drawn there first, then built.

## Roles and workflow

- The main session designs and reviews. It writes the design files, then dispatches implementation to subagents, one lane per coherent piece of work. Each task names the design nodes it implements and the documents to read (from M3 it carries `unicode slice`).
- A lane works in its own worktree at `../unicode-wt/<lane>` on branch `<lane>`, commits there, and never touches the main checkout.
- The main session reviews every lane with a code-reviewer subagent and by reading the code, runs `make check` on the result, merges with `git merge --no-ff`, and removes the worktree. Findings about the specification go into the documents in the same merge. A review reads the code and may run a few targeted probes; it does not run batches of mutants, which belong to the deep stage.
- Lane briefs ask for `make check` and a commit, never for mutant confirmation or long runs.
- The toolchain itself never schedules agents.

## Go

- Go 1.26, module `github.com/lincyaw/unicode`. `make check` runs golangci-lint (with gofmt), `go test ./...` and `unicode fmt --check` on this repository.
- Each package under `internal/` is one component. Its imports follow the design's `uses`, checked by golangci-lint's depguard until M2 and by `unicode check` from M2; a change to the depguard rules follows a change to the design.
- The libraries in use are listed in `docs/design.md` §3. Adding or replacing one is a design decision (principle 8).
- General Go rules belong in the conventions skill as `reference/go.md`. Until that reference exists, this section holds them.

## Python

- The Python backend's extraction script and the `uc` package use only the standard library. The skill's Python reference applies to them.

## Documents

- `docs/` holds documents only. Each subject has one current document, edited in place on each decision. Documents are in Chinese, direct and linear; clarifications go in an appendix.

import re
import sys
from pathlib import Path

# Checks that every numbered rule of docs/language.md and docs/model.md has a
# row in the document's appendix B, naming the diagnostic codes that report it
# broken and the properties that verify it, and that these tables together
# with the table of docs/design.md §8 list exactly the codes of the Go source.

ROOT = Path(__file__).resolve().parent.parent
DESIGN_DIR = ROOT / "design"
LANGUAGE_DOC = ROOT / "docs" / "language.md"
MODEL_DOC = ROOT / "docs" / "model.md"
DESIGN_DOC = ROOT / "docs" / "design.md"

RULE_HEADER = "| 规则 | 诊断码 | 验证的性质 |"
OTHER_HEADER = "| 诊断码 | 原因 |"
UNIMPLEMENTED = "未实现："
BY_PROPERTY = "由性质验证"

KEBAB = r"[a-z][a-z0-9]*(?:-[a-z0-9]+)*"
LANGUAGE_RULE = re.compile(r"^- \*\*([FLGPR][0-9]+)\*\*", re.MULTILINE)
DERIVATION = re.compile(r"^- \*\*(D[0-9]+) ", re.MULTILINE)
CONSTRAINTS = re.compile(r"^## 10 约束\n(.*?)^## ", re.MULTILINE | re.DOTALL)
CONSTRAINT = re.compile(r"^([0-9]+)\. ", re.MULTILINE)
COMPONENT = re.compile(r'^component ([a-z0-9.-]+) "', re.MULTILINE)
PATHS = re.compile(r"^  paths (.+)$", re.MULTILINE)
CHECKED = re.compile(r"^(?:property|flow) (" + KEBAB + ") ", re.MULTILINE)
QUOTED = re.compile(r'"([^"]*)"')
BACKTICKED = re.compile(r"`([^`]+)`")
# Comments, strings and runes of Go, so that a quote inside one starts no string.
GO_TOKEN = re.compile(r'//[^\n]*|/\*.*?\*/|"(?:\\.|[^"\\\n])*"|`[^`]*`|\'(?:\\.|[^\'\\\n])*\'', re.DOTALL)
GLOB_PART = re.compile(r"(\*\*/|\*\*|\*|\?)")
GLOB_REGEX = {"**/": "(?:.*/)?", "**": ".*", "*": "[^/]*", "?": "[^/]"}


def relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def component_texts() -> dict[str, str]:
    texts = {}
    for path in sorted(DESIGN_DIR.rglob("*.uc")):
        text = path.read_text(encoding="utf-8")
        if declared := COMPONENT.search(text):
            texts[declared.group(1)] = text
    return texts


def glob(pattern: str) -> re.Pattern[str]:
    parts = GLOB_PART.split(pattern)
    return re.compile("".join(GLOB_REGEX.get(part, re.escape(part)) for part in parts) + r"\Z")


def owner(path: str, patterns: dict[str, list[re.Pattern[str]]]) -> str | None:
    matching = [c for c, globs in patterns.items() if any(g.match(path) for g in globs)]
    return max(matching, key=lambda c: c.count("."), default=None)


def go_strings(text: str) -> list[tuple[int, str]]:
    strings = []
    for token in GO_TOKEN.finditer(text):
        if token.group()[0] in '"`':
            strings.append((text.count("\n", 0, token.start()) + 1, token.group()[1:-1]))
    return strings


# A diagnostic code is `<component>.<kebab-name>`, named after the component
# whose source reports it (CLAUDE.md principle 4); a string of that component
# that ends in "." or "-" is the start of a code assembled at run time.
def source_codes(patterns: dict[str, list[re.Pattern[str]]]) -> tuple[dict[str, str], list[str]]:
    codes, problems = {}, []
    for path in sorted(ROOT.rglob("*.go")):
        rel = relative(path)
        if rel.endswith("_test.go") or any(part.startswith(".") for part in path.relative_to(ROOT).parts):
            continue
        component = owner(rel, patterns)
        if component is None:
            problems.append(
                f"{rel}: the file belongs to no component of design/*.uc; add it to the paths of its component"
            )
            continue
        whole = re.compile(re.escape(component) + r"\." + KEBAB)
        partial = re.compile(re.escape(component) + r"\.(?:" + KEBAB + "-)?")
        for line, literal in go_strings(path.read_text(encoding="utf-8")):
            if whole.fullmatch(literal):
                codes.setdefault(literal, f"{rel}:{line}")
            elif partial.fullmatch(literal):
                problems.append(
                    f'{rel}:{line}: the code is assembled at run time from "{literal}"; '
                    "write each code that can be reported as one complete string literal"
                )
    return codes, problems


def cells(line: str) -> list[str]:
    return [cell.strip() for cell in line.strip("|").split("|")]


def table(doc: Path, header: str) -> list[tuple[str, list[str]]]:
    lines = [line.strip() for line in doc.read_text(encoding="utf-8").splitlines()]
    if header not in lines:
        sys.exit(f"{relative(doc)}: no table with the header {header}; add the table that rulecodes.py reads")
    start = lines.index(header) + 2
    rows = []
    for number, line in enumerate(lines[start:], start=start + 1):
        if not line.startswith("|"):
            break
        rows.append((f"{relative(doc)}:{number}", cells(line)))
    return rows


def language_rules() -> list[str]:
    return LANGUAGE_RULE.findall(LANGUAGE_DOC.read_text(encoding="utf-8"))


def model_rules() -> list[str]:
    text = MODEL_DOC.read_text(encoding="utf-8")
    section = CONSTRAINTS.search(text)
    if section is None:
        sys.exit(
            f"{relative(MODEL_DOC)}: no section '## 10 约束' followed by another section; rulecodes.py reads the constraints there"
        )
    return DERIVATION.findall(text) + CONSTRAINT.findall(section.group(1))


def cell_codes(cell: str, code_form: re.Pattern[str]) -> list[str]:
    return [t for t in BACKTICKED.findall(cell) if code_form.fullmatch(t)]


def row_problems(
    where: str, row: list[str], code_form: re.Pattern[str], source: dict[str, str], checks: set[str]
) -> list[str]:
    rule, code_cell, check_cell = row
    codes = cell_codes(code_cell, code_form)
    verified_by = BACKTICKED.findall(check_cell)
    problems = [
        f"{where}: {c} appears nowhere in the Go source; correct the code, or remove it from the row of {rule}"
        for c in codes
        if c not in source
    ]
    problems += [
        f"{where}: {p} is no property or flow of design/*.uc; name a declared one by its identity, like syntax.rejects-broken-text"
        for p in verified_by
        if p not in checks
    ]
    if not codes and UNIMPLEMENTED not in code_cell and BY_PROPERTY not in code_cell:
        problems.append(
            f"{where}: the row of {rule} names no code; list the codes reported when {rule} is broken, "
            f"or write {UNIMPLEMENTED}<milestone or reason>"
        )
    if not verified_by and (codes or BY_PROPERTY in code_cell):
        problems.append(
            f"{where}: the row of {rule} names no property; name the property or flow whose generator breaks {rule}"
        )
    return problems


def rule_table(
    doc: Path, defined: list[str], code_form: re.Pattern[str], source: dict[str, str], checks: set[str]
) -> tuple[set[str], list[str]]:
    named, problems, seen = set(), [], set()
    for where, row in table(doc, RULE_HEADER):
        if len(row) != len(cells(RULE_HEADER)):
            problems.append(f"{where}: a row has the cells {', '.join(cells(RULE_HEADER))}")
            continue
        rule = row[0]
        if rule not in defined:
            problems.append(
                f"{where}: {rule} is no rule of {relative(doc)}; write the rule's id as the document defines it"
            )
        if rule in seen:
            problems.append(f"{where}: {rule} has a second row; merge the two rows into one")
        seen.add(rule)
        named.update(cell_codes(row[1], code_form))
        problems += row_problems(where, row, code_form, source, checks)
    problems += [
        f"{relative(doc)}: the rule {r} has no row in 附录 B; add one with the codes that report it broken and the property that verifies it"
        for r in defined
        if r not in seen
    ]
    return named, problems


def other_table(source: dict[str, str], ruled: set[str]) -> tuple[set[str], list[str]]:
    named, problems = set(), []
    for where, row in table(DESIGN_DOC, OTHER_HEADER):
        codes = BACKTICKED.findall(row[0])
        if len(row) != len(cells(OTHER_HEADER)) or len(codes) != 1 or not row[1]:
            problems.append(
                f"{where}: a row has the cells {', '.join(cells(OTHER_HEADER))}: one code, and the reason it belongs to no rule"
            )
            continue
        code = codes[0]
        named.add(code)
        if code not in source:
            problems.append(f"{where}: {code} appears nowhere in the Go source; correct the code, or remove its row")
        if code in ruled:
            problems.append(f"{where}: {code} is in the row of a rule too; keep it in one of the two places")
    return named, problems


def main() -> int:
    texts = component_texts()
    patterns = {c: [glob(p) for line in PATHS.findall(text) for p in QUOTED.findall(line)] for c, text in texts.items()}
    checks = {c + "." + name for c, text in texts.items() for name in CHECKED.findall(text)}
    code_form = re.compile("(?:" + "|".join(re.escape(c) for c in texts) + r")\." + KEBAB)
    source, problems = source_codes(patterns)
    language = language_rules()
    model = model_rules()
    in_language, found = rule_table(LANGUAGE_DOC, language, code_form, source, checks)
    problems += found
    in_model, found = rule_table(MODEL_DOC, model, code_form, source, checks)
    problems += found
    others, found = other_table(source, in_language | in_model)
    problems += found
    listed = in_language | in_model | others
    problems += [
        f"{where}: the code {code} is in no table; add it to the row of the rule whose breach it reports, in "
        f"{relative(LANGUAGE_DOC)} or {relative(MODEL_DOC)} 附录 B, or to {relative(DESIGN_DOC)} §8 with the reason it belongs to no rule"
        for code, where in sorted(source.items())
        if code not in listed
    ]
    for problem in problems:
        print(problem, file=sys.stderr)
    if problems:
        return 1
    print(
        f"rulecodes: {len(language) + len(model)} rules, {len(source)} codes, {len(others)} of them outside the rules"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())

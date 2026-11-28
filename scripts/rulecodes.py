import re
import sys
from pathlib import Path

# Checks that every numbered rule of docs/language.md and docs/model.md has a
# row in the document's appendix B, naming the diagnostic codes that report it
# broken and the properties that verify it, and that these tables together
# with the table of docs/design.md §8 list exactly the codes of the Go source,
# each of them written in the form <component>.<kebab-name>.

ROOT = Path(__file__).resolve().parent.parent
DESIGN_DIR = ROOT / "design"
LANGUAGE_DOC = ROOT / "docs" / "language.md"
MODEL_DOC = ROOT / "docs" / "model.md"
DESIGN_DOC = ROOT / "docs" / "design.md"

RULE_HEADER = "| 规则 | 诊断码 | 验证的性质 |"
OTHER_HEADER = "| 诊断码 | 原因 |"
UNIMPLEMENTED = "待做："
BY_PROPERTY = "由性质验证"

KEBAB = r"[a-z][a-z0-9]*(?:-[a-z0-9]+)*"
LANGUAGE_RULE = re.compile(r"^- \*\*([BEFLGPR][0-9]+)\*\*", re.MULTILINE)
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


# Where a string literal of Go source is written decides whether it is a
# diagnostic code. A code is reported where it is the value of a Code field
# or the argument for the string parameter named code of a function of the
# same component; it is referenced where it is compared with == or != to a
# Code field or a variable named code, or listed in a case of a switch on one.
# Any other string, such as a file name like graph.json, is not a code.
CODE_FIELD = re.compile(r"\bCode:\s*\Z")
COMPARED_BEFORE = re.compile(r"\b(?:\w+\.)*(?:Code|code)\s*(?:==|!=)\s*\Z")
COMPARED_AFTER = re.compile(r"\s*(?:==|!=)\s*(?:\w+\.)*(?:Code|code)\b")
CODE_SWITCH = re.compile(r"\bswitch\s+(?:\w+\.)*(?:Code|code)\s*\{")
FUNC_DECL = re.compile(r"\bfunc\s*(?:\([^()]*\)\s*)?(\w+)\s*(?:\[[^\]]*\])?\(")
CALL = re.compile(r"\b(\w+)\s*\(")
DECLARED = re.compile(r"\bfunc\s*(?:\([^()]*\)\s*)?\Z")
LITERAL = re.compile(r'"(?:\\.|[^"\\\n])*"|`[^`]*`')


class Source:
    """A Go file with the spans of its comments, strings and runes."""

    def __init__(self, text: str):
        self.text = text
        self.tokens = list(GO_TOKEN.finditer(text))
        self.skip = {t.start(): t.end() for t in self.tokens}

    def line(self, offset: int) -> int:
        return self.text.count("\n", 0, offset) + 1

    def inside(self, offset: int) -> bool:
        return any(t.start() <= offset < t.end() for t in self.tokens)

    def strings(self) -> list[tuple[int, int, str]]:
        return [(t.start(), t.end(), t.group()[1:-1]) for t in self.tokens if t.group()[0] in '"`']

    # The spans of the comma-separated items between the bracket at open and
    # the bracket that closes it.
    def items(self, open: int) -> list[tuple[int, int]]:
        depth, i, start, items = 0, open + 1, open + 1, []
        while i < len(self.text):
            if i in self.skip:
                i = self.skip[i]
                continue
            c = self.text[i]
            if c in "([{":
                depth += 1
            elif c in ")]}":
                if depth == 0:
                    items.append((start, i))
                    return items
                depth -= 1
            elif c == "," and depth == 0:
                items.append((start, i))
                start = i + 1
            i += 1
        return items

    # The offset just past the bracket that closes the one at open.
    def close(self, open: int) -> int:
        items = self.items(open)
        return items[-1][1] + 1 if items else len(self.text)

    # The functions declared here that take a string parameter named code:
    # each with that parameter's index and its body.
    def code_parameters(self) -> dict[str, tuple[int, str]]:
        found = {}
        for m in FUNC_DECL.finditer(self.text):
            if self.inside(m.start()):
                continue
            named, pending = [], []
            for a, b in self.items(m.end() - 1):
                bits = self.text[a:b].split(None, 1)
                if len(bits) == 1:
                    pending.append(bits[0])
                elif bits:
                    named += [(n, bits[1].strip()) for n in pending + [bits[0]]]
                    pending = []
            body = self.text.find("{", self.close(m.end() - 1))
            for index, (name, kind) in enumerate(named):
                if name == "code" and kind == "string" and body >= 0:
                    found[m.group(1)] = (index, self.text[body : self.close(body)])
        return found

    # The string literals written where a code goes: (offset, literal,
    # reported), reported being false for a code that is only referenced.
    def code_uses(self, parameters: dict[str, int]) -> list[tuple[int, str, bool]]:
        uses = []
        for start, end, literal in self.strings():
            if CODE_FIELD.search(self.text, max(0, start - 64), start):
                uses.append((start, literal, True))
            elif literal and (COMPARED_BEFORE.search(self.text, max(0, start - 64), start) or COMPARED_AFTER.match(self.text, end)):
                uses.append((start, literal, False))
        for m in CALL.finditer(self.text):
            if m.group(1) not in parameters or self.inside(m.start()) or DECLARED.search(self.text, max(0, m.start() - 64), m.start()):
                continue
            items = self.items(m.end() - 1)
            index = parameters[m.group(1)]
            if index < len(items):
                a, b = items[index]
                argument = self.text[a:b].strip()
                if LITERAL.fullmatch(argument):
                    uses.append((a + self.text[a:b].index(argument), argument[1:-1], True))
        for m in CODE_SWITCH.finditer(self.text):
            if self.inside(m.start()):
                continue
            uses += [(offset, literal, False) for offset, literal in self.case_literals(m.end() - 1)]
        return uses

    # The string literals in the case lists of the switch whose block opens at open.
    def case_literals(self, open: int) -> list[tuple[int, str]]:
        found, depth, i, listing = [], 0, open + 1, False
        while i < len(self.text):
            if i in self.skip:
                if listing and self.text[i] in '"`':
                    found.append((i, self.text[i + 1 : self.skip[i] - 1]))
                i = self.skip[i]
                continue
            c = self.text[i]
            if c in "([{":
                depth += 1
            elif c in ")]}":
                if depth == 0:
                    return found
                depth -= 1
            elif depth == 0 and self.text.startswith("case", i) and not self.text[i - 1].isalnum() and not self.text[i + 4].isalnum():
                listing = True
            elif depth == 0 and c == ":":
                listing = False
            i += 1
        return found


# A function reports the code it takes when its body puts its parameter code
# into a Code field or a field named code (a diagnostic, an issue), into the
# first field of a struct literal, or passes it on to a function that reports
# it. A parameter named code that the body only compares is not reported.
STORES_CODE = re.compile(r"\b(?:Code|code):\s*code\b|\{\s*code\s*,")


def reporting(declared: dict[str, tuple[int, str]]) -> dict[str, int]:
    found = {name: index for name, (index, body) in declared.items() if STORES_CODE.search(body)}
    while True:
        passed = {
            name: index
            for name, (index, body) in declared.items()
            if name not in found
            and any(
                index_of == argument
                for callee, index_of in found.items()
                for argument in passed_code(body, callee)
            )
        }
        if not passed:
            return found
        found.update(passed)


# The indices at which body passes its parameter code to callee.
def passed_code(body: str, callee: str) -> list[int]:
    source = Source(body)
    out = []
    for m in re.finditer(r"\b" + re.escape(callee) + r"\s*\(", body):
        if source.inside(m.start()):
            continue
        for index, (a, b) in enumerate(source.items(m.end() - 1)):
            if body[a:b].strip() == "code":
                out.append(index)
    return out


def go_files(patterns: dict[str, list[re.Pattern[str]]]) -> tuple[list[tuple[str, str, Source]], list[str]]:
    files, problems = [], []
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
        files.append((rel, component, Source(path.read_text(encoding="utf-8"))))
    return files, problems


# A diagnostic code is `<component>.<kebab-name>`, named after the component
# whose source reports it (CLAUDE.md principle 4). A string of a component
# that starts with its name and a dot and has no white space is one of its
# codes: it must be a whole code, or end in "." or "-", the start of a code
# assembled at run time, which is reported too. A string written where a code
# is reported must be a whole code of that component, and one written where a
# code is referenced must be a code some component's source reports, so that
# a malformed, misspelled or borrowed code is reported here rather than by a
# check of the running code.
def source_codes(
    patterns: dict[str, list[re.Pattern[str]]], code_form: re.Pattern[str]
) -> tuple[dict[str, str], list[str]]:
    files, problems = go_files(patterns)
    codes = {}
    declared: dict[str, dict[str, tuple[int, str]]] = {}
    for rel, component, source in files:
        whole = re.compile(re.escape(component) + r"\." + KEBAB)
        partial = re.compile(re.escape(component) + r"\.(?:" + KEBAB + "-)?")
        prefixed = re.compile(re.escape(component) + r"\.\S+")
        declared.setdefault(component, {}).update(source.code_parameters())
        for start, _, literal in source.strings():
            line = source.line(start)
            if whole.fullmatch(literal):
                codes.setdefault(literal, f"{rel}:{line}")
            elif partial.fullmatch(literal):
                problems.append(
                    f'{rel}:{line}: the code is assembled at run time from "{literal}"; '
                    "write each code that can be reported as one complete string literal"
                )
            elif prefixed.fullmatch(literal) and not code_form.fullmatch(literal):
                problems.append(
                    f'{rel}:{line}: "{literal}" is not a code of the form {component}.<kebab-name>; '
                    "write the name after the dot in lower-case letters and digits, its words joined by single hyphens"
                )
    parameters = {component: reporting(functions) for component, functions in declared.items()}
    for rel, component, source in files:
        whole = re.compile(re.escape(component) + r"\." + KEBAB)
        for start, literal, reported in source.code_uses(parameters[component]):
            line = source.line(start)
            if reported and not whole.fullmatch(literal):
                problems.append(
                    f'{rel}:{line}: "{literal}" is reported by the component {component}, so it must be a code of the form '
                    f"{component}.<kebab-name>; correct the component and the name, or report the code where its component reports it"
                )
            elif not reported and literal not in codes:
                problems.append(
                    f'{rel}:{line}: "{literal}" is compared with a diagnostic code, yet no component reports it; '
                    "write a code of the form <component>.<kebab-name> that the source of that component reports"
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
    source, problems = source_codes(patterns, code_form)
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

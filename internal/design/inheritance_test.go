package design

import (
	"fmt"
	"strconv"
	"strings"

	"pgregory.net/rapid"
)

// inheritanceSource writes a component whose child type extends a parent
// type with a field of the given value and implements an interface whose
// operation a property calls on the child; when overridden, the child
// declares a field and an operation of the parent's names, which shadow the
// parent's.
func inheritanceSource(value int, overridden bool) string {
	childField, childOperation := "", ""
	shown := strconv.Itoa(value)
	if overridden {
		childField = "  field value: bool\n"
		childOperation = "  op read() -> bool pure\n"
		shown = strconv.FormatBool(value%2 == 0)
	}
	return fmt.Sprintf(`component domain "Inherited members" {
  focus core
  paths "src/**"
}

type Rejected error "Declared failure" {
  field reason: int
}

type Parent object "Parent state" {
  field value: int
  field stamp: int
  op read() -> int pure
  op shift(delta: int) raises Rejected
}

type Readable interface "Readable state" {
  op look() -> int pure
}

type Child object "Child state" {
  extends Parent
  implements Readable
  field marker: int
%s%s}

property inherited-value postcondition "Visible field and operation agree" {
  about Child.read
  forall child: Child
  where child.stamp in %d..%d
  observe child
  changes none
  call child.read()
  ensure result == child.value
  ensure child.stamp == %d
  case supplied {
    child = {value: %s, stamp: %d, marker: %d}
  }
}

property implemented-operation postcondition "An implemented interface operation is visible on the type" {
  about Readable.look
  forall child: Child
  call child.look()
  ensure result == result
}

property inherited-sequence transition "A type reference retains its concrete receiver" {
  about Parent.shift, Child.read
  forall child: Child
  forall steps: list<call<Child>>
  where steps size 0..3
  where steps.shift.delta in %d..%d
  ensure child.stamp == old(child.stamp)
  case supplied {
    child = {value: %s, stamp: %d, marker: %d}
    steps = [shift(delta: %d), read()]
  }
}
`, childField, childOperation, value+1, value+1, value+1, shown, value+1, value, value, value, shown, value+1, value, value)
}

// inheritanceNames are the names of inheritanceSource and its broken forms,
// and the names each may take instead.
var inheritanceNames = map[string][]string{
	"domain":                {"domain", "family", "lineage"},
	"Rejected":              {"Rejected", "Refused", "Failed"},
	"reason":                {"reason", "why", "cause"},
	"Parent":                {"Parent", "Base", "Ancestor"},
	"Child":                 {"Child", "Derived", "Heir"},
	"stamp":                 {"stamp", "mark", "tick"},
	"marker":                {"marker", "tag", "flag"},
	"read":                  {"read", "fetch", "peek"},
	"shift":                 {"shift", "bump", "move"},
	"delta":                 {"delta", "offset", "jump"},
	"inherited-value":       {"inherited-value", "shared-value", "base-value"},
	"inherited-sequence":    {"inherited-sequence", "call-sequence", "base-sequence"},
	"child":                 {"child", "heir", "kid"},
	"steps":                 {"steps", "sequence", "trail"},
	"supplied":              {"supplied", "given", "fixed"},
	"Nominal":               {"Nominal", "Plain", "Bare"},
	"nominal-field":         {"nominal-field", "plain-field", "bare-field"},
	"nominal":               {"nominal", "plain", "bare"},
	"chosen":                {"chosen", "picked", "first"},
	"Readable":              {"Readable", "Viewable", "Inspectable"},
	"look":                  {"look", "glance", "scan"},
	"implemented-operation": {"implemented-operation", "interface-operation", "granted-operation"},
}

// inheritanceFiles are the system file and the component of inheritanceSource.
func inheritanceFiles(value int, overridden bool) map[string]string {
	return map[string]string{"design/system.uc": "system demo \"Inheritance fixture\"\n", "design/domain.uc": inheritanceSource(value, overridden)}
}

// genInheritance draws a valid design with inherited members under drawn
// names, with the fields each type declares, the symbols of the sequence,
// and the operations the tested calls refer to: the child's when it shadows
// the parent's, and the interface's for the operation the child implements.
//
//uc:generates design.inheritance-members-are-resolved.expected
func genInheritance() *rapid.Generator[Expected] {
	return rapid.Custom(func(t *rapid.T) Expected {
		value, overridden := rapid.IntRange(-20, 20).Draw(t, "value"), rapid.Bool().Draw(t, "overridden")
		childFields, operation := []string{"marker: int"}, "domain.Parent.read"
		if overridden {
			childFields, operation = append(childFields, "value: bool"), "domain.Child.read"
		}
		r := drawRenaming(t, inheritanceNames)
		return Expected{
			Sources: sourcesOf(r.files(inheritanceFiles(value, overridden))),
			Attrs: r.attrs(map[string]map[string][]string{
				"domain.Child":              {"fields": childFields},
				"domain.Parent":             {"fields": {"value: int", "stamp: int"}},
				"domain.inherited-sequence": {"symbols": {"child: domain.Child", "steps: list<call<domain.Child>>"}},
			}),
			Edges: []EdgeRef{
				{Kind: "refers", From: r.apply("domain.inherited-value"), To: r.apply(operation)},
				{Kind: "refers", From: r.apply("domain.implemented-operation"), To: r.apply("domain.Readable.look")},
			},
		}
	})
}

// genBrokenInheritance draws the valid design of genInheritance broken in
// one of four ways, a quarter of the time each: a record case leaves out an
// inherited field; an enum or an interface extends the parent and a limit
// and an ensure read its inherited field; a sequence of calls names the
// parent's operation for a child receiver; or the child no longer implements
// the interface whose operation a property calls on it; the names are drawn.
//
//uc:generates design.inheritance-rejects-invalid-members.broken
func genBrokenInheritance() *rapid.Generator[Broken] {
	return rapid.Custom(func(t *rapid.T) Broken {
		value, overridden := rapid.IntRange(-20, 20).Draw(t, "value"), rapid.Bool().Draw(t, "overridden")
		files := inheritanceFiles(value, overridden)
		source := files["design/domain.uc"]
		b := brokenFiles{files: files, path: "design/domain.uc"}
		switch rapid.IntRange(0, 3).Draw(t, "break") {
		case 0:
			files["design/domain.uc"] = strings.Replace(source, fmt.Sprintf(", stamp: %d", value+1), "", 1)
			b.codes = []string{"design.value-type-mismatch"}
		case 1:
			form, lines, supplied := "interface", "", ""
			if rapid.Bool().Draw(t, "enum") {
				form, lines, supplied = "enum", "  value chosen\n", "  case supplied {\n    nominal = chosen\n  }\n"
			}
			source = strings.Replace(source, "property inherited-value", fmt.Sprintf("type Nominal %s \"A non-record value\" {\n  extends Parent\n%s}\n\nproperty inherited-value", form, lines), 1)
			files["design/domain.uc"] = source + fmt.Sprintf(`
property nominal-field invariant "Only record values expose fields" {
  about Parent
  forall nominal: Nominal
  where nominal.stamp in %d..%d
  ensure nominal.stamp == %d
%s}
`, value, value, value, supplied)
			b.codes, b.subject = []string{"design.invalid-limit-path", "design.expression-type"}, "domain.nominal-field"
		case 2:
			files["design/domain.uc"] = strings.Replace(source, "list<call<Child>>", "list<call<Parent.shift>>", 1)
			b.codes = []string{"design.invalid-call-sequence"}
		default:
			files["design/domain.uc"] = strings.Replace(source, "  implements Readable\n", "", 1)
			b.codes, b.subject = []string{"design.expression-type"}, "domain.implemented-operation"
		}
		return drawRenaming(t, inheritanceNames).broken(b).broken()
	})
}

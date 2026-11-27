package design

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

//uc:realizes design.InheritanceSample
type InheritanceSample struct {
	Value      int
	Overridden bool
}

//uc:generates design.inheritance-members-are-resolved.sample
func genInheritanceSample() *rapid.Generator[InheritanceSample] {
	return rapid.Custom(func(t *rapid.T) InheritanceSample {
		return InheritanceSample{rapid.IntRange(-20, 20).Draw(t, "value"), rapid.Bool().Draw(t, "overridden")}
	})
}
func inheritanceSource(sample InheritanceSample) string {
	childField, childOperation := "", ""
	value := strconv.Itoa(sample.Value)
	if sample.Overridden {
		childField = "  field value: bool\n"
		childOperation = "  op read() -> bool pure\n"
		value = strconv.FormatBool(sample.Value%2 == 0)
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

type Child object "Child state" {
  extends Parent
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
`, childField, childOperation, sample.Value+1, sample.Value+1, sample.Value+1, value, sample.Value+1, sample.Value, sample.Value, sample.Value, value, sample.Value+1, sample.Value, sample.Value)
}

//uc:checks design.inheritance-members-are-resolved
func checkInheritanceMembersAreResolved(t rapid.TB, sample InheritanceSample) {
	files := map[string]string{"design/system.uc": "system demo \"Inheritance fixture\"\n", "design/domain.uc": inheritanceSource(sample)}
	loaded := Load(parseAll(t, files, slices.Sorted(maps.Keys(files))))
	if len(loaded.Diagnostics) > 0 {
		t.Fatalf("visible inherited members were rejected: %v\n%s", loaded.Diagnostics, files["design/domain.uc"])
	}
	// The graph retains direct declarations. Semantic lookup does not create
	// copied child operations or flattened fields that would change ownership.
	wantChildFields := []string{"marker: int"}
	operation := "domain.Parent.read"
	if sample.Overridden {
		wantChildFields = append(wantChildFields, "value: bool")
		operation = "domain.Child.read"
	}
	if !slices.Equal(loaded.Graph.Node("domain.Child").Attrs["fields"], wantChildFields) {
		t.Fatalf("child graph attributes were flattened or shadowed incorrectly: %v", loaded.Graph.Node("domain.Child").Attrs)
	}
	if !slices.Equal(loaded.Graph.Node("domain.Parent").Attrs["fields"], []string{"value: int", "stamp: int"}) {
		t.Fatal("resolving child members changed parent declarations")
	}
	referred := false
	for _, edge := range loaded.Graph.Edges() {
		if edge.Kind == graph.EdgeKindRefers && edge.From == "domain.inherited-value" && edge.To == operation {
			referred = true
		}
	}
	if !referred {
		t.Fatalf("the inherited call did not refer to its actual declaration %s", operation)
	}
	symbols := loaded.Graph.Node("domain.inherited-sequence").Attrs["symbols"]
	if !slices.Equal(symbols, []string{"child: domain.Child", "steps: list<call<domain.Child>>"}) {
		t.Fatalf("the type-referenced sequence lost its concrete receiver: %v", symbols)
	}
	// A visible inherited field is required in a record case, including when a
	// different inherited field was shadowed by an incompatible child type.
	missing := maps.Clone(files)
	missing["design/domain.uc"] = strings.Replace(files["design/domain.uc"], fmt.Sprintf(", stamp: %d", sample.Value+1), "", 1)
	rejected := Load(parseAll(t, missing, slices.Sorted(maps.Keys(missing))))
	found := false
	for _, diagnostic := range rejected.Diagnostics {
		if diagnostic.Code == "design.value-type-mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatal("a record case omitted a required inherited field without a diagnostic")
	}
	wellFormed(t, rejected.Diagnostics, missing)
	// Enum values and interface scripts do not become record literals when
	// their type extends a type that has fields.
	for _, form := range []string{"enum", "interface"} {
		lines, supplied := "", ""
		if form == "enum" {
			lines = "  value chosen\n"
			supplied = "  case supplied {\n    nominal = chosen\n  }\n"
		}
		nominal := maps.Clone(files)
		nominal["design/domain.uc"] = strings.Replace(nominal["design/domain.uc"], "property inherited-value", fmt.Sprintf("type Nominal %s \"A non-record value\" {\n  extends Parent\n%s}\n\nproperty inherited-value", form, lines), 1)
		nominal["design/domain.uc"] += fmt.Sprintf(`

property nominal-field invariant "Only record values expose fields" {
  about Parent
  forall nominal: Nominal
  where nominal.stamp in %d..%d
  ensure nominal.stamp == %d
%s}
`, sample.Value, sample.Value, sample.Value, supplied)
		invalid := Load(parseAll(t, nominal, slices.Sorted(maps.Keys(nominal))))
		for _, code := range []string{"design.invalid-limit-path", "design.expression-type"} {
			if !slices.ContainsFunc(invalid.Diagnostics, func(diagnostic diag.Diagnostic) bool {
				return diagnostic.Subject == "domain.nominal-field" && diagnostic.Code == code
			}) {
				t.Fatalf("%s inherited a record field without %s: %v", form, code, invalid.Diagnostics)
			}
		}
		wellFormed(t, invalid.Diagnostics, nominal)
	}

	// A direct operation reference uses its declaring owner, unlike call<Child>.
	owner := maps.Clone(files)
	owner["design/domain.uc"] = strings.Replace(files["design/domain.uc"], "list<call<Child>>", "list<call<Parent.shift>>", 1)
	invalid := Load(parseAll(t, owner, slices.Sorted(maps.Keys(owner))))
	found = false
	for _, diagnostic := range invalid.Diagnostics {
		if diagnostic.Code == "design.invalid-call-sequence" {
			found = true
		}
	}
	if !found {
		t.Fatal("a direct operation sequence silently substituted a descendant receiver")
	}
}

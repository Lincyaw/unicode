package design

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

type recordingFixture struct {
	path, owner, number string
	file                syntax.File
	record              *syntax.RecordedCase
}

func explorationRecording(t rapid.TB, files map[string]string, loaded Loaded, value int) (map[string]string, recordingFixture) {
	component := "recording-fixture"
	for loaded.Graph.Node(component) != nil {
		component += "-x"
	}
	out := maps.Clone(files)
	out[componentPath(component)] = fmt.Sprintf(`component %s "A recording boundary" {
  focus core
  paths "recording-fixture/**"
}

op perform(value: int) -> int

property supplied invariant "The recording remains in its declared domain" {
  about perform
  forall number: int
  where number in 0..10
  manual "The design loader checks this input without executing the operation"
  case declared {
    number = %d
  }
}
`, component, value)
	owner := component + ".supplied"
	path := corpusPath(owner)
	out[path] = fmt.Sprintf("corpus %s\n\ncase r-00000000 coverage {\n  number = %d\n}\n", owner, value)
	file := parseAll(t, out, []string{path})[0]
	return out, recordingFixture{path, owner, "number", file, file.Decls[1].(*syntax.RecordedCase)}
}

func addedRecording(files map[string]string, fixture recordingFixture, change func(*syntax.RecordedCase)) (map[string]string, string) {
	file := fixture.file
	file.Decls = slices.Clone(file.Decls)
	record := *fixture.record
	record.Lines = slices.Clone(record.Lines)
	for index := 0; ; index++ {
		record.Name = syntax.Kebab(fmt.Sprintf("r-%08x", index))
		if !slices.ContainsFunc(file.Decls, func(declaration syntax.Decl) bool {
			prior, ok := declaration.(*syntax.RecordedCase)
			return ok && prior.Name == record.Name
		}) {
			break
		}
	}
	change(&record)
	file.Decls = append(file.Decls, &record)
	out := maps.Clone(files)
	out[fixture.path] = string(file.Print())
	return out, fixture.owner + "." + string(record.Name)
}

func recordingNumber(record *syntax.RecordedCase, name string, replacement syntax.Value) {
	for index, line := range record.Lines {
		if assignment, ok := line.(*syntax.Assignment); ok && string(assignment.Symbol) == name {
			copy := *assignment
			copy.Value = replacement
			record.Lines[index] = &copy
			return
		}
	}
	panic("the recorded input lost its independently selected numeric symbol")
}

func explorationModes(t rapid.TB, files map[string]string) (Loaded, Explored) {
	parsed := parseAll(t, files, slices.Sorted(maps.Keys(files)))
	before := make([]string, len(parsed))
	for index := range parsed {
		before[index] = string(parsed[index].Print())
	}
	strict, explored := Load(parsed), LoadForExploration(parsed)
	for index := range parsed {
		if string(parsed[index].Print()) != before[index] {
			t.Fatal("design loading changed its source syntax tree")
		}
	}
	wellFormed(t, strict.Diagnostics, files)
	wellFormed(t, explored.Loaded.Diagnostics, files)
	return strict, explored
}

//uc:checks design.exploration-prunes-recorded-inputs
func checkExplorationPrunesRecordedInputs(t rapid.TB, sample Sample) {
	base, unchanged := explorationModes(t, sample.Files)
	if len(base.Diagnostics) != 0 || len(unchanged.Loaded.Diagnostics) != 0 || len(unchanged.Discarded)+len(unchanged.Orphaned) != 0 || !bytes.Equal(base.Graph.Encode(), unchanged.Loaded.Graph.Encode()) {
		t.Fatalf("exploration changed valid recordings: %+v", unchanged)
	}
	files, fixture := explorationRecording(t, sample.Files, base, len(sample.Nodes)%11)
	base, unchanged = explorationModes(t, files)
	if len(base.Diagnostics)+len(unchanged.Loaded.Diagnostics) != 0 {
		t.Fatalf("preparing a recording boundary broke the generated design: %v", base.Diagnostics)
	}
	changes := []struct {
		code  string
		apply func(*syntax.RecordedCase)
	}{
		{"design.unknown-symbol", func(record *syntax.RecordedCase) {
			name := "unknownInput"
			for slices.ContainsFunc(record.Lines, func(line syntax.CaseLine) bool {
				assignment, ok := line.(*syntax.Assignment)
				return ok && string(assignment.Symbol) == name
			}) {
				name += "x"
			}
			record.Lines = append(record.Lines, &syntax.Assignment{Symbol: syntax.Lower(name), Value: &syntax.BoolValue{Value: true}})
		}},
		{"design.missing-value", func(record *syntax.RecordedCase) {
			record.Lines = slices.DeleteFunc(record.Lines, func(line syntax.CaseLine) bool {
				assignment, ok := line.(*syntax.Assignment)
				return ok && string(assignment.Symbol) == fixture.number
			})
		}},
		{"design.duplicate-value", func(record *syntax.RecordedCase) {
			for _, line := range record.Lines {
				if assignment, ok := line.(*syntax.Assignment); ok && string(assignment.Symbol) == fixture.number {
					record.Lines = append(record.Lines, assignment)
					return
				}
			}
		}},
		{"design.value-type-mismatch", func(record *syntax.RecordedCase) {
			recordingNumber(record, fixture.number, &syntax.StringValue{Text: "not a number"})
		}},
		{"design.value-out-of-limit", func(record *syntax.RecordedCase) {
			recordingNumber(record, fixture.number, &syntax.NumberValue{Text: "11"})
		}},
	}
	for _, change := range changes {
		files, id := addedRecording(files, fixture, change.apply)
		strict, explored := explorationModes(t, files)
		if !slices.ContainsFunc(strict.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == change.code && d.Subject == id }) {
			t.Fatalf("ordinary loading accepted stale input %s (%s): %v", id, change.code, strict.Diagnostics)
		}
		if len(explored.Loaded.Diagnostics) != 0 || !slices.Equal(explored.Discarded, []string{id}) || len(explored.Orphaned) != 0 || explored.Loaded.Graph.Node(id) != nil || !bytes.Equal(explored.Loaded.Graph.Encode(), base.Graph.Encode()) {
			t.Fatalf("exploration did not remove only stale recording %s (%s): %+v", id, change.code, explored)
		}
	}
	valid, id := addedRecording(files, fixture, func(*syntax.RecordedCase) {})
	strict, explored := explorationModes(t, valid)
	if len(strict.Diagnostics)+len(explored.Loaded.Diagnostics)+len(explored.Discarded)+len(explored.Orphaned) != 0 || explored.Loaded.Graph.Node(id) == nil || !bytes.Equal(strict.Graph.Encode(), explored.Loaded.Graph.Encode()) {
		t.Fatal("exploration discarded a new valid recording")
	}
	checkOrphanedRecordings(t, files, fixture, base)
	checkStrictExplorationErrors(t, files, fixture, base)
}

func checkOrphanedRecordings(t rapid.TB, files map[string]string, fixture recordingFixture, base Loaded) {
	missing := "retired.check"
	for base.Graph.Node(missing) != nil {
		missing += "x"
	}
	wrongKind := fixture.owner[:strings.LastIndex(fixture.owner, ".")]
	for _, node := range base.Graph.Nodes() {
		if node.Layer == graph.LayerDesign && node.Kind == graph.NodeKindType {
			wrongKind = node.Id
			break
		}
	}
	for _, target := range []string{missing, wrongKind} {
		file := fixture.file
		file.Decls = slices.Clone(file.Decls)
		file.Decls[0] = &syntax.CorpusDecl{Target: syntax.Ref{Segments: strings.Split(target, ".")}}
		file.Path = corpusPath(target)
		candidate := maps.Clone(files)
		candidate[file.Path] = string(file.Print())
		strict, explored := explorationModes(t, candidate)
		if len(strict.Diagnostics) == 0 || len(explored.Loaded.Diagnostics)+len(explored.Discarded) != 0 || !slices.Equal(explored.Orphaned, []string{file.Path}) || !bytes.Equal(explored.Loaded.Graph.Encode(), base.Graph.Encode()) {
			t.Fatalf("exploration did not isolate orphaned corpus %s: %+v", file.Path, explored)
		}
		delete(candidate, file.Path)
		file.Path = "corpus/wrong-location.uc"
		candidate[file.Path] = string(file.Print())
		_, bad := explorationModes(t, candidate)
		if !slices.ContainsFunc(bad.Loaded.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "design.corpus-path-mismatch" }) || len(bad.Orphaned) != 0 {
			t.Fatal("exploration hid a corpus path error")
		}
	}
}

func checkStrictExplorationErrors(t rapid.TB, files map[string]string, fixture recordingFixture, base Loaded) {
	file := fixture.file
	file.Decls = append(slices.Clone(file.Decls), fixture.record)
	candidate := maps.Clone(files)
	candidate[fixture.path] = string(file.Print())
	_, duplicate := explorationModes(t, candidate)
	if !slices.ContainsFunc(duplicate.Loaded.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "design.duplicate-name" }) || len(duplicate.Discarded) != 0 {
		t.Fatalf("exploration hid a duplicate recording identity: %v", duplicate.Loaded.Diagnostics)
	}
	path := base.Graph.Node(fixture.owner).Origin.Path
	for _, mutation := range []string{"declared", "reference"} {
		parsed := parseAll(t, files, []string{path})[0]
		for _, declaration := range parsed.Decls {
			property, ok := declaration.(*syntax.PropertyDecl)
			if !ok || string(property.Name) != fixture.owner[strings.LastIndex(fixture.owner, ".")+1:] {
				continue
			}
			for _, line := range property.Lines {
				if record, ok := line.(*syntax.CaseDecl); ok && mutation == "declared" {
					for _, value := range record.Lines {
						if assignment, ok := value.(*syntax.Assignment); ok && string(assignment.Symbol) == fixture.number {
							assignment.Value = &syntax.NumberValue{Text: "11"}
						}
					}
				}
				if about, ok := line.(*syntax.About); ok && mutation == "reference" {
					about.Refs = append(about.Refs, syntax.Ref{Segments: []string{"retired", "missing"}})
				}
			}
		}
		candidate := maps.Clone(files)
		candidate[path] = string(parsed.Print())
		strict, explored := explorationModes(t, candidate)
		if len(strict.Diagnostics) == 0 || len(explored.Loaded.Diagnostics) == 0 || len(explored.Discarded)+len(explored.Orphaned) != 0 {
			t.Fatalf("exploration hid a %s design error: %+v", mutation, explored)
		}
	}
}

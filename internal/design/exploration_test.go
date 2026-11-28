package design

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/syntax"
)

// Exploration is a set of sources, the sources before one recorded case or
// recording file, the target, was added to them, and the code loading the
// sources must report.
//
//uc:realizes design.Exploration
type Exploration struct {
	Sources []syntax.Source
	Base    []syntax.Source
	Target  string
	Code    string
}

// recordingFixture is the recording boundary added to a valid design: a
// component whose property has a declared case and a recording file with
// one recorded case of the integer symbol number.
type recordingFixture struct {
	files                  map[string]string
	nodes                  []string
	path, owner, component string
	number                 string
	file                   syntax.File
	record                 *syntax.RecordedCase
}

// parseFile parses a file a generator wrote, which must parse.
func parseFile(t *rapid.T, path, text string) syntax.File {
	kind, err := syntax.KindOf(path)
	if err != nil {
		t.Fatalf("setup failed: %s is not a design or corpus file", path)
	}
	parsed := syntax.Parse(path, kind, []byte(text))
	if parsed.File == nil {
		t.Fatalf("setup failed: the generated file %s does not parse: %v\n%s", path, parsed.Diagnostics, text)
	}
	return *parsed.File
}

// explorationRecording draws a valid design and adds a recording boundary
// whose recorded case gives number a drawn value of its limit 0..10.
func explorationRecording(t *rapid.T) recordingFixture {
	design := designFixture(t)
	component := "recording-fixture"
	for slices.Contains(design.nodes, component) {
		component += "-x"
	}
	value := rapid.IntRange(0, 10).Draw(t, "recorded value")
	files := maps.Clone(design.files)
	files[componentPath(component)] = fmt.Sprintf(`component %s "A recording boundary" {
  focus core
  paths "recording-fixture/**"
}

type Probe record "A type that no recording can target" {
  field n: int
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
	files[path] = fmt.Sprintf("corpus %s\n\ncase r-00000000 coverage {\n  number = %d\n}\n", owner, value)
	file := parseFile(t, path, files[path])
	return recordingFixture{files, design.nodes, path, owner, component, "number", file, file.Decls[1].(*syntax.RecordedCase)}
}

// addedRecording adds to the recording file a copy of its recorded case
// under a new name, changed by change, and returns the files and the
// identity of the new case.
func (f recordingFixture) addedRecording(change func(*syntax.RecordedCase)) (map[string]string, string) {
	file := f.file
	file.Decls = slices.Clone(file.Decls)
	record := *f.record
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
	out := maps.Clone(f.files)
	out[f.path] = string(file.Print())
	return out, f.owner + "." + string(record.Name)
}

// recordingNumber replaces the value the recorded case gives the symbol name.
func recordingNumber(record *syntax.RecordedCase, name string, replacement syntax.Value) {
	for index, line := range record.Lines {
		if assignment, ok := line.(*syntax.Assignment); ok && string(assignment.Symbol) == name {
			copy := *assignment
			copy.Value = replacement
			record.Lines[index] = &copy
			return
		}
	}
	panic("the recorded input lost its numeric symbol")
}

// genExplorationValid draws a valid design with a recording boundary whose
// recording file gains a second valid recorded case, the target.
//
//uc:generates design.exploration-keeps-valid-recordings.sample
func genExplorationValid() *rapid.Generator[Exploration] {
	return rapid.Custom(func(t *rapid.T) Exploration {
		f := explorationRecording(t)
		files, id := f.addedRecording(func(*syntax.RecordedCase) {})
		return Exploration{Sources: sourcesOf(files), Base: sourcesOf(f.files), Target: id}
	})
}

// staleChanges make a recorded case stale, each with the code ordinary
// loading reports for it; outside is a value outside the limit 0..10.
func staleChanges(number string, outside int) []struct {
	code  string
	apply func(*syntax.RecordedCase)
} {
	return []struct {
		code  string
		apply func(*syntax.RecordedCase)
	}{
		{"design.unknown-symbol", func(record *syntax.RecordedCase) {
			record.Lines = append(record.Lines, &syntax.Assignment{Symbol: syntax.Lower(number + "Unknown"), Value: &syntax.BoolValue{Value: true}})
		}},
		{"design.missing-value", func(record *syntax.RecordedCase) {
			record.Lines = slices.DeleteFunc(record.Lines, func(line syntax.CaseLine) bool {
				assignment, ok := line.(*syntax.Assignment)
				return ok && string(assignment.Symbol) == number
			})
		}},
		{"design.duplicate-value", func(record *syntax.RecordedCase) {
			for _, line := range record.Lines {
				if assignment, ok := line.(*syntax.Assignment); ok && string(assignment.Symbol) == number {
					record.Lines = append(record.Lines, assignment)
					return
				}
			}
		}},
		{"design.value-type-mismatch", func(record *syntax.RecordedCase) {
			recordingNumber(record, number, &syntax.StringValue{Text: "not a number"})
		}},
		{"design.value-out-of-limit", func(record *syntax.RecordedCase) {
			recordingNumber(record, number, &syntax.NumberValue{Text: syntax.Number(fmt.Sprint(outside))})
		}},
	}
}

// genExplorationStale draws a valid design with a recording boundary whose
// recording file gains a recorded case, the target, that no longer fits the
// design: an unknown symbol, a missing or repeated value, a string value or
// one outside its limit (-1000 to -1 or 11 to 1000), each a fifth of the time.
//
//uc:generates design.exploration-discards-stale-recordings.sample
func genExplorationStale() *rapid.Generator[Exploration] {
	return rapid.Custom(func(t *rapid.T) Exploration {
		f := explorationRecording(t)
		outside := rapid.OneOf(rapid.IntRange(-1000, -1), rapid.IntRange(11, 1000)).Draw(t, "outside")
		change := rapid.SampledFrom(staleChanges(f.number, outside)).Draw(t, "stale")
		files, id := f.addedRecording(change.apply)
		return Exploration{Sources: sourcesOf(files), Base: sourcesOf(f.files), Target: id, Code: change.code}
	})
}

// orphanTarget draws what a recording file points to that is no property
// or flow: an identity of no node, or the boundary's type.
func orphanTarget(t *rapid.T, f recordingFixture) string {
	if rapid.Bool().Draw(t, "missing target") {
		return absentID(f.nodes, "retired.check")
	}
	return f.component + ".Probe"
}

// absentID is id, with x appended until it is the identity of no node.
func absentID(nodes []string, id string) string {
	for slices.Contains(nodes, id) {
		id += "x"
	}
	return id
}

// retargeted is the recording file pointing to target instead.
func (f recordingFixture) retargeted(target string) syntax.File {
	file := f.file
	file.Decls = slices.Clone(file.Decls)
	file.Decls[0] = &syntax.CorpusDecl{Target: syntax.Ref{Segments: strings.Split(target, ".")}}
	return file
}

// genExplorationOrphan draws a valid design with a recording boundary and a
// second recording file, the target, at the path of what it points to: an
// identity of no node or a type, half the time each.
//
//uc:generates design.exploration-isolates-orphaned-corpus.sample
func genExplorationOrphan() *rapid.Generator[Exploration] {
	return rapid.Custom(func(t *rapid.T) Exploration {
		f := explorationRecording(t)
		target := orphanTarget(t, f)
		file := f.retargeted(target)
		files := maps.Clone(f.files)
		path := corpusPath(target)
		files[path] = string(file.Print())
		return Exploration{Sources: sourcesOf(files), Base: sourcesOf(f.files), Target: path}
	})
}

// genExplorationCorpus draws a valid design with a recording boundary and,
// half the time each, a recording file that points to no property or flow
// placed at a path that does not follow its target, or the recording file
// with its recorded case repeated under the same name.
//
//uc:generates design.exploration-keeps-corpus-errors.sample
func genExplorationCorpus() *rapid.Generator[Exploration] {
	return rapid.Custom(func(t *rapid.T) Exploration {
		f := explorationRecording(t)
		files := maps.Clone(f.files)
		if rapid.Bool().Draw(t, "misplaced") {
			file := f.retargeted(orphanTarget(t, f))
			files["corpus/wrong-location.uc"] = string(file.Print())
			return Exploration{Sources: sourcesOf(files), Base: sourcesOf(f.files), Target: "corpus/wrong-location.uc", Code: "design.corpus-path-mismatch"}
		}
		file := f.file
		file.Decls = append(slices.Clone(file.Decls), f.record)
		files[f.path] = string(file.Print())
		return Exploration{Sources: sourcesOf(files), Base: sourcesOf(f.files), Target: f.owner + "." + string(f.record.Name), Code: "design.duplicate-name"}
	})
}

// genExplorationDesign draws a valid design with a recording boundary whose
// property, half the time each, gives its declared case a value outside the
// limit, or names an operation that does not exist in its about.
//
//uc:generates design.exploration-keeps-design-errors.sources
func genExplorationDesign() *rapid.Generator[[]syntax.Source] {
	return rapid.Custom(func(t *rapid.T) []syntax.Source {
		f := explorationRecording(t)
		files := maps.Clone(f.files)
		path := componentPath(f.component)
		if rapid.Bool().Draw(t, "declared out of limit") {
			at := strings.Index(files[path], "    number = ")
			end := at + strings.Index(files[path][at:], "\n")
			files[path] = files[path][:at] + "    number = 11" + files[path][end:]
		} else {
			files[path] = strings.Replace(files[path], "  about perform\n", "  about perform, "+absentID(f.nodes, "retired.missing")+"\n", 1)
		}
		return sourcesOf(files)
	})
}

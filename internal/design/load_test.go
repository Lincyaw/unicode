package design

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// This file turns the designs the generators write as text into the inputs
// of the properties of loadSources: sources ordered by path, edges as
// records, and the number of lines of each file.

// sourcesOf returns the files as sources, ordered by path, each of the kind
// its path gives. Every path a generator writes is a design or corpus file.
func sourcesOf(files map[string]string) []syntax.Source {
	var out []syntax.Source
	for _, path := range slices.Sorted(maps.Keys(files)) {
		kind, err := syntax.KindOf(path)
		if err != nil {
			panic("a generator wrote " + path + ", which is not a design or corpus file")
		}
		out = append(out, syntax.Source{Path: path, Kind: kind, Text: []byte(files[path])})
	}
	return out
}

// edgeRef reads an edge written `kind from to`; identities hold no spaces.
func edgeRef(key string) EdgeRef {
	parts := strings.Split(key, " ")
	if len(parts) != 3 {
		panic("an edge is written `kind from to`, not " + key)
	}
	return EdgeRef{Kind: graph.EdgeKind(parts[0]), From: parts[1], To: parts[2]}
}

func edgeRefs(keys []string) []EdgeRef {
	out := []EdgeRef{}
	for _, key := range keys {
		out = append(out, edgeRef(key))
	}
	return out
}

// lineCounts gives each file's number of lines: one more than its newlines.
func lineCounts(files map[string]string) map[string]int {
	out := map[string]int{}
	for path, text := range files {
		out[path] = strings.Count(text, "\n") + 1
	}
	return out
}

// renaming gives some names of a fixture other names.
type renaming map[string]string

// drawRenaming draws for each name one of its candidates. The candidates of
// different names are disjoint, so the names stay distinct.
func drawRenaming(t *rapid.T, candidates map[string][]string) renaming {
	r := renaming{}
	for _, name := range slices.Sorted(maps.Keys(candidates)) {
		r[name] = rapid.SampledFrom(candidates[name]).Draw(t, "name for "+name)
	}
	return r
}

var wordPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:-[A-Za-z0-9]+)*`)

// apply renames every whole word of s, a name possibly in kebab case, that r renames.
func (r renaming) apply(s string) string {
	return wordPattern.ReplaceAllStringFunc(s, func(word string) string {
		if renamed, ok := r[word]; ok {
			return renamed
		}
		return word
	})
}

// files renames the names in the paths and texts of files.
func (r renaming) files(files map[string]string) map[string]string {
	out := map[string]string{}
	for path, text := range files {
		out[r.apply(path)] = r.apply(text)
	}
	return out
}

// attrs renames the names in the identities and values of attributes.
func (r renaming) attrs(attrs map[string]map[string][]string) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for id, byKey := range attrs {
		renamed := map[string][]string{}
		for key, values := range byKey {
			for _, v := range values {
				renamed[key] = append(renamed[key], r.apply(v))
			}
		}
		out[r.apply(id)] = renamed
	}
	return out
}

// broken renames the names of a broken design.
func (r renaming) broken(b brokenFiles) brokenFiles {
	b.files, b.path, b.subject = r.files(b.files), r.apply(b.path), r.apply(b.subject)
	absent := map[string][]string{}
	for id, keys := range b.absent {
		absent[r.apply(id)] = keys
	}
	b.absent = absent
	return b
}

// genSources draws the sources of a valid design, of one broken in one
// place (a skeleton breakage or a contract relation), of a valid design with an @ line inserted into one to three of its
// files, which then do not parse, or of a valid design with one of its
// component files copied to a new path, whose declarations then clash with
// the original's; a design without component files gets the @ lines.
//
//uc:generates design.load-ignores-file-order.sources
func genSources() *rapid.Generator[[]syntax.Source] {
	return rapid.Custom(func(t *rapid.T) []syntax.Source {
		switch rapid.IntRange(0, 3).Draw(t, "sources") {
		case 0:
			return genSample().Draw(t, "sample").Sources
		case 1:
			return rapid.OneOf(genBroken(), genBrokenContract()).Draw(t, "broken").Sources
		}
		files := designFixture(t).files
		paths := slices.Sorted(maps.Keys(files))
		var components []string
		for _, path := range paths {
			if kind, _ := syntax.KindOf(path); kind == syntax.FileKindComponent {
				components = append(components, path)
			}
		}
		if len(components) == 0 || rapid.Bool().Draw(t, "unparsable") {
			for _, path := range rapid.SliceOfNDistinct(rapid.SampledFrom(paths), 1, 3, rapid.ID[string]).Draw(t, "unparsable files") {
				lines := strings.SplitAfter(files[path], "\n")
				at := rapid.IntRange(0, len(lines)-1).Draw(t, "line")
				files[path] = strings.Join(slices.Insert(lines, at, "@\n"), "")
			}
			return sourcesOf(files)
		}
		copied := rapid.SampledFrom(components).Draw(t, "copied")
		target := "design/copy.uc"
		for files[target] != "" {
			target = strings.TrimSuffix(target, ".uc") + "x.uc"
		}
		files[target] = files[copied]
		return sourcesOf(files)
	})
}

// genReordered draws a permutation of sources.
//
//uc:generates design.load-ignores-file-order.reordered
func genReordered(sources []syntax.Source) *rapid.Generator[[]syntax.Source] {
	return rapid.Permutation(sources)
}

// genDescribed draws an input of one of the five properties whose designs
// are broken in one place, or the sources of one of the four exploration
// properties whose loads give diagnostics, with the lines of each file.
//
//uc:generates design.diagnostics-are-described.broken
func genDescribed() *rapid.Generator[Broken] {
	explored := func(e Exploration) Broken { return describedSources(e.Sources) }
	return rapid.OneOf(genBroken(), genBrokenContract(), genBrokenReplies(), genBrokenInheritance(), genBrokenCodec(),
		rapid.Map(genExplorationStale(), explored), rapid.Map(genExplorationOrphan(), explored),
		rapid.Map(genExplorationCorpus(), explored), rapid.Map(genExplorationDesign(), describedSources))
}

// describedSources is the input of diagnostics-are-described for sources:
// the number of lines of each, and no expected code.
func describedSources(sources []syntax.Source) Broken {
	files := map[string]string{}
	for _, s := range sources {
		files[s.Path] = string(s.Text)
	}
	return Broken{Sources: sources, Lines: lineCounts(files), Codes: []string{}, Absent: map[string][]string{}}
}

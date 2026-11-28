package syntax

import (
	"slices"
	"strings"

	"pgregory.net/rapid"
)

// KindSample is a path and the kind docs/language.md §5.1 gives it, nil
// when the path is no source file of the language.
//
//uc:realizes syntax.KindSample
type KindSample struct {
	Path string
	Kind *FileKind
}

// dirNames are directory names that keep a path clean: they may hold a dot,
// equal a top directory or a file name, or hold a space or wide characters.
var dirNames = rapid.OneOf(rapid.SampledFrom([]string{"system", "design", "corpus", "x y", "中文", "a.b", "v1.uc"}),
	rapid.StringMatching(`[a-z][a-z0-9-]{0,6}`))

// fileNames are what comes before .uc in a valid file name: never empty,
// and now and then a dot, two dots or system.
var fileNames = rapid.OneOf(rapid.SampledFrom([]string{"system", ".", "..", "x y", "中文", "a.b"}),
	rapid.StringMatching(`[a-z][a-z0-9-]{0,6}`))

// plainNames have no dot, so that a segment of them is never a .uc file.
var plainNames = rapid.StringMatching(`[a-z][a-z0-9-]{0,6}`)

func dirs(t *rapid.T, names *rapid.Generator[string]) []string {
	return rapid.SliceOfN(names, 0, 2).Draw(t, "dirs")
}

// sourcePath draws the segments of a clean path to a .uc file under top.
func sourcePath(t *rapid.T, top string) []string {
	return append(append([]string{top}, dirs(t, dirNames)...), fileNames.Draw(t, "name")+".uc")
}

func kindOf(kind FileKind) *FileKind { return &kind }

// validPaths draw the segments of a system, a component and a corpus path.
var validPaths = map[FileKind]func(t *rapid.T) []string{
	FileKindSystem: func(*rapid.T) []string { return []string{"design", "system.uc"} },
	FileKindComponent: func(t *rapid.T) []string {
		return rapid.Custom(func(t *rapid.T) []string { return sourcePath(t, "design") }).
			Filter(func(segs []string) bool { return strings.Join(segs, "/") != systemFilePath }).Draw(t, "component")
	},
	FileKindCorpus: func(t *rapid.T) []string { return sourcePath(t, "corpus") },
}

func validPath(t *rapid.T) []string { return validPaths[fileKinds[uniform(t, len(fileKinds))]](t) }

func validClass(kind FileKind) func(t *rapid.T) KindSample {
	return func(t *rapid.T) KindSample { return KindSample{strings.Join(validPaths[kind](t), "/"), kindOf(kind)} }
}

// kindClasses build a path of one class: a system, component or corpus
// path with its kind, or one that breaks one condition of docs/language.md §5.1.
var kindClasses = []func(t *rapid.T) KindSample{
	validClass(FileKindSystem), validClass(FileKindComponent), validClass(FileKindCorpus),
	// The first segment is neither design nor corpus, or the path is one file name.
	func(t *rapid.T) KindSample {
		top := rapid.OneOf(rapid.SampledFrom([]string{"designs", "Design", "corpora", "docs", "design.uc", "src"}),
			plainNames.Filter(func(s string) bool { return s != "design" && s != "corpus" })).Draw(t, "top")
		if rapid.Bool().Draw(t, "one segment") {
			return KindSample{fileNames.Draw(t, "name") + ".uc", nil}
		}
		return KindSample{strings.Join(sourcePath(t, top), "/"), nil}
	},
	// The file's extension is not .uc: another one, none, or a name that ends
	// in uc without the dot.
	func(t *rapid.T) KindSample {
		segs := append([]string{rapid.SampledFrom([]string{"design", "corpus"}).Draw(t, "top")}, dirs(t, dirNames)...)
		ext := rapid.SampledFrom([]string{".UC", ".Uc", ".ucx", ".uc.bak", ".txt", ".u", ".go", "", "uc", "-uc", "_uc"}).Draw(t, "extension")
		return KindSample{strings.Join(append(segs, plainNames.Draw(t, "name")+ext), "/"), nil}
	},
	// The file is named only .uc.
	func(t *rapid.T) KindSample {
		segs := append([]string{rapid.SampledFrom([]string{"design", "corpus"}).Draw(t, "top")}, dirs(t, dirNames)...)
		return KindSample{strings.Join(append(segs, ".uc"), "/"), nil}
	},
	// The path names a directory: no segment is a file.
	func(t *rapid.T) KindSample {
		segs := append([]string{rapid.SampledFrom([]string{"design", "corpus"}).Draw(t, "top")}, dirs(t, plainNames)...)
		return KindSample{strings.Join(segs, "/"), nil}
	},
	// A segment . or .. stands somewhere in a valid path.
	func(t *rapid.T) KindSample {
		segs := validPath(t)
		segs = slices.Insert(segs, rapid.IntRange(0, len(segs)).Draw(t, "at"), rapid.SampledFrom([]string{".", ".."}).Draw(t, "segment"))
		return KindSample{strings.Join(segs, "/"), nil}
	},
	// An empty segment stands somewhere in a valid path: a doubled /, or a / at either end.
	func(t *rapid.T) KindSample {
		segs := validPath(t)
		segs = slices.Insert(segs, rapid.IntRange(0, len(segs)).Draw(t, "at"), "")
		return KindSample{strings.Join(segs, "/"), nil}
	},
}

// genKindSample first draws the class of a path, a valid one or one reason
// to reject it, each as likely, and then builds a path of that class.
//
//uc:generates syntax.kind-of-follows-path.sample
func genKindSample() *rapid.Generator[KindSample] {
	return rapid.Custom(func(t *rapid.T) KindSample { return kindClasses[uniform(t, len(kindClasses))](t) })
}

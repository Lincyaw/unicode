package evidence

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/syntax"
)

// Recording is a complete, read-only plan for the recorder-owned namespace.
//
//uc:realizes evidence.Recording
type Recording struct {
	Edits       []lang.Edit       `json:"edits"`
	Removals    []string          `json:"removals"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

type corpusFile struct {
	path string
	data []byte
}

var recordedName = regexp.MustCompile(`^r-[0-9a-f]{8}$`)

func corpusPath(check string) (string, error) {
	if !checkName.MatchString(check) {
		return "", fmt.Errorf("the check identity cannot name a corpus file")
	}
	return "corpus/" + strings.ReplaceAll(check, ".", "/") + ".uc", nil
}

func corpusFiles(root string, g *graph.Graph) (map[string]corpusFile, []string, []diag.Diagnostic) {
	files := map[string]corpusFile{}
	removals := []string{}
	problems := []diag.Diagnostic{}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return files, removals, []diag.Diagnostic{storeDiagnostic("evidence.invalid-corpus", "corpus", err.Error(), "give a repository directory")}
	}
	for _, path := range []string{absolute, filepath.Join(absolute, "corpus")} {
		info, problem := os.Lstat(path)
		if errors.Is(problem, fs.ErrNotExist) && path != absolute {
			return files, removals, problems
		}
		if problem != nil {
			return files, removals, []diag.Diagnostic{storeDiagnostic("evidence.invalid-corpus", "corpus", problem.Error(), "restore readable corpus directories")}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return files, removals, []diag.Diagnostic{storeDiagnostic("evidence.symlink", "corpus", "the corpus path is a symbolic link", "replace it with a regular directory inside the repository")}
		}
		if !info.IsDir() {
			return files, removals, []diag.Diagnostic{storeDiagnostic("evidence.invalid-corpus", "corpus", "the corpus path is not a directory", "restore the corpus directory")}
		}
	}
	err = filepath.WalkDir(filepath.Join(absolute, "corpus"), func(path string, entry fs.DirEntry, problem error) error {
		relative, _ := filepath.Rel(absolute, path)
		relative = filepath.ToSlash(relative)
		if problem != nil {
			problems = append(problems, storeDiagnostic("evidence.invalid-corpus", relative, problem.Error(), "restore read access to the corpus path"))
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			problems = append(problems, storeDiagnostic("evidence.symlink", relative, "a corpus entry is a symbolic link", "replace it with a regular file or directory before recording"))
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		// Only .uc files belong to the corpus recorder; unrelated files are untouched.
		if filepath.Ext(path) != ".uc" {
			return nil
		}
		info, problem := entry.Info()
		if problem != nil || !info.Mode().IsRegular() {
			cause := "the corpus entry is not a regular file"
			if problem != nil {
				cause = problem.Error()
			}
			problems = append(problems, storeDiagnostic("evidence.invalid-corpus", relative, cause, "restore the regular corpus file"))
			return nil
		}
		data, problem := os.ReadFile(path)
		if problem != nil {
			problems = append(problems, storeDiagnostic("evidence.invalid-corpus", relative, problem.Error(), "restore read access to the corpus file"))
			return nil
		}
		parsed := syntax.Parse(relative, syntax.FileKindCorpus, data)
		if len(parsed.Diagnostics) > 0 {
			d := storeDiagnostic("evidence.invalid-corpus", relative, parsed.Diagnostics[0].Message, "correct the corpus syntax before recording")
			d.Position = parsed.Diagnostics[0].Position
			problems = append(problems, d)
			return nil
		}
		if !bytes.Equal(parsed.File.Print(), data) {
			problems = append(problems, storeDiagnostic("evidence.invalid-corpus", relative, "the corpus file is not in canonical format", "run unicode fmt on the corpus file before recording"))
			return nil
		}
		check := ""
		for _, declaration := range parsed.File.Decls {
			switch declaration := declaration.(type) {
			case *syntax.CorpusDecl:
				check = strings.Join(declaration.Target.Segments, ".")
			case *syntax.RecordedCase:
				if !recordedName.MatchString(string(declaration.Name)) {
					problems = append(problems, storeDiagnostic("evidence.invalid-corpus", relative, "the corpus case name is not r- followed by eight lowercase hexadecimal digits", "restore the recorder-owned case identity"))
					return nil
				}
			}
		}
		expected, problem := corpusPath(check)
		if problem != nil || expected != relative {
			problems = append(problems, storeDiagnostic("evidence.invalid-corpus", relative, "the corpus header does not match its namespace path", "move the file to the path of its exact corpus target"))
			return nil
		}
		target := g.Node(check)
		if target == nil {
			removals = append(removals, relative)
			return nil
		}
		if target.Layer != graph.LayerDesign || target.Kind != graph.NodeKindProperty && target.Kind != graph.NodeKindFlow {
			problems = append(problems, storeDiagnostic("evidence.invalid-corpus", relative, "the corpus target is not a property or flow", "use the identity of an existing check"))
			return nil
		}
		files[check] = corpusFile{path: relative, data: data}
		return nil
	})
	if err != nil {
		problems = append(problems, storeDiagnostic("evidence.invalid-corpus", "corpus", err.Error(), "restore readable corpus directories"))
	}
	slices.Sort(removals)
	return files, removals, problems
}

func corpusCase(result lang.CaseResult) (string, error) {
	name := "r-" + result.InputHash[:8]
	text := "case " + name + " " + result.Reason + " {\n"
	for _, symbol := range slices.Sorted(maps.Keys(result.Input.Values)) {
		if !lowerName.MatchString(symbol) {
			return "", fmt.Errorf("the input symbol is not a model name")
		}
		value, err := valueText(result.Input.Values[symbol])
		if err != nil {
			return "", err
		}
		text += "  " + symbol + " = " + value + "\n"
	}
	return text + "}\n", nil
}

// Recordings accepts every actual case of selected checks, including replay and
// nongaining cases. Reduction happens here, so an empty retained set still has a
// known owner; unselected corpus files remain unchanged.
//
//uc:realizes evidence.recordings
func Recordings(root string, g *graph.Graph, cases []lang.CaseResult) Recording {
	out := Recording{Edits: []lang.Edit{}, Removals: []string{}, Diagnostics: []diag.Diagnostic{}}
	fail := func(code, check, cause, fix string) Recording {
		out.Edits = []lang.Edit{}
		out.Removals = []string{}
		out.Diagnostics = append(out.Diagnostics, diagnostic(code, check, cause, fix))
		return out
	}
	if len(cases) == 0 {
		return fail("evidence.invalid-input", "", "recording has no actual executed inputs", "run at least one selected check before planning corpus updates")
	}
	files, removals, problems := corpusFiles(root, g)
	if len(problems) > 0 {
		out.Diagnostics = problems
		return out
	}
	groups := map[string][]lang.CaseResult{}
	for _, source := range cases {
		result, err := cloneCase(source)
		if err != nil {
			return fail("evidence.invalid-input", source.Check, err.Error(), "provide complete frozen inputs with matching canonical digests")
		}
		if _, exists := groups[result.Check]; !exists {
			groups[result.Check] = []lang.CaseResult{}
		}
		if result.Rejected {
			continue
		}
		target := g.Node(result.Check)
		if target == nil || target.Layer != graph.LayerDesign || target.Kind != graph.NodeKindProperty && target.Kind != graph.NodeKindFlow {
			return fail("evidence.invalid-input", result.Check, "a case names an unavailable check", "select a current property or flow")
		}
		groups[result.Check] = append(groups[result.Check], result)
	}
	for _, check := range slices.Sorted(maps.Keys(groups)) {
		if len(groups[check]) == 0 {
			return fail("evidence.invalid-input", check, "the selected check has no effective input", "satisfy its types, limits and preconditions before recording")
		}
		reduced := Reduce(groups[check], 64)
		if len(reduced.Diagnostics) > 0 {
			out.Edits = []lang.Edit{}
			out.Removals = []string{}
			out.Diagnostics = append(out.Diagnostics, reduced.Diagnostics...)
			return out
		}
		byName := map[string]lang.CaseResult{}
		for _, result := range reduced.Cases {
			// Declared cases are maintained by the designer, never by corpus updates.
			if result.Form == "declared" {
				continue
			}
			name := "r-" + result.InputHash[:8]
			if before, exists := byName[name]; exists {
				if before.InputHash != result.InputHash {
					return fail("evidence.input-hash-collision", check, "different inputs share the recorded case's eight-digit name", "resolve the case-name collision before replacing any corpus files")
				}
				if result.Reason != "counterexample" {
					continue
				}
			}
			byName[name] = result
		}
		path, err := corpusPath(check)
		if err != nil {
			return fail("evidence.invalid-corpus", check, err.Error(), "use a check identity with a valid corpus namespace")
		}
		text := "corpus " + check + "\n"
		for _, name := range slices.Sorted(maps.Keys(byName)) {
			fragment, err := corpusCase(byName[name])
			if err != nil {
				return fail("evidence.unreplayable-input", check, err.Error(), "provide a lossless model value or a declared codec")
			}
			text += "\n" + fragment
		}
		parsed := syntax.Parse(path, syntax.FileKindCorpus, []byte(text))
		if len(parsed.Diagnostics) > 0 {
			return fail("evidence.unreplayable-input", check, parsed.Diagnostics[0].Message, "provide values with a valid lossless corpus notation")
		}
		for _, declaration := range parsed.File.Decls {
			if candidate, ok := declaration.(*syntax.RecordedCase); ok {
				if err := validateRoundtrip(g, check, byName[string(candidate.Name)].Input, candidate.Lines); err != nil {
					return fail("evidence.unreplayable-input", check, err.Error(), "provide complete values matching the declared symbols and codec representations")
				}
			}
		}
		content := parsed.File.Print()
		before, exists := files[check]
		if len(byName) == 0 && !exists {
			continue
		}
		if !exists || !bytes.Equal(before.data, content) {
			out.Edits = append(out.Edits, lang.Edit{Path: path, Content: content})
		}
	}
	out.Removals = removals
	slices.SortFunc(out.Edits, func(a, b lang.Edit) int { return strings.Compare(a.Path, b.Path) })
	return out
}

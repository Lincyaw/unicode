package evidence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/renameio/v2/maybe"

	"github.com/lincyaw/unicode/internal/diag"
)

func safeCorpusPath(path string) bool {
	return filepath.IsLocal(path) && !strings.Contains(path, "\\") && filepath.ToSlash(filepath.Clean(path)) == path && strings.HasPrefix(path, "corpus/") && strings.HasSuffix(path, ".uc")
}

func corpusParents(root, path string, create bool) *diag.Diagnostic {
	absolute, err := filepath.Abs(root)
	if err != nil {
		problem := storeDiagnostic("evidence.invalid-corpus", "corpus", err.Error(), "give an existing repository root")
		return &problem
	}
	parts := append([]string{""}, strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")...)
	current := absolute
	relative := ""
	for index, part := range parts {
		if index > 0 {
			current = filepath.Join(current, part)
			relative = filepath.ToSlash(filepath.Join(relative, part))
		}
		shown := relative
		if shown == "" {
			shown = "design/system.uc"
		}
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) && index > 0 {
			if !create {
				return nil
			}
			if err = os.Mkdir(current, 0o755); err == nil || errors.Is(err, fs.ErrExist) {
				info, err = os.Lstat(current)
			}
		}
		if err != nil {
			problem := storeDiagnostic("evidence.write-failed", shown, err.Error(), "restore the corpus directory and its access permissions")
			return &problem
		}
		if info.Mode()&os.ModeSymlink != 0 {
			problem := storeDiagnostic("evidence.symlink", shown, "the corpus parent is a symbolic link", "replace the link with a regular directory inside the repository")
			return &problem
		}
		if !info.IsDir() {
			problem := storeDiagnostic("evidence.invalid-corpus", shown, "the corpus parent is not a directory", "restore the corpus namespace directory")
			return &problem
		}
	}
	return nil
}

type plannedCorpus struct {
	path       string
	content    []byte
	remove     bool
	mode       fs.FileMode
	createOnly bool
}

func inspectCorpus(root string, item plannedCorpus) *diag.Diagnostic {
	if !safeCorpusPath(item.path) {
		problem := storeDiagnostic("evidence.invalid-corpus", "corpus", fmt.Sprintf("%q is not a clean relative corpus namespace path", item.path), "only write canonical corpus/<target>.uc paths")
		return &problem
	}
	if problem := corpusParents(root, item.path, false); problem != nil {
		return problem
	}
	path := filepath.Join(root, filepath.FromSlash(item.path))
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		problem := storeDiagnostic("evidence.write-failed", item.path, err.Error(), "restore read and write access to the corpus path")
		return &problem
	}
	if info.Mode()&os.ModeSymlink != 0 {
		problem := storeDiagnostic("evidence.symlink", item.path, "the corpus file is a symbolic link", "replace it with its regular canonical corpus file")
		return &problem
	}
	if !info.Mode().IsRegular() {
		problem := storeDiagnostic("evidence.invalid-corpus", item.path, "the corpus target is not a regular file", "restore the recorder-owned regular corpus file")
		return &problem
	}
	if item.createOnly {
		problem := storeDiagnostic("evidence.write-failed", item.path, "a create-only corpus plan would overwrite an existing file", "replan against the current corpus files")
		return &problem
	}
	data, err := os.ReadFile(path)
	if err != nil {
		problem := storeDiagnostic("evidence.write-failed", item.path, err.Error(), "restore read access before replacing this corpus file")
		return &problem
	}
	_, _, problem := parseCorpusFile(item.path, data)
	return problem
}

// ApplyRecordings preflights the entire namespace plan before touching files.
// Each publication is atomic; a later failure is reported rather than hidden.
//
//uc:realizes evidence.applyRecordings
func ApplyRecordings(root string, recording Recording) []diag.Diagnostic {
	if len(recording.Diagnostics) > 0 {
		return slices.Clone(recording.Diagnostics)
	}
	plans := []plannedCorpus{}
	seen := map[string]bool{}
	fail := func(path, cause, fix string) []diag.Diagnostic {
		return []diag.Diagnostic{storeDiagnostic("evidence.invalid-corpus", path, cause, fix)}
	}
	for _, edit := range recording.Edits {
		if !safeCorpusPath(edit.Path) {
			return fail("corpus", "the plan contains a non-corpus or noncanonical path", "only write canonical relative corpus/<target>.uc paths")
		}
		if seen[edit.Path] {
			return fail(edit.Path, "the plan repeats a corpus path", "provide exactly one action for each namespace path")
		}
		seen[edit.Path] = true
		if _, _, problem := parseCorpusFile(edit.Path, edit.Content); problem != nil {
			return []diag.Diagnostic{*problem}
		}
		plans = append(plans, plannedCorpus{path: edit.Path, content: slices.Clone(edit.Content), mode: 0o644, createOnly: edit.CreateOnly})
	}
	for _, path := range recording.Removals {
		if !safeCorpusPath(path) {
			return fail("corpus", "a removal is outside the canonical corpus namespace", "only remove a recorder-owned corpus/<target>.uc file")
		}
		if seen[path] {
			return fail(path, "the plan both writes and removes, or repeats, a corpus path", "provide exactly one action for each namespace path")
		}
		seen[path] = true
		plans = append(plans, plannedCorpus{path: path, remove: true})
	}
	slices.SortFunc(plans, func(a, b plannedCorpus) int { return strings.Compare(a.path, b.path) })
	for _, item := range plans {
		if problem := inspectCorpus(root, item); problem != nil {
			return []diag.Diagnostic{*problem}
		}
	}
	for _, item := range plans {
		if problem := corpusParents(root, item.path, !item.remove); problem != nil {
			return []diag.Diagnostic{*problem}
		}
		path := filepath.Join(root, filepath.FromSlash(item.path))
		if item.remove {
			if problem := inspectCorpus(root, item); problem != nil {
				return []diag.Diagnostic{*problem}
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return []diag.Diagnostic{storeDiagnostic("evidence.write-failed", item.path, err.Error(), "restore access and retry the remaining corpus plan")}
			}
			continue
		}
		if info, err := os.Lstat(path); err == nil {
			item.mode = info.Mode().Perm()
		}
		if problem := inspectCorpus(root, item); problem != nil {
			return []diag.Diagnostic{*problem}
		}
		if item.createOnly {
			if err := createCorpus(path, item.content, item.mode); err != nil {
				return []diag.Diagnostic{storeDiagnostic("evidence.write-failed", item.path, err.Error(), "replan against the current corpus files and retry")}
			}

		} else if err := maybe.WriteFile(path, item.content, item.mode); err != nil {
			return []diag.Diagnostic{storeDiagnostic("evidence.write-failed", item.path, err.Error(), "restore access and retry the remaining corpus plan")}
		}
	}
	return []diag.Diagnostic{}
}

func createCorpus(path string, content []byte, mode fs.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".corpus-pending-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	// An incomplete staging file is never a .uc file and is never published.
	defer func() { _ = os.Remove(temporary) }()
	if err = file.Chmod(mode); err == nil {
		_, err = file.Write(content)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Link(temporary, path)
}

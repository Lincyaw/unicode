package gen

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
)

// CheckFiles validates the plan's paths and ownership, then compares every
// existing generated file with its planned bytes without changing the tree.
//
//uc:realizes gen.checkFiles
func CheckFiles(root string, edits []lang.Edit) []diag.Diagnostic {
	_, ds := preflightEdits(root, edits, readDiagnostic)
	if len(ds) > 0 {
		return ds
	}
	for _, edit := range edits {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(edit.Path)))
		switch {
		case errors.Is(err, os.ErrNotExist):
			ds = append(ds, staleDiagnostic(edit.Path, "the generated file is missing"))
		case err != nil:
			ds = append(ds, readDiagnostic(edit.Path, err))
		case !bytes.Equal(content, edit.Content):
			ds = append(ds, staleDiagnostic(edit.Path, "the generated file differs from the current generation plan"))
		}
	}
	sortDiagnostics(ds)
	return ds
}

func staleDiagnostic(path, cause string) diag.Diagnostic {
	return diagnostic("gen.stale-file", path, cause, "run unicode gen to update generated files")
}

func readDiagnostic(path string, err error) diag.Diagnostic {
	return diagnostic("gen.read-failed", path, fmt.Sprintf("cannot inspect generated file: %v", err), "make this repository path and its parents readable")
}

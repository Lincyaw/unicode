package golang

import (
	"bytes"
	"fmt"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

var _ lang.Runner = (*Backend)(nil)

// Run checks generated files before running only their contract drivers.
func (b *Backend) Run(plan lang.RunPlan) (report lang.RunReport) {
	report = lang.RunReport{}
	generation := b.Generate(lang.GenPlan{Root: plan.Root, Roots: plan.Roots, Graph: plan.Graph})
	report.Diagnostics = append(report.Diagnostics, generation.Diagnostics...)
	if len(report.Diagnostics) > 0 {
		return report
	}
	for _, edit := range generation.Edits {
		path := filepath.Join(plan.Root, edit.Path)
		err := safeContractPath(plan.Root, edit.Path)
		var data []byte
		if err == nil {
			data, err = os.ReadFile(path)
		}
		current := err == nil && bytes.Equal(data, edit.Content)
		if plan.Exploring() && !edit.CreateOnly && err == nil {
			parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, data, parser.PackageClauseOnly|parser.ParseComments)
			current = parseErr == nil && unicodeGenerated(parsed)
		}
		if !current {
			report.Diagnostics = append(report.Diagnostics, diag.Diagnostic{Code: "lang.golang.stale-contracts", Subject: edit.Path, Position: diag.Position{Path: edit.Path, Line: 1, Column: 1}, Message: "the generated contract file is missing or differs from the current design", Fix: "run unicode gen before executing contracts"})
		}
	}
	if len(report.Diagnostics) > 0 {
		return report
	}
	return b.runMeasured(plan, generation)
}

func runDiagnostic(code, message, fix string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Position: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}, Message: message, Fix: fix}
}

func safeContractPath(root, relative string) error {
	if !filepath.IsLocal(relative) {
		return fmt.Errorf("generated path leaves the repository")
	}
	path := root
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("the repository root is a symbolic link")
	}
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("generated path contains a symbolic link: %s", path)
		}
	}
	return nil
}

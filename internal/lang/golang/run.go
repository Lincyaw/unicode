package golang

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
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
		if err != nil || !bytes.Equal(data, edit.Content) {
			report.Diagnostics = append(report.Diagnostics, diag.Diagnostic{Code: "lang.golang.stale-contracts", Subject: edit.Path, Position: diag.Position{Path: edit.Path, Line: 1, Column: 1}, Message: "the generated contract file is missing or differs from the current design", Fix: "run unicode gen before executing contracts"})
		}
	}
	if len(report.Diagnostics) > 0 {
		return report
	}
	selected := map[string]bool{}
	for _, id := range plan.Ids {
		selected[id] = true
	}
	temp, err := os.MkdirTemp("", "unicode-contract-results-")
	if err != nil {
		return lang.RunReport{Diagnostics: []diag.Diagnostic{runDiagnostic("lang.golang.run-failed", err.Error(), "make a writable temporary directory available")}}
	}
	defer func() {
		if err := os.RemoveAll(temp); err != nil {
			report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.run-failed", "cannot remove contract result files: "+err.Error(), "make the temporary result directory removable"))
		}
	}()
	for index, edit := range generation.Edits {
		if len(plan.Ids) > 0 {
			needed := false
			for _, id := range plan.Ids {
				if bytes.Contains(edit.Content, []byte(strconv.Quote(id))) {
					needed = true
					break
				}
			}
			if !needed {
				continue
			}
		}
		resultFile := filepath.Join(temp, fmt.Sprintf("%d.json", index))
		dir := filepath.Dir(edit.Path)
		target := "./" + filepath.ToSlash(dir)
		if dir == "." {
			target = "."
		}
		timeout := 2 * time.Minute
		if plan.Explore > 0 {
			count := len(plan.Ids)
			if count == 0 {
				count = len(plan.Graph.Nodes())
			}
			timeout += plan.Explore * time.Duration(count)
		}
		cmd := exec.Command("go", "test", target, "-run", "^TestUnicodeContracts$", "-count=1", "-timeout="+timeout.String())
		cmd.Dir = plan.Root
		cmd.Env = os.Environ()
		ids, _ := json.Marshal(plan.Ids)
		cmd.Env = append(cmd.Env, "UNICODE_CONTRACT_RESULTS="+resultFile, "UNICODE_CONTRACT_IDS="+string(ids))
		if plan.Explore > 0 {
			cmd.Env = append(cmd.Env, "UNICODE_CONTRACT_EXPLORE="+plan.Explore.String())
		}
		output, runErr := cmd.CombinedOutput()
		data, readErr := os.ReadFile(resultFile)
		if readErr != nil {
			if runErr == nil {
				report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.no-effective-input", "the Go test process returned no contract report for "+target, "regenerate the contract drivers and ensure they execute"))
			} else {
				report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.run-failed", strings.TrimSpace(string(output)), "make the generated Go tests compile and their adapters available"))
			}
			continue
		}
		var results []ucRunResult
		if err := json.Unmarshal(data, &results); err != nil {
			report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.run-failed", err.Error(), "regenerate the contract driver with this unicode version"))
			continue
		}
		packageFailed := false
		for _, wire := range results {
			if !wire.Passed {
				packageFailed = true
			}
			result := lang.RunResult{Id: wire.Id, Passed: wire.Passed, Accepted: wire.Accepted, Rejected: wire.Rejected, Observations: wire.Observations, Differences: wire.Differences}
			for _, d := range wire.Diagnostics {
				result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: d.Code, Subject: d.Subject, Position: diag.Position{Path: d.Position.Path, Line: d.Position.Line, Column: d.Position.Column}, Message: d.Message, Fix: d.Fix})
			}
			if result.Accepted == 0 && !wire.Infrastructure {
				wire.Infrastructure = true
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.no-effective-input", "the selected property "+result.Id+" has no effective execution", "declare a replay case or run a bounded exploration"))
			}
			if wire.Infrastructure {
				report.Diagnostics = append(report.Diagnostics, result.Diagnostics...)
			}

			report.Results = append(report.Results, result)
			delete(selected, result.Id)
		}
		if runErr != nil && !packageFailed {
			report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.driver-failed", strings.TrimSpace(string(output)), "inspect the Go test driver failure outside its contract results"))
		}
	}
	for id := range selected {
		report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.unsupported-contract", "no executable Go contract was generated for "+id, "select a property with a Go realization or supported manual predicate"))
	}
	if len(report.Results) == 0 && len(report.Diagnostics) == 0 {
		report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.no-effective-input", "no Go property was executed", "declare an executable property and replay case or use bounded exploration"))
	}
	slices.SortFunc(report.Results, func(a, b lang.RunResult) int { return strings.Compare(a.Id, b.Id) })
	return report
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

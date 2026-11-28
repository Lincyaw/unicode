package golang

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

type measuredRunner struct {
	program                            *measuredProgram
	binary, check                      string
	deadline                           time.Time
	trial                              bool
	codeTotal, branchTotal, errorTotal int
	durations                          []time.Duration
}

func (b *Backend) runMeasured(plan lang.RunPlan, generation lang.Generation) (report lang.RunReport) {
	report = lang.RunReport{Results: []lang.RunResult{}, Diagnostics: []diag.Diagnostic{}}
	temp, err := os.MkdirTemp("", "unicode-measured-run-")
	if err != nil {
		report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.run-failed", err.Error(), "provide a writable temporary directory"))
		return report
	}
	defer func() {
		if err := os.RemoveAll(temp); err != nil {
			report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.run-failed", err.Error(), "make temporary measurement files removable"))
		}
	}()
	program, err := prepareMeasuredProgram(plan, generation, temp)
	if err != nil {
		report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.trace-instrumentation-failed", err.Error(), "make source files and native declarations readable for measurement"))
		return report
	}
	toolContext, toolCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	toolErr := program.prepareTool(toolContext)
	toolCancel()
	if toolErr != nil {
		report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.trace-instrumentation-failed", toolErr.Error(), "make the Go toolchain available for read-only measurement"))
		return report
	}
	selected := map[string]bool{}
	for _, id := range plan.Ids {
		selected[id] = true
	}
	if len(selected) == 0 {
		for _, node := range plan.Graph.Nodes() {
			if node.Layer == graph.LayerDesign && (node.Kind == graph.NodeKindProperty || node.Kind == graph.NodeKindFlow) {
				selected[node.Id] = true
			}
		}
	}
	for index, edit := range generation.Edits {
		if !bytes.Contains(edit.Content, []byte("func TestUnicodeContracts(")) {
			continue
		}
		var ids []string
		for id := range selected {
			if bytes.Contains(edit.Content, []byte("t.Run("+strconv.Quote(id)+",")) {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		slices.Sort(ids)
		dir := filepath.Dir(edit.Path)
		target := "./" + filepath.ToSlash(dir)
		if dir == "." {
			target = "."
		}
		buildContext, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		binary, buildErr := program.compile(buildContext, target, index)
		cancel()
		if buildErr != nil {
			report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.run-failed", buildErr.Error(), "make generated contracts compile with atomic coverage"))
			continue
		}
		for _, id := range ids {
			// A file contains every referenced identity in its schema; only units whose
			// worker inventory recognizes the check belong to this package.
			runner := &measuredRunner{program: program, binary: binary, check: id}
			inventory, _, inventoryErr := runner.worker(context.Background(), ucWorkerRequest{Mode: "inventory", Check: id}, false)
			if inventoryErr != nil {
				report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.run-failed", inventoryErr.Error(), "make the generated candidate worker produce its inventory"))
				continue
			}
			if len(inventory.Diagnostics) > 0 {
				if strings.Contains(inventory.Diagnostics[0].Message, "does not contain the selected check") {
					continue
				}
				report.Diagnostics = append(report.Diagnostics, workerDiagnostics(inventory.Diagnostics)...)
				continue
			}
			result := runner.runCheck(plan, inventory.Cases)
			report.Results = append(report.Results, result)
			for _, diagnostic := range result.Diagnostics {
				if isMeasuredEnvironment(diagnostic.Code) {
					report.Diagnostics = append(report.Diagnostics, diagnostic)
				}
			}
			delete(selected, id)
		}
	}
	for id := range selected {
		report.Diagnostics = append(report.Diagnostics, runDiagnostic("lang.golang.unsupported-contract", "no generated candidate worker executed "+id, "generate its Go contract and bind its native symbols"))
	}
	slices.SortFunc(report.Results, func(a, b lang.RunResult) int { return strings.Compare(a.Id, b.Id) })
	return report
}
func (r *measuredRunner) runCheck(plan lang.RunPlan, replay []ucReplayCase) lang.RunResult {
	result := lang.RunResult{Id: r.check, Passed: true, Cases: []lang.CaseResult{}, Observations: []string{}, Differences: []string{}, Diagnostics: []diag.Diagnostic{}}
	evaluate := func(input lang.Input, form, id, reason string, replaying bool) bool {
		priorTrial := r.trial
		r.trial = !replaying
		evaluated := r.Evaluate(input)
		r.trial = priorTrial
		if len(evaluated.Diagnostics) > 0 {
			result.Diagnostics = append(result.Diagnostics, evaluated.Diagnostics...)
			result.Passed = false
			return false
		}
		if evaluated.Exhausted {
			return false
		}
		if evaluated.Result == nil {
			result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.run-failed", "the candidate returned no result", "make the candidate worker return an execution result"))
			result.Passed = false
			return false
		}
		candidate := *evaluated.Result
		candidate.Form = form
		candidate.Id = id
		candidate.Reason = reason
		if candidate.Rejected {
			result.Rejected++
			result.Cases = append(result.Cases, candidate)
			if replaying {
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.input-outside-domain", "a replayed input no longer satisfies its type, limits or preconditions", "correct the declared case or run exploration to retire invalid recorded inputs"))
			}
			return true
		}
		result.Accepted++
		result.Observations = append(result.Observations, candidate.Observations...)
		if !candidate.Passed {
			result.Passed = false

		}
		result.Cases = append(result.Cases, candidate)
		return true
	}
	for _, recorded := range replay {
		input, err := hostInput(recorded.Input)
		if err != nil {
			result.Passed = false
			result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.input-codec-failed", err.Error(), "make recorded inputs satisfy their declared codecs"))
			continue
		}
		evaluate(input, recorded.Form, recorded.Id, recorded.Reason, true)
	}
	if plan.Explore > 0 && result.Passed {
		r.deadline = time.Now().Add(plan.Explore)
		for time.Now().Before(r.deadline) {
			seed := make([]byte, 32)
			if _, err := rand.Read(seed); err != nil {
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.run-failed", err.Error(), "make the operating system random source available"))
				break
			}
			ctx, cancel := context.WithDeadline(context.Background(), r.deadline)
			drawn, _, err := r.worker(ctx, ucWorkerRequest{Mode: "draw", Check: r.check, Seed: hex.EncodeToString(seed)}, false)
			cancel()
			if err != nil {
				if !time.Now().Before(r.deadline) {
					break
				}
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.run-failed", err.Error(), "make the input generator executable"))
				break
			}
			if len(drawn.Diagnostics) > 0 {
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, workerDiagnostics(drawn.Diagnostics)...)
				break
			}
			if drawn.Rejected {
				result.Rejected++
				continue
			}
			input, err := hostInput(drawn.Input)
			if err != nil {
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.input-codec-failed", err.Error(), "declare a replayable model codec for every input"))
				break
			}
			hash, err := evidence.InputHash(input)
			if err != nil {
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.input-codec-failed", err.Error(), "use finite canonical model inputs"))
				break
			}
			prior := len(result.Cases)
			if !evaluate(input, "recorded", r.check+".r-"+hash[:8], "coverage", false) {
				break
			}
			if len(result.Cases) == prior {
				continue
			}
			candidate := result.Cases[len(result.Cases)-1]
			if candidate.Rejected {
				continue
			}
			if !candidate.Passed {
				trial := *r
				trial.trial = true
				shrunk := evidence.Shrink(candidate, &trial, time.Until(r.deadline))
				result.Diagnostics = append(result.Diagnostics, shrunk.Diagnostics...)
				smallest := shrunk.Result
				smallest.Form = "recorded"
				smallest.Reason = "counterexample"
				smallest.Id = r.check + ".r-" + smallest.InputHash[:8]
				if plan.Record {
					reproduction := *r
					reproduction.deadline = time.Time{}
					reproduction.trial = false
					confirmed := reproduction.Evaluate(smallest.Input)
					if len(confirmed.Diagnostics) > 0 {
						result.Diagnostics = append(result.Diagnostics, confirmed.Diagnostics...)
					} else if confirmed.Result == nil || confirmed.Result.Passed || confirmed.Result.Rejected {
						result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.unreproducible-counterexample", "the frozen counterexample did not fail when replayed", "remove nondeterministic state or declare the missing input before recording"))
					} else {
						smallest = *confirmed.Result
						smallest.Form = "recorded"
						smallest.Reason = "counterexample"
						smallest.Id = r.check + ".r-" + smallest.InputHash[:8]
					}
				}
				result.Cases[len(result.Cases)-1] = smallest
				break
			}
		}
		eligible := []lang.CaseResult{}
		for _, candidate := range result.Cases {
			if !candidate.Rejected {
				eligible = append(eligible, candidate)
			}
		}
		reduction := evidence.Reduce(eligible, 64)
		if len(reduction.Diagnostics) > 0 {
			result.Passed = false
			result.Diagnostics = append(result.Diagnostics, reduction.Diagnostics...)
		}
	}
	for _, candidate := range result.Cases {
		if !candidate.Passed && !candidate.Rejected {
			result.Diagnostics = append(result.Diagnostics, candidate.Diagnostics...)
			result.Differences = append(result.Differences, candidate.Differences...)
		}
	}
	result.CodeTotal = r.codeTotal
	result.BranchTotal = r.branchTotal
	result.ErrorTotal = r.errorTotal
	if result.Accepted == 0 {
		result.Passed = false
		result.Diagnostics = append(result.Diagnostics, runDiagnostic("lang.golang.no-effective-input", "the selected check has no effective candidate execution", "declare a replay case or use a sufficient exploration budget"))
	}
	if node := plan.Graph.Node(r.check); node != nil && len(node.Attrs["within"]) > 0 && len(r.durations) > 0 {
		budget, err := time.ParseDuration(node.Attrs["within"][0])
		if err == nil {
			slices.Sort(r.durations)
			at := (95*len(r.durations)+99)/100 - 1
			if r.durations[at] > budget {
				result.Passed = false
				result.Diagnostics = append(result.Diagnostics, measuredDiagnostic(*node, "lang.golang.within-exceeded", fmt.Sprintf("the 95th percentile %s exceeds within %s", r.durations[at], budget), "make the measured business call satisfy its declared budget"))
			}
		}
	}
	slices.Sort(result.Observations)
	result.Observations = slices.Compact(result.Observations)
	return result
}
func (r *measuredRunner) Evaluate(input lang.Input) lang.CandidateEvaluation {
	out := lang.CandidateEvaluation{Diagnostics: []diag.Diagnostic{}}
	if !r.deadline.IsZero() && !time.Now().Before(r.deadline) {
		out.Exhausted = true
		return out
	}
	normalized, err := evidence.NormalizeInput(input)
	if err != nil {
		if r.trial {
			out.Result = &lang.CaseResult{Check: r.check, Input: input, Rejected: true}
			return out
		}
		out.Diagnostics = append(out.Diagnostics, runDiagnostic("lang.golang.input-codec-failed", err.Error(), "supply canonical finite model values"))
		return out
	}
	wire, err := wireInput(normalized)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, runDiagnostic("lang.golang.input-codec-failed", err.Error(), "make frozen inputs JSON serializable"))
		return out
	}
	ctx := context.Background()
	var cancel context.CancelFunc
	if !r.deadline.IsZero() {
		ctx, cancel = context.WithDeadline(ctx, r.deadline)
	} else {
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	}
	defer cancel()
	request := ucWorkerRequest{Check: r.check, Mode: "prepare", Input: wire, Trial: r.trial}
	prepared, before, err := r.worker(ctx, request, true)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !r.deadline.IsZero() {
			out.Exhausted = true
			return out
		}
		out.Diagnostics = append(out.Diagnostics, runDiagnostic("lang.golang.run-failed", err.Error(), "make candidate reconstruction finish and emit coverage"))
		return out
	}
	if prepared.Rejected && r.trial {
		out.Result = &lang.CaseResult{Check: r.check, Input: normalized, Rejected: true}
		return out
	}
	if len(prepared.Diagnostics) > 0 {
		out.Diagnostics = append(out.Diagnostics, workerDiagnostics(prepared.Diagnostics)...)
		return out
	}
	request.Mode = "execute"
	executed, after, err := r.worker(ctx, request, true)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !r.deadline.IsZero() {
			out.Exhausted = true
			return out
		}
		out.Diagnostics = append(out.Diagnostics, runDiagnostic("lang.golang.run-failed", err.Error(), "make the candidate execute and emit its complete report"))
		return out
	}
	if len(executed.Diagnostics) > 0 {
		out.Diagnostics = append(out.Diagnostics, workerDiagnostics(executed.Diagnostics)...)
		return out
	}
	hash, _ := evidence.InputHash(normalized)
	candidate := lang.CaseResult{Check: r.check, Id: r.check + ".r-" + hash[:8], Form: "recorded", Input: normalized, InputHash: hash, Passed: executed.Result.Passed, Rejected: executed.Rejected, Coverage: []lang.Coverage{}, Trace: []lang.TraceEvent{}, Observations: executed.Result.Observations, Differences: executed.Result.Differences, Diagnostics: workerDiagnostics(executed.Result.Diagnostics)}
	if !candidate.Rejected {
		points, total, err := r.program.coverage(r.check, before, after)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, runDiagnostic("lang.golang.coverage-failed", err.Error(), "produce complete atomic coverage with original source mappings"))
			return out
		}
		r.codeTotal = max(r.codeTotal, total)
		candidate.Coverage = points
		observed, err := r.program.trace(after + ".trace")
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, runDiagnostic("lang.golang.trace-failed", err.Error(), "produce a valid Go runtime trace for the candidate"))
			return out
		}
		candidate.Trace = observed.events
		design, branches, errorTotal, diagnostics := r.program.designCoverage(r.check, observed)
		r.branchTotal = max(r.branchTotal, branches)
		r.errorTotal = max(r.errorTotal, errorTotal)
		candidate.Coverage = append(candidate.Coverage, design...)
		candidate.Diagnostics = append(candidate.Diagnostics, diagnostics...)
		if len(diagnostics) > 0 {
			candidate.Passed = false
		}
		r.durations = append(r.durations, executed.Durations...)
	}
	out.Result = &candidate
	return out
}
func (r *measuredRunner) worker(ctx context.Context, request ucWorkerRequest, profile bool) (ucWorkerResponse, string, error) {
	result := ucWorkerResponse{}
	dir, err := os.MkdirTemp(r.program.temp, "candidate-")
	if err != nil {
		return result, "", err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return result, "", err
	}
	requestFile, responseFile := filepath.Join(dir, "request.json"), filepath.Join(dir, "response.json")
	if err = os.WriteFile(requestFile, data, 0600); err != nil {
		return result, "", err
	}
	coverageFile := filepath.Join(dir, "coverage.txt")
	args := []string{"-test.run=^TestUnicodeContracts$", "-test.count=1"}
	if profile {
		args = append(args, "-test.coverprofile="+coverageFile)
	}
	if request.Mode == "execute" {
		args = append(args, "-test.trace="+coverageFile+".trace")
	}
	cmd := exec.CommandContext(ctx, r.binary, args...)
	cmd.Dir = r.program.root
	cmd.Env = append(os.Environ(), "UNICODE_CONTRACT_WORKER_REQUEST="+requestFile, "UNICODE_CONTRACT_WORKER_RESPONSE="+responseFile, "UNICODE_CONTRACT_RESULTS=", "UNICODE_CONTRACT_EXPLORE=")
	output, runErr := cmd.CombinedOutput()
	data, readErr := os.ReadFile(responseFile)
	if readErr != nil {
		return result, coverageFile, fmt.Errorf("candidate returned no report: %w: %v: %s", readErr, runErr, output)
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return result, coverageFile, fmt.Errorf("invalid candidate report: %w", err)
	}
	if runErr != nil && len(result.Diagnostics) == 0 && result.Result.Passed && !result.Rejected {
		return result, coverageFile, fmt.Errorf("candidate driver failed outside its reported result: %w: %s", runErr, output)
	}
	return result, coverageFile, nil
}
func hostInput(input ucWireInput) (lang.Input, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return lang.Input{}, err
	}
	var out lang.Input
	err = json.Unmarshal(data, &out)
	if err != nil {
		return out, err
	}
	return evidence.NormalizeInput(out)
}
func wireInput(input lang.Input) (ucWireInput, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return ucWireInput{}, err
	}
	var out ucWireInput
	err = json.Unmarshal(data, &out)
	return out, err
}
func workerDiagnostics(input []ucDiagnostic) []diag.Diagnostic {
	out := make([]diag.Diagnostic, 0, len(input))
	for _, d := range input {
		out = append(out, diag.Diagnostic{Code: d.Code, Subject: d.Subject, Position: diag.Position{Path: d.Position.Path, Line: d.Position.Line, Column: d.Position.Column}, Message: d.Message, Fix: d.Fix})
	}
	return out
}
func isMeasuredEnvironment(code string) bool {
	switch code {
	case "lang.golang.no-effective-input", "lang.golang.derivation-failed", "lang.golang.input-codec-failed", "lang.golang.input-outside-domain", "lang.golang.run-failed", "lang.golang.coverage-failed", "lang.golang.trace-failed", "lang.golang.trace-instrumentation-failed", "lang.golang.trace-missing", "lang.golang.observation-failed", "lang.golang.adapter-failed", "lang.golang.unsupported-contract":
		return true
	}
	return false
}

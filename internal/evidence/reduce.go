package evidence

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
)

// Reduction retains required cases and the complete measured coverage union.
//
//uc:realizes evidence.Reduction
type Reduction struct {
	Cases       []lang.CaseResult `json:"cases"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

func diagnostic(code, subject, cause, fix string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Subject: subject, Position: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}, Message: cause, Fix: fix}
}

func caseLess(a, b lang.CaseResult) int {
	return cmp.Or(cmp.Compare(a.InputHash, b.InputHash), cmp.Compare(a.Id, b.Id), bytes.Compare(canonical(a), canonical(b)))
}
func coverageSet(cases []lang.CaseResult) map[string]bool {
	out := map[string]bool{}
	for _, result := range cases {
		for _, point := range result.Coverage {
			out[string(canonical(point))] = true
		}
	}
	return out
}
func unionInto(target map[string]bool, result lang.CaseResult) {
	for _, point := range result.Coverage {
		target[string(canonical(point))] = true
	}
}

// Reduce is deterministic. The caller supplies cases of exactly one check;
// recorded cases share its cap, while declared cases never consume it.
//
//uc:realizes evidence.reduce
func Reduce(cases []lang.CaseResult, limit int) Reduction {
	out := Reduction{Cases: []lang.CaseResult{}, Diagnostics: []diag.Diagnostic{}}
	fail := func(code, cause, fix string) Reduction {
		out.Diagnostics = append(out.Diagnostics, diagnostic(code, "", cause, fix))
		return out
	}
	if limit < 0 {
		return fail("evidence.coverage-limit", "the recorded-case limit is negative", "give a nonnegative recorded-case limit")
	}
	unique := map[string]lang.CaseResult{}
	check := ""
	for _, input := range cases {
		result, err := cloneCase(input)
		if err != nil {
			return fail("evidence.invalid-input", err.Error(), "provide a complete frozen input and its matching canonical digest")
		}
		if result.Rejected {
			return fail("evidence.invalid-input", "a rejected candidate is not executable coverage evidence", "exclude rejected candidates before reducing recorded cases")
		}
		if check != "" && check != result.Check {
			return fail("evidence.invalid-input", "coverage reduction mixes checks with independent case limits", "reduce each check's cases separately")
		}
		check = result.Check
		unique[string(canonical(result))] = result
	}
	ordered := slices.Collect(maps.Values(unique))
	slices.SortFunc(ordered, caseLess)
	required, optional := []lang.CaseResult{}, []lang.CaseResult{}
	for _, result := range ordered {
		if result.Form == "declared" || result.Reason == "counterexample" || !result.Passed {
			required = append(required, result)
		} else {
			optional = append(optional, result)
		}
	}
	retained := slices.Clone(required)
	covered := coverageSet(retained)
	universe := coverageSet(ordered)
	for len(covered) < len(universe) {
		best, gain := -1, 0
		for index, result := range optional {
			extra := map[string]bool{}
			for _, point := range result.Coverage {
				key := string(canonical(point))
				if !covered[key] {
					extra[key] = true
				}
			}
			if len(extra) > gain {
				best, gain = index, len(extra)
			}
		}
		if best < 0 {
			return fail("evidence.invalid-input", "the measured coverage union cannot be reconstructed from its cases", "provide complete per-input coverage points")
		}
		next := optional[best]
		retained = append(retained, next)
		unionInto(covered, next)
		optional = slices.Delete(optional, best, best+1)
	}
	for index := len(retained) - 1; index >= len(required); index-- {
		without := append(slices.Clone(retained[:index]), retained[index+1:]...)
		if len(coverageSet(without)) == len(universe) {
			retained = without
		}
	}
	recorded := 0
	for _, result := range retained {
		if result.Form != "declared" {
			recorded++
		}
	}
	if recorded > limit {
		return fail("evidence.coverage-limit", fmt.Sprintf("the retained evidence needs %d recorded cases but the limit is %d", recorded, limit), "raise the limit or resolve retained counterexamples; the old corpus has not been replaced")
	}
	slices.SortFunc(retained, caseLess)
	out.Cases = append([]lang.CaseResult{}, retained...)
	return out
}

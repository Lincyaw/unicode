package evidence

import (
	"encoding/base64"
	"iter"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
)

// Shrunk contains only the last confirmed counterexample, not trial executions.
//
//uc:realizes evidence.Shrunk
type Shrunk struct {
	Result      lang.CaseResult   `json:"result"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// Shrink accepts only an executed, non-rejected failure. Runner boundaries
// reconstruct each proposed input independently; exhausted budgets keep best.
//
//uc:realizes evidence.shrink
func Shrink(initial lang.CaseResult, runner lang.CandidateRunner, budget time.Duration) Shrunk {
	out := Shrunk{Result: initial, Diagnostics: []diag.Diagnostic{}}
	best, err := cloneCase(initial, false)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, diagnostic("evidence.invalid-input", initial.Check, err.Error(), "start shrinking from a replayable confirmed failure"))
		return out
	}
	out.Result = best
	if best.Passed || best.Rejected {
		out.Diagnostics = append(out.Diagnostics, diagnostic("evidence.shrink-failed", best.Check, "the initial result is not a confirmed failure", "only shrink a failed, non-rejected candidate"))
		return out
	}
	if budget <= 0 {
		return out
	}
	if runner == nil {
		out.Diagnostics = append(out.Diagnostics, diagnostic("evidence.shrink-failed", best.Check, "the candidate execution boundary is unavailable", "provide a runner that rebuilds and executes frozen inputs"))
		return out
	}
	until := time.Now().Add(budget)
	tried := map[string]bool{best.InputHash: true}
	for {
		improved := false
		names := make([]string, 0, len(best.Input.Values))
		for name := range best.Input.Values {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			for value := range simpler(best.Input.Values[name]) {
				if !time.Now().Before(until) {
					out.Result = best
					return out
				}
				proposal, _ := NormalizeInput(best.Input)
				proposal.Values[name] = value
				frozen, problem := NormalizeInput(proposal)
				if problem != nil {
					continue
				}
				key := hash(frozen.Values)
				if tried[key] {
					continue
				}
				tried[key] = true
				request, _ := NormalizeInput(frozen)
				evaluated := runner.Evaluate(request)
				if len(evaluated.Diagnostics) > 0 {
					out.Diagnostics = append(out.Diagnostics, evaluated.Diagnostics...)
					out.Result = best
					return out
				}
				if evaluated.Exhausted {
					if evaluated.Result != nil {
						out.Diagnostics = append(out.Diagnostics, diagnostic("evidence.shrink-failed", best.Check, "an exhausted candidate returned an execution result", "return exhaustion without a fabricated candidate result"))
					}
					out.Result = best
					return out
				}
				if evaluated.Result == nil {
					out.Diagnostics = append(out.Diagnostics, diagnostic("evidence.shrink-failed", best.Check, "the runner returned neither an execution result nor exhaustion", "return the isolated candidate's real result"))
					out.Result = best
					return out
				}
				result, problem := cloneCase(*evaluated.Result, false)
				if problem != nil || result.InputHash != key || result.Check != best.Check {
					cause := "the runner returned a different or invalid frozen input"
					if problem != nil {
						cause = problem.Error()
					}
					out.Diagnostics = append(out.Diagnostics, diagnostic("evidence.shrink-failed", best.Check, cause, "return the executed candidate's complete, matching model input"))
					out.Result = best
					return out
				}
				if result.Rejected || result.Passed {
					continue
				}
				result.Form, result.Reason = "recorded", "counterexample"
				result.Id = result.Check + ".r-" + result.InputHash[:8]
				best = result
				improved = true
				break
			}
			if improved {
				break
			}
		}
		if !improved {
			out.Result = best
			return out
		}
	}
}

func simpler(value lang.Value) iter.Seq[lang.Value] {
	return func(yield func(lang.Value) bool) {
		if value.Kind != "none" && !yield(lang.Value{Kind: "none"}) {
			return
		}
		addText := func(text string) bool { copy := value; copy.Text = text; return yield(copy) }
		switch value.Kind {
		case "bool":
			if value.Text == "true" {
				addText("false")
			}
		case "int":
			n, _ := new(big.Int).SetString(value.Text, 10)
			for _, candidate := range []*big.Int{big.NewInt(0), new(big.Int).Quo(n, big.NewInt(2)), new(big.Int).Sub(n, big.NewInt(int64(n.Sign())))} {
				if new(big.Int).Abs(candidate).Cmp(new(big.Int).Abs(n)) < 0 && !addText(candidate.String()) {
					return
				}
			}
		case "float":
			n, _ := new(big.Rat).SetString(value.Text)
			if !addText("0") {
				return
			}
			if n.Sign() != 0 {
				half := new(big.Rat).Quo(n, big.NewRat(2, 1))
				places := 0
				if dot := strings.IndexByte(value.Text, '.'); dot >= 0 {
					places = len(value.Text) - dot - 1
				}
				addText(half.FloatString(places + 1))
			}
		case "duration":
			duration, _ := time.ParseDuration(value.Text)
			for _, span := range []time.Duration{0, duration / 2} {
				if !addText(span.String()) {
					return
				}
			}
		case "time":
			stamp, _ := time.Parse(time.RFC3339Nano, value.Text)
			if !addText(time.Unix(0, 0).UTC().Format(time.RFC3339Nano)) {
				return
			}
			nanos := new(big.Int).Add(new(big.Int).Mul(big.NewInt(stamp.Unix()), big.NewInt(1e9)), big.NewInt(int64(stamp.Nanosecond())))
			nanos.Quo(nanos, big.NewInt(2))
			seconds, remainder := new(big.Int), new(big.Int)
			seconds.QuoRem(nanos, big.NewInt(1e9), remainder)
			addText(time.Unix(seconds.Int64(), remainder.Int64()).UTC().Format(time.RFC3339Nano))
		case "string":
			for text := range smallerSlice([]rune(value.Text)) {
				if !addText(string(text)) {
					return
				}
			}
		case "stringBytes", "bytes":
			data, _ := base64.StdEncoding.DecodeString(value.Text)
			for bytes := range smallerSlice(data) {
				if !addText(base64.StdEncoding.EncodeToString(bytes)) {
					return
				}
			}
		case "list", "set":
			for items := range smallerSlice(value.Items) {
				copy := value
				copy.Items = items
				if !yield(copy) {
					return
				}
			}
			for index, child := range value.Items {
				for small := range simpler(child) {
					copy := value
					copy.Items = slices.Clone(value.Items)
					copy.Items[index] = small
					if !yield(copy) {
						return
					}
				}
			}
		case "encoded":
			if len(value.Items) == 1 {
				for small := range simpler(value.Items[0]) {
					copy := value
					copy.Items = []lang.Value{small}
					if !yield(copy) {
						return
					}
				}
			}
		case "record", "map", "call":
			if value.Kind == "map" {
				for entries := range smallerSlice(value.Entries) {
					copy := value
					copy.Entries = entries
					if !yield(copy) {
						return
					}
				}
			}
			for index, entry := range value.Entries {
				for small := range simpler(entry.Value) {
					copy := value
					copy.Entries = slices.Clone(value.Entries)
					copy.Entries[index].Value = small
					if !yield(copy) {
						return
					}
				}
				if value.Kind == "map" {
					for small := range simpler(entry.Key) {
						copy := value
						copy.Entries = slices.Clone(value.Entries)
						copy.Entries[index].Key = small
						if !yield(copy) {
							return
						}
					}
				}
			}
		}
	}
}

func smallerSlice[T any](values []T) iter.Seq[[]T] {
	return func(yield func([]T) bool) {
		for width := len(values); width > 0; width /= 2 {
			for start := 0; start+width <= len(values); start += width {
				candidate := append(slices.Clone(values[:start]), values[start+width:]...)
				if candidate == nil {
					candidate = []T{}
				}
				if !yield(candidate) {
					return
				}
			}
		}
	}
}

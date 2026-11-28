package monitor

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"
)

// Verdict is the conclusion of a property or flow on one monitored call
// (docs/language.md E26).
//
//uc:realizes monitor.Verdict
type Verdict string

const (
	VerdictPassed         Verdict = "passed"
	VerdictCounterexample Verdict = "counterexample"
	VerdictOutsideDomain  Verdict = "outsideDomain"
	VerdictCheckError     Verdict = "checkError"
)

// Judgment is the judgment of one property or flow on one monitored call: its
// verdict; the clause, limit or precondition that does not hold, such as
// ensure#2, where#1 or requires#1, or changes, raises and call for an
// out-of-frame change, an undeclared error and a panic; the clauses judged; a
// counterexample's frozen, redacted input and its hash; a check error's
// cause; and the within budget, zero without one. Path, Line and Column are
// where the property is declared, the position of its diagnostics.
//
//uc:realizes monitor.Judgment
type Judgment struct {
	Check     string           `json:"check"`
	Verdict   Verdict          `json:"verdict"`
	Clause    string           `json:"clause"`
	Judged    []string         `json:"judged"`
	Input     map[string]Value `json:"input"`
	InputHash string           `json:"inputHash"`
	Message   string           `json:"message"`
	Within    time.Duration    `json:"within"`
	Path      string           `json:"path"`
	Line      int              `json:"line"`
	Column    int              `json:"column"`
}

// JudgedCall is one monitored call in a record period: the design operation
// called, when the call began, how long it took from its start to its return,
// and the judgments of the properties and flows that judged it when sampled.
//
//uc:realizes monitor.JudgedCall
type JudgedCall struct {
	Op        string        `json:"op"`
	At        time.Time     `json:"at"`
	Duration  time.Duration `json:"duration"`
	Judgments []Judgment    `json:"judgments"`
}

// Build is the commit the monitored program was built from, empty without
// one, and whether its working tree had changes, from Go's build information.
//
//uc:realizes monitor.Build
type Build struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

// ClauseCalls is a limit or precondition that did not hold, and on how many
// calls.
//
//uc:realizes monitor.ClauseCalls
type ClauseCalls struct {
	Clause string `json:"clause"`
	Calls  int    `json:"calls"`
}

// ClauseJudged is a clause, and how many times it was judged.
//
//uc:realizes monitor.ClauseJudged
type ClauseJudged struct {
	Clause string `json:"clause"`
	Judged int    `json:"judged"`
}

// Diagnostic is a diagnostic of a run record, with the JSON of
// diag.Diagnostic.
//
//uc:realizes monitor.Diagnostic
type Diagnostic struct {
	Code    string `json:"code"`
	Subject string `json:"subject"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// Case is a counterexample of a runtime record, with the JSON of
// lang.CaseResult: its check, its frozen and redacted input and that input's
// hash, the clause that does not hold and when the call began.
//
//uc:realizes monitor.Case
type Case struct {
	Id           string       `json:"id"`
	Check        string       `json:"check"`
	Form         string       `json:"form"`
	Reason       string       `json:"reason"`
	Input        Input        `json:"input"`
	InputHash    string       `json:"inputHash"`
	Passed       bool         `json:"passed"`
	Rejected     bool         `json:"rejected"`
	Coverage     []struct{}   `json:"coverage"`
	Trace        []struct{}   `json:"trace"`
	Observations []string     `json:"observations"`
	Differences  []string     `json:"differences"`
	Diagnostics  []Diagnostic `json:"diagnostics"`
	Clause       string       `json:"clause"`
	At           time.Time    `json:"at"`
}

// Result is the runtime conclusion of one property or flow over a record
// period, with the JSON of lang.RunResult: accepted counts the applicable
// calls; it adds the calls outside the domain by the limit or precondition
// that failed, how many times each clause was judged, the counterexamples,
// the 95th percentile of the applicable calls' durations when it has a
// within budget, and the traces checked.
//
//uc:realizes monitor.Result
type Result struct {
	Id            string         `json:"id"`
	Passed        bool           `json:"passed"`
	Accepted      int            `json:"accepted"`
	Rejected      int            `json:"rejected"`
	Observations  []string       `json:"observations"`
	Differences   []string       `json:"differences"`
	Diagnostics   []Diagnostic   `json:"diagnostics"`
	Cases         []Case         `json:"cases"`
	OutsideDomain []ClauseCalls  `json:"outsideDomain"`
	Clauses       []ClauseJudged `json:"clauses"`
	P95           time.Duration  `json:"p95"`
	Traces        int            `json:"traces"`
}

// Record is the runtime run record of one record period, with the JSON of
// lang.RunReport: venue runtime, no snapshot and no mode, and from, the
// start of the period, besides at, its end.
//
//uc:realizes monitor.Record
type Record struct {
	Results     []Result     `json:"results"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Version     int          `json:"version"`
	Commit      string       `json:"commit"`
	Dirty       bool         `json:"dirty"`
	At          time.Time    `json:"at"`
	Venue       string       `json:"venue"`
	From        time.Time    `json:"from"`
}

// The clause names of a counterexample that is not a false ensure.
const (
	clauseFrame   = "changes"
	clauseRaises  = "raises"
	clausePanic   = "call"
	venueRuntime  = "runtime"
	formRuntime   = "runtime"
	reasonExample = "counterexample"
)

// Summarize turns the judgments of a record period's monitored calls into
// the runtime run record: one result for each property or flow that judged a
// call, a counterexample with its input, a passing call with none, and each
// check error in the outer diagnostics.
//
//uc:realizes monitor.summarize
func Summarize(calls []JudgedCall, build Build, from, to time.Time) Record {
	record := Record{Results: []Result{}, Diagnostics: []Diagnostic{}, Version: 1, Commit: build.Commit, Dirty: build.Dirty, At: to.UTC(), Venue: venueRuntime, From: from.UTC()}
	type tally struct {
		result    Result
		within    time.Duration
		durations []time.Duration
		outside   map[string]int
		judged    map[string]int
	}
	tallies := map[string]*tally{}
	for _, call := range calls {
		for _, j := range call.Judgments {
			t := tallies[j.Check]
			if t == nil {
				t = &tally{outside: map[string]int{}, judged: map[string]int{}}
				t.result = Result{Id: j.Check, Observations: []string{}, Differences: []string{}, Diagnostics: []Diagnostic{}, Cases: []Case{}}
				tallies[j.Check] = t
			}
			t.within = max(t.within, j.Within)
			for _, clause := range j.Judged {
				t.judged[clause]++
			}
			switch j.Verdict {
			case VerdictPassed, VerdictCounterexample:
				t.result.Accepted++
				t.durations = append(t.durations, call.Duration)
				if j.Verdict == VerdictCounterexample {
					t.result.Cases = append(t.result.Cases, counterexample(j, call.At))
					t.result.Diagnostics = append(t.result.Diagnostics, failure(j))
				}
			case VerdictOutsideDomain:
				t.outside[j.Clause]++
			case VerdictCheckError:
				record.Diagnostics = append(record.Diagnostics, diagnostic(j, "monitor.check-failed",
					"the judgment could not be made: "+j.Message,
					"make the call's values convertible to model values and its observations readable, or correct the judge"))
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(tallies)) {
		t := tallies[id]
		r := t.result
		r.OutsideDomain = []ClauseCalls{}
		for _, clause := range slices.Sorted(maps.Keys(t.outside)) {
			r.OutsideDomain = append(r.OutsideDomain, ClauseCalls{Clause: clause, Calls: t.outside[clause]})
		}
		r.Clauses = []ClauseJudged{}
		for _, clause := range slices.Sorted(maps.Keys(t.judged)) {
			r.Clauses = append(r.Clauses, ClauseJudged{Clause: clause, Judged: t.judged[clause]})
		}
		if t.within > 0 {
			r.P95 = Percentile95(t.durations)
		}
		r.Passed = len(r.Cases) == 0 && (t.within == 0 || r.P95 <= t.within)
		if t.within > 0 && r.P95 > t.within {
			j := Judgment{Check: id, Within: t.within}
			for _, call := range calls {
				for _, candidate := range call.Judgments {
					if candidate.Check == id {
						j.Path, j.Line, j.Column = candidate.Path, candidate.Line, candidate.Column
					}
				}
			}
			r.Diagnostics = append(r.Diagnostics, diagnostic(j, "monitor.within-exceeded",
				"the 95th percentile "+r.P95.String()+" of the applicable calls exceeds within "+t.within.String(),
				"make the operation meet its declared time budget, or correct the budget"))
		}
		slices.SortStableFunc(r.Cases, func(a, b Case) int {
			return cmp.Or(a.At.Compare(b.At), strings.Compare(a.InputHash, b.InputHash), strings.Compare(a.Clause, b.Clause))
		})
		record.Results = append(record.Results, r)
	}
	return record
}

// Percentile95 is the 95th percentile of durations as docs/language.md E16
// takes it: the value at rank ⌈0.95 n⌉ of the sorted durations; zero for
// none.
func Percentile95(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	return sorted[(95*len(sorted)+99)/100-1]
}

// counterexample is the case a counterexample judgment records.
func counterexample(j Judgment, at time.Time) Case {
	values := j.Input
	if values == nil {
		values = map[string]Value{}
	}
	id := j.Check + ".rt-"
	if len(j.InputHash) >= 8 {
		id += j.InputHash[:8]
	}
	return Case{Id: id, Check: j.Check, Form: formRuntime, Reason: reasonExample, Input: Input{Values: values}, InputHash: j.InputHash,
		Coverage: []struct{}{}, Trace: []struct{}{}, Observations: []string{}, Differences: []string{}, Diagnostics: []Diagnostic{failure(j)},
		Clause: j.Clause, At: at.UTC()}
}

// failure is the diagnostic of a counterexample, by what does not hold.
func failure(j Judgment) Diagnostic {
	switch {
	case j.Clause == clauseFrame:
		return diagnostic(j, "monitor.frame-violation", "a real call changed observed state outside its changes",
			"correct the operation, or declare the change in the property's changes")
	case j.Clause == clauseRaises:
		return diagnostic(j, "monitor.undeclared-error", "a real call raised an error its operation does not declare: "+j.Message,
			"declare the error in the operation's raises, or stop raising it")
	case j.Clause == clausePanic:
		return diagnostic(j, "monitor.call-panicked", "a real call panicked: "+j.Message,
			"correct the operation so that it returns or raises a declared error")
	}
	message := "the " + j.Clause + " expression is false on a real call"
	if j.Message != "" {
		message = j.Clause + " cannot be evaluated on a real call: " + j.Message
	}
	return diagnostic(j, "monitor.ensure-failed", message,
		"correct the operation, or the property if the real call shows that it states too much")
}

func diagnostic(j Judgment, code, message, fix string) Diagnostic {
	d := Diagnostic{Code: code, Subject: j.Check, Path: j.Path, Line: j.Line, Column: j.Column, Message: message, Fix: fix}
	if d.Path == "" {
		d.Path = "design/system.uc"
	}
	d.Line, d.Column = max(d.Line, 1), max(d.Column, 1)
	return d
}

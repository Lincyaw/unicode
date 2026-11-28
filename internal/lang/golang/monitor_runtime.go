package golang

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/lincyaw/unicode/monitor"
)

// This file is the part of the contract runtime that only monitor files use.
// unicode gen embeds it, with the parts of the contract runtime it reaches,
// into each monitor file (docs/design.md §6.6), so that a judgment evaluates
// a property's statement exactly as the test predicate does.

// ucMonitorUnit is what a monitor file knows of a property it judges at
// runtime: how to build its contract, the design operation the call clause
// calls, the symbols of the receiver and of the arguments, the Go types of
// the arguments and of the results without the trailing error, whether the
// judgment is deep, and the types labelled sensitive.
type ucMonitorUnit struct {
	Spec      func() *ucContract
	Op        string
	Receiver  string
	Args      []string
	ArgTypes  []reflect.Type
	Results   []reflect.Type
	Deep      bool
	Sensitive []string

	once   sync.Once
	spec   *ucContract
	schema monitor.Schema
}

// contract builds the unit's contract and the schema of its types once.
func (u *ucMonitorUnit) contract() (*ucContract, monitor.Schema) {
	u.once.Do(func() {
		u.spec = u.Spec()
		u.schema = ucMonitorSchema(u.spec, u.Sensitive)
	})
	return u.spec, u.schema
}

// ucMonitorSchema is the type information redaction reads.
func ucMonitorSchema(s *ucContract, sensitive []string) monitor.Schema {
	out := monitor.Schema{Types: map[string]monitor.ModelType{}, Operations: map[string]map[string]string{}}
	for id, ty := range s.Types {
		fields := map[string]string{}
		for _, field := range ucFields(s.Types, id) {
			fields[field.Name] = field.Type
		}
		model := monitor.ModelType{Form: ty.Form, Fields: fields, Sensitive: slices.Contains(sensitive, id)}
		if codec, ok := s.Codecs[id]; ok {
			model.Codec = codec.Representation
		}
		out.Types[id] = model
	}
	for id, op := range s.Operations {
		params := map[string]string{}
		for i, name := range op.ParamNames {
			params[name] = op.Params[i]
		}
		out.Operations[id] = params
	}
	return out
}

// ucMonitorReads tells whether an expression reads a value in unreadable.
func ucMonitorReads(e *ucExpr, unreadable map[string]bool) bool {
	reads := false
	ucWalk(e, func(x *ucExpr) { reads = reads || x.Kind == "name" && unreadable[x.Text] })
	return reads
}

// ucMonitorGuard runs part of a judgment, turning a failure of the judging
// code into the cause of a check error.
func ucMonitorGuard(part func()) (problem string) {
	defer func() {
		if p := recover(); p != nil {
			if issue, ok := p.(ucIssue); ok {
				problem = issue.message
			} else {
				problem = fmt.Sprint(p)
			}
		}
	}()
	part()
	return ""
}

// ucMonitorJudge does, for one monitored call, the part of docs/language.md
// E26 that precedes the call and returns the Finish that does the rest; it
// returns nil when the property does not judge the call, since a limit or a
// precondition reads a value it may not read. A shallow judgment reads the
// arguments before and after the call and its result or error; a deep one
// reads the receiver too, when no other monitored call on it overlaps, and
// checks the frame when none overlaps at all. Named observations are not
// read: whether their adapters are cheap needs the effects of the code.
func ucMonitorJudge(u *ucMonitorUnit, observed monitor.Observed) monitor.Finish {
	s, schema := u.contract()
	judgment := monitor.Judgment{Check: s.ID, Within: s.Within, Path: s.Path, Line: s.Line, Column: s.Column, Judged: []string{}}
	models := map[string]string{}
	for _, symbol := range s.Symbols {
		models[symbol.Name] = symbol.Type
	}
	unreadable := map[string]bool{}
	for _, o := range s.Observers {
		unreadable[o.Name] = true
	}
	natives := map[string]reflect.Value{}
	if u.Receiver != "" {
		if u.Deep && observed.Reads.Receiver {
			natives[u.Receiver] = reflect.ValueOf(observed.Receiver)
		} else {
			unreadable[u.Receiver] = true
		}
	}
	for i, name := range u.Args {
		var value reflect.Value
		if i < len(observed.Args) {
			value = reflect.ValueOf(observed.Args[i])
		}
		if !value.IsValid() && i < len(u.ArgTypes) {
			value = reflect.Zero(u.ArgTypes[i])
		}
		natives[name] = value
	}
	env := &ucEnvironment{spec: s, values: map[string]ucValue{}, old: map[string]ucValue{}, failures: map[string]ucValue{}, once: true}
	for name, value := range natives {
		env.values[name] = ucLazyValue(value, models[name], s.Types)
	}
	settled := func(j monitor.Judgment) monitor.Finish {
		j = ucMonitorInput(s, schema, natives, models, j)
		return func([]any, error, any, monitor.Reads) (monitor.Judgment, bool) { return j, true }
	}
	var finish monitor.Finish
	judged := true
	type root struct {
		name    string
		path    []ucSegment
		value   ucValue
		allowed [][]ucSegment
	}
	var roots []root
	ensures := map[int]bool{}
	failedOld := map[int]string{}
	problem := ucMonitorGuard(func() {
		clause, readable := ucDomain(s, env.values)
		if !readable {
			judged = false
			return
		}
		if clause != "" {
			judgment.Verdict, judgment.Clause = monitor.VerdictOutsideDomain, clause
			finish = settled(judgment)
			return
		}
		for i, e := range s.Requires {
			if ucMonitorReads(e, unreadable) {
				judged = false
				return
			}
			if !ucTruth(env.eval(e), e) {
				judgment.Verdict, judgment.Clause = monitor.VerdictOutsideDomain, "requires#"+strconv.Itoa(i+1)
				finish = settled(judgment)
				return
			}
		}
		for i, e := range s.Ensures {
			if ucMonitorReads(e, unreadable) {
				continue
			}
			ensures[i] = true
			ucWalk(e, func(x *ucExpr) {
				if x.Kind != "old" {
					return
				}
				if problem := ucMonitorGuard(func() { env.old[ucExprKey(x)] = ucDetach(env.eval(x.Children[0])) }); problem != "" && failedOld[i] == "" {
					failedOld[i] = problem
				}
			})
		}
		if !u.Deep || !observed.Reads.State || len(s.Observers) > 0 {
			return
		}
		for _, name := range s.Observe {
			parts := strings.Split(name, ".")
			if name == "none" || unreadable[parts[0]] {
				continue
			}
			var path []ucSegment
			for _, p := range parts[1:] {
				path = append(path, ucSegment{Field: p})
			}
			roots = append(roots, root{name: parts[0], path: path, value: ucMaterialize(ucGetPath(env.values[parts[0]], path, false))})
		}
		for _, e := range s.Changes {
			name, path := env.path(e)
			for i := range roots {
				if roots[i].name == name && ucPrefix(roots[i].path, path) {
					roots[i].allowed = append(roots[i].allowed, path[len(roots[i].path):])
				}
			}
		}
	})
	switch {
	case problem != "":
		judgment.Verdict, judgment.Message = monitor.VerdictCheckError, problem
		return settled(judgment)
	case !judged:
		return nil
	case finish != nil:
		return finish
	}
	return func(results []any, failure error, panicked any, reads monitor.Reads) (monitor.Judgment, bool) {
		j := judgment
		counterexample := func(clause, message string) {
			if j.Clause == "" {
				j.Clause, j.Message = clause, message
			}
		}
		if panicked != nil {
			counterexample("call", fmt.Sprint(panicked))
			j.Verdict = monitor.VerdictCounterexample
			return ucMonitorInput(s, schema, natives, models, j), true
		}
		if u.Receiver != "" && !unreadable[u.Receiver] && !reads.Receiver {
			// Another monitored call on the receiver overlapped this one: what
			// reads the receiver after the call is not judged.
			unreadable[u.Receiver] = true
			for i := range ensures {
				ensures[i] = !ucMonitorReads(s.Ensures[i], unreadable)
			}
			roots = slices.DeleteFunc(roots, func(r root) bool { return r.name == u.Receiver })
		}
		op := s.Operations[u.Op]
		values := make([]reflect.Value, 0, len(results)+1)
		for i, result := range results {
			value := reflect.ValueOf(result)
			if !value.IsValid() && i < len(u.Results) {
				value = reflect.Zero(u.Results[i])
			}
			values = append(values, value)
		}
		if len(op.Raises) > 0 || failure != nil {
			value := reflect.New(reflect.TypeFor[error]()).Elem()
			if failure != nil {
				value.Set(reflect.ValueOf(failure))
			}
			values = append(values, value)
		}
		var out ucOutcome
		if problem := ucMonitorGuard(func() { env.captureCall(op, values, s.Call, &out) }); problem != "" {
			j.Verdict, j.Message = monitor.VerdictCheckError, "the call's result is no model value: "+problem
			return ucMonitorInput(s, schema, natives, models, j), true
		}
		if len(out.Diagnostics) > 0 {
			counterexample("raises", out.Diagnostics[0].Message)
		}
		for i, e := range s.Ensures {
			if !ensures[i] {
				continue
			}
			clause := "ensure#" + strconv.Itoa(i+1)
			j.Judged = append(j.Judged, clause)
			if failedOld[i] != "" {
				counterexample(clause, failedOld[i])
				continue
			}
			holds := false
			if problem := ucMonitorGuard(func() { holds = ucTruth(env.eval(e), e) }); problem != "" {
				counterexample(clause, problem)
			} else if !holds {
				counterexample(clause, "")
			}
		}
		if len(roots) > 0 && reads.State {
			j.Judged = append(j.Judged, "changes")
			var differences []string
			if problem := ucMonitorGuard(func() {
				for _, r := range roots {
					after := ucMaterialize(ucGetPath(env.values[r.name], r.path, false))
					ucDiff(r.value, after, nil, r.name+ucPathLabel(r.path), r.allowed, &differences)
				}
			}); problem != "" {
				j.Verdict, j.Message = monitor.VerdictCheckError, "the frame cannot be read after the call: "+problem
				return ucMonitorInput(s, schema, natives, models, j), true
			}
			if len(differences) > 0 {
				slices.Sort(differences)
				counterexample("changes", strings.Join(differences, "; "))
			}
		}
		j.Verdict = monitor.VerdictPassed
		if j.Clause != "" {
			j.Verdict = monitor.VerdictCounterexample
			j = ucMonitorInput(s, schema, natives, models, j)
		}
		return j, true
	}
}

// ucMonitorInput freezes the readable inputs of a judgment that is not a
// pass, redacts them, and gives the judgment their hash, and their values
// when it is a counterexample. A value that is no model value makes the
// judgment a check error (docs/language.md E26).
func ucMonitorInput(s *ucContract, schema monitor.Schema, natives map[string]reflect.Value, models map[string]string, j monitor.Judgment) monitor.Judgment {
	values := map[string]monitor.Value{}
	problem := ucMonitorGuard(func() {
		for name, native := range natives {
			frozen := ucWireCanonical(ucFreezeValue(native, models[name], s, map[uintptr]bool{}, 0))
			values[name] = monitor.Conceal(ucWireToMonitor(frozen), models[name], schema)
		}
	})
	if problem != "" && j.Verdict != monitor.VerdictCheckError {
		j.Verdict, j.Clause, j.Message = monitor.VerdictCheckError, "", "an input is no model value: "+problem
	}
	j.InputHash = monitor.InputHash(values)
	if j.Verdict == monitor.VerdictCounterexample {
		j.Input = values
	}
	return j
}

// ucMonitorCollect collects a sampled call of the unit's operation: the
// receiver's model value and the call value operation(parameter: value, …),
// redacted (docs/design.md §6.6).
func ucMonitorCollect(u *ucMonitorUnit, receiver any, args []any) (monitor.Collected, error) {
	s, schema := u.contract()
	out := monitor.Collected{}
	models := map[string]string{}
	for _, symbol := range s.Symbols {
		models[symbol.Name] = symbol.Type
	}
	op := s.Operations[u.Op]
	problem := ucMonitorGuard(func() {
		if u.Receiver != "" {
			frozen := ucWireCanonical(ucFreezeValue(reflect.ValueOf(receiver), models[u.Receiver], s, map[uintptr]bool{}, 0))
			value := monitor.Conceal(ucWireToMonitor(frozen), models[u.Receiver], schema)
			out.Receiver = &value
		}
		call := monitor.Value{Kind: "call", Text: u.Op[strings.LastIndexByte(u.Op, '.')+1:], Entries: []monitor.ValueEntry{}}
		for i, name := range op.ParamNames {
			var native reflect.Value
			if i < len(args) {
				native = reflect.ValueOf(args[i])
			}
			if !native.IsValid() && i < len(u.ArgTypes) {
				native = reflect.Zero(u.ArgTypes[i])
			}
			frozen := ucWireCanonical(ucFreezeValue(native, op.Params[i], s, map[uintptr]bool{}, 0))
			call.Entries = append(call.Entries, monitor.ValueEntry{Key: monitor.Value{Kind: "string", Text: name}, Value: ucWireToMonitor(frozen)})
		}
		out.Call = monitor.Conceal(call, "call<"+u.Op+">", schema)
	})
	if problem != "" {
		return out, fmt.Errorf("the call's input is no model value: %s", problem)
	}
	return out, nil
}

// ucWireToMonitor converts a frozen value to the monitor's model value; their
// JSON is the same.
func ucWireToMonitor(w ucWireValue) monitor.Value {
	out := monitor.Value{Kind: w.Kind, Text: w.Text}
	if w.Items != nil {
		out.Items = make([]monitor.Value, 0, len(w.Items))
		for _, item := range w.Items {
			out.Items = append(out.Items, ucWireToMonitor(item))
		}
	}
	if w.Entries != nil {
		out.Entries = make([]monitor.ValueEntry, 0, len(w.Entries))
		for _, entry := range w.Entries {
			out.Entries = append(out.Entries, monitor.ValueEntry{Key: ucWireToMonitor(entry.Key), Value: ucWireToMonitor(entry.Value)})
		}
	}
	return out
}

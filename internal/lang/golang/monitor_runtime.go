package golang

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/lincyaw/unicode/monitor"
)

// This file is the part of the contract runtime that only monitor files use.
// unicode gen embeds it, with the parts of the contract runtime it reaches,
// into each monitor file (docs/design.md §6.6). A judgment runs the same two
// halves of the check as the test predicate, beforeCall and afterCall, around
// the program's own call.
//
// Nothing a judgment writes holds the program's text: an error or a panic is
// named by its Go type, a declared error by its redacted model value, and a
// failure inside the judgment by its diagnostic code or its type, since any
// of their messages may carry a sensitive value.

// ucMonitorUnit is what a monitor file knows of a property it judges at
// runtime: how to build its contract, the design operation the call clause
// calls, the symbols of the receiver and of the arguments, the Go types of
// the arguments and of the results without the trailing error, whether the
// judgment is deep, and the types whose values are sensitive.
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

// ucJudging is one judgment of a monitored call in progress.
type ucJudging struct {
	unit       *ucMonitorUnit
	spec       *ucContract
	schema     monitor.Schema
	live       map[string]reflect.Value
	judgment   monitor.Judgment
	env        *ucEnvironment
	models     map[string]string
	unreadable map[string]bool
	input      map[string]monitor.Value
	prepared   ucPrepared
}

// ucMonitorJudge does, for one monitored call, the part of docs/language.md
// E26 that precedes the call and returns the Finish that does the rest; it
// returns nil when the property does not judge the call, since a limit or a
// precondition reads a value it may not read. A shallow judgment reads the
// arguments and the call's result or error; a deep one reads the receiver
// too, only while no other monitored call on it overlaps, and checks the
// frame then. Named observations are not read: whether their adapters are
// cheap needs the effects of the code.
func ucMonitorJudge(u *ucMonitorUnit, observed monitor.Observed) monitor.Finish {
	s, schema := u.contract()
	j := &ucJudging{unit: u, spec: s, schema: schema, models: map[string]string{}, unreadable: map[string]bool{},
		judgment: monitor.Judgment{Check: s.ID, Within: s.Within, Path: s.Path, Line: s.Line, Column: s.Column, Judged: []string{}},
		env:      &ucEnvironment{spec: s, values: map[string]ucValue{}, old: map[string]ucValue{}, failures: map[string]ucValue{}, once: true}}
	for _, symbol := range s.Symbols {
		j.models[symbol.Name] = symbol.Type
	}
	for _, o := range s.Observers {
		j.unreadable[o.Name] = true
	}
	j.live = j.natives(observed)
	judged, finish := true, monitor.Finish(nil)
	if problem := ucMonitorGuard(func() { judged, finish = j.before(observed.Reads) }); problem != "" {
		j.judgment.Verdict, j.judgment.Message = monitor.VerdictCheckError, "the judgment cannot be made before the call: "+problem
		return j.settled()
	}
	if !judged {
		return nil
	}
	return finish
}

// natives are the values the judgment reads: the arguments, and the
// receiver when the judgment is deep and the receiver may be read now.
func (j *ucJudging) natives(observed monitor.Observed) map[string]reflect.Value {
	u := j.unit
	out := map[string]reflect.Value{}
	if u.Receiver != "" {
		if u.Deep && observed.Reads.Receiver {
			out[u.Receiver] = reflect.ValueOf(observed.Receiver)
		} else {
			j.unreadable[u.Receiver] = true
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
		out[name] = value
	}
	return out
}

// before is the first half of E26: it freezes and redacts the input before
// the call can change it, takes the values it reads as detached model values,
// checks the domain and the preconditions, and does beforeCall. It reports
// false when the property does not judge the call, and a Finish that settles
// the judgment when the call is outside the domain.
func (j *ucJudging) before(reads monitor.Reads) (bool, monitor.Finish) {
	s := j.spec
	j.input = map[string]monitor.Value{}
	for name, native := range j.live {
		frozen := ucWireCanonical(ucFreezeValue(native, j.models[name], s, map[uintptr]bool{}, 0))
		j.input[name] = monitor.Conceal(ucWireToMonitor(frozen), j.models[name], j.schema)
	}
	j.read()
	j.judgment.InputHash = monitor.InputHash(j.input)
	clause, readable := ucDomain(s, j.env.values)
	if !readable {
		return false, nil
	}
	if clause != "" {
		j.judgment.Verdict, j.judgment.Clause = monitor.VerdictOutsideDomain, clause
		return true, j.settled()
	}
	for i, e := range s.Requires {
		if ucMonitorReads(e, j.unreadable) {
			return false, nil
		}
		if !ucTruth(j.env.eval(e), e) {
			j.judgment.Verdict, j.judgment.Clause = monitor.VerdictOutsideDomain, "requires#"+strconv.Itoa(i+1)
			return true, j.settled()
		}
	}
	frame := j.unit.Deep && reads.State && len(s.Observers) == 0
	j.prepared = j.env.beforeCall(func(e *ucExpr) bool { return !ucMonitorReads(e, j.unreadable) }, frame)
	return true, j.after
}

// after is the second half of E26, once the program's call returned or
// panicked: it binds the result or the error, then does afterCall.
func (j *ucJudging) after(results []any, failure error, panicked any, reads monitor.Reads) (monitor.Judgment, bool) {
	if panicked != nil {
		return j.counterexample("call", fmt.Sprintf("the call panicked with a value of type %T", panicked)), true
	}
	if j.unit.Receiver != "" && !j.unreadable[j.unit.Receiver] && !reads.Receiver {
		// Another monitored call on the receiver overlapped this one: nothing
		// that reads the receiver after the call is judged.
		j.unreadable[j.unit.Receiver] = true
		delete(j.env.values, j.unit.Receiver)
		j.prepared.judged = slices.DeleteFunc(j.prepared.judged, func(i int) bool { return ucMonitorReads(j.spec.Ensures[i], j.unreadable) })
		j.prepared.roots = nil
	}
	if !reads.State {
		j.prepared.roots = nil
	}
	op := j.spec.Operations[j.unit.Op]
	raised := ""
	var out ucOutcome
	if problem := ucMonitorGuard(func() {
		j.read()
		j.env.captureCall(op, j.values(results, failure, op), j.spec.Call, &out)
		j.env.result = ucDetach(j.env.result)
		raised = j.raised(failure)
	}); problem != "" {
		j.judgment.Verdict, j.judgment.Message = monitor.VerdictCheckError, "the call's result is no model value: "+problem
		return j.judgment, true
	}
	if len(out.Diagnostics) > 0 {
		return j.counterexample("raises", fmt.Sprintf("the call returned an undeclared error of type %T", failure)), true
	}
	var ensures []ucEnsure
	var differences []string
	if problem := ucMonitorGuard(func() { ensures, differences = j.env.afterCall(j.prepared) }); problem != "" {
		j.judgment.Verdict, j.judgment.Message = monitor.VerdictCheckError, "the frame cannot be read after the call: "+problem
		return j.judgment, true
	}
	clause, message := "", ""
	for _, r := range ensures {
		name := "ensure#" + strconv.Itoa(r.Index+1)
		j.judgment.Judged = append(j.judgment.Judged, name)
		if clause != "" || r.Problem == nil && r.Holds {
			continue
		}
		clause, message = name, "the "+name+" expression is false on a real call"+raised
		if r.Problem != nil {
			message = name + " cannot be evaluated on a real call: " + ucProblemText(r.Problem)
		}
	}
	if len(j.prepared.roots) > 0 {
		j.judgment.Judged = append(j.judgment.Judged, "changes")
		if clause == "" && len(differences) > 0 {
			// The differences name model paths of the frame, never values.
			clause, message = "changes", fmt.Sprintf("a real call changed %d observed values outside its changes", len(differences))
		}
	}
	if clause != "" {
		return j.counterexample(clause, message), true
	}
	j.judgment.Verdict = monitor.VerdictPassed
	return j.judgment, true
}

// read binds each value the judgment may read to its model value, detached
// from the program's objects: an operation an expression calls then runs on a
// copy rebuilt from it, so that, while the judgment reads its receiver, the
// only code of the program that runs on a live object is a codec's encoder
// (docs/design.md §6.6).
func (j *ucJudging) read() {
	for name, native := range j.live {
		if !j.unreadable[name] {
			j.env.values[name] = ucDetach(ucLazyValue(native, j.models[name], j.spec.Types))
		}
	}
}

// values are the call's results as captureCall takes them: the results, each
// of its Go type even when nil, and the error when the operation declares
// errors or one was returned.
func (j *ucJudging) values(results []any, failure error, op ucOperation) []reflect.Value {
	values := make([]reflect.Value, 0, len(results)+1)
	for i, result := range results {
		value := reflect.ValueOf(result)
		if !value.IsValid() && i < len(j.unit.Results) {
			value = reflect.Zero(j.unit.Results[i])
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
	return values
}

// raised describes a declared error the call raised, as the error's
// identity and its redacted model value, or gives "".
func (j *ucJudging) raised(failure error) string {
	if failure == nil {
		return ""
	}
	for _, id := range slices.Sorted(maps.Keys(j.spec.ErrorTypes)) {
		target := reflect.New(j.spec.ErrorTypes[id])
		if !errors.As(failure, target.Interface()) {
			continue
		}
		frozen := ucWireCanonical(ucFreezeValue(target.Elem(), id, j.spec, map[uintptr]bool{}, 0))
		value := monitor.Conceal(ucWireToMonitor(frozen), id, j.schema)
		data, err := json.Marshal(value)
		if err != nil {
			return ", which raised " + id
		}
		return ", which raised " + id + " " + string(data)
	}
	return ""
}

// counterexample settles the judgment as a counterexample of clause, with
// the input frozen before the call.
func (j *ucJudging) counterexample(clause, message string) monitor.Judgment {
	j.judgment.Verdict, j.judgment.Clause, j.judgment.Message, j.judgment.Input = monitor.VerdictCounterexample, clause, message, j.input
	return j.judgment
}

// settled is the Finish of a judgment already made before the call.
func (j *ucJudging) settled() monitor.Finish {
	judgment := j.judgment
	return func([]any, error, any, monitor.Reads) (monitor.Judgment, bool) { return judgment, true }
}

// ucMonitorReads tells whether an expression reads a value in unreadable.
func ucMonitorReads(e *ucExpr, unreadable map[string]bool) bool {
	reads := false
	ucWalk(e, func(x *ucExpr) { reads = reads || x.Kind == "name" && unreadable[x.Text] })
	return reads
}

// ucMonitorGuard runs part of a judgment and names what stopped it, if
// anything, without the program's text.
func ucMonitorGuard(part func()) string {
	if problem := ucRecovered(part); problem != nil {
		return ucProblemText(problem)
	}
	return ""
}

// ucProblemText names a problem by its diagnostic code, or by its Go type
// when it is a panic of the program's code: neither carries the program's
// data.
func ucProblemText(problem any) string {
	if issue, ok := problem.(ucIssue); ok {
		return issue.code
	}
	return fmt.Sprintf("a panic with a value of type %T", problem)
}

// ucMonitorCollect collects a sampled call of the unit's operation: the
// receiver's model value and the call value operation(parameter: value, …),
// redacted (docs/design.md §6.6). The monitor calls it only while the
// receiver may be read.
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
		return out, errors.New("the call's input is no model value: " + problem)
	}
	return out, nil
}

// ucWireToMonitor converts a frozen value to the monitor's model value; the
// frozen value's type is the runtime's own, since test drivers do not import
// the monitor package.
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

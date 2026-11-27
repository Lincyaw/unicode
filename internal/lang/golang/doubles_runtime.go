package golang

import (
	"fmt"
	"reflect"
	"slices"

	"pgregory.net/rapid"
)

// Response scripts are generated before execution. The final response repeats;
// business calls never draw random values and never mutate the saved script.
type ucReplyOperation struct {
	Name, Result string
	Signature    reflect.Type
	Errors       map[string]reflect.Type
}
type ucDoubleFactory struct {
	Operations map[string]ucReplyOperation
	New        func(map[string][]map[string]any) reflect.Value
}

func ucNextReply(script []map[string]any, cursor *int) map[string]any {
	if len(script) == 0 {
		ucFail("lang.golang.input-outside-domain", "a response script must be nonempty", nil)
	}
	i := *cursor
	if i >= len(script) {
		i = len(script) - 1
	}
	if *cursor < len(script) {
		*cursor++
	}
	return script[i]
}

func ucDrawReplies(t *rapid.T, op ucReplyOperation, types map[string]ucType, limits []string, path string, depth int) []map[string]any {
	minSize, maxSize := 1, 5
	for _, lim := range limits {
		var p, kind string
		var low, high int
		if _, err := fmt.Sscanf(lim, "%s %s %d..%d", &p, &kind, &low, &high); err == nil && p == path && kind == "size" {
			minSize, maxSize = max(1, low), high
		}
	}
	if maxSize < minSize {
		ucFail("lang.golang.input-outside-domain", "the response size domain contains no nonempty script", nil)
	}
	n := rapid.IntRange(minSize, maxSize).Draw(t, path+".size")
	script := make([]map[string]any, n)
	ids := slices.Sorted(slices.Values(ucMapKeysReply(op.Errors)))
	for i := range script {
		choice := rapid.IntRange(0, len(ids)).Draw(t, path+".response")
		if choice == 0 {
			var value any
			if op.Result != "" {
				value = ucDraw(t, op.Signature.Out(0), op.Result, types, nil, path+".returns", depth+1).Interface()
			}
			script[i] = map[string]any{"returns": value}
		} else {
			id := ids[choice-1]
			script[i] = map[string]any{"raises": id, "fields": ucDraw(t, op.Errors[id], id, types, nil, path+".fields", depth+1).Interface()}
		}
	}
	return script
}
func ucMapKeysReply(m map[string]reflect.Type) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func ucDrawDouble(t *rapid.T, typ reflect.Type, model string, types map[string]ucType, limits []string, path string, depth int) (reflect.Value, bool) {
	factory := types[model].Double
	if factory == nil {
		return reflect.Value{}, false
	}
	kind, args := ucSplitType(model)
	if kind == "replies" {
		op := factory.Operations[args[0]]
		out := reflect.New(typ).Elem()
		field := out.FieldByName("Replies")
		if !field.IsValid() || !field.CanSet() {
			ucFail("lang.golang.input-outside-domain", "a response carrier has no writable Replies field", nil)
		}
		field.Set(reflect.ValueOf(ucDrawReplies(t, op, types, limits, path, depth)))
		return out, true
	}
	scripts := map[string][]map[string]any{}
	for _, key := range slices.Sorted(slices.Values(ucMapKeysReplyOperations(factory.Operations))) {
		scripts[key] = ucDrawReplies(t, factory.Operations[key], types, nil, path+"."+key, depth)
	}
	value := factory.New(scripts)
	out := reflect.New(typ).Elem()
	out.Set(value)
	return out, true
}
func ucMapKeysReplyOperations(m map[string]ucReplyOperation) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func ucReplyValues(op ucReplyOperation, reply map[string]any, types map[string]ucType) []reflect.Value {
	results := make([]reflect.Value, op.Signature.NumOut())
	for i := range results {
		results[i] = reflect.Zero(op.Signature.Out(i))
	}
	if value, ok := reply["returns"]; ok {
		if len(reply) != 1 {
			ucFail("lang.golang.input-outside-domain", "a successful response has exactly returns", nil)
		}
		if op.Result == "" {
			if value != nil {
				ucFail("lang.golang.input-outside-domain", "a void response returns none", nil)
			}
		} else {
			if value == nil {
				kind, _ := ucSplitType(op.Result)
				if kind != "optional" {
					ucFail("lang.golang.input-outside-domain", "a required response result is none", nil)
				}
			}
			if value != nil {
				results[0] = ucNative(ucDetach(ucNormalize(reflect.ValueOf(value), op.Result, types)), op.Signature.Out(0))
			}
		}
		return results
	}
	id, ok := reply["raises"].(string)
	native, declared := op.Errors[id]
	if !ok || !declared || len(reply) != 2 || reply["fields"] == nil {
		ucFail("lang.golang.input-outside-domain", "a failed response must provide a declared error identity and its fields", nil)
	}
	if len(results) == 0 || !op.Signature.Out(len(results)-1).Implements(reflect.TypeFor[error]()) {
		ucFail("lang.golang.input-outside-domain", "this operation cannot return an error", nil)
	}
	value := ucNative(ucDetach(ucNormalize(reflect.ValueOf(reply["fields"]), id, types)), native)
	target := reflect.New(op.Signature.Out(len(results) - 1)).Elem()
	target.Set(value)
	results[len(results)-1] = target
	return results
}

func ucInstallDoubles(s *ucContract, inputs map[string]reflect.Value) func() {
	var cleanups []reflect.Value
	closed := false
	cleanup := func() {
		if closed {
			return
		}
		closed = true
		var issue any
		for i := len(cleanups) - 1; i >= 0; i-- {
			func() {
				defer func() {
					if p := recover(); p != nil {
						issue = p
					}
				}()
				cleanups[i].Call(nil)
			}()
		}
		if issue != nil {
			ucFail("lang.golang.substitute-failed", fmt.Sprintf("a substitute cleanup panicked: %v", issue), nil)
		}
	}
	defer func() {
		if p := recover(); p != nil {
			cleanup()
			if issue, ok := p.(ucIssue); ok {
				panic(issue)
			}
			ucFail("lang.golang.substitute-failed", fmt.Sprintf("a substitute installer panicked: %v", p), nil)
		}
	}()
	for _, symbol := range s.Symbols {
		kind, args := ucSplitType(symbol.Type)
		if kind != "replies" {
			continue
		}
		id := args[0]
		installer := s.Installers[id]
		factory := s.Types[symbol.Type].Double
		if !installer.IsValid() || factory == nil {
			ucFail("lang.golang.substitute-failed", "the external response script has no installer", nil)
		}
		value := inputs[symbol.Name]
		for value.Kind() == reflect.Pointer {
			value = value.Elem()
		}
		field := value.FieldByName("Replies")
		if !field.IsValid() || !field.CanInterface() {
			ucFail("lang.golang.input-outside-domain", "the external response script has no readable Replies field", nil)
		}
		script, ok := field.Interface().([]map[string]any)
		if !ok || len(script) == 0 {
			ucFail("lang.golang.input-outside-domain", "the external response script is empty or malformed", nil)
		}
		op := factory.Operations[id]
		for _, reply := range script {
			ucReplyValues(op, reply, s.Types)
		}
		cursor := 0
		replacement := reflect.MakeFunc(installer.Type().In(0), func([]reflect.Value) []reflect.Value { return ucReplyValues(op, ucNextReply(script, &cursor), s.Types) })
		values := installer.Call([]reflect.Value{replacement})
		if len(values) != 1 || values[0].Kind() != reflect.Func || values[0].IsNil() {
			ucFail("lang.golang.substitute-failed", "a substitute installer returned no cleanup function", nil)
		}
		cleanups = append(cleanups, values[0])
	}
	return cleanup
}

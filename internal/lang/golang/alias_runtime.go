package golang

import (
	"reflect"
	"strconv"
)

type ucMutableSpan struct {
	start, end uintptr
	path       string
}

// Model inputs are value trees. Detect visible mutable sharing that rebuilding
// their separate values cannot preserve; a combined codec is the explicit
// boundary for identity-sensitive state.
func ucCheckInputAliases(s *ucContract, inputs map[string]reflect.Value) {
	seen := map[uintptr]string{}
	var spans []ucMutableSpan
	duplicate := func(prior, path string) {
		ucFail("lang.golang.input-codec-failed", "inputs "+prior+" and "+path+" share mutable state that a value tree cannot replay; combine the related state under one declared codec", nil)
	}
	register := func(pointer uintptr, path string) {
		if pointer == 0 {
			return
		}
		if prior, ok := seen[pointer]; ok {
			duplicate(prior, path)
		}
		seen[pointer] = path
	}
	var visit func(reflect.Value, string, string, int)
	visit = func(value reflect.Value, model, path string, depth int) {
		if depth > 128 {
			ucFail("lang.golang.input-codec-failed", "the input alias check exceeds the finite model depth", nil)
		}
		kind, args := ucSplitType(model)
		if kind == "optional" {
			if !value.IsValid() || ((value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil()) {
				return
			}
			model = args[0]
			kind, args = ucSplitType(model)
		}
		for value.IsValid() && value.Kind() == reflect.Interface {
			if value.IsNil() {
				return
			}
			value = value.Elem()
		}
		if value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() && value.Type().Elem().Size() > 0 {
			register(value.Pointer(), path)
		}
		if _, ok := s.Codecs[model]; ok {
			return
		}
		value = ucDeref(value)
		if !value.IsValid() {
			return
		}
		switch kind {
		case "list", "bytes":
			if value.Kind() != reflect.Slice {
				return
			}
			if value.Len() > 0 && value.Type().Elem().Size() > 0 {
				start := value.Pointer()
				end := start + uintptr(value.Len())*value.Type().Elem().Size()
				for _, span := range spans {
					if start < span.end && span.start < end {
						duplicate(span.path, path)
					}
				}
				spans = append(spans, ucMutableSpan{start, end, path})
			}
			if kind == "list" {
				for i := 0; i < value.Len(); i++ {
					visit(value.Index(i), args[0], path+"["+strconv.Itoa(i)+"]", depth+1)
				}
			}
		case "map", "set":
			if value.IsNil() {
				return
			}
			register(value.Pointer(), path)
			for _, key := range value.MapKeys() {
				if kind == "map" {
					visit(value.MapIndex(key), args[1], path+"[]", depth+1)
				}
			}
		case "call":
			operation, parameters := ucCallParts(value)
			for _, op := range s.Types[model].Calls {
				if op.Name == operation {
					for i, name := range op.ParamNames {
						visit(parameters[name], op.Params[i], path+"."+name, depth+1)
					}
				}
			}
		default:
			ty := s.Types[model]
			if ty.Form == "interface" || kind == "time" || kind == "duration" || kind == "replies" {
				return
			}
			if value.Kind() == reflect.Struct {
				for _, field := range ucFields(s.Types, model) {
					visit(ucOwnedField(value, s.Types, model, field, false), field.Type, path+"."+field.Name, depth+1)
				}
			}
		}
	}
	for _, symbol := range s.Symbols {
		visit(inputs[symbol.Name], symbol.Type, symbol.Name, 0)
	}
}

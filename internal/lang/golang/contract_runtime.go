package golang

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// This runtime is also embedded verbatim into generated test files. It has no
// dependency on unicode packages, so generated consumers need no internal imports.
type ucExpr struct {
	Kind, Text   string
	Children     []*ucExpr
	Line, Column int
}
type ucField struct{ Name, GoName, Type string }
type ucType struct {
	Form   string
	Fields []ucField
	Values []string
	Parent string
	Calls  []ucOperation
	Double *ucDoubleFactory `json:"-"`
}
type ucObserver struct {
	Name, Type string
	Inputs     []string
	Read       reflect.Value
}
type ucOperation struct {
	Name, GoName, Owner, Result string
	Raises                      []string
	Params                      []string
	ParamNames                  []string
	ParamTypes                  []reflect.Type `json:"-"`
	receiver                    ucValue
	Pure                        bool
	Function                    reflect.Value
}
type ucSymbol struct{ Name, Type string }
type ucContract struct {
	ID, Path                   string
	Line, Column               int
	Symbols                    []ucSymbol
	Requires, Ensures          []*ucExpr
	Observe                    []string
	Changes                    []*ucExpr
	Call                       *ucExpr
	Sequence, SequenceReceiver string
	Within                     time.Duration
	Types                      map[string]ucType
	Operations                 map[string]ucOperation
	Observers                  []ucObserver
	ErrorTypes                 map[string]reflect.Type
	Installers                 map[string]reflect.Value `json:"-"`
}
type ucDiagnostic struct {
	Code, Subject string
	Position      ucPosition
	Message, Fix  string
}
type ucPosition struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}
type ucRunResult struct {
	Id             string         `json:"id"`
	Passed         bool           `json:"passed"`
	Accepted       int            `json:"accepted"`
	Rejected       int            `json:"rejected"`
	Observations   []string       `json:"observations"`
	Differences    []string       `json:"differences"`
	Diagnostics    []ucDiagnostic `json:"diagnostics"`
	Infrastructure bool           `json:"infrastructure,omitempty"`
	durations      []time.Duration
}
type ucValue struct {
	Type   string
	Data   any
	Native reflect.Value
	Shape  reflect.Type
	Schema map[string]ucType
}
type ucEntry struct{ Key, Value ucValue }
type ucMap []ucEntry
type ucEnvironment struct {
	spec     *ucContract
	values   map[string]ucValue
	old      map[string]ucValue
	result   ucValue
	failure  error
	failures map[string]ucValue
}
type ucOutcome struct {
	Rejected       bool
	Executed       bool
	Infrastructure bool
	Diagnostics    []ucDiagnostic
	Differences    []string
	Duration       time.Duration
}
type ucIssue struct {
	code, message string
	expr          *ucExpr
}

func (e ucIssue) Error() string                 { return e.message }
func ucFail(code, message string, expr *ucExpr) { panic(ucIssue{code, message, expr}) }
func ucDiagnosticOf(s *ucContract, issue ucIssue) ucDiagnostic {
	line, column := s.Line, s.Column
	if issue.expr != nil {
		if issue.expr.Line > 0 {
			line = issue.expr.Line
		}
		if issue.expr.Column > 0 {
			column = issue.expr.Column
		}
	}
	return ucDiagnostic{Code: issue.code, Subject: s.ID, Position: ucPosition{s.Path, line, column}, Message: issue.message, Fix: "correct the input, adapter or expression at this position and rerun the contract"}
}
func ucSplitType(t string) (string, []string) {
	i := strings.IndexByte(t, '<')
	if i < 0 {
		return t, nil
	}
	body := t[i+1 : len(t)-1]
	depth, start := 0, 0
	var args []string
	for j, c := range body {
		if c == '<' {
			depth++
		}
		if c == '>' {
			depth--
		}
		if c == ',' && depth == 0 {
			args = append(args, strings.TrimSpace(body[start:j]))
			start = j + 1
		}
	}
	return t[:i], append(args, strings.TrimSpace(body[start:]))
}
func ucBaseType(t string) string {
	for {
		kind, args := ucSplitType(t)
		if kind != "optional" {
			return t
		}
		t = args[0]
	}
}
func ucFields(types map[string]ucType, t string) []ucField {
	ty := types[t]
	var out []ucField
	if ty.Parent != "" {
		out = append(out, ucFields(types, ty.Parent)...)
	}
	return append(out, ty.Fields...)
}
func ucDeref(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}
func ucNormalize(v reflect.Value, t string, types map[string]ucType) ucValue {
	return ucNormalizeAt(v, t, types, map[uintptr]bool{}, 0)
}
func ucNormalizeAt(v reflect.Value, t string, types map[string]ucType, seen map[uintptr]bool, depth int) ucValue {
	if depth > 128 {
		ucFail("lang.golang.observation-failed", "the model value exceeds the finite snapshot depth", nil)
	}
	out := ucValue{Type: t, Native: v, Schema: types}
	if v.IsValid() {
		out.Shape = v.Type()
	}
	kind, args := ucSplitType(t)
	if kind == "optional" {
		if !v.IsValid() || ((v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()) {
			return out
		}
		n := ucNormalizeAt(v, args[0], types, seen, depth+1)
		n.Type = t
		return n
	}
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			ucFail("lang.golang.observation-failed", "a required model value is nil", nil)
		}
		if v.Kind() == reflect.Pointer {
			ptr := v.Pointer()
			if seen[ptr] {
				ucFail("lang.golang.observation-failed", "a model snapshot contains a reference cycle", nil)
			}
			seen[ptr] = true
			defer delete(seen, ptr)
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		ucFail("lang.golang.observation-failed", "a model value is unavailable", nil)
	}
	switch kind {
	case "bool":
		out.Data = v.Bool()
	case "int":
		if v.Kind() >= reflect.Int && v.Kind() <= reflect.Int64 {
			out.Data = big.NewInt(v.Int())
		} else if v.Kind() >= reflect.Uint && v.Kind() <= reflect.Uint64 || v.Kind() == reflect.Uintptr {
			out.Data = new(big.Int).SetUint64(v.Uint())
		} else {
			ucFail("lang.golang.observation-failed", "the integer adapter returned a non-integer value", nil)
		}
	case "float":
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			ucFail("lang.golang.expression-failed", "a model float is not finite", nil)
		}
		out.Data = f
	case "string":
		out.Data = v.String()
	case "bytes":
		b := make([]byte, v.Len())
		reflect.Copy(reflect.ValueOf(b), v)
		out.Data = string(b)
	case "time":
		if !v.CanInterface() {
			ucFail("lang.golang.observation-failed", "a time field is unreadable", nil)
		}
		out.Data = v.Interface().(time.Time)
	case "duration":
		out.Data = time.Duration(v.Int())
	case "list":
		xs := make([]ucValue, v.Len())
		for i := range xs {
			xs[i] = ucNormalizeAt(v.Index(i), args[0], types, seen, depth+1)
		}
		out.Data = xs
	case "map", "set":
		var xs ucMap
		for _, key := range v.MapKeys() {
			item := ucEntry{Key: ucNormalizeAt(key, args[0], types, seen, depth+1)}
			if kind == "map" {
				item.Value = ucNormalizeAt(v.MapIndex(key), args[1], types, seen, depth+1)
			}
			xs = append(xs, item)
		}
		slices.SortFunc(xs, func(a, b ucEntry) int { return strings.Compare(ucKey(a.Key), ucKey(b.Key)) })
		out.Data = xs
	case "call":
		name, arguments := ucCallParts(v)
		var selected *ucOperation
		for _, op := range types[t].Calls {
			if op.Name == name {
				copy := op
				selected = &copy
				break
			}
		}
		if selected == nil {
			ucFail("lang.golang.input-outside-domain", "the call value names an operation outside its model type", nil)
		}
		fields := map[string]ucValue{"operation": {Type: "string", Data: name}}
		if len(arguments) != len(selected.Params) {
			ucFail("lang.golang.input-outside-domain", "the call value has the wrong parameter set", nil)
		}
		for i, param := range selected.ParamNames {
			arg, ok := arguments[param]
			if !ok {
				ucFail("lang.golang.input-outside-domain", "the call value is missing parameter "+param, nil)
			}
			fields[param] = ucNormalizeAt(arg, selected.Params[i], types, seen, depth+1)
		}
		out.Data = fields
	default:
		ty, ok := types[t]
		if !ok {
			ucFail("lang.golang.observation-failed", "no model type is available for "+t, nil)
		}
		if ty.Form == "interface" {
			ucFail("lang.golang.unsupported-contract", "interface values require a generated double or a named value observer", nil)
		}
		if ty.Form == "enum" {
			out.Data = v.String()
			break
		}
		fields := map[string]ucValue{}
		for _, field := range ucFields(types, t) {
			f := v.FieldByName(field.GoName)
			if !f.IsValid() {
				ucFail("lang.golang.observation-failed", "the adapter cannot read model field "+t+"."+field.Name, nil)
			}
			fields[field.Name] = ucNormalizeAt(f, field.Type, types, seen, depth+1)
		}
		out.Data = fields
	}
	return out
}

type ucLazy struct{ types map[string]ucType }

func ucLazyValue(v reflect.Value, t string, types map[string]ucType) ucValue {
	value := ucValue{Type: t, Native: v, Data: ucLazy{types}, Schema: types}
	if v.IsValid() {
		value.Shape = v.Type()
	}
	return value
}
func ucDetach(v ucValue) ucValue {
	v = ucMaterialize(v)
	v.Native = reflect.Value{}
	switch data := v.Data.(type) {
	case map[string]ucValue:
		copy := map[string]ucValue{}
		for k, x := range data {
			copy[k] = ucDetach(x)
		}
		v.Data = copy
	case []ucValue:
		copy := make([]ucValue, len(data))
		for i, x := range data {
			copy[i] = ucDetach(x)
		}
		v.Data = copy
	case ucMap:
		copy := make(ucMap, len(data))
		for i, x := range data {
			copy[i] = ucEntry{ucDetach(x.Key), ucDetach(x.Value)}
		}
		v.Data = copy
	}
	return v
}
func ucResultValue(v reflect.Value, t string, types map[string]ucType) (result ucValue) {
	defer func() {
		if p := recover(); p != nil {
			if issue, ok := p.(ucIssue); ok {
				issue.code = "lang.golang.invalid-result"
				panic(issue)
			}
			panic(p)
		}
	}()
	return ucNormalize(v, t, types)
}
func ucMaterialize(v ucValue) ucValue {
	if lazy, ok := v.Data.(ucLazy); ok {
		return ucNormalize(v.Native, v.Type, lazy.types)
	}
	return v
}
func ucNone(v ucValue) bool {
	if _, lazy := v.Data.(ucLazy); lazy {
		return !v.Native.IsValid() || ((v.Native.Kind() == reflect.Pointer || v.Native.Kind() == reflect.Interface) && v.Native.IsNil())
	}
	return v.Data == nil
}
func ucFieldValue(v ucValue, name string) ucValue {
	if lazy, ok := v.Data.(ucLazy); ok {
		model := v.Type
		kind, args := ucSplitType(model)
		if kind == "optional" {
			model = args[0]
		}
		for _, f := range ucFields(lazy.types, model) {
			if f.Name == name {
				native := ucDeref(v.Native)
				if !native.IsValid() {
					ucFail("lang.golang.expression-failed", "a field was read from none", nil)
				}
				field := native.FieldByName(f.GoName)
				if !field.IsValid() {
					ucFail("lang.golang.observation-failed", "the model field is unavailable: "+name, nil)
				}
				return ucLazyValue(field, f.Type, lazy.types)
			}
		}
		ucFail("lang.golang.expression-failed", "the model does not declare field "+name, nil)
	}
	fields, ok := v.Data.(map[string]ucValue)
	if !ok {
		ucFail("lang.golang.expression-failed", "a field was read from a non-record value", nil)
	}
	field, ok := fields[name]
	if !ok {
		ucFail("lang.golang.expression-failed", "the model field is unavailable: "+name, nil)
	}
	return field
}
func ucKey(v ucValue) string {
	v = ucMaterialize(v)
	switch x := v.Data.(type) {
	case *big.Int:
		return "i:" + x.String()
	case string:
		return "s:" + x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return "f:" + strconv.FormatFloat(x, 'g', -1, 64)
	case time.Time:
		return "t:" + x.UTC().Format(time.RFC3339Nano)
	case time.Duration:
		return "d:" + x.String()
	case []ucValue:
		items := make([]string, len(x))
		for i, item := range x {
			items[i] = ucKey(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case ucMap:
		items := make([]string, len(x))
		for i, item := range x {
			items[i] = ucKey(item.Key) + ": " + ucKey(item.Value)
		}
		return "{" + strings.Join(items, ", ") + "}"
	case map[string]ucValue:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, key := range keys {
			items = append(items, key+": "+ucKey(x[key]))
		}
		return "{" + strings.Join(items, ", ") + "}"
	}
	return fmt.Sprint(v.Data)
}
func ucEqual(a, b ucValue) bool {
	a = ucMaterialize(a)
	b = ucMaterialize(b)
	if a.Data == nil || b.Data == nil {
		return a.Data == nil && b.Data == nil
	}
	switch x := a.Data.(type) {
	case *big.Int:
		y, ok := b.Data.(*big.Int)
		return ok && x.Cmp(y) == 0
	case []ucValue:
		y, ok := b.Data.([]ucValue)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !ucEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case ucMap:
		y, ok := b.Data.(ucMap)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !ucEqual(x[i].Key, y[i].Key) || !ucEqual(x[i].Value, y[i].Value) {
				return false
			}
		}
		return true
	case map[string]ucValue:
		y, ok := b.Data.(map[string]ucValue)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !ucEqual(v, w) {
				return false
			}
		}
		return true
	case time.Time:
		y, ok := b.Data.(time.Time)
		return ok && x.Equal(y)
	}
	return reflect.DeepEqual(a.Data, b.Data)
}
func ucTruth(v ucValue, e *ucExpr) bool {
	v = ucMaterialize(v)
	b, ok := v.Data.(bool)
	if !ok {
		ucFail("lang.golang.expression-failed", "the expression did not produce a boolean", e)
	}
	return b
}
func ucBool(b bool) ucValue      { return ucValue{Type: "bool", Data: b} }
func ucExprKey(e *ucExpr) string { b, _ := json.Marshal(e); return string(b) }
func ucName(e *ucExpr) string {
	if e.Kind == "name" {
		return e.Text
	}
	if e.Kind == "field" && len(e.Children) == 1 {
		return ucName(e.Children[0]) + "." + e.Text
	}
	return ""
}
func ucLookup(v ucValue, key ucValue, e *ucExpr) ucValue {
	v = ucMaterialize(v)
	items, ok := v.Data.(ucMap)
	if !ok {
		ucFail("lang.golang.expression-failed", "a map was required but its optional value is none", e)
	}
	for _, item := range items {
		if ucEqual(item.Key, key) {
			return item.Value
		}
	}
	ucFail("lang.golang.expression-failed", "a map key does not exist", e)
	return ucValue{}
}
func (env *ucEnvironment) eval(e *ucExpr) ucValue {
	if e == nil {
		ucFail("lang.golang.expression-failed", "an expression is missing", e)
	}
	child := func(i int) ucValue { return env.eval(e.Children[i]) }
	switch e.Kind {
	case "number":
		if strings.Contains(e.Text, ".") {
			f, err := strconv.ParseFloat(e.Text, 64)
			if err != nil || math.IsInf(f, 0) {
				ucFail("lang.golang.expression-failed", "the float literal is not finite", e)
			}
			return ucValue{Type: "float", Data: f}
		}
		n, ok := new(big.Int).SetString(e.Text, 10)
		if !ok {
			ucFail("lang.golang.expression-failed", "the integer literal is invalid", e)
		}
		return ucValue{Type: "int", Data: n}
	case "string":
		return ucValue{Type: "string", Data: e.Text}
	case "bool":
		return ucBool(e.Text == "true")
	case "none":
		return ucValue{}
	case "name":
		if i := strings.LastIndexByte(e.Text, '.'); i > 0 {
			if ty, ok := env.spec.Types[e.Text[:i]]; ok && ty.Form == "enum" {
				return ucValue{Type: e.Text[:i], Data: e.Text[i+1:]}
			}
		}
		if e.Text == "result" {
			if env.failure != nil {
				ucFail("lang.golang.expression-failed", "result was evaluated after an error return", e)
			}
			return env.result
		}
		v, ok := env.values[e.Text]
		if !ok {
			ucFail("lang.golang.expression-failed", "the input or observation "+e.Text+" is unavailable", e)
		}
		return v
	case "field":
		name := ucName(e)
		if i := strings.LastIndexByte(name, '.'); i > 0 {
			if ty, ok := env.spec.Types[name[:i]]; ok && ty.Form == "enum" {
				return ucValue{Type: name[:i], Data: name[i+1:]}
			}
		}
		return ucFieldValue(child(0), e.Text)
	case "index":
		return ucLookup(child(0), child(1), e)
	case "unary":
		return ucBool(!ucTruth(child(0), e))
	case "old":
		v, ok := env.old[ucExprKey(e)]
		if !ok {
			ucFail("lang.golang.expression-failed", "the pre-call value was not captured", e)
		}
		return v
	case "raised":
		_, ok := env.failures[e.Text]
		return ucBool(ok)
	case "failure":
		v, ok := env.failures[e.Text]
		if !ok {
			ucFail("lang.golang.expression-failed", "failure was evaluated without a matching error", e)
		}
		return v
	case "call":
		if ucName(e.Children[0]) == "size" {
			v := ucMaterialize(child(1))
			n := 0
			switch x := v.Data.(type) {
			case string:
				if ucBaseType(v.Type) == "bytes" {
					n = len(x)
				} else {
					n = utf8.RuneCountInString(x)
				}
			case []ucValue:
				n = len(x)
			case ucMap:
				n = len(x)
			default:
				ucFail("lang.golang.expression-failed", "size requires a collection, string or bytes", e)
			}
			return ucValue{Type: "int", Data: big.NewInt(int64(n))}
		}
		op, fn, args := env.prepare(e)
		return env.invoke(op, fn, args, e, true)
	case "binary":
		a := ucMaterialize(child(0))
		switch e.Text {
		case "and":
			if !ucTruth(a, e) {
				return ucBool(false)
			}
			return ucBool(ucTruth(child(1), e))
		case "or":
			if ucTruth(a, e) {
				return ucBool(true)
			}
			return ucBool(ucTruth(child(1), e))
		case "implies":
			if !ucTruth(a, e) {
				return ucBool(true)
			}
			return ucBool(ucTruth(child(1), e))
		}
		b := ucMaterialize(child(1))
		switch e.Text {
		case "==":
			return ucBool(ucEqual(a, b))
		case "!=":
			return ucBool(!ucEqual(a, b))
		case "in":
			if b.Data == nil {
				ucFail("lang.golang.expression-failed", "membership requires a collection but the optional value is none", e)
			}
			switch xs := b.Data.(type) {
			case []ucValue:
				for _, x := range xs {
					if ucEqual(a, x) {
						return ucBool(true)
					}
				}
			case ucMap:
				for _, x := range xs {
					if ucEqual(a, x.Key) {
						return ucBool(true)
					}
				}
			}
			return ucBool(false)
		}
		if a.Data == nil || b.Data == nil {
			ucFail("lang.golang.expression-failed", "none was used where a numeric or ordered value is required", e)
		}
		if x, ok := a.Data.(*big.Int); ok {
			y, ok := b.Data.(*big.Int)
			if !ok {
				ucFail("lang.golang.expression-failed", "integer arithmetic has a non-integer operand", e)
			}
			switch e.Text {
			case "+":
				return ucValue{Type: "int", Data: new(big.Int).Add(x, y)}
			case "-":
				return ucValue{Type: "int", Data: new(big.Int).Sub(x, y)}
			case "*":
				return ucValue{Type: "int", Data: new(big.Int).Mul(x, y)}
			case "/":
				if y.Sign() == 0 {
					ucFail("lang.golang.expression-failed", "division by zero", e)
				}
				return ucValue{Type: "int", Data: new(big.Int).Quo(x, y)}
			}
			return ucBool(ucCompare(x.Cmp(y), e.Text))
		}
		if x, ok := a.Data.(float64); ok {
			y := b.Data.(float64)
			var z float64
			switch e.Text {
			case "+":
				z = x + y
			case "-":
				z = x - y
			case "*":
				z = x * y
			case "/":
				if y == 0 {
					ucFail("lang.golang.expression-failed", "division by zero", e)
				}
				z = x / y
			default:
				return ucBool(ucCompare(ucOrdered(x, y), e.Text))
			}
			if math.IsNaN(z) || math.IsInf(z, 0) {
				ucFail("lang.golang.expression-failed", "floating-point arithmetic produced a non-finite value", e)
			}
			return ucValue{Type: "float", Data: z}
		}
		switch x := a.Data.(type) {
		case string:
			return ucBool(ucCompare(strings.Compare(x, b.Data.(string)), e.Text))
		case time.Time:
			return ucBool(ucCompare(x.Compare(b.Data.(time.Time)), e.Text))
		case time.Duration:
			return ucBool(ucCompare(ucOrdered(x, b.Data.(time.Duration)), e.Text))
		}
	case "all", "any", "count", "sum":
		collection := ucMaterialize(child(0))
		var items []ucValue
		switch xs := collection.Data.(type) {
		case []ucValue:
			items = xs
		case ucMap:
			for _, x := range xs {
				items = append(items, x.Key)
			}
		default:
			ucFail("lang.golang.expression-failed", "a quantifier requires a collection", e)
		}
		previous, had := env.values[e.Text]
		defer func() {
			if had {
				env.values[e.Text] = previous
			} else {
				delete(env.values, e.Text)
			}
		}()
		count := 0
		all, any := true, false
		integer := new(big.Int)
		rational := new(big.Rat)
		collectionKind, collectionArgs := ucSplitType(ucBaseType(collection.Type))
		_ = collectionKind
		locals := map[string]string{}
		if len(collectionArgs) > 0 {
			locals[e.Text] = collectionArgs[0]
		}
		floating := ucBaseType(env.typeOf(e.Children[1], locals)) == "float"
		for _, item := range items {
			env.values[e.Text] = item
			value := env.eval(e.Children[1])
			if e.Kind == "sum" {
				switch x := value.Data.(type) {
				case *big.Int:
					integer.Add(integer, x)
				case float64:
					floating = true
					rational.Add(rational, new(big.Rat).SetFloat64(x))
				default:
					ucFail("lang.golang.expression-failed", "sum requires numeric values", e)
				}
			} else {
				b := ucTruth(value, e)
				all = all && b
				any = any || b
				if b {
					count++
				}
			}
		}
		switch e.Kind {
		case "all":
			return ucBool(all)
		case "any":
			return ucBool(any)
		case "count":
			return ucValue{Type: "int", Data: big.NewInt(int64(count))}
		case "sum":
			if floating {
				f, _ := rational.Float64()
				if math.IsInf(f, 0) {
					ucFail("lang.golang.expression-failed", "sum is not finite", e)
				}
				return ucValue{Type: "float", Data: f}
			}
			return ucValue{Type: "int", Data: integer}
		}
	}
	ucFail("lang.golang.unsupported-contract", "the runtime cannot evaluate expression kind "+e.Kind, e)
	return ucValue{}
}
func (env *ucEnvironment) typeOf(e *ucExpr, locals map[string]string) string {
	switch e.Kind {
	case "number":
		if strings.Contains(e.Text, ".") {
			return "float"
		}
		return "int"
	case "string":
		return "string"
	case "bool", "unary", "raised":
		return "bool"
	case "name":
		if t, ok := locals[e.Text]; ok {
			return t
		}
		if v, ok := env.values[e.Text]; ok {
			return v.Type
		}
		if e.Text == "result" {
			return env.result.Type
		}
		if i := strings.LastIndexByte(e.Text, '.'); i > 0 {
			if ty, ok := env.spec.Types[e.Text[:i]]; ok && ty.Form == "enum" {
				return e.Text[:i]
			}
		}
	case "old":
		return env.typeOf(e.Children[0], locals)
	case "failure":
		return e.Text
	case "field":
		model := env.typeOf(e.Children[0], locals)
		if kind, args := ucSplitType(model); kind == "optional" {
			model = args[0]
		}
		for _, f := range ucFields(env.spec.Types, model) {
			if f.Name == e.Text {
				return f.Type
			}
		}
	case "index":
		_, args := ucSplitType(ucBaseType(env.typeOf(e.Children[0], locals)))
		if len(args) > 1 {
			return args[1]
		}
	case "call":
		name := ucName(e.Children[0])
		if name == "size" {
			return "int"
		}
		if op, ok := env.spec.Operations[name]; ok {
			return op.Result
		}
		callee := e.Children[0]
		if callee.Kind == "field" {
			model := env.typeOf(callee.Children[0], locals)
			if kind, args := ucSplitType(model); kind == "optional" {
				model = args[0]
			}
			return env.spec.Operations[model+"."+callee.Text].Result
		}
	case "binary":
		switch e.Text {
		case "+", "-", "*", "/":
			return env.typeOf(e.Children[0], locals)
		default:
			return "bool"
		}
	case "all", "any":
		return "bool"
	case "count":
		return "int"
	case "sum":
		_, args := ucSplitType(ucBaseType(env.typeOf(e.Children[0], locals)))
		copy := map[string]string{}
		for k, v := range locals {
			copy[k] = v
		}
		if len(args) > 0 {
			copy[e.Text] = args[0]
		}
		return env.typeOf(e.Children[1], copy)
	}
	return ""
}
func ucOrdered[T ~int64 | ~float64](a, b T) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func ucCompare(c int, op string) bool {
	switch op {
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	}
	ucFail("lang.golang.expression-failed", "unknown comparison operator "+op, nil)
	return false
}
func ucNative(v ucValue, t reflect.Type) reflect.Value {
	// Preserve mathematical integers before Go's generic conversion can truncate them.
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		number, isInteger := v.Data.(*big.Int)
		if isInteger || ucBaseType(v.Type) == "int" {
			if !isInteger {
				value := ucMaterialize(v)
				number, isInteger = value.Data.(*big.Int)
			}
			if !isInteger {
				ucFail("lang.golang.expression-failed", "none was used where an integer argument is required", nil)
			}
			out := reflect.New(t).Elem()
			switch t.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if !number.IsInt64() || out.OverflowInt(number.Int64()) {
					ucFail("lang.golang.input-outside-domain", "an integer argument is outside the Go parameter range", nil)
				}
				out.SetInt(number.Int64())
			default:
				if !number.IsUint64() || out.OverflowUint(number.Uint64()) {
					ucFail("lang.golang.input-outside-domain", "an integer argument is outside the Go parameter range", nil)
				}
				out.SetUint(number.Uint64())
			}
			return out
		}
	}
	if v.Native.IsValid() {
		n := v.Native
		if n.Type().AssignableTo(t) {
			return n
		}
		if n.Type().ConvertibleTo(t) {
			return n.Convert(t)
		}
	}
	if v.Data == nil {
		if t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface {
			return reflect.Zero(t)
		}
		ucFail("lang.golang.expression-failed", "none cannot be adapted to a required Go value", nil)
	}
	out := reflect.New(t).Elem()
	if t.Kind() == reflect.Pointer {
		out.Set(reflect.New(t.Elem()))
		out.Elem().Set(ucNative(v, t.Elem()))
		return out
	}
	switch x := v.Data.(type) {
	case *big.Int:
		ucFail("lang.golang.expression-failed", "an integer value requires an integer Go parameter", nil)
	case float64:
		out.SetFloat(x)
	case string:
		if out.Kind() == reflect.Slice {
			out.SetBytes([]byte(x))
		} else {
			out.SetString(x)
		}
	case bool:
		out.SetBool(x)
	case time.Time:
		out.Set(reflect.ValueOf(x))
	case time.Duration:
		out.SetInt(int64(x))
	case []ucValue:
		if t.Kind() != reflect.Slice {
			ucFail("lang.golang.unsupported-contract", "the detached list has no supported Go slice shape", nil)
		}
		out.Set(reflect.MakeSlice(t, len(x), len(x)))
		for i, item := range x {
			out.Index(i).Set(ucNative(item, t.Elem()))
		}
	case ucMap:
		if t.Kind() != reflect.Map {
			ucFail("lang.golang.unsupported-contract", "the detached collection has no supported Go map shape", nil)
		}
		out.Set(reflect.MakeMap(t))
		kind, _ := ucSplitType(ucBaseType(v.Type))
		for _, item := range x {
			value := reflect.Zero(t.Elem())
			if kind != "set" {
				value = ucNative(item.Value, t.Elem())
			}
			out.SetMapIndex(ucNative(item.Key, t.Key()), value)
		}
	case map[string]ucValue:
		if t.Kind() != reflect.Struct {
			ucFail("lang.golang.unsupported-contract", "the detached object has no supported Go record shape", nil)
		}
		model := v.Type
		if kind, args := ucSplitType(model); kind == "optional" {
			model = args[0]
		}
		fields := ucFields(v.Schema, model)
		known := map[string]bool{}
		for _, field := range fields {
			known[field.GoName] = true
		}
		for i := 0; i < t.NumField(); i++ {
			if !known[t.Field(i).Name] {
				ucFail("lang.golang.unsupported-contract", "a pure operation needs hidden Go state that cannot be reconstructed from the old model value: "+t.Field(i).Name, nil)
			}
		}
		for _, field := range fields {
			target := out.FieldByName(field.GoName)
			if !target.IsValid() || !target.CanSet() {
				ucFail("lang.golang.unsupported-contract", "a detached model field cannot be reconstructed: "+field.Name, nil)
			}
			target.Set(ucNative(x[field.Name], target.Type()))
		}
	default:
		ucFail("lang.golang.unsupported-contract", "the detached model value cannot be adapted to the Go operation", nil)
	}
	return out
}
func (env *ucEnvironment) prepare(e *ucExpr) (ucOperation, reflect.Value, []reflect.Value) {
	callee := e.Children[0]
	name := ucName(callee)
	op, ok := env.spec.Operations[name]
	fn := op.Function
	if !ok && callee.Kind == "field" {
		receiver := env.eval(callee.Children[0])
		if ucNone(receiver) {
			ucFail("lang.golang.expression-failed", "none was used as an operation receiver", e)
		}
		if !receiver.Native.IsValid() && receiver.Shape != nil {
			receiver.Native = ucNative(receiver, receiver.Shape)
		}
		base, _ := ucSplitType(receiver.Type)
		if base == "optional" {
			_, a := ucSplitType(receiver.Type)
			base = a[0]
		}
		op, ok = env.spec.Operations[base+"."+callee.Text]
		if ok {
			op.receiver = receiver
			fn = receiver.Native.MethodByName(op.GoName)
			if !fn.IsValid() {
				v := ucDeref(receiver.Native)
				if v.IsValid() {
					if v.CanAddr() {
						fn = v.Addr().MethodByName(op.GoName)
					} else {
						copy := reflect.New(v.Type())
						copy.Elem().Set(v)
						fn = copy.MethodByName(op.GoName)
						if fn.IsValid() {
							receiver.Native = copy
							op.receiver = receiver
						}
					}
				}
			}
		}
	}
	if !ok || !fn.IsValid() {
		ucFail("lang.golang.expression-failed", "the bound operation "+name+" is unavailable", e)
	}
	if fn.Type().NumIn() != len(e.Children)-1 {
		ucFail("lang.golang.expression-failed", "the operation argument count does not match", e)
	}
	args := make([]reflect.Value, len(e.Children)-1)
	for i := range args {
		value := env.eval(e.Children[i+1])
		if i < len(op.Params) {
			kind, _ := ucSplitType(op.Params[i])
			if kind != "optional" && ucNone(value) {
				ucFail("lang.golang.expression-failed", "none was passed where the operation requires a value", e)
			}
		}
		args[i] = ucNative(value, fn.Type().In(i))
	}
	return op, fn, args
}
func (env *ucEnvironment) invoke(op ucOperation, fn reflect.Value, args []reflect.Value, e *ucExpr, pure bool) ucValue {
	if pure && !op.Pure {
		ucFail("lang.golang.expression-failed", "a non-pure operation was used in an expression", e)
	}
	snapshot := func() []ucValue {
		var values []ucValue
		if op.receiver.Native.IsValid() {
			values = append(values, ucNormalize(op.receiver.Native, op.receiver.Type, env.spec.Types))
		}
		for i, arg := range args {
			if i >= len(op.Params) {
				ucFail("lang.golang.expression-failed", "the pure operation parameter types are unavailable", e)
			}
			values = append(values, ucNormalize(arg, op.Params[i], env.spec.Types))
		}
		return values
	}
	before := snapshot()
	invoke := func() ucValue {
		result := fn.Call(args)
		if len(result) > 0 && result[len(result)-1].Type().Implements(reflect.TypeFor[error]()) {
			err := result[len(result)-1]
			if !err.IsNil() {
				ucFail("lang.golang.expression-failed", "a pure operation returned an error: "+err.Interface().(error).Error(), e)
			}
			result = result[:len(result)-1]
		}
		if len(result) != 1 {
			ucFail("lang.golang.expression-failed", "a pure operation must return exactly one model value", e)
		}
		return ucNormalize(result[0], op.Result, env.spec.Types)
	}
	first := invoke()
	afterFirst := snapshot()
	second := invoke()
	afterSecond := snapshot()
	if !ucEqual(first, second) {
		ucFail("lang.golang.purity-violation", "repeated pure calls returned different model values", e)
	}
	for i := range before {
		if !ucEqual(before[i], afterFirst[i]) || !ucEqual(before[i], afterSecond[i]) {
			ucFail("lang.golang.purity-violation", "a pure operation changed its receiver or arguments", e)
		}
	}
	return first
}

func ucWalk(e *ucExpr, visit func(*ucExpr)) {
	if e == nil {
		return
	}
	visit(e)
	for _, child := range e.Children {
		ucWalk(child, visit)
	}
}
func (env *ucEnvironment) readObservers() {
	for _, o := range env.spec.Observers {
		args := make([]reflect.Value, len(o.Inputs))
		for i, input := range o.Inputs {
			args[i] = ucNative(env.values[input], o.Read.Type().In(i))
		}
		values := o.Read.Call(args)
		if len(values) == 2 {
			if !values[1].IsNil() {
				ucFail("lang.golang.observation-failed", "observer "+o.Name+" failed: "+values[1].Interface().(error).Error(), nil)
			}
		}
		if len(values) < 1 || len(values) > 2 {
			ucFail("lang.golang.observation-failed", "observer "+o.Name+" returned an invalid signature", nil)
		}
		env.values[o.Name] = ucNormalize(values[0], o.Type, env.spec.Types)
	}
}

type ucSegment struct {
	Field string
	Key   *ucValue
}

func (env *ucEnvironment) path(e *ucExpr) (string, []ucSegment) {
	switch e.Kind {
	case "name":
		return e.Text, nil
	case "field":
		r, p := env.path(e.Children[0])
		return r, append(p, ucSegment{Field: e.Text})
	case "index":
		r, p := env.path(e.Children[0])
		key := ucDetach(env.eval(e.Children[1]))
		return r, append(p, ucSegment{Key: &key})
	}
	ucFail("lang.golang.expression-failed", "a change target is not a state path", e)
	return "", nil
}
func ucGetPath(v ucValue, path []ucSegment, allowMissing bool) ucValue {
	for i, s := range path {
		if s.Key == nil {
			v = ucFieldValue(v, s.Field)
		} else {
			found := false
			v = ucMaterialize(v)
			items, ok := v.Data.(ucMap)
			if !ok {
				ucFail("lang.golang.expression-failed", "a change path indexes a non-map parent", nil)
			}
			for _, item := range items {
				if ucEqual(item.Key, *s.Key) {
					v = item.Value
					found = true
					break
				}
			}
			if !found {
				if allowMissing && i == len(path)-1 {
					return ucValue{}
				}
				ucFail("lang.golang.expression-failed", "a change path has a missing parent map entry", nil)
			}
		}
	}
	return v
}
func ucSameSegment(a, b ucSegment) bool {
	if a.Key == nil || b.Key == nil {
		return a.Key == nil && b.Key == nil && a.Field == b.Field
	}
	return ucEqual(*a.Key, *b.Key)
}
func ucPrefix(a, b []ucSegment) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if !ucSameSegment(a[i], b[i]) {
			return false
		}
	}
	return true
}
func ucDiff(before, after ucValue, path []ucSegment, label string, allowed [][]ucSegment, out *[]string) {
	for _, a := range allowed {
		if ucPrefix(a, path) {
			return
		}
	}
	if ucEqual(before, after) {
		return
	}
	if a, ok := before.Data.(map[string]ucValue); ok {
		if b, ok := after.Data.(map[string]ucValue); ok {
			keys := make([]string, 0, len(a))
			for k := range a {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v, present := b[k]
				if !present {
					*out = append(*out, label+"."+k+" was removed")
					continue
				}
				ucDiff(a[k], v, append(slices.Clone(path), ucSegment{Field: k}), label+"."+k, allowed, out)
			}
			for k := range b {
				if _, ok := a[k]; !ok {
					*out = append(*out, label+"."+k+" was added")
				}
			}
			return
		}
	}
	if a, ok := before.Data.(ucMap); ok {
		if b, ok := after.Data.(ucMap); ok {
			for _, old := range a {
				var value ucValue
				found := false
				for _, now := range b {
					if ucEqual(old.Key, now.Key) {
						value = now.Value
						found = true
						break
					}
				}
				p := append(slices.Clone(path), ucSegment{Key: &old.Key})
				name := label + "[" + ucKey(old.Key) + "]"
				if !found {
					permitted := false
					for _, allow := range allowed {
						if ucPrefix(allow, p) {
							permitted = true
						}
					}
					if !permitted {
						*out = append(*out, name+" was removed")
					}
				} else {
					ucDiff(old.Value, value, p, name, allowed, out)
				}
			}
			for _, now := range b {
				found := false
				for _, old := range a {
					if ucEqual(old.Key, now.Key) {
						found = true
						break
					}
				}
				if !found {
					p := append(slices.Clone(path), ucSegment{Key: &now.Key})
					permitted := false
					for _, allow := range allowed {
						if ucPrefix(allow, p) {
							permitted = true
						}
					}
					if !permitted {
						*out = append(*out, label+"["+ucKey(now.Key)+"] was added")
					}
				}
			}
			return
		}
	}
	*out = append(*out, fmt.Sprintf("%s changed from %s to %s", label, ucKey(before), ucKey(after)))
}
func ucExecuteSingle(s *ucContract, inputs map[string]reflect.Value, explore bool) (out ucOutcome) {
	phase := "input"
	defer func() {
		if p := recover(); p != nil {
			if issue, ok := p.(ucIssue); ok {
				out.Infrastructure = phase == "input" || phase == "observer" || phase == "requires" || issue.code == "lang.golang.observation-failed" || issue.code == "lang.golang.unsupported-contract" || issue.code == "lang.golang.input-outside-domain"
				out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(s, issue))
			} else {
				out.Infrastructure = phase != "call" && phase != "assertion"
				code := "lang.golang.adapter-failed"
				if !out.Infrastructure {
					code = "lang.golang.call-panicked"
				}
				out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(s, ucIssue{code, fmt.Sprintf("the call or adapter panicked: %v", p), nil}))
			}
		}
	}()
	env := ucEnvironment{spec: s, values: map[string]ucValue{}, old: map[string]ucValue{}, failures: map[string]ucValue{}}
	for _, symbol := range s.Symbols {
		env.values[symbol.Name] = ucLazyValue(inputs[symbol.Name], symbol.Type, s.Types)
	}
	phase = "observer"
	env.readObservers()
	phase = "requires"
	for _, e := range s.Requires {
		if !ucTruth(env.eval(e), e) {
			if explore {
				out.Rejected = true
				return out
			}
			ucFail("lang.golang.input-outside-domain", "a replay input does not satisfy its precondition", e)
		}
	}
	out.Executed = true
	phase = "assertion"
	type root struct {
		Name    string
		Path    []ucSegment
		Value   ucValue
		Allowed [][]ucSegment
	}
	var roots []root
	for _, name := range s.Observe {
		if name == "none" {
			continue
		}
		parts := strings.Split(name, ".")
		var path []ucSegment
		for _, p := range parts[1:] {
			path = append(path, ucSegment{Field: p})
		}
		value := ucMaterialize(ucGetPath(env.values[parts[0]], path, false))
		roots = append(roots, root{Name: parts[0], Path: path, Value: value})
	}
	for _, o := range s.Observers {
		roots = append(roots, root{Name: o.Name, Value: env.values[o.Name]})
	}
	for _, e := range s.Changes {
		name, path := env.path(e)
		ucGetPath(env.values[name], path, true)
		for i := range roots {
			if roots[i].Name == name && ucPrefix(roots[i].Path, path) {
				roots[i].Allowed = append(roots[i].Allowed, path[len(roots[i].Path):])
			}
		}
	}
	for _, e := range s.Ensures {
		ucWalk(e, func(e *ucExpr) {
			if e.Kind == "old" {
				env.old[ucExprKey(e)] = ucDetach(env.eval(e.Children[0]))
			}
		})
	}
	if s.Call != nil {
		op, fn, args := env.prepare(s.Call)
		var before, afterFirst []ucValue
		if op.Pure {
			before = env.callInputs(op, args)
		}
		start := time.Now()
		phase = "call"
		values := fn.Call(args)
		out.Duration = time.Since(start)
		phase = "assertion"
		env.captureCall(op, values, s.Call, &out)
		if op.Pure {
			afterFirst = env.callInputs(op, args)
			firstErrorText := ""
			if env.failure != nil {
				firstErrorText = env.failure.Error()
			}
			second := ucEnvironment{spec: s, failures: map[string]ucValue{}}
			phase = "call"
			repeated := fn.Call(args)
			phase = "assertion"
			var ignored ucOutcome
			second.captureCall(op, repeated, s.Call, &ignored)
			afterSecond := env.callInputs(op, args)
			different := (env.failure == nil) != (second.failure == nil) || !ucEqual(env.result, second.result)
			if env.failure != nil && second.failure != nil {
				different = different || reflect.TypeOf(env.failure) != reflect.TypeOf(second.failure) || firstErrorText != second.failure.Error()
			}
			if len(env.failures) != len(second.failures) {
				different = true
			}
			for id, value := range env.failures {
				other, ok := second.failures[id]
				if !ok || !ucEqual(value, other) {
					different = true
				}
			}
			for i := range before {
				if !ucEqual(before[i], afterFirst[i]) || !ucEqual(before[i], afterSecond[i]) {
					different = true
				}
			}
			if different {
				diagnostic := ucDiagnosticOf(s, ucIssue{"lang.golang.purity-violation", "repeated pure calls changed a result, declared error, receiver or argument", s.Call})
				diagnostic.Subject = op.Name
				out.Diagnostics = append(out.Diagnostics, diagnostic)
				out.Differences = append(out.Differences, diagnostic.Message)
			}
		}
		for _, symbol := range s.Symbols {
			env.values[symbol.Name] = ucLazyValue(inputs[symbol.Name], symbol.Type, s.Types)
		}
		phase = "observer"
		env.readObservers()
		phase = "assertion"
	}
	for _, e := range s.Ensures {
		if !ucTruth(env.eval(e), e) {
			out.Differences = append(out.Differences, fmt.Sprintf("%s:%d:%d: ensure is false", s.Path, e.Line, e.Column))
			out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(s, ucIssue{"lang.golang.ensure-failed", "the ensure expression is false", e}))
		}
	}
	for _, r := range roots {
		after := ucMaterialize(ucGetPath(env.values[r.Name], r.Path, false))
		ucDiff(r.Value, after, nil, r.Name+ucPathLabel(r.Path), r.Allowed, &out.Differences)
	}
	if len(out.Differences) > 0 && len(out.Diagnostics) == 0 {
		out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(s, ucIssue{"lang.golang.frame-violation", strings.Join(out.Differences, "; "), nil}))
	}
	slices.Sort(out.Differences)
	return out
}
func ucPathLabel(path []ucSegment) string {
	var out string
	for _, s := range path {
		if s.Key == nil {
			out += "." + s.Field
		} else {
			out += "[" + ucKey(*s.Key) + "]"
		}
	}
	return out
}

// ucDraw uses the same model types as snapshots. Custom generated symbols use
// their bound generators instead, including dependencies on earlier symbols.
func ucDraw(t *rapid.T, typ reflect.Type, model string, types map[string]ucType, limits []string, path string, depth int) reflect.Value {
	if value, ok := ucDrawDouble(t, typ, model, types, limits, path, depth); ok {
		return value
	}
	kind, args := ucSplitType(model)
	out := reflect.New(typ).Elem()
	if depth > 8 {
		if kind == "optional" {
			return out
		}
		ucFail("lang.golang.unsupported-contract", "the generated input exceeds the recursive value budget", nil)
	}
	low, high := int64(-100), int64(100)
	floatLow, floatHigh := -100.0, 100.0
	minSize, maxSize := 0, 5
	var subset []string
	for _, limit := range limits {
		parts := strings.Fields(limit)
		if len(parts) < 3 || parts[0] != path {
			continue
		}
		if parts[2][0] == '{' {
			for _, v := range strings.Split(strings.Trim(strings.Join(parts[2:], " "), "{}"), ",") {
				subset = append(subset, strings.TrimSpace(v))
			}
			continue
		}
		a, b, ok := strings.Cut(parts[2], "..")
		if !ok {
			continue
		}
		if kind == "float" && parts[1] == "in" {
			var err error
			floatLow, err = strconv.ParseFloat(a, 64)
			if err != nil {
				ucFail("lang.golang.input-outside-domain", "a float sampling bound is invalid", nil)
			}
			floatHigh, err = strconv.ParseFloat(b, 64)
			if err != nil {
				ucFail("lang.golang.input-outside-domain", "a float sampling bound is invalid", nil)
			}
			continue
		}
		x, ex := strconv.ParseInt(a, 10, 64)
		y, ey := strconv.ParseInt(b, 10, 64)
		if ex != nil || ey != nil {
			ucFail("lang.golang.unsupported-contract", "a non-integral sampling bound is unsupported", nil)
		}
		if parts[1] == "size" {
			minSize, maxSize = int(x), int(y)
		} else {
			low, high = x, y
		}
	}
	if kind == "optional" {
		if rapid.Bool().Draw(t, path+".none") {
			return out
		}
		out.Set(reflect.New(typ.Elem()))
		out.Elem().Set(ucDraw(t, typ.Elem(), args[0], types, limits, path, depth+1))
		return out
	}
	if typ.Kind() == reflect.Pointer {
		out.Set(reflect.New(typ.Elem()))
		out.Elem().Set(ucDraw(t, typ.Elem(), model, types, limits, path, depth+1))
		return out
	}
	switch kind {
	case "call":
		operations := types[model].Calls
		if len(operations) == 0 {
			ucFail("lang.golang.unsupported-contract", "the call type has no available operation", nil)
		}
		op := rapid.SampledFrom(operations).Draw(t, path+".operation")
		out.FieldByName("Operation").SetString(op.Name)
		arguments := out.FieldByName("Arguments")
		arguments.Set(reflect.MakeMap(arguments.Type()))
		for i, param := range op.ParamNames {
			if i >= len(op.ParamTypes) {
				ucFail("lang.golang.unsupported-contract", "the native parameter type is unavailable", nil)
			}
			argument := ucDraw(t, op.ParamTypes[i], op.Params[i], types, limits, path+"."+op.Name[strings.LastIndexByte(op.Name, '.')+1:]+"."+param, depth+1)
			arguments.SetMapIndex(reflect.ValueOf(param), argument)
		}
	case "bool":
		out.SetBool(rapid.Bool().Draw(t, path))
	case "int":
		out.SetInt(rapid.Int64Range(low, high).Draw(t, path))
	case "float":
		out.SetFloat(rapid.Float64Range(floatLow, floatHigh).Draw(t, path))
	case "string":
		out.SetString(rapid.StringN(minSize, maxSize, maxSize*4).Draw(t, path))
	case "bytes":
		out.SetBytes(rapid.SliceOfN(rapid.Byte(), minSize, maxSize).Draw(t, path))
	case "time":
		out.Set(reflect.ValueOf(time.Unix(rapid.Int64Range(-100000, 100000).Draw(t, path), 0).UTC()))
	case "duration":
		out.SetInt(rapid.Int64Range(low, high).Draw(t, path))
	case "list":
		n := rapid.IntRange(minSize, maxSize).Draw(t, path+".size")
		out.Set(reflect.MakeSlice(typ, n, n))
		for i := 0; i < n; i++ {
			out.Index(i).Set(ucDraw(t, typ.Elem(), args[0], types, limits, path, depth+1))
		}
	case "map", "set":
		n := rapid.IntRange(minSize, maxSize).Draw(t, path+".size")
		out.Set(reflect.MakeMap(typ))
		for i := 0; i < n; i++ {
			key := ucDraw(t, typ.Key(), args[0], types, nil, path+".key", depth+1)
			value := reflect.Zero(typ.Elem())
			if kind == "map" {
				value = ucDraw(t, typ.Elem(), args[1], types, limits, path, depth+1)
			}
			out.SetMapIndex(key, value)
		}
		if out.Len() < minSize {
			t.SkipNow()
		}
	default:
		ty, ok := types[model]
		if !ok || ty.Form == "interface" || kind == "call" {
			ucFail("lang.golang.unsupported-contract", "cannot generate the model type "+model, nil)
		}
		if ty.Form == "enum" {
			values := ty.Values
			if subset != nil {
				values = subset
			}
			if len(values) == 0 {
				ucFail("lang.golang.input-outside-domain", "an enum has no allowed values", nil)
			}
			out.SetString(rapid.SampledFrom(values).Draw(t, path))
		} else {
			for _, f := range ucFields(types, model) {
				field := out.FieldByName(f.GoName)
				if !field.IsValid() || !field.CanSet() {
					ucFail("lang.golang.observation-failed", "cannot construct the model field "+f.Name, nil)
				}
				field.Set(ucDraw(t, field.Type(), f.Type, types, limits, path+"."+f.Name, depth+1))
			}
		}
	}
	return out
}

var ucCurrent *ucRunResult
var ucExploring bool

// ucContext scopes driver state even when a predicate skips, fails, or panics.
func ucContext(result *ucRunResult, exploring bool) func() {
	previous, wasExploring := ucCurrent, ucExploring
	ucCurrent, ucExploring = result, exploring
	return func() { ucCurrent, ucExploring = previous, wasExploring }
}

func ucCheck(t rapid.TB, s *ucContract, inputs map[string]reflect.Value) {
	out := ucExecute(s, inputs, ucExploring)
	if ucCurrent != nil {
		if out.Rejected {
			ucCurrent.Rejected++
		}
		if out.Executed {
			ucCurrent.Accepted++
		}
		ucCurrent.Diagnostics = append(ucCurrent.Diagnostics, out.Diagnostics...)
		ucCurrent.Differences = append(ucCurrent.Differences, out.Differences...)
		ucCurrent.Infrastructure = ucCurrent.Infrastructure || out.Infrastructure
		if out.Duration > 0 {
			ucCurrent.durations = append(ucCurrent.durations, out.Duration)
		}
		if len(out.Diagnostics) > 0 || len(out.Differences) > 0 {
			ucCurrent.Passed = false
		}
	}
	if out.Rejected {
		t.SkipNow()
	}
	if len(out.Diagnostics) > 0 || len(out.Differences) > 0 {
		t.Fatalf("contract %s failed: %v %v", s.ID, out.Diagnostics, out.Differences)
	}
}
func ucParseTime(text string) time.Time {
	value, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		panic(err)
	}
	return value
}
func ucParseDuration(text string) time.Duration {
	value, err := time.ParseDuration(text)
	if err != nil {
		panic(err)
	}
	return value
}
func (d ucDiagnostic) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Code    string `json:"code"`
		Subject string `json:"subject"`
		Path    string `json:"path"`
		Line    int    `json:"line"`
		Column  int    `json:"column"`
		Message string `json:"message"`
		Fix     string `json:"fix"`
	}{d.Code, d.Subject, d.Position.Path, d.Position.Line, d.Position.Column, d.Message, d.Fix})
}
func (d *ucDiagnostic) UnmarshalJSON(data []byte) error {
	var wire struct {
		Code    string `json:"code"`
		Subject string `json:"subject"`
		Path    string `json:"path"`
		Line    int    `json:"line"`
		Column  int    `json:"column"`
		Message string `json:"message"`
		Fix     string `json:"fix"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*d = ucDiagnostic{wire.Code, wire.Subject, ucPosition{wire.Path, wire.Line, wire.Column}, wire.Message, wire.Fix}
	return nil
}

func ucFailedPredicate(t rapid.TB, s *ucContract, r *ucRunResult) {
	if t.Failed() {
		r.Passed = false
		if len(r.Diagnostics) == 0 {
			r.Diagnostics = append(r.Diagnostics, ucDiagnosticOf(s, ucIssue{"lang.golang.predicate-failed", "the bound predicate or its driver failed", nil}))
		}
	}
}
func ucFinish(t rapid.TB, s *ucContract, r *ucRunResult) {
	if r.Accepted == 0 {
		r.Diagnostics = append(r.Diagnostics, ucDiagnosticOf(s, ucIssue{"lang.golang.no-effective-input", "the selected property has no effective execution", nil}))
		r.Passed = false
		r.Infrastructure = true
	}
	if s.Within > 0 && len(r.durations) > 0 {
		slices.Sort(r.durations)
		i := (95*len(r.durations)+99)/100 - 1
		if r.durations[i] > s.Within {
			r.Passed = false
			message := fmt.Sprintf("the 95th percentile %s exceeds within %s", r.durations[i], s.Within)
			r.Differences = append(r.Differences, message)
			r.Diagnostics = append(r.Diagnostics, ucDiagnosticOf(s, ucIssue{"lang.golang.within-exceeded", message, nil}))
		}
	}
	r.Passed = r.Passed && !t.Failed()
	if !r.Passed {
		t.Errorf("contract %s failed: %v %v", s.ID, r.Diagnostics, r.Differences)
	}
}

// ucCallParts reads the self-contained generated carrier without erasing native
// argument types. Only the model's declared operation and parameters are used.
func ucCallParts(value reflect.Value) (string, map[string]reflect.Value) {
	value = ucDeref(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		ucFail("lang.golang.input-outside-domain", "a call input is not a generated carrier", nil)
	}
	operation, arguments := value.FieldByName("Operation"), value.FieldByName("Arguments")
	if !operation.IsValid() || operation.Kind() != reflect.String || !arguments.IsValid() || arguments.Kind() != reflect.Map {
		ucFail("lang.golang.input-outside-domain", "the call carrier has an invalid shape", nil)
	}
	out := map[string]reflect.Value{}
	for _, key := range arguments.MapKeys() {
		argument := arguments.MapIndex(key)
		if argument.Kind() == reflect.Interface {
			if argument.IsNil() {
				argument = reflect.Value{}
			} else {
				argument = argument.Elem()
			}
		}
		out[key.String()] = argument
	}
	return operation.String(), out
}
func (env *ucEnvironment) callInputs(op ucOperation, args []reflect.Value) []ucValue {
	var out []ucValue
	if op.receiver.Native.IsValid() {
		out = append(out, ucDetach(ucNormalize(op.receiver.Native, op.receiver.Type, env.spec.Types)))
	}
	for i, arg := range args {
		out = append(out, ucDetach(ucNormalize(arg, op.Params[i], env.spec.Types)))
	}
	return out
}
func (env *ucEnvironment) captureCall(op ucOperation, values []reflect.Value, expr *ucExpr, out *ucOutcome) {
	if len(values) > 0 && values[len(values)-1].Type().Implements(reflect.TypeFor[error]()) {
		value := values[len(values)-1]
		if !value.IsNil() {
			env.failure = value.Interface().(error)
		}
		values = values[:len(values)-1]
	}
	if env.failure != nil {
		matched := false
		for id, typ := range env.spec.ErrorTypes {
			target := reflect.New(typ)
			if errors.As(env.failure, target.Interface()) {
				env.failures[id] = ucDetach(ucNormalize(target.Elem(), id, env.spec.Types))
				if slices.Contains(op.Raises, id) {
					matched = true
				}
				for parent := env.spec.Types[id].Parent; parent != ""; parent = env.spec.Types[parent].Parent {
					env.failures[parent] = ucDetach(ucNormalize(target.Elem(), parent, env.spec.Types))
					if slices.Contains(op.Raises, parent) {
						matched = true
					}
				}
			}
		}
		if !matched {
			message := "the call returned an undeclared error: " + env.failure.Error()
			out.Differences = append(out.Differences, message)
			out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(env.spec, ucIssue{"lang.golang.undeclared-error", message, expr}))
		}
	}
	if env.failure == nil && len(values) > 0 {
		env.result = ucDetach(ucResultValue(values[0], op.Result, env.spec.Types))
	}
}
func ucExecute(s *ucContract, inputs map[string]reflect.Value, explore bool) (out ucOutcome) {
	defer func() {
		if problem := recover(); problem != nil {
			if issue, ok := problem.(ucIssue); ok {
				out.Infrastructure = true
				out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(s, issue))
			} else {
				out.Infrastructure = true
				out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(s, ucIssue{"lang.golang.substitute-failed", fmt.Sprintf("the substitute boundary panicked: %v", problem), nil}))
			}
		}
	}()
	cleanup := ucInstallDoubles(s, inputs)
	defer cleanup()
	if s.Sequence == "" {
		return ucExecuteSingle(s, inputs, explore)
	}
	initial := *s
	initial.Sequence = ""
	initial.SequenceReceiver = ""
	out = ucExecuteSingle(&initial, inputs, explore)
	if out.Rejected || len(out.Diagnostics) > 0 || len(out.Differences) > 0 {
		return out
	}
	sequence := ucDeref(inputs[s.Sequence])
	if !sequence.IsValid() || sequence.Kind() != reflect.Slice {
		out.Infrastructure = true
		out.Diagnostics = append(out.Diagnostics, ucDiagnosticOf(s, ucIssue{"lang.golang.input-outside-domain", "the sequence input is not a list", nil}))
		return out
	}
	for index := 0; index < sequence.Len(); index++ {
		step := *s
		step.Sequence = ""
		step.SequenceReceiver = ""
		step.Requires = nil
		local := map[string]reflect.Value{}
		for key, value := range inputs {
			local[key] = value
		}
		var prepared ucOutcome
		func() {
			defer func() {
				if problem := recover(); problem != nil {
					if issue, ok := problem.(ucIssue); ok {
						prepared.Infrastructure = true
						prepared.Diagnostics = append(prepared.Diagnostics, ucDiagnosticOf(s, issue))
					} else {
						panic(problem)
					}
				}
			}()
			model := ""
			for _, symbol := range s.Symbols {
				if symbol.Name == s.Sequence {
					_, parts := ucSplitType(symbol.Type)
					model = parts[0]
				}
			}
			call := sequence.Index(index)
			ucNormalize(call, model, s.Types)
			operation, arguments := ucCallParts(call)
			op := s.Operations[operation]
			receiver := &ucExpr{Kind: "name", Text: s.SequenceReceiver}
			step.Call = &ucExpr{Kind: "call", Children: []*ucExpr{{Kind: "field", Text: operation[strings.LastIndexByte(operation, '.')+1:], Children: []*ucExpr{receiver}}}}
			for i, name := range op.ParamNames {
				temporary := fmt.Sprintf("#unicodeArgument%d", i)
				local[temporary] = arguments[name]
				step.Symbols = append(append([]ucSymbol{}, step.Symbols...), ucSymbol{temporary, op.Params[i]})
				step.Call.Children = append(step.Call.Children, &ucExpr{Kind: "name", Text: temporary})
			}
			prepared = ucExecuteSingle(&step, local, explore)
		}()
		out.Duration += prepared.Duration
		out.Infrastructure = out.Infrastructure || prepared.Infrastructure
		out.Diagnostics = append(out.Diagnostics, prepared.Diagnostics...)
		out.Differences = append(out.Differences, prepared.Differences...)
		if len(prepared.Diagnostics) > 0 || len(prepared.Differences) > 0 {
			break
		}
	}
	return out
}

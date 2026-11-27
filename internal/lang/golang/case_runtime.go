package golang

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// These wire values are embedded into consumers without importing unicode.
type ucWireValue struct {
	Kind    string        `json:"kind"`
	Text    string        `json:"text"`
	Items   []ucWireValue `json:"items"`
	Entries []ucWireEntry `json:"entries"`
}
type ucWireEntry struct {
	Key   ucWireValue `json:"key"`
	Value ucWireValue `json:"value"`
}
type ucWireInput struct {
	Values map[string]ucWireValue `json:"values"`
}
type ucCodec struct {
	Representation string
	Encode, Decode reflect.Value `json:"-"`
}

func ucWireCanonical(v ucWireValue) ucWireValue {
	if v.Kind == "string" && !utf8.ValidString(v.Text) {
		v.Kind = "stringBytes"
		v.Text = base64.StdEncoding.EncodeToString([]byte(v.Text))
	}
	if v.Kind == "stringBytes" {
		if raw, err := base64.StdEncoding.DecodeString(v.Text); err == nil && utf8.Valid(raw) {
			v.Kind = "string"
			v.Text = string(raw)
		}
	}
	for i := range v.Items {
		v.Items[i] = ucWireCanonical(v.Items[i])
	}
	for i := range v.Entries {
		v.Entries[i].Key = ucWireCanonical(v.Entries[i].Key)
		v.Entries[i].Value = ucWireCanonical(v.Entries[i].Value)
	}
	switch v.Kind {
	case "list", "set":
		v.Text = ""
		v.Entries = nil
		if v.Items == nil {
			v.Items = []ucWireValue{}
		}
		if v.Kind == "set" {
			slices.SortFunc(v.Items, func(a, b ucWireValue) int { return strings.Compare(ucWireText(a), ucWireText(b)) })
		}
	case "record", "map", "call":
		v.Items = nil
		if v.Kind != "call" {
			v.Text = ""
		}
		if v.Entries == nil {
			v.Entries = []ucWireEntry{}
		}
		slices.SortFunc(v.Entries, func(a, b ucWireEntry) int { return strings.Compare(ucWireText(a.Key), ucWireText(b.Key)) })
	case "encoded":
		v.Text = ""
		v.Entries = nil
	default:
		v.Items = nil
		v.Entries = nil
	}
	return v
}
func ucWireText(v ucWireValue) string { b, _ := json.Marshal(v); return string(b) }
func ucWireHash(input ucWireInput) string {
	values := map[string]ucWireValue{}
	for name, value := range input.Values {
		values[name] = ucWireCanonical(value)
	}
	b, _ := json.Marshal(values)
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}
func ucCodecCall(fn reflect.Value, argument reflect.Value) reflect.Value {
	if !fn.IsValid() || fn.Type().NumIn() != 1 {
		ucFail("lang.golang.input-codec-failed", "a model codec has no unary native implementation", nil)
	}
	argument = ucWireAdapt(argument, fn.Type().In(0))
	results := fn.Call([]reflect.Value{argument})
	if len(results) > 1 {
		if !results[len(results)-1].Type().Implements(reflect.TypeFor[error]()) {
			ucFail("lang.golang.input-codec-failed", "a model decoder returned an unsupported native signature", nil)
		}
		if !results[len(results)-1].IsNil() {
			ucFail("lang.golang.input-outside-domain", "the model decoder rejected its representation: "+results[len(results)-1].Interface().(error).Error(), nil)
		}
		results = results[:len(results)-1]
	}
	if len(results) != 1 {
		ucFail("lang.golang.input-codec-failed", "a model codec must return exactly one value", nil)
	}
	return results[0]
}
func ucWireAdapt(value reflect.Value, target reflect.Type) reflect.Value {
	if !value.IsValid() {
		return reflect.Zero(target)
	}
	if value.Type().AssignableTo(target) {
		return value
	}
	if target.Kind() == reflect.Pointer {
		v := reflect.New(target.Elem())
		v.Elem().Set(ucWireAdapt(value, target.Elem()))
		return v
	}
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		return ucWireAdapt(value.Elem(), target)
	}
	if value.Type().ConvertibleTo(target) {
		return value.Convert(target)
	}
	ucFail("lang.golang.input-codec-failed", "a model codec's native type does not match its declaration", nil)
	return reflect.Value{}
}
func ucFreezeInputs(s *ucContract, values map[string]reflect.Value) ucWireInput {
	ucCheckInputAliases(s, values)
	input := ucWireInput{Values: map[string]ucWireValue{}}
	for _, symbol := range s.Symbols {
		value, ok := values[symbol.Name]
		if !ok {
			ucFail("lang.golang.input-codec-failed", "an input symbol is missing: "+symbol.Name, nil)
		}
		input.Values[symbol.Name] = ucWireCanonical(ucFreezeValue(value, symbol.Type, s, map[uintptr]bool{}, 0))
	}
	return input
}
func ucFreezeValue(value reflect.Value, model string, s *ucContract, seen map[uintptr]bool, depth int) ucWireValue {
	if depth > 128 {
		ucFail("lang.golang.input-codec-failed", "a recorded input exceeds the finite model depth", nil)
	}
	kind, args := ucSplitType(model)
	if kind == "optional" {
		if !value.IsValid() || ((value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil()) {
			return ucWireValue{Kind: "none"}
		}
		return ucFreezeValue(value, args[0], s, seen, depth+1)
	}
	if codec, ok := s.Codecs[model]; ok {
		encoded := ucCodecCall(codec.Encode, value)
		representation := ucFreezeValue(encoded, codec.Representation, s, seen, depth+1)
		decoded := ucCodecCall(codec.Decode, ucThawValue(representation, encoded.Type(), codec.Representation, s, depth+1))
		roundtrip := ucFreezeValue(ucCodecCall(codec.Encode, decoded), codec.Representation, s, seen, depth+1)
		if ucWireText(ucWireCanonical(representation)) != ucWireText(ucWireCanonical(roundtrip)) {
			ucFail("lang.golang.input-codec-failed", "a model codec does not preserve its canonical representation", nil)
		}

		return ucWireValue{Kind: "encoded", Items: []ucWireValue{representation}}
	}
	native := value
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			ucFail("lang.golang.input-codec-failed", "a required input is nil", nil)
		}
		if value.Kind() == reflect.Pointer {
			pointer := value.Pointer()
			if seen[pointer] {
				ucFail("lang.golang.input-codec-failed", "a cyclic input needs an explicit model codec", nil)
			}
			seen[pointer] = true
			defer delete(seen, pointer)
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		ucFail("lang.golang.input-codec-failed", "a required input has no native value", nil)
	}
	scalar := func(k, t string) ucWireValue { return ucWireValue{Kind: k, Text: t} }
	switch kind {
	case "bool":
		return scalar("bool", strconv.FormatBool(value.Bool()))
	case "int":
		switch value.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return scalar("int", strconv.FormatInt(value.Int(), 10))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return scalar("int", strconv.FormatUint(value.Uint(), 10))
		}
	case "float":
		f := value.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			ucFail("lang.golang.input-codec-failed", "a recorded number must be finite", nil)
		}
		return scalar("float", strconv.FormatFloat(f, 'f', -1, value.Type().Bits()))
	case "string":
		if !utf8.ValidString(value.String()) {
			return scalar("stringBytes", base64.StdEncoding.EncodeToString([]byte(value.String())))
		}
		return scalar("string", value.String())
	case "bytes":
		return scalar("bytes", base64.StdEncoding.EncodeToString(value.Bytes()))
	case "time":
		return scalar("time", value.Interface().(time.Time).UTC().Format(time.RFC3339Nano))
	case "duration":
		return scalar("duration", time.Duration(value.Int()).String())
	case "list", "set":
		out := ucWireValue{Kind: kind, Items: []ucWireValue{}}
		if kind == "set" {
			for _, key := range value.MapKeys() {
				out.Items = append(out.Items, ucFreezeValue(key, args[0], s, seen, depth+1))
			}
		} else {
			for i := 0; i < value.Len(); i++ {
				out.Items = append(out.Items, ucFreezeValue(value.Index(i), args[0], s, seen, depth+1))
			}
		}
		return ucWireCanonical(out)
	case "map":
		out := ucWireValue{Kind: "map", Entries: []ucWireEntry{}}
		for _, key := range value.MapKeys() {
			out.Entries = append(out.Entries, ucWireEntry{ucFreezeValue(key, args[0], s, seen, depth+1), ucFreezeValue(value.MapIndex(key), args[1], s, seen, depth+1)})
		}
		return ucWireCanonical(out)
	case "call":
		name, parameters := ucCallParts(value)
		out := ucWireValue{Kind: "call", Text: name, Entries: []ucWireEntry{}}
		var operation ucOperation
		for _, op := range s.Types[model].Calls {
			if op.Name == name || op.GoName == name || ucLastModelSegment(op.Name) == name {
				operation = op
				break
			}
		}
		if operation.Name == "" {
			ucFail("lang.golang.input-codec-failed", "a recorded call has no declared operation: "+name, nil)
		}
		out.Text = ucLastModelSegment(operation.Name)
		for i, name := range operation.ParamNames {
			field := parameters[operation.GoName+"."+ucUpperModelFirst(name)]
			if !field.IsValid() {
				field = parameters[name]
			}
			if !field.IsValid() {
				ucFail("lang.golang.input-codec-failed", "a recorded call parameter is unavailable: "+name, nil)
			}
			out.Entries = append(out.Entries, ucWireEntry{scalar("string", name), ucFreezeValue(field, operation.Params[i], s, seen, depth+1)})
		}
		return out
	case "replies":
		return ucFreezeScript(value.FieldByName("Replies").Interface().([]map[string]any), s.Types[model].Double.Operations[args[0]], s, seen, depth)
	}
	ty := s.Types[model]
	if ty.Form == "enum" {
		return scalar("enum", value.String())
	}
	if ty.Form == "interface" {
		scripts, ok := native.Interface().(interface {
			_unicodeInputScripts() map[string][]map[string]any
		})
		if !ok {
			ucFail("lang.golang.input-codec-failed", "an interface input is not a replayable scripted double", nil)
		}
		out := ucWireValue{Kind: "record", Entries: []ucWireEntry{}}
		for name, script := range scripts._unicodeInputScripts() {
			out.Entries = append(out.Entries, ucWireEntry{scalar("string", name), ucFreezeScript(script, ty.Double.Operations[name], s, seen, depth+1)})
		}
		return ucWireCanonical(out)
	}
	if value.Kind() != reflect.Struct {
		ucFail("lang.golang.input-codec-failed", "a model record has no reconstructable native shape: "+model, nil)
	}
	out := ucWireValue{Kind: "record", Entries: []ucWireEntry{}}
	allowed := map[string]bool{}
	for _, field := range ucFields(s.Types, model) {
		owner := ucModelFieldOwner(s.Types, model, field.Name)
		index, ok := ucOwnerIndex(value.Type(), s.Types, model, owner)
		if !ok {
			ucFail("lang.golang.input-codec-failed", "a model field has no native owner: "+field.Name, nil)
		}
		ownerType := value.Type()
		for _, i := range index {
			ownerType = ucNativeBaseType(ownerType).Field(i).Type
		}
		ownerType = ucNativeBaseType(ownerType)
		f, ok := ownerType.FieldByName(field.GoName)
		if !ok {
			ucFail("lang.golang.input-codec-failed", "a model field is unavailable: "+field.Name, nil)
		}
		indices := append(append([]int{}, index...), f.Index...)
		parts := make([]string, len(indices))
		for i, n := range indices {
			parts[i] = strconv.Itoa(n)
		}
		allowed[strings.Join(parts, ".")] = true
		v := ucOwnedField(value, s.Types, model, field, false)
		out.Entries = append(out.Entries, ucWireEntry{scalar("string", field.Name), ucFreezeValue(v, field.Type, s, seen, depth+1)})
	}
	var hidden func(reflect.Value, string) bool
	hidden = func(v reflect.Value, prefix string) bool {
		v = ucDeref(v)
		if !v.IsValid() || v.Kind() != reflect.Struct {
			return false
		}
		for i := 0; i < v.NumField(); i++ {
			key := strconv.Itoa(i)
			if prefix != "" {
				key = prefix + "." + key
			}
			if allowed[key] {
				continue
			}
			prefixAllowed := false
			for path := range allowed {
				if strings.HasPrefix(path, key+".") {
					prefixAllowed = true
					break
				}
			}
			if prefixAllowed {
				if hidden(v.Field(i), key) {
					return true
				}
			} else if !v.Field(i).IsZero() {
				return true
			}
		}
		return false
	}
	if hidden(value, "") {
		ucFail("lang.golang.input-codec-failed", "input type "+model+" has nonzero implementation state outside its model; declare a codec", nil)
	}
	return ucWireCanonical(out)
}
func ucLastModelSegment(text string) string {
	_, tail, ok := strings.Cut(text, ".")
	for ok {
		text = tail
		_, tail, ok = strings.Cut(text, ".")
	}
	return text
}
func ucUpperModelFirst(text string) string {
	if text == "" {
		return ""
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

func ucFreezeScript(script []map[string]any, op ucReplyOperation, s *ucContract, seen map[uintptr]bool, depth int) ucWireValue {
	out := ucWireValue{Kind: "list", Items: []ucWireValue{}}
	for _, reply := range script {
		item := ucWireValue{Kind: "record", Entries: []ucWireEntry{}}
		if result, ok := reply["returns"]; ok {
			value := ucWireValue{Kind: "none"}
			if op.Result != "" {
				value = ucFreezeValue(reflect.ValueOf(result), op.Result, s, seen, depth+1)
			}
			item.Entries = append(item.Entries, ucWireEntry{ucWireValue{Kind: "string", Text: "returns"}, value})
		} else {
			name, ok := reply["raises"].(string)
			if !ok {
				ucFail("lang.golang.input-codec-failed", "a response script has no declared error", nil)
			}
			item.Entries = append(item.Entries, ucWireEntry{ucWireValue{Kind: "string", Text: "raises"}, ucWireValue{Kind: "string", Text: name}}, ucWireEntry{ucWireValue{Kind: "string", Text: "fields"}, ucFreezeValue(reflect.ValueOf(reply["fields"]), name, s, seen, depth+1)})
		}
		out.Items = append(out.Items, item)
	}
	return ucWireCanonical(out)
}
func ucWireEntries(v ucWireValue) map[string]ucWireValue {
	out := map[string]ucWireValue{}
	for _, entry := range v.Entries {
		if entry.Key.Kind != "string" {
			ucFail("lang.golang.input-outside-domain", "a record key is not a field name", nil)
		}
		if _, ok := out[entry.Key.Text]; ok {
			ucFail("lang.golang.input-outside-domain", "a model field occurs twice", nil)
		}
		out[entry.Key.Text] = entry.Value
	}
	return out
}
func ucThawValue(w ucWireValue, target reflect.Type, model string, s *ucContract, depth int) reflect.Value {
	if depth > 128 {
		ucFail("lang.golang.input-outside-domain", "a recorded input exceeds the finite model depth", nil)
	}
	kind, args := ucSplitType(model)
	if kind == "optional" {
		if w.Kind == "none" {
			return reflect.Zero(target)
		}
		if target.Kind() == reflect.Pointer {
			out := reflect.New(target.Elem())
			out.Elem().Set(ucThawValue(w, target.Elem(), args[0], s, depth+1))
			return out
		}
		return ucThawValue(w, target, args[0], s, depth+1)
	}
	if codec, ok := s.Codecs[model]; ok {
		if w.Kind != "encoded" || len(w.Items) != 1 {
			ucFail("lang.golang.input-outside-domain", "a codec input needs exactly one encoded representation", nil)
		}
		representation := ucThawValue(w.Items[0], codec.Decode.Type().In(0), codec.Representation, s, depth+1)
		return ucWireAdapt(ucCodecCall(codec.Decode, representation), target)
	}
	if target.Kind() == reflect.Pointer {
		out := reflect.New(target.Elem())
		out.Elem().Set(ucThawValue(w, target.Elem(), model, s, depth+1))
		return out
	}
	out := reflect.New(target).Elem()
	fail := func() {
		ucFail("lang.golang.input-outside-domain", "the recorded value "+w.Kind+" does not fit "+model, nil)
	}
	scalar := func(expected string) {
		if w.Kind != expected {
			fail()
		}
	}
	switch kind {
	case "bool":
		scalar("bool")
		v, err := strconv.ParseBool(w.Text)
		if err != nil {
			fail()
		}
		out.SetBool(v)
	case "int":
		scalar("int")
		switch out.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			v, err := strconv.ParseUint(w.Text, 10, out.Type().Bits())
			if err != nil {
				fail()
			}
			out.SetUint(v)
		default:
			v, err := strconv.ParseInt(w.Text, 10, out.Type().Bits())
			if err != nil {
				fail()
			}
			out.SetInt(v)
		}
	case "float":
		scalar("float")
		v, err := strconv.ParseFloat(w.Text, out.Type().Bits())
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			fail()
		}
		out.SetFloat(v)
	case "string":
		if w.Kind == "stringBytes" {
			raw, err := base64.StdEncoding.DecodeString(w.Text)
			if err != nil {
				fail()
			}
			out.SetString(string(raw))
		} else {
			scalar("string")
			out.SetString(w.Text)
		}
	case "bytes":
		scalar("bytes")
		v, err := base64.StdEncoding.DecodeString(w.Text)
		if err != nil {
			fail()
		}
		out.SetBytes(v)
	case "time":
		scalar("time")
		v, err := time.Parse(time.RFC3339Nano, w.Text)
		if err != nil {
			fail()
		}
		out.Set(reflect.ValueOf(v))
	case "duration":
		scalar("duration")
		v, err := time.ParseDuration(w.Text)
		if err != nil {
			fail()
		}
		out.SetInt(int64(v))
	case "list":
		scalar("list")
		out.Set(reflect.MakeSlice(target, len(w.Items), len(w.Items)))
		for i, item := range w.Items {
			out.Index(i).Set(ucThawValue(item, target.Elem(), args[0], s, depth+1))
		}
	case "set":
		scalar("set")
		out.Set(reflect.MakeMap(target))
		for _, item := range w.Items {
			key := ucThawValue(item, target.Key(), args[0], s, depth+1)
			out.SetMapIndex(key, reflect.Zero(target.Elem()))
		}
	case "map":
		scalar("map")
		out.Set(reflect.MakeMap(target))
		for _, entry := range w.Entries {
			out.SetMapIndex(ucThawValue(entry.Key, target.Key(), args[0], s, depth+1), ucThawValue(entry.Value, target.Elem(), args[1], s, depth+1))
		}
	case "call":
		scalar("call")
		var operation ucOperation
		for _, op := range s.Types[model].Calls {
			if ucLastModelSegment(op.Name) == w.Text {
				operation = op
				break
			}
		}
		if operation.Name == "" {
			fail()
		}
		out.FieldByName("Operation").SetString(operation.Name)
		entries := ucWireEntries(w)
		arguments := out.FieldByName("Arguments")
		arguments.Set(reflect.MakeMap(arguments.Type()))
		if len(entries) != len(operation.ParamNames) {
			fail()
		}
		for i, name := range operation.ParamNames {
			v, ok := entries[name]
			if !ok || i >= len(operation.ParamTypes) {
				fail()
			}
			argument := ucThawValue(v, operation.ParamTypes[i], operation.Params[i], s, depth+1)
			arguments.SetMapIndex(reflect.ValueOf(name), argument)
		}
	case "replies":
		scalar("list")
		out.FieldByName("Replies").Set(reflect.ValueOf(ucThawScript(w, s.Types[model].Double.Operations[args[0]], s, depth)))
	default:
		ty := s.Types[model]
		if ty.Form == "enum" {
			scalar("enum")
			if !slices.Contains(ty.Values, w.Text) {
				fail()
			}
			out.SetString(w.Text)
			break
		}
		scalar("record")
		entries := ucWireEntries(w)
		if ty.Form == "interface" {
			scripts := map[string][]map[string]any{}
			for name, op := range ty.Double.Operations {
				value, ok := entries[name]
				if !ok {
					fail()
				}
				scripts[name] = ucThawScript(value, op, s, depth+1)
			}
			out.Set(ty.Double.New(scripts))
			break
		}
		fields := ucFields(s.Types, model)
		if len(entries) != len(fields) {
			fail()
		}
		for _, field := range fields {
			v, ok := entries[field.Name]
			if !ok {
				fail()
			}
			f := ucOwnedField(out, s.Types, model, field, true)
			if !f.IsValid() || !f.CanSet() {
				fail()
			}
			f.Set(ucThawValue(v, f.Type(), field.Type, s, depth+1))
		}
	}
	return out
}
func ucThawScript(w ucWireValue, op ucReplyOperation, s *ucContract, depth int) []map[string]any {
	if w.Kind != "list" || len(w.Items) == 0 {
		ucFail("lang.golang.input-outside-domain", "a response script must be a nonempty list", nil)
	}
	out := make([]map[string]any, len(w.Items))
	for i, item := range w.Items {
		fields := ucWireEntries(item)
		if value, ok := fields["returns"]; ok && len(fields) == 1 {
			var native any
			if op.Result != "" {
				native = ucThawValue(value, op.Signature.Out(0), op.Result, s, depth+1).Interface()
			} else if value.Kind != "none" {
				ucFail("lang.golang.input-outside-domain", "a void response must return none", nil)
			}
			out[i] = map[string]any{"returns": native}
		} else {
			errorValue, ok := fields["raises"]
			data, present := fields["fields"]
			native, declared := op.Errors[errorValue.Text]
			if !ok || !present || !declared || len(fields) != 2 {
				ucFail("lang.golang.input-outside-domain", "a response error is not declared", nil)
			}
			out[i] = map[string]any{"raises": errorValue.Text, "fields": ucThawValue(data, native, errorValue.Text, s, depth+1).Interface()}
		}
	}
	return out
}

func ucThawInputs(input ucWireInput, native []reflect.Type, s *ucContract) map[string]reflect.Value {
	if len(input.Values) != len(s.Symbols) || len(native) != len(s.Symbols) {
		ucFail("lang.golang.input-outside-domain", "the recorded inputs do not match this check's symbols", nil)
	}
	values := map[string]reflect.Value{}
	for i, symbol := range s.Symbols {
		value, ok := input.Values[symbol.Name]
		if !ok {
			ucFail("lang.golang.input-outside-domain", "a recorded input symbol is missing: "+symbol.Name, nil)
		}
		values[symbol.Name] = ucThawValue(value, native[i], symbol.Type, s, 0)
	}
	roundtrip := ucFreezeInputs(s, values)
	if ucWireHash(roundtrip) != ucWireHash(input) {
		ucFail("lang.golang.input-outside-domain", fmt.Sprintf("input roundtrip changed its canonical value for %s", s.ID), nil)
	}
	return values
}

func ucEncodedLiteral(fn, argument reflect.Value, target reflect.Type) reflect.Value {
	return ucWireAdapt(ucCodecCall(fn, argument), target)
}

package monitor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ModelType is what redaction reads of one named type: its form, the types of
// its fields, inherited ones included, the representation type of its codec
// (empty without one), and whether it is labelled sensitive.
//
//uc:realizes monitor.ModelType
type ModelType struct {
	Form      string            `json:"form"`
	Fields    map[string]string `json:"fields"`
	Codec     string            `json:"codec"`
	Sensitive bool              `json:"sensitive"`
}

// Schema is the type information redaction reads: the named types by their
// identity, and each operation's parameter types by parameter name.
//
//uc:realizes monitor.Schema
type Schema struct {
	Types      map[string]ModelType         `json:"types"`
	Operations map[string]map[string]string `json:"operations"`
}

// Redact replaces in value, of the model type model, every value whose type is
// labelled sensitive by a value of the kind redacted whose text is the
// HMAC-SHA-256 of the original value's canonical encoding under key, in
// lowercase hexadecimal: the symbol itself, a field, an element of a list or a
// set, a key or a value of a map, an optional's content, a codec's
// representation or an argument of a call value. Equal values under one key
// stay equal, and the original cannot be read back.
//
//uc:realizes monitor.redact
func Redact(value Value, model string, schema Schema, key []byte) Value {
	return redactAt(value, strings.TrimSpace(model), schema, key, 0)
}

// maxDepth bounds the walk, as the evidence store bounds model values.
const maxDepth = 128

func redactAt(value Value, model string, schema Schema, key []byte, depth int) Value {
	if depth > maxDepth {
		return value
	}
	kind, args := splitType(model)
	switch kind {
	case "optional":
		if value.Kind == "none" || len(args) != 1 {
			return value
		}
		return redactAt(value, args[0], schema, key, depth+1)
	case "list", "set":
		if len(args) != 1 || value.Kind != kind {
			return value
		}
		out := Value{Kind: value.Kind, Text: value.Text, Items: make([]Value, 0, len(value.Items))}
		for _, item := range value.Items {
			out.Items = append(out.Items, redactAt(item, args[0], schema, key, depth+1))
		}
		if kind == "set" {
			sortItems(out.Items)
		}
		return out
	case "map":
		if len(args) != 2 || value.Kind != "map" {
			return value
		}
		out := Value{Kind: value.Kind, Text: value.Text, Entries: make([]ValueEntry, 0, len(value.Entries))}
		for _, entry := range value.Entries {
			out.Entries = append(out.Entries, ValueEntry{
				Key:   redactAt(entry.Key, args[0], schema, key, depth+1),
				Value: redactAt(entry.Value, args[1], schema, key, depth+1),
			})
		}
		sortEntries(out.Entries)
		return out
	case "call":
		if value.Kind != "call" {
			return value
		}
		params, ok := operationParams(schema, value.Text, args)
		if !ok {
			return value
		}
		return redactEntries(value, params, schema, key, depth)
	}
	named, ok := schema.Types[model]
	if !ok {
		return value
	}
	if named.Sensitive {
		return redacted(value, key)
	}
	switch {
	case named.Codec != "" && value.Kind == "encoded" && len(value.Items) == 1:
		return Value{Kind: value.Kind, Text: value.Text, Items: []Value{redactAt(value.Items[0], named.Codec, schema, key, depth+1)}}
	case value.Kind == "record" && named.Fields != nil:
		return redactEntries(value, named.Fields, schema, key, depth)
	}
	return value
}

// redactEntries redacts the values of a record's fields or a call's
// arguments by the types their names give.
func redactEntries(value Value, types map[string]string, schema Schema, key []byte, depth int) Value {
	out := Value{Kind: value.Kind, Text: value.Text, Entries: make([]ValueEntry, 0, len(value.Entries))}
	for _, entry := range value.Entries {
		child := entry.Value
		if model, ok := types[entry.Key.Text]; ok && entry.Key.Kind == "string" {
			child = redactAt(child, model, schema, key, depth+1)
		}
		out.Entries = append(out.Entries, ValueEntry{Key: entry.Key, Value: child})
	}
	sortEntries(out.Entries)
	return out
}

// operationParams finds the parameter types of the operation a call value
// names by its name: among the operations of a type member of call<…>, or a
// member that is itself the operation.
func operationParams(schema Schema, name string, members []string) (map[string]string, bool) {
	for _, member := range members {
		if params, ok := schema.Operations[member+"."+name]; ok {
			return params, true
		}
		if params, ok := schema.Operations[member]; ok && member[strings.LastIndexByte(member, '.')+1:] == name {
			return params, true
		}
	}
	return nil, false
}

// redacted is the redacted form of value under key.
func redacted(value Value, key []byte) Value {
	mac := hmac.New(sha256.New, key)
	mac.Write(canonical(value))
	return Value{Kind: "redacted", Text: hex.EncodeToString(mac.Sum(nil))}
}

// splitType splits a model type expression into its constructor and
// arguments: list<T> gives list and [T], map<K, V> gives map and [K, V]; a
// named or builtin type gives itself and no arguments.
func splitType(model string) (string, []string) {
	open := strings.IndexByte(model, '<')
	if open < 0 || !strings.HasSuffix(model, ">") {
		return model, nil
	}
	kind, inner := model[:open], model[open+1:len(model)-1]
	var args []string
	depth, start := 0, 0
	for i, r := range inner {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	return kind, append(args, strings.TrimSpace(inner[start:]))
}

package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/monitor"
)

// InvalidTraffic is the error of a file under .unicode/traffic that is not a
// collected call a monitored program wrote.
//
//uc:realizes evidence.InvalidTraffic
type InvalidTraffic struct {
	Cause string
}

func (e InvalidTraffic) Error() string { return "not a collected call: " + e.Cause }

// Traffic is the collected calls under .unicode/traffic, and the diagnostics
// of the files that cannot be replayed.
//
//uc:realizes evidence.Traffic
type Traffic struct {
	Collected   []monitor.Collected `json:"collected"`
	Diagnostics []diag.Diagnostic   `json:"diagnostics"`
}

// DecodeCollected reads one collected call from the JSON monitor wrote: one
// object with exactly its fields, a call value and an optional receiver in
// the canonical encoding, and an empty or complete commit.
//
//uc:realizes evidence.decodeCollected
func DecodeCollected(data []byte) (monitor.Collected, error) {
	var out monitor.Collected
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return out, InvalidTraffic{err.Error()}
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return out, InvalidTraffic{"more data follows the collected call"}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 5 {
		return out, InvalidTraffic{"a collected call has exactly the fields op, receiver, call, commit and at"}
	}
	if strings.TrimSpace(out.Op) == "" || strings.ContainsAny(out.Op, `/\`) {
		return out, InvalidTraffic{"the operation identity is empty or not a directory name"}
	}
	if out.Commit != "" && !hexDigest(out.Commit, 40) && !hexDigest(out.Commit, 64) {
		return out, InvalidTraffic{"the commit is neither empty nor a complete Git commit hash"}
	}
	if out.Call.Kind != "call" {
		return out, InvalidTraffic{"the call value is not of the kind call"}
	}
	values := []*monitor.Value{&out.Call}
	if out.Receiver != nil {
		values = append(values, out.Receiver)
	}
	for _, value := range values {
		normalized, err := normalizeValue(langValue(*value), 0)
		if err != nil {
			return out, InvalidTraffic{err.Error()}
		}
		*value = monitorValue(normalized)
	}
	out.At = out.At.UTC()
	return out, nil
}

// ReadTraffic reads the collected calls under root's .unicode/traffic, each
// <operation>/<input hash>.json, in the order of their paths. A missing
// directory holds none. A file that is not a collected call, names another
// operation or hash, calls an operation g lacks or not with its parameters,
// or is a symbolic link gets evidence.invalid-traffic.
//
//uc:realizes evidence.readTraffic
func ReadTraffic(root string, g *graph.Graph) Traffic {
	out := Traffic{Collected: []monitor.Collected{}, Diagnostics: []diag.Diagnostic{}}
	base := filepath.Join(root, ".unicode", "traffic")
	invalid := func(path, cause, fix string) {
		out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.invalid-traffic", filepath.ToSlash(path), cause, fix))
	}
	ops, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return out
	}
	if err != nil {
		invalid(".unicode/traffic", err.Error(), "restore read access to the collected calls")
		return out
	}
	for _, op := range ops {
		dir := ".unicode/traffic/" + op.Name()
		if op.Type()&fs.ModeSymlink != 0 || !op.IsDir() {
			invalid(dir, "the entry is not a directory of an operation's collected calls", "keep only the directories that a monitored program writes under traffic/")
			continue
		}
		files, err := os.ReadDir(filepath.Join(base, op.Name()))
		if err != nil {
			invalid(dir, err.Error(), "restore read access to the collected calls")
			continue
		}
		params, known := parameters(g, op.Name())
		for _, file := range files {
			path := dir + "/" + file.Name()
			if file.Type()&fs.ModeSymlink != 0 || !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".json" {
				invalid(path, "the collected call is not a regular JSON file", "restore the file a monitored program wrote")
				continue
			}
			data, err := os.ReadFile(filepath.Join(base, op.Name(), file.Name()))
			if err != nil {
				invalid(path, err.Error(), "restore read access to the collected call")
				continue
			}
			collected, err := DecodeCollected(data)
			switch {
			case err != nil:
				invalid(path, err.Error(), "restore the collected call a monitored program wrote, or remove it")
			case collected.Op != op.Name() || collectedHash(collected)+".json" != file.Name():
				invalid(path, "the file is not named by its operation and the hash of its receiver and call value", "restore the file's name, or remove it")
			case !known:
				invalid(path, "the collected call names "+collected.Op+", which is no operation of the design", "remove the collected calls of an operation the design no longer has")
			case !sameParameters(collected.Call, params):
				invalid(path, "the call value's arguments are not the parameters of "+collected.Op, "remove the collected calls of an operation whose signature changed")
			default:
				out.Collected = append(out.Collected, collected)
			}
		}
	}
	return out
}

// TrafficInputs gives the inputs that collected calls give the single-call
// contracts of g judged at runtime (lang.MonitoredCalls), docs/language.md
// E29: each collected call of a contract's operation gives one input, its
// receiver the receiver's symbol and each argument the symbol in the same
// place. A contract that no collected call gives an input is left out.
//
//uc:realizes evidence.trafficInputs
func TrafficInputs(g *graph.Graph, collected []monitor.Collected) map[string][]lang.Input {
	out := map[string][]lang.Input{}
	monitored := lang.MonitoredCalls(g)
	for _, id := range slices.Sorted(maps.Keys(monitored)) {
		call := monitored[id]
		params, known := parameters(g, call.Op)
		if !known || len(params) != len(call.Args) {
			continue
		}
		for _, c := range collected {
			if c.Op != call.Op || (call.Receiver != "") != (c.Receiver != nil) {
				continue
			}
			arguments := map[string]monitor.Value{}
			for _, entry := range c.Call.Entries {
				arguments[entry.Key.Text] = entry.Value
			}
			input := lang.Input{Values: map[string]lang.Value{}}
			if call.Receiver != "" {
				input.Values[call.Receiver] = langValue(*c.Receiver)
			}
			complete := len(arguments) == len(params)
			for i, param := range params {
				value, ok := arguments[param]
				complete = complete && ok
				input.Values[call.Args[i]] = langValue(value)
			}
			if complete {
				out[id] = append(out[id], input)
			}
		}
	}
	return out
}

// collectedHash is the hash a collected call is stored under: that of its
// receiver and its call value.
func collectedHash(c monitor.Collected) string {
	values := map[string]monitor.Value{"call": c.Call}
	if c.Receiver != nil {
		values["receiver"] = *c.Receiver
	}
	return monitor.InputHash(values)
}

// parameters are the parameter names of the design operation op, in order,
// read from its signature (name: type, …): types hold no parentheses, and
// their commas are inside angle brackets.
func parameters(g *graph.Graph, op string) ([]string, bool) {
	n := g.Node(op)
	if n == nil || n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindOp || len(n.Attrs["signature"]) == 0 {
		return nil, false
	}
	signature := n.Attrs["signature"][0]
	open, closing := strings.IndexByte(signature, '('), strings.IndexByte(signature, ')')
	if open < 0 || closing < open {
		return nil, false
	}
	params := []string{}
	depth, start, inner := 0, 0, signature[open+1:closing]
	for i := 0; i <= len(inner); i++ {
		if i < len(inner) {
			switch inner[i] {
			case '<':
				depth++
			case '>':
				depth--
			}
			if inner[i] != ',' || depth > 0 {
				continue
			}
		}
		if name, _, ok := strings.Cut(inner[start:i], ":"); ok {
			params = append(params, strings.TrimSpace(name))
		}
		start = i + 1
	}
	return params, true
}

// sameParameters tells whether a call value has exactly the parameters
// params as its arguments.
func sameParameters(call monitor.Value, params []string) bool {
	if len(call.Entries) != len(params) {
		return false
	}
	for _, entry := range call.Entries {
		if entry.Key.Kind != "string" || !slices.Contains(params, entry.Key.Text) {
			return false
		}
	}
	return true
}

// langValue and monitorValue convert between the two Go types of a model
// value; their JSON is the same.
func langValue(v monitor.Value) lang.Value {
	out := lang.Value{Kind: v.Kind, Text: v.Text}
	if v.Items != nil {
		out.Items = []lang.Value{}
		for _, item := range v.Items {
			out.Items = append(out.Items, langValue(item))
		}
	}
	if v.Entries != nil {
		out.Entries = []lang.ValueEntry{}
		for _, entry := range v.Entries {
			out.Entries = append(out.Entries, lang.ValueEntry{Key: langValue(entry.Key), Value: langValue(entry.Value)})
		}
	}
	return out
}

func monitorValue(v lang.Value) monitor.Value {
	out := monitor.Value{Kind: v.Kind, Text: v.Text}
	if v.Items != nil {
		out.Items = []monitor.Value{}
		for _, item := range v.Items {
			out.Items = append(out.Items, monitorValue(item))
		}
	}
	if v.Entries != nil {
		out.Entries = []monitor.ValueEntry{}
		for _, entry := range v.Entries {
			out.Entries = append(out.Entries, monitor.ValueEntry{Key: monitorValue(entry.Key), Value: monitorValue(entry.Value)})
		}
	}
	return out
}

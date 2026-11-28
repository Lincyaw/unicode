package monitor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"pgregory.net/rapid"
)

// RatesText is a set of sampling rates and the UNICODE_MONITOR_RATE text that
// writes it.
//
//uc:realizes monitor.RatesText
type RatesText struct {
	Rates Rates
	Text  string
}

// SummarySample is a record period's monitored calls, with the 95th
// percentile docs/language.md E16 gives each property and the within budgets.
//
//uc:realizes monitor.SummarySample
type SummarySample struct {
	Calls  []JudgedCall
	Build  Build
	From   time.Time
	To     time.Time
	P95    map[string]time.Duration
	Within map[string]time.Duration
}

// RedactionSample is a model value of a model type, the schema and key it is
// redacted with, and the value redaction gives by definition.
//
//uc:realizes monitor.RedactionSample
type RedactionSample struct {
	Value    Value
	Model    string
	Schema   Schema
	Key      []byte
	Expected Value
}

var lowerNames = rapid.StringMatching(`[a-z]{1,6}`)

// genSamplingOp draws an operation: about half the time one that rates gives
// a rate, otherwise any text of at most 8 characters.
//
//uc:generates monitor.sampling-follows-rates.op
func genSamplingOp(rates Rates) *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if len(rates.Ops) > 0 && rapid.Bool().Draw(t, "known") {
			keys := make([]string, 0, len(rates.Ops))
			for key := range rates.Ops {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			return rapid.SampledFrom(keys).Draw(t, "op")
		}
		return rapid.StringN(0, 8, -1).Draw(t, "op")
	})
}

// genRatesText draws a fallback rate and zero to three operations' rates
// from 0 to 1, each operation one to three dot-separated lowercase names,
// and writes them as fallback[,operation=rate]... in a random order, each
// rate in the shortest decimal that reads back exactly.
//
//uc:generates monitor.rates-read-their-text.sample
func genRatesText() *rapid.Generator[RatesText] {
	return rapid.Custom(func(t *rapid.T) RatesText {
		rate := rapid.Float64Range(0, 1)
		rates := Rates{Fallback: rate.Draw(t, "fallback"), Ops: map[string]float64{}}
		for range rapid.IntRange(0, 3).Draw(t, "ops") {
			segments := rapid.SliceOfN(lowerNames, 1, 3).Draw(t, "segments")
			rates.Ops[strings.Join(segments, ".")] = rate.Draw(t, "rate")
		}
		ops := make([]string, 0, len(rates.Ops))
		for op := range rates.Ops {
			ops = append(ops, op)
		}
		slices.Sort(ops)
		ops = rapid.Permutation(ops).Draw(t, "order")
		items := []string{strconv.FormatFloat(rates.Fallback, 'f', -1, 64)}
		for _, op := range ops {
			items = append(items, op+"="+strconv.FormatFloat(rates.Ops[op], 'f', -1, 64))
		}
		return RatesText{Rates: rates, Text: strings.Join(items, ",")}
	})
}

// genMalformedRates draws a text that is not fallback[,operation=rate]...:
// empty, a fallback that is no number, a rate below 0 or above 1, an item
// without =, with an empty operation or with a rate that is no number, or a
// trailing comma.
//
//uc:generates monitor.malformed-rates-are-refused.text
func genMalformedRates() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		valid := strconv.FormatFloat(rapid.Float64Range(0, 1).Draw(t, "valid"), 'f', -1, 64)
		op := lowerNames.Draw(t, "operation")
		outside := strconv.FormatFloat(rapid.OneOf(rapid.Float64Range(-10, -0.001), rapid.Float64Range(1.001, 10)).Draw(t, "outside"), 'f', -1, 64)
		word := lowerNames.Draw(t, "word")
		return rapid.SampledFrom([]string{
			"",
			word,
			outside,
			valid + "," + op + "=" + outside,
			valid + "," + op,
			valid + ",=" + valid,
			valid + "," + op + "=" + word,
			valid + ",",
		}).Draw(t, "text")
	})
}

var (
	summaryChecks = []string{"domain.p0", "domain.p1", "domain.p2", "domain.p3"}
	summaryOps    = []string{"domain.Box.put", "domain.scale", "domain.other"}
)

// genSummarySample draws zero to eight calls of three operations within one
// period, each taking 1µs to 1s, each judged by zero to three of four
// properties, two of them with within budgets, the verdicts drawn evenly,
// clauses from ensure#1, ensure#2, where#1 and requires#1, a counterexample
// with an input of one or two symbols; and the 95th percentile E16 gives
// each property judged.
//
//uc:generates monitor.summary-counts-judgments.sample
func genSummarySample() *rapid.Generator[SummarySample] {
	return rapid.Custom(func(t *rapid.T) SummarySample {
		from := time.Unix(rapid.Int64Range(0, 1<<32).Draw(t, "from"), 0).UTC()
		out := SummarySample{From: from, To: from.Add(period), P95: map[string]time.Duration{}, Within: map[string]time.Duration{}}
		if rapid.Bool().Draw(t, "committed") {
			out.Build.Commit = rapid.StringMatching(`[0-9a-f]{40}`).Draw(t, "commit")
		}
		out.Build.Dirty = out.Build.Commit == "" || rapid.Bool().Draw(t, "dirty")
		for _, check := range summaryChecks[:2] {
			out.Within[check] = time.Duration(rapid.Int64Range(int64(time.Microsecond), int64(time.Second)).Draw(t, "within"))
		}
		durations := map[string][]time.Duration{}
		for range rapid.IntRange(0, 8).Draw(t, "calls") {
			call := JudgedCall{Op: rapid.SampledFrom(summaryOps).Draw(t, "op"),
				At:       from.Add(time.Duration(rapid.Int64Range(0, int64(period)-1).Draw(t, "at"))),
				Duration: time.Duration(rapid.Int64Range(int64(time.Microsecond), int64(time.Second)).Draw(t, "duration"))}
			checks := rapid.SliceOfNDistinct(rapid.SampledFrom(summaryChecks), 0, 3, func(c string) string { return c }).Draw(t, "checks")
			for _, check := range checks {
				j := Judgment{Check: check, Judged: []string{}, Within: out.Within[check], Path: "design/domain.uc", Line: 3, Column: 1}
				switch rapid.IntRange(0, 3).Draw(t, "verdict") {
				case 0:
					j.Verdict = VerdictPassed
					j.Judged = rapid.SliceOfNDistinct(rapid.SampledFrom([]string{"ensure#1", "ensure#2"}), 1, 2, func(c string) string { return c }).Draw(t, "judged")
				case 1:
					j.Verdict = VerdictCounterexample
					j.Clause = rapid.SampledFrom([]string{"ensure#1", "ensure#2"}).Draw(t, "clause")
					j.Judged = []string{j.Clause}
					j.Input = map[string]Value{}
					for i := range rapid.IntRange(1, 2).Draw(t, "symbols") {
						j.Input["x"+strconv.Itoa(i)] = Value{Kind: "int", Text: strconv.Itoa(rapid.IntRange(-100, 100).Draw(t, "value"))}
					}
					j.InputHash = InputHash(j.Input)
				case 2:
					j.Verdict = VerdictOutsideDomain
					j.Clause = rapid.SampledFrom([]string{"where#1", "requires#1"}).Draw(t, "clause")
				default:
					j.Verdict = VerdictCheckError
					j.Message = "the input is no model value"
				}
				if j.Verdict == VerdictPassed || j.Verdict == VerdictCounterexample {
					durations[check] = append(durations[check], call.Duration)
				}
				if _, ok := out.P95[check]; !ok {
					out.P95[check] = 0
				}
				call.Judgments = append(call.Judgments, j)
			}
			out.Calls = append(out.Calls, call)
		}
		for check, list := range durations {
			if out.Within[check] > 0 {
				sorted := slices.Clone(list)
				slices.Sort(sorted)
				rank := (95*len(sorted) + 99) / 100
				out.P95[check] = sorted[rank-1]
			}
		}
		return out
	})
}

// The schema of genRedactionSample: a sensitive record and enum, a record
// holding them, a codec type represented as a list of the sensitive record,
// and an operation taking the sensitive record.
var redactionSchema = Schema{
	Types: map[string]ModelType{
		"domain.Secret": {Form: "record", Fields: map[string]string{"text": "string"}, Sensitive: true},
		"domain.Token":  {Form: "enum", Fields: map[string]string{}, Sensitive: true},
		"domain.Holder": {Form: "record", Fields: map[string]string{"secret": "domain.Secret", "names": "list<string>", "tokens": "map<string, domain.Token>", "maybe": "optional<domain.Secret>"}},
		"domain.Coded":  {Form: "record", Fields: map[string]string{}, Codec: "list<domain.Secret>"},
	},
	Operations: map[string]map[string]string{"domain.store": {"secret": "domain.Secret", "count": "int"}},
}

// genRedactionSample draws a model type of at most four levels from the
// types and the operation of redactionSchema, list, set, map and optional,
// a value of it, a key of 1 to 16 bytes, and the value with every value of a
// sensitive type replaced by the HMAC-SHA-256 of its canonical encoding.
//
//uc:generates monitor.redaction-replaces-sensitive-values.sample
func genRedactionSample() *rapid.Generator[RedactionSample] {
	return rapid.Custom(func(t *rapid.T) RedactionSample {
		key := rapid.SliceOfN(rapid.Byte(), 1, 16).Draw(t, "key")
		model := drawModel(t, 0)
		value, expected := drawRedacted(t, model, key, 0)
		return RedactionSample{Value: value, Model: model, Schema: redactionSchema, Key: key, Expected: expected}
	})
}

func drawModel(t *rapid.T, depth int) string {
	models := []string{"string", "int", "domain.Secret", "domain.Token", "domain.Holder", "domain.Coded", "call<domain.store>"}
	if depth < 3 {
		models = append(models, "list", "set", "map", "optional")
	}
	switch model := rapid.SampledFrom(models).Draw(t, "model"); model {
	case "list", "set", "optional":
		return model + "<" + drawModel(t, depth+1) + ">"
	case "map":
		return "map<" + rapid.SampledFrom([]string{"string", "domain.Secret"}).Draw(t, "key") + ", " + drawModel(t, depth+1) + ">"
	default:
		return model
	}
}

// drawRedacted draws a canonical value of model and its redaction under key.
func drawRedacted(t *rapid.T, model string, key []byte, depth int) (Value, Value) {
	kind, args := splitType(model)
	switch kind {
	case "string":
		v := Value{Kind: "string", Text: lowerNames.Draw(t, "text")}
		return v, v
	case "int":
		v := Value{Kind: "int", Text: strconv.Itoa(rapid.IntRange(-50, 50).Draw(t, "int"))}
		return v, v
	case "domain.Secret":
		text, _ := drawRedacted(t, "string", key, depth+1)
		v := Value{Kind: "record", Entries: []ValueEntry{{Key: Value{Kind: "string", Text: "text"}, Value: text}}}
		return v, hmacOf(v, key)
	case "domain.Token":
		v := Value{Kind: "enum", Text: rapid.SampledFrom([]string{"alpha", "beta"}).Draw(t, "token")}
		return v, hmacOf(v, key)
	case "domain.Holder":
		return drawEntries(t, "record", "", map[string]string{"secret": "domain.Secret", "names": "list<string>", "tokens": "map<string, domain.Token>", "maybe": "optional<domain.Secret>"}, key, depth)
	case "domain.Coded":
		inner, redacted := drawRedacted(t, "list<domain.Secret>", key, depth+1)
		return Value{Kind: "encoded", Items: []Value{inner}}, Value{Kind: "encoded", Items: []Value{redacted}}
	case "call":
		return drawEntries(t, "call", "store", map[string]string{"secret": "domain.Secret", "count": "int"}, key, depth)
	case "optional":
		if rapid.Bool().Draw(t, "none") {
			v := Value{Kind: "none"}
			return v, v
		}
		return drawRedacted(t, args[0], key, depth+1)
	case "list", "set":
		value, redacted := Value{Kind: kind, Items: []Value{}}, Value{Kind: kind, Items: []Value{}}
		seen := map[string]bool{}
		for range rapid.IntRange(0, 3).Draw(t, "items") {
			item, out := drawRedacted(t, args[0], key, depth+1)
			if kind == "set" && seen[string(canonical(item))] {
				continue
			}
			seen[string(canonical(item))] = true
			value.Items, redacted.Items = append(value.Items, item), append(redacted.Items, out)
		}
		if kind == "set" {
			sortCanonical(value.Items)
			sortCanonical(redacted.Items)
		}
		return value, redacted
	case "map":
		value, redacted := Value{Kind: "map", Entries: []ValueEntry{}}, Value{Kind: "map", Entries: []ValueEntry{}}
		seen := map[string]bool{}
		for range rapid.IntRange(0, 3).Draw(t, "entries") {
			k, kOut := drawRedacted(t, args[0], key, depth+1)
			if seen[string(canonical(k))] {
				continue
			}
			seen[string(canonical(k))] = true
			v, vOut := drawRedacted(t, args[1], key, depth+1)
			value.Entries = append(value.Entries, ValueEntry{Key: k, Value: v})
			redacted.Entries = append(redacted.Entries, ValueEntry{Key: kOut, Value: vOut})
		}
		sortByKey(value.Entries)
		sortByKey(redacted.Entries)
		return value, redacted
	}
	t.Fatalf("no value is drawn for the model %s", model)
	return Value{}, Value{}
}

// drawEntries draws a record's or a call's entries by their types.
func drawEntries(t *rapid.T, kind, text string, types map[string]string, key []byte, depth int) (Value, Value) {
	value, redacted := Value{Kind: kind, Text: text, Entries: []ValueEntry{}}, Value{Kind: kind, Text: text, Entries: []ValueEntry{}}
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		v, out := drawRedacted(t, types[name], key, depth+1)
		label := Value{Kind: "string", Text: name}
		value.Entries = append(value.Entries, ValueEntry{Key: label, Value: v})
		redacted.Entries = append(redacted.Entries, ValueEntry{Key: label, Value: out})
	}
	sortByKey(value.Entries)
	sortByKey(redacted.Entries)
	return value, redacted
}

// hmacOf is a sensitive value's redacted form by its definition: the
// HMAC-SHA-256 of its canonical JSON under key, in lowercase hexadecimal.
func hmacOf(v Value, key []byte) Value {
	data, _ := json.Marshal(v)
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return Value{Kind: "redacted", Text: hex.EncodeToString(mac.Sum(nil))}
}

func sortCanonical(items []Value) {
	slices.SortFunc(items, func(a, b Value) int { return strings.Compare(string(canonical(a)), string(canonical(b))) })
}

func sortByKey(entries []ValueEntry) {
	slices.SortFunc(entries, func(a, b ValueEntry) int {
		return strings.Compare(string(canonical(a.Key)), string(canonical(b.Key)))
	})
}

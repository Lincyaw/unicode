package evidence

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/monitor"
)

// TrafficSample is a graph, collected calls, and the inputs E29 gives its
// contracts judged at runtime.
//
//uc:realizes evidence.TrafficSample
type TrafficSample struct {
	Graph     *graph.Graph
	Collected []monitor.Collected
	Expected  map[string][]lang.Input
}

// VenueSample is a graph and reports of the test and runtime venues.
//
//uc:realizes evidence.VenueSample
type VenueSample struct {
	Graph   *graph.Graph
	Reports []lang.RunReport
}

var trafficName = rapid.StringMatching(`[a-z]{1,6}`)

// genCollected draws a collected call: an operation of one to three
// dot-separated lowercase names, a receiver about half the time, receiver
// and call value canonical model values of at most three levels of the kinds
// int, string, bytes, bool, list, set, map, record, none and redacted, an
// empty or forty-hexadecimal commit, and a whole-second UTC time.
//
//uc:generates evidence.collected-calls-round-trip.collected
func genCollected() *rapid.Generator[monitor.Collected] {
	return rapid.Custom(func(t *rapid.T) monitor.Collected {
		out := monitor.Collected{Op: strings.Join(rapid.SliceOfN(trafficName, 1, 3).Draw(t, "op"), "."),
			At: time.Unix(rapid.Int64Range(0, 1<<32).Draw(t, "at"), 0).UTC()}
		if rapid.Bool().Draw(t, "committed") {
			out.Commit = rapid.StringMatching(`[0-9a-f]{40}`).Draw(t, "commit")
		}
		if rapid.Bool().Draw(t, "receiver") {
			receiver := drawModelValue(t, 0)
			out.Receiver = &receiver
		}
		out.Call = monitor.Value{Kind: "call", Text: trafficName.Draw(t, "name"), Entries: []monitor.ValueEntry{}}
		for _, param := range rapid.SliceOfNDistinct(trafficName, 0, 3, func(s string) string { return s }).Draw(t, "params") {
			out.Call.Entries = append(out.Call.Entries, monitor.ValueEntry{Key: monitor.Value{Kind: "string", Text: param}, Value: drawModelValue(t, 1)})
		}
		return canonicalCollected(out)
	})
}

// drawModelValue draws a model value of at most three levels.
func drawModelValue(t *rapid.T, depth int) monitor.Value {
	kinds := []string{"int", "string", "bytes", "bool", "none", "redacted"}
	if depth < 2 {
		kinds = append(kinds, "list", "set", "map", "record")
	}
	switch kind := rapid.SampledFrom(kinds).Draw(t, "kind"); kind {
	case "int":
		return monitor.Value{Kind: kind, Text: strconv.Itoa(rapid.IntRange(-1000, 1000).Draw(t, "int"))}
	case "string":
		return monitor.Value{Kind: kind, Text: rapid.String().Draw(t, "string")}
	case "bytes":
		return monitor.Value{Kind: kind, Text: rapid.StringMatching(`([A-Za-z0-9+/]{4}){0,3}`).Draw(t, "base64")}
	case "bool":
		return monitor.Value{Kind: kind, Text: strconv.FormatBool(rapid.Bool().Draw(t, "bool"))}
	case "none":
		return monitor.Value{Kind: kind}
	case "redacted":
		return monitor.Value{Kind: kind, Text: rapid.StringMatching(`[0-9a-f]{64}`).Draw(t, "digest")}
	case "list", "set":
		out := monitor.Value{Kind: kind, Items: []monitor.Value{}}
		for range rapid.IntRange(0, 3).Draw(t, "items") {
			out.Items = append(out.Items, drawModelValue(t, depth+1))
		}
		return out
	default:
		out := monitor.Value{Kind: kind, Entries: []monitor.ValueEntry{}}
		for range rapid.IntRange(0, 3).Draw(t, "entries") {
			key := monitor.Value{Kind: "string", Text: trafficName.Draw(t, "key")}
			if kind == "map" {
				key = drawModelValue(t, 2)
			}
			out.Entries = append(out.Entries, monitor.ValueEntry{Key: key, Value: drawModelValue(t, depth+1)})
		}
		return out
	}
}

// canonicalCollected writes a collected call's values in their canonical
// encoding, as a monitored program writes them.
func canonicalCollected(c monitor.Collected) monitor.Collected {
	normalize := func(v monitor.Value) monitor.Value {
		out, err := normalizeValue(dedupeEntries(langValue(v)), 0)
		if err != nil {
			panic(err)
		}
		return monitorValue(out)
	}
	c.Call = normalize(c.Call)
	if c.Receiver != nil {
		receiver := normalize(*c.Receiver)
		c.Receiver = &receiver
	}
	return c
}

// dedupeEntries keeps the first entry of each key, which canonical maps and
// records have once.
func dedupeEntries(v lang.Value) lang.Value {
	for i := range v.Items {
		v.Items[i] = dedupeEntries(v.Items[i])
	}
	if v.Entries != nil {
		var kept []lang.ValueEntry
		for _, entry := range v.Entries {
			if !slices.ContainsFunc(kept, func(k lang.ValueEntry) bool { return string(canonical(k.Key)) == string(canonical(entry.Key)) }) {
				entry.Value = dedupeEntries(entry.Value)
				kept = append(kept, entry)
			}
		}
		v.Entries = append([]lang.ValueEntry{}, kept...)
	}
	return v
}

// genTrafficSample draws a graph of the type domain.Box with put(key: string,
// count: int) and the container operations domain.scale(factor: int) and
// domain.other(), zero to three contracts whose call clause calls put or
// scale with symbols, a contract calling scale with a field and a manual
// property, zero to six collected calls of put, scale and other, and the
// inputs each contract gets, collected call by collected call.
//
//uc:generates evidence.traffic-gives-inputs.sample
func genTrafficSample() *rapid.Generator[TrafficSample] {
	return rapid.Custom(func(t *rapid.T) TrafficSample {
		nodes := []graph.Node{
			{Id: "domain", Kind: graph.NodeKindContainer, Form: "component", Layer: graph.LayerDesign},
			{Id: "domain.Box", Kind: graph.NodeKindType, Form: "object", Layer: graph.LayerDesign},
			{Id: "domain.Box.put", Kind: graph.NodeKindOp, Form: "op", Layer: graph.LayerDesign, Attrs: map[string][]string{"signature": {"(key: string, count: int)"}}},
			{Id: "domain.scale", Kind: graph.NodeKindOp, Form: "op", Layer: graph.LayerDesign, Attrs: map[string][]string{"signature": {"(factor: int) -> int"}}},
			{Id: "domain.other", Kind: graph.NodeKindOp, Form: "op", Layer: graph.LayerDesign, Attrs: map[string][]string{"signature": {"() -> int"}}},
			{Id: "domain.Holder", Kind: graph.NodeKindType, Form: "record", Layer: graph.LayerDesign, Attrs: map[string][]string{"fields": {"factor: int"}}},
		}
		edges := []graph.Edge{
			{Kind: graph.EdgeKindContains, From: "domain", To: "domain.Box"},
			{Kind: graph.EdgeKindContains, From: "domain.Box", To: "domain.Box.put"},
			{Kind: graph.EdgeKindContains, From: "domain", To: "domain.scale"},
			{Kind: graph.EdgeKindContains, From: "domain", To: "domain.other"},
			{Kind: graph.EdgeKindContains, From: "domain", To: "domain.Holder"},
		}
		type contract struct {
			id, op   string
			receiver string
			args     []string
		}
		var contracts []contract
		property := func(id, call string, symbols []string, op string, manual bool) {
			attrs := map[string][]string{"symbols": symbols, "call": {call}}
			if manual {
				attrs["manual"] = []string{"needs an adapter"}
			}
			nodes = append(nodes, graph.Node{Id: id, Kind: graph.NodeKindProperty, Form: "postcondition", Layer: graph.LayerDesign, Attrs: attrs})
			edges = append(edges, graph.Edge{Kind: graph.EdgeKindContains, From: "domain", To: id},
				graph.Edge{Kind: graph.EdgeKindRefers, From: id, To: op, Attrs: map[string][]string{"clauses": {"call#1"}}})
		}
		for i := range rapid.IntRange(0, 3).Draw(t, "contracts") {
			id := "domain.c" + strconv.Itoa(i)
			names := rapid.SliceOfNDistinct(trafficName, 3, 3, func(s string) string { return s }).Draw(t, "names")
			if rapid.Bool().Draw(t, "put") {
				property(id, names[0]+".put("+names[1]+", "+names[2]+")", []string{names[0] + ": domain.Box", names[1] + ": string", names[2] + ": int"}, "domain.Box.put", false)
				contracts = append(contracts, contract{id, "domain.Box.put", names[0], names[1:]})
			} else {
				property(id, "scale("+names[0]+")", []string{names[0] + ": int"}, "domain.scale", false)
				contracts = append(contracts, contract{id, "domain.scale", "", names[:1]})
			}
		}
		property("domain.fielded", "scale(holder.factor)", []string{"holder: domain.Holder"}, "domain.scale", false)
		property("domain.handwritten", "scale(factor)", []string{"factor: int"}, "domain.scale", true)
		g := graph.NewGraph()
		for _, n := range nodes {
			if err := g.AddNode(n); err != nil {
				t.Fatal(err)
			}
		}
		for _, e := range edges {
			if err := g.AddEdge(e); err != nil {
				t.Fatal(err)
			}
		}
		out := TrafficSample{Graph: &g, Collected: []monitor.Collected{}, Expected: map[string][]lang.Input{}}
		integer := func(label string) monitor.Value {
			return monitor.Value{Kind: "int", Text: strconv.Itoa(rapid.IntRange(-9, 9).Draw(t, label))}
		}
		entry := func(name string, value monitor.Value) monitor.ValueEntry {
			return monitor.ValueEntry{Key: monitor.Value{Kind: "string", Text: name}, Value: value}
		}
		for range rapid.IntRange(0, 6).Draw(t, "collected") {
			c := monitor.Collected{Op: rapid.SampledFrom([]string{"domain.Box.put", "domain.scale", "domain.other"}).Draw(t, "op"), At: time.Unix(0, 0).UTC()}
			switch c.Op {
			case "domain.Box.put":
				receiver := monitor.Value{Kind: "record", Entries: []monitor.ValueEntry{entry("size", integer("size"))}}
				c.Receiver = &receiver
				c.Call = monitor.Value{Kind: "call", Text: "put", Entries: []monitor.ValueEntry{entry("count", integer("count")), entry("key", monitor.Value{Kind: "string", Text: trafficName.Draw(t, "key")})}}
			case "domain.scale":
				c.Call = monitor.Value{Kind: "call", Text: "scale", Entries: []monitor.ValueEntry{entry("factor", integer("factor"))}}
			default:
				c.Call = monitor.Value{Kind: "call", Text: "other", Entries: []monitor.ValueEntry{}}
			}
			out.Collected = append(out.Collected, c)
		}
		for _, k := range contracts {
			for _, c := range out.Collected {
				if c.Op != k.op {
					continue
				}
				input := lang.Input{Values: map[string]lang.Value{}}
				if k.receiver != "" {
					input.Values[k.receiver] = langValue(*c.Receiver)
					input.Values[k.args[0]] = langValue(c.Call.Entries[1].Value)
					input.Values[k.args[1]] = langValue(c.Call.Entries[0].Value)
				} else {
					input.Values[k.args[0]] = langValue(c.Call.Entries[0].Value)
				}
				out.Expected[k.id] = append(out.Expected[k.id], input)
			}
		}
		return out
	})
}

// genVenueSample draws the graph of attachment-preserves-input-identity and
// one to three reports of the venues test, runtime or none, about half with
// the graph's execution-input digest, a runtime conclusion with one or two
// limits or preconditions that failed, one or two judged clauses, a 95th
// percentile of 0 to 1 second and zero or one counterexample.
//
//uc:generates evidence.runtime-evidence-keeps-its-venue.sample
func genVenueSample() *rapid.Generator[VenueSample] {
	return rapid.Custom(func(t *rapid.T) VenueSample {
		g := attachmentGraph(t, rapid.IntRange(-50, 50).Draw(t, "value"), 0)
		out := VenueSample{Graph: g}
		for i := range rapid.IntRange(1, 3).Draw(t, "reports") {
			at := time.Unix(int64(2000+i*100), 0).UTC()
			venue := rapid.SampledFrom([]string{lang.VenueTest, lang.VenueRuntime, ""}).Draw(t, "venue")
			snapshot := graph.InputDigest(g)
			if !rapid.Bool().Draw(t, "current") {
				snapshot = strings.Repeat("0", 64)
			}
			result := lang.RunResult{Id: "domain.rule", Passed: true, Accepted: 1, Observations: []string{}, Differences: []string{}, Diagnostics: []diag.Diagnostic{}, Cases: []lang.CaseResult{}}
			report := lang.RunReport{Version: 1, Snapshot: snapshot, At: at.Format(time.RFC3339Nano), Mode: "replay", Venue: venue, Results: []lang.RunResult{result}, Diagnostics: []diag.Diagnostic{}}
			if venue == lang.VenueRuntime {
				report.Mode, report.From = "", at.Add(-time.Minute).Format(time.RFC3339Nano)
				for _, clause := range rapid.SliceOfNDistinct(rapid.SampledFrom([]string{"where#1", "requires#1"}), 1, 2, func(s string) string { return s }).Draw(t, "outside") {
					result.OutsideDomain = append(result.OutsideDomain, lang.ClauseCalls{Clause: clause, Calls: rapid.IntRange(1, 9).Draw(t, "calls")})
				}
				for _, clause := range rapid.SliceOfNDistinct(rapid.SampledFrom([]string{"ensure#1", "ensure#2"}), 1, 2, func(s string) string { return s }).Draw(t, "clauses") {
					result.Clauses = append(result.Clauses, lang.ClauseJudged{Clause: clause, Judged: rapid.IntRange(1, 9).Draw(t, "judged")})
				}
				result.P95 = rapid.IntRange(0, int(time.Second)).Draw(t, "p95")
				if rapid.Bool().Draw(t, "counterexample") {
					input, _ := NormalizeInput(lang.Input{Values: map[string]lang.Value{"payload": intValue(rapid.IntRange(-50, 50).Draw(t, "payload"))}})
					result.Passed = false
					result.Cases = []lang.CaseResult{{Id: "domain.rule.rt-1", Check: "domain.rule", Form: "runtime", Reason: "counterexample", Input: input, InputHash: hash(input.Values),
						Coverage: []lang.Coverage{}, Trace: []lang.TraceEvent{}, Observations: []string{}, Differences: []string{}, Diagnostics: []diag.Diagnostic{sampleDiagnostic("monitor.ensure-failed")},
						Clause: "ensure#1", At: at.Add(-time.Second).Format(time.RFC3339Nano)}}
				}
				report.Results = []lang.RunResult{result}
			}
			out.Reports = append(out.Reports, report)
		}
		return out
	})
}

package evidence

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strconv"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// Sample constructs measured evidence without an expected-result Boolean.
//
//uc:realizes evidence.Sample
type Sample struct {
	Cases    []lang.CaseResult
	Limit    int
	Required []string
}

//uc:generates evidence.reduction-preserves-evidence.sample
func genSample() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample {
		shape := rapid.IntRange(0, 13).Draw(t, "shape")
		count := rapid.IntRange(1, 8).Draw(t, "cases")
		points := rapid.IntRange(1, 6).Draw(t, "points")
		out := Sample{Cases: []lang.CaseResult{}, Limit: rapid.IntRange(0, 9).Draw(t, "limit"), Required: []string{}}
		seen := map[int]bool{}
		for index := range count {
			seed := rapid.IntRange(-100, 100).Draw(t, "value")
			input, err := NormalizeInput(lang.Input{Values: map[string]lang.Value{"payload": sampleValue(shape, seed)}})
			if err != nil {
				t.Fatal(err)
			}
			result := lang.CaseResult{Check: "domain.rule", Form: "recorded", Reason: "coverage", Input: input, InputHash: hash(input.Values), Passed: true, Coverage: []lang.Coverage{}, Trace: []lang.TraceEvent{}, Diagnostics: []diag.Diagnostic{}}
			result.Id = result.Check + ".r-" + result.InputHash[:8]
			if rapid.Bool().Draw(t, "declared") {
				result.Form = "declared"
				result.Reason = ""
				result.Id = fmt.Sprintf("domain.rule.declared%d", index)
			} else if rapid.Bool().Draw(t, "counterexample") {
				result.Passed = false
				result.Reason = "counterexample"
			}
			for point := range points {
				if rapid.Bool().Draw(t, "covered") {
					seen[point] = true
					result.Coverage = append(result.Coverage, lang.Coverage{Kind: "code", Node: fmt.Sprintf("go:sample.F%d", point), Path: "source.go", Line: point + 1, Column: 1, EndLine: point + 1, EndColumn: 3, Label: fmt.Sprint(point)})
				}
			}
			out.Cases = append(out.Cases, result)
			if rapid.Bool().Draw(t, "duplicate") {
				out.Cases = append(out.Cases, result)
			}
		}
		for point := range points {
			if seen[point] {
				out.Required = append(out.Required, strconv.Itoa(point))
			}
		}
		return out
	})
}

func entry(name string, value lang.Value) lang.ValueEntry {
	return lang.ValueEntry{Key: lang.Value{Kind: "string", Text: name}, Value: value}
}
func intValue(value int) lang.Value { return lang.Value{Kind: "int", Text: strconv.Itoa(value)} }
func sampleValue(shape, value int) lang.Value {
	switch shape {
	case 0:
		return intValue(value)
	case 1:
		return lang.Value{Kind: "list", Items: []lang.Value{intValue(value), intValue(value / 2)}}
	case 2:
		return lang.Value{Kind: "set", Items: []lang.Value{{Kind: "string", Text: fmt.Sprint(value)}, {Kind: "string", Text: "界"}}}
	case 3:
		return lang.Value{Kind: "record", Entries: []lang.ValueEntry{entry("n", intValue(value)), entry("text", lang.Value{Kind: "string", Text: fmt.Sprintf("value %d", value)})}}
	case 4:
		return lang.Value{Kind: "encoded", Items: []lang.Value{{Kind: "record", Entries: []lang.ValueEntry{entry("n", intValue(value)), entry("secret", lang.Value{Kind: "string", Text: string([]byte{0xff, byte(value)})})}}}}
	case 5:
		return lang.Value{Kind: "map", Entries: []lang.ValueEntry{{Key: lang.Value{Kind: "string", Text: string([]byte{0xff, byte(value)})}, Value: intValue(value)}}}
	case 6:
		return lang.Value{Kind: "call", Text: "advance", Entries: []lang.ValueEntry{entry("amount", intValue(value))}}
	case 7:
		response := lang.Value{Kind: "record", Entries: []lang.ValueEntry{entry("returns", intValue(value))}}
		if value%2 != 0 {
			response.Entries = []lang.ValueEntry{entry("raises", lang.Value{Kind: "string", Text: "domain.Declined"}), entry("fields", lang.Value{Kind: "record", Entries: []lang.ValueEntry{entry("code", intValue(value))}})}
		}
		return lang.Value{Kind: "list", Items: []lang.Value{response}}
	case 8:
		return lang.Value{Kind: "bytes", Text: base64.StdEncoding.EncodeToString([]byte{0, 0xff, byte(value)})}
	case 9:
		return lang.Value{Kind: "time", Text: time.Unix(int64(value), int64(value+100)).UTC().Format(time.RFC3339Nano)}
	case 10:
		return lang.Value{Kind: "duration", Text: time.Duration(value).String()}
	case 11:
		return lang.Value{Kind: "float", Text: fmt.Sprintf("%d.%03d", value, value*value%1000)}
	case 12:
		return lang.Value{Kind: "enum", Text: []string{"draft", "done"}[(value*value)%2]}
	default:
		return lang.Value{Kind: "record", Entries: []lang.ValueEntry{entry("port", lang.Value{Kind: "record", Entries: []lang.ValueEntry{entry("charge", sampleValue(7, value))}})}}
	}
}

// AttachmentSample records independent current-input and business-failure sets.
//
//uc:realizes evidence.AttachmentSample
type AttachmentSample struct {
	Graph        *graph.Graph
	Reports      []lang.RunReport
	FreshCount   int
	CurrentCases []string
	FailedChecks []string
}

//uc:generates evidence.attachment-preserves-input-identity.sample
func genAttachmentSample() *rapid.Generator[AttachmentSample] {
	return rapid.Custom(func(t *rapid.T) AttachmentSample {
		value := rapid.IntRange(-50, 50).Draw(t, "value")
		mode := rapid.IntRange(0, 9).Draw(t, "mode")
		g := attachmentGraph(t, value, mode)
		input, _ := NormalizeInput(lang.Input{Values: map[string]lang.Value{"payload": intValue(value)}})
		candidate := lang.CaseResult{Id: "domain.rule.example", Check: "domain.rule", Form: "declared", Input: input, InputHash: hash(input.Values), Passed: true, Coverage: []lang.Coverage{{Kind: "code", Node: "go:sample.F0", Path: "source.go", Line: 3, Column: 1, EndLine: 3, EndColumn: 5}}, Trace: []lang.TraceEvent{}, Diagnostics: []diag.Diagnostic{}}
		if mode == 2 {
			candidate.Id = "domain.rule.before"
		}
		if mode == 3 {
			candidate.Form = "recorded"
			candidate.Id = "domain.rule.r-" + candidate.InputHash[:8]
			candidate.Reason = "counterexample"
		}
		if mode == 1 || mode == 2 || mode == 3 || mode == 5 || mode == 6 {
			candidate.Passed = false
			candidate.Diagnostics = []diag.Diagnostic{sampleDiagnostic("lang.golang.ensure-failed")}
		}
		result := lang.RunResult{Id: candidate.Check, Passed: candidate.Passed, Accepted: 1, Cases: []lang.CaseResult{candidate}, CodeTotal: 2}
		stamp := time.Unix(1000+int64(value), 0).UTC().Format(time.RFC3339Nano)
		report := lang.RunReport{Version: 1, Snapshot: graph.InputDigest(g), At: stamp, Mode: "replay", Results: []lang.RunResult{result}, Diagnostics: []diag.Diagnostic{}}
		if mode == 1 {
			previous := cloneFixture(t, g)
			nodes := previous.Nodes()
			for index := range nodes {
				if nodes[index].Id == "go:sample.F0" {
					nodes[index].Origin.Line++
					nodes[index].Attrs["body"] = []string{"prior"}
				}
			}
			previous = fixtureBuild(t, nodes, previous.Edges())
			report.Snapshot = graph.InputDigest(previous)
		}
		if mode == 4 || mode == 5 || mode == 8 {
			code := "lang.golang.observation-failed"
			if mode == 8 {
				code = "lang.golang.expression-failed"
			}
			problem := sampleDiagnostic(code)
			report.Diagnostics = []diag.Diagnostic{problem}
			report.Results[0].Passed = false
			report.Results[0].Diagnostics = []diag.Diagnostic{problem}
		}
		if mode == 7 {
			report.Results[0].Passed = false
			report.Results[0].Diagnostics = []diag.Diagnostic{sampleDiagnostic("lang.golang.within-exceeded")}
		}
		if mode == 9 {
			report.Results[0].Accepted = 0
			report.Results[0].Cases = []lang.CaseResult{}
			report.Results[0].Passed = false
			report.Diagnostics = []diag.Diagnostic{sampleDiagnostic("lang.golang.no-effective-input")}
		}
		out := AttachmentSample{Graph: g, Reports: []lang.RunReport{report}, CurrentCases: []string{}, FailedChecks: []string{}}
		if mode != 1 && mode != 9 {
			out.FreshCount = 1
		}
		if mode != 1 && mode != 4 && mode != 7 && mode != 8 && mode != 9 && !candidate.Passed {
			out.FailedChecks = []string{candidate.Check}
		}
		if mode == 7 {
			out.FailedChecks = []string{candidate.Check}
		}
		if mode != 1 && mode != 3 && mode != 6 && mode != 9 {
			id := "domain.rule.example"
			if mode == 2 {
				id = "domain.rule.renamed"
			}
			out.CurrentCases = []string{id}
		}
		if rapid.Bool().Draw(t, "second-current") && mode != 9 {
			second := report
			second.Results = slices.Clone(report.Results)
			second.Results[0] = lang.RunResult{Id: candidate.Check, Passed: true, Accepted: 1, Cases: []lang.CaseResult{candidate}, CodeTotal: 2}
			second.Results[0].Cases[0].Passed = true
			second.Results[0].Cases[0].Diagnostics = []diag.Diagnostic{}
			second.Diagnostics = []diag.Diagnostic{}
			second.At = time.Unix(1001+int64(value), 0).UTC().Format(time.RFC3339Nano)
			out.Reports = append(out.Reports, second)
			if mode != 1 {
				out.FreshCount++
			}
		}
		return out
	})
}

func sampleDiagnostic(code string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Subject: "domain.rule", Position: diag.Position{Path: "design/domain.uc", Line: 1, Column: 1}, Message: "measured failure", Fix: "correct the observed behavior"}
}
func fixtureBuild(t rapid.TB, nodes []graph.Node, edges []graph.Edge) *graph.Graph {
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
	return &g
}
func cloneFixture(t rapid.TB, g *graph.Graph) *graph.Graph {
	return fixtureBuild(t, g.Nodes(), g.Edges())
}
func attachmentGraph(t rapid.TB, value, mode int) *graph.Graph {
	nodes := []graph.Node{{Id: "domain", Kind: graph.NodeKindContainer, Form: "component", Layer: graph.LayerDesign}, {Id: "domain.rule", Kind: graph.NodeKindProperty, Form: "postcondition", Layer: graph.LayerDesign, Attrs: map[string][]string{"symbols": {"payload: int"}}}, {Id: "go:sample.F0", Kind: graph.NodeKindOp, Form: "func", Layer: graph.LayerCode, Origin: diag.Position{Path: "source.go", Line: 3, Column: 1}, Attrs: map[string][]string{"body": {"current"}}}}
	edges := []graph.Edge{{Kind: graph.EdgeKindContains, From: "domain", To: "domain.rule"}}
	if mode != 3 {
		id := "domain.rule.example"
		if mode == 2 {
			id = "domain.rule.renamed"
		}
		current := value
		if mode == 6 {
			current++
		}
		nodes = append(nodes, graph.Node{Id: id, Kind: graph.NodeKindCase, Form: "declared", Layer: graph.LayerDesign, Attrs: map[string][]string{"values": {"payload = " + strconv.Itoa(current)}}})
		edges = append(edges, graph.Edge{Kind: graph.EdgeKindContains, From: "domain.rule", To: id})
	}
	return fixtureBuild(t, nodes, edges)
}

func fixtureType(t rapid.TB, g *graph.Graph, value lang.Value, counter *int) string {
	addNode := func(n graph.Node) {
		if g.Node(n.Id) == nil {
			if err := g.AddNode(n); err != nil {
				t.Fatal(err)
			}
		}
	}
	addEdge := func(e graph.Edge) {
		for _, old := range g.Edges() {
			if old.Kind == e.Kind && old.From == e.From && old.To == e.To {
				return
			}
		}
		if err := g.AddEdge(e); err != nil {
			t.Fatal(err)
		}
	}
	addReply := func(id, owner string) {
		addNode(graph.Node{Id: "domain.Declined", Kind: graph.NodeKindType, Form: "error", Layer: graph.LayerDesign, Attrs: map[string][]string{"fields": {"code: int"}}})
		addNode(graph.Node{Id: id, Kind: graph.NodeKindOp, Form: "op", Layer: graph.LayerDesign, Attrs: map[string][]string{"signature": {"() -> int raises domain.Declined"}}})
		addEdge(graph.Edge{Kind: graph.EdgeKindContains, From: owner, To: id})
	}
	switch value.Kind {
	case "stringBytes":
		return "string"
	case "bool", "int", "float", "string", "bytes", "time", "duration":
		return value.Kind
	case "none":
		return "optional<int>"
	case "enum":
		addNode(graph.Node{Id: "domain.Status", Kind: graph.NodeKindType, Form: "enum", Layer: graph.LayerDesign, Attrs: map[string][]string{"values": {"draft", "done"}}})
		return "domain.Status"
	case "encoded":
		representation := fixtureType(t, g, value.Items[0], counter)
		id := fmt.Sprintf("domain.Hidden%d", *counter)
		*counter++
		addNode(graph.Node{Id: id, Kind: graph.NodeKindType, Form: "record", Layer: graph.LayerDesign, Attrs: map[string][]string{"codec": {id + ".encode", "domain.decode"}, "codecType": {representation}}})
		return id
	case "call":
		addNode(graph.Node{Id: "domain.Stock", Kind: graph.NodeKindType, Form: "object", Layer: graph.LayerDesign})
		addNode(graph.Node{Id: "domain.Stock.advance", Kind: graph.NodeKindOp, Form: "op", Layer: graph.LayerDesign, Attrs: map[string][]string{"signature": {"(amount: int)"}}})
		addEdge(graph.Edge{Kind: graph.EdgeKindContains, From: "domain.Stock", To: "domain.Stock.advance"})
		return "call<domain.Stock>"
	case "list", "set":
		if value.Kind == "list" && len(value.Items) > 0 && value.Items[0].Kind == "record" && slices.ContainsFunc(value.Items[0].Entries, func(e lang.ValueEntry) bool { return e.Key.Text == "returns" || e.Key.Text == "raises" }) {
			addNode(graph.Node{Id: "gateway", Kind: graph.NodeKindActor, Form: "external", Layer: graph.LayerDesign})
			addReply("gateway.charge", "gateway")
			return "replies<gateway.charge>"
		}
		element := "int"
		if len(value.Items) > 0 {
			element = fixtureType(t, g, value.Items[0], counter)
		}
		return value.Kind + "<" + element + ">"
	case "map":
		key, child := "string", "int"
		if len(value.Entries) > 0 {
			key = fixtureType(t, g, value.Entries[0].Key, counter)
			child = fixtureType(t, g, value.Entries[0].Value, counter)
		}
		return "map<" + key + ", " + child + ">"
	case "record":
		if len(value.Entries) == 1 && value.Entries[0].Key.Text == "charge" && value.Entries[0].Value.Kind == "list" {
			addNode(graph.Node{Id: "domain.Port", Kind: graph.NodeKindType, Form: "interface", Layer: graph.LayerDesign})
			addReply("domain.Port.charge", "domain.Port")
			return "domain.Port"
		}
		id := fmt.Sprintf("domain.Data%d", *counter)
		*counter++
		fields := []string{}
		for _, entry := range value.Entries {
			fields = append(fields, entry.Key.Text+": "+fixtureType(t, g, entry.Value, counter))
		}
		addNode(graph.Node{Id: id, Kind: graph.NodeKindType, Form: "record", Layer: graph.LayerDesign, Attrs: map[string][]string{"fields": fields}})
		return id
	}
	t.Fatalf("unknown fixture value kind %s", value.Kind)
	return ""
}

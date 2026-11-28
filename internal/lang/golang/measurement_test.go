package golang

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// MeasurementSample contains only the data needed to reconstruct the consumer.
//
//uc:realizes lang.golang.MeasurementSample
type MeasurementSample struct{ Value, Variant int }

//uc:generates lang.golang.measured-execution-keeps-real-evidence.sample
func genMeasurementSample() *rapid.Generator[MeasurementSample] {
	return rapid.Custom(func(t *rapid.T) MeasurementSample {
		return MeasurementSample{rapid.IntRange(-100, 100).Draw(t, "value"), rapid.IntRange(0, 7).Draw(t, "variant")}
	})
}

//uc:checks lang.golang.measured-execution-keeps-real-evidence
func checkMeasuredExecutionKeepsRealEvidence(t rapid.TB, sample MeasurementSample) {
	value := sample.Value
	if value < 0 {
		value = -value
	}
	value++
	root, err := os.MkdirTemp("", "unicode-measurement-property-")
	if err != nil {
		t.Fatal(err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	writeSkeletonFixture(t, root, "go.mod", []byte("module example.test/measured\n\ngo 1.26.2\n\nrequire pgregory.net/rapid v1.3.0\n"))
	sum, err := os.ReadFile(filepath.Join("..", "..", "..", "go.sum"))
	if err != nil {
		t.Fatal(err)
		return
	}
	writeSkeletonFixture(t, root, "go.sum", sum)
	source := measurementSource(sample, value)
	writeSkeletonFixture(t, root, "src/domain.go", []byte(source))
	writeSkeletonFixture(t, root, "design/domain.uc", []byte("component domain \"Reconstructed measurement input\" {\n  paths \"src/**\"\n}\n"))
	g := measurementGraph(t, sample, value)
	attachSkeletonCode(t, root, &g)
	plan := lang.GenPlan{Root: root, Roots: []string{"src"}, Graph: &g}
	generated := NewBackend().Generate(plan)
	if len(generated.Diagnostics) > 0 {
		t.Fatalf("measured generation failed: %+v", generated.Diagnostics)
		return
	}
	for _, edit := range generated.Edits {
		writeSkeletonFixture(t, root, edit.Path, edit.Content)
	}
	before := skeletonTree(t, root)
	run := lang.RunPlan{Root: root, Roots: plan.Roots, Graph: &g, Ids: []string{"domain.measured"}}
	switch sample.Variant {
	case 1, 7:
		// Variants 1 and 7 need one and two explored candidates; each costs
		// two worker processes, so the budget leaves room for a loaded machine.
		run.Explore = 2 * time.Second
	case 3:
		run.Explore = 350 * time.Millisecond
	}
	report := NewBackend().Run(run)
	if after := skeletonTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("read-only measured execution changed the consumer files")
	}
	if sample.Variant == 3 {
		found := false
		for _, d := range report.Diagnostics {
			if d.Code == "lang.golang.input-codec-failed" {
				found = true
			}
		}
		if !found {
			t.Fatalf("a shared mutable input was silently copied: %+v", report)
		}
		for _, result := range report.Results {
			if result.Passed || result.Accepted != 0 || result.Rejected != 0 {
				t.Fatalf("a shared mutable input executed after codec rejection: %+v", result)
			}
			for _, candidate := range result.Cases {
				if !candidate.Rejected {
					t.Fatalf("a shared mutable input produced execution evidence: %+v", candidate)
				}
			}
		}
		return
	}
	if len(report.Diagnostics) > 0 || len(report.Results) != 1 {
		t.Fatalf("measured worker failed: %+v", report)
		return
	}
	result := report.Results[0]
	expectPass := sample.Variant != 6
	if result.Passed != expectPass {
		t.Fatalf("unexpected measured claim result: variant=%d report=%+v", sample.Variant, result)
	}
	if !expectPass {
		if !slices.ContainsFunc(result.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "lang.golang.flow-mismatch" }) {
			t.Fatalf("an extra asynchronous call has no flow mismatch: %+v", result)
		}
	} else if len(result.Diagnostics) > 0 {
		t.Fatalf("a valid measured execution returned diagnostics: %+v", result.Diagnostics)
	}
	if result.Accepted+result.Rejected != len(result.Cases) {
		t.Fatalf("the report omitted completed candidates: accepted=%d rejected=%d cases=%d", result.Accepted, result.Rejected, len(result.Cases))
	}
	if result.Accepted == 0 {
		t.Fatal("a declared effective replay was lost")
	}
	if sample.Variant == 1 && result.Rejected == 0 {
		t.Fatal("out-of-domain generated inputs were not counted and continued")
	}
	if sample.Variant == 7 && result.Accepted < 2 {
		t.Fatal("exploration did not retain repeated effective candidates without coverage gain")
	}
	if sample.Variant == 0 && result.Accepted != 2 {
		t.Fatalf("both declared inputs must replay: %+v", result)
	}
	if result.CodeTotal <= 0 {
		t.Fatal("the measured source has no Go block denominator")
	}
	codePoints := map[string]bool{}
	for _, candidate := range result.Cases {
		hash, err := evidence.InputHash(candidate.Input)
		if err != nil || candidate.InputHash != hash {
			t.Fatalf("frozen input digest differs from canonical values: %+v %v", candidate, err)
		}
		if candidate.Rejected {
			if candidate.Passed || len(candidate.Diagnostics) > 0 || len(candidate.Trace) > 0 {
				t.Fatalf("a rejected candidate became counterexample evidence: %+v", candidate)
			}
			continue
		}
		if sample.Variant == 2 {
			box := candidate.Input.Values["box"]
			if box.Kind != "encoded" || len(box.Items) != 1 {
				t.Fatalf("a private-state input was not represented by its codec: %+v", box)
			}
			fields := map[string]lang.Value{}
			for _, field := range box.Items[0].Entries {
				fields[field.Key.Text] = field.Value
			}
			if fields["value"].Text != strconv.Itoa(value) || fields["fee"].Text != strconv.Itoa(sample.Value) {
				t.Fatalf("the codec representation lost modeled private state: %+v", box)
			}
		} else if candidate.Input.Values["n"].Text != strconv.Itoa(value) && candidate.Form == "declared" && sample.Variant != 0 {
			t.Fatalf("the declared input was not reconstructed: %+v", candidate.Input)
		}
		enters := []string{}
		publishes := []lang.TraceEvent{}
		for _, event := range candidate.Trace {
			if event.Kind == "enter" {
				enters = append(enters, event.Node)
			}
			if event.Kind == "publish" {
				publishes = append(publishes, event)
			}
		}
		if sample.Variant == 0 || sample.Variant == 1 || sample.Variant == 7 {
			if len(enters) != 1 || !strings.HasSuffix(enters[0], ".Classify") {
				t.Fatalf("predicate or repeated pure validation polluted the business trace: %+v", candidate.Trace)
			}
		}
		if sample.Variant == 4 {
			if len(enters) != 2 || !strings.HasSuffix(enters[0], ".Route") || !strings.HasSuffix(enters[1], ".Classify") {
				t.Fatalf("the synchronous trace does not preserve actual call order: %+v", candidate.Trace)
			}
		}
		if sample.Variant == 5 || sample.Variant == 6 {
			if len(publishes) != 1 || publishes[0].Node != "domain.Notice" {
				t.Fatalf("explicit normal publication was not observed: %+v", candidate.Trace)
			}
			subscriber := lang.TraceEvent{}
			for _, event := range candidate.Trace {
				if event.Kind == "enter" && strings.HasSuffix(event.Node, ".Receive") {
					subscriber = event
				}
			}
			if subscriber.Goroutine == 0 || subscriber.Goroutine == publishes[0].Goroutine || subscriber.Parent != publishes[0].Goroutine || subscriber.Time < publishes[0].Time {
				t.Fatalf("async lineage or publication time was guessed: %+v", candidate.Trace)
			}
		}
		for _, point := range candidate.Coverage {
			if point.Kind != "code" {
				continue
			}
			if point.Path != "src/domain.go" || point.Line < 1 || point.EndLine < point.Line || point.EndLine > strings.Count(source, "\n")+1 || point.Column < 1 || point.EndColumn < 1 {
				t.Fatalf("coverage does not map to original source: %+v", point)
			}
			a, okA := measuredOffset([]byte(source), point.Line, point.Column)
			b, okB := measuredOffset([]byte(source), point.EndLine, point.EndColumn)
			if !okA || !okB || a >= b || bytes.Contains([]byte(source[a:b]), []byte("unicodeMeasured")) {
				t.Fatalf("coverage includes injected source: %+v", point)
			}
			codePoints[fmt.Sprint(point)] = true
		}
		if expectPass && (sample.Variant == 4 || sample.Variant == 5) {
			count := 0
			for _, point := range candidate.Coverage {
				if point.Kind == "step" {
					count++
				}
			}
			expected := 2
			if sample.Variant == 5 {
				expected = 3
			}
			if sample.Variant == 4 {
				expected = 3
				if result.BranchTotal != 2 || !slices.ContainsFunc(candidate.Coverage, func(point lang.Coverage) bool {
					return point.Kind == "branch" && point.Node == "domain.measured#2" && point.Label == "taken"
				}) {
					t.Fatalf("the taken control branch has no branch coverage point: %+v", candidate.Coverage)
				}
			}
			if count != expected {
				t.Fatalf("actual flow steps were not measured exactly: %+v", candidate.Coverage)
			}
		}
	}
	if len(codePoints) == 0 {
		t.Fatal("no actual Go blocks were measured")
	}
	if sample.Variant == 0 && result.CodeTotal != 5 {
		t.Fatalf("branch source should have five eligible original basic blocks, got %d", result.CodeTotal)
	}
}

func measurementGraph(t rapid.TB, sample MeasurementSample, value int) graph.Graph {
	g := graph.NewGraph()
	edge := func(kind graph.EdgeKind, from, to string) {
		if err := g.AddEdge(graph.Edge{Kind: kind, From: from, To: to, Source: graph.SourceDeclared}); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id string, kind graph.NodeKind, form, parent string, attrs map[string][]string) {
		if err := g.AddNode(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerDesign, Source: graph.SourceDeclared, Origin: diag.Position{Path: "design/domain.uc", Line: 1, Column: 1}, Attrs: attrs}); err != nil {
			t.Fatal(err)
		}
		if parent != "" {
			edge(graph.EdgeKindContains, parent, id)
		}
	}
	add("domain", graph.NodeKindContainer, "component", "", map[string][]string{"paths": {"src/**"}})
	add("domain.Box", graph.NodeKindType, "object", "domain", map[string][]string{"fields": {"value: int"}, "codec": {"domain.Box.encode", "domain.decodeBox"}, "codecType": {"domain.BoxData"}})
	add("domain.BoxData", graph.NodeKindType, "record", "domain", map[string][]string{"fields": {"value: int", "fee: int"}})
	add("domain.Notice", graph.NodeKindType, "event", "domain", map[string][]string{"fields": {"value: int"}})
	add("domain.Alias", graph.NodeKindType, "object", "domain", map[string][]string{"fields": {"value: int"}})
	add("domain.Pair", graph.NodeKindType, "record", "domain", map[string][]string{"fields": {"left: domain.Alias", "right: domain.Alias"}})
	for _, op := range []struct{ id, parent, sig string }{{"domain.classify", "domain", "(n: int) -> int"}, {"domain.route", "domain", "(n: int) -> int"}, {"domain.launch", "domain", "(n: int) -> int"}, {"domain.receive", "domain", "(e: domain.Notice)"}, {"domain.decodeBox", "domain", "(d: domain.BoxData) -> domain.Box"}, {"domain.totalPair", "domain", "(pair: domain.Pair) -> int"}, {"domain.Box.encode", "domain.Box", "() -> domain.BoxData"}, {"domain.Box.total", "domain.Box", "() -> int"}} {
		add(op.id, graph.NodeKindOp, "op", op.parent, map[string][]string{"signature": {op.sig}, "pure": {"true"}})
	}
	edge(graph.EdgeKindSubscribes, "domain.receive", "domain.Notice")
	attrs := map[string][]string{"symbols": {"n: int"}, "call": {"classify(n)"}, "ensure": {"n < 0 implies result == -1", "n == 0 implies result == 0", "n > 0 implies result == 1"}}
	kind := graph.NodeKindProperty
	op := "domain.classify"
	switch sample.Variant {
	case 1:
		attrs["symbols"] = []string{"n: int generated"}
		attrs["requires"] = []string{"n > 0"}
	case 2:
		attrs = map[string][]string{"symbols": {"box: domain.Box generated"}, "call": {"box.total()"}, "ensure": {fmt.Sprintf("result == %d", value+sample.Value)}}
		op = "domain.Box.total"
	case 3:
		attrs = map[string][]string{"symbols": {"pair: domain.Pair generated"}, "call": {"totalPair(pair)"}, "ensure": {"result == pair.left.value + pair.right.value"}}
		op = "domain.totalPair"
	case 4:
		kind = graph.NodeKindFlow
		attrs["call"] = []string{"route(n)"}
		op = "domain.route"
	case 5, 6:
		kind = graph.NodeKindFlow
		attrs["call"] = []string{"launch(n)"}
		attrs["ensure"] = []string{"result == n"}
		op = "domain.launch"
	case 7:
		attrs["symbols"] = []string{"n: int generated"}
	}
	add("domain.measured", kind, "postcondition", "domain", attrs)
	edge(graph.EdgeKindRefers, "domain.measured", op)
	if sample.Variant != 3 {
		values := []string{fmt.Sprintf("n = %d", value)}
		if sample.Variant == 2 {
			values = []string{fmt.Sprintf("box = encoded({value: %d, fee: %d})", value, sample.Value)}
		}
		add("domain.measured.replay", graph.NodeKindCase, "declared", "domain.measured", map[string][]string{"values": values})
		if sample.Variant == 0 {
			add("domain.measured.other", graph.NodeKindCase, "declared", "domain.measured", map[string][]string{"values": {fmt.Sprintf("n = %d", -value)}})
		}
	}
	if kind == graph.NodeKindFlow {
		add("domain.measured#1", graph.NodeKindStep, "call", "domain.measured", map[string][]string{"index": {"1"}})
		edge(graph.EdgeKindCalls, "domain.measured#1", op)
		if sample.Variant == 4 {
			add("domain.measured#2", graph.NodeKindStep, "opt", "domain.measured#1", map[string][]string{"index": {"1"}})
			add("domain.measured#3", graph.NodeKindStep, "call", "domain.measured#2", map[string][]string{"index": {"1"}})
			edge(graph.EdgeKindCalls, "domain.measured#3", "domain.classify")
			return g
		}
		form := "call"
		target := "domain.classify"
		if sample.Variant == 5 || sample.Variant == 6 {
			form = "emit"
			target = "domain.Notice"
		}
		add("domain.measured#2", graph.NodeKindStep, form, "domain.measured#1", map[string][]string{"index": {"1"}})
		ek := graph.EdgeKindCalls
		if form == "emit" {
			ek = graph.EdgeKindEmits
		}
		edge(ek, "domain.measured#2", target)
		if form == "emit" {
			add("domain.measured#3", graph.NodeKindStep, "async", "domain.measured", map[string][]string{"index": {"2"}})
			edge(graph.EdgeKindCalls, "domain.measured#3", "domain.receive")
		}
	}
	return g
}

func measurementSource(sample MeasurementSample, value int) string {
	extra := ""
	if sample.Variant == 6 {
		extra = "Classify(e.Value);"
	}
	source := `package measured
//uc:realizes domain.classify
func Classify(n int) int {
 if n < 0 { return -1 }
 if n == 0 { return 0 }
 return 1
}
//uc:realizes domain.route
func Route(n int) int { return Classify(n) }
//uc:realizes domain.Box
type Box struct { Value int; fee int }
func (b *Box) Encode() BoxData { return BoxData{Value:b.Value,Fee:b.fee} }
func (b *Box) Total() int { return b.Value+b.fee }
//uc:realizes domain.BoxData
type BoxData struct { Value int; Fee int }
//uc:realizes domain.decodeBox
func DecodeBox(d BoxData) *Box { return &Box{Value:d.Value,fee:d.Fee} }
//uc:realizes domain.Alias
type Alias struct { Value int }
//uc:realizes domain.Pair
type Pair struct { Left,Right *Alias }
//uc:realizes domain.totalPair
func TotalPair(pair Pair) int { return pair.Left.Value+pair.Right.Value }
//uc:realizes domain.Notice
type Notice struct { Value int }
//uc:realizes domain.launch
func Launch(n int) int { Publish(Notice{Value:n});return n }
//uc:publishes domain.Notice
func Publish(e Notice) { done:=make(chan struct{});go func(){Receive(e);` + extra + `close(done)}();<-done }
//uc:realizes domain.receive
func Receive(e Notice) { _=e.Value }
`
	switch sample.Variant {
	case 1, 7:
		generated := value
		if sample.Variant == 1 {
			generated = 0
		}
		source += "\nimportPlaceholder\n"
		source = strings.Replace(source, "package measured", "package measured\nimport \"pgregory.net/rapid\"", 1)
		source = strings.Replace(source, "importPlaceholder", fmt.Sprintf("//uc:generates domain.measured.n\nfunc genInput() *rapid.Generator[int] {return rapid.Just(%d)}", generated), 1)
	case 2:
		source = strings.Replace(source, "package measured", "package measured\nimport \"pgregory.net/rapid\"", 1)
		source += fmt.Sprintf("\n//uc:generates domain.measured.box\nfunc genInput() *rapid.Generator[*Box] {return rapid.Just(&Box{Value:%d,fee:%d})}\n", value, sample.Value)
	case 3:
		source = strings.Replace(source, "package measured", "package measured\nimport \"pgregory.net/rapid\"", 1)
		source += fmt.Sprintf("\n//uc:generates domain.measured.pair\nfunc genInput() *rapid.Generator[Pair] {same:=&Alias{Value:%d};return rapid.Just(Pair{Left:same,Right:same})}\n", value)
	}
	return source
}

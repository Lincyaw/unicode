package golang

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"golang.org/x/mod/modfile"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// MonitoredRun is a copy of the monitored module after its monitored program
// called demo.score once through its entry wrapper and once directly on each
// level: the copy's directory, the runtime run record the program wrote (an
// empty report when nothing was judged), and what each call gave.
//
//uc:realizes lang.golang.MonitoredRun
type MonitoredRun struct {
	Root    string
	Record  lang.RunReport
	Wrapped []string
	Direct  []string
}

// monitoredFiles is the monitored module, but for its go.mod and go.sum:
// demo.score breaks result >= level on 3 and panics on 4, demo.ratio panics
// on -3, and the program calls score through monitor.Entry and directly on
// each level its arguments give, writing both results as JSON.
var monitoredFiles = map[string]string{
	"src/demo.go": "package demo\n\n" +
		"//uc:realizes demo.score\nfunc Score(level int) int {\n\tswitch level {\n\tcase 3:\n\t\treturn 2\n\tcase 4:\n\t\tpanic(\"score panics on four\")\n\t}\n\treturn level + 1\n}\n\n" +
		"//uc:realizes demo.ratio\nfunc Ratio(level int) int {\n\tif level == -3 {\n\t\tpanic(\"ratio panics on minus three\")\n\t}\n\treturn level\n}\n",
	"src/cmd/demo/main.go": "package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"os\"\n\t\"strconv\"\n\n" +
		"\t\"example.test/monitored/src\"\n\t\"github.com/lincyaw/unicode/monitor\"\n)\n\n" +
		"func call(score func(int) int, level int) (out string) {\n\tdefer func() {\n\t\tif p := recover(); p != nil {\n\t\t\tout = fmt.Sprint(\"panic: \", p)\n\t\t}\n\t}()\n\treturn strconv.Itoa(score(level))\n}\n\n" +
		"func main() {\n\tscore := monitor.Entry(demo.Score)\n\tout := map[string][]string{\"wrapped\": {}, \"direct\": {}}\n" +
		"\tfor _, arg := range os.Args[1:] {\n\t\tlevel, _ := strconv.Atoi(arg)\n\t\tout[\"wrapped\"] = append(out[\"wrapped\"], call(score, level))\n\t\tout[\"direct\"] = append(out[\"direct\"], call(demo.Score, level))\n\t}\n" +
		"\tmonitor.Flush()\n\t_ = json.NewEncoder(os.Stdout).Encode(out)\n}\n",
}

// monitoredTemp is the process's temporary directory, taken before any case
// points TMPDIR at a directory of its own, which the case's end removes.
var monitoredTemp = os.TempDir()

// monitored is the monitored module, written, generated and built once per
// process: the copies of the cases share its graph and its program.
var monitored struct {
	once    sync.Once
	dir     string
	program string
	graph   *graph.Graph
	err     error
}

// monitoredModule writes the monitored module into a new directory with a
// go.mod that replaces this repository's module by its directory, generates
// its drivers and its monitor file, and builds its program with the build
// constraint unicode_monitor.
func monitoredModule() (string, string, *graph.Graph, error) {
	monitored.once.Do(func() {
		monitored.err = func() error {
			// A derivation runs in a directory of its own, so the repository is
			// found from this file's path rather than the working directory.
			_, file, _, ok := runtime.Caller(0)
			if !ok {
				return errors.New("the test file's path is unknown")
			}
			repository := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
			sum, err := os.ReadFile(filepath.Join(repository, "go.sum"))
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp(monitoredTemp, "monitored-module-")
			if err != nil {
				return err
			}
			// The module requires every module this repository requires, at the
			// same versions, so that its build list is complete without the network.
			modData, err := os.ReadFile(filepath.Join(repository, "go.mod"))
			if err != nil {
				return err
			}
			parsed, err := modfile.Parse("go.mod", modData, nil)
			if err != nil {
				return err
			}
			mod := "module example.test/monitored\n\ngo 1.26.2\n\nrequire (\n\tgithub.com/lincyaw/unicode v0.0.0\n"
			for _, required := range parsed.Require {
				mod += "\t" + required.Mod.Path + " " + required.Mod.Version + "\n"
			}
			mod += ")\n\nreplace github.com/lincyaw/unicode => " + repository + "\n"
			files := map[string]string{"go.mod": mod, "go.sum": string(sum)}
			for name, content := range monitoredFiles {
				files[name] = content
			}
			if err := writeFiles(dir, files); err != nil {
				return err
			}
			g, err := monitoredGraph(dir)
			if err != nil {
				return err
			}
			generated := NewBackend().Generate(lang.GenPlan{Root: dir, Roots: []string{"src"}, Graph: &g})
			if len(generated.Diagnostics) > 0 {
				return fmt.Errorf("the monitored module does not generate: %+v", generated.Diagnostics)
			}
			edits := map[string]string{}
			for _, edit := range generated.Edits {
				edits[edit.Path] = string(edit.Content)
			}
			if err := writeFiles(dir, edits); err != nil {
				return err
			}
			program := filepath.Join(dir, "demo-program")
			build := exec.Command("go", "build", "-buildvcs=false", "-tags", monitorBuildTag, "-o", program, "./src/cmd/demo")
			build.Dir = dir
			if output, err := build.CombinedOutput(); err != nil {
				return fmt.Errorf("the monitored program does not build: %w: %s", err, output)
			}
			monitored.dir, monitored.program, monitored.graph = dir, program, &g
			return nil
		}()
	})
	return monitored.dir, monitored.program, monitored.graph, monitored.err
}

func writeFiles(root string, files map[string]string) error {
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// monitoredGraph is the monitored module's design, linked with its code:
// the pure operations demo.score and demo.ratio, and demo.bounded, judged at
// runtime on score.
func monitoredGraph(root string) (graph.Graph, error) {
	g := graph.NewGraph()
	var problems []error
	at := diag.Position{Path: "design/demo.uc", Line: 1, Column: 1}
	add := func(id string, kind graph.NodeKind, form, parent string, attrs map[string][]string) {
		problems = append(problems, g.AddNode(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerDesign, Source: graph.SourceDeclared, Origin: at, Attrs: attrs}))
		if parent != "" {
			problems = append(problems, g.AddEdge(graph.Edge{Kind: graph.EdgeKindContains, From: parent, To: id, Source: graph.SourceDeclared}))
		}
	}
	add("demo", graph.NodeKindContainer, "component", "", map[string][]string{"paths": {"src/**"}})
	add("demo.score", graph.NodeKindOp, "op", "demo", map[string][]string{"signature": {"(level: int) -> int"}, "pure": {"true"}})
	add("demo.ratio", graph.NodeKindOp, "op", "demo", map[string][]string{"signature": {"(level: int) -> int"}, "pure": {"true"}})
	add("demo.bounded", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{
		"symbols": {"level: int"}, "limits": {"level in -5..5"}, "requires": {"level != 0"}, "call": {"score(level)"},
		"ensure": {"result >= level", "ratio(level) == level"},
	})
	problems = append(problems,
		g.AddEdge(graph.Edge{Kind: graph.EdgeKindRefers, From: "demo.bounded", To: "demo.score", Source: graph.SourceDeclared, Attrs: map[string][]string{"clauses": {"call#1"}}}),
		g.AddEdge(graph.Edge{Kind: graph.EdgeKindRefers, From: "demo.bounded", To: "demo.ratio", Source: graph.SourceDeclared, Attrs: map[string][]string{"clauses": {"ensure#2"}}}))
	if err := errors.Join(problems...); err != nil {
		return g, err
	}
	extracted, err := NewBackend().ExtractForGeneration(root, []string{"src"})
	if err != nil {
		return g, err
	}
	if err := g.AddFragment(extracted.Fragment); err != nil {
		return g, err
	}
	if ds := g.Derive(); len(ds) > 0 {
		return g, fmt.Errorf("the monitored module does not link: %+v", ds)
	}
	return g, nil
}

// deriveMonitoredRun copies the monitored module into a new directory and
// runs its monitored program there with the sampling rate 1 and collection
// on, on each level, reading back the runtime run record it wrote.
//
//uc:generates lang.golang.monitored-judgments-match-replay.run
//uc:generates lang.golang.monitoring-keeps-calls.run
func deriveMonitoredRun(levels []int) (MonitoredRun, error) {
	module, program, _, err := monitoredModule()
	if err != nil {
		return MonitoredRun{}, err
	}
	root, err := os.MkdirTemp("", "monitored-run-")
	if err != nil {
		return MonitoredRun{}, err
	}
	if err := os.CopyFS(root, os.DirFS(module)); err != nil {
		return MonitoredRun{}, err
	}
	var args []string
	for _, level := range levels {
		args = append(args, strconv.Itoa(level))
	}
	cmd := exec.Command(program, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "UNICODE_MONITOR_RATE=1", "UNICODE_MONITOR_TRAFFIC=on", "UNICODE_MONITOR_KEY=monitored-run",
		"UNICODE_MONITOR_DIR="+filepath.Join(root, ".unicode"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return MonitoredRun{}, fmt.Errorf("the monitored program failed: %w: %s", err, stderr.Bytes())
	}
	var calls struct{ Wrapped, Direct []string }
	if err := json.Unmarshal(stdout.Bytes(), &calls); err != nil {
		return MonitoredRun{}, err
	}
	out := MonitoredRun{Root: root, Wrapped: calls.Wrapped, Direct: calls.Direct, Record: lang.RunReport{Results: []lang.RunResult{}, Diagnostics: []diag.Diagnostic{}}}
	loaded := evidence.Read(root)
	if len(loaded.Diagnostics) > 0 {
		return out, fmt.Errorf("the runtime record cannot be read: %+v", loaded.Diagnostics)
	}
	for _, report := range loaded.Reports {
		if report.Venue == lang.VenueRuntime {
			out.Record = report
		}
	}
	return out, nil
}

// deriveMonitoredPlan is the plan that replays demo.bounded on the copy run
// holds, its inputs those the collected calls give it (docs/language.md E29).
//
//uc:generates lang.golang.monitored-judgments-match-replay.plan
func deriveMonitoredPlan(run MonitoredRun) (lang.RunPlan, error) {
	_, _, g, err := monitoredModule()
	if err != nil {
		return lang.RunPlan{}, err
	}
	traffic := evidence.ReadTraffic(run.Root, g)
	if len(traffic.Diagnostics) > 0 {
		return lang.RunPlan{}, fmt.Errorf("the collected calls cannot be read: %+v", traffic.Diagnostics)
	}
	return lang.RunPlan{Root: run.Root, Roots: []string{"src"}, Graph: g, Ids: []string{"demo.bounded"}, Traffic: evidence.TrafficInputs(g, traffic.Collected)}, nil
}

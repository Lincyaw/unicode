package main

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/design"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
	"github.com/lincyaw/unicode/internal/view"
)

// candidate is a node that a query command may name: its identity, kind,
// form and layer.
type candidate struct {
	id    string
	kind  graph.NodeKind
	form  string
	layer graph.Layer
}

// candidates lists the nodes of a repository's graph that can be told from
// its files without extracting the code: the design's, from design.Load,
// and the Go packages, files, types and functions, from go/parser.
func candidates(files map[string]RepoFile) []candidate {
	_, trees := readExpected(files)
	loaded := design.Load(trees)
	var out []candidate
	for _, n := range loaded.Graph.Nodes() {
		out = append(out, candidate{n.Id, n.Kind, n.Form, n.Layer})
	}
	module := ""
	if mod, ok := files["go.mod"]; ok {
		module, _, _ = strings.Cut(strings.TrimPrefix(mod.Content, "module "), "\n")
	}
	packages := map[string]bool{}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if !strings.HasSuffix(p, ".go") || strings.HasPrefix(p, "rapid/") {
			continue
		}
		pkg := "go:" + module + "/" + path.Dir(p)
		if !packages[pkg] {
			packages[pkg] = true
			out = append(out, candidate{pkg, graph.NodeKindContainer, "package", graph.LayerCode})
		}
		out = append(out, candidate{"file:" + p, graph.NodeKindContainer, "file", graph.LayerCode})
		f, err := parser.ParseFile(token.NewFileSet(), p, files[p].Content, 0)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				id := pkg + "." + d.Name.Name
				if d.Recv != nil {
					if recv, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
						id = pkg + "." + recv.Name + "." + d.Name.Name
					}
				}
				out = append(out, candidate{id, graph.NodeKindOp, "func", graph.LayerCode})
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if s, ok := s.(*ast.TypeSpec); ok {
						out = append(out, candidate{pkg + "." + s.Name.Name, graph.NodeKindType, "", graph.LayerCode})
					}
				}
			}
		}
	}
	return out
}

// viewInputs tells which candidates a view kind takes (docs/design.md §7.1).
var viewInputs = map[view.ViewKind]func(candidate) bool{
	view.ViewKindArchitecture: isDesignContainerCandidate,
	view.ViewKindClass: func(c candidate) bool {
		return c.layer == graph.LayerDesign && (c.kind == graph.NodeKindContainer || c.kind == graph.NodeKindType)
	},
	view.ViewKindSequence: func(c candidate) bool { return c.kind == graph.NodeKindFlow },
	view.ViewKindActivity: func(c candidate) bool { return c.kind == graph.NodeKindFlow },
	view.ViewKindEvents:   isDesignContainerCandidate,
	view.ViewKindCalls:    func(c candidate) bool { return c.kind == graph.NodeKindOp },
	view.ViewKindProperty: func(c candidate) bool {
		return c.kind == graph.NodeKindProperty || isDesignContainerCandidate(c)
	},
	view.ViewKindCode: func(c candidate) bool {
		return c.layer == graph.LayerCode && (c.kind == graph.NodeKindType || c.form == "file")
	},
	view.ViewKindDdd: isDesignContainerCandidate,
	view.ViewKindC4:  isDesignContainerCandidate,
}

func isDesignContainerCandidate(c candidate) bool {
	return c.kind == graph.NodeKindContainer && c.layer == graph.LayerDesign
}

// genQueryCommand draws a query command and its arguments for the repository:
// node get, node refs, node search, slice, impact or view, with identities of
// its graph's nodes and now and then one that is not, and now and then
// --rev HEAD. A search text goes after --, as a user writes free text, when it
// starts with - and now and then otherwise; -- comes last, since it ends the flags.
//
//uc:generates cli.query-graph.command
func genQueryCommand(files map[string]RepoFile) *rapid.Generator[[]string] {
	known := candidates(files)
	return rapid.Custom(func(t *rapid.T) []string {
		id := func(label string, of func(candidate) bool) string {
			pool := known
			if of != nil {
				pool = slices.DeleteFunc(slices.Clone(known), func(c candidate) bool { return !of(c) })
			}
			if len(pool) == 0 || rapid.IntRange(0, 9).Draw(t, label+"Unknown?") == 0 {
				return rapid.SampledFrom([]string{"nowhere", "order.Nothing", "go:" + modulePath + "/internal/none"}).Draw(t, label+"Unknown")
			}
			return rapid.SampledFrom(pool).Draw(t, label).id
		}
		ids := func(label string, least int) []string {
			var out []string
			for range rapid.IntRange(least, 3).Draw(t, label+"Count") {
				out = append(out, id(label, nil))
			}
			return out
		}
		var command, free []string
		switch rapid.IntRange(0, 5).Draw(t, "command") {
		case 0:
			command = []string{"node", "get", id("get", nil)}
		case 1:
			command = []string{"node", "refs", id("refs", nil)}
		case 2:
			observations := slices.DeleteFunc(slices.Clone(known), func(c candidate) bool { return c.kind != graph.NodeKindObservation })
			if len(observations) > 0 && rapid.Bool().Draw(t, "observationSearch") {
				command = []string{"node", "search", rapid.SampledFrom(observations).Draw(t, "observation").id, "--kind", string(graph.NodeKindObservation)}
				break
			}
			text := id("searched", nil)
			runes := []rune(text)
			start := rapid.IntRange(0, len(runes)).Draw(t, "start")
			searched := string(runes[start:rapid.IntRange(start, len(runes)).Draw(t, "end")])
			command = []string{"node", "search"}
			if strings.HasPrefix(searched, "-") || rapid.Bool().Draw(t, "endOfFlags?") {
				free = []string{searched}
			} else {
				command = append(command, searched)
			}
			if rapid.Bool().Draw(t, "kind?") {
				command = append(command, "--kind", string(rapid.SampledFrom(nodeKinds).Draw(t, "kind")))
			}
		case 3:
			command = append([]string{"slice"}, ids("focus", 1)...)
			for _, x := range ids("expand", 0) {
				command = append(command, "--expand", x)
			}
			if rapid.Bool().Draw(t, "withCode?") {
				command = append(command, "--with-code")
			}
		case 4:
			command = append([]string{"impact"}, ids("changed", 1)...)
		default:
			kind := rapid.SampledFrom(slices.Sorted(maps.Keys(viewInputs))).Draw(t, "view")
			command = []string{"view", string(kind)}
			takes := viewInputs[kind]
			if rapid.IntRange(0, 4).Draw(t, "anyInput?") == 0 {
				takes = nil
			}
			if !slices.Contains(wholeSystemViews, kind) || rapid.Bool().Draw(t, "input?") {
				command = append(command, id("input", takes))
			}
			if rapid.IntRange(0, 3).Draw(t, "layer?") == 0 {
				command = append(command, "--layer", rapid.SampledFrom([]string{"design", "code"}).Draw(t, "layer"))
			}
			if rapid.IntRange(0, 3).Draw(t, "level?") == 0 {
				command = append(command, "--level", rapid.SampledFrom([]string{"context", "container"}).Draw(t, "level"))
			}
			for range rapid.IntRange(0, 2).Draw(t, "expandCount") {
				x := id("expand", func(c candidate) bool { return c.kind == graph.NodeKindContainer || c.kind == graph.NodeKindOp })
				if rapid.IntRange(0, 3).Draw(t, "reaches?") == 0 {
					x = "reaches " + x + " " + id("reachesTo", func(c candidate) bool { return c.kind == graph.NodeKindOp })
				}
				command = append(command, "--expand", x)
			}
		}
		if rapid.IntRange(0, 4).Draw(t, "rev?") == 0 {
			command = append(command, "--rev", "HEAD")
		}
		if free != nil {
			command = append(append(command, "--"), free...)
		}
		return command
	})
}

// wholeSystemViews are the views that take the whole system when no input is given.
var wholeSystemViews = []view.ViewKind{view.ViewKindArchitecture, view.ViewKindEvents, view.ViewKindDdd, view.ViewKindC4}

// parsedCommand is a query command as genCommand writes it.
type parsedCommand struct {
	positional []string
	expand     []string
	kind       string
	layer      string
	level      string
	withCode   bool
	rev        bool
}

func parseCommand(command []string) parsedCommand {
	var p parsedCommand
	for i := 0; i < len(command); i++ {
		switch command[i] {
		case "--expand":
			i++
			p.expand = append(p.expand, command[i])
		case "--kind":
			i++
			p.kind = command[i]
		case "--layer":
			i++
			p.layer = command[i]
		case "--level":
			i++
			p.level = command[i]
		case "--rev":
			i++
			p.rev = true
		case "--with-code":
			p.withCode = true
		case "--":
			p.positional = append(p.positional, command[i+1:]...)
			return p
		default:
			p.positional = append(p.positional, command[i])
		}
	}
	return p
}

// directResult calls query or view as the command asks, on the graph g, and
// returns the result and whether the command names an identity that is not
// a node of g, or a view that does not apply to its input or does not read
// its --layer or --level.
func directResult(t rapid.TB, g *graph.Graph, p parsedCommand) (any, bool, bool) {
	missing := func(ids ...string) bool {
		return slices.ContainsFunc(ids, func(id string) bool { return g.Node(id) == nil })
	}
	args := p.positional
	var result any
	var err error
	switch args[0] {
	case "node":
		switch args[1] {
		case "get":
			result, err = query.Get(g, args[2])
		case "refs":
			result, err = query.Refs(g, args[2])
		default:
			var kind *graph.NodeKind
			if p.kind != "" {
				k := graph.NodeKind(p.kind)
				kind = &k
			}
			result = query.Search(g, args[2], kind)
		}
	case "slice":
		result, err = query.Slice(g, args[1:], p.expand)
	case "impact":
		result, err = query.Impact(g, args[1:])
	case "view":
		kind, id := view.ViewKind(args[1]), ""
		if len(args) > 2 {
			id = args[2]
		}
		if p.layer != "" && kind != view.ViewKindClass || p.level != "" && kind != view.ViewKindC4 {
			return nil, false, true
		}
		if missing(viewIdentitiesOf(id, p.expand)...) {
			return nil, true, false
		}
		layer, level := view.ClassLayerDesign, view.C4LevelContainer
		if p.layer != "" {
			layer = view.ClassLayer(p.layer)
		}
		if p.level != "" {
			level = view.C4Level(p.level)
		}
		result, err = view.Select(g, kind, id, p.expand, layer, level)
		if errors.As(err, new(view.NotApplicable)) {
			return nil, false, true
		}
	}
	if errors.As(err, new(query.NotFound)) {
		return nil, true, false
	}
	if err != nil {
		t.Fatalf("%s refused %v for a reason other than an unknown identity: %v", args[0], args, err)
	}
	return result, false, false
}

// viewIdentitiesOf are the node identities a view request names: its input
// and each expanded node or end of an expanded reaches edge.
func viewIdentitiesOf(id string, expand []string) []string {
	var ids []string
	if id != "" {
		ids = append(ids, id)
	}
	for _, x := range expand {
		ids = append(ids, strings.Fields(strings.TrimPrefix(x, "reaches "))...)
	}
	return ids
}

// identities collects every string under a key "id" in decoded JSON.
func identities(v any, into map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if s, ok := x.(string); ok && k == "id" {
				into[s] = true
			}
			identities(x, into)
		}
	case []any:
		for _, x := range v {
			identities(x, into)
		}
	}
}

func decodeJSON(t rapid.TB, what string, data []byte) any {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s is not JSON: %v\n%s", what, err, data)
	}
	return v
}

//uc:checks cli.query-graph
func checkFlowQueryGraph(t rapid.TB, files map[string]RepoFile, command []string) {
	root := writeRepo(t, files)
	defer removeAll(t, filepath.Dir(root))
	direct := buildDirectly(t, root, files)
	if len(direct.diags) > 0 {
		t.Fatalf("setup failed: the drawn repository is not legal, building it gives %+v", direct.diags)
	}
	p := parseCommand(command)
	if p.rev {
		// The working tree no longer holds a design, so only the commit's graph answers.
		gitCommit(t, root)
		if err := os.RemoveAll(filepath.Join(root, "design")); err != nil {
			t.Fatalf("setup failed: cannot remove design/ from the working tree: %v", err)
		}
	}
	result, unknown, inapplicable := directResult(t, &direct.graph, p)
	args := append([]string{"--root", root, "--format", "json"}, command...)
	code, stdout, stderr := runMain(t, args)
	switch {
	case unknown:
		if code != exitNotFound || stdout != "" || stderr == "" {
			t.Fatalf("unicode %s names an identity that is not a node of the graph, so it should exit with %d and say so on stderr, yet it exited with %d\nstdout: %s\nstderr: %s",
				strings.Join(command, " "), exitNotFound, code, stdout, stderr)
		}
		return
	case inapplicable:
		if code != exitUsage || stdout != "" || stderr == "" {
			t.Fatalf("the view of unicode %s does not apply to its input, or does not read its --layer or --level, so it should exit with %d and say why on stderr, yet it exited with %d\nstdout: %s\nstderr: %s",
				strings.Join(command, " "), exitUsage, code, stdout, stderr)
		}
		return
	}
	if code != exitOK || stderr != "" {
		t.Fatalf("unicode %s exited with %d instead of %d, or wrote to stderr\nstdout: %s\nstderr: %s", strings.Join(command, " "), code, exitOK, stdout, stderr)
	}
	got := decodeJSON(t, "the output of unicode "+strings.Join(command, " "), []byte(stdout))
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("cannot encode the result of calling %s directly: %v", p.positional[0], err)
	}
	want := decodeJSON(t, "the direct result", encoded)
	if p.withCode {
		checkCode(t, root, files, got, result.(query.Context))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the JSON of unicode %s differs from the result of calling %s directly\ngot:\n%s\nwant:\n%s", strings.Join(command, " "), p.positional[0], stdout, encoded)
	}

	code, text, stderr := runMain(t, append([]string{"--root", root}, command...))
	if code != exitOK || stderr != "" {
		t.Fatalf("with --format text, unicode %s exited with %d instead of %d, or wrote to stderr: %s", strings.Join(command, " "), code, exitOK, stderr)
	}
	listed := map[string]bool{}
	identities(want, listed)
	for _, id := range sortedKeys(listed) {
		if !strings.Contains(text, id) {
			t.Fatalf("with --format text, unicode %s leaves out %s, which its result lists\n%s", strings.Join(command, " "), id, text)
		}
	}
}

// checkCode checks the code that slice --with-code attaches, and removes it
// from got: one entry for each code type and function of the slice, holding
// the line of the file where the node's declaration stands.
func checkCode(t rapid.TB, root string, files map[string]RepoFile, got any, s query.Context) {
	out, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("the output of slice --with-code is not a JSON object")
	}
	code, _ := out["code"].(map[string]any)
	delete(out, "code")
	want := map[string]string{}
	for _, n := range sliceNodes(s) {
		if n.Layer == graph.LayerCode && (n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindOp) && strings.HasSuffix(n.Origin.Path, ".go") {
			want[n.Id] = strings.Split(files[n.Origin.Path].Content, "\n")[n.Origin.Line-1]
		}
	}
	if len(code) != len(want) {
		t.Fatalf("slice --with-code attaches the code of %d nodes, yet the slice has %d code types and functions: %v", len(code), len(want), sortedKeys(want))
	}
	for id, line := range want {
		if text, _ := code[id].(string); !strings.Contains(text, line) {
			t.Fatalf("slice --with-code attaches to %s the code\n%s\nwhich lacks the line of its declaration %q", id, text, line)
		}
	}
}

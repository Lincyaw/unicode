package golang

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	gotrace "golang.org/x/exp/trace"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

type measuredFrame struct {
	publicationStarts   map[string]int
	owner               *measuredFrame
	node                string
	goroutine, parent   int
	begin, end, created int
	children            []*measuredFrame
	publications        []lang.TraceEvent
	errors              []string
}
type measuredTrace struct {
	events   []lang.TraceEvent
	roots    []*measuredFrame
	measured bool
}

func (p *measuredProgram) trace(path string) (measuredTrace, error) {
	file, err := os.Open(path)
	if err != nil {
		return measuredTrace{}, err
	}
	// The trace is read-only; close cannot change the decoded execution.
	defer func() { _ = file.Close() }()
	reader, err := gotrace.NewReader(file)
	if err != nil {
		return measuredTrace{}, err
	}
	out := measuredTrace{events: []lang.TraceEvent{}, roots: []*measuredFrame{}}
	parents := map[int]int{}
	creators := map[int]*measuredFrame{}
	createdAt := map[int]int{}
	active := map[int]bool{}
	stacks := map[int][]*measuredFrame{}
	for {
		event, err := reader.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		gid := int(event.Goroutine())
		at := int(event.Time())
		if event.Kind() == gotrace.EventStateTransition {
			transition := event.StateTransition()
			if transition.Resource.Kind == gotrace.ResourceGoroutine {
				old, next := transition.Goroutine()
				child := int(transition.Resource.Goroutine())
				if old == gotrace.GoNotExist && next == gotrace.GoRunnable {
					parents[child] = gid
					createdAt[child] = at
					if active[gid] {
						active[child] = true
						if stack := stacks[gid]; len(stack) > 0 {
							creators[child] = stack[len(stack)-1]
						}
					}
				}
				if next == gotrace.GoNotExist {
					active[child] = false
				}
			}
			continue
		}
		if event.Kind() != gotrace.EventLog {
			continue
		}
		log := event.Log()
		if log.Category != "unicode" {
			continue
		}
		if log.Message == "begin" {
			active[gid] = true
			out.measured = true
			continue
		}
		if log.Message == "end" {
			active[gid] = false
			continue
		}
		if !active[gid] {
			continue
		}
		kind, node, ok := strings.Cut(log.Message, ":")
		if !ok {
			return out, fmt.Errorf("the measured trace contains an invalid event")
		}
		if kind == "invalid" {
			return out, fmt.Errorf("the measured trace could not observe %s", node)
		}
		if kind == "modelEnter" {
			kind = "enter"
		}
		if kind == "modelExit" {
			kind = "exit"
		}
		if kind == "publication" {
			stack := stacks[gid]
			if len(stack) == 0 {
				return out, fmt.Errorf("a publishing boundary has no active native frame")
			}
			frame := stack[len(stack)-1]
			if frame.publicationStarts == nil {
				frame.publicationStarts = map[string]int{}
			}
			frame.publicationStarts[node] = at
			continue
		}
		nodes := []string{node}
		if kind == "error" {
			nodes = p.nativeErrors[node]
			if len(nodes) == 0 {
				continue
			}
		}
		if kind == "modelError" {
			kind = "error"
		}
		for _, node := range nodes {
			observed := lang.TraceEvent{Kind: kind, Node: node, Goroutine: gid, Parent: parents[gid], Time: at}
			if kind == "publish" {
				stack := stacks[gid]
				if len(stack) > 0 {
					if started, ok := stack[len(stack)-1].publicationStarts[node]; ok {
						observed.Time = started
						observed.Detail = "publishing boundary returned normally"
					}
				}
			}
			out.events = append(out.events, observed)
			switch kind {
			case "enter":
				frame := &measuredFrame{node: node, goroutine: gid, parent: parents[gid], begin: at, created: createdAt[gid]}
				if stack := stacks[gid]; len(stack) > 0 {
					owner := stack[len(stack)-1]
					owner.children = append(owner.children, frame)
					frame.owner = owner
				} else if owner := creators[gid]; owner != nil {
					owner.children = append(owner.children, frame)
					frame.owner = owner
				} else {
					out.roots = append(out.roots, frame)
				}
				stacks[gid] = append(stacks[gid], frame)
			case "exit":
				stack := stacks[gid]
				if len(stack) == 0 || stack[len(stack)-1].node != node {
					return out, fmt.Errorf("the measured function trace is unbalanced at %s", node)
				}
				stack[len(stack)-1].end = at
				stacks[gid] = stack[:len(stack)-1]
			case "publish":
				stack := stacks[gid]
				if len(stack) == 0 {
					return out, fmt.Errorf("an event publication has no measured publishing function")
				}
				owner := stack[len(stack)-1]
				owner.publications = append(owner.publications, observed)
			case "error":
				stack := stacks[gid]
				if len(stack) > 0 {
					owner := stack[len(stack)-1]
					owner.errors = append(owner.errors, node)
				}
			default:
				return out, fmt.Errorf("unsupported measured event %s", kind)
			}
		}
	}
	slices.SortStableFunc(out.events, func(a, b lang.TraceEvent) int { return a.Time - b.Time })
	return out, nil
}

type flowAction struct {
	kind, node         string
	at, end, goroutine int
	entered            int
	causes             map[string]bool
	children           []*flowAction
	async              []*flowAction
	errors             []string
}
type flowMatcher struct {
	allAsync      []*flowAction
	overflow      bool
	g             *graph.Graph
	nodes         map[string]graph.Node
	children      map[string][]graph.Node
	targets       map[string]string
	errors        map[string]string
	subscriptions map[string]string
}
type flowState struct {
	index      int
	used       map[*flowAction]bool
	sent       map[string][]*flowAction
	published  map[string]int
	coverage   []lang.Coverage
	lastErrors []string
	lastTime   int
}

func newFlowMatcher(g *graph.Graph) *flowMatcher {
	matcher := &flowMatcher{g: g, nodes: map[string]graph.Node{}, children: map[string][]graph.Node{}, targets: map[string]string{}, errors: map[string]string{}, subscriptions: map[string]string{}}
	for _, node := range g.Nodes() {
		matcher.nodes[node.Id] = node
	}
	for _, edge := range g.Edges() {
		switch edge.Kind {
		case graph.EdgeKindContains:
			if matcher.nodes[edge.To].Kind == graph.NodeKindStep {
				matcher.children[edge.From] = append(matcher.children[edge.From], matcher.nodes[edge.To])
			}
		case graph.EdgeKindCalls, graph.EdgeKindEmits:
			if matcher.nodes[edge.From].Kind == graph.NodeKindStep {
				matcher.targets[edge.From] = edge.To
			}
		case graph.EdgeKindHandles:
			matcher.errors[edge.From] = edge.To
		case graph.EdgeKindSubscribes:
			matcher.subscriptions[edge.From] = edge.To
		}
	}
	for id := range matcher.children {
		slices.SortFunc(matcher.children[id], func(a, b graph.Node) int {
			ai, _ := strconv.Atoi(first(a.Attrs["index"]))
			bi, _ := strconv.Atoi(first(b.Attrs["index"]))
			return ai - bi
		})
	}
	return matcher
}
func (m *flowMatcher) model(node string) string {
	if m.nodes[node].Layer == graph.LayerDesign {
		return node
	}
	for _, edge := range m.g.Edges() {
		if edge.Kind == graph.EdgeKindRealizes && edge.From == node && m.nodes[edge.To].Kind == graph.NodeKindOp {
			return edge.To
		}
	}
	return ""
}
func (m *flowMatcher) actions(frames []*measuredFrame, parent int) ([]*flowAction, []*flowAction) {
	var syncActions, asyncActions []*flowAction
	for _, frame := range frames {
		children, async := m.actions(frame.children, frame.goroutine)
		for _, publication := range frame.publications {
			children = append(children, &flowAction{kind: "emit", node: publication.Node, at: publication.Time, end: publication.Time, goroutine: frame.goroutine})
		}
		slices.SortStableFunc(children, func(a, b *flowAction) int { return a.at - b.at })
		if model := m.model(frame.node); model != "" {
			causes := map[string]bool{}
			for owner := frame.owner; owner != nil; owner = owner.owner {
				for _, published := range owner.publications {
					causes[published.Node] = true
				}
			}
			action := &flowAction{kind: "call", node: model, entered: frame.begin, causes: causes, at: frame.begin, end: frame.end, goroutine: frame.goroutine, children: children, async: async, errors: frame.errors}
			if parent != 0 && frame.goroutine != parent {
				action.at = frame.created
				asyncActions = append(asyncActions, action)
			} else {
				syncActions = append(syncActions, action)
			}
		} else {
			if parent != 0 && frame.goroutine != parent {
				asyncActions = append(asyncActions, children...)
				asyncActions = append(asyncActions, async...)
			} else {
				syncActions = append(syncActions, children...)
				asyncActions = append(asyncActions, async...)
			}
		}
	}
	slices.SortStableFunc(syncActions, func(a, b *flowAction) int { return a.at - b.at })
	slices.SortStableFunc(asyncActions, func(a, b *flowAction) int { return a.at - b.at })
	return syncActions, asyncActions
}
func cloneFlowState(s flowState) flowState {
	s.used = maps.Clone(s.used)
	s.published = maps.Clone(s.published)
	s.sent = maps.Clone(s.sent)
	s.coverage = slices.Clone(s.coverage)
	s.lastErrors = slices.Clone(s.lastErrors)
	return s
}
func (m *flowMatcher) covers(s flowState, node graph.Node, label string) flowState {
	point := lang.Coverage{Kind: "step", Node: node.Id, Path: node.Origin.Path, Line: node.Origin.Line, Column: node.Origin.Column, EndLine: node.Origin.Line, EndColumn: node.Origin.Column, Label: label}
	s.coverage = append(s.coverage, point)
	if label != "" {
		point.Kind = "branch"
		s.coverage = append(s.coverage, point)
	}
	return s
}
func (m *flowMatcher) errorMatches(actual, want string) bool {
	pending := []string{actual}
	seen := map[string]bool{}
	for len(pending) > 0 {
		id := pending[0]
		pending = pending[1:]
		if id == want {
			return true
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, edge := range m.g.Edges() {
			if edge.Kind == graph.EdgeKindExtends && edge.From == id {
				pending = append(pending, edge.To)
			}
		}
	}
	return false
}
func (m *flowMatcher) sequence(steps []graph.Node, actions, async []*flowAction, s flowState, depth int) []flowState {
	if depth > 256 {
		return nil
	}
	if len(steps) == 0 {
		return []flowState{s}
	}
	step, rest := steps[0], steps[1:]
	var alternatives []flowState
	target := m.targets[step.Id]
	switch step.Form {
	case "call", "emit":
		if s.index >= len(actions) {
			return nil
		}
		action := actions[s.index]
		if action.node != target || action.end == 0 || action.kind != map[string]string{"call": "call", "emit": "emit"}[step.Form] || action.at < s.lastTime {
			return nil
		}
		next := cloneFlowState(s)
		next.index++
		next.lastTime = action.end
		next.lastErrors = action.errors
		next = m.covers(next, step, "")
		if step.Form == "emit" {
			next.published[target] = action.at
			alternatives = append(alternatives, next)
			break
		}
		sub := cloneFlowState(next)
		sub.index = 0
		sub.lastTime = 0
		for _, matched := range m.sequence(m.children[step.Id], action.children, action.async, sub, depth+1) {
			if matched.index != len(action.children) {
				continue
			}
			matched.index = next.index
			matched.lastTime = next.lastTime
			matched.lastErrors = action.errors
			alternatives = append(alternatives, matched)
		}
	case "send":
		for _, action := range async {
			if s.used[action] || action.node != target || action.at < s.lastTime {
				continue
			}
			next := cloneFlowState(s)
			next.used[action] = true
			next.sent[target] = append(slices.Clone(next.sent[target]), action)
			next.lastTime = action.at
			alternatives = append(alternatives, m.covers(next, step, ""))
		}
	case "async":
		candidates := append(append(slices.Clone(s.sent[target]), async...), m.allAsync...)
		for _, action := range candidates {
			if action.node != target || action.end == 0 {
				continue
			}
			sent := slices.Contains(s.sent[target], action)
			if s.used[action] && !sent {
				continue
			}
			event := m.subscriptions[target]
			if !sent && (s.published[event] == 0 || action.entered < s.published[event] || !action.causes[event]) {
				continue
			}
			next := cloneFlowState(s)
			next.used[action] = true
			next.sent[target] = nil
			next = m.covers(next, step, "")
			sub := cloneFlowState(next)
			sub.index = 0
			sub.lastTime = 0
			for _, matched := range m.sequence(m.children[step.Id], action.children, action.async, sub, depth+1) {
				if matched.index != len(action.children) {
					continue
				}
				matched.index = s.index
				matched.lastTime = s.lastTime
				alternatives = append(alternatives, matched)
			}
		}
	case "on":
		handled := false
		for _, actual := range s.lastErrors {
			if m.errorMatches(actual, m.errors[step.Id]) {
				handled = true
			}
		}
		if !handled {
			alternatives = append(alternatives, m.covers(cloneFlowState(s), step, "skipped"))
		} else {
			alternatives = append(alternatives, m.sequence(m.children[step.Id], actions, async, m.covers(cloneFlowState(s), step, "taken"), depth+1)...)
		}
	case "alt":
		for _, label := range step.Attrs["label"] {
			var branch []graph.Node
			for _, child := range m.children[step.Id] {
				if first(child.Attrs["branch"]) == label {
					branch = append(branch, child)
				}
			}
			alternatives = append(alternatives, m.sequence(branch, actions, async, m.covers(cloneFlowState(s), step, label), depth+1)...)
		}
	case "opt":
		alternatives = append(alternatives, m.covers(cloneFlowState(s), step, "skipped"))
		alternatives = append(alternatives, m.sequence(m.children[step.Id], actions, async, m.covers(cloneFlowState(s), step, "taken"), depth+1)...)
	case "loop":
		alternatives = append(alternatives, m.covers(cloneFlowState(s), step, "skipped"))
		frontier := []flowState{s}
		for repetition := 0; repetition < len(actions)+len(async)+1 && len(frontier) > 0; repetition++ {
			var following []flowState
			for _, prior := range frontier {
				for _, next := range m.sequence(m.children[step.Id], actions, async, cloneFlowState(prior), depth+1) {
					if next.index == prior.index && len(next.used) == len(prior.used) {
						continue
					}
					next = m.covers(next, step, "taken")
					alternatives = append(alternatives, next)
					following = append(following, next)
				}
			}
			frontier = following
		}
	}
	var out []flowState
	for _, next := range alternatives {
		out = append(out, m.sequence(rest, actions, async, next, depth+1)...)
		if len(out) > 4096 {
			m.overflow = true
			return out[:4096]
		}
	}
	return out
}
func (p *measuredProgram) designCoverage(check string, trace measuredTrace) ([]lang.Coverage, int, int, []diag.Diagnostic) {
	matcher := newFlowMatcher(p.graph)
	var coverage []lang.Coverage
	errorSet := map[string]bool{}
	var visit func([]*measuredFrame)
	visit = func(frames []*measuredFrame) {
		for _, frame := range frames {
			op := matcher.model(frame.node)
			if op != "" {
				_, _, declared := ucSignature(first(matcher.nodes[op].Attrs["signature"]))
				for _, want := range declared {
					for _, actual := range frame.errors {
						if matcher.errorMatches(actual, want) {
							errorSet[want] = true
						}
					}
				}
			}
			visit(frame.children)
		}
	}
	visit(trace.roots)
	for _, id := range slices.Sorted(maps.Keys(errorSet)) {
		node := matcher.nodes[id]
		coverage = append(coverage, lang.Coverage{Kind: "error", Node: id, Path: node.Origin.Path, Line: node.Origin.Line, Column: node.Origin.Column, EndLine: node.Origin.Line, EndColumn: node.Origin.Column})
	}
	errorTotalSet := map[string]bool{}
	for _, edge := range p.graph.Edges() {
		if edge.Kind == graph.EdgeKindRefers && edge.From == check {
			_, _, errors := ucSignature(first(matcher.nodes[edge.To].Attrs["signature"]))
			for _, id := range errors {
				errorTotalSet[id] = true
			}
		}
	}
	node := matcher.nodes[check]
	if node.Kind != graph.NodeKindFlow || len(node.Attrs["manual"]) > 0 {
		return coverage, 0, len(errorTotalSet), nil
	}
	branches := 0
	for id, step := range matcher.nodes {
		if step.Kind == graph.NodeKindStep && strings.HasPrefix(id, check+"#") {
			switch step.Form {
			case "alt":
				branches += len(step.Attrs["label"])
			case "opt", "loop", "on":
				branches += 2
			}
		}
	}
	if !trace.measured {
		return coverage, branches, len(errorTotalSet), []diag.Diagnostic{measuredDiagnostic(node, "lang.golang.trace-missing", "the automatic flow returned no observed business-call trace", "provide an instrumentable synchronous entry and explicit event publishing boundaries")}
	}
	actions, async := matcher.actions(trace.roots, 0)
	initial := flowState{used: map[*flowAction]bool{}, sent: map[string][]*flowAction{}, published: map[string]int{}}
	var collectAsync func([]*flowAction)
	collectAsync = func(actions []*flowAction) {
		for _, action := range actions {
			matcher.allAsync = append(matcher.allAsync, action.async...)
			collectAsync(action.children)
			collectAsync(action.async)
		}
	}
	matcher.allAsync = append(matcher.allAsync, async...)
	collectAsync(actions)
	collectAsync(async)
	var common map[string]lang.Coverage
	accepted := false
	for _, result := range matcher.sequence(matcher.children[check], actions, async, initial, 0) {
		if result.index != len(actions) {
			continue
		}
		complete := true
		for _, action := range matcher.allAsync {
			if !result.used[action] {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		points := map[string]lang.Coverage{}
		for _, point := range result.coverage {
			b, _ := json.Marshal(point)
			points[string(b)] = point
		}
		if !accepted {
			common = points
			accepted = true
		} else {
			for key := range common {
				if _, ok := points[key]; !ok {
					delete(common, key)
				}
			}
		}
	}
	if accepted {
		if !matcher.overflow {
			for _, key := range slices.Sorted(maps.Keys(common)) {
				coverage = append(coverage, common[key])
			}
		}
		return coverage, branches, len(errorTotalSet), nil
	}
	return coverage, branches, len(errorTotalSet), []diag.Diagnostic{measuredDiagnostic(node, "lang.golang.flow-mismatch", "the actual invocation, publication, error or asynchronous lineage does not match the flow steps", "make the implementation follow the declared flow or correct its step structure")}
}
func measuredDiagnostic(node graph.Node, code, message, fix string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Subject: node.Id, Position: node.Origin, Message: message, Fix: fix}
}

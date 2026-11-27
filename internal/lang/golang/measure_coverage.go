package golang

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/cover"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

func (p *measuredProgram) scope(check string) map[string]bool {
	selected := map[string]bool{}
	nodes := map[string]graph.Node{}
	children := map[string][]string{}
	for _, node := range p.graph.Nodes() {
		nodes[node.Id] = node
	}
	for _, edge := range p.graph.Edges() {
		if edge.Kind == graph.EdgeKindContains {
			children[edge.From] = append(children[edge.From], edge.To)
		}
	}
	var pending []string
	for _, edge := range p.graph.Edges() {
		if edge.Kind == graph.EdgeKindChecks && edge.To == check {
			pending = append(pending, edge.From)
		}
		if edge.Kind == graph.EdgeKindAbout || edge.Kind == graph.EdgeKindRefers {
			if edge.From == check && nodes[edge.To].Kind == graph.NodeKindOp {
				pending = append(pending, edge.To)
			}
		}
		if edge.Kind == graph.EdgeKindTraces && strings.HasPrefix(edge.From, check+"#") {
			pending = append(pending, edge.To)
		}
	}
	var calls func(string)
	calls = func(model string) {
		kind, args := ucSplitType(model)
		if kind == "call" {
			pending = append(pending, args...)
		} else {
			for _, child := range args {
				calls(child)
			}
		}
	}
	for _, symbol := range nodes[check].Attrs["symbols"] {
		_, model, _ := strings.Cut(strings.TrimSuffix(symbol, " generated"), ": ")
		calls(model)
	}
	seen := map[string]bool{}
	for len(pending) > 0 {
		id := pending[0]
		pending = pending[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if nodes[id].Layer == graph.LayerCode && nodes[id].Kind == graph.NodeKindOp {
			selected[id] = true
		}
		if nodes[id].Kind == graph.NodeKindType || nodes[id].Kind == graph.NodeKindContainer {
			pending = append(pending, children[id]...)
		}
		for _, edge := range p.graph.Edges() {
			if edge.From == id && (edge.Kind == graph.EdgeKindCalls || edge.Kind == graph.EdgeKindExercises) {
				pending = append(pending, edge.To)
			}
			if edge.To == id && (edge.Kind == graph.EdgeKindRealizes || edge.Kind == graph.EdgeKindObserves || edge.Kind == graph.EdgeKindGenerates || edge.Kind == graph.EdgeKindSubstitutes) {
				pending = append(pending, edge.From)
			}
		}
	}
	return selected
}
func profileCounts(path string) (map[string]int64, error) {
	profiles, err := cover.ParseProfiles(path)
	if err != nil {
		return nil, fmt.Errorf("read candidate coverage: %w", err)
	}
	out := map[string]int64{}
	for _, profile := range profiles {
		if profile.Mode != "atomic" {
			return nil, fmt.Errorf("candidate coverage mode is %s, not atomic", profile.Mode)
		}
		for _, block := range profile.Blocks {
			out[profileBlockKey(profile.FileName, block)] = int64(block.Count)
		}
	}
	return out, nil
}
func profileBlockKey(path string, block cover.ProfileBlock) string {
	return fmt.Sprintf("%s:%d.%d,%d.%d", path, block.StartLine, block.StartCol, block.EndLine, block.EndCol)
}
func (p *measuredProgram) coverage(check, beforePath, afterPath string) ([]lang.Coverage, int, error) {
	before, err := profileCounts(beforePath)
	if err != nil {
		return nil, 0, err
	}
	profiles, err := cover.ParseProfiles(afterPath)
	if err != nil {
		return nil, 0, err
	}
	scope := p.scope(check)
	mapped := false
	requireMapping := false
	for id := range scope {
		node := p.functions[id]
		if _, ok := p.sources[node.Origin.Path]; ok && !strings.HasSuffix(node.Origin.Path, "_test.go") {
			requireMapping = true
		}
	}
	eligible := map[string]bool{}
	covered := map[string]lang.Coverage{}
	for _, profile := range profiles {
		if profile.Mode != "atomic" {
			return nil, 0, fmt.Errorf("candidate coverage must use atomic counters")
		}
		relative := p.profileSources[filepath.ToSlash(profile.FileName)]
		if relative == "" {
			continue
		}
		mapped = true
		source := p.sources[relative]
		for _, block := range profile.Blocks {
			start, ok := measuredOffset(source.modified, block.StartLine, block.StartCol)
			if !ok {
				return nil, 0, fmt.Errorf("the candidate profile has an invalid source position")
			}
			end, ok := measuredOffset(source.modified, block.EndLine, block.EndCol)
			if !ok {
				return nil, 0, fmt.Errorf("the candidate profile has an invalid end position")
			}
			a, b, ok := source.originalRange(start, end)
			if !ok {
				continue
			}
			line, column := measuredPosition(source.original, a)
			endLine, endColumn := measuredPosition(source.original, b)
			owner := ""
			ownerLine := -1
			for id, node := range p.functions {
				if scope[id] && node.Origin.Path == relative && node.Origin.Line <= line && p.functionEnds[id] >= endLine && node.Origin.Line > ownerLine {
					owner = id
					ownerLine = node.Origin.Line
				}
			}
			if owner == "" {
				continue
			}
			point := lang.Coverage{Kind: "code", Node: owner, Path: relative, Line: line, Column: column, EndLine: endLine, EndColumn: endColumn}
			key := owner + ":" + strconv.Itoa(line) + "." + strconv.Itoa(column) + "," + strconv.Itoa(endLine) + "." + strconv.Itoa(endColumn)
			eligible[key] = true
			if int64(block.Count) > before[profileBlockKey(profile.FileName, block)] {
				covered[key] = point
			}
		}
	}
	if requireMapping && (len(profiles) == 0 || !mapped) {
		return nil, 0, fmt.Errorf("the candidate coverage has no original source mapping")
	}
	out := []lang.Coverage{}
	for _, key := range slices.Sorted(maps.Keys(covered)) {
		out = append(out, covered[key])
	}
	return out, len(eligible), nil
}

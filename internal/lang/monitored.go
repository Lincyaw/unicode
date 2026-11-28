package lang

import (
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// Monitored is the tested call of a single-call contract judged at runtime
// (docs/model.md §3.4): the design operation called, the symbol of the
// receiver, empty for an operation of a container, and the symbols of the
// arguments in the order of the parameters.
//
//uc:realizes lang.Monitored
type Monitored struct {
	Op       string   `json:"op"`
	Receiver string   `json:"receiver"`
	Args     []string `json:"args"`
}

// MonitoredCalls gives, for each single-call contract of g judged at runtime
// (docs/model.md §3.4), its tested call: a postcondition with a call clause
// that is not manual, whose receiver and arguments are distinct symbols and
// whose every symbol is one of them, the operation called being the one the
// call clause refers to.
//
//uc:realizes lang.monitoredCalls
func MonitoredCalls(g *graph.Graph) map[string]Monitored {
	called := map[string]string{}
	for _, edge := range g.Edges() {
		if edge.Kind == graph.EdgeKindRefers && slices.Contains(edge.Attrs["clauses"], "call#1") {
			if target := g.Node(edge.To); target != nil && target.Kind == graph.NodeKindOp {
				called[edge.From] = edge.To
			}
		}
	}
	out := map[string]Monitored{}
	for _, n := range g.Nodes() {
		if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindProperty || n.Form != "postcondition" || len(n.Attrs["manual"]) > 0 || len(n.Attrs["call"]) != 1 || called[n.Id] == "" {
			continue
		}
		if call, ok := monitoredCall(n.Attrs["call"][0], n.Attrs["symbols"]); ok {
			call.Op = called[n.Id]
			out[n.Id] = call
		}
	}
	return out
}

// monitoredCall reads a call clause whose receiver and arguments are distinct
// symbols covering every symbol, as recv.op(a, b) or op(a, b), the operation
// possibly qualified; any other clause gives false.
func monitoredCall(text string, symbols []string) (Monitored, bool) {
	declared := map[string]bool{}
	for _, symbol := range symbols {
		name, model, _ := strings.Cut(symbol, ": ")
		if strings.HasPrefix(model, "replies<") {
			return Monitored{}, false
		}
		declared[name] = true
	}
	open := strings.IndexByte(text, '(')
	if open < 0 || !strings.HasSuffix(text, ")") {
		return Monitored{}, false
	}
	out := Monitored{Args: []string{}}
	callee := text[:open]
	if head, rest, ok := strings.Cut(callee, "."); ok && declared[head] {
		if strings.Contains(rest, ".") {
			return Monitored{}, false
		}
		out.Receiver = head
	}
	used := map[string]bool{}
	if out.Receiver != "" {
		used[out.Receiver] = true
	}
	if inner := strings.TrimSpace(text[open+1 : len(text)-1]); inner != "" {
		for _, arg := range strings.Split(inner, ",") {
			arg = strings.TrimSpace(arg)
			if !declared[arg] || used[arg] {
				return Monitored{}, false
			}
			used[arg] = true
			out.Args = append(out.Args, arg)
		}
	}
	return out, len(used) == len(declared)
}

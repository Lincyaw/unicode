package check

import "github.com/lincyaw/unicode/internal/graph"

// substituteSample adds an independently valid installer to a conforming world.
func (w *world) substituteSample() (string, string) {
	if !w.external {
		w.external = true
		w.client = choose(w, w.comps)
		w.node("x", graph.LayerDesign, graph.NodeKindActor, "external", "design/system.uc", nil)
		w.declared(graph.EdgeKindContains, "sys", "x")
		w.declared(graph.EdgeKindUses, w.client.id, "x")
		w.client.uses = append(w.client.uses, "x")
		w.directOp(nil, "x.charge", w.client.pkg+".Charge", w.client.file, "charge")
	}
	installer := w.function(w.client.file, w.client.pkg+".InstallCharge", "installCharge", "(replacement: func(int) int) -> func()", "func")
	w.edge(graph.EdgeKindSubstitutes, installer, "x.charge", graph.SourceDeclared, map[string][]string{"nativeSignature": {"true"}})
	return installer, "x.charge"
}

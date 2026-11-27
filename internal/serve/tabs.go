package serve

import (
	"cmp"
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/view"
)

// tabs are where each view's tab leads from the node id (docs/design.md
// §7.5.1): the first of the node's related views of the kind; else the view
// of the default input its scopes give, else of the first input the view
// takes; else why there is none, which is undeclared-notation for a labelled
// view whose notation the system file does not declare and no-input for a
// view that takes no node. Every view given applies.
func (st *state) tabs(id string) []ViewTab {
	return st.tabsWith(id, st.relatedViews(id))
}

// tabsWith are the tabs of the node id whose related views are known.
func (st *state) tabsWith(id string, related []ViewRequest) []ViewTab {
	places := st.scopes(id)
	notations := st.nodes[st.system].Attrs["notations"]
	out := make([]ViewTab, 0, len(viewKinds))
	for _, kind := range viewKinds {
		tab := ViewTab{Kind: ViewKind(kind)}
		if i := slices.IndexFunc(related, func(r ViewRequest) bool { return r.Kind == ViewKind(kind) }); i >= 0 {
			tab.View = &related[i]
		} else if input := cmp.Or(st.defaultInput(kind, id, places), first(st.inputsOf(kind), func(string) bool { return true })); input != "" {
			tab.View = &ViewRequest{Kind: ViewKind(kind), Input: st.brief(input), Expand: []string{}}
		} else {
			closed := ClosedTabNoInput
			if (kind == view.ViewKindDdd || kind == view.ViewKindC4) && !slices.Contains(notations, string(kind)) {
				closed = ClosedTabUndeclaredNotation
			}
			tab.Closed = &closed
		}
		out = append(out, tab)
	}
	return out
}

// scopes are the places a default input is looked for, nearest first: the
// node, then the containers above it; a code element stands at the design
// nodes it realizes, and a node the design tree leaves out, such as a step,
// at its nearest ancestor in it. The system comes last.
func (st *state) scopes(id string) []string {
	starts := []string{id}
	if st.nodes[id].Layer == graph.LayerCode {
		starts = st.targets(id, graph.EdgeKindRealizes, graph.EdgeKindChecks, graph.EdgeKindGenerates, graph.EdgeKindObserves, graph.EdgeKindSubstitutes)
	}
	var places []string
	for _, s := range starts {
		chain := append(st.ancestors(s), s)
		slices.Reverse(chain)
		for _, p := range chain {
			if p != st.system && st.inTree(p) && !slices.Contains(places, p) {
				places = append(places, p)
			}
		}
	}
	return append(places, st.system)
}

// inTree tells whether the design tree holds a node: a design node other than a step or a case.
func (st *state) inTree(id string) bool {
	n, ok := st.nodes[id]
	return ok && n.Layer == graph.LayerDesign && n.Kind != graph.NodeKindStep && n.Kind != graph.NodeKindCase
}

// defaultInput is the input a view's tab takes when the node offers no view
// of the kind (docs/design.md §7.5.1), "" when the graph holds none.
func (st *state) defaultInput(kind view.ViewKind, id string, places []string) string {
	applies := func(input string) bool { return st.viewApplies(kind, input) }
	switch kind {
	case view.ViewKindArchitecture, view.ViewKindEvents, view.ViewKindDdd, view.ViewKindC4:
		return first([]string{st.system}, applies)
	case view.ViewKindClass:
		return first(places, func(p string) bool { return st.nodes[p].Kind == graph.NodeKindContainer && applies(p) })
	case view.ViewKindSequence, view.ViewKindActivity:
		return st.nearest(places, func(n string) bool { return st.nodes[n].Kind == graph.NodeKindFlow && applies(n) })
	case view.ViewKindCalls:
		return st.nearest(places, func(n string) bool { return st.nodes[n].Kind == graph.NodeKindOp && st.nodes[n].Salient && applies(n) })
	case view.ViewKindProperty:
		return first(places, func(p string) bool {
			holds := st.nearest([]string{p}, func(n string) bool { return st.nodes[n].Kind == graph.NodeKindProperty }) != ""
			return st.nodes[p].Kind == graph.NodeKindContainer && holds && applies(p)
		})
	case view.ViewKindCode:
		if file := st.codeFile(id); file != "" && applies(file) {
			return file
		}
		// A container's first owned file the view takes, a detail file being one it does not.
		owned := func(c string) string { return first(st.filesByCode(st.ownedFiles(c)), applies) }
		holder := first(places, func(p string) bool { return owned(p) != "" })
		if holder == "" {
			holder = st.nearest([]string{st.system}, func(n string) bool { return owned(n) != "" })
		}
		return owned(holder)
	}
	return ""
}

// nearest is the first node, in the design tree's order, under the first of
// the places that holds one that passes the test; "" when none does.
func (st *state) nearest(places []string, test func(string) bool) string {
	var walk func(string) string
	walk = func(n string) string {
		if test(n) {
			return n
		}
		for _, c := range st.treeChildren(n) {
			if found := walk(c); found != "" {
				return found
			}
		}
		return ""
	}
	for _, p := range places {
		if found := walk(p); found != "" {
			return found
		}
	}
	return ""
}

// codeFile is the file a node's code lies in: a file itself, the file a
// code element is declared in, the file of the first code element that
// realizes a design node, and the first file of a package or of a
// container's owned files in the order of filesByCode; "" when there is none.
func (st *state) codeFile(id string) string {
	n := st.nodes[id]
	switch {
	case n.Layer == graph.LayerCode && n.Kind == graph.NodeKindContainer && n.Form == "file":
		return id
	case n.Layer == graph.LayerCode && n.Kind == graph.NodeKindContainer:
		files := slices.DeleteFunc(st.children(id), func(c string) bool { return st.nodes[c].Form != "file" })
		return first(st.filesByCode(files), func(string) bool { return true })
	case n.Layer == graph.LayerCode:
		ancestors := st.ancestors(id)
		slices.Reverse(ancestors)
		return first(ancestors, func(a string) bool { return st.nodes[a].Form == "file" })
	}
	if r := st.realizations(id); len(r) > 0 {
		return st.codeFile(r[0].edge.From)
	}
	return first(st.filesByCode(st.ownedFiles(id)), func(string) bool { return true })
}

// ownedFiles are the code files a design container owns.
func (st *state) ownedFiles(id string) []string {
	return st.targets(id, graph.EdgeKindOwns)
}

// filesByCode orders code files as the code tab prefers them (docs/design.md
// §7.5.1): a file that is not a test file first, then the file with more
// code elements that realize a design node, then by path.
func (st *state) filesByCode(files []string) []string {
	type weight struct {
		test      bool
		realizing int
	}
	weights := map[string]weight{}
	for _, f := range files {
		w := weight{}
		for _, c := range st.within(f) {
			w.test = w.test || st.verifies(c)
			if len(st.targets(c, graph.EdgeKindRealizes)) > 0 {
				w.realizing++
			}
		}
		weights[f] = w
	}
	ordered := slices.Clone(files)
	slices.SortFunc(ordered, func(a, b string) int {
		wa, wb := weights[a], weights[b]
		return cmp.Or(compareTest(wa.test, wb.test), cmp.Compare(wb.realizing, wa.realizing), cmp.Compare(a, b))
	})
	return ordered
}

// verifies tells whether a code element belongs to the tests: a test
// function, or code bound by checks, generates, observes or substitutes.
func (st *state) verifies(id string) bool {
	if kind := st.attr(id, "testKind"); kind != "" && kind != "none" {
		return true
	}
	return len(st.targets(id, graph.EdgeKindChecks, graph.EdgeKindGenerates, graph.EdgeKindObserves, graph.EdgeKindSubstitutes)) > 0
}

// compareTest orders a file that is not a test file before one that is.
func compareTest(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	}
	return 1
}

// within are the nodes that id contains, at any depth.
func (st *state) within(id string) []string {
	var ids []string
	for _, c := range st.targets(id, graph.EdgeKindContains) {
		ids = append(ids, c)
		ids = append(ids, st.within(c)...)
	}
	return ids
}

// first is the first of the ids that passes the test, "" when none does.
func first(ids []string, test func(string) bool) string {
	if i := slices.IndexFunc(ids, test); i >= 0 {
		return ids[i]
	}
	return ""
}

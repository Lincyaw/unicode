package design

import (
	"fmt"
	"slices"
	"strings"

	"pgregory.net/rapid"
)

// This file draws black-box contracts: the names of their types, fields,
// operations and symbols, the interval of a limit, and which preconditions
// and ensures they hold in which order. The expected graph is assembled from
// the clauses drawn, not read from the graph the loader builds.

// contractNames are the names of one drawn contract.
type contractNames struct {
	component, account, rejected, snapshot, balance, owner, move string
	accountSymbol, amount, items, optionalItems, state, property string
}

func drawContractNames(t *rapid.T) contractNames {
	return contractNames{
		component:     fmt.Sprintf("domain%d", rapid.IntRange(0, 9999).Draw(t, "domain")),
		account:       rapid.SampledFrom([]string{"Account", "Wallet", "Ledger", "Purse"}).Draw(t, "account type"),
		rejected:      rapid.SampledFrom([]string{"Rejected", "Refused", "Denied"}).Draw(t, "error type"),
		snapshot:      rapid.SampledFrom([]string{"Snapshot", "Reading", "Balance"}).Draw(t, "snapshot type"),
		balance:       rapid.SampledFrom([]string{"balance", "funds", "total"}).Draw(t, "balance field"),
		owner:         rapid.SampledFrom([]string{"owner", "holder", "label"}).Draw(t, "owner field"),
		move:          rapid.SampledFrom([]string{"move", "shift", "take"}).Draw(t, "operation"),
		accountSymbol: rapid.SampledFrom([]string{"account", "wallet", "source"}).Draw(t, "account symbol"),
		amount:        rapid.SampledFrom([]string{"amount", "delta", "quantity"}).Draw(t, "amount symbol"),
		items:         rapid.SampledFrom([]string{"items", "entries", "parts"}).Draw(t, "items symbol"),
		optionalItems: rapid.SampledFrom([]string{"optionalItems", "maybeItems", "gaps"}).Draw(t, "optional items symbol"),
		state:         rapid.SampledFrom([]string{"state", "seen", "view"}).Draw(t, "observation"),
		property:      rapid.SampledFrom([]string{"moves-balance", "transfer-holds", "takes-funds"}).Draw(t, "property"),
	}
}

// clauseRef is a type or operation a clause refers to, with the field path
// it reads there, if any.
type clauseRef struct{ to, path string }

// clause is one line of a contract, such as `ensure …`, with what it refers to.
type clause struct {
	kind, text string
	refs       []clauseRef
}

// contract is a drawn contract: its names, the bounds of its limit, the
// preconditions and ensures it holds, and the identities of its nodes.
type contract struct {
	n                      contractNames
	low, high              int
	requires, ensures      []clause
	accountID, moveID      string
	rejectedID, snapshotID string
	propertyID, observeID  string
}

// drawContract draws the names and bounds of a contract and which of its
// preconditions and ensures it holds, in a drawn order. Each clause listed
// in must is held; the rest are each held half the time, and at least one
// ensure is.
func drawContract(t *rapid.T, mustRequire, mustEnsure []int) contract {
	n := drawContractNames(t)
	c := contract{n: n, low: rapid.IntRange(1, 100).Draw(t, "low")}
	c.high = rapid.IntRange(c.low, 1000).Draw(t, "high")
	c.accountID = n.component + "." + n.account
	c.moveID = c.accountID + "." + n.move
	c.rejectedID = n.component + "." + n.rejected
	c.snapshotID = n.component + "." + n.snapshot
	c.propertyID = n.component + "." + n.property
	c.observeID = c.propertyID + "." + n.state
	balance := n.accountSymbol + "." + n.balance
	observed := n.state + "." + n.balance
	requires := []clause{
		{"requires", n.amount + " <= " + balance, []clauseRef{{c.accountID, balance}}},
		{"requires", "all(item in " + n.optionalItems + ": item != none and item >= 0)", nil},
	}
	ensures := []clause{
		{"ensure", "not raised(" + n.rejected + ")", []clauseRef{{c.rejectedID, ""}}},
		{"ensure", "result == " + n.amount, nil},
		{"ensure", balance + " == old(" + balance + ") - " + n.amount, []clauseRef{{c.accountID, balance}}},
		{"ensure", observed + " == " + balance, []clauseRef{{c.snapshotID, observed}, {c.accountID, balance}}},
		{"ensure", "all(item in " + n.items + ": item >= 0) implies sum(item in " + n.items + ": item) >= 0", nil},
		{"ensure", "sum(item in " + n.optionalItems + ": item) >= 0", nil},
	}
	c.requires = pickClauses(t, "requires", requires, mustRequire, false)
	c.ensures = pickClauses(t, "ensures", ensures, mustEnsure, true)
	return c
}

// pickClauses holds the clauses at must and each other one half the time,
// at least one when nonEmpty, in a drawn order.
func pickClauses(t *rapid.T, label string, from []clause, must []int, nonEmpty bool) []clause {
	var held []clause
	for i, c := range from {
		if slices.Contains(must, i) || rapid.Bool().Draw(t, fmt.Sprintf("%s %d", label, i)) {
			held = append(held, c)
		}
	}
	if nonEmpty && len(held) == 0 {
		held = append(held, rapid.SampledFrom(from).Draw(t, label+" fallback"))
	}
	return rapid.Permutation(held).Draw(t, label+" order")
}

// lines are the lines of the component file, keyed so that a breakage can
// replace one: the key of a clause is its kind and its index in c.
func (c contract) lines() []struct{ key, text string } {
	n := c.n
	var out []struct{ key, text string }
	add := func(key, text string) { out = append(out, struct{ key, text string }{key, text}) }
	add("", fmt.Sprintf("component %s \"Contract fixture\" {\n  focus core\n  paths \"*.go\"\n}\n", n.component))
	add("", fmt.Sprintf("type %s object \"The account of a transfer\" {\n  field %s: int\n  field %s: string\n  op %s(amount: int) -> int raises %s\n}\n", n.account, n.balance, n.owner, n.move, n.rejected))
	add("", fmt.Sprintf("type %s error \"Rejected operation\" {\n  field amount: int\n}\n", n.rejected))
	add("", fmt.Sprintf("type %s record \"Observed account state\" {\n  field %s: int\n}\n", n.snapshot, n.balance))
	add("", fmt.Sprintf("property %s postcondition \"Transfer changes only the allowed state\" {", n.property))
	add("about", fmt.Sprintf("  about %s.%s", n.account, n.move))
	add("forall", fmt.Sprintf("  forall %s: %s", n.accountSymbol, n.account))
	add("", fmt.Sprintf("  forall %s: int", n.amount))
	add("items", fmt.Sprintf("  forall %s: list<int>", n.items))
	add("", fmt.Sprintf("  forall %s: list<optional<int>>", n.optionalItems))
	add("", fmt.Sprintf("  where %s in %d..%d", n.amount, c.low, c.high))
	add("", "  observe "+n.accountSymbol)
	add("observation", fmt.Sprintf("  observe %s: %s from %s", n.state, n.snapshot, n.accountSymbol))
	for i, r := range c.requires {
		add(fmt.Sprintf("requires %d", i), "  requires "+r.text)
	}
	add("changes", fmt.Sprintf("  changes %s.%s, %s.%s", n.accountSymbol, n.balance, n.state, n.balance))
	add("call", fmt.Sprintf("  call %s.%s(%s)", n.accountSymbol, n.move, n.amount))
	for i, e := range c.ensures {
		add(fmt.Sprintf("ensure %d", i), "  ensure "+e.text)
	}
	add("", "}")
	return out
}

// contractText joins the lines into the component file, a blank line between declarations.
func contractText(lines []struct{ key, text string }) string {
	var b strings.Builder
	for i, l := range lines {
		b.WriteString(l.text + "\n")
		if i < 4 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// files are the system file and the component file of the contract.
func (c contract) files(component string) map[string]string {
	return map[string]string{"design/system.uc": "system demo \"Contract fixture\"\n", "design/" + c.n.component + ".uc": component}
}

// contractFixture draws a valid contract with the nodes, edges and attributes
// its declarations and clauses give it.
func contractFixture(t *rapid.T) designFiles {
	c := drawContract(t, nil, nil)
	n := c.n
	balance := n.accountSymbol + "." + n.balance
	observed := n.state + "." + n.balance
	component := n.component
	// The clauses other than requires and ensure always refer to the same targets.
	fixed := []clause{
		{"observe", n.accountSymbol, []clauseRef{{c.accountID, ""}}},
		{"changes", balance + ", " + observed, []clauseRef{{c.accountID, balance}, {c.snapshotID, observed}}},
		{"call", n.accountSymbol + "." + n.move + "(" + n.amount + ")", []clauseRef{{c.moveID, ""}}},
	}
	clauses, paths := map[string][]string{}, map[string][]string{}
	note := func(kind string, index int, cl clause) {
		for _, r := range cl.refs {
			clauses[r.to] = append(clauses[r.to], fmt.Sprintf("%s#%d", kind, index))
			if r.path != "" {
				paths[r.to] = append(paths[r.to], r.path)
			}
		}
	}
	for _, cl := range fixed {
		note(cl.kind, 1, cl)
	}
	for i, cl := range c.requires {
		note("requires", i+1, cl)
	}
	for i, cl := range c.ensures {
		note("ensure", i+1, cl)
	}
	edge := func(kind, from, to string) string { return kind + " " + from + " " + to }
	nodes := []string{"demo", component, c.accountID, c.moveID, c.rejectedID, c.snapshotID, c.propertyID, c.observeID}
	edges := []string{
		edge("contains", "demo", component),
		edge("contains", component, c.accountID),
		edge("contains", component, c.rejectedID),
		edge("contains", component, c.snapshotID),
		edge("contains", component, c.propertyID),
		edge("contains", c.accountID, c.moveID),
		edge("contains", c.propertyID, c.observeID),
		edge("about", c.propertyID, c.moveID),
		edge("refers", c.moveID, c.rejectedID),
		edge("refers", c.observeID, c.snapshotID),
		// The symbol's type is referred to whether or not a clause reads it.
		edge("refers", c.propertyID, c.accountID),
	}
	edgeAttrs := map[string]map[string][]string{}
	for to, kinds := range clauses {
		key := edge("refers", c.propertyID, to)
		if !slices.Contains(edges, key) {
			edges = append(edges, key)
		}
		attrs := map[string][]string{"clauses": slices.Compact(slices.Sorted(slices.Values(kinds)))}
		if len(paths[to]) > 0 {
			attrs["paths"] = slices.Compact(slices.Sorted(slices.Values(paths[to])))
		}
		edgeAttrs[key] = attrs
	}
	property := map[string][]string{
		"symbols": {n.accountSymbol + ": " + c.accountID, n.amount + ": int", n.items + ": list<int>", n.optionalItems + ": list<optional<int>>"},
		"limits":  {fmt.Sprintf("%s in %d..%d", n.amount, c.low, c.high)},
		"observe": {n.accountSymbol, n.state + ": " + c.snapshotID + " from " + n.accountSymbol},
		"changes": {balance, observed},
		"call":    {n.accountSymbol + "." + n.move + "(" + n.amount + ")"},
	}
	for _, r := range c.requires {
		property["requires"] = append(property["requires"], r.text)
	}
	for _, e := range c.ensures {
		property["ensure"] = append(property["ensure"], e.text)
	}
	return designFiles{
		files: c.files(contractText(c.lines())), nodes: nodes, edges: edges,
		attrs: map[string]map[string][]string{
			c.propertyID: property,
			c.observeID:  {"type": {c.snapshotID}, "inputs": {n.accountSymbol}},
		},
		edgeAttrs: edgeAttrs,
		others:    []string{component, c.accountID, c.moveID, c.rejectedID, c.snapshotID},
	}
}

//uc:generates design.contract-graph-is-declared.sample
func genContractFixture() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample { return contractFixture(t).sample() })
}

// resultContractFixture draws a contract whose ensures read the result of
// the tested call: a field of it, a pure operation on it, or both, in a
// drawn order.
func resultContractFixture(t *rapid.T) designFiles {
	component := fmt.Sprintf("result%d", rapid.IntRange(0, 9999).Draw(t, "result domain"))
	item := rapid.SampledFrom([]string{"Item", "Token", "Entry"}).Draw(t, "result type")
	field := rapid.SampledFrom([]string{"value", "count", "weight"}).Draw(t, "result field")
	measure := rapid.SampledFrom([]string{"amount", "size", "measure"}).Draw(t, "pure operation")
	maker := rapid.SampledFrom([]string{"make", "build", "mint"}).Draw(t, "tested operation")
	symbol := rapid.SampledFrom([]string{"amount", "input", "seed"}).Draw(t, "symbol")
	name := rapid.SampledFrom([]string{"returned-value", "result-reads", "made-item"}).Draw(t, "property")
	itemID, measureID, makeID, property := component+"."+item, component+"."+item+"."+measure, component+"."+maker, component+"."+name
	ensures := pickClauses(t, "ensures", []clause{
		{"ensure", "result." + measure + "() == " + symbol, []clauseRef{{measureID, ""}}},
		{"ensure", "result." + field + " == " + symbol, []clauseRef{{itemID, "result." + field}}},
	}, nil, true)
	source := fmt.Sprintf("component %s \"Returned value fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\n"+
		"type %s object \"Returned item\" {\n  field %s: int\n  op %s() -> int pure\n}\n\n"+
		"op %s(value: int) -> %s\n\n"+
		"property %s postcondition \"Returned values support pure observations\" {\n  about %s\n  forall %s: int\n  call %s(%s)\n",
		component, item, field, measure, maker, item, name, maker, symbol, maker, symbol)
	edge := func(kind, from, to string) string { return kind + " " + from + " " + to }
	edges := []string{edge("contains", "demo", component), edge("contains", component, itemID), edge("contains", itemID, measureID),
		edge("contains", component, makeID), edge("contains", component, property), edge("refers", makeID, itemID),
		edge("about", property, makeID), edge("refers", property, makeID)}
	edgeAttrs := map[string]map[string][]string{edge("refers", property, makeID): {"clauses": {"call#1"}}}
	ensureTexts := []string{}
	for i, e := range ensures {
		source += "  ensure " + e.text + "\n"
		ensureTexts = append(ensureTexts, e.text)
		r := e.refs[0]
		key := edge("refers", property, r.to)
		edges = append(edges, key)
		edgeAttrs[key] = map[string][]string{"clauses": {fmt.Sprintf("ensure#%d", i+1)}}
		if r.path != "" {
			edgeAttrs[key]["paths"] = []string{r.path}
		}
	}
	source += "}\n"
	return designFiles{
		files:     map[string]string{"design/system.uc": "system demo \"Result fixture\"\n", "design/" + component + ".uc": source},
		nodes:     []string{"demo", component, itemID, measureID, makeID, property},
		edges:     edges,
		attrs:     map[string]map[string][]string{property: {"symbols": {symbol + ": int"}, "call": {maker + "(" + symbol + ")"}, "ensure": ensureTexts}},
		edgeAttrs: edgeAttrs,
		others:    []string{component, itemID, measureID, makeID},
	}
}

//uc:generates design.returned-value-contract-is-declared.sample
func genResultContractFixture() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample { return resultContractFixture(t).sample() })
}

// relationBreakages are the changes that break one rule of a contract's
// relations: the key of the line changed, its replacement, the code, and
// the key of the line the diagnostic stands on when that is another line.
func relationBreakages(c contract) []struct{ key, replacement, code, at string } {
	n := c.n
	return []struct{ key, replacement, code, at string }{
		{"requires 0", "  requires " + n.amount + " + 1", "design.expression-type", ""},
		{"requires 0", "  requires result == " + n.amount, "design.expression-type", ""},
		{"changes", "  changes " + n.amount, "design.invalid-change-path", ""},
		{"observation", fmt.Sprintf("  observe %s: %s from missingInput", n.state, n.snapshot), "design.invalid-observation", ""},
		{"observation", fmt.Sprintf("  observe %s: %s from %s, %s", n.state, n.snapshot, n.accountSymbol, n.accountSymbol), "design.invalid-observation", ""},
		{"observation", fmt.Sprintf("  observe %s: call<%s> from %s", n.state, n.account, n.accountSymbol), "design.invalid-observation", ""},
		{"about", "  about " + n.account, "design.call-not-about", "call"},
		{"items", fmt.Sprintf("  forall %s: list<int>\n  forall steps: list<call<%s>>", n.items, n.account), "design.invalid-call-sequence", "call"},
		{"call", fmt.Sprintf("  call %s.%s(\"wrong\")", n.accountSymbol, n.move), "design.expression-type", ""},
		{"ensure 0", fmt.Sprintf("  ensure %s.%s(%s) == %s", n.accountSymbol, n.move, n.amount, n.amount), "design.expression-type", ""},
		{"ensure 0", fmt.Sprintf("  ensure %s.%s[%s] == %s", n.accountSymbol, n.balance, n.amount, n.amount), "design.expression-type", ""},
		{"ensure 0", "  ensure old(result) == " + n.amount, "design.expression-type", ""},
		{"ensure 0", fmt.Sprintf("  ensure all(%s in %s: %s > 0)", n.amount, n.items, n.amount), "design.expression-type", ""},
	}
}

// brokenContractFixture draws a contract that holds the first precondition
// and the ensure `result == amount`, and breaks one of its relations.
func brokenContractFixture(t *rapid.T) brokenFiles {
	c := drawContract(t, []int{0}, []int{1})
	// The held clauses come in a drawn order; the breakages change the first
	// precondition and the ensure `result == amount`, so these go first.
	c.requires = moveFirst(c.requires, c.n.amount+" <= "+c.n.accountSymbol+"."+c.n.balance)
	c.ensures = moveFirst(c.ensures, "result == "+c.n.amount)
	lines := c.lines()
	b := rapid.SampledFrom(relationBreakages(c)).Draw(t, "invalid relation")
	at := b.at
	if at == "" {
		at = b.key
	}
	for i := range lines {
		if lines[i].key == b.key {
			lines[i].text = b.replacement
		}
	}
	line := 0
	for i := range lines {
		if lines[i].key == at {
			line = strings.Count(contractText(lines[:i]), "\n") + 1
		}
	}
	path := "design/" + c.n.component + ".uc"
	return brokenFiles{files: c.files(contractText(lines)), codes: []string{b.code}, path: path, line: line}
}

// moveFirst moves the clause with the given text to the front.
func moveFirst(cs []clause, text string) []clause {
	i := slices.IndexFunc(cs, func(c clause) bool { return c.text == text })
	return append([]clause{cs[i]}, slices.Delete(slices.Clone(cs), i, i+1)...)
}

//uc:generates design.contract-rejects-invalid-relations.broken
func genBrokenContract() *rapid.Generator[Broken] {
	return rapid.Custom(func(t *rapid.T) Broken { return brokenContractFixture(t).broken() })
}

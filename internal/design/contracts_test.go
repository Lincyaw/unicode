package design

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// contractFixture describes a complete contract independently of the loader.
// Names and numeric bounds vary, while the expected model is assembled from
// the declarations rather than inferred from the produced graph.
func contractFixture(t *rapid.T) Sample {
	component := fmt.Sprintf("domain%d", rapid.IntRange(0, 9999).Draw(t, "domain"))
	low := rapid.IntRange(1, 100).Draw(t, "low")
	high := rapid.IntRange(low, 1000).Draw(t, "high")
	property := component + ".moves-balance"
	source := fmt.Sprintf(`component %s "Contract fixture" {
  focus core
  paths "*.go"
}

type Account object "Account" {
  field balance: int
  field owner: string
  op move(amount: int) -> int raises Rejected
}

type Rejected error "Rejected operation" {
  field amount: int
}

type Snapshot record "Observed account state" {
  field balance: int
}

property moves-balance postcondition "Transfer changes only the allowed state" {
  about Account.move
  forall account: Account
  forall amount: int
  forall items: list<int>
  forall optionalItems: list<optional<int>>
  where amount in %d..%d
  observe account
  observe state: Snapshot from account
  requires amount <= account.balance
  requires all(item in optionalItems: item != none and item >= 0)
  changes account.balance, state.balance
  call account.move(amount)
  ensure not raised(Rejected)
  ensure result == amount
  ensure account.balance == old(account.balance) - amount
  ensure state.balance == account.balance
  ensure all(item in items: item >= 0) implies sum(item in items: item) >= 0
  ensure sum(item in optionalItems: item) >= 0
}
`, component, low, high)
	nodes := []string{"demo", component, component + ".Account", component + ".Account.move", component + ".Rejected", component + ".Snapshot", property, property + ".state"}
	edge := func(kind, from, to string) string { return kind + " " + from + " " + to }
	edges := []string{
		edge("contains", "demo", component),
		edge("contains", component, component+".Account"),
		edge("contains", component, component+".Rejected"),
		edge("contains", component, component+".Snapshot"),
		edge("contains", component, property),
		edge("contains", component+".Account", component+".Account.move"),
		edge("contains", property, property+".state"),
		edge("about", property, component+".Account.move"),
		edge("refers", component+".Account.move", component+".Rejected"),
		edge("refers", property, component+".Account"),
		edge("refers", property, component+".Account.move"),
		edge("refers", property, component+".Rejected"),
		edge("refers", property, component+".Snapshot"),
		edge("refers", property+".state", component+".Snapshot"),
	}
	return Sample{Files: map[string]string{"design/system.uc": "system demo \"Contract fixture\"\n", "design/" + component + ".uc": source}, Nodes: nodes, Edges: edges, Attrs: map[string]map[string][]string{
		property: {
			"symbols":  {"account: " + component + ".Account", "amount: int", "items: list<int>", "optionalItems: list<optional<int>>"},
			"limits":   {fmt.Sprintf("amount in %d..%d", low, high)},
			"observe":  {"account", "state: " + component + ".Snapshot from account"},
			"requires": {"amount <= account.balance", "all(item in optionalItems: item != none and item >= 0)"},
			"changes":  {"account.balance", "state.balance"},
			"call":     {"account.move(amount)"},
			"ensure":   {"not raised(Rejected)", "result == amount", "account.balance == old(account.balance) - amount", "state.balance == account.balance", "all(item in items: item >= 0) implies sum(item in items: item) >= 0", "sum(item in optionalItems: item) >= 0"},
		},
		property + ".state": {"type": {component + ".Snapshot"}, "inputs": {"account"}},
	}, EdgeAttrs: map[string]map[string][]string{
		edge("refers", property, component+".Account"):      {"clauses": {"changes#1", "ensure#3", "ensure#4", "observe#1", "requires#1"}, "paths": {"account.balance"}},
		edge("refers", property, component+".Snapshot"):     {"clauses": {"changes#1", "ensure#4"}, "paths": {"state.balance"}},
		edge("refers", property, component+".Account.move"): {"clauses": {"call#1"}},
		edge("refers", property, component+".Rejected"):     {"clauses": {"ensure#1"}},
	}}
}

func TestContractGraphIsDeclared(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkAcceptsValidDesign(t, contractFixture(t)) })
}

func resultContractFixture(t *rapid.T) Sample {
	component := fmt.Sprintf("result%d", rapid.IntRange(0, 9999).Draw(t, "result domain"))
	property := component + ".returned-value"
	source := fmt.Sprintf(`component %s "Returned value fixture" {
  focus core
  paths "*.go"
}

type Item object "Returned item" {
  field value: int
  op amount() -> int pure
}

op make(value: int) -> Item

property returned-value postcondition "Returned values support pure observations" {
  about make
  forall amount: int
  call make(amount)
  ensure result.amount() == amount
  ensure result.value == amount
}
`, component)
	edge := func(kind, from, to string) string { return kind + " " + from + " " + to }
	return Sample{Files: map[string]string{"design/system.uc": "system demo \"Result fixture\"\n", "design/" + component + ".uc": source},
		Nodes:     []string{"demo", component, component + ".Item", component + ".Item.amount", component + ".make", property},
		Edges:     []string{edge("contains", "demo", component), edge("contains", component, component+".Item"), edge("contains", component+".Item", component+".Item.amount"), edge("contains", component, component+".make"), edge("contains", component, property), edge("refers", component+".make", component+".Item"), edge("about", property, component+".make"), edge("refers", property, component+".make"), edge("refers", property, component+".Item.amount"), edge("refers", property, component+".Item")},
		Attrs:     map[string]map[string][]string{property: {"symbols": {"amount: int"}, "call": {"make(amount)"}, "ensure": {"result.amount() == amount", "result.value == amount"}}},
		EdgeAttrs: map[string]map[string][]string{edge("refers", property, component+".make"): {"clauses": {"call#1"}}, edge("refers", property, component+".Item.amount"): {"clauses": {"ensure#1"}}, edge("refers", property, component+".Item"): {"clauses": {"ensure#2"}, "paths": {"result.value"}}},
	}
}

func TestReturnedValueContractIsDeclared(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkAcceptsValidDesign(t, resultContractFixture(t)) })
}

func TestContractRejectsInvalidRelations(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkRejectsBrokenDesign(t, brokenContractFixture(t))
	})
}

func brokenContractFixture(t *rapid.T) Broken {
	sample := contractFixture(t)
	var path string
	for p := range sample.Files {
		if p != "design/system.uc" {
			path = p
		}
	}
	variants := []struct{ old, replacement, code string }{
		{"requires amount <= account.balance", "requires amount + 1", "design.expression-type"},
		{"requires amount <= account.balance", "requires result == amount", "design.expression-type"},
		{"changes account.balance, state.balance", "changes amount", "design.invalid-change-path"},
		{"observe state: Snapshot from account", "observe state: Snapshot from missing", "design.invalid-observation"},
		{"observe state: Snapshot from account", "observe state: Snapshot from account, account", "design.invalid-observation"},
		{"observe state: Snapshot from account", "observe state: call<Account> from account", "design.invalid-observation"},
		{"about Account.move", "about Account", "design.call-not-about"},
		{"forall items: list<int>", "forall items: list<int>\n  forall steps: list<call<Account>>", "design.invalid-call-sequence"},
		{"call account.move(amount)", "call account.move(\"wrong\")", "design.expression-type"},
		{"ensure result == amount", "ensure account.move(amount) == amount", "design.expression-type"},
		{"ensure result == amount", "ensure account.balance[amount] == amount", "design.expression-type"},
		{"ensure result == amount", "ensure old(result) == amount", "design.expression-type"},
		{"ensure result == amount", "ensure all(amount in items: amount > 0)", "design.expression-type"},
	}
	variant := rapid.SampledFrom(variants).Draw(t, "invalid relation")
	source := sample.Files[path]
	at := strings.Index(source, variant.old)
	line := strings.Count(source[:at], "\n") + 1
	sample.Files[path] = strings.Replace(source, variant.old, variant.replacement, 1)
	if variant.code == "design.call-not-about" || variant.code == "design.invalid-call-sequence" {
		line = strings.Count(sample.Files[path][:strings.Index(sample.Files[path], "call account.move")], "\n") + 1
	}
	return Broken{Files: sample.Files, Path: path, Line: line, Code: variant.code}
}

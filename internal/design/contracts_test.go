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
  where amount in %d..%d
  observe account
  observe state: Snapshot from account
  requires amount <= account.balance
  changes account.balance, state.balance
  call account.move(amount)
  ensure not raised(Rejected)
  ensure result == amount
  ensure account.balance == old(account.balance) - amount
  ensure state.balance == account.balance
  ensure all(item in items: item >= 0) implies sum(item in items: item) >= 0
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
			"symbols":  {"account: " + component + ".Account", "amount: int", "items: list<int>"},
			"limits":   {fmt.Sprintf("amount in %d..%d", low, high)},
			"observe":  {"account", "state: " + component + ".Snapshot from account"},
			"requires": {"amount <= account.balance"},
			"changes":  {"account.balance", "state.balance"},
			"call":     {"account.move(amount)"},
			"ensure":   {"not raised(Rejected)", "result == amount", "account.balance == old(account.balance) - amount", "state.balance == account.balance", "all(item in items: item >= 0) implies sum(item in items: item) >= 0"},
		},
		property + ".state": {"type": {component + ".Snapshot"}, "inputs": {"account"}},
	}}
}

func TestContractGraphIsDeclared(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkAcceptsValidDesign(t, contractFixture(t)) })
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
	return Broken{Files: sample.Files, Path: path, Line: line, Code: variant.code}
}

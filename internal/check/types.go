package check

import (
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// language is what check knows of how a backend writes signatures
// (docs/design.md §4, §5.2): the type of the test handle that a predicate
// takes before its symbols, and whether an operation's errors are left out
// of its signature instead of being written ` raises error`.
type language struct {
	handle   string
	noRaises bool
}

// languages are keyed by the prefix of their code identities (docs/model.md §7).
var languages = map[string]language{
	"go:": {handle: "go:pgregory.net/rapid.TB"},
	"py:": {noRaises: true},
}

func languageOf(id string) (string, language) {
	for prefix, l := range languages {
		if strings.HasPrefix(id, prefix) {
			return prefix, l
		}
	}
	return "", language{}
}

// typeExpr is a type in the design's notation: a name, like int or
// order.Order, with the arguments of list, set, optional, map or call.
type typeExpr struct {
	name string
	args []typeExpr
}

// parseType reads a type like map<string, list<order.Order>>. It fails on
// anything else, like a Go result list (A, B).
func parseType(s string) (typeExpr, bool) {
	t, rest, ok := parseTypeAt(s)
	return t, ok && rest == ""
}

func parseTypeAt(s string) (typeExpr, string, bool) {
	end, depth := len(s), 0
scan:
	for i := range len(s) {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case '<', ',', '>', '(', ')', ' ':
			if depth == 0 {
				end = i
				break scan
			}
		}
	}
	t, rest := typeExpr{name: s[:end]}, s[end:]
	if t.name == "" {
		return t, rest, false
	}
	if !strings.HasPrefix(rest, "<") {
		return t, rest, true
	}
	rest = rest[1:]
	for {
		arg, r, ok := parseTypeAt(rest)
		if !ok {
			return t, r, false
		}
		t.args = append(t.args, arg)
		switch {
		case strings.HasPrefix(r, ", "):
			rest = r[2:]
		case strings.HasPrefix(r, ">"):
			return t, r[1:], true
		default:
			return t, r, false
		}
	}
}

// sameType tells whether a type written by the code matches the design's,
// by the nodes of the checked graph.
func (c *checker) sameType(design, code string) bool {
	return sameType(func(id string) (graph.Node, bool) { n, ok := c.byID[id]; return n, ok }, design, code)
}

// sameType tells whether a type written by the code matches the design's:
// the same type, except that optional<X> in the code, a Go pointer, also
// matches X when X is an object type of the design, as node tells.
func sameType(node func(string) (graph.Node, bool), design, code string) bool {
	d, ok := parseType(design)
	k, ok2 := parseType(code)
	if !ok || !ok2 {
		return design == code
	}
	return typeMatches(node, d, k)
}

func typeMatches(node func(string) (graph.Node, bool), design, code typeExpr) bool {
	if code.name == "optional" && len(code.args) == 1 && len(code.args[0].args) == 0 && len(design.args) == 0 &&
		code.args[0].name == design.name {
		if n, ok := node(design.name); ok && n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindType && n.Form == "object" {
			return true
		}
	}
	if design.name != code.name || len(design.args) != len(code.args) {
		return false
	}
	for i := range design.args {
		if !typeMatches(node, design.args[i], code.args[i]) {
			return false
		}
	}
	return true
}

// signature is an operation's signature `(a: T, …) -> R raises E, …` taken apart.
type signature struct {
	params []string
	result string
	raises string
}

func parseSignature(s string) (signature, bool) {
	params, rest, ok := splitParameters(s)
	if !ok {
		return signature{}, false
	}
	rest, raises, _ := strings.Cut(rest, " raises ")
	result, arrow := strings.CutPrefix(rest, " -> ")
	if !arrow && rest != "" {
		return signature{}, false
	}
	return signature{params, result, raises}, true
}

// parameters lists the parameters of a signature, each `name: type`.
func parameters(s string) []string {
	params, _, _ := splitParameters(s)
	return params
}

// splitParameters splits `(a: T, b: map<K, V>) -> R` into its parameters and
// what follows the closing parenthesis.
func splitParameters(s string) ([]string, string, bool) {
	if !strings.HasPrefix(s, "(") {
		return nil, "", false
	}
	var params []string
	depth, start := 0, 1
	for i := 1; i < len(s); i++ {
		switch ch := s[i]; {
		case ch == '<' || ch == '(':
			depth++
		case depth > 0:
			if ch == '>' || ch == ')' {
				depth--
			}
		case ch == ',' || ch == ')':
			if p := strings.TrimSpace(s[start:i]); p != "" {
				params = append(params, p)
			}
			if ch == ')' {
				return params, s[i+1:], true
			}
			start = i + 1
		}
	}
	return params, "", false
}

// sameSignature tells whether the signature that the code function fn has
// matches the design's (rule 18): the parameters' types in order and the
// result by sameType, whatever the parameters are named; for a language
// that writes errors, ` raises error` exactly when the design raises any. A
// result list like (A, B) matches nothing.
func (c *checker) sameSignature(design, code, fn string) bool {
	d, ok := parseSignature(design)
	k, ok2 := parseSignature(code)
	if !ok || !ok2 {
		return design == code
	}
	if !c.sameTypes(typesOf(d.params), typesOf(k.params)) {
		return false
	}
	if (d.result == "") != (k.result == "") || d.result != "" && !c.sameType(d.result, k.result) {
		return false
	}
	if _, l := languageOf(fn); l.noRaises {
		return true
	}
	return d.raises == "" && k.raises == "" || d.raises != "" && k.raises == "error"
}

// sameParams tells whether the code's parameters match the design's, in
// order, by name and by sameType, as a predicate's match its symbols (rule 20).
func (c *checker) sameParams(design, code []string) bool {
	return slices.Equal(namesOf(design), namesOf(code)) && c.sameTypes(typesOf(design), typesOf(code))
}

// sameTypes tells whether the code's types match the design's, in order, by sameType.
func (c *checker) sameTypes(design, code []string) bool {
	return slices.EqualFunc(design, code, c.sameType)
}

// namesOf and typesOf split parameters, each `name: type`, into their names and their types.
func namesOf(params []string) []string {
	var out []string
	for _, p := range params {
		n, _, _ := strings.Cut(p, ": ")
		out = append(out, n)
	}
	return out
}

func typesOf(params []string) []string {
	var out []string
	for _, p := range params {
		_, t, _ := strings.Cut(p, ": ")
		out = append(out, t)
	}
	return out
}

package syntax

import (
	"slices"
	"strings"

	"pgregory.net/rapid"
)

// exprTree is a drawn expression. An atom's text is its written form; a
// binary expression's is its operator, a field's its name, a call's its
// operation and a quantified expression's its quantifier and variable.
type exprTree struct {
	kind     string
	text     string
	variable string
	children []*exprTree
}

var binaryOperators = []string{"+", "-", "*", "/", "==", "!=", "<", "<=", ">", ">=", "and", "or", "implies", "in"}

// genExprTree draws an expression of at most depth levels below the top:
// literals, names, every binary operator, not, field and index access, calls
// of two arguments, quantified expressions, old, raised and failure. Above
// the last level four in five subexpressions are compound, a third of those
// binary, so that operators nest in each other; forms and operators are
// drawn with equal chances, since rapid's integers favour the smallest and
// the largest.
func genExprTree(t *rapid.T, depth int) *exprTree {
	choice := uniform(t, 4)
	if depth > 0 && uniform(t, 5) > 0 {
		choice = []int{4, 4, 4, 5, 6, 7, 8, 9, 10}[uniform(t, 9)]
	}
	child := func() *exprTree { return genExprTree(t, depth-1) }
	switch choice {
	case 0:
		return &exprTree{kind: "atom", text: string(genNumber(t, false))}
	case 1:
		return &exprTree{kind: "atom", text: quote(string(genString(t, "literal")))}
	case 2:
		return &exprTree{kind: "atom", text: string(genLower(t, "reference"))}
	case 3:
		return &exprTree{kind: "atom", text: rapid.SampledFrom([]string{"true", "false", "none", "result"}).Draw(t, "literal")}
	case 4:
		return &exprTree{kind: "binary", text: binaryOperators[uniform(t, len(binaryOperators))], children: []*exprTree{child(), child()}}
	case 5:
		return &exprTree{kind: "not", children: []*exprTree{child()}}
	case 6:
		return &exprTree{kind: "field", text: string(genLower(t, "field")), children: []*exprTree{child()}}
	case 7:
		return &exprTree{kind: "index", children: []*exprTree{child(), child()}}
	case 8:
		return &exprTree{kind: "call", text: string(genLower(t, "operation")), children: []*exprTree{child(), child()}}
	case 9:
		quantifier := rapid.SampledFrom([]string{"all", "any", "count", "sum"}).Draw(t, "quantifier")
		return &exprTree{kind: "each", text: quantifier, variable: string(genLower(t, "variable")), children: []*exprTree{child(), child()}}
	}
	if rapid.Bool().Draw(t, "snapshot") {
		return &exprTree{kind: "old", children: []*exprTree{child()}}
	}
	kind := rapid.SampledFrom([]string{"raised", "failure"}).Draw(t, "error")
	return &exprTree{kind: "atom", text: kind + "(" + strings.Join(genTypeRef(t).Segments, ".") + ")"}
}

// bracketed writes e with brackets around every compound subexpression.
func bracketed(e *exprTree) string {
	c := e.children
	switch e.kind {
	case "binary":
		return "(" + bracketed(c[0]) + " " + e.text + " " + bracketed(c[1]) + ")"
	case "not":
		return "(not (" + bracketed(c[0]) + "))"
	case "field":
		return "(" + bracketed(c[0]) + ")." + e.text
	case "index":
		return "(" + bracketed(c[0]) + ")[" + bracketed(c[1]) + "]"
	case "call":
		return e.text + "(" + bracketed(c[0]) + ", " + bracketed(c[1]) + ")"
	case "each":
		return e.text + "(" + e.variable + " in " + bracketed(c[0]) + ": " + bracketed(c[1]) + ")"
	case "old":
		return "old(" + bracketed(c[0]) + ")"
	}
	return e.text
}

// genExprText draws an expression tree and writes it fully bracketed.
//
//uc:generates syntax.expression-format-is-stable.text
func genExprText() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string { return bracketed(genExprTree(t, 3)) })
}

// newBroken is a broken file of kind at a path drawn for its kind.
func newBroken(t *rapid.T, kind FileKind, text string, line, column int, code string) Broken {
	return Broken{Kind: kind, Path: genPathOf(t, kind), Text: []byte(text), Line: line, Column: column, Code: code}
}

// contractHead is the component's header that every drawn contract follows;
// the property starts on line len(contractHead)+1.
func contractHead(component string) []string {
	return []string{"component " + component + " \"Contract syntax\" {", "  focus core", "  paths \"*.go\"", "}", ""}
}

// comparisons are the operators a drawn ensure compares two operands with.
var comparisons = []string{"==", "!=", "<", "<=", ">", ">="}

// brokenContractSyntax draws a component with one black-box contract, a
// property of random name whose about and call name a random operation of a
// random type, with one to three symbols of builtin types, the first passed
// to the call, and one to three ensures each comparing two of result, the
// symbols and small numbers; it then makes one of ten edits at a random place
// that fits it, or one edit of brokenObservationSyntax, and gives the line,
// column and code by construction.
func brokenContractSyntax(t *rapid.T) Broken {
	variant := uniform(t, 11)
	if variant == 10 {
		return brokenObservationSyntax(t)
	}
	head := contractHead(kebabNames.Draw(t, "component"))
	name := kebabNames.Draw(t, "property")
	operation := string(genUpper(t, "type")) + "." + string(genLower(t, "operation"))
	var symbols []string
	for range rapid.IntRange(1, 3).Draw(t, "symbols") {
		symbols = append(symbols, rapid.Custom(func(t *rapid.T) string { return string(genLower(t, "symbol")) }).
			Filter(func(s string) bool { return !slices.Contains(symbols, s) }).Draw(t, "symbol"))
	}
	operand := rapid.OneOf(rapid.SampledFrom(append([]string{"result"}, symbols...)), rapid.StringMatching(`0|[1-9][0-9]{0,2}`))
	var ensures []string
	for range rapid.IntRange(1, 3).Draw(t, "ensures") {
		ensures = append(ensures, operand.Draw(t, "left")+" "+rapid.SampledFrom(comparisons).Draw(t, "comparison")+" "+operand.Draw(t, "right"))
	}
	header := "property " + name + " postcondition \"Contract claim\" {"
	lines := []string{"  about " + operation}
	for _, s := range symbols {
		lines = append(lines, "  forall "+s+": "+rapid.SampledFrom([]string{"int", "float", "string", "bool"}).Draw(t, "type"))
	}
	call := len(lines)
	lines = append(lines, "  call "+operation+"("+symbols[0]+")")
	first := len(lines)
	for _, e := range ensures {
		lines = append(lines, "  ensure "+e)
	}
	// at is the line and column of the property's header, or of a body line and a byte offset in it.
	at, column := -1, 1
	code := ""
	switch variant {
	case 0:
		lines = lines[:first]
		code = "syntax.missing-claim"
	case 1:
		lines = append(lines, "  manual "+label(t))
		code = "syntax.mixed-claim"
	case 2:
		lines = slices.Delete(lines, call, call+1)
		code = "syntax.invalid-tested-call"
	case 3:
		lines = slices.Insert(lines, call, "  changes none")
		code = "syntax.invalid-observation-scope"
	case 4:
		at = rapid.IntRange(1, len(symbols)).Draw(t, "renamed symbol")
		lines[at] = "  forall " + rapid.SampledFrom(expressionWords).Draw(t, "keyword") + lines[at][strings.Index(lines[at], ":"):]
		column = 3
		code = "syntax.expression-keyword-name"
	case 5:
		at = first + rapid.IntRange(0, len(ensures)-1).Draw(t, "unspaced ensure")
		parts := strings.SplitN(lines[at], " ", 6)
		lines[at] = strings.Join(parts[:4], " ") + parts[4] + parts[5]
		column = len(strings.Join(parts[:4], " ")) + 1
		code = "syntax.expression-spacing"
	case 6:
		lines = slices.Insert(lines, first, "  within \""+words.Draw(t, "duration")+"\"")
		at, column = first, 3
		code = "syntax.invalid-time-budget"
	case 7:
		header = "flow " + name + " \"Flow claim\" {"
		lines = []string{"  alt " + label(t) + " {", "  }", "  ensure true"}
		at, column = 0, 3
		code = "syntax.invalid-flow-call"
	case 8:
		header = "flow " + name + " \"Empty checked flow\" {"
		lines = lines[first:]
		code = "syntax.invalid-flow-call"
	case 9:
		symbol := symbols[0]
		lines = slices.Insert(lines, call, "  observe "+symbol, "  changes "+symbol+" + 1")
		at, column = call+1, len("  changes "+symbol+" ")+1
		code = "syntax.unexpected-token"
	}
	text := strings.Join(head, "\n") + "\n" + header + "\n" + strings.Join(lines, "\n") + "\n}\n"
	line := len(head) + 1
	if at >= 0 {
		line += 1 + at
	}
	return newBroken(t, FileKindComponent, text, line, column, code)
}

//uc:generates syntax.contract-syntax-rejects-invalid-claims.broken
func genBrokenContractSyntax() *rapid.Generator[Broken] {
	return rapid.Custom(brokenContractSyntax)
}

func brokenObservationSyntax(t *rapid.T) Broken {
	paths := make([]string, rapid.IntRange(1, 4).Draw(t, "observed paths"))
	badPath := rapid.IntRange(0, len(paths)-1).Draw(t, "malformed path")
	badName := string(genUpper(t, "uppercase segment"))
	if rapid.Bool().Draw(t, "kebab segment") {
		badName = words.Draw(t, "segment prefix") + "-" + words.Draw(t, "segment suffix")
	}
	var badOffset int
	for i := range paths {
		segments := make([]string, rapid.IntRange(1, 5).Draw(t, "path segments"))
		for j := range segments {
			segments[j] = string(genLower(t, "path segment"))
		}
		if i == badPath {
			badSegment := rapid.IntRange(0, len(segments)-1).Draw(t, "malformed segment")
			for _, segment := range segments[:badSegment] {
				badOffset += len(segment) + 1
			}
			segments[badSegment] = badName
		}
		paths[i] = strings.Join(segments, ".")
	}
	for _, path := range paths[:badPath] {
		badOffset += len(path) + 2
	}
	observation := strings.Join(paths, ", ")
	text := "component sample \"Observation syntax\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty " + kebabNames.Draw(t, "property") +
		" postcondition \"Observation paths\" {\n  about Account.touch\n  forall account: Account\n  observe " + observation + "\n  changes none\n  call account.touch()\n  ensure true\n}\n"
	offset := strings.Index(text, "  observe ") + len("  observe ") + badOffset
	line, column := lineColumn(text, offset)
	return newBroken(t, FileKindComponent, text, line, column, "syntax.invalid-name")
}

//uc:generates syntax.observation-syntax-rejects-malformed-paths.broken
func genBrokenObservationSyntax() *rapid.Generator[Broken] {
	return rapid.Custom(brokenObservationSyntax)
}

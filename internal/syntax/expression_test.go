package syntax

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// expressionSource deliberately brackets every compound expression, so the
// format predicate checks that removing redundant brackets preserves its tree.
func expressionSource(t *rapid.T, depth int) string {
	choice := rapid.IntRange(0, 10).Draw(t, "expression")
	if depth == 0 {
		choice %= 4
	}
	child := func() string { return expressionSource(t, depth-1) }
	switch choice {
	case 0:
		return string(genNumber(t, false))
	case 1:
		return quote(string(genString(t, "literal")))
	case 2:
		return string(genLower(t, "reference"))
	case 3:
		return rapid.SampledFrom([]string{"true", "false", "none", "result"}).Draw(t, "literal")
	case 4:
		return "(" + child() + " " + rapid.SampledFrom([]string{"+", "-", "*", "/", "==", "!=", "<", "<=", ">", ">=", "and", "or", "implies", "in"}).Draw(t, "operator") + " " + child() + ")"
	case 5:
		return "(not (" + child() + "))"
	case 6:
		return "(" + child() + ")." + string(genLower(t, "field"))
	case 7:
		return "(" + child() + ")[" + child() + "]"
	case 8:
		return string(genLower(t, "operation")) + "(" + child() + ", " + child() + ")"
	case 9:
		return rapid.SampledFrom([]string{"all", "any", "count", "sum"}).Draw(t, "quantifier") + "(" + string(genLower(t, "variable")) + " in " + child() + ": " + child() + ")"
	default:
		if rapid.Bool().Draw(t, "snapshot") {
			return "old(" + child() + ")"
		}
		return rapid.SampledFrom([]string{"raised", "failure"}).Draw(t, "error") + "(" + strings.Join(genTypeRef(t).Segments, ".") + ")"
	}
}

func TestExpressionFormatIsStable(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFormatIsStable(t, expressionSample(t))
	})
}

func FuzzExpressionFormatIsStable(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFormatIsStable(t, expressionSample(t))
	}))
}

func expressionSample(t *rapid.T) Sample {
	text := "component sample \"Expression fixture\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty expression invariant \"Expression normalization\" {\n  about compute\n  ensure " + expressionSource(t, 3) + "\n}\n"
	return Sample{Kind: FileKindComponent, Text: text}
}

func brokenContractSyntax(t *rapid.T) Broken {
	name := fmt.Sprintf("contract%d", rapid.IntRange(0, 9999).Draw(t, "contract name"))
	base := "component sample \"Contract syntax\" {\n  focus core\n  paths \"*.go\"\n}\n\nproperty " + name + " postcondition \"Contract claim\" {\n  about Account.move\n  forall amount: int\n  call Account.move(amount)\n  ensure result == amount\n}\n"
	variant := rapid.IntRange(0, 9).Draw(t, "invalid contract syntax")
	text, code, mark := base, "", "property "
	switch variant {
	case 0:
		text = strings.Replace(base, "  ensure result == amount\n", "", 1)
		code = "syntax.missing-claim"
	case 1:
		text = strings.Replace(base, "  ensure result == amount\n", "  ensure result == amount\n  manual \"Conflicting claim\"\n", 1)
		code = "syntax.mixed-claim"
	case 2:
		text = strings.Replace(base, "  call Account.move(amount)\n", "", 1)
		code = "syntax.invalid-tested-call"
	case 3:
		text = strings.Replace(base, "  call Account.move(amount)\n", "  changes none\n  call Account.move(amount)\n", 1)
		code = "syntax.invalid-observation-scope"
	case 4:
		text = strings.Replace(base, "forall amount: int", "forall size: int", 1)
		code = "syntax.expression-keyword-name"
		mark = "forall size"
	case 5:
		text = strings.Replace(base, "ensure result == amount", "ensure result==amount", 1)
		code = "syntax.expression-spacing"
		mark = "=="
	case 6:
		text = strings.Replace(base, "  ensure result == amount\n", "  within \"invalid\"\n  ensure result == amount\n", 1)
		code = "syntax.invalid-time-budget"
		mark = "within "
	case 7:
		text = base[:strings.Index(base, "property ")] + "flow " + name + " \"Flow claim\" {\n  alt \"Optional call\" {\n  }\n  ensure true\n}\n"
		code = "syntax.invalid-flow-call"
		mark = "alt "
	case 8:
		text = base[:strings.Index(base, "property ")] + "flow " + name + " \"Empty checked flow\" {\n  ensure true\n}\n"
		code = "syntax.invalid-flow-call"
		mark = "flow "
	case 9:
		text = strings.Replace(base, "  call Account.move(amount)\n", "  observe amount\n  changes amount + 1\n  call Account.move(amount)\n", 1)
		code = "syntax.unexpected-token"
		mark = "+"
	}
	offset := strings.Index(text, mark)
	line, column := lineColumn(text, offset)
	return Broken{Kind: FileKindComponent, Text: text, Line: line, Column: column, Code: code}
}

func TestContractSyntaxRejectsInvalidClaims(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkRejectsBrokenText(t, brokenContractSyntax(t)) })
}

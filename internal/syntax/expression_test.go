package syntax

import (
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

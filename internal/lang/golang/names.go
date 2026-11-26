package golang

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lincyaw/unicode/internal/lang"
)

// MapName writes a name of the design as Go writes it (docs/design.md §4).
// An enum value is named with its type, as "OrderStatus.draft", and maps to
// the constant OrderStatusDraft; a predicate's name is the kebab name of its
// property or flow.
func (b *Backend) MapName(role lang.NameRole, designName string) string {
	switch role {
	case lang.NameRoleOp, lang.NameRoleField:
		return upperFirst(designName)
	case lang.NameRoleEnumValue:
		typeName, value, _ := cutLast(designName, ".")
		return typeName + upperFirst(value)
	case lang.NameRolePredicate:
		return "check" + upperCamel(designName)
	case lang.NameRoleFlowPredicate:
		return "checkFlow" + upperCamel(designName)
	default:
		return designName
	}
}

// designName maps the Go name of a field, method or function back to the
// design: an exported name loses its leading capital. Any other name is the
// mapping of no design name, and stays as it is.
func designName(goName string) string {
	if !startsUpper(goName) {
		return goName
	}
	_, size := utf8.DecodeRuneInString(goName)
	return strings.ToLower(goName[:size]) + goName[size:]
}

// predicateName maps a predicate's Go name back to its property's or flow's
// name: checkTotalIsSum to total-is-sum, checkFlowSubmitOrder to
// submit-order. A name that is not a predicate's mapping maps back as an
// operation's.
func predicateName(goName string) string {
	for _, prefix := range []string{"checkFlow", "check"} {
		if rest, ok := strings.CutPrefix(goName, prefix); ok && startsUpper(rest) {
			return kebab(rest)
		}
	}
	return designName(goName)
}

// enumValueName maps a constant of a string type back to its enum value:
// OrderStatusDraft of OrderStatus to draft. A constant whose name is not
// the type's name and a capitalized value is no value's mapping, and stays
// as it is.
func enumValueName(constName, typeName string) string {
	if rest, ok := strings.CutPrefix(constName, typeName); ok && startsUpper(rest) {
		return designName(rest)
	}
	return constName
}

func startsUpper(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

// kebab writes an upper camel name, "TotalIsSum", as "total-is-sum".
func kebab(camel string) string {
	var b strings.Builder
	for i, r := range camel {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('-')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// upperCamel writes a kebab name, "total-is-sum", as "TotalIsSum".
func upperCamel(kebab string) string {
	var b strings.Builder
	for part := range strings.SplitSeq(kebab, "-") {
		b.WriteString(upperFirst(part))
	}
	return b.String()
}

// cutLast cuts s around the last sep; without one, before is empty.
func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return "", s, false
	}
	return s[:i], s[i+len(sep):], true
}

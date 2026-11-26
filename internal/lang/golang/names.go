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
	r, size := utf8.DecodeRuneInString(goName)
	if !unicode.IsUpper(r) {
		return goName
	}
	return string(unicode.ToLower(r)) + goName[size:]
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

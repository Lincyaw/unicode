package design

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// checkCase checks that a case of owner gives every symbol exactly one value,
// of the symbol's type and within its limits (docs/model.md §10 rule 26).
func (l *loader) checkCase(c, owner *entity, lines []syntax.CaseLine) {
	given := map[string]bool{}
	for _, line := range lines {
		a, ok := line.(*syntax.Assignment)
		if !ok {
			continue
		}
		name := string(a.Symbol)
		s := owner.symbol(name)
		switch {
		case s == nil:
			l.report(c.file, a.Pos, "design.unknown-symbol", c.id,
				fmt.Sprintf("%s is not a symbol of %s", name, owner.id),
				"give values only to the symbols that "+owner.id+" declares with forall, or remove this line")
			continue
		case given[name]:
			l.report(c.file, a.Pos, "design.duplicate-value", c.id,
				fmt.Sprintf("the case gives the symbol %s a second value", name), "keep one value for "+name)
			continue
		}
		given[name] = true
		if !l.conform(c, a.Value, s.typ) {
			continue
		}
		for _, lim := range owner.limits {
			if lim.at != nil && lim.path[0] == name {
				l.within(c, a.Value, s.typ, lim.path[1:], lim)
			}
		}
	}
	var missing []string
	for _, s := range owner.symbols {
		if !given[s.name] {
			missing = append(missing, s.name)
		}
	}
	if len(missing) > 0 {
		l.report(c.file, c.pos, "design.missing-value", c.id,
			fmt.Sprintf("the case gives no value to %s", strings.Join(missing, ", ")),
			"give every symbol of "+owner.id+" a value, written symbol = value")
	}
}

func valuePos(v syntax.Value) lexer.Position {
	switch v := v.(type) {
	case *syntax.NumberValue:
		return v.Pos
	case *syntax.StringValue:
		return v.Pos
	case *syntax.CallValue:
		return v.Pos
	case *syntax.BoolValue:
		return v.Pos
	case *syntax.NoneValue:
		return v.Pos
	case *syntax.NameValue:
		return v.Pos
	case *syntax.ListValue:
		return v.Pos
	case *syntax.BraceValue:
		return v.Pos
	}
	panic("design: a value of an unknown kind")
}

// conform checks that v is a value of t (docs/language.md §3.1) and reports
// each place where it is not. A value of an unresolved type, an interface or
// an error passes: those already have their diagnostics, or no literal values.
func (l *loader) conform(c *entity, v syntax.Value, t *typ) bool {
	wrong := func(at syntax.Value, why string) bool {
		l.report(c.file, valuePos(at), "design.value-type-mismatch", c.id,
			fmt.Sprintf("%s is not a value of %s: %s", syntax.ValueText(at), t.text(), why),
			"write a value of "+t.text()+" by docs/language.md §3.1")
		return false
	}
	if _, none := v.(*syntax.NoneValue); none && t.kind != "optional" {
		return wrong(v, "only an optional may be none")
	}
	switch t.kind {
	case "optional":
		if _, none := v.(*syntax.NoneValue); none {
			return true
		}
		return l.conform(c, v, t.elems[0])
	case "list", "set":
		list, ok := v.(*syntax.ListValue)
		if !ok {
			return wrong(v, "a "+t.kind+" is written [item, ...]")
		}
		all := true
		for _, item := range list.Items {
			all = l.conform(c, item, t.elems[0]) && all
		}
		return all
	case "map":
		return l.conformMap(c, v, t, wrong)
	case "call":
		return l.conformCall(c, v, t, wrong)
	case "replies":
		if len(t.refs) == 0 || t.refs[0].target == nil {
			return true
		}
		return l.conformReplies(c, v, t.refs[0].target, wrong)
	}
	if t.builtin != "" {
		if why := builtinMismatch(v, t.builtin); why != "" {
			return wrong(v, why)
		}
		return true
	}
	n := t.named()
	switch {
	case n != nil && n.form == "interface":
		return l.conformInterface(c, v, n, wrong)
	case n == nil || n.form == "error":
		return true
	case n.form == "enum":
		name, ok := v.(*syntax.NameValue)
		switch {
		case !ok:
			return wrong(v, "an enum's value is written as the name of one of its values")
		case !slices.Contains(n.values, string(name.Name)):
			return wrong(v, fmt.Sprintf("%s has no value %s", n.id, name.Name))
		}
		return true
	}
	return l.conformRecord(c, v, n, wrong)
}

func builtinMismatch(v syntax.Value, builtin string) string {
	switch builtin {
	case "bool":
		if _, ok := v.(*syntax.BoolValue); !ok {
			return "a bool is true or false"
		}
	case "int", "float":
		n, ok := v.(*syntax.NumberValue)
		switch {
		case !ok:
			return "a " + builtin + " is a number"
		case builtin == "int" && strings.Contains(string(n.Text), "."):
			return "an int has no fraction"
		}
	default:
		s, ok := v.(*syntax.StringValue)
		if !ok {
			return "a " + builtin + " is written as a string"
		}
		return textMismatch(string(s.Text), builtin)
	}
	return ""
}

// textMismatch tells why a string is not a value of a builtin written as a string.
func textMismatch(s, builtin string) string {
	switch builtin {
	case "time":
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return "a time is written in RFC 3339, like \"2026-11-26T08:00:00Z\""
		}
	case "duration":
		if _, err := time.ParseDuration(s); err != nil {
			return "a duration is written like \"1h30m\""
		}
	}
	return ""
}

func (l *loader) conformMap(c *entity, v syntax.Value, t *typ, wrong func(syntax.Value, string) bool) bool {
	braces, ok := v.(*syntax.BraceValue)
	if !ok {
		return wrong(v, `a map is written {key: value, ...}`)
	}
	key := t.elems[0]
	all := true
	seen := map[string]bool{}
	for _, entry := range braces.Entries {
		k, why := keyText(entry.Key, key)
		switch {
		case why != "":
			all = wrong(v, why)
		case seen[k]:
			all = wrong(v, "the key "+k+" is given twice")
		}
		seen[k] = true
		all = l.conform(c, entry.Value, t.elems[1]) && all
	}
	return all
}

// keyText writes a map key, or tells why it is not a key of type key.
func keyText(k syntax.Key, key *typ) (string, string) {
	switch {
	case key.builtin == "int" || key.builtin == "float":
		if k.Number == nil || key.builtin == "int" && strings.Contains(string(*k.Number), ".") {
			return "", "a key of type " + key.builtin + " is written as a number of that type"
		}
		return string(*k.Number), ""
	case key.builtin == "string" || key.builtin == "bytes" || key.builtin == "time" || key.builtin == "duration":
		if k.String == nil {
			return "", "a key of type " + key.builtin + " is written as a string"
		}
		return strconv.Quote(string(*k.String)), textMismatch(string(*k.String), key.builtin)
	case key.named().is(graph.NodeKindType, "enum"):
		if k.Name == nil || !slices.Contains(key.named().values, string(*k.Name)) {
			return "", "a key of type " + key.text() + " is one of its values"
		}
		return string(*k.Name), ""
	case !key.resolved():
		return "", ""
	}
	return "", "a key of type " + key.text() + " cannot be written"
}

func (l *loader) conformRecord(c *entity, v syntax.Value, n *entity, wrong func(syntax.Value, string) bool) bool {
	braces, ok := v.(*syntax.BraceValue)
	if !ok {
		return wrong(v, "a "+n.form+" is written {field: value, ...} with all its fields")
	}
	all := true
	given := map[string]bool{}
	for _, entry := range braces.Entries {
		var f *member
		if entry.Key.Name != nil {
			f = findMember(visibleFields(n), string(*entry.Key.Name))
		}
		switch {
		case f == nil:
			all = wrong(v, n.id+" has no field "+keyName(entry.Key))
			continue
		case given[f.name]:
			all = wrong(v, "the field "+f.name+" is given twice")
			continue
		}
		given[f.name] = true
		all = l.conform(c, entry.Value, f.typ) && all
	}
	for _, f := range visibleFields(n) {
		if !given[f.name] {
			all = wrong(v, "the field "+f.name+" is missing; a value gives every field")
		}
	}
	return all
}

func keyName(k syntax.Key) string {
	switch {
	case k.Name != nil:
		return string(*k.Name)
	case k.String != nil:
		return strconv.Quote(string(*k.String))
	}
	return string(*k.Number)
}

func (l *loader) conformCall(c *entity, v syntax.Value, t *typ, wrong func(syntax.Value, string) bool) bool {
	call, ok := v.(*syntax.CallValue)
	if !ok {
		return wrong(v, "a call is written op(param: value, ...)")
	}
	op := findEntity(t.ops(), string(call.Op))
	if op == nil {
		return wrong(v, string(call.Op)+" is not an operation that "+t.text()+" may call")
	}
	all := true
	given := map[string]bool{}
	for _, arg := range call.Args {
		p := findMember(op.params, string(arg.Name))
		switch {
		case p == nil:
			all = wrong(v, op.id+" has no parameter "+string(arg.Name))
			continue
		case given[p.name]:
			all = wrong(v, "the parameter "+p.name+" is given twice")
			continue
		}
		given[p.name] = true
		all = l.conform(c, arg.Value, p.typ) && all
	}
	for _, p := range op.params {
		if !given[p.name] {
			all = wrong(v, "the parameter "+p.name+" is missing; a call gives every parameter")
		}
	}
	return all
}

// within checks the parts of v, a value of t, that a limit's remaining path
// segs leads to (docs/model.md §10 rule 26); v is known to conform to t.
func (l *loader) within(c *entity, v syntax.Value, t *typ, segs []string, lim *limit) {
	if len(segs) == 0 && t == lim.at {
		if why := outside(v, t, lim); why != "" {
			l.report(c.file, valuePos(v), "design.value-out-of-limit", c.id,
				fmt.Sprintf("%s is outside the limit %s: %s", syntax.ValueText(v), lim.text(), why),
				"give a value within the limit")
		}
		return
	}
	if _, none := v.(*syntax.NoneValue); none {
		return
	}
	switch t.kind {
	case "optional":
		l.within(c, v, t.elems[0], segs, lim)
	case "list", "set":
		for _, item := range v.(*syntax.ListValue).Items {
			l.within(c, item, t.elems[0], segs, lim)
		}
	case "map":
		for _, entry := range v.(*syntax.BraceValue).Entries {
			l.within(c, entry.Value, t.elems[1], segs, lim)
		}
	case "call":
		call := v.(*syntax.CallValue)
		if len(segs) < 2 || string(call.Op) != segs[0] {
			return
		}
		p := findMember(findEntity(t.ops(), segs[0]).params, segs[1])
		for _, arg := range call.Args {
			if string(arg.Name) == segs[1] {
				l.within(c, arg.Value, p.typ, segs[2:], lim)
			}
		}
	case "":
		if len(segs) == 0 || t.named() == nil {
			return
		}
		f := findMember(visibleFields(t.named()), segs[0])
		for _, entry := range v.(*syntax.BraceValue).Entries {
			if string(*entry.Key.Name) == segs[0] {
				l.within(c, entry.Value, f.typ, segs[1:], lim)
			}
		}
	}
}

// outside tells why v, at the type the limit applies to, breaks it, or returns "".
func outside(v syntax.Value, t *typ, lim *limit) string {
	switch lim.op {
	case "in":
		n := rat(string(v.(*syntax.NumberValue).Text))
		if n.Cmp(rat(lim.low)) < 0 || n.Cmp(rat(lim.high)) > 0 {
			return "it is not between " + lim.low + " and " + lim.high
		}
	case "subset":
		if name := string(v.(*syntax.NameValue).Name); !slices.Contains(lim.members, name) {
			return name + " is not among " + strings.Join(lim.members, ", ")
		}
	default:
		var size int
		switch x := v.(type) {
		case *syntax.ListValue:
			size = len(x.Items)
		case *syntax.BraceValue:
			size = len(x.Entries)
		case *syntax.StringValue:
			size = len(x.Text)
			if t.builtin == "string" {
				size = utf8.RuneCountInString(string(x.Text))
			}
		}
		if low, high, n := rat(lim.low), rat(lim.high), big.NewRat(int64(size), 1); n.Cmp(low) < 0 || n.Cmp(high) > 0 {
			return fmt.Sprintf("its size %d is not between %s and %s", size, lim.low, lim.high)
		}
	}
	return ""
}

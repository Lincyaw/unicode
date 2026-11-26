package design

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// nameRole is what a name mapping applies to: lang.NameRole of design/lang.uc,
// which takes its place when the mapping moves to the lang package in M2.
type nameRole string

const (
	roleType          nameRole = "type"
	roleOp            nameRole = "op"
	roleField         nameRole = "field"
	rolePredicate     nameRole = "predicate"
	roleFlowPredicate nameRole = "flowPredicate"
)

// mapping is the name mapping of one language, and the language's name for messages.
type mapping struct {
	title   string
	mapName func(role nameRole, name string) string
}

// mappings holds the name mapping of each language that has one here.
var mappings = map[string]mapping{"go": {"Go", goName}}

// goName writes a design name as Go code names it (docs/design.md §4): a
// type, an operation or a field in upper camel case, the predicate of
// property p as checkP and that of flow f as checkFlowF. The lang package
// owns name mapping; in M2 this function moves there, as the Go backend's MapName.
func goName(role nameRole, name string) string {
	switch role {
	case rolePredicate:
		return "check" + upperCamel(name)
	case roleFlowPredicate:
		return "checkFlow" + upperCamel(name)
	}
	return upperCamel(name)
}

// upperCamel joins the segments of a kebab name, or the one segment of a
// camel name, each with an upper-case first letter.
func upperCamel(name string) string {
	segs := strings.Split(name, "-")
	for i, s := range segs {
		segs[i] = strings.ToUpper(s[:1]) + s[1:]
	}
	return strings.Join(segs, "")
}

// codeKey is a code name in one language within one scope: a component, for
// its types, operations and predicates, or a type, for its fields and operations.
type codeKey struct {
	scope    *entity
	language string
	code     string
}

// codeHolder is the member that took a code name first.
type codeHolder struct {
	name, what, where string
}

// codeNameOf records the code names of e, a type or an operation, or the
// predicate of a flow or a property, in its parent, when its parent is a
// component or a type.
func (l *loader) codeNameOf(e *entity, role nameRole) {
	what := describe(e)
	if role == rolePredicate || role == roleFlowPredicate {
		what = "the predicate of " + what
	}
	if e.parent.is(graph.NodeKindContainer, "component") || e.parent.is(graph.NodeKindType) {
		l.codeName(e.parent, role, e.name, what, e.file, e.pos, e.id)
	}
}

// codeName records the code name that each configured language gives a
// member of scope, and reports a member whose code name an earlier member
// has taken (docs/model.md §10 rule 30). Two members with the same design
// name already have their design.duplicate-name, so they get no second diagnostic.
func (l *loader) codeName(scope *entity, role nameRole, name, what string, f *syntax.File, pos lexer.Position, subject string) {
	for _, language := range l.languages {
		m, ok := mappings[language]
		if !ok {
			continue
		}
		key := codeKey{scope, language, m.mapName(role, name)}
		prior, taken := l.codes[key]
		switch {
		case !taken:
			l.codes[key] = codeHolder{name, what, fmt.Sprintf("%s:%d", f.Path, pos.Line)}
		case prior.name != name:
			l.report(f, pos, "design.name-collision", subject,
				fmt.Sprintf("in %s, %s takes the name %s in %s, as %s at %s does", m.title, what, key.code, describe(scope), prior.what, prior.where),
				fmt.Sprintf("rename one of the two, so that the name mapping of docs/design.md §4 gives them different %s names", m.title))
		}
	}
}

// digitAfterDash finds a dash that a digit follows: upper camel case keeps no
// trace of that dash, so a-2b and a2b map to the same A2b.
var digitAfterDash = regexp.MustCompile(`-[0-9]`)

// roundTrip reports the name of a property or a flow, e, whose predicate's
// code name does not map back to it (docs/model.md §10 rule 30): a property
// named flow-x, whose predicate a flow x's would be, or a name with a digit
// right after a dash.
func (l *loader) roundTrip(e *entity, role nameRole) {
	var why, fix string
	switch code := goName(role, e.name); {
	case role == rolePredicate && strings.HasPrefix(e.name, "flow-"):
		why = fmt.Sprintf("its predicate's Go name %s is the one that the predicate of a flow %s takes", code, strings.TrimPrefix(e.name, "flow-"))
		fix = "rename the property so that its name does not start with flow-"
	case digitAfterDash.MatchString(e.name):
		why = fmt.Sprintf("a digit follows a dash, and its predicate's Go name %s keeps no trace of that dash", code)
		fix = "rename it so that every segment after a dash starts with a letter"
	default:
		return
	}
	l.report(e.file, e.pos, "design.irreversible-name", e.id,
		fmt.Sprintf("the name of %s does not map back from the code (docs/design.md §4): %s", describe(e), why), fix)
}

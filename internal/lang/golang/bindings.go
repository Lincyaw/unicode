package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// The bindings of docs/language.md §6.1: a line `//uc:<verb> <identity>` in the
// doc comment of a type or a function.
const bindingPrefix = "//uc:"

var bindingKinds = map[string]graph.EdgeKind{
	"realizes":  graph.EdgeKindRealizes,
	"checks":    graph.EdgeKindChecks,
	"generates": graph.EdgeKindGenerates,
}

// namePattern is a name of the design: kebab, upper or lower camel
// (docs/language.md §2). designRef is an identity, names joined by dots
// (docs/language.md §3, Ref), and symbolSuffix the lower camel name of a symbol
// at the end of one.
const namePattern = `(?:[a-z][a-z0-9]*(?:-[a-z0-9]+)*|[A-Za-z][A-Za-z0-9]*)`

var (
	designRef    = regexp.MustCompile(`^` + namePattern + `(?:\.` + namePattern + `)*$`)
	symbolSuffix = regexp.MustCompile(`\.[a-z][A-Za-z0-9]*$`)
)

// bound is the declaration a doc comment belongs to.
type bound struct {
	id     string
	isType bool
}

// bindingKey is one binding of one declaration.
type bindingKey struct{ id, verb, target string }

// readBindings reads every //uc: line of a file. A well-formed one in the doc
// comment of a declaration that is a node becomes an edge; every other one
// gets exactly one diagnostic.
func (x *extraction) readBindings(p *packages.Package, f sourceFile, fileID string, docs map[*ast.CommentGroup]bound) {
	for _, group := range f.syntax.Comments {
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, bindingPrefix) {
				owner, attached := docs[group]
				x.readBinding(x.position(p, c.Pos()), c.Text, fileID, owner, attached)
			}
		}
	}
}

func (x *extraction) readBinding(at diag.Position, text, fileID string, owner bound, attached bool) {
	subject := fileID
	if attached {
		subject = owner.id
	}
	report := func(code, message, fix string) {
		x.diagnostics = append(x.diagnostics, diag.Diagnostic{
			Code: code, Subject: subject, Position: at, Message: message, Fix: fix,
		})
	}
	verb, target, _ := strings.Cut(strings.TrimPrefix(text, bindingPrefix), " ")
	kind, known := bindingKinds[verb]
	switch {
	case !known:
		report("lang.golang.unknown-binding-verb", fmt.Sprintf("the binding verb %q is not realizes, checks or generates", verb),
			"write the binding as //uc:realizes, //uc:checks or //uc:generates, one space and a design identity")
		return
	case target == "":
		report("lang.golang.missing-binding-id", fmt.Sprintf("the binding //uc:%s names no design identity", verb),
			fmt.Sprintf("write the identity after one space: //uc:%s <identity>", verb))
		return
	case !designRef.MatchString(target):
		report("lang.golang.invalid-binding-id", fmt.Sprintf("%q is not an identity of the design", target),
			"write the identity as names joined by dots, with one space before it and nothing after it")
		return
	case kind == graph.EdgeKindGenerates && !symbolSuffix.MatchString(target):
		report("lang.golang.invalid-binding-id", fmt.Sprintf("%q does not name a symbol of a property or flow", target),
			"write the symbol's identity: the property's or flow's identity, a dot and the symbol's name")
		return
	case !attached:
		report("lang.golang.misplaced-binding", "the binding is not in the doc comment of a type or a function",
			"move it into the doc comment right above the type or function it binds")
		return
	case owner.isType && kind != graph.EdgeKindRealizes:
		report("lang.golang.misplaced-binding", fmt.Sprintf("a %s binding is on a type, and only a function can carry one", verb),
			"move it into the doc comment of the function it binds")
		return
	}
	key := bindingKey{owner.id, verb, target}
	if x.bindings[key] {
		report("lang.golang.duplicate-binding", fmt.Sprintf("the declaration already has the binding //uc:%s %s", verb, target),
			"delete the repeated line")
		return
	}
	x.bindings[key] = true
	x.addBinding(kind, owner, target, at)
}

// addBinding adds the edge of a binding; a function that generates several
// symbols of one property or flow has one edge listing them.
func (x *extraction) addBinding(kind graph.EdgeKind, owner bound, target string, at diag.Position) {
	attrs := map[string][]string(nil)
	if kind == graph.EdgeKindGenerates {
		var symbol string
		target, symbol, _ = cutLast(target, ".")
		attrs = map[string][]string{"symbols": {symbol}}
	}
	key := edgeKey{kind, owner.id, target}
	if e, ok := x.edges[key]; ok {
		e.Attrs["symbols"] = append(e.Attrs["symbols"], attrs["symbols"]...)
		return
	}
	x.edges[key] = &graph.Edge{Kind: kind, From: owner.id, To: target, Source: graph.SourceDeclared, Origin: at, Attrs: attrs}
	switch {
	case kind == graph.EdgeKindRealizes && owner.isType:
		x.realizes[owner.id] = append(x.realizes[owner.id], target)
	case kind == graph.EdgeKindChecks:
		x.predicates[owner.id] = true
	}
}

// addCandidates adds, for every exported method of a type that realizes a
// design type, an interface's methods included, a derived realizes edge to
// the operation its name maps back to (docs/model.md §9). Deriving drops the
// ones whose operation does not exist.
func (x *extraction) addCandidates() {
	for _, d := range x.declared {
		if d.fn == nil || receiver(d.fn) == nil || !token.IsExported(d.fn.Name()) {
			continue
		}
		for _, target := range x.realizes[typeID(receiver(d.fn).Obj())] {
			key := edgeKey{graph.EdgeKindRealizes, d.id, target + "." + designName(d.fn.Name())}
			if _, ok := x.edges[key]; !ok {
				x.edges[key] = &graph.Edge{
					Kind: key.kind, From: key.from, To: key.to, Source: graph.SourceDerived, Origin: x.position(d.pkg, d.pos),
				}
			}
		}
	}
}

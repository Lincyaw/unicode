package design

import (
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// inheritedMembers includes inherited declarations once, even for a malformed
// cyclic hierarchy: the hierarchy checker owns that diagnostic.
func (l *loader) inheritedMembers(n *entity) ([]member, []*entity) { return visibleMembers(n) }

func (l *loader) conformReplies(c *entity, v syntax.Value, op *entity, wrong func(syntax.Value, string) bool) bool {
	list, ok := v.(*syntax.ListValue)
	if !ok || len(list.Items) == 0 {
		return wrong(v, "a response script is a nonempty list of response records")
	}
	all := true
	for _, item := range list.Items {
		b, ok := item.(*syntax.BraceValue)
		if !ok {
			all = wrong(item, "a response is a record")
			continue
		}
		entries := map[string]syntax.Value{}
		for _, e := range b.Entries {
			if e.Key.Name == nil {
				all = wrong(item, "response keys are returns, raises or fields")
				continue
			}
			key := string(*e.Key.Name)
			if entries[key] != nil {
				all = wrong(item, "a response key is repeated")
			}
			entries[key] = e.Value
		}
		if value, ok := entries["returns"]; ok {
			if len(entries) != 1 {
				all = wrong(item, "a successful response has exactly one returns field")
				continue
			}
			if op.result == nil {
				if _, ok := value.(*syntax.NoneValue); !ok {
					all = wrong(value, "a void response returns none")
				}
			} else {
				all = l.conform(c, value, op.result) && all
			}
			continue
		}
		name, ok := entries["raises"].(*syntax.StringValue)
		if !ok || entries["fields"] == nil || len(entries) != 2 {
			all = wrong(item, "a failed response has exactly raises and fields")
			continue
		}
		failure := l.byID[string(name.Text)]
		if !failure.is(graph.NodeKindType, "error") {
			all = wrong(item, "raises names an absolute declared error identity")
			continue
		}
		allowed := slices.Contains(op.raises, failure)
		queue := []*entity{failure}
		seen := map[string]bool{}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if seen[n.id] {
				continue
			}
			seen[n.id] = true
			allowed = allowed || slices.Contains(op.raises, n)
			for _, e := range l.edges {
				if e.Kind == graph.EdgeKindExtends && e.From == n.id && l.byID[e.To] != nil {
					queue = append(queue, l.byID[e.To])
				}
			}
		}
		if !allowed {
			all = wrong(item, "the error is not declared by the operation or descended from a declared error")
			continue
		}
		fields, _ := l.inheritedMembers(failure)
		view := *failure
		view.fields = fields
		all = l.conformRecord(c, entries["fields"], &view, wrong) && all
	}
	return all
}

func (l *loader) conformInterface(c *entity, v syntax.Value, n *entity, wrong func(syntax.Value, string) bool) bool {
	b, ok := v.(*syntax.BraceValue)
	if !ok {
		return wrong(v, "an interface value is a record of nonempty operation response scripts")
	}
	_, ops := l.inheritedMembers(n)
	given := map[string]bool{}
	all := true
	for _, e := range b.Entries {
		if e.Key.Name == nil {
			all = wrong(v, "interface response keys are operation names")
			continue
		}
		key := string(*e.Key.Name)
		op := findEntity(ops, key)
		if op == nil || given[key] {
			all = wrong(v, "the interface operation is unknown or repeated: "+key)
			continue
		}
		given[key] = true
		all = l.conformReplies(c, e.Value, op, wrong) && all
	}
	for _, op := range ops {
		if !given[op.name] {
			all = wrong(v, "the interface response script is missing: "+op.name)
		}
	}
	return all
}

func containsReplies(t *typ) bool {
	if t.kind == "replies" {
		return true
	}
	for _, e := range t.elems {
		if containsReplies(e) {
			return true
		}
	}
	return false
}

func generableResponse(t *typ, required, seen map[*entity]bool) string {
	n := t.named()
	if !n.is(graph.NodeKindType, "interface") {
		return generable(t, required, seen)
	}
	if required[n] {
		return n.id + " requires itself through response fields that cannot be empty"
	}
	if seen[n] {
		return ""
	}
	required[n], seen[n] = true, true
	defer delete(required, n)
	defer delete(seen, n)
	for _, op := range visibleOperations(n) {
		if op.result != nil {
			if why := generableResponse(op.result, required, seen); why != "" {
				return why
			}
		}
		for _, failure := range op.raises {
			for _, field := range visibleFields(failure) {
				if why := generableResponse(field.typ, required, seen); why != "" {
					return why
				}
			}
		}
	}
	return ""
}

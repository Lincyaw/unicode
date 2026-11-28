package design

import "github.com/lincyaw/unicode/internal/graph"

// visibleMembers preserves declarations on the type before traversing bases in
// source order, and then the interfaces it implements, which add operations
// (docs/language.md §4.4). A member name is chosen once; diamonds and cycles
// are finite.
func visibleMembers(n *entity) ([]member, []*entity) {
	var fields []member
	var operations []*entity
	typesSeen := map[*entity]bool{}
	fieldsSeen, operationsSeen := map[string]bool{}, map[string]bool{}
	var visit func(current *entity, withFields bool)
	visit = func(current *entity, withFields bool) {
		if current == nil || typesSeen[current] {
			return
		}
		typesSeen[current] = true
		for _, field := range current.fields {
			if withFields && !fieldsSeen[field.name] {
				fieldsSeen[field.name] = true
				fields = append(fields, field)
			}
		}
		for _, operation := range current.ops {
			if !operationsSeen[operation.name] {
				operationsSeen[operation.name] = true
				operations = append(operations, operation)
			}
		}
		for _, base := range current.bases {
			visit(base, withFields)
		}
	}
	visit(n, true)
	// The interfaces the type and its bases implement come after the type's
	// own and inherited members, and give operations only (docs/language.md
	// §4.4), those their own bases declare included.
	implementing := []*entity{n}
	seen := map[*entity]bool{n: true}
	for i := 0; i < len(implementing); i++ {
		for _, base := range implementing[i].bases {
			if !seen[base] {
				seen[base] = true
				implementing = append(implementing, base)
			}
		}
	}
	for _, current := range implementing {
		for _, iface := range current.interfaces {
			visit(iface, false)
		}
	}
	return fields, operations
}
func visibleFields(n *entity) []member      { fields, _ := visibleMembers(n); return fields }
func visibleOperations(n *entity) []*entity { _, operations := visibleMembers(n); return operations }

// receiverType preserves a type reference's receiver even when the selected
// visible operations were declared on one of its bases.
func (t *typ) receiverType() (*entity, bool) {
	var receiver *entity
	for _, reference := range t.refs {
		target := reference.target
		if target == nil {
			continue
		}
		if target.kind == graph.NodeKindOp {
			target = target.parent
		}
		if receiver == nil {
			receiver = target
		} else if receiver != target {
			return nil, false
		}
	}
	return receiver, true
}

package design

import "github.com/lincyaw/unicode/internal/graph"

// visibleMembers preserves declarations on the type before traversing bases in
// source order. A member name is chosen once; diamonds and cycles are finite.
func visibleMembers(n *entity) ([]member, []*entity) {
	var fields []member
	var operations []*entity
	typesSeen := map[*entity]bool{}
	fieldsSeen, operationsSeen := map[string]bool{}, map[string]bool{}
	var visit func(*entity)
	visit = func(current *entity) {
		if current == nil || typesSeen[current] {
			return
		}
		typesSeen[current] = true
		for _, field := range current.fields {
			if !fieldsSeen[field.name] {
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
			visit(base)
		}
	}
	visit(n)
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

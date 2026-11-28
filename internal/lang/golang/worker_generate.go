package golang

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func (c *contractCompiler) renderWorkerUnits(file *contractFile) string {
	var out strings.Builder
	out.WriteString("[]ucWorkerUnit{")
	for i, unit := range file.units {
		index := strconv.Itoa(i)
		fmt.Fprintf(&out, "{Spec:ucSpec%s,Types:[]reflect.Type{", index)
		for _, symbol := range unit.spec.Symbols {
			fmt.Fprintf(&out, "reflect.TypeFor[%s](),", c.goType(symbol.Type, file.pkg, file.imports))
		}
		fmt.Fprintf(&out, "},Draw:ucInputs%s,Derive:ucDerive%s,Invoke:ucInvoke%s,Cases:[]ucWorkerCase{", index, index, index)
		for ci, ca := range unit.cases {
			node := unit.caseNodes[ci]
			fmt.Fprintf(&out, "{Id:%s,Form:%s,Reason:%s,Make:func(s *ucContract)map[string]reflect.Value{return map[string]reflect.Value{", strconv.Quote(node.Id), strconv.Quote(node.Form), strconv.Quote(first(node.Attrs["reason"])))
			for si, text := range ca {
				if slices.Contains(unit.spec.Derived, unit.spec.Symbols[si].Name) {
					continue
				}
				literal, err := c.literal(text, c.goType(unit.spec.Symbols[si].Type, file.pkg, file.imports), file.pkg)
				if err != nil {
					c.diagnostic(unit.node, "lang.golang.unsupported-case", err.Error(), "use an explicitly replayable model value or encoded representation")
					return "nil"
				}
				fmt.Fprintf(&out, "%s:reflect.ValueOf(%s),", strconv.Quote(unit.spec.Symbols[si].Name), literal)
			}
			out.WriteString("}}},")
		}
		out.WriteString("}},")
	}
	out.WriteString("}")
	return out.String()
}

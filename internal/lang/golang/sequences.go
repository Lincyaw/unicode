package golang

import (
	"fmt"
	"go/ast"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

func (c *contractCompiler) carrierType(model string, _ *packages.Package) string {
	if c.carrierModels == nil {
		c.carrierModels = map[string]string{}
	}
	name := modelCarrierName(model)
	c.carrierModels[name] = model
	return name
}
func (c *contractCompiler) configureSequences(unit *contractUnit) bool {
	var visit func(string) bool
	visiting := map[string]bool{}
	visit = func(model string) bool {
		if visiting[model] {
			return true
		}
		visiting[model] = true
		kind, args := ucSplitType(model)
		if kind != "call" {
			for _, arg := range args {
				if !visit(arg) {
					return false
				}
			}
			for _, field := range unit.spec.Types[model].Fields {
				if !visit(field.Type) {
					return false
				}
			}
			return true
		}
		c.carrierType(model, unit.pkg)
		ty := ucType{Form: "call"}
		chosen := map[string]bool{}
		for _, target := range args {
			if node := c.nodes[target]; node.Kind == graph.NodeKindType {
				for _, operation := range c.visibleOperationIDs(target) {
					chosen[operation] = true
				}
			} else {
				chosen[target] = true
			}
		}
		for _, id := range slices.Sorted(maps.Keys(chosen)) {
			op, ok := unit.spec.Operations[id]
			if !ok {
				c.diagnostic(unit.node, "lang.golang.unsupported-contract", "the call type names an unavailable operation "+id, "realize every operation selected by the call type")
				return false
			}
			d, bound := c.bound(graph.EdgeKindRealizes, id)
			fn, isFunc := d.obj.(*types.Func)
			if !bound || !isFunc {
				c.diagnostic(unit.node, "lang.golang.unsupported-contract", "the call operation "+id+" has no native Go signature", "realize the operation before generating its calls")
				return false
			}
			if fn.Signature().Params().Len() != len(op.Params) {
				c.diagnostic(unit.node, "lang.golang.unsupported-contract", "the call operation's native parameter count differs from its model", "make the realization signature match the operation")
				return false
			}
			ty.Calls = append(ty.Calls, op)
		}
		unit.spec.Types[model] = ty
		return true
	}
	for _, symbol := range unit.spec.Symbols {
		if !visit(symbol.Type) {
			return false
		}
	}
	if unit.predicate != nil {
		return true
	}
	for _, symbol := range unit.spec.Symbols {
		kind, args := ucSplitType(symbol.Type)
		if kind != "list" {
			continue
		}
		callKind, _ := ucSplitType(args[0])
		if callKind != "call" {
			continue
		}
		calls := unit.spec.Types[args[0]].Calls
		if len(calls) == 0 {
			c.diagnostic(unit.node, "lang.golang.unsupported-contract", "the sequence has no callable operations", "select at least one operation in its call type")
			return false
		}
		unit.spec.Sequence = symbol.Name
		_, targets := ucSplitType(args[0])
		receiverType := ""
		for _, target := range targets {
			owner := target
			if c.nodes[target].Kind == graph.NodeKindOp {
				owner = c.parents[target]
			}
			if receiverType != "" && receiverType != owner {
				c.diagnostic(unit.node, "lang.golang.unsupported-contract", "the sequence combines different receiver types", "use one receiver type in the call union")
				return false
			}
			receiverType = owner
		}
		for _, receiver := range unit.spec.Symbols {
			if receiver.Type == receiverType {
				unit.spec.SequenceReceiver = receiver.Name
			}
		}
	}
	return true
}
func (c *contractCompiler) nativeType(t types.Type, pkg *packages.Package, imports map[string]string) string {
	return types.TypeString(t, func(p *types.Package) string {
		if p.Path() == pkg.PkgPath {
			return ""
		}
		if existing, ok := imports[p.Path()]; ok {
			return existing
		}
		alias := "ucpkg" + strconv.Itoa(len(imports))
		imports[p.Path()] = alias
		return alias
	})
}
func (c *contractCompiler) renderCallDeclarations(file *contractFile) string {
	models := map[string]bool{}
	for _, unit := range file.units {
		for model, ty := range unit.spec.Types {
			if ty.Form == "call" {
				models[model] = true
			}
		}
	}
	var out strings.Builder
	for _, model := range slices.Sorted(maps.Keys(models)) {
		out.WriteString(modelCarrierDeclaration(model))
	}
	return out.String()
}
func (c *contractCompiler) renderCallSpec(body *strings.Builder, file *contractFile, unit *contractUnit) {
	for _, model := range slices.Sorted(maps.Keys(unit.spec.Types)) {
		ty := unit.spec.Types[model]
		if ty.Form != "call" {
			continue
		}
		fmt.Fprintf(body, "{ty:=s.Types[%s];", strconv.Quote(model))
		for i, op := range ty.Calls {
			d, _ := c.bound(graph.EdgeKindRealizes, op.Name)
			fn, _ := d.obj.(*types.Func)
			var parameterTypes []string
			if fn != nil {
				for parameter := range fn.Signature().Params().Variables() {
					parameterTypes = append(parameterTypes, "reflect.TypeFor["+c.nativeType(parameter.Type(), file.pkg, file.imports)+"]()")
				}
			}
			fmt.Fprintf(body, "ty.Calls[%d].ParamTypes=[]reflect.Type{%s};", i, strings.Join(parameterTypes, ","))
		}
		fmt.Fprintf(body, "s.Types[%s]=ty};", strconv.Quote(model))
	}
}
func (c *contractCompiler) callLiteral(value syntax.Value, t string, pkg *packages.Package) (string, bool, error) {
	call, ok := value.(*syntax.CallValue)
	if !ok {
		return "", false, nil
	}
	model := c.carrierModels[t]
	kind, _ := ucSplitType(model)
	if kind != "call" {
		return "", false, nil
	}
	var selected ucOperation
	_, targets := ucSplitType(model)
	for _, target := range targets {
		ids := []string{target}
		if c.nodes[target].Kind == graph.NodeKindType {
			ids = c.visibleOperationIDs(target)
		}
		for _, id := range ids {
			if lastName(id) == string(call.Op) {
				selected.Name = id
				break
			}
		}
	}

	if selected.Name == "" {
		return "", true, fmt.Errorf("the call %s is not an operation of %s", call.Op, model)
	}
	d, ok := c.bound(graph.EdgeKindRealizes, selected.Name)
	fn, isFunc := d.obj.(*types.Func)
	if !ok || !isFunc {
		return "", true, fmt.Errorf("the call %s has no Go realization", selected.Name)
	}
	parameters, _, _ := ucSignature(first(c.nodes[selected.Name].Attrs["signature"]))
	values := map[string]syntax.Value{}
	for _, arg := range call.Args {
		values[string(arg.Name)] = arg.Value
	}
	var arguments []string
	for i, parameter := range parameters {
		value, ok := values[parameter.Name]
		if !ok {
			return "", true, fmt.Errorf("the call parameter %s is missing", parameter.Name)
		}
		native := c.nativeType(fn.Signature().Params().At(i).Type(), pkg, c.literalImports)
		literal, err := c.valueLiteral(value, native, pkg)
		if err != nil {
			return "", true, err
		}
		arguments = append(arguments, strconv.Quote(parameter.Name)+":"+literal)
	}
	return t + "{Operation:" + strconv.Quote(selected.Name) + ",Arguments:map[string]any{" + strings.Join(arguments, ",") + "}}", true, nil
}
func (c *contractCompiler) retainedCarrier(name string, pkg *packages.Package) bool {
	for _, file := range filesUnder(c.loaded.root, pkg) {
		for _, decl := range file.syntax.Decls {
			if group, ok := decl.(*ast.GenDecl); ok {
				for _, item := range group.Specs {
					if spec, ok := item.(*ast.TypeSpec); ok && spec.Name.Name == name && carrierDeclarationModel(file.syntax, typeDoc(group, spec), spec) != "" {
						return true
					}
				}
			}
		}
	}
	return false
}

// recordLiteral writes promoted native fields through an initialized value;
// promoted fields cannot be keyed directly in a Go composite literal.
func (c *contractCompiler) recordLiteral(value *syntax.BraceValue, t string, named *types.Named, pkg *packages.Package) (string, error) {
	var body strings.Builder
	if strings.HasPrefix(t, "*") {
		body.WriteString("value:=new(" + strings.TrimPrefix(t, "*") + ");")
	} else {
		body.WriteString("var value " + t + ";")
	}
	for _, entry := range value.Entries {
		if entry.Key.Name == nil {
			return "", fmt.Errorf("a record field needs a model name")
		}
		name := upperFirst(string(*entry.Key.Name))
		field, index, ok := c.nativeFieldIndex(named, c.modelForNative(named), string(*entry.Key.Name), pkg)
		if !ok || len(index) == 0 {
			return "", fmt.Errorf("record field %s is unavailable or ambiguous", name)
		}
		current := types.Type(named)
		path := "value"
		for position, part := range index {
			if pointer, ok := types.Unalias(current).(*types.Pointer); ok {
				current = pointer.Elem()
			}
			structure, ok := types.Unalias(current).Underlying().(*types.Struct)
			if !ok {
				return "", fmt.Errorf("the promoted field %s has no native struct path", name)
			}
			member := structure.Field(part)
			path += "." + member.Name()
			current = member.Type()
			if position < len(index)-1 {
				if pointer, ok := types.Unalias(current).(*types.Pointer); ok {
					fmt.Fprintf(&body, "if %s==nil {%s=new(%s)};", path, path, c.nativeType(pointer.Elem(), pkg, c.literalImports))
				}
			}
		}
		literal, err := c.valueLiteral(entry.Value, c.nativeType(field.Type(), pkg, c.literalImports), pkg)
		if err != nil {
			return "", err
		}
		body.WriteString(path + "=" + literal + ";")
	}
	return "func() " + t + " {" + body.String() + "return value}()", nil
}

// modelParents are the types id extends, in the order the design declares
// them, from an index of all extends edges built once per compiler: schemas
// ask for the parents of every type in every contract.
func (c *contractCompiler) modelParents(id string) []string {
	edges := c.graphEdges()
	if c.extends == nil {
		c.extends = map[string][]string{}
		var extends []graph.Edge
		for _, edge := range edges {
			if edge.Kind == graph.EdgeKindExtends {
				extends = append(extends, edge)
			}
		}
		slices.SortStableFunc(extends, func(a, b graph.Edge) int {
			if a.Origin.Path != b.Origin.Path {
				return strings.Compare(a.Origin.Path, b.Origin.Path)
			}
			if a.Origin.Line != b.Origin.Line {
				return a.Origin.Line - b.Origin.Line
			}
			if a.Origin.Column != b.Origin.Column {
				return a.Origin.Column - b.Origin.Column
			}
			return strings.Compare(a.To, b.To)
		})
		for _, edge := range extends {
			c.extends[edge.From] = append(c.extends[edge.From], edge.To)
		}
	}
	return slices.Clone(c.extends[id])
}

// modelInterfaces are the interfaces id declares that it implements, in
// identity order.
func (c *contractCompiler) modelInterfaces(id string) []string {
	var out []string
	for _, edge := range c.graphEdges() {
		if edge.Kind == graph.EdgeKindImplements && edge.From == id && c.nodes[edge.To].Layer == graph.LayerDesign {
			out = append(out, edge.To)
		}
	}
	slices.Sort(out)
	return out
}

func (c *contractCompiler) visibleOperationIDs(id string) []string {
	var out []string
	seenTypes, seenNames := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(current string) {
		if seenTypes[current] {
			return
		}
		seenTypes[current] = true
		for _, node := range c.plan.Graph.Nodes() {
			if node.Kind == graph.NodeKindOp && c.parents[node.Id] == current && !seenNames[lastName(node.Id)] {
				seenNames[lastName(node.Id)] = true
				out = append(out, node.Id)
			}
		}
		for _, parent := range c.modelParents(current) {
			visit(parent)
		}
	}
	visit(id)
	// The interfaces the type and its bases implement come last, as in
	// the design (docs/language.md §4.4).
	implementing, seen := []string{id}, map[string]bool{id: true}
	for i := 0; i < len(implementing); i++ {
		for _, parent := range c.modelParents(implementing[i]) {
			if !seen[parent] {
				seen[parent] = true
				implementing = append(implementing, parent)
			}
		}
	}
	for _, current := range implementing {
		for _, iface := range c.modelInterfaces(current) {
			visit(iface)
		}
	}
	return out
}

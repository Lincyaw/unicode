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
				for _, op := range unit.spec.Operations {
					if op.Owner == target {
						chosen[op.Name] = true
					}
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
		for _, receiver := range unit.spec.Symbols {
			if receiver.Type == calls[0].Owner {
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
		if c.nodes[target].Kind == graph.NodeKindType {
			for _, node := range c.nodes {
				if node.Kind == graph.NodeKindOp && c.parents[node.Id] == target && lastName(node.Id) == string(call.Op) {
					params, ret, raises := ucSignature(first(node.Attrs["signature"]))
					selected = ucOperation{Name: node.Id, Result: ret, Raises: raises}
					for _, p := range params {
						selected.Params = append(selected.Params, p.Type)
						selected.ParamNames = append(selected.ParamNames, p.Name)
					}
				}
			}
		} else if lastName(target) == string(call.Op) {
			selected.Name = target
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

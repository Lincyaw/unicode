package golang

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
)

func modelNativeNamed(t types.Type) *types.Named {
	t = types.Unalias(t)
	for {
		pointer, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = types.Unalias(pointer.Elem())
	}
	named, _ := t.(*types.Named)
	return named
}
func (c *contractCompiler) nativeModelType(id string, ty ucType) ucType {
	if declaration, ok := c.bound(graph.EdgeKindRealizes, id); ok {
		if named := modelNativeNamed(declaration.obj.Type()); named != nil && named.Obj().Pkg() != nil {
			ty.NativeName = named.Obj().Name()
			ty.NativePkg = named.Obj().Pkg().Path()
		}
	}
	return ty
}
func (c *contractCompiler) modelForNative(named *types.Named) string {
	for model, ids := range c.bindings[graph.EdgeKindRealizes] {
		if len(ids) != 1 || c.nodes[model].Kind != graph.NodeKindType {
			continue
		}
		declaration, ok := c.declarations[ids[0]]
		if !ok {
			continue
		}
		other := modelNativeNamed(declaration.obj.Type())
		if sameNativeNamed(named, other) {
			return model
		}
	}
	return ""
}
func sameNativeNamed(a, b *types.Named) bool {
	return a != nil && b != nil && a.Obj().Pkg() != nil && b.Obj().Pkg() != nil && a.Obj().Name() == b.Obj().Name() && a.Obj().Pkg().Path() == b.Obj().Pkg().Path()
}
func (c *contractCompiler) modelFieldOwners(model, name string) ([]string, bool) {
	seen := map[string]bool{}
	var find func(string) ([]string, bool)
	find = func(current string) ([]string, bool) {
		if seen[current] {
			return nil, false
		}
		seen[current] = true
		for _, field := range c.nodes[current].Attrs["fields"] {
			member, _, _ := strings.Cut(field, ": ")
			if member == name {
				return []string{current}, true
			}
		}
		for _, parent := range c.modelParents(current) {
			if path, ok := find(parent); ok {
				return append([]string{current}, path...), true
			}
		}
		return nil, false
	}
	return find(model)
}
func (c *contractCompiler) nativeFieldIndex(named *types.Named, model, name string, pkg *packages.Package) (*types.Var, []int, bool) {
	if model == "" {
		object, index, _ := types.LookupFieldOrMethod(named, true, pkg.Types, upperFirst(name))
		field, ok := object.(*types.Var)
		return field, index, ok && len(index) > 0
	}
	owners, ok := c.modelFieldOwners(model, name)
	if !ok {
		return nil, nil, false
	}
	current := types.Type(named)
	var index []int
	for _, owner := range owners[1:] {
		declaration, bound := c.bound(graph.EdgeKindRealizes, owner)
		if !bound {
			return nil, nil, false
		}
		wanted := modelNativeNamed(declaration.obj.Type())
		structure, ok := modelNativeNamed(current).Underlying().(*types.Struct)
		if !ok {
			return nil, nil, false
		}
		found := false
		for i := 0; i < structure.NumFields(); i++ {
			field := structure.Field(i)
			if field.Embedded() && sameNativeNamed(modelNativeNamed(field.Type()), wanted) {
				index = append(index, i)
				current = field.Type()
				found = true
				break
			}
		}
		if !found {
			return nil, nil, false
		}
	}
	structure, ok := modelNativeNamed(current).Underlying().(*types.Struct)
	if !ok {
		return nil, nil, false
	}
	for i := 0; i < structure.NumFields(); i++ {
		field := structure.Field(i)
		if field.Name() == upperFirst(name) {
			return field, append(index, i), true
		}
	}
	return nil, nil, false
}

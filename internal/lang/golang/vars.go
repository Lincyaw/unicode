package golang

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// A function or method referred to outside a call, by its name, a method
// expression or a method value, gives a function value that may be called:
// like dispatch through an interface, the reference counts as a call
// (docs/design.md §4). Calls and references belong to the functions that can
// run them. Those in a body, its function literals included, are its
// function's. Those in the value of a package-level variable are run by
// whoever uses the variable, so they belong to every function whose body
// uses the variable, and through the variables that value uses in turn.
// Variables are no nodes (docs/model.md §3.1), so no other node could own
// them. A call in the value outside any literal runs when the package is
// initialized, and neither it nor the function it calls belongs to anyone.

// initializer is an expression a package-level variable is declared with,
// and the package whose types it reads.
type initializer struct {
	pkg  *packages.Package
	expr ast.Expr
}

// varUse is what a body, or the value of a package-level variable, gives the
// functions that run it: its calls and references, as calls, and the
// package-level variables it uses.
type varUse struct {
	calls []call
	vars  []string
}

// addInitializers records the values of a declaration of package-level
// variables. Where one expression gives several variables their values, as
// a call with several results does, each has the whole expression.
func (x *extraction) addInitializers(p *packages.Package, spec *ast.ValueSpec) {
	for i, ident := range spec.Names {
		v, ok := p.TypesInfo.Defs[ident].(*types.Var)
		if !ok || ident.Name == "_" {
			continue
		}
		values := spec.Values
		if len(values) == len(spec.Names) {
			values = values[i : i+1]
		}
		for _, e := range values {
			x.vars[varID(v)] = append(x.vars[varID(v)], initializer{pkg: p, expr: e})
		}
	}
}

// addClosureCalls adds to a function the calls of the variables its body
// uses, and of the variables those use, each call once.
func (x *extraction) addClosureCalls(from string, vars []string) {
	seen := map[string]bool{}
	added := map[call]bool{}
	for len(vars) > 0 {
		id := vars[len(vars)-1]
		vars = vars[:len(vars)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		use := x.varUse(id)
		for _, c := range use.calls {
			if !added[c] {
				added[c] = true
				x.addCall(from, c)
			}
		}
		vars = append(vars, use.vars...)
	}
}

// varUse reads a variable's values once: the variables they use anywhere,
// the calls and references inside their function literals, and the functions
// and methods they refer to outside them, other than those called there.
func (x *extraction) varUse(id string) varUse {
	if use, ok := x.varUses[id]; ok {
		return use
	}
	var use varUse
	for _, init := range x.vars[id] {
		info := init.pkg.TypesInfo
		called := map[*ast.Ident]bool{}
		ast.Inspect(init.expr, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				x.bodyUse(&use, init.pkg, n.Body)
				return false
			case *ast.CallExpr:
				called[calleeName(n.Fun)] = true
			case *ast.Ident:
				if v, ok := packageVar(info, n); ok {
					use.vars = append(use.vars, v)
				} else if callee, ok := referredFunc(info, n); ok && !called[n] {
					use.calls = append(use.calls, call{callee: callee, site: x.position(init.pkg, n.Pos())})
				}
			}
			return true
		})
	}
	x.varUses[id] = use
	return use
}

// bodyUse adds what a function's body, or a function literal's, gives the
// functions that run it: the package-level variables it uses, its static
// calls, and the functions and methods it refers to outside a call.
func (x *extraction) bodyUse(use *varUse, p *packages.Package, body *ast.BlockStmt) {
	info := p.TypesInfo
	called := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			// The walk reaches a call before the name it calls.
			called[calleeName(n.Fun)] = true
			if callee, name, ok := staticCall(info, n); ok {
				use.calls = append(use.calls, call{callee: callee, site: x.position(p, name.Pos())})
			}
		case *ast.Ident:
			if v, ok := packageVar(info, n); ok {
				use.vars = append(use.vars, v)
			} else if callee, ok := referredFunc(info, n); ok && !called[n] {
				use.calls = append(use.calls, call{callee: callee, site: x.position(p, n.Pos())})
			}
		}
		return true
	})
}

// referredFunc is the identity of the package-level function or method an
// identifier refers to, if it refers to one: the name of a function, or the
// method's name in a method expression or a method value or call.
func referredFunc(info *types.Info, n *ast.Ident) (string, bool) {
	fn, ok := info.Uses[n].(*types.Func)
	if !ok || !packageLevel(fn) {
		return "", false
	}
	return funcID(fn.Origin()), true
}

// packageVar is the identity of the package-level variable an identifier
// uses, if it uses one.
func packageVar(info *types.Info, n ast.Node) (string, bool) {
	ident, ok := n.(*ast.Ident)
	if !ok {
		return "", false
	}
	v, ok := info.Uses[ident].(*types.Var)
	if !ok || v.Pkg() == nil || v.Parent() != v.Pkg().Scope() {
		return "", false
	}
	return varID(v), true
}

func varID(v *types.Var) string { return "go:" + v.Pkg().Path() + "." + v.Name() }

package golang

import (
	"go/token"
	"go/types"
	"strings"
)

// The type mapping of docs/design.md §4, read from Go to the design. A type
// that realizes a design type is written as the design's identity, any other
// named type as its code identity; a pointer is always an optional, and a
// trailing error result is written `raises error`, since Go names neither
// whether a pointer is to an object nor which errors a function returns.

var basicTypes = map[types.BasicKind]string{
	types.Bool:    "bool",
	types.Int:     "int",
	types.Float64: "float",
	types.String:  "string",
}

var errorType = types.Universe.Lookup("error").Type()

// designType writes a Go type in the design's notation.
func (x *extraction) designType(t types.Type) string {
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		if name, ok := basicTypes[t.Kind()]; ok {
			return name
		}
		return t.Name()
	case *types.Named:
		return x.namedType(t)
	case *types.Pointer:
		return "optional<" + x.designType(t.Elem()) + ">"
	case *types.Slice:
		if elem, ok := types.Unalias(t.Elem()).(*types.Basic); ok && elem.Kind() == types.Byte {
			return "bytes"
		}
		return "list<" + x.designType(t.Elem()) + ">"
	case *types.Map:
		if elem, ok := types.Unalias(t.Elem()).(*types.Struct); ok && elem.NumFields() == 0 {
			return "set<" + x.designType(t.Key()) + ">"
		}
		return "map<" + x.designType(t.Key()) + ", " + x.designType(t.Elem()) + ">"
	case *types.TypeParam:
		return t.Obj().Name()
	}
	return types.TypeString(t, func(p *types.Package) string { return "go:" + p.Path() })
}

func (x *extraction) namedType(t *types.Named) string {
	obj := t.Obj()
	switch {
	case obj.Pkg() == nil:
		return obj.Name()
	case obj.Pkg().Path() == "time" && obj.Name() == "Time":
		return "time"
	case obj.Pkg().Path() == "time" && obj.Name() == "Duration":
		return "duration"
	}
	id := typeID(t.Origin().Obj())
	if targets := x.realizes[id]; len(targets) > 0 {
		id = targets[0]
	}
	if t.TypeArgs().Len() == 0 {
		return id
	}
	var args []string
	for arg := range t.TypeArgs().Types() {
		args = append(args, x.designType(arg))
	}
	return id + "[" + strings.Join(args, ", ") + "]"
}

// signature writes a function's parameters and results as the design writes
// an operation's: `(name: type, …) -> type raises error`.
func (x *extraction) signature(sig *types.Signature) string {
	var b strings.Builder
	b.WriteString("(")
	for i := range sig.Params().Len() {
		if i > 0 {
			b.WriteString(", ")
		}
		v := sig.Params().At(i)
		name := v.Name()
		if name == "" {
			name = "_"
		}
		b.WriteString(name + ": " + x.designType(v.Type()))
	}
	b.WriteString(")")
	var results []string
	raises := false
	for i := range sig.Results().Len() {
		t := sig.Results().At(i).Type()
		if i == sig.Results().Len()-1 && types.Identical(t, errorType) {
			raises = true
		} else {
			results = append(results, x.designType(t))
		}
	}
	switch len(results) {
	case 0:
	case 1:
		b.WriteString(" -> " + results[0])
	default:
		b.WriteString(" -> (" + strings.Join(results, ", ") + ")")
	}
	if raises {
		b.WriteString(" raises error")
	}
	return b.String()
}

// addShapes adds the attrs that are written in the design's notation, once
// every binding is known: a function's signature, a struct's exported fields
// and an interface's exported methods. Unexported names map back to no
// design name, so they are left out.
func (x *extraction) addShapes() {
	for _, d := range x.declared {
		n := x.nodes[d.id]
		switch {
		case d.fn != nil:
			n.Attrs["signature"] = []string{x.signature(d.fn.Signature())}
		case typeForm(d.name) == "struct":
			fields := []string{}
			for field := range d.name.Type().Underlying().(*types.Struct).Fields() {
				if !field.Embedded() && token.IsExported(field.Name()) {
					fields = append(fields, designName(field.Name())+": "+x.designType(field.Type()))
				}
			}
			n.Attrs["fields"] = fields
		case typeForm(d.name) == "interface":
			methods := []string{}
			for m := range d.name.Type().Underlying().(*types.Interface).Methods() {
				if token.IsExported(m.Name()) {
					methods = append(methods, designName(m.Name())+" "+x.signature(m.Signature()))
				}
			}
			n.Attrs["methods"] = methods
		}
	}
}

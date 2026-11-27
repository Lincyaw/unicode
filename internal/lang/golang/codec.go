package golang

import (
	"fmt"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

func (c *contractCompiler) configureCodecs(unit *contractUnit) {
	unit.spec.Codecs = map[string]ucCodec{}
	seen := map[string]bool{}
	var visitType func(string)
	var visitOperation func(string)
	visitOperation = func(id string) {
		key := "op:" + id
		if seen[key] {
			return
		}
		seen[key] = true
		op, ok := unit.spec.Operations[id]
		if !ok {
			return
		}
		if c.nodes[op.Owner].Kind == graph.NodeKindType {
			visitType(op.Owner)
		}
		visitType(op.Result)
		for _, model := range append(slices.Clone(op.Params), op.Raises...) {
			visitType(model)
		}
	}
	visitType = func(model string) {
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		kind, args := ucSplitType(model)
		for _, arg := range args {
			visitType(arg)
			if kind == "call" || kind == "replies" {
				if c.nodes[arg].Kind == graph.NodeKindType {
					for _, id := range c.visibleOperationIDs(arg) {
						visitOperation(id)
					}
				} else {
					visitOperation(arg)
				}
			}
		}
		node := c.nodes[model]
		if len(node.Attrs["codec"]) == 2 {
			encode := c.operationOf(node.Attrs["codec"][0])
			unit.spec.Codecs[model] = ucCodec{Representation: encode.Result}
			visitType(encode.Result)
		}
		ty := unit.spec.Types[model]
		for _, field := range ty.Fields {
			visitType(field.Type)
		}
		for _, parent := range ucParents(ty) {
			visitType(parent)
		}
	}
	for _, symbol := range unit.spec.Symbols {
		visitType(symbol.Type)
	}
	for _, observer := range unit.spec.Observers {
		visitType(observer.Type)
	}
	for _, edge := range c.plan.Graph.Edges() {
		if edge.From != unit.node.Id && !strings.HasPrefix(edge.From, unit.node.Id+"#") {
			continue
		}
		if edge.Kind != graph.EdgeKindRefers && edge.Kind != graph.EdgeKindAbout && edge.Kind != graph.EdgeKindCalls {
			continue
		}
		switch c.nodes[edge.To].Kind {
		case graph.NodeKindType:
			visitType(edge.To)
		case graph.NodeKindOp:
			visitOperation(edge.To)
		}
	}
}
func (c *contractCompiler) codecFunction(id string, pkg *packages.Package, imports map[string]string) (string, bool) {
	declaration, ok := c.bound(graph.EdgeKindRealizes, id)
	if !ok {
		return "", false
	}
	fn, ok := declaration.obj.(*types.Func)
	if !ok {
		return "", false
	}
	if fn.Signature().Recv() != nil {
		if fn.Pkg().Path() != pkg.PkgPath && !fn.Exported() {
			return "", false
		}
		return "(" + c.nativeType(fn.Signature().Recv().Type(), pkg, imports) + ")." + fn.Name(), true
	}
	name := c.qualify(fn, pkg, imports)
	return name, name != ""
}
func (c *contractCompiler) renderCodecs(body *strings.Builder, file *contractFile, unit *contractUnit) {
	for _, model := range slices.Sorted(maps.Keys(unit.spec.Codecs)) {
		codec := unit.spec.Codecs[model]
		ids := c.nodes[model].Attrs["codec"]
		encode, ok := c.codecFunction(ids[0], file.pkg, file.imports)
		if !ok {
			continue
		}
		decode, ok := c.codecFunction(ids[1], file.pkg, file.imports)
		if !ok {
			continue
		}
		fmt.Fprintf(body, "s.Codecs[%s]=ucCodec{Representation:%s,Encode:reflect.ValueOf(%s),Decode:reflect.ValueOf(%s)};", strconv.Quote(model), strconv.Quote(codec.Representation), encode, decode)
	}
}
func (c *contractCompiler) encodedLiteral(value *syntax.EncodedValue, t string, pkg *packages.Package) (string, error) {
	object := c.literalTypeObject(t, pkg)
	if object == nil {
		return "", fmt.Errorf("cannot resolve the codec type %s", t)
	}
	var model string
	for id, node := range c.nodes {
		if len(node.Attrs["codec"]) != 2 {
			continue
		}
		native, ok := c.bound(graph.EdgeKindRealizes, id)
		if !ok {
			continue
		}
		a, ok := types.Unalias(native.obj.Type()).(*types.Named)
		if !ok {
			continue
		}
		b, ok := types.Unalias(object.Type()).(*types.Named)
		if ok && types.Identical(a, b) {
			model = id
			break
		}
	}
	if model == "" {
		return "", fmt.Errorf("the type %s has no declared codec", t)
	}
	decodeID := c.nodes[model].Attrs["codec"][1]
	native, ok := c.bound(graph.EdgeKindRealizes, decodeID)
	if !ok {
		return "", fmt.Errorf("the codec decoder has no native realization")
	}
	fn, ok := native.obj.(*types.Func)
	if !ok || fn.Signature().Params().Len() != 1 {
		return "", fmt.Errorf("the codec decoder has no unary native signature")
	}
	decoder, ok := c.codecFunction(decodeID, pkg, c.literalImports)
	if !ok {
		return "", fmt.Errorf("the codec decoder is inaccessible")
	}
	representation := c.nativeType(fn.Signature().Params().At(0).Type(), pkg, c.literalImports)
	literal, err := c.valueLiteral(value.Value, representation, pkg)
	if err != nil {
		return "", err
	}
	return "ucEncodedLiteral(reflect.ValueOf(" + decoder + "),reflect.ValueOf(" + literal + "),reflect.TypeFor[" + t + "]()).Interface().(" + t + ")", nil
}

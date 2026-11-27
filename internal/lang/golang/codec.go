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
	for id, node := range c.nodes {
		if node.Layer != graph.LayerDesign || node.Kind != graph.NodeKindType || len(node.Attrs["codec"]) != 2 {
			continue
		}
		encode := c.operationOf(node.Attrs["codec"][0])
		unit.spec.Codecs[id] = ucCodec{Representation: encode.Result}
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

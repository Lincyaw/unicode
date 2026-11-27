package golang

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strings"
)

// Model carriers are import-free declarations retained in generation overlays.
// Their annotation is metadata only: it never adds a realization binding.
func modelCarrierName(model string) string {
	kind, _ := ucSplitType(model)
	return fmt.Sprintf("unicodeGenerated%s_%x", upperFirst(kind), sha256.Sum256([]byte(model)))
}
func modelCarrierDeclaration(model string) string {
	fields := "Operation string; Arguments map[string]any"
	if kind, _ := ucSplitType(model); kind == "replies" {
		fields = "Replies []map[string]any"
	}
	return "//uc:model-type " + model + "\ntype " + modelCarrierName(model) + " struct {" + fields + "}\n"
}
func carrierModelText(text string) string {
	if i := strings.IndexByte(text, '<'); i < 0 || len(text) <= i+2 || !strings.HasSuffix(text, ">") {
		return ""
	}
	kind, args := ucSplitType(text)
	if kind != "call" && kind != "replies" || len(args) == 0 || kind == "replies" && len(args) != 1 {
		return ""
	}
	for _, arg := range args {
		if !designRef.MatchString(arg) {
			return ""
		}
	}
	if text != kind+"<"+strings.Join(args, ", ")+">" {
		return ""
	}
	return text
}
func carrierAnnotation(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	var model string
	for _, comment := range doc.List {
		if strings.HasPrefix(comment.Text, "//uc:model-type ") {
			if model != "" {
				return ""
			}
			model = carrierModelText(strings.TrimPrefix(comment.Text, "//uc:model-type "))
			if model == "" {
				return ""
			}
		}
	}
	return model
}
func carrierDeclarationModel(file *ast.File, doc *ast.CommentGroup, spec *ast.TypeSpec) string {
	if !unicodeGenerated(file) || spec.Assign.IsValid() {
		return ""
	}
	model := carrierAnnotation(doc)
	if model == "" || spec.Name.Name != modelCarrierName(model) {
		return ""
	}
	structure, ok := spec.Type.(*ast.StructType)
	if !ok {
		return ""
	}
	var b bytes.Buffer
	if format.Node(&b, token.NewFileSet(), structure) != nil {
		return ""
	}
	expected := "struct { Operation string; Arguments map[string]any }"
	if kind, _ := ucSplitType(model); kind == "replies" {
		expected = "struct { Replies []map[string]any }"
	}
	compact := func(s string) string { return strings.Join(strings.Fields(strings.ReplaceAll(s, ";", " ")), "") }
	if compact(b.String()) != compact(expected) {
		return ""
	}
	return model
}
func retainGeneratedCarriers(file *ast.File, source []byte, set *token.FileSet) []byte {
	var out []byte
	for _, decl := range file.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.TYPE || len(group.Specs) != 1 {
			continue
		}
		spec := group.Specs[0].(*ast.TypeSpec)
		doc := typeDoc(group, spec)
		if carrierDeclarationModel(file, doc, spec) == "" {
			continue
		}
		start := group.Pos()
		if doc != nil {
			start = doc.Pos()
		}
		from, to := set.Position(start).Offset, set.Position(group.End()).Offset
		if from >= 0 && to <= len(source) {
			out = append(out, source[from:to]...)
			out = append(out, '\n')
		}
	}
	return out
}
func (x *extraction) carrierModel(t *types.Named) string {
	if t == nil || t.Obj() == nil || t.Obj().Pkg() == nil {
		return ""
	}
	id := typeID(t.Origin().Obj())
	for _, d := range x.declared {
		if d.id == id && d.spec != nil {
			for _, decl := range d.file.syntax.Decls {
				if group, ok := decl.(*ast.GenDecl); ok {
					for _, s := range group.Specs {
						if s == d.spec {
							return carrierDeclarationModel(d.file.syntax, typeDoc(group, d.spec), d.spec)
						}
					}
				}
			}
		}
	}
	return ""
}

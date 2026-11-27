package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// addContent records implementation syntax without retaining source layout or
// comments. Abstract operations have no body; a type's methods are separate ops.
func (x *extraction) addContent(d declared) {
	attrs := x.nodes[d.id].Attrs
	if d.decl != nil && d.decl.Body != nil {
		attrs["body"] = []string{normalizedContent(d.pkg.Fset, d.decl.Body)}
	}
	if d.spec != nil {
		attrs["declaration"] = []string{normalizedContent(d.pkg.Fset, d.spec)}
	}
}

// normalizedContent reparses a standalone declaration without comments so the
// loaded AST and its binding metadata remain intact. Printing the clean node
// with an empty file set removes positions, including line directives.
func normalizedContent(positions *token.FileSet, node ast.Node) string {
	var source bytes.Buffer
	if err := format.Node(&source, positions, node); err != nil {
		panic(fmt.Sprintf("golang: cannot print a loaded declaration: %v", err))
	}
	prefix := "package content\n"
	switch node.(type) {
	case *ast.BlockStmt:
		prefix += "func content() "
	case *ast.TypeSpec:
		prefix += "type "
	default:
		panic(fmt.Sprintf("golang: unsupported implementation node %T", node))
	}
	file, err := parser.ParseFile(token.NewFileSet(), "", prefix+source.String(), parser.SkipObjectResolution)
	if err != nil {
		panic(fmt.Sprintf("golang: cannot reparse a loaded declaration: %v", err))
	}
	var clean ast.Node
	switch node.(type) {
	case *ast.BlockStmt:
		clean = file.Decls[0].(*ast.FuncDecl).Body
	case *ast.TypeSpec:
		clean = file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
	}
	var canonical bytes.Buffer
	if err := format.Node(&canonical, token.NewFileSet(), clean); err != nil {
		panic(fmt.Sprintf("golang: cannot normalize a loaded declaration: %v", err))
	}
	return canonical.String()
}

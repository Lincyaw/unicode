package golang

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strings"
)

// The content that a code node adds to its content hash (docs/model.md §6,
// docs/design.md §5.2): function bodies, type definitions, native operation
// headers and file-level declarations. Each is a formatting-independent SHA-256.

var (
	posType      = reflect.TypeFor[token.Pos]()
	commentsType = reflect.TypeFor[*ast.CommentGroup]()
)

// addContent retains the native declaration of a concrete operation, including
// its receiver and type parameters. The loaded syntax remains unchanged; the
// body and type declaration digests are already recorded when nodes are added.
func (x *extraction) addContent(d declared) {
	if d.decl == nil {
		return
	}
	header := *d.decl
	header.Body = nil
	x.nodes[d.id].Attrs["declaration"] = []string{digest(&header)}
}

// bodyDigest is the digest of a function's body.
func bodyDigest(body *ast.BlockStmt) string { return digest(body) }

// declarationDigest is the digest of what a type declares: its type
// parameters and its definition.
func declarationDigest(spec *ast.TypeSpec) string { return digest(spec.TypeParams, spec.Type) }

// globalsDigest is the digest of a file's top-level declarations that are no
// nodes, in the order of the source: each declaration of variables,
// constants or type aliases with those specs, and each init function or
// function named _ whole; "" when it has none.
func globalsDigest(file *ast.File) string {
	var items []any
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.GenDecl:
			if specs := looseSpecs(decl); len(specs) > 0 {
				items = append(items, decl.Tok.String(), specs)
			}
		case *ast.FuncDecl:
			if !funcIsNode(decl) {
				items = append(items, decl)
			}
		}
	}
	if len(items) == 0 {
		return ""
	}
	return digest(items...)
}

// looseSpecs are the specs of a declaration that are no nodes: all of a
// declaration of variables or constants, and a type declaration's aliases.
func looseSpecs(decl *ast.GenDecl) []ast.Spec {
	switch decl.Tok {
	case token.VAR, token.CONST:
		return decl.Specs
	case token.TYPE:
		var aliases []ast.Spec
		for _, spec := range decl.Specs {
			if !typeIsNode(spec.(*ast.TypeSpec)) {
				aliases = append(aliases, spec)
			}
		}
		return aliases
	}
	return nil
}

func digest(nodes ...any) string {
	var b strings.Builder
	for _, n := range nodes {
		normalForm(&b, reflect.ValueOf(n))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// normalForm writes a syntax tree, parsed without object resolution
// (parseFile), as only its structure decides: every node's type and fields
// in order, of a position only whether it is present, and no comments.
// Neither printer of the standard library does this: go/printer prints the
// comments attached to nodes, and ast.Fprint cannot keep a position's
// presence, which tells f(xs...) from f(xs).
func normalForm(b *strings.Builder, v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			b.WriteString("nil;")
			return
		}
		normalForm(b, v.Elem())
	case reflect.Slice:
		fmt.Fprintf(b, "[%d;", v.Len())
		for i := range v.Len() {
			normalForm(b, v.Index(i))
		}
	case reflect.Struct:
		b.WriteString(v.Type().Name() + "{")
		for i := range v.NumField() {
			switch field := v.Field(i); field.Type() {
			case posType:
				fmt.Fprintf(b, "%t;", token.Pos(field.Int()).IsValid())
			case commentsType:
			default:
				normalForm(b, field)
			}
		}
		b.WriteString("}")
	case reflect.String:
		fmt.Fprintf(b, "%q;", v.String())
	case reflect.Bool:
		fmt.Fprintf(b, "%t;", v.Bool())
	default:
		fmt.Fprintf(b, "%d;", v.Int())
	}
}

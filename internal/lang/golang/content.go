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
// docs/design.md §5.2): a function's body, a type's declaration without its
// name, and a file's declarations of package-level variables, constants and
// init functions, which are no nodes. Each is the SHA-256 of a normal form
// that formatting and comments do not change.

var (
	posType      = reflect.TypeFor[token.Pos]()
	commentsType = reflect.TypeFor[*ast.CommentGroup]()
)

// bodyDigest is the digest of a function's body.
func bodyDigest(body *ast.BlockStmt) string { return digest(body) }

// declarationDigest is the digest of what a type declares: its type
// parameters and its definition.
func declarationDigest(spec *ast.TypeSpec) string { return digest(spec.TypeParams, spec.Type) }

// globalsDigest is the digest of a file's declarations of package-level
// variables and constants, each with its specs, and of its init functions'
// bodies, in the order of the source; "" when it has none.
func globalsDigest(file *ast.File) string {
	var items []any
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.GenDecl:
			if decl.Tok == token.VAR || decl.Tok == token.CONST {
				items = append(items, decl.Tok.String(), decl.Specs)
			}
		case *ast.FuncDecl:
			if decl.Recv == nil && decl.Name.Name == "init" {
				items = append(items, "init", decl.Body)
			}
		}
	}
	if len(items) == 0 {
		return ""
	}
	return digest(items...)
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

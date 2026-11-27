package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// Lines are adjacent lines of a file, From to To, both counted from 1 and
// both included.
//
//uc:realizes lang.golang.Lines
type Lines struct {
	From int
	To   int
}

// Declaration returns the lines of the top-level declaration of the Go
// source src that holds the position line:column, from its doc comment on;
// in a grouped type declaration, of the type that holds it. It returns nil
// when src does not parse or no declaration holds the position.
//
//uc:realizes lang.golang.declaration
func Declaration(src []byte, line, column int) *Lines {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil
	}
	tf := fset.File(f.Pos())
	if line < 1 || line > tf.LineCount() || column < 1 {
		return nil
	}
	pos := tf.LineStart(line) + token.Pos(column-1)
	lines := func(doc *ast.CommentGroup, from, to token.Pos) *Lines {
		if doc != nil {
			from = doc.Pos()
		}
		return &Lines{From: tf.Line(from), To: tf.Line(to)}
	}
	for _, d := range f.Decls {
		if pos < d.Pos() || pos >= d.End() {
			continue
		}
		switch d := d.(type) {
		case *ast.FuncDecl:
			return lines(d.Doc, d.Pos(), d.End())
		case *ast.GenDecl:
			if d.Lparen.IsValid() {
				for _, s := range d.Specs {
					if s, ok := s.(*ast.TypeSpec); ok && pos >= s.Pos() && pos < s.End() {
						return lines(s.Doc, s.Pos(), s.End())
					}
				}
			}
			return lines(d.Doc, d.Pos(), d.End())
		}
	}
	return nil
}

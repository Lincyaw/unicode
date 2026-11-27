package golang

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
	panic("unicode: unimplemented")
}

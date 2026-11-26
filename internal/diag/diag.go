package diag

// Position is a place in a file. Line and Column start at 1; Column counts bytes.
//
//uc:realizes diag.Position
type Position struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Diagnostic is one violation of a constraint. Code is stable and written
// `<component>.<kebab-name>`; Message states the cause and Fix the remedy.
//
//uc:realizes diag.Diagnostic
type Diagnostic struct {
	Code     string
	Subject  string
	Position Position
	Message  string
	Fix      string
}

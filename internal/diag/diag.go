package diag

import "encoding/json"

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

// MarshalJSON writes a diagnostic with its source position as flat fields.
func (d Diagnostic) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Code    string `json:"code"`
		Subject string `json:"subject"`
		Path    string `json:"path"`
		Line    int    `json:"line"`
		Column  int    `json:"column"`
		Message string `json:"message"`
		Fix     string `json:"fix"`
	}{d.Code, d.Subject, d.Position.Path, d.Position.Line, d.Position.Column, d.Message, d.Fix})
}

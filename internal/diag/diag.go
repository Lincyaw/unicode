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

// diagnosticJSON is the public wire representation, with its position flattened.
type diagnosticJSON struct {
	Code    string `json:"code"`
	Subject string `json:"subject"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// MarshalJSON writes a diagnostic with its source position as flat fields.
func (d Diagnostic) MarshalJSON() ([]byte, error) {
	return json.Marshal(diagnosticJSON{d.Code, d.Subject, d.Position.Path, d.Position.Line, d.Position.Column, d.Message, d.Fix})
}

// UnmarshalJSON restores a diagnostic's position from its flat wire fields.
func (d *Diagnostic) UnmarshalJSON(src []byte) error {
	var encoded diagnosticJSON
	if err := json.Unmarshal(src, &encoded); err != nil {
		return err
	}
	*d = Diagnostic{Code: encoded.Code, Subject: encoded.Subject, Position: Position{Path: encoded.Path, Line: encoded.Line, Column: encoded.Column}, Message: encoded.Message, Fix: encoded.Fix}
	return nil
}

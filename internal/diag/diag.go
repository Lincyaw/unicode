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

// Flat is the wire form of a diagnostic, with its position's path, line and
// column as fields of their own. Its JSON keys are its field names in lower
// camel case, which golangci-lint's tagliatelle keeps so.
//
//uc:realizes diag.Flat
type Flat struct {
	Code    string `json:"code"`
	Subject string `json:"subject"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// Flatten gives the wire form of a diagnostic.
//
//uc:realizes diag.flatten
func Flatten(diagnostic Diagnostic) Flat {
	return Flat{
		Code: diagnostic.Code, Subject: diagnostic.Subject,
		Path: diagnostic.Position.Path, Line: diagnostic.Position.Line, Column: diagnostic.Position.Column,
		Message: diagnostic.Message, Fix: diagnostic.Fix,
	}
}

// Unflatten restores a diagnostic from its wire form.
//
//uc:realizes diag.unflatten
func Unflatten(flat Flat) Diagnostic {
	return Diagnostic{
		Code: flat.Code, Subject: flat.Subject,
		Position: Position{Path: flat.Path, Line: flat.Line, Column: flat.Column},
		Message:  flat.Message, Fix: flat.Fix,
	}
}

// MarshalJSON writes a diagnostic in its wire form.
func (d Diagnostic) MarshalJSON() ([]byte, error) {
	return json.Marshal(Flatten(d))
}

// UnmarshalJSON restores a diagnostic from its wire form.
func (d *Diagnostic) UnmarshalJSON(src []byte) error {
	var flat Flat
	if err := json.Unmarshal(src, &flat); err != nil {
		return err
	}
	*d = Unflatten(flat)
	return nil
}

package diag

import (
	"encoding/json"
	"reflect"
	"testing"

	"pgregory.net/rapid"
)

//uc:checks diag.diagnostic-json-is-flat
func checkDiagnosticJSONIsFlat(t rapid.TB, diagnostic Diagnostic) {
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatalf("cannot encode a diagnostic: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("a diagnostic did not encode as an object: %v", err)
	}
	want := map[string]any{"code": diagnostic.Code, "subject": diagnostic.Subject, "path": diagnostic.Position.Path, "line": diagnostic.Position.Line, "column": diagnostic.Position.Column, "message": diagnostic.Message, "fix": diagnostic.Fix}
	if len(fields) != len(want) {
		t.Fatalf("diagnostic JSON has %d fields, want %d: %s", len(fields), len(want), encoded)
	}
	for key, value := range want {
		got, exists := fields[key]
		if !exists {
			t.Fatalf("diagnostic JSON omits %s", key)
		}
		valueJSON, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, json.RawMessage(valueJSON)) {
			t.Fatalf("diagnostic JSON field %s = %s, want %s", key, got, valueJSON)
		}
	}
}

func TestDiagnosticJSONIsFlat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkDiagnosticJSONIsFlat(t, rapid.Make[Diagnostic]().Draw(t, "diagnostic")) })
}
func FuzzDiagnosticJSONIsFlat(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) { checkDiagnosticJSONIsFlat(t, rapid.Make[Diagnostic]().Draw(t, "diagnostic")) }))
}

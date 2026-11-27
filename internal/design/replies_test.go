package design

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"pgregory.net/rapid"
)

//uc:realizes design.ReplySample
type ReplySample struct {
	Count   int
	Variant int
}

//uc:generates design.reply-scripts-are-typed.sample
func genReplySample() *rapid.Generator[ReplySample] {
	return rapid.Custom(func(t *rapid.T) ReplySample {
		return ReplySample{Count: rapid.IntRange(1, 5).Draw(t, "count"), Variant: rapid.IntRange(0, 12).Draw(t, "variant")}
	})
}

//uc:checks design.reply-scripts-are-typed
func checkReplyScriptsAreTyped(t rapid.TB, sample ReplySample) {
	count := max(1, sample.Count)
	variant := sample.Variant % 13
	var responses []string
	for i := 0; i < count; i++ {
		responses = append(responses, fmt.Sprintf("{returns: %d}", i+count))
	}
	script := "[" + strings.Join(responses, ", ") + "]"
	symbol := "replies<gateway.charge>"
	extras := ""
	assignments := ""
	holder := "{charge: [{returns: 1}], notify: [{returns: none}]}"
	operation := ""
	code := "design.value-type-mismatch"
	switch variant {
	case 0:
		code = ""
	case 1:
		symbol = "replies<app.local>"
		code = "design.invalid-replies-type"
	case 2:
		script = "[]"
	case 3:
		script = "[{returns: \"wrong\"}]"
	case 4:
		script = "[{raises: \"gateway.Unknown\", fields: {}}]"
	case 5:
		script = "[{raises: \"gateway.Failed\", fields: {}}]"
	case 6:
		extras = "  forall duplicate: replies<gateway.charge>\n"
		assignments = "    duplicate = " + script + "\n"
		code = "design.invalid-replies-type"
	case 7:
		symbol = "list<replies<gateway.charge>>"
		script = "[" + script + "]"
		code = "design.invalid-replies-type"
	case 8:
		operation = "op bad(answers: replies<gateway.charge>)\n\n"
		code = "design.invalid-replies-type"
	case 9:
		holder = "{charge: [{returns: 1}], notify: [{returns: 2}]}"
	case 10:
		holder = "{charge: [{returns: 1}]}"
	case 11:
		script = fmt.Sprintf("[{raises: \"gateway.Child\", fields: {code: %d, reason: \"child\"}}]", count)
		code = ""
	case 12:
		script = "[{returns: 1, raises: \"gateway.Failed\", fields: {code: 1}}]"
	}
	files := map[string]string{
		"design/system.uc": `system validation "Response validation"

actor gateway external "Gateway" {
  type Failed error "Failure" {
    field code: int
  }
  type Child error "Failure subtype" {
    extends Failed
    field reason: string
  }
  op charge(amount: int) -> int raises Failed
}
`,
		"design/app.uc": fmt.Sprintf(`component app "Response validation" {
  focus core
  paths "src/**"
  uses gateway
}

type Gateway interface "Gateway interface" {
  op charge(amount: int) -> int
  op notify()
}

type Holder record "Interface holder" {
  field gateway: Gateway
}

op local()

%sproperty replies invariant "Response schema" {
  about gateway.charge
  forall answers: %s
  forall holder: Holder
%s  ensure true
  case sample {
    answers = %s
    holder = {gateway: %s}
%s  }
}
`, operation, symbol, extras, script, holder, assignments),
	}
	loaded := Load(parseAll(t, files, slices.Sorted(maps.Keys(files))))
	wellFormed(t, loaded.Diagnostics, files)
	if code == "" {
		if len(loaded.Diagnostics) > 0 {
			t.Fatalf("valid reply variant %d was rejected: %+v", variant, loaded.Diagnostics)
		}
	} else {
		found := false
		for _, d := range loaded.Diagnostics {
			found = found || d.Code == code
		}
		if !found {
			t.Fatalf("reply variant %d must report %s: %+v", variant, code, loaded.Diagnostics)
		}
	}
}

package design

import (
	"fmt"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/syntax"
)

// replyFiles writes a design whose property holds a reply script of count
// replies for an external operation and an interface field, changed by
// variant: 0 and 11 are valid, every other variant breaks one rule of
// docs/model.md §10 rules 23 and 26 and comes with the code it must get.
func replyFiles(count, variant int) (map[string]string, string) {
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
	return files, code
}

// replyNames are the names of replyFiles and the names each may take instead.
var replyNames = map[string][]string{
	"validation": {"validation", "checks", "replay"},
	"gateway":    {"gateway", "bank", "payer"},
	"Failed":     {"Failed", "Declined", "Faulted"},
	"Child":      {"Child", "Narrow", "Specific"},
	"charge":     {"charge", "debit", "collect"},
	"app":        {"app", "shop", "store"},
	"Gateway":    {"Gateway", "Port", "Channel"},
	"Holder":     {"Holder", "Carrier", "Keeper"},
	"local":      {"local", "inner", "own"},
	"notify":     {"notify", "signal", "alert"},
	"answers":    {"answers", "script", "responses"},
	"holder":     {"holder", "carrier", "keeper"},
	"duplicate":  {"duplicate", "again", "second"},
	"sample":     {"sample", "given", "fixed"},
	"code":       {"code", "status", "number"},
	"reason":     {"reason", "detail", "cause"},
}

// validReplyVariants are the variants of replyFiles that load without
// diagnostics, and invalidReplyVariants the others.
var (
	validReplyVariants   = []int{0, 11}
	invalidReplyVariants = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12}
)

// genValidReplies draws the design of replyFiles with one to five replies,
// valid or with a reply of the error's subtype, under drawn names.
//
//uc:generates design.valid-reply-scripts-load.sources
func genValidReplies() *rapid.Generator[[]syntax.Source] {
	return rapid.Custom(func(t *rapid.T) []syntax.Source {
		files, _ := replyFiles(rapid.IntRange(1, 5).Draw(t, "count"), rapid.SampledFrom(validReplyVariants).Draw(t, "variant"))
		return sourcesOf(drawRenaming(t, replyNames).files(files))
	})
}

// genBrokenReplies draws the design of replyFiles with one to five replies,
// broken by one of its eleven invalid variants, under drawn names.
//
//uc:generates design.reply-scripts-are-typed.broken
func genBrokenReplies() *rapid.Generator[Broken] {
	return rapid.Custom(func(t *rapid.T) Broken {
		files, code := replyFiles(rapid.IntRange(1, 5).Draw(t, "count"), rapid.SampledFrom(invalidReplyVariants).Draw(t, "variant"))
		return drawRenaming(t, replyNames).broken(brokenFiles{files: files, codes: []string{code}, path: "design/app.uc"}).broken()
	})
}

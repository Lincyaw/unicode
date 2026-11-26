package syntax

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/diag"
)

// Parsed is the result of Parse. File is set exactly when there are no diagnostics.
//
//uc:realizes syntax.Parsed
type Parsed struct {
	File        *File
	Diagnostics []diag.Diagnostic
}

// Parse parses one file with the lexis and grammar of docs/format.md §2–§3.
// Every forbidden character gets a diagnostic; a file that does not parse
// gets one, the first problem found; a file that parses gets one for each
// rule of check.go it breaks.
//
//uc:realizes syntax.parse
func Parse(path string, kind FileKind, src []byte) Parsed {
	at := locator{path: path, src: src}
	if diags := forbidden(at); len(diags) > 0 {
		return Parsed{Diagnostics: diags}
	}
	text := src
	if !bytes.HasSuffix(text, []byte("\n")) {
		// The grammar ends every line with NL; a missing final newline is only layout.
		text = append(bytes.Clone(src), '\n')
	}
	f, err := ucParser.ParseBytes(path, text)
	if err != nil {
		return Parsed{Diagnostics: []diag.Diagnostic{parseError(at, err)}}
	}
	f.Path, f.Kind = path, kind
	if diags := check(at, f); len(diags) > 0 {
		return Parsed{Diagnostics: diags}
	}
	return Parsed{File: f}
}

// locator turns byte offsets of one file into diagnostics.
type locator struct {
	path string
	src  []byte
}

func (l locator) diagnostic(offset int, code, message, fix string) diag.Diagnostic {
	offset = min(max(offset, 0), len(l.src))
	lineStart := bytes.LastIndexByte(l.src[:offset], '\n') + 1
	return diag.Diagnostic{
		Code:    code,
		Subject: l.path,
		Position: diag.Position{
			Path:   l.path,
			Line:   bytes.Count(l.src[:offset], []byte{'\n'}) + 1,
			Column: offset - lineStart + 1,
		},
		Message: message,
		Fix:     fix,
	}
}

// parseError turns what participle returned into a diagnostic.
func parseError(at locator, err error) diag.Diagnostic {
	var te *tokenError
	if errors.As(err, &te) {
		return at.diagnostic(te.offset, te.code, te.message, te.fix)
	}
	var lexErr *lexer.Error
	if errors.As(err, &lexErr) {
		off := lexErr.Pos.Offset
		if off < len(at.src) && at.src[off] == '"' {
			return at.diagnostic(off, "syntax.unterminated-string", "the string does not end on its line",
				`close the string with "; inside a string, write a newline as \n`)
		}
		r, _ := utf8.DecodeRune(at.src[min(off, len(at.src)):])
		return at.diagnostic(off, "syntax.unexpected-character", fmt.Sprintf("the character %q may not appear outside strings and comments", r),
			"remove it, or put it inside a string or a comment")
	}
	var unexpected *participle.UnexpectedTokenError
	if errors.As(err, &unexpected) {
		return at.unexpected(unexpected)
	}
	// Participle's other errors (a capture that fails, too many repetitions)
	// cannot arise from this grammar; one is still reported where it stands.
	var perr participle.Error
	if errors.As(err, &perr) {
		return at.diagnostic(perr.Position().Offset, "syntax.unexpected-token", perr.Message(), "rewrite this line by the grammar of docs/format.md §3")
	}
	return at.diagnostic(0, "syntax.unexpected-token", err.Error(), "rewrite the file by the grammar of docs/format.md §3")
}

// unexpected reports a token the grammar did not expect. Participle's message
// names the token and its expectation; the fix gives the form of the line's
// construct, named by the line's first keyword. Participle leaves the
// expectation out of UnexpectedTokenError's Expect field and writes it only
// into the message.
func (l locator) unexpected(u *participle.UnexpectedTokenError) diag.Diagnostic {
	t := u.Unexpected
	off := t.Pos.Offset
	if t.EOF() {
		return l.diagnostic(off, "syntax.unexpected-end", "the file ends unexpectedly: its last line is unfinished, or a block lacks its }",
			"finish the last line, and close every { with a } on a line of its own")
	}
	line := l.line(off)
	key := line.construct(l.inValue(off))
	first := len(line) > 0 && line[0].Pos.Offset == off
	value := key == "assignment" || key == "literal" || key == "where"
	switch {
	case t.Type == symbols["Comment"] && first:
		return l.diagnostic(off, "syntax.comment-in-value", "a value may not contain a comment", "move the comment above the assignment")
	case t.Type == symbols["Comment"]:
		return l.diagnostic(off, "syntax.trailing-comment", "a comment must be on a line of its own", "move the comment onto its own line, above what it describes")
	case key == "else" && l.closesBranch(line[0].Pos.Offset):
		return l.diagnostic(line[0].Pos.Offset, "syntax.misplaced-else", "else must follow the previous branch's } on the same line",
			`join this line to the } above it: } else "label" {`)
	case t.Value == "}" && !first && !value:
		return l.diagnostic(off, "syntax.brace-position", "} must start its line", "move the } to the start of the next line")
	case l.after(off) == '{' && !value:
		return l.diagnostic(off, "syntax.brace-position", "{ must end its line", "move what follows the { to the next line")
	case key == "close" && first:
		return l.diagnostic(off, "syntax.unexpected-token", "this } closes no block: a value before it is unfinished, or the } is extra",
			"complete the value before it so that every [, ( and { is closed, or remove the }")
	}
	c := constructs[key]
	switch {
	case first && key == "unknown":
		return l.diagnostic(off, "syntax.unexpected-token", u.Message(),
			"start the line with a keyword such as type, op, flow, property or case; a step is written sender -> op(args)")
	case first && key != "literal":
		return l.diagnostic(off, "syntax.unexpected-token", u.Message(), "a "+c.name+" belongs "+c.place)
	}
	return l.diagnostic(off, "syntax.unexpected-token", u.Message(), "write the "+c.name+" as "+c.form)
}

// construct is one kind of line of docs/format.md §3: its name, how it is
// written, and where it may stand.
type construct struct{ name, form, place string }

var constructs = map[string]construct{
	"system":     {"system declaration", `system name "summary"`, "at the start of the system file"},
	"notation":   {"notation declaration", "notation ddd or notation c4", "at the top level of the system file"},
	"language":   {"language declaration", "language go { or language python {, with one root per line", "at the top level of the system file"},
	"root":       {"source root", `root "directory"`, "in a language block"},
	"actor":      {"actor declaration", `actor name person "summary", or actor name external "summary" with an optional block of types and operations`, "at the top level of the system file"},
	"component":  {"component declaration", `component path "summary" {`, "at the start of a component file"},
	"label":      {"label", "label name", "in a component block or a type's block"},
	"focus":      {"focus", "focus core, focus support or focus utility", "in a component block"},
	"paths":      {"paths line", `paths "pattern", ...`, "in a component block"},
	"uses":       {"uses line", "uses component-or-actor, ...", "in a component block"},
	"type":       {"type declaration", `type Name form "summary", optionally followed by {, where the form is object, record, interface, enum, error or event`, "at the top level of a component file or in an external actor's block"},
	"extends":    {"extends line", "extends Type", "in a type's block"},
	"implements": {"implements line", "implements Type", "in a type's block"},
	"field":      {"field", "field name: type", "in a type's block"},
	"value":      {"enum value", "value name", "in an enum's block"},
	"else":       {"branch introduced by else", `} else "label" {, on the line of the } that closes the previous branch`, "only right after a branch of an alt fragment, on the line of its }"},
	"literal":    {"value", "a number, a string, true, false, none, an enum value, [item, ...], {key: value, ...} or op(param: value, ...), broken across lines only after an opening bracket, around commas and before a closing bracket", "in an assignment"},
	"op":         {"operation declaration", `op name(param: type, ...), then optionally -> type, raises Type, ..., handles Type and "summary" in this order`, "at the top level of a component file, in a type's block or in an external actor's block"},
	"flow":       {"flow declaration", `flow name "summary" {`, "at the top level of a component file"},
	"property":   {"property declaration", `property name form "summary" {, where the form is invariant, postcondition, relation or transition`, "at the top level of a component file"},
	"about":      {"about line", "about ref, ...", "in a property's block"},
	"forall":     {"symbol", `forall name: type, followed by generated "domain" for a generated symbol`, "in a flow's or a property's block"},
	"where":      {"limit", "where path in low..high, where path in {value, ...} or where path size low..high", "in a flow's or a property's block"},
	"case":       {"case", `case name "summary" {, where the summary may be left out; a recorded case is case name coverage { or case name counterexample {`, "in a flow's or a property's block, or at the top level of a corpus file"},
	"corpus":     {"corpus declaration", "corpus property-or-flow", "at the start of a corpus file"},
	"on":         {"on block", "on Error {, right after a call", "among a flow's steps"},
	"async":      {"async block", "async op {", "among a flow's steps"},
	"alt":        {"alt fragment", `alt "label" {, with each further branch written } else "label" {`, "among a flow's steps"},
	"opt":        {"opt fragment", `opt "label" {`, "among a flow's steps"},
	"loop":       {"loop fragment", `loop "label" {`, "among a flow's steps"},
	step:         {"step", "sender -> op(args) with an optional {, sender ->> op(args), or sender emits Event(args)", "among a flow's steps"},
	"assignment": {"assignment", "symbol = value", "in a case's block"},
	"unknown":    {"line", "a declaration or member that starts with a keyword, a step (sender -> op(args)) or an assignment (symbol = value)", "in a block"},
}

// unnamed are the constructs that no keyword starts.
var unnamed = []string{step, "assignment", "literal", "unknown"}

// lineTokens are the tokens of one line, without spaces.
type lineTokens []lexer.Token

// line lexes the line that holds offset; its tokens keep their offsets in the file.
func (l locator) line(offset int) lineTokens {
	start := bytes.LastIndexByte(l.src[:min(offset, len(l.src))], '\n') + 1
	end := bytes.IndexByte(l.src[start:], '\n')
	if end < 0 {
		end = len(l.src) - start
	}
	lex, err := ucLexer.LexString("", string(l.src[start:start+end]))
	if err != nil {
		return nil
	}
	var toks lineTokens
	for {
		t, err := lex.Next()
		if err != nil || t.EOF() {
			return toks
		}
		if t.Type != symbols["Whitespace"] {
			t.Pos.Offset += start
			toks = append(toks, t)
		}
	}
}

// construct names the kind of the line: its first keyword, a misplaced else,
// a step, an assignment, a closing brace, part of a broken value, or unknown.
// A first word followed by ->, ->>, . or emits starts a step, as in
// docs/format.md §3. Any other line inside a value (inValue) is part of it.
func (ts lineTokens) construct(inValue bool) string {
	if len(ts) == 0 {
		return "unknown"
	}
	first := ts[0]
	startsStep := len(ts) > 1 && (ts[1].Type == symbols["Arrow"] || ts[1].Type == symbols["SendArrow"] ||
		ts[1].Value == "." || ts[1].Value == "emits")
	_, keyword := constructs[first.Value]
	keyword = keyword && !slices.Contains(unnamed, first.Value)
	word := first.Type == symbols["Ident"]
	switch {
	case word && first.Value == "else" && !startsStep:
		return "else"
	case word && keyword && !startsStep:
		return first.Value
	case startsStep:
		return step
	case slices.ContainsFunc(ts, func(t lexer.Token) bool { return t.Value == "=" }):
		return "assignment"
	case first.Value == "}":
		return "close"
	case inValue:
		return "literal"
	}
	return "unknown"
}

// inValue tells whether the line that holds offset lies inside a value: a
// bracket that follows an = on an earlier line is still open there.
func (l locator) inValue(offset int) bool {
	start := bytes.LastIndexByte(l.src[:min(offset, len(l.src))], '\n') + 1
	lex, err := ucLexer.LexString("", string(l.src[:start]))
	if err != nil {
		return false
	}
	var open []bool // for each open bracket, whether it belongs to a value
	assigned := false
	for {
		t, err := lex.Next()
		if err != nil || t.EOF() {
			return len(open) > 0 && open[len(open)-1]
		}
		switch t.Value {
		case "=":
			assigned = true
		case "[", "(", "{":
			open = append(open, assigned || len(open) > 0 && open[len(open)-1])
		case "]", ")", "}":
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
		if t.Type == symbols["NL"] {
			assigned = false
		}
	}
}

// closesBranch tells whether the line above the one at offset, past blank
// lines and comments, is a } that closes a branch of an alt fragment.
func (l locator) closesBranch(offset int) bool {
	depth := 0
	for end := bytes.LastIndexByte(l.src[:offset], '\n'); end >= 0; {
		start := bytes.LastIndexByte(l.src[:end], '\n') + 1
		ts := l.line(start)
		end = start - 1
		if depth == 0 {
			if len(ts) == 0 || ts[0].Type == symbols["Comment"] {
				continue
			}
			if len(ts) != 1 || ts[0].Value != "}" {
				return false
			}
			depth = 1
			continue
		}
		for i := len(ts) - 1; i >= 0; i-- {
			switch ts[i].Value {
			case "}":
				depth++
			case "{":
				depth--
			}
			if depth == 0 {
				opensBranch := len(ts) > 1 && ts[0].Value == "alt" && ts[1].Type == symbols["String"]
				return opensBranch || len(ts) > 1 && ts[0].Value == "}" && ts[1].Value == "else"
			}
		}
	}
	return false
}

// after returns the last character before offset that is not a space.
func (l locator) after(offset int) byte {
	before := bytes.TrimRight(l.src[:min(offset, len(l.src))], " ")
	if len(before) == 0 {
		return 0
	}
	return before[len(before)-1]
}

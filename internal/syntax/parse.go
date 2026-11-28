package syntax

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"regexp"
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

// Parse parses one file with the lexis and grammar of docs/language.md §2–§3.
// Every forbidden character gets a diagnostic; a file that does not parse
// gets one, the first problem found; a file that parses gets one for each
// rule of check.go it breaks. The diagnostics come ordered by line, column and code.
//
//uc:realizes syntax.parse
func Parse(path string, kind FileKind, src []byte) Parsed {
	at := newLocator(path, src)
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
	// starts are the offsets where the lines start, in order.
	starts []int
	// lines holds the tokens of each line lexed so far, by the offset where
	// the line starts, so that a line is lexed once however many tokens of it
	// the checker looks for.
	lines map[int]lineTokens
}

func newLocator(path string, src []byte) locator {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return locator{path: path, src: src, starts: starts, lines: map[int]lineTokens{}}
}

// lineStart is where the line that holds offset starts.
func (l locator) lineStart(offset int) int {
	i, found := slices.BinarySearch(l.starts, offset)
	if !found {
		i--
	}
	return l.starts[i]
}

func (l locator) diagnostic(offset int, code, message, fix string) diag.Diagnostic {
	offset = min(max(offset, 0), len(l.src))
	line, found := slices.BinarySearch(l.starts, offset)
	if !found {
		line--
	}
	return diag.Diagnostic{
		Code:    code,
		Subject: l.path,
		Position: diag.Position{
			Path:   l.path,
			Line:   line + 1,
			Column: offset - l.starts[line] + 1,
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
		return at.diagnostic(perr.Position().Offset, "syntax.unexpected-token", perr.Message(), "rewrite this line by the grammar of docs/language.md §3")
	}
	return at.diagnostic(0, "syntax.unexpected-token", err.Error(), "rewrite the file by the grammar of docs/language.md §3")
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
	inValue := l.inValue(off)
	key := line.construct(inValue)
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
	// A line that no construct starts, that starts with else, or whose
	// construct the block around it may not hold is wrong from its first token
	// on; any other line is wrong where the parser stopped. Among steps, the
	// parser reads a misplaced keyword as a sender and stops after it. A }
	// that gets here closed its block, and its line is wrong after it.
	c := constructs[key]
	misplaced := first
	if !inValue && key != "close" && len(line) > 0 {
		misplaced = !l.holds(line[0].Pos.Offset, key)
	}
	switch {
	case key == "unknown" && len(line) > 0:
		return l.diagnostic(line[0].Pos.Offset, "syntax.unexpected-token", u.Message(),
			"start the line with a keyword such as type, op, flow, property or case; a step is written sender -> op(args)")
	case key == "else" || misplaced && key != "literal":
		return l.diagnostic(line[0].Pos.Offset, "syntax.unexpected-token", u.Message(), "a "+c.name+" belongs "+c.place)
	}
	return l.diagnostic(off, "syntax.unexpected-token", u.Message(), "write the "+c.name+" as "+c.form)
}

// construct is one kind of line of docs/language.md §3: its name, how it is
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
	"codec":      {"model codec", "codec encoder, decoder", "in a type's block before fields and operations"},
	"field":      {"field", "field name: type", "in a type's block"},
	"value":      {"enum value", "value name", "in an enum's block"},
	"else":       {"branch introduced by else", `} else "label" {, on the line of the } that closes the previous branch`, "only right after a branch of an alt fragment, on the line of its }"},
	"literal":    {"value", "a number, a string, true, false, none, an enum value, [item, ...], {key: value, ...} op(param: value, ...) or encoded(value), broken across lines only after an opening bracket, around commas and before a closing bracket", "in an assignment"},
	"op":         {"operation declaration", `op name(param: type, ...), then optionally -> type, raises Type, ..., handles Type and "summary" in this order`, "at the top level of a component file, in a type's block or in an external actor's block"},
	"flow":       {"flow declaration", `flow name "summary" {`, "at the top level of a component file"},
	"property":   {"property declaration", `property name form "summary" {, where the form is invariant, postcondition, relation or transition`, "at the top level of a component file"},
	"about":      {"about line", "about ref, ...", "in a property's block"},
	"forall":     {"symbol", `forall name: type, followed by generated "domain" for a generated symbol`, "in a flow's or a property's block"},
	"where":      {"limit", "where path in low..high, where path in {value, ...} or where path size low..high", "in a flow's or a property's block"},
	"call":       {"tested call", "call receiver.op(args) or call op(args)", "in a postcondition property's block"},
	"ensure":     {"assertion", "ensure expression", "in a property's or flow's block"},
	"requires":   {"precondition", "requires expression", "in a property's block"},
	"within":     {"time budget", `within "duration"`, "after the tested call or flow steps"},
	"manual":     {"manual claim", `manual "reason"`, "in a property's block"},
	"observe":    {"observation", "observe path, ... or observe name: Type from input, ... or observe none", "in a postcondition property's block"},
	"changes":    {"allowed changes", "changes path, ... or changes none", "in a postcondition property's block"},
	"case":       {"case", `case name "summary" {, where the summary may be left out; a recorded case is case name coverage { or case name counterexample {`, "in a flow's or a property's block, or at the top level of a corpus file"},
	"corpus":     {"corpus declaration", "corpus property-or-flow", "at the start of a corpus file"},
	"on":         {"on block", "on Error {, right after a call", "among a flow's steps"},
	"async":      {"async block", "async op {", "among a flow's steps"},
	"alt":        {"alt fragment", `alt "label" {, with each further branch written } else "label" {`, "among a flow's steps"},
	"opt":        {"opt fragment", `opt "label" {`, "among a flow's steps"},
	"loop":       {"loop fragment", `loop "label" {`, "among a flow's steps"},
	step:         {"step", "sender -> op(args) with an optional {, sender ->> op(args), or sender emits Event(args)", "among a flow's steps"},
	"assignment": {"assignment", "symbol = value", "in a case's block"},
	"close":      {"closing brace", `} alone on its line, or } else "label" { between two branches of an alt fragment`, "at the end of a block"},
	"unknown":    {"line", "a declaration or member that starts with a keyword, a step (sender -> op(args)) or an assignment (symbol = value)", "in a block"},
}

// unnamed are the constructs that no keyword starts.
var unnamed = []string{step, "assignment", "close", "literal", "unknown"}

// blockLines are the constructs that the grammar lets each kind of block
// hold, by the construct of the line that opens it: "" for the top level,
// and step for a call's block.
var blockLines = grammarBlocks()

// grammarBlocks reads blockLines from the grammar's unions: a block is a
// slice of a union's members, and each member is a line of its construct.
func grammarBlocks() map[string][]string {
	members := map[reflect.Type][]any{}
	for _, u := range unions {
		members[u.iface] = u.members
	}
	blocks := map[string][]string{}
	seen := map[reflect.Type]bool{}
	var visit func(opener string, t reflect.Type)
	visit = func(opener string, t reflect.Type) {
		if seen[t] {
			return
		}
		seen[t] = true
		for field := range t.Fields() {
			if field.Type.Kind() != reflect.Slice {
				continue
			}
			for _, m := range members[field.Type.Elem()] {
				key := lineConstruct(m)
				if key == "" {
					continue
				}
				if !slices.Contains(blocks[opener], key) {
					blocks[opener] = append(blocks[opener], key)
				}
				visit(key, reflect.TypeOf(m).Elem())
			}
		}
	}
	visit("", reflect.TypeFor[File]())
	return blocks
}

// leadingKeyword matches the keyword that a tag of the grammar starts with.
var leadingKeyword = regexp.MustCompile(`^'([a-z]+)'`)

// lineConstruct is the construct of the lines that a member of a union of
// lines parses: the keyword that its first tag starts with; a step or an
// assignment for a line that no keyword starts; "" for a comment, which
// every block holds. Every construct has an entry in constructs, so that a
// diagnostic can say how it is written and where it stands.
func lineConstruct(member any) string {
	key := ""
	for field := range reflect.TypeOf(member).Elem().Fields() {
		if tag, ok := field.Tag.Lookup("parser"); ok {
			if m := leadingKeyword.FindStringSubmatch(tag); m != nil {
				key = m[1]
			}
			break
		}
	}
	if key == "" {
		switch member.(type) {
		case *Comment:
			return ""
		case *Assignment:
			key = "assignment"
		case Step:
			key = step
		}
	}
	if _, ok := constructs[key]; !ok {
		panic(fmt.Sprintf("syntax: the grammar's %T is a line of no construct in constructs", member))
	}
	return key
}

// holds tells whether the block around the line that holds offset may hold
// a line of the construct key. A block that blockLines does not name, a
// branch of an alt fragment, holds steps.
func (l locator) holds(offset int, key string) bool {
	block := ""
	if ts := l.opener(offset); len(ts) > 0 {
		block = ts.construct(false)
		if blockLines[block] == nil {
			block = step
		}
	}
	return slices.Contains(blockLines[block], key)
}

// opener lexes the line whose { opens the innermost block around the line
// that holds offset; it returns nil at the top level.
func (l locator) opener(offset int) lineTokens {
	depth := 0
	for end := bytes.LastIndexByte(l.src[:offset], '\n'); end >= 0; {
		start := bytes.LastIndexByte(l.src[:end], '\n') + 1
		ts := l.line(start)
		end = start - 1
		for i := len(ts) - 1; i >= 0; i-- {
			switch ts[i].Value {
			case "}":
				depth++
			case "{":
				depth--
			}
			if depth < 0 {
				return ts
			}
		}
	}
	return nil
}

// lineTokens are the tokens of one line, without spaces.
type lineTokens []lexer.Token

// line lexes the line that holds offset; its tokens keep their offsets in the file.
func (l locator) line(offset int) lineTokens {
	return l.tokens(l.lineStart(min(offset, len(l.src))))
}

// tokens gives the tokens from start, a token's boundary, to the end of its
// line. It lexes the whole line once and keeps it, so that looking up many
// tokens of a long line costs the length of the line, not its square.
func (l locator) tokens(start int) lineTokens {
	start = min(start, len(l.src))
	lineStart := l.lineStart(start)
	ts, ok := l.lines[lineStart]
	if !ok {
		ts = l.lex(lineStart)
		l.lines[lineStart] = ts
	}
	i, _ := slices.BinarySearchFunc(ts, start, func(t lexer.Token, offset int) int { return cmp.Compare(t.Pos.Offset, offset) })
	return ts[i:]
}

// lex lexes from start, a line's start, to the end of the line, without white
// space; the tokens keep their offsets in the file.
func (l locator) lex(start int) lineTokens {
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
// A first word followed by ->, ->>, ., or emits and then an UpperCamelCase
// word or a dotted reference starts a step (G5), and one followed by = an
// assignment, as in docs/language.md §3, even when the word is a keyword. Any
// other line inside a value (inValue) is part of it.
func (ts lineTokens) construct(inValue bool) string {
	if len(ts) == 0 {
		return "unknown"
	}
	first := ts[0]
	startsStep := len(ts) > 1 && (ts[1].Type == symbols["Arrow"] || ts[1].Type == symbols["SendArrow"] ||
		ts[1].Value == "." || ts[1].Value == "emits" && ts.emitted())
	_, keyword := constructs[first.Value]
	keyword = keyword && !slices.Contains(unnamed, first.Value)
	word := first.Type == symbols["Ident"]
	switch {
	case startsStep:
		return step
	case len(ts) > 1 && ts[1].Value == "=":
		return "assignment"
	case word && first.Value == "else":
		return "else"
	case word && keyword:
		return first.Value
	case slices.ContainsFunc(ts, func(t lexer.Token) bool { return t.Value == "=" }):
		return "assignment"
	case first.Value == "}":
		return "close"
	case inValue:
		return "literal"
	}
	return "unknown"
}

// emitted tells whether the third token of the line starts the event of an
// emit step: an UpperCamelCase word, or the first segment of a dotted reference.
func (ts lineTokens) emitted() bool {
	return len(ts) > 2 && ts[2].Type == symbols["Ident"] &&
		(upperClass.pattern(ts[2].Value) || len(ts) > 3 && ts[3].Value == ".")
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
	for end := bytes.LastIndexByte(l.src[:offset], '\n'); end >= 0; {
		start := bytes.LastIndexByte(l.src[:end], '\n') + 1
		ts := l.line(start)
		end = start - 1
		switch {
		case len(ts) == 0 || ts[0].Type == symbols["Comment"]:
			continue
		case len(ts) != 1 || ts[0].Value != "}":
			return false
		}
		ts = l.opener(start)
		opensBranch := len(ts) > 1 && ts[0].Value == "alt" && ts[1].Type == symbols["String"]
		return opensBranch || len(ts) > 1 && ts[0].Value == "}" && ts[1].Value == "else"
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

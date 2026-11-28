package syntax

import (
	"cmp"
	"reflect"
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/diag"
)

// Expr is a normalized expression. Text holds a literal, name, field,
// operator or quantified variable; Children are its operands in source order.
//
//uc:realizes syntax.Expr
type Expr struct {
	Pos      lexer.Position
	Kind     string
	Text     string
	Children []*Expr
}

// ExprParsed is the result of parsing one expression.
//
//uc:realizes syntax.ExprParsed
type ExprParsed struct {
	Expr        *Expr
	Diagnostics []diag.Diagnostic
}

// ParseExpr parses an expression independently of a design file.
//
//uc:realizes syntax.parseExpr
func ParseExpr(path, text string) ExprParsed {
	at := newLocator(path, []byte(text))
	if ds := forbidden(at); len(ds) != 0 {
		return ExprParsed{Diagnostics: ds}
	}
	tree, err := expressionParser.ParseString(path, text)
	if err != nil {
		return ExprParsed{Diagnostics: []diag.Diagnostic{parseError(at, err)}}
	}
	expr := tree.Expr()
	c := &checker{at: at}
	c.walk(reflect.ValueOf(tree), false)
	c.expression(expr)
	if len(c.diags) != 0 {
		sortDiagnostics(c.diags)
		return ExprParsed{Diagnostics: c.diags}
	}
	return ExprParsed{Expr: expr}
}

// ExprText prints the normalized expression, retaining source reference names.
//
//uc:realizes syntax.exprText
func ExprText(expr *Expr) string { return expressionText(expr, 0) }

type expression struct {
	Pos     lexer.Position
	Left    *orExpression `parser:"@@"`
	Implies *expression   `parser:"( 'implies' @@ )?"`
}

type orExpression struct {
	Terms []*andExpression `parser:"@@ ( 'or' @@ )*"`
}

type andExpression struct {
	Terms []*notExpression `parser:"@@ ( 'and' @@ )*"`
}

type notExpression struct {
	Pos     lexer.Position
	Not     *notExpression        `parser:"  'not' @@"`
	Compare *comparisonExpression `parser:"| @@"`
}

type comparisonExpression struct {
	Left  *sumExpression `parser:"@@"`
	Op    string         `parser:"( @( '==' | '!=' | '<' | '<=' | '>' | '>=' | 'in' )"`
	Right *sumExpression `parser:"@@ )?"`
}

type sumExpression struct {
	Left  *productExpression `parser:"@@"`
	Tails []*sumTail         `parser:"@@*"`
}

type sumTail struct {
	Op    string             `parser:"@( '+' | '-' )"`
	Right *productExpression `parser:"@@"`
}

type productExpression struct {
	Left  *postfixExpression `parser:"@@"`
	Tails []*productTail     `parser:"@@*"`
}

type productTail struct {
	Op    string             `parser:"@( '*' | '/' )"`
	Right *postfixExpression `parser:"@@"`
}

type postfixExpression struct {
	Atom  *atomExpression `parser:"@@"`
	Tails []*postfixTail  `parser:"@@*"`
}

type postfixTail struct {
	Pos   lexer.Position
	Field string      `parser:"  ( '.' @Ident"`
	Args  *arguments  `parser:"@@? )"`
	Index *expression `parser:"| '[' @@ ']'"`
}

type arguments struct {
	Values []*expression `parser:"'(' ( @@ ( ',' @@ )* )? ')'"`
}

type atomExpression struct {
	Pos     lexer.Position
	Number  *Number          `parser:"  @Number"`
	String  *Text            `parser:"| @String"`
	Boolean *Boolean         `parser:"| @( 'true' | 'false' )"`
	None    bool             `parser:"| @'none'"`
	Old     *expression      `parser:"| 'old' '(' @@ ')'"`
	Raised  *TypeRef         `parser:"| 'raised' '(' @@ ')'"`
	Failure *TypeRef         `parser:"| 'failure' '(' @@ ')'"`
	Each    *eachExpression  `parser:"| @@"`
	Named   *namedExpression `parser:"| @@"`
	Group   *expression      `parser:"| '(' @@ ')'"`
}

type namedExpression struct {
	Name string     `parser:"@Ident"`
	Args *arguments `parser:"@@?"`
}

type eachExpression struct {
	Kind       string      `parser:"@( 'all' | 'any' | 'count' | 'sum' ) '('"`
	Variable   Lower       `parser:"@Ident 'in'"`
	Collection *expression `parser:"@@ ':'"`
	Body       *expression `parser:"@@ ')'"`
}

var expressionParser = participle.MustBuild[expression](
	participle.Lexer(byteColumns{ucLexer}), participle.Elide("Whitespace"),
	participle.Map(checkEscapes, "String"), participle.UseLookahead(participle.MaxLookahead))

func node(kind, text string, pos lexer.Position, children ...*Expr) *Expr {
	return &Expr{Pos: pos, Kind: kind, Text: text, Children: children}
}

func binary(op string, left, right *Expr) *Expr { return node("binary", op, left.Pos, left, right) }

// Expr normalizes the parser's precedence layers into the public operand tree.
func (x *expression) Expr() *Expr {
	e := x.Left.expr()
	if x.Implies != nil {
		e = binary("implies", e, x.Implies.Expr())
	}
	return e
}

func (x *orExpression) expr() *Expr {
	e := x.Terms[0].expr()
	for _, term := range x.Terms[1:] {
		e = binary("or", e, term.expr())
	}
	return e
}

func (x *andExpression) expr() *Expr {
	e := x.Terms[0].expr()
	for _, term := range x.Terms[1:] {
		e = binary("and", e, term.expr())
	}
	return e
}

func (x *notExpression) expr() *Expr {
	if x.Not != nil {
		return node("unary", "not", x.Pos, x.Not.expr())
	}
	e := x.Compare.Left.expr()
	if x.Compare.Right != nil {
		e = binary(x.Compare.Op, e, x.Compare.Right.expr())
	}
	return e
}

func (x *sumExpression) expr() *Expr {
	e := x.Left.expr()
	for _, tail := range x.Tails {
		e = binary(tail.Op, e, tail.Right.expr())
	}
	return e
}

func (x *productExpression) expr() *Expr {
	e := x.Left.expr()
	for _, tail := range x.Tails {
		e = binary(tail.Op, e, tail.Right.expr())
	}
	return e
}

func (x *postfixExpression) expr() *Expr {
	e := x.Atom.expr()
	for _, tail := range x.Tails {
		if tail.Index != nil {
			e = node("index", "", e.Pos, e, tail.Index.Expr())
		} else {
			e = node("field", tail.Field, e.Pos, e)
			if tail.Args != nil {
				e = invocation(e, tail.Args)
			}
		}
	}
	return e
}

func invocation(callee *Expr, args *arguments) *Expr {
	children := []*Expr{callee}
	for _, arg := range args.Values {
		children = append(children, arg.Expr())
	}
	return node("call", "", callee.Pos, children...)
}

func (x *atomExpression) expr() *Expr {
	switch {
	case x.Number != nil:
		return node("number", string(*x.Number), x.Pos)
	case x.String != nil:
		return node("string", string(*x.String), x.Pos)
	case x.Boolean != nil:
		if *x.Boolean {
			return node("bool", "true", x.Pos)
		}
		return node("bool", "false", x.Pos)
	case x.None:
		return node("none", "", x.Pos)
	case x.Old != nil:
		return node("old", "", x.Pos, x.Old.Expr())
	case x.Raised != nil:
		return node("raised", strings.Join(x.Raised.Segments, "."), x.Pos)
	case x.Failure != nil:
		return node("failure", strings.Join(x.Failure.Segments, "."), x.Pos)
	case x.Each != nil:
		return node(x.Each.Kind, string(x.Each.Variable), x.Pos, x.Each.Collection.Expr(), x.Each.Body.Expr())
	case x.Named != nil:
		e := node("name", x.Named.Name, x.Pos)
		if x.Named.Args != nil {
			return invocation(e, x.Named.Args)
		}
		return e
	default:
		return x.Group.Expr()
	}
}

func precedence(e *Expr) int {
	if e.Kind == "unary" {
		return 4
	}
	if e.Kind != "binary" {
		return 8
	}
	switch e.Text {
	case "implies":
		return 1
	case "or":
		return 2
	case "and":
		return 3
	case "==", "!=", "<", "<=", ">", ">=", "in":
		return 5
	case "+", "-":
		return 6
	default:
		return 7
	}
}

func expressionText(e *Expr, parent int) string {
	var b strings.Builder
	writeExpression(&b, e, parent)
	return b.String()
}

// writeExpression writes e into b, bracketed when its precedence is below
// parent's. Writing into one builder keeps a long expression linear, where
// joining the texts of the children would copy them once per level.
func writeExpression(b *strings.Builder, e *Expr, parent int) {
	if e == nil {
		return
	}
	p := precedence(e)
	if p < parent {
		b.WriteString("(")
		defer b.WriteString(")")
	}
	switch e.Kind {
	case "number", "bool", "name":
		b.WriteString(e.Text)
	case "string":
		b.WriteString(quote(e.Text))
	case "none":
		b.WriteString("none")
	case "field":
		writeExpression(b, e.Children[0], 8)
		b.WriteString("." + e.Text)
	case "index":
		writeExpression(b, e.Children[0], 8)
		b.WriteString("[")
		writeExpression(b, e.Children[1], 0)
		b.WriteString("]")
	case "call":
		writeExpression(b, e.Children[0], 8)
		b.WriteString("(")
		for i, arg := range e.Children[1:] {
			if i > 0 {
				b.WriteString(", ")
			}
			writeExpression(b, arg, 0)
		}
		b.WriteString(")")
	case "binary":
		left, right := p, p+1
		if e.Text == "implies" {
			left, right = p+1, p
		} else if p == 5 {
			left = p + 1
		}
		writeExpression(b, e.Children[0], left)
		b.WriteString(" " + e.Text + " ")
		writeExpression(b, e.Children[1], right)
	case "unary":
		b.WriteString("not ")
		writeExpression(b, e.Children[0], p)
	case "old":
		b.WriteString("old(")
		writeExpression(b, e.Children[0], 0)
		b.WriteString(")")
	case "raised", "failure":
		b.WriteString(e.Kind + "(" + e.Text + ")")
	case "all", "any", "count", "sum":
		b.WriteString(e.Kind + "(" + e.Text + " in ")
		writeExpression(b, e.Children[0], 0)
		b.WriteString(": ")
		writeExpression(b, e.Children[1], 0)
		b.WriteString(")")
	}
}

var expressionWords = []string{"implies", "or", "and", "not", "in", "old", "raised", "failure", "result", "all", "any", "count", "sum", "size"}

func (c *checker) expression(e *Expr) {
	if e == nil {
		return
	}
	if e.Kind == "name" || e.Kind == "field" || e.Kind == "all" || e.Kind == "any" || e.Kind == "count" || e.Kind == "sum" {
		if !kebabClass.pattern(e.Text) && !upperClass.pattern(e.Text) && !lowerClass.pattern(e.Text) {
			c.report(e.Pos, "syntax.invalid-name", "an expression name is not kebab-case, UpperCamelCase or lowerCamelCase", "use a valid expression name")
		}
	}
	if e.Kind == "all" || e.Kind == "any" || e.Kind == "count" || e.Kind == "sum" {
		c.expressionName(e.Text, e.Pos)
	}
	if e.Kind == "binary" {
		if operator, ok := c.at.operator(e); ok {
			at := operator.Pos.Offset
			end := at + len(operator.Value)
			if at == 0 || end == len(c.at.src) || c.at.src[at-1] != ' ' || c.at.src[end] != ' ' {
				c.report(operator.Pos, "syntax.expression-spacing", "a binary expression operator needs spaces on both sides", "separate each binary operator from its operands with spaces")
			}
		}
	}
	for _, child := range e.Children {
		c.expression(child)
	}
}

// operator finds the token of the binary expression e's operator: the last
// token spelled like it that starts at or after the left operand and before
// the right operand, on the left operand's line. It searches back from the
// right operand, so it passes only the brackets between the two.
func (l locator) operator(e *Expr) (lexer.Token, bool) {
	ts := l.tokens(e.Children[0].Pos.Offset)
	right, _ := slices.BinarySearchFunc(ts, e.Children[1].Pos.Offset, func(t lexer.Token, offset int) int { return cmp.Compare(t.Pos.Offset, offset) })
	for i := right - 1; i >= 0; i-- {
		if ts[i].Value == e.Text {
			return ts[i], true
		}
	}
	return lexer.Token{}, false
}

func (c *checker) expressionName(name string, pos lexer.Position) {
	if slices.Contains(expressionWords, name) {
		c.report(pos, "syntax.expression-keyword-name", "an expression keyword cannot name an input, observation or quantified variable", "choose a name other than an expression keyword")
	}
}

// checkExpressions visits the expression-bearing lines without walking the
// parser's private precedence layers a second time through token positions.
func (c *checker) checkExpressions(value reflect.Value) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			if tree, ok := value.Interface().(*expression); ok {
				c.expression(tree.Expr())
				return
			}
			c.checkExpressions(value.Elem())
		}
	case reflect.Slice:
		for i := range value.Len() {
			c.checkExpressions(value.Index(i))
		}
	case reflect.Struct:
		for i := range value.NumField() {
			c.checkExpressions(value.Field(i))
		}
	}
}

func sortDiagnostics(ds []diag.Diagnostic) {
	slices.SortStableFunc(ds, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column), strings.Compare(a.Code, b.Code))
	})
}

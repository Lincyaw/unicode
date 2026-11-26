package syntax

import (
	"reflect"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// The grammar of docs/format.md §3, written as participle struct tags; these
// structs are the syntax tree. Every block is a list of lines, comments
// included, so the printer keeps each comment where it was. The lines of a
// block are taken in any order, and check.go checks the order afterwards, so
// that a misplaced declaration or member gets its own diagnostic rather than
// a grammar error. check.go also checks what a tag cannot say: the three name
// classes, the forms of references, summaries and reserved names.

// Kebab, Upper and Lower are names of the three classes of docs/format.md §2.
type (
	Kebab string
	Upper string
	Lower string
)

// Text is a string with its escapes resolved. Tokens keep their text as
// written, quotes included, so that no string can stand for a keyword or
// punctuation of the grammar; the escapes are resolved when a string is captured.
type Text string

func (t *Text) Capture(values []string) error {
	*t = Text(unquoted(values[0]))
	return nil
}

// Summary is a summary string, with its escapes resolved.
type Summary string

func (s *Summary) Capture(values []string) error {
	*s = Summary(unquoted(values[0]))
	return nil
}

// Number is a number in its canonical form (docs/format.md §2): no leading
// zeros in the integer part, no -0, and no trailing zeros in the fraction
// beyond its first digit. Parsing normalizes it, so fmt prints it canonical.
type Number string

func (n *Number) Capture(values []string) error {
	*n = Number(canonicalNumber(values[0]))
	return nil
}

// CommentText is what follows `//` on a comment line, without trailing spaces.
type CommentText string

func (c *CommentText) Capture(values []string) error {
	*c = CommentText(commentText(values[0]))
	return nil
}

// Ref is a dotted reference, resolved by the design component. TypeRef,
// OpRef, Participant and Path are the forms the grammar uses.
type Ref struct {
	Pos      lexer.Position
	Segments []string `parser:"@Ident ( '.' @Ident )*"`
}

type (
	TypeRef     Ref
	OpRef       Ref
	Participant Ref
	Path        Ref
)

// File is the syntax tree of one file: its declarations and comments in order.
//
//uc:realizes syntax.File
type File struct {
	Path  string
	Kind  FileKind
	Decls []Decl `parser:"NL? @@*"`
}

// Comment is one comment line; Text is what follows `//`, without trailing spaces.
type Comment struct {
	Pos  lexer.Position
	Text CommentText `parser:"@Comment NL"`
}

// Decl is a top-level line: *Comment, *SystemDecl, *NotationDecl,
// *LanguageDecl, *PersonDecl, *ExternalDecl, *ComponentDecl, *TypeDecl, *OpDecl, *FlowDecl,
// *PropertyDecl, *CorpusDecl or *RecordedCase. Which ones a file may hold
// depends on its kind.
type Decl interface{ decl() }

type SystemDecl struct {
	Pos     lexer.Position
	Name    Kebab   `parser:"'system' @Ident"`
	Summary Summary `parser:"@String NL"`
}

type NotationDecl struct {
	Pos  lexer.Position
	Name string `parser:"'notation' @( 'ddd' | 'c4' ) NL"`
}

type LanguageDecl struct {
	Pos   lexer.Position
	Name  string         `parser:"'language' @( 'go' | 'python' ) '{' NL"`
	Lines []LanguageLine `parser:"@@* '}' NL"`
}

// LanguageLine is *Comment or *Root.
type LanguageLine interface{ languageLine() }

type Root struct {
	Pos  lexer.Position
	Path Text `parser:"'root' @String NL"`
}

// PersonDecl is an actor of the form person, which has no block.
type PersonDecl struct {
	Pos     lexer.Position
	Name    Kebab   `parser:"'actor' @Ident 'person'"`
	Summary Summary `parser:"@String NL"`
}

// ExternalDecl is an actor of the form external: the types and operations of
// an external system that we use.
type ExternalDecl struct {
	Pos     lexer.Position
	Name    Kebab          `parser:"'actor' @Ident 'external'"`
	Summary Summary        `parser:"@String"`
	Lines   []ExternalLine `parser:"( '{' NL @@* '}' )? NL"`
}

// ExternalLine is *Comment, *TypeDecl or *OpDecl.
type ExternalLine interface{ externalLine() }

type ComponentDecl struct {
	Pos     lexer.Position
	Path    Path            `parser:"'component' @@"`
	Summary Summary         `parser:"@String '{' NL"`
	Lines   []ComponentLine `parser:"@@* '}' NL"`
}

// ComponentLine is *Comment, *Label, *Focus, *Paths or *Uses.
type ComponentLine interface{ componentLine() }

type Label struct {
	Pos  lexer.Position
	Name Kebab `parser:"'label' @Ident NL"`
}

type Focus struct {
	Pos   lexer.Position
	Level string `parser:"'focus' @( 'core' | 'support' | 'utility' ) NL"`
}

type Paths struct {
	Pos      lexer.Position
	Patterns []Text `parser:"'paths' @String ( ',' @String )* NL"`
}

type Uses struct {
	Pos  lexer.Position
	Refs []Ref `parser:"'uses' @@ ( ',' @@ )* NL"`
}

type TypeDecl struct {
	Pos     lexer.Position
	Name    Upper    `parser:"'type' @Ident"`
	Form    string   `parser:"@( 'object' | 'record' | 'interface' | 'enum' | 'error' | 'event' )"`
	Summary Summary  `parser:"@String"`
	Lines   []Member `parser:"( '{' NL @@* '}' )? NL"`
}

// Member is *Comment, *Label, *Extends, *Implements, *Field, *EnumValue or *OpDecl.
type Member interface{ member() }

type Extends struct {
	Pos  lexer.Position
	Type TypeRef `parser:"'extends' @@ NL"`
}

type Implements struct {
	Pos  lexer.Position
	Type TypeRef `parser:"'implements' @@ NL"`
}

type Field struct {
	Pos  lexer.Position
	Name Lower    `parser:"'field' @Ident ':'"`
	Type TypeExpr `parser:"@@ NL"`
}

type EnumValue struct {
	Pos  lexer.Position
	Name Lower `parser:"'value' @Ident NL"`
}

// OpDecl is an operation; Result, Handles and Summary are nil when absent.
type OpDecl struct {
	Pos     lexer.Position
	Name    Lower     `parser:"'op' @Ident '('"`
	Params  []Param   `parser:"( @@ ( ',' @@ )* )? ')'"`
	Result  TypeExpr  `parser:"( '->' @@ )?"`
	Raises  []TypeRef `parser:"( 'raises' @@ ( ',' @@ )* )?"`
	Handles *TypeRef  `parser:"( 'handles' @@ )?"`
	Summary *Summary  `parser:"@String? NL"`
}

type Param struct {
	Pos  lexer.Position
	Name Lower    `parser:"@Ident ':'"`
	Type TypeExpr `parser:"@@"`
}

// TypeExpr is *Builtin, *ElemType, *MapType, *CallType or *NamedType.
type TypeExpr interface{ typeExpr() }

// Builtin is a builtin type; a builtin's name followed by `.` starts a type reference instead.
type Builtin struct {
	Pos  lexer.Position
	Name string `parser:"@( 'bool' | 'int' | 'float' | 'string' | 'bytes' | 'time' | 'duration' ) (?! '.' )"`
}

// ElemType is list<T>, set<T> or optional<T>.
type ElemType struct {
	Pos  lexer.Position
	Kind string   `parser:"@( 'list' | 'set' | 'optional' ) '<'"`
	Elem TypeExpr `parser:"@@ '>'"`
}

type MapType struct {
	Pos   lexer.Position
	Key   TypeExpr `parser:"'map' '<' @@ ','"`
	Value TypeExpr `parser:"@@ '>'"`
}

type CallType struct {
	Pos     lexer.Position
	Targets []Ref `parser:"'call' '<' @@ ( ',' @@ )* '>'"`
}

type NamedType struct {
	Pos lexer.Position
	Ref TypeRef `parser:"@@"`
}

type FlowDecl struct {
	Pos     lexer.Position
	Name    Kebab      `parser:"'flow' @Ident"`
	Summary Summary    `parser:"@String '{' NL"`
	Lines   []FlowLine `parser:"@@* '}' NL"`
}

// FlowLine is *Comment, *Symbol, *SubsetLimit, *IntervalLimit, *CaseDecl or a step.
type FlowLine interface{ flowLine() }

type PropertyDecl struct {
	Pos     lexer.Position
	Name    Kebab          `parser:"'property' @Ident"`
	Form    string         `parser:"@( 'invariant' | 'postcondition' | 'relation' | 'transition' )"`
	Summary Summary        `parser:"@String '{' NL"`
	Lines   []PropertyLine `parser:"@@* '}' NL"`
}

// PropertyLine is *Comment, *About, *Symbol, *SubsetLimit, *IntervalLimit or *CaseDecl.
type PropertyLine interface{ propertyLine() }

type About struct {
	Pos  lexer.Position
	Refs []Ref `parser:"'about' @@ ( ',' @@ )* NL"`
}

// Symbol is a `forall`; Domain describes a generated symbol and is nil
// otherwise. Domain and the labels of fragments follow the rules of summaries.
type Symbol struct {
	Pos    lexer.Position
	Name   Lower    `parser:"'forall' @Ident ':'"`
	Type   TypeExpr `parser:"@@"`
	Domain *Summary `parser:"( 'generated' @String )? NL"`
}

// SubsetLimit is a `where` that narrows an enum to some of its values.
type SubsetLimit struct {
	Pos     lexer.Position
	Path    []Lower `parser:"'where' @Ident ( '.' @Ident )* 'in'"`
	Members []Lower `parser:"'{' @Ident ( ',' @Ident )* '}' NL"`
}

// IntervalLimit is a `where` that bounds a number (Op "in") or a size (Op "size").
type IntervalLimit struct {
	Pos  lexer.Position
	Path []Lower `parser:"'where' @Ident ( '.' @Ident )*"`
	Op   string  `parser:"@( 'in' | 'size' )"`
	Low  Number  `parser:"@Number '..'"`
	High Number  `parser:"@Number NL"`
}

// CaseDecl is a declared case; Summary is nil when absent.
type CaseDecl struct {
	Pos     lexer.Position
	Name    Kebab      `parser:"'case' @Ident"`
	Summary *Summary   `parser:"@String? '{' NL"`
	Lines   []CaseLine `parser:"@@* '}' NL"`
}

// CaseLine is *Comment or *Assignment.
type CaseLine interface{ caseLine() }

type Assignment struct {
	Pos    lexer.Position
	Symbol Lower `parser:"@Ident '='"`
	Value  Value `parser:"@@ NL"`
}

type CorpusDecl struct {
	Pos    lexer.Position
	Target Ref `parser:"'corpus' @@ NL"`
}

type RecordedCase struct {
	Pos    lexer.Position
	Name   Kebab      `parser:"'case' @Ident"`
	Reason string     `parser:"@( 'coverage' | 'counterexample' ) '{' NL"`
	Lines  []CaseLine `parser:"@@* '}' NL"`
}

// Step is *Comment or a step: *Call, *Send, *Emit, *OnBlock, *Async, *Alt, *Opt or *Loop.
type Step interface{ step() }

// Call is a synchronous call; Lines are what the receiver does during it.
type Call struct {
	Pos   lexer.Position
	From  Participant `parser:"@@ '->'"`
	Op    OpRef       `parser:"@@"`
	Args  []Lower     `parser:"'(' ( @Ident ( ',' @Ident )* )? ')'"`
	Lines []Step      `parser:"( '{' NL @@* '}' )? NL"`
}

type Send struct {
	Pos  lexer.Position
	From Participant `parser:"@@ '->>'"`
	Op   OpRef       `parser:"@@"`
	Args []Lower     `parser:"'(' ( @Ident ( ',' @Ident )* )? ')' NL"`
}

type Emit struct {
	Pos   lexer.Position
	From  Participant `parser:"@@ 'emits'"`
	Event TypeRef     `parser:"@@"`
	Args  []Lower     `parser:"'(' ( @Ident ( ',' @Ident )* )? ')' NL"`
}

// OnBlock handles an error of the call before it.
type OnBlock struct {
	Pos   lexer.Position
	Error TypeRef `parser:"'on' @@ '{' NL"`
	Lines []Step  `parser:"@@* '}' NL"`
}

type Async struct {
	Pos   lexer.Position
	Op    OpRef  `parser:"'async' @@ '{' NL"`
	Lines []Step `parser:"@@* '}' NL"`
}

// Alt has one branch per label; every branch after the first is an `else`.
type Alt struct {
	Pos      lexer.Position
	Branches []Branch `parser:"'alt' @@ ( 'else' @@ )* NL"`
}

type Branch struct {
	Pos   lexer.Position
	Label Summary `parser:"@String '{' NL"`
	Lines []Step  `parser:"@@* '}'"`
}

type Opt struct {
	Pos   lexer.Position
	Label Summary `parser:"'opt' @String '{' NL"`
	Lines []Step  `parser:"@@* '}' NL"`
}

type Loop struct {
	Pos   lexer.Position
	Label Summary `parser:"'loop' @String '{' NL"`
	Lines []Step  `parser:"@@* '}' NL"`
}

// Value is *NumberValue, *StringValue, *CallValue, *BoolValue, *NoneValue,
// *NameValue, *ListValue or *BraceValue. A value is read by its symbol's
// type, which the design component knows. Inside brackets a value may break
// its line after the opening bracket, around commas and before the closing one.
type Value interface{ value() }

type NumberValue struct {
	Pos  lexer.Position
	Text Number `parser:"@Number"`
}

type StringValue struct {
	Pos  lexer.Position
	Text Text `parser:"@String"`
}

// CallValue is `op(param: value, …)`, a value of a call<…> type.
type CallValue struct {
	Pos  lexer.Position
	Op   Lower `parser:"@Ident '(' NL?"`
	Args []Arg `parser:"( @@ NL? ( ',' NL? @@ NL? )* ( ',' NL? )? )? ')'"`
}

type Arg struct {
	Pos   lexer.Position
	Name  Lower `parser:"@Ident ':'"`
	Value Value `parser:"@@"`
}

type BoolValue struct {
	Pos   lexer.Position
	Value Boolean `parser:"@( 'true' | 'false' )"`
}

// Boolean captures the literal true or false.
type Boolean bool

func (b *Boolean) Capture(values []string) error {
	*b = values[0] == "true"
	return nil
}

type NoneValue struct {
	Pos  lexer.Position
	None bool `parser:"@'none'"`
}

// NameValue is a bare name: an enum value.
type NameValue struct {
	Pos  lexer.Position
	Name Lower `parser:"@Ident"`
}

type ListValue struct {
	Pos   lexer.Position
	Items []Value `parser:"'[' NL? ( @@ NL? ( ',' NL? @@ NL? )* ( ',' NL? )? )? ']'"`
}

// BraceValue is `{key: value, …}`: a record or a map.
type BraceValue struct {
	Pos     lexer.Position
	Entries []Entry `parser:"'{' NL? ( @@ NL? ( ',' NL? @@ NL? )* ( ',' NL? )? )? '}'"`
}

type Entry struct {
	Pos   lexer.Position
	Key   Key   `parser:"@@ ':'"`
	Value Value `parser:"@@"`
}

// Key is a record field's name, or a map key written as a string or a number.
type Key struct {
	Pos    lexer.Position
	Name   *Lower  `parser:"  @Ident"`
	String *Text   `parser:"| @String"`
	Number *Number `parser:"| @Number"`
}

func (*Comment) decl()       {}
func (*SystemDecl) decl()    {}
func (*NotationDecl) decl()  {}
func (*LanguageDecl) decl()  {}
func (*PersonDecl) decl()    {}
func (*ExternalDecl) decl()  {}
func (*ComponentDecl) decl() {}
func (*TypeDecl) decl()      {}
func (*OpDecl) decl()        {}
func (*FlowDecl) decl()      {}
func (*PropertyDecl) decl()  {}
func (*CorpusDecl) decl()    {}
func (*RecordedCase) decl()  {}

func (*Comment) languageLine() {}
func (*Root) languageLine()    {}

func (*Comment) externalLine()  {}
func (*TypeDecl) externalLine() {}
func (*OpDecl) externalLine()   {}

func (*Comment) componentLine() {}
func (*Label) componentLine()   {}
func (*Focus) componentLine()   {}
func (*Paths) componentLine()   {}
func (*Uses) componentLine()    {}

func (*Comment) member()    {}
func (*Label) member()      {}
func (*Extends) member()    {}
func (*Implements) member() {}
func (*Field) member()      {}
func (*EnumValue) member()  {}
func (*OpDecl) member()     {}

func (*Builtin) typeExpr()   {}
func (*ElemType) typeExpr()  {}
func (*MapType) typeExpr()   {}
func (*CallType) typeExpr()  {}
func (*NamedType) typeExpr() {}

func (*Comment) flowLine()       {}
func (*Symbol) flowLine()        {}
func (*SubsetLimit) flowLine()   {}
func (*IntervalLimit) flowLine() {}
func (*CaseDecl) flowLine()      {}
func (*Call) flowLine()          {}
func (*Send) flowLine()          {}
func (*Emit) flowLine()          {}
func (*OnBlock) flowLine()       {}
func (*Async) flowLine()         {}
func (*Alt) flowLine()           {}
func (*Opt) flowLine()           {}
func (*Loop) flowLine()          {}

func (*Comment) propertyLine()       {}
func (*About) propertyLine()         {}
func (*Symbol) propertyLine()        {}
func (*SubsetLimit) propertyLine()   {}
func (*IntervalLimit) propertyLine() {}
func (*CaseDecl) propertyLine()      {}

func (*Comment) caseLine()    {}
func (*Assignment) caseLine() {}

func (*Comment) step() {}
func (*Call) step()    {}
func (*Send) step()    {}
func (*Emit) step()    {}
func (*OnBlock) step() {}
func (*Async) step()   {}
func (*Alt) step()     {}
func (*Opt) step()     {}
func (*Loop) step()    {}

func (*NumberValue) value() {}
func (*StringValue) value() {}
func (*CallValue) value()   {}
func (*BoolValue) value()   {}
func (*NoneValue) value()   {}
func (*NameValue) value()   {}
func (*ListValue) value()   {}
func (*BraceValue) value()  {}

// union is one union of the grammar: an interface of the syntax tree and its
// members, in the order participle tries them.
type union struct {
	iface   reflect.Type
	members []any
	option  participle.Option
}

func unionOf[T any](members ...T) union {
	u := union{iface: reflect.TypeFor[T](), option: participle.Union[T](members...)}
	for _, m := range members {
		u.members = append(u.members, m)
	}
	return u
}

// unions are the grammar's unions; parse.go reads from them which lines each
// block holds. Steps come last in them: a line that starts with a keyword is
// that construct unless it only parses as a step whose sender has the keyword's name.
var unions = []union{
	unionOf[Decl](&Comment{}, &SystemDecl{}, &NotationDecl{}, &LanguageDecl{}, &PersonDecl{}, &ExternalDecl{}, &ComponentDecl{},
		&TypeDecl{}, &OpDecl{}, &FlowDecl{}, &PropertyDecl{}, &CorpusDecl{}, &RecordedCase{}),
	unionOf[LanguageLine](&Comment{}, &Root{}),
	unionOf[ExternalLine](&Comment{}, &TypeDecl{}, &OpDecl{}),
	unionOf[ComponentLine](&Comment{}, &Label{}, &Focus{}, &Paths{}, &Uses{}),
	unionOf[Member](&Comment{}, &Label{}, &Extends{}, &Implements{}, &Field{}, &EnumValue{}, &OpDecl{}),
	unionOf[TypeExpr](&Builtin{}, &ElemType{}, &MapType{}, &CallType{}, &NamedType{}),
	unionOf[FlowLine](&Comment{}, &Symbol{}, &SubsetLimit{}, &IntervalLimit{}, &CaseDecl{},
		&OnBlock{}, &Async{}, &Alt{}, &Opt{}, &Loop{}, &Call{}, &Send{}, &Emit{}),
	unionOf[PropertyLine](&Comment{}, &About{}, &Symbol{}, &SubsetLimit{}, &IntervalLimit{}, &CaseDecl{}),
	unionOf[CaseLine](&Comment{}, &Assignment{}),
	unionOf[Step](&Comment{}, &OnBlock{}, &Async{}, &Alt{}, &Opt{}, &Loop{}, &Call{}, &Send{}, &Emit{}),
	unionOf[Value](&NumberValue{}, &StringValue{}, &CallValue{}, &BoolValue{}, &NoneValue{},
		&NameValue{}, &ListValue{}, &BraceValue{}),
}

var ucParser = participle.MustBuild[File](append([]participle.Option{
	participle.Lexer(byteColumns{ucLexer}),
	participle.Elide("Whitespace"),
	participle.Map(checkEscapes, "String"),
	participle.UseLookahead(participle.MaxLookahead),
}, unionOptions()...)...)

func unionOptions() []participle.Option {
	var out []participle.Option
	for _, u := range unions {
		out = append(out, u.option)
	}
	return out
}

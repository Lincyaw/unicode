package syntax

// Pos is where a node starts: its first token's line and byte column, both from 1.
type Pos struct {
	Line   int
	Column int
}

// Comment is one comment line. Text is what follows the `//`, without trailing spaces.
type Comment struct {
	Pos  Pos
	Text string
}

// File is the syntax tree of one file. Exactly one of System, Component and
// Corpus is set, the one that Kind names. Trailing holds the comments after the
// last declaration.
//
//uc:realizes syntax.File
type File struct {
	Path      string
	Kind      FileKind
	System    *SystemFile
	Component *ComponentFile
	Corpus    *CorpusFile
	Trailing  []Comment
}

// SystemFile is `design/system.uc`; its declaration groups keep the order of the grammar.
type SystemFile struct {
	System    SystemDecl
	Notations []NotationDecl
	Languages []LanguageDecl
	Actors    []ActorDecl
}

type SystemDecl struct {
	Comments []Comment
	Pos      Pos
	Name     string
	Summary  string
}

type NotationDecl struct {
	Comments []Comment
	Pos      Pos
	Name     string
}

type LanguageDecl struct {
	Comments []Comment
	Pos      Pos
	Name     string
	Roots    []Root
	Trailing []Comment
}

type Root struct {
	Comments []Comment
	Pos      Pos
	Path     string
}

// ActorDecl is an actor; only an external one has Types, Ops and Trailing.
type ActorDecl struct {
	Comments []Comment
	Pos      Pos
	Name     string
	Form     string
	Summary  string
	Types    []TypeDecl
	Ops      []OpDecl
	Trailing []Comment
}

// ComponentFile is a component's design file; its declaration groups keep the order of the grammar.
type ComponentFile struct {
	Component  ComponentDecl
	Types      []TypeDecl
	Ops        []OpDecl
	Flows      []FlowDecl
	Properties []PropertyDecl
}

// ComponentDecl is the `component` block. Focus and Uses are nil when absent.
type ComponentDecl struct {
	Comments []Comment
	Pos      Pos
	Path     []string
	Summary  string
	Labels   []Label
	Focus    *Focus
	Paths    Paths
	Uses     *Uses
	Trailing []Comment
}

type Label struct {
	Comments []Comment
	Pos      Pos
	Name     string
}

type Focus struct {
	Comments []Comment
	Pos      Pos
	Level    string
}

type Paths struct {
	Comments []Comment
	Pos      Pos
	Patterns []string
}

type Uses struct {
	Comments []Comment
	Pos      Pos
	Refs     []Ref
}

// Ref is a dotted reference; resolving it is the design component's job.
type Ref struct {
	Pos      Pos
	Segments []string
}

// TypeDecl is a type; its members are grouped in the order of docs/format.md §3.
type TypeDecl struct {
	Comments   []Comment
	Pos        Pos
	Name       string
	Form       string
	Summary    string
	Labels     []Label
	Extends    []Supertype
	Implements []Supertype
	Fields     []Field
	Values     []EnumValue
	Ops        []OpDecl
	Trailing   []Comment
}

// Supertype is one `extends` or `implements` member.
type Supertype struct {
	Comments []Comment
	Pos      Pos
	Type     Ref
}

type Field struct {
	Comments []Comment
	Pos      Pos
	Name     string
	Type     TypeExpr
}

type EnumValue struct {
	Comments []Comment
	Pos      Pos
	Name     string
}

// OpDecl is an operation. Result and Handles are nil and Summary is empty when absent.
type OpDecl struct {
	Comments []Comment
	Pos      Pos
	Name     string
	Params   []Param
	Result   TypeExpr
	Raises   []Ref
	Handles  *Ref
	Summary  string
}

type Param struct {
	Pos  Pos
	Name string
	Type TypeExpr
}

// TypeExpr is one of *BuiltinType, *NamedType, *ElemType, *MapType and *CallType.
type TypeExpr interface {
	typeExpr()
}

// BuiltinType is one of bool, int, float, string, bytes, time and duration.
type BuiltinType struct {
	Pos  Pos
	Name string
}

type NamedType struct {
	Pos Pos
	Ref Ref
}

// ElemType is list<T>, set<T> or optional<T>; Kind is the keyword.
type ElemType struct {
	Pos  Pos
	Kind string
	Elem TypeExpr
}

type MapType struct {
	Pos   Pos
	Key   TypeExpr
	Value TypeExpr
}

type CallType struct {
	Pos     Pos
	Targets []Ref
}

func (*BuiltinType) typeExpr() {}
func (*NamedType) typeExpr()   {}
func (*ElemType) typeExpr()    {}
func (*MapType) typeExpr()     {}
func (*CallType) typeExpr()    {}

// FlowDecl is a flow; its members are grouped in the order of docs/format.md §3.
type FlowDecl struct {
	Comments []Comment
	Pos      Pos
	Name     string
	Summary  string
	Symbols  []Symbol
	Limits   []Limit
	Steps    []Step
	Cases    []CaseDecl
	Trailing []Comment
}

// PropertyDecl is a property; its members are grouped in the order of docs/format.md §3.
type PropertyDecl struct {
	Comments []Comment
	Pos      Pos
	Name     string
	Form     string
	Summary  string
	About    About
	Symbols  []Symbol
	Limits   []Limit
	Cases    []CaseDecl
	Trailing []Comment
}

type About struct {
	Comments []Comment
	Pos      Pos
	Refs     []Ref
}

// Symbol is a `forall`. Domain is the description of a generated symbol.
type Symbol struct {
	Comments  []Comment
	Pos       Pos
	Name      string
	Type      TypeExpr
	Generated bool
	Domain    string
}

// LimitKind tells which of the three forms a `where` has.
type LimitKind string

const (
	LimitRange   LimitKind = "range"
	LimitMembers LimitKind = "members"
	LimitSize    LimitKind = "size"
)

// Limit is a `where`. Low and High are the bounds of a range or a size as
// written; Members is the subset of an enum.
type Limit struct {
	Comments []Comment
	Pos      Pos
	Path     []string
	Kind     LimitKind
	Low      string
	High     string
	Members  []string
}

// CaseDecl is a declared case; Summary is empty when absent.
type CaseDecl struct {
	Comments    []Comment
	Pos         Pos
	Name        string
	Summary     string
	Assignments []Assignment
	Trailing    []Comment
}

// Assignment is one `symbol = value` line of a case.
type Assignment struct {
	Comments []Comment
	Pos      Pos
	Symbol   string
	Value    Value
}

// Step is one of *CallStep, *SendStep, *EmitStep, *AsyncStep, *AltStep, *OptStep and *LoopStep.
type Step interface {
	step()
}

// CallStep is a synchronous call. Steps are what the receiver does during the
// call; Handlers are the `on` blocks that follow it.
type CallStep struct {
	Comments []Comment
	Pos      Pos
	From     Ref
	Op       Ref
	Args     []string
	Steps    []Step
	Trailing []Comment
	Handlers []OnBlock
}

type OnBlock struct {
	Comments []Comment
	Pos      Pos
	Error    Ref
	Steps    []Step
	Trailing []Comment
}

type SendStep struct {
	Comments []Comment
	Pos      Pos
	From     Ref
	Op       Ref
	Args     []string
}

type EmitStep struct {
	Comments []Comment
	Pos      Pos
	From     Ref
	Event    Ref
	Args     []string
}

type AsyncStep struct {
	Comments []Comment
	Pos      Pos
	Op       Ref
	Steps    []Step
	Trailing []Comment
}

// AltStep has at least one branch; every branch after the first is an `else`.
type AltStep struct {
	Comments []Comment
	Pos      Pos
	Branches []Branch
}

type Branch struct {
	Pos      Pos
	Label    string
	Steps    []Step
	Trailing []Comment
}

type OptStep struct {
	Comments []Comment
	Pos      Pos
	Label    string
	Steps    []Step
	Trailing []Comment
}

type LoopStep struct {
	Comments []Comment
	Pos      Pos
	Label    string
	Steps    []Step
	Trailing []Comment
}

func (*CallStep) step()  {}
func (*SendStep) step()  {}
func (*EmitStep) step()  {}
func (*AsyncStep) step() {}
func (*AltStep) step()   {}
func (*OptStep) step()   {}
func (*LoopStep) step()  {}

// CorpusFile is a recorded-case file.
type CorpusFile struct {
	Corpus CorpusDecl
	Cases  []RecordedCase
}

type CorpusDecl struct {
	Comments []Comment
	Pos      Pos
	Target   Ref
}

type RecordedCase struct {
	Comments    []Comment
	Pos         Pos
	Name        string
	Reason      string
	Assignments []Assignment
	Trailing    []Comment
}

// Value is one of *NumberValue, *StringValue, *BoolValue, *NoneValue,
// *NameValue, *CallValue, *ListValue and *BraceValue. Values are untyped here;
// the symbol's type decides how one is read.
type Value interface {
	value()
}

// NumberValue keeps the number as written.
type NumberValue struct {
	Pos  Pos
	Text string
}

// StringValue holds the string with its escapes resolved.
type StringValue struct {
	Pos  Pos
	Text string
}

type BoolValue struct {
	Pos   Pos
	Value bool
}

type NoneValue struct {
	Pos Pos
}

// NameValue is a bare lower-camel name, an enum value.
type NameValue struct {
	Pos  Pos
	Name string
}

// CallValue is `op(param: value, …)`, a value of a call<…> type.
type CallValue struct {
	Pos  Pos
	Op   string
	Args []Arg
}

type Arg struct {
	Pos   Pos
	Name  string
	Value Value
}

type ListValue struct {
	Pos   Pos
	Items []Value
}

// BraceValue is `{key: value, …}`, a record or a map. A key is a *NameValue,
// a *StringValue or a *NumberValue.
type BraceValue struct {
	Pos     Pos
	Entries []Entry
}

type Entry struct {
	Pos   Pos
	Key   Value
	Value Value
}

func (*NumberValue) value() {}
func (*StringValue) value() {}
func (*BoolValue) value()   {}
func (*NoneValue) value()   {}
func (*NameValue) value()   {}
func (*CallValue) value()   {}
func (*ListValue) value()   {}
func (*BraceValue) value()  {}

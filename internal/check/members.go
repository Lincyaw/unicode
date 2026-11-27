package check

import (
	"github.com/lincyaw/unicode/internal/graph"
)

// MemberKind tells whether a member of a type is a field or an enum value.
//
//uc:realizes check.MemberKind
type MemberKind string

const (
	MemberKindField MemberKind = "field"
	MemberKindValue MemberKind = "value"
)

// Member is a field or an enum value of a design type, compared with the
// code type that realizes it (docs/model.md §10 rule 18). Type and CodeType
// are a field's; Refers lists the design types its type names; Status is
// nil when no code type is compared.
//
//uc:realizes check.Member
type Member struct {
	Kind     MemberKind    `json:"kind"`
	Name     string        `json:"name"`
	Type     string        `json:"type,omitempty"`
	CodeType string        `json:"codeType,omitempty"`
	InDesign bool          `json:"inDesign"`
	InCode   bool          `json:"inCode"`
	Refers   []string      `json:"refers"`
	Status   *graph.Status `json:"status,omitempty"`
}

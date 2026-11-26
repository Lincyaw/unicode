package lang

import (
	"fmt"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// NameRole is what a name of the design stands for when a backend maps it to
// its language (docs/design.md §4).
//
//uc:realizes lang.NameRole
type NameRole string

const (
	NameRoleType          NameRole = "type"
	NameRoleOp            NameRole = "op"
	NameRoleField         NameRole = "field"
	NameRoleEnumValue     NameRole = "enumValue"
	NameRolePredicate     NameRole = "predicate"
	NameRoleFlowPredicate NameRole = "flowPredicate"
)

// ExtractFailed is the error of Extract when the code cannot be extracted:
// a source root does not exist, the language's toolchain is not available,
// or the toolchain cannot load the code under Root.
//
//uc:realizes lang.ExtractFailed
type ExtractFailed struct {
	Language string
	Root     string
	Cause    error
}

func (e ExtractFailed) Error() string {
	return fmt.Sprintf("cannot extract the %s code under %s: %v", e.Language, e.Root, e.Cause)
}

func (e ExtractFailed) Unwrap() error { return e.Cause }

// Extracted is the result of one extraction: the nodes and edges of the code
// layer, and a diagnostic for every binding that is not well-formed.
//
//uc:realizes lang.Extracted
type Extracted struct {
	Fragment    graph.Fragment
	Diagnostics []diag.Diagnostic
}

// Backend is one language's backend: it extracts the code layer statically
// and maps the design's names to its language.
//
//uc:realizes lang.Backend
type Backend interface {
	// Language is the language's name as the system file writes it.
	Language() string
	// Extract reads the code under the source roots, which are relative to
	// root. Methods of a type that realizes a design type get candidate
	// realizes edges to the operations their names map back to.
	Extract(root string, srcRoots []string) (Extracted, error)
	// MapName is how the language writes a name of the design in the given role.
	MapName(role NameRole, designName string) string
}

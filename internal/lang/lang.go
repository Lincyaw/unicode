package lang

import (
	"fmt"
	"time"

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

// Edit is a complete generated file at a path relative to the repository root.
//
//uc:realizes lang.Edit
type Edit struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
}

// Generation is the output of a backend. Diagnostics prevent applying its edits.
//
//uc:realizes lang.Generation
type Generation struct {
	Edits       []Edit            `json:"edits"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// GenPlan is the repository, source roots and linked graph of one language.
//
//uc:realizes lang.GenPlan
type GenPlan struct {
	Root  string       `json:"root"`
	Roots []string     `json:"roots"`
	Graph *graph.Graph `json:"graph"`
}

// RunPlan selects checks and an exploration budget for one language.
//
//uc:realizes lang.RunPlan
type RunPlan struct {
	Root    string        `json:"root"`
	Roots   []string      `json:"roots"`
	Graph   *graph.Graph  `json:"graph"`
	Ids     []string      `json:"ids"`
	Explore time.Duration `json:"explore"`
}

// RunResult records one check's effective inputs, observations and failures.
//
//uc:realizes lang.RunResult
type RunResult struct {
	Id           string            `json:"id"`
	Passed       bool              `json:"passed"`
	Accepted     int               `json:"accepted"`
	Rejected     int               `json:"rejected"`
	Observations []string          `json:"observations"`
	Differences  []string          `json:"differences"`
	Diagnostics  []diag.Diagnostic `json:"diagnostics"`
}

// RunReport combines check results and toolchain diagnostics.
//
//uc:realizes lang.RunReport
type RunReport struct {
	Results     []RunResult       `json:"results"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// Generator is the optional generation capability of a language backend.
//
//uc:realizes lang.Generator
type Generator interface {
	Language() string
	Generate(plan GenPlan) Generation
}

// Runner is the optional execution capability of a language backend.
//
//uc:realizes lang.Runner
type Runner interface {
	Language() string
	Run(plan RunPlan) RunReport
}

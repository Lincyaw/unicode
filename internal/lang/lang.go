package lang

import (
	"fmt"
	"time"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/monitor"
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
	Path       string `json:"path"`
	Content    []byte `json:"content"`
	CreateOnly bool   `json:"createOnly"`
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

// RunPlan selects checks and an exploration budget for one language: a
// duration, as unicode test --explore gives it, or a number of generated
// candidates, whose outcome no machine's speed changes.
//
//uc:realizes lang.RunPlan
type RunPlan struct {
	Root         string        `json:"root"`
	Roots        []string      `json:"roots"`
	Graph        *graph.Graph  `json:"graph"`
	Ids          []string      `json:"ids"`
	Explore      time.Duration `json:"explore"`
	Explorations int           `json:"explorations"`
	Record       bool          `json:"record"`
	// Traffic gives, by property or flow, the inputs unicode test --traffic
	// takes from collected calls (docs/language.md E29).
	Traffic map[string][]Input `json:"traffic"`
}

// Exploring tells whether the plan explores generated inputs.
func (p RunPlan) Exploring() bool { return p.Explore > 0 || p.Explorations > 0 }

// Value, ValueEntry and Input are the model values of docs/design.md §6.4,
// which the monitor package owns: a monitored program writes them, and the
// toolchain reads them with the same types.
type (
	Value      = monitor.Value
	ValueEntry = monitor.ValueEntry
	Input      = monitor.Input
)

// Coverage is a measured source block or design step, branch or declared error.
//
//uc:realizes lang.Coverage
type Coverage struct {
	Kind      string `json:"kind"`
	Node      string `json:"node"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine"`
	EndColumn int    `json:"endColumn"`
	Label     string `json:"label"`
}

// TraceEvent is an observed runtime event with its actual goroutine lineage.
//
//uc:realizes lang.TraceEvent
type TraceEvent struct {
	Kind      string `json:"kind"`
	Node      string `json:"node"`
	Goroutine int    `json:"goroutine"`
	Parent    int    `json:"parent"`
	Time      int    `json:"time"`
	Detail    string `json:"detail"`
}

// CaseResult is a final candidate result, independent of minimization attempts.
//
//uc:realizes lang.CaseResult
type CaseResult struct {
	Id           string            `json:"id"`
	Check        string            `json:"check"`
	Form         string            `json:"form"`
	Reason       string            `json:"reason"`
	Input        Input             `json:"input"`
	InputHash    string            `json:"inputHash"`
	Passed       bool              `json:"passed"`
	Rejected     bool              `json:"rejected"`
	Coverage     []Coverage        `json:"coverage"`
	Trace        []TraceEvent      `json:"trace"`
	Observations []string          `json:"observations"`
	Differences  []string          `json:"differences"`
	Diagnostics  []diag.Diagnostic `json:"diagnostics"`
	Duration     int               `json:"duration"` // nanoseconds
	Calls        []CallTime        `json:"calls"`
	// Clause is the clause, limit or precondition that does not hold: of a
	// runtime counterexample, or of a collected call outside the domain.
	Clause string `json:"clause,omitempty"`
	// At is when the call of a runtime counterexample began.
	At string `json:"at,omitempty"`
}

// CallTime is one call of a design operation that a case made, the call
// under test or one of a sequence's, and how long it took.
//
//uc:realizes lang.CallTime
type CallTime struct {
	Op       string `json:"op"`
	Duration int    `json:"duration"` // nanoseconds
}

// CandidateEvaluation distinguishes a real result, budget exhaustion and tool errors.
//
//uc:realizes lang.CandidateEvaluation
type CandidateEvaluation struct {
	Result      *CaseResult       `json:"result"`
	Exhausted   bool              `json:"exhausted"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// CandidateRunner executes one reconstructed input at an isolated boundary.
//
//uc:realizes lang.CandidateRunner
type CandidateRunner interface {
	Evaluate(input Input) CandidateEvaluation
}

// ClauseCalls and ClauseJudged are a runtime conclusion's counts, which the
// monitor package owns.
type (
	ClauseCalls  = monitor.ClauseCalls
	ClauseJudged = monitor.ClauseJudged
)

// RunResult records one check's effective inputs, observations and failures.
// A runtime result counts its applicable calls as accepted and adds the calls
// outside the domain, how many times each clause was judged, the 95th
// percentile of the durations, in nanoseconds, and the traces checked.
//
//uc:realizes lang.RunResult
type RunResult struct {
	Id            string            `json:"id"`
	Passed        bool              `json:"passed"`
	Accepted      int               `json:"accepted"`
	Rejected      int               `json:"rejected"`
	Observations  []string          `json:"observations"`
	Differences   []string          `json:"differences"`
	Diagnostics   []diag.Diagnostic `json:"diagnostics"`
	Cases         []CaseResult      `json:"cases"`
	CodeTotal     int               `json:"codeTotal,omitempty"`
	BranchTotal   int               `json:"branchTotal,omitempty"`
	ErrorTotal    int               `json:"errorTotal,omitempty"`
	OutsideDomain []ClauseCalls     `json:"outsideDomain,omitempty"`
	Clauses       []ClauseJudged    `json:"clauses,omitempty"`
	P95           int               `json:"p95,omitempty"`
	Traces        int               `json:"traces,omitempty"`
}

// RunReport combines check results and toolchain diagnostics. Venue is where
// the checks ran (docs/model.md §3.5): test, written by unicode test, or
// runtime, written by a monitored program, whose record has no snapshot and
// no mode and adds From, the start of its record period. A report without a
// venue was written by unicode test.
//
//uc:realizes lang.RunReport
type RunReport struct {
	Results     []RunResult       `json:"results"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
	Version     int               `json:"version"`
	Snapshot    string            `json:"snapshot"`
	Commit      string            `json:"commit"`
	Dirty       bool              `json:"dirty"`
	At          string            `json:"at"`
	Mode        string            `json:"mode"`
	Venue       string            `json:"venue,omitempty"`
	From        string            `json:"from,omitempty"`
}

// The venues of a report (docs/model.md §3.5).
const (
	VenueTest    = "test"
	VenueRuntime = "runtime"
)

// Generator is the optional generation capability of a language backend.
//
//uc:realizes lang.Generator
type Generator interface {
	Language() string
	Generate(plan GenPlan) Generation
}

// GenerationExtractor is the optional extraction path for repairing stale
// generated output without changing the files on disk.
//
//uc:realizes lang.GenerationExtractor
type GenerationExtractor interface {
	Language() string
	ExtractForGeneration(root string, srcRoots []string) (Extracted, error)
}

// Runner is the optional execution capability of a language backend.
//
//uc:realizes lang.Runner
type Runner interface {
	Language() string
	Run(plan RunPlan) RunReport
}

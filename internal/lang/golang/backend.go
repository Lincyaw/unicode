package golang

import (
	"github.com/lincyaw/unicode/internal/lang"
)

const language = "go"

// Backend is the Go backend. It reads the code with go/packages and go/types
// and never runs it.
//
//uc:realizes lang.golang.Backend
type Backend struct{}

var _ lang.Backend = (*Backend)(nil)
var _ lang.GenerationExtractor = (*Backend)(nil)

// NewBackend returns a Go backend.
//
//uc:realizes lang.golang.newBackend
func NewBackend() *Backend { return &Backend{} }

func (b *Backend) Language() string { return language }

// Extract loads the packages under the source roots, their test files
// included, and writes their code layer. Code that does not load is an
// ExtractFailed; bindings that are not well-formed are diagnostics.
func (b *Backend) Extract(root string, srcRoots []string) (lang.Extracted, error) {
	return cachedExtraction(root, srcRoots, false)
}

// ExtractForGeneration loads the code used to plan generation, omitting only
// this tool's existing generated tests through an in-memory package overlay.
func (b *Backend) ExtractForGeneration(root string, srcRoots []string) (lang.Extracted, error) {
	return cachedExtraction(root, srcRoots, true)
}

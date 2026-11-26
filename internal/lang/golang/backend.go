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

// NewBackend returns a Go backend.
//
//uc:realizes lang.golang.newBackend
func NewBackend() *Backend { return &Backend{} }

func (b *Backend) Language() string { return language }

// Extract loads the packages under the source roots, their test files
// included, and writes their code layer. Code that does not load is an
// ExtractFailed; bindings that are not well-formed are diagnostics.
func (b *Backend) Extract(root string, srcRoots []string) (lang.Extracted, error) {
	loaded, err := load(root, srcRoots)
	if err != nil {
		return lang.Extracted{}, lang.ExtractFailed{Language: language, Root: root, Cause: err}
	}
	return extract(loaded), nil
}

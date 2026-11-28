package golang

import (
	"encoding/json"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// cachedExtraction extracts the code under the source roots through the
// shared content cache.
func cachedExtraction(root string, srcRoots []string, forGeneration bool) (lang.Extracted, error) {
	cache, err := sharedCache()
	if err != nil {
		return lang.Extracted{}, err
	}
	extracted, _, err := cache.extraction(root, srcRoots, forGeneration)
	return extracted, err
}

// extraction extracts the code under the source roots, reading the result
// from the cache when the inputs of the load are those of an earlier
// extraction in the same mode (docs/design.md §6.3), and tells whether it
// did. The entry is found by goInputKey and answered only while the files
// the load read (readInputs) are unchanged. Only extractions that succeed
// are kept, and only when neither the key nor those files changed during
// the load.
func (c contentCache) extraction(root string, srcRoots []string, forGeneration bool) (lang.Extracted, bool, error) {
	const kind = cacheExtraction
	mode := "extract"
	if forGeneration {
		mode = "extract-for-generation"
	}
	if c.dir == "" {
		extracted, err := freshExtraction(root, srcRoots, forGeneration)
		return extracted, false, err
	}
	key, keyed := goInputKey(root, srcRoots, mode, nil)
	if !keyed {
		extracted, err := freshExtraction(root, srcRoots, forGeneration)
		return extracted, false, err
	}
	value, hit, err := c.getChecked(kind, key, root, nil)
	if err != nil {
		return lang.Extracted{}, false, err
	}
	if hit {
		extracted, err := decodeExtraction(value)
		if err != nil {
			return lang.Extracted{}, false, c.corrupt(c.entryPath(kind, key), err)
		}
		return extracted, true, nil
	}
	// The inputs are listed while the load runs; both only read.
	listed := make(chan inputSet, 1)
	go func() {
		inputs, err := readInputs(root, srcRoots, forGeneration, nil)
		if err != nil {
			inputs = nil
		}
		listed <- inputs
	}()
	extracted, err := freshExtraction(root, srcRoots, forGeneration)
	inputs := <-listed
	if err != nil || inputs == nil {
		return extracted, false, err
	}
	if after, ok := goInputKey(root, srcRoots, mode, nil); ok && after == key && inputs.current(root, nil) {
		c.putChecked(kind, key, inputs, encodeExtraction(extracted))
	}
	return extracted, false, nil
}

// freshExtraction loads and extracts without the cache.
func freshExtraction(root string, srcRoots []string, forGeneration bool) (lang.Extracted, error) {
	loaded, err := loadPackages(root, srcRoots, forGeneration)
	if err != nil {
		return lang.Extracted{}, lang.ExtractFailed{Language: language, Root: root, Cause: err}
	}
	extracted, err := extract(loaded)
	if err != nil {
		return lang.Extracted{}, lang.ExtractFailed{Language: language, Root: root, Cause: err}
	}
	return extracted, nil
}

// The cached form keeps every map and slice as it was, empty or absent, so
// that a decoded extraction is deeply equal to the one encoded; the
// snapshot's JSON leaves empty attributes out and would not. Each type has
// exactly the fields of the one it stands for, so a field added there stops
// the conversion from compiling until it is added here.
type cachedExtracted struct {
	Nodes       []cachedNode
	Edges       []cachedEdge
	Diagnostics []cachedDiagnostic
}

type cachedNode struct {
	Id      string
	Kind    graph.NodeKind
	Form    string
	Layer   graph.Layer
	Source  graph.Source
	Origin  diag.Position
	Summary string
	Salient bool
	Attrs   map[string][]string
}

type cachedEdge struct {
	Kind   graph.EdgeKind
	From   string
	To     string
	Source graph.Source
	Origin diag.Position
	Attrs  map[string][]string
}

type cachedDiagnostic struct {
	Code     string
	Subject  string
	Position diag.Position
	Message  string
	Fix      string
}

// encodeExtraction is the cached form of an extraction.
func encodeExtraction(x lang.Extracted) cachedExtracted {
	return cachedExtracted{
		Nodes:       convertAll(x.Fragment.Nodes, func(n graph.Node) cachedNode { return cachedNode(n) }),
		Edges:       convertAll(x.Fragment.Edges, func(e graph.Edge) cachedEdge { return cachedEdge(e) }),
		Diagnostics: convertAll(x.Diagnostics, func(d diag.Diagnostic) cachedDiagnostic { return cachedDiagnostic(d) }),
	}
}

func decodeExtraction(data []byte) (lang.Extracted, error) {
	var c cachedExtracted
	if err := json.Unmarshal(data, &c); err != nil {
		return lang.Extracted{}, err
	}
	return lang.Extracted{
		Fragment: graph.Fragment{
			Nodes: convertAll(c.Nodes, func(n cachedNode) graph.Node { return graph.Node(n) }),
			Edges: convertAll(c.Edges, func(e cachedEdge) graph.Edge { return graph.Edge(e) }),
		},
		Diagnostics: convertAll(c.Diagnostics, func(d cachedDiagnostic) diag.Diagnostic { return diag.Diagnostic(d) }),
	}, nil
}

// convertAll converts each element, keeping a nil slice nil and an empty one empty.
func convertAll[T, U any](in []T, convert func(T) U) []U {
	if in == nil {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = convert(v)
	}
	return out
}

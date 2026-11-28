package golang

import (
	"bytes"
	"compress/gzip"
	"encoding/json"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// cachedExtraction extracts the code under the source roots through the
// shared content cache.
func cachedExtraction(root string, srcRoots []string, forGeneration bool) (lang.Extracted, error) {
	extracted, _, err := sharedCache().extraction(root, srcRoots, forGeneration)
	return extracted, err
}

// extraction extracts the code under the source roots, reading the result
// from the cache when the inputs of the load are those of an earlier
// extraction in the same mode (docs/design.md §6.3), and tells whether it
// did. Only extractions that succeed are kept; the key is taken again after
// the load and the result kept only if no input changed meanwhile.
func (c contentCache) extraction(root string, srcRoots []string, forGeneration bool) (lang.Extracted, bool, error) {
	const kind = cacheExtraction
	mode := "extract"
	if forGeneration {
		mode = "extract-for-generation"
	}
	key, keyed := goInputKey(root, srcRoots, mode, nil)
	if keyed {
		if data, hit := c.get(kind, key); hit {
			if extracted, err := decodeExtraction(data); err == nil {
				return extracted, true, nil
			}
		}
	}
	extracted, err := freshExtraction(root, srcRoots, forGeneration)
	if err != nil || !keyed {
		return extracted, false, err
	}
	if after, ok := goInputKey(root, srcRoots, mode, nil); ok && after == key {
		if data, err := encodeExtraction(extracted); err == nil {
			c.put(kind, key, data)
		}
	}
	return extracted, false, nil
}

// freshExtraction loads and extracts without the cache.
func freshExtraction(root string, srcRoots []string, forGeneration bool) (lang.Extracted, error) {
	loaded, err := loadPackages(root, srcRoots, forGeneration)
	if err != nil {
		return lang.Extracted{}, lang.ExtractFailed{Language: language, Root: root, Cause: err}
	}
	return extract(loaded), nil
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

// encodeExtraction writes an extraction as gzip-compressed JSON, about a
// tenth of its plain size.
func encodeExtraction(x lang.Extracted) ([]byte, error) {
	var out bytes.Buffer
	zipped, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(zipped).Encode(cachedExtracted{
		Nodes:       convertAll(x.Fragment.Nodes, func(n graph.Node) cachedNode { return cachedNode(n) }),
		Edges:       convertAll(x.Fragment.Edges, func(e graph.Edge) cachedEdge { return cachedEdge(e) }),
		Diagnostics: convertAll(x.Diagnostics, func(d diag.Diagnostic) cachedDiagnostic { return cachedDiagnostic(d) }),
	}); err != nil {
		return nil, err
	}
	if err := zipped.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func decodeExtraction(data []byte) (lang.Extracted, error) {
	zipped, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return lang.Extracted{}, err
	}
	var c cachedExtracted
	if err := json.NewDecoder(zipped).Decode(&c); err != nil {
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

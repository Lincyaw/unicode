package graph

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"

	"github.com/lincyaw/unicode/internal/diag"
)

// Digest returns the SHA-256, in lower-case hex, of the content of the node
// with the identity id: its kind, form, summary and attrs, without its
// children (docs/model.md §6). It returns "" when the graph has no such node.
func (g *Graph) Digest(id string) string {
	n, ok := g.nodes[id]
	if !ok {
		return ""
	}
	sum := sha256.Sum256(contentEncoding(n))
	return hex.EncodeToString(sum[:])
}

// contentEncoding writes the content of a node so that two contents are
// equal exactly when their encodings are: every string is prefixed by its
// length, every list by its count, and attrs come sorted by key. An absent
// key and a key with an empty list differ; a nil and an empty list do not.
func contentEncoding(n Node) []byte {
	var b []byte
	str := func(s string) {
		b = binary.AppendUvarint(b, uint64(len(s)))
		b = append(b, s...)
	}
	str(string(n.Kind))
	str(n.Form)
	str(n.Summary)
	b = binary.AppendUvarint(b, uint64(len(n.Attrs)))
	for _, k := range slices.Sorted(maps.Keys(n.Attrs)) {
		str(k)
		b = binary.AppendUvarint(b, uint64(len(n.Attrs[k])))
		for _, v := range n.Attrs[k] {
			str(v)
		}
	}
	return b
}

// The snapshot's JSON: nodes sorted by identity, edges by kind, start and
// end, attrs by key (encoding/json sorts map keys), every list present.
type snapshot struct {
	Nodes []nodeJSON `json:"nodes"`
	Edges []edgeJSON `json:"edges"`
}

type nodeJSON struct {
	Id      string              `json:"id"`
	Kind    NodeKind            `json:"kind"`
	Form    string              `json:"form"`
	Layer   Layer               `json:"layer"`
	Source  Source              `json:"source"`
	Origin  originJSON          `json:"origin"`
	Summary string              `json:"summary"`
	Salient bool                `json:"salient"`
	Digest  string              `json:"digest"`
	Attrs   map[string][]string `json:"attrs"`
}

type edgeJSON struct {
	Kind   EdgeKind            `json:"kind"`
	From   string              `json:"from"`
	To     string              `json:"to"`
	Source Source              `json:"source"`
	Origin originJSON          `json:"origin"`
	Attrs  map[string][]string `json:"attrs"`
}

type originJSON struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Encode writes the graph as its JSON snapshot (docs/model.md §8): the same
// nodes and edges give the same bytes, whatever order they were added in.
func (g *Graph) Encode() []byte {
	s := snapshot{Nodes: []nodeJSON{}, Edges: []edgeJSON{}}
	for _, n := range g.Nodes() {
		s.Nodes = append(s.Nodes, nodeJSON{
			Id: n.Id, Kind: n.Kind, Form: n.Form, Layer: n.Layer, Source: n.Source,
			Origin:  originJSON{n.Origin.Path, n.Origin.Line, n.Origin.Column},
			Summary: n.Summary, Salient: n.Salient, Digest: g.Digest(n.Id), Attrs: presentAttrs(n.Attrs),
		})
	}
	for _, e := range g.Edges() {
		s.Edges = append(s.Edges, edgeJSON{
			Kind: e.Kind, From: e.From, To: e.To, Source: e.Source,
			Origin: originJSON{e.Origin.Path, e.Origin.Line, e.Origin.Column}, Attrs: presentAttrs(e.Attrs),
		})
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		// Strings, integers, lists and maps of strings always encode.
		panic("graph: cannot encode the snapshot: " + err.Error())
	}
	return out.Bytes()
}

// presentAttrs writes a missing map as {} and a nil list as [], never null.
func presentAttrs(attrs map[string][]string) map[string][]string {
	out := make(map[string][]string, len(attrs))
	for k, v := range attrs {
		out[k] = append([]string{}, v...)
	}
	return out
}

// NotASnapshot is the error of Decode for bytes that are not a snapshot
// Encode wrote: not its JSON, or nodes and edges that make no graph.
//
//uc:realizes graph.NotASnapshot
type NotASnapshot struct{ Cause error }

func (e NotASnapshot) Error() string { return "not a snapshot of the graph: " + e.Cause.Error() }

func (e NotASnapshot) Unwrap() error { return e.Cause }

// Decode is the inverse of Encode: it rebuilds the graph a snapshot holds,
// adding its nodes, then its edges. The digests are not read: Digest
// computes them from the content. Bytes that are not one JSON object of the
// snapshot's fields, or whose nodes and edges make no graph, give NotASnapshot.
//
//uc:realizes graph.decode
func Decode(data []byte) (Graph, error) {
	var s snapshot
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Graph{}, NotASnapshot{Cause: err}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Graph{}, NotASnapshot{Cause: errors.New("more data follows the snapshot's JSON object")}
	}
	g := NewGraph()
	for _, n := range s.Nodes {
		node := Node{Id: n.Id, Kind: n.Kind, Form: n.Form, Layer: n.Layer, Source: n.Source,
			Origin: diag.Position(n.Origin), Summary: n.Summary, Salient: n.Salient, Attrs: n.Attrs}
		if err := g.AddNode(node); err != nil {
			return Graph{}, NotASnapshot{Cause: err}
		}
	}
	for _, e := range s.Edges {
		edge := Edge{Kind: e.Kind, From: e.From, To: e.To, Source: e.Source, Origin: diag.Position(e.Origin), Attrs: e.Attrs}
		if err := g.AddEdge(edge); err != nil {
			return Graph{}, NotASnapshot{Cause: err}
		}
	}
	return g, nil
}

package graph

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
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
			Summary: n.Summary, Digest: g.Digest(n.Id), Attrs: presentAttrs(n.Attrs),
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

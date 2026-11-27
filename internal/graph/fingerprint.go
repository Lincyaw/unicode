package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// EdgeDigest hashes an edge's identity and attributes, as Compare compares it.
//
//uc:realizes graph.edgeDigest
func EdgeDigest(edge Edge) string {
	data, err := json.Marshal(struct {
		Kind  EdgeKind            `json:"kind"`
		From  string              `json:"from"`
		To    string              `json:"to"`
		Attrs map[string][]string `json:"attrs"`
	}{edge.Kind, edge.From, edge.To, presentAttrs(edge.Attrs)})
	if err != nil {
		panic("graph: cannot encode edge content: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// InputDigest anchors runtime evidence to the actual source positions while
// ignoring cases and generated outputs, which recording itself may update.
//
//uc:realizes graph.inputDigest
func InputDigest(g *Graph) string {
	omit := map[string]bool{}
	for _, node := range g.Nodes() {
		if node.Layer == LayerEvidence || node.Layer == LayerReview || node.Kind == NodeKindCase ||
			node.Layer == LayerCode && slices.Contains(node.Attrs["generated"], "true") {
			omit[node.Id] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, edge := range g.Edges() {
			if edge.Kind == EdgeKindContains && omit[edge.From] && !omit[edge.To] {
				omit[edge.To], changed = true, true
			}
		}
	}
	// Filtering preserves the original unique identities and existing endpoints.
	input := NewGraph()
	for _, node := range g.Nodes() {
		if !omit[node.Id] {
			_ = input.AddNode(node)
		}
	}
	for _, edge := range g.Edges() {
		if !omit[edge.From] && !omit[edge.To] && edge.Kind != EdgeKindCovers {
			_ = input.AddEdge(edge)
		}
	}
	sum := sha256.Sum256(input.Encode())
	return hex.EncodeToString(sum[:])
}

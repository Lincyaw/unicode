package graph

import (
	"slices"

	"pgregory.net/rapid"
)

// genNoise draws bytes that are often near JSON: brackets, quotes, the
// snapshot's field names and digits, and now and then any byte.
//
//uc:generates graph.decode-is-total.noise
func genNoise() *rapid.Generator[[]byte] {
	pieces := []string{"{", "}", "[", "]", `"`, ":", ",", " ", "0", "-1", "null", "true", `"nodes"`, `"edges"`, `"id"`, `"a"`, `"attrs"`, `"origin"`}
	return rapid.Custom(func(t *rapid.T) []byte {
		var b []byte
		for range rapid.IntRange(0, 12).Draw(t, "pieces") {
			if rapid.IntRange(0, 5).Draw(t, "anyByte?") == 0 {
				b = append(b, rapid.Byte().Draw(t, "byte"))
				continue
			}
			b = append(b, rapid.SampledFrom(pieces).Draw(t, "piece")...)
		}
		return b
	})
}

// deriveNoisyData is the noise alone, or the noise spliced into the snapshot
// of content where its length points.
//
//uc:generates graph.decode-is-total.data
func deriveNoisyData(content Fragment, noise []byte, alone bool) ([]byte, error) {
	if alone {
		return slices.Clone(noise), nil
	}
	g, err := graphOf(content.Nodes, content.Edges)
	if err != nil {
		return nil, err
	}
	snapshot := g.Encode()
	at := len(noise) % (len(snapshot) + 1)
	return slices.Concat(snapshot[:at], noise, snapshot[at:]), nil
}

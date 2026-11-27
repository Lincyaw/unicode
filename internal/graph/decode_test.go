package graph

import (
	"bytes"
	"errors"

	"pgregory.net/rapid"
)

//uc:checks graph.decode-inverts-encode
func checkDecodeInvertsEncode(t rapid.TB, content Fragment, noise []byte) {
	g := build(t, content.Nodes, content.Edges)
	snapshot := g.Encode()
	decoded, err := Decode(snapshot)
	if err != nil {
		t.Fatalf("Decode refused the snapshot that Encode wrote: %v\n%s", err, snapshot)
	}
	if again := decoded.Encode(); !bytes.Equal(again, snapshot) {
		t.Fatalf("the graph Decode rebuilt encodes differently from the snapshot it read\nread:\n%s\nencoded again:\n%s", snapshot, again)
	}
	// The noise alone, and spliced into the snapshot where its length points.
	at := len(noise) % (len(snapshot) + 1)
	spliced := append(append(append([]byte{}, snapshot[:at]...), noise...), snapshot[at:]...)
	for _, data := range [][]byte{noise, spliced} {
		if _, err := Decode(data); err != nil && !errors.As(err, new(NotASnapshot)) {
			t.Fatalf("Decode refused the bytes %q with %T, not with NotASnapshot: %v", data, err, err)
		}
	}
}

// genNoise draws bytes that are often near JSON: brackets, quotes, the
// snapshot's field names and digits, and now and then any byte.
//
//uc:generates graph.decode-inverts-encode.noise
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

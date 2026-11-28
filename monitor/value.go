package monitor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// Value is a model value in the canonical encoding of docs/design.md §6.4,
// the JSON of lang.Value: kind decides whether text, items or entries hold
// it. A scalar's items and entries are null; a list's or set's items and a
// record's, map's or call's entries are [] when empty. A redacted value has
// the kind redacted and the digest as its text.
//
//uc:realizes monitor.Value
type Value struct {
	Kind    string       `json:"kind"`
	Text    string       `json:"text"`
	Items   []Value      `json:"items"`
	Entries []ValueEntry `json:"entries"`
}

// ValueEntry is one entry of a record, a map or a call value.
//
//uc:realizes monitor.ValueEntry
type ValueEntry struct {
	Key   Value `json:"key"`
	Value Value `json:"value"`
}

// Input is the frozen input of one call: each symbol's model value.
//
//uc:realizes monitor.Input
type Input struct {
	Values map[string]Value `json:"values"`
}

// canonical is the JSON of a value, as the evidence store encodes it.
func canonical(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		// Values are plain strings, slices and maps of them: they always encode.
		panic("monitor: cannot encode a model value: " + err.Error())
	}
	return data
}

// digest is the lowercase hexadecimal SHA-256 of a value's canonical JSON.
func digest(v any) string {
	sum := sha256.Sum256(canonical(v))
	return hex.EncodeToString(sum[:])
}

// InputHash is the hash of an input as the evidence store computes it: the
// SHA-256 of the canonical JSON of its values, without the outer values key.
func InputHash(values map[string]Value) string {
	if values == nil {
		values = map[string]Value{}
	}
	return digest(values)
}

// sortItems orders a set's items by their canonical encoding.
func sortItems(items []Value) {
	slices.SortFunc(items, func(a, b Value) int { return bytes.Compare(canonical(a), canonical(b)) })
}

// sortEntries orders a map's, record's or call's entries by their keys'
// canonical encoding.
func sortEntries(entries []ValueEntry) {
	slices.SortFunc(entries, func(a, b ValueEntry) int { return bytes.Compare(canonical(a.Key), canonical(b.Key)) })
}

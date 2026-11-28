package monitor

import (
	"encoding/json"
	"time"
)

// Collected is a collected call (docs/design.md §6.6): the identity of the
// operation called, the receiver's model value for an operation of a type,
// the call value operation(parameter: value, …), the commit the program was
// built from, and when it was collected. Its values are redacted.
//
//uc:realizes monitor.Collected
type Collected struct {
	Op       string    `json:"op"`
	Receiver *Value    `json:"receiver"`
	Call     Value     `json:"call"`
	Commit   string    `json:"commit"`
	At       time.Time `json:"at"`
}

// EncodeCollected is the JSON of a collected call that traffic/ holds.
//
//uc:realizes monitor.encodeCollected
func EncodeCollected(collected Collected) []byte {
	collected.At = collected.At.UTC()
	data, err := json.Marshal(collected)
	if err != nil {
		// A collected call is strings, times and model values: it always encodes.
		panic("monitor: cannot encode a collected call: " + err.Error())
	}
	return data
}

// collectedHash is the input hash a collected call is stored under: that of
// its receiver and its call value.
func collectedHash(collected Collected) string {
	values := map[string]Value{"call": collected.Call}
	if collected.Receiver != nil {
		values["receiver"] = *collected.Receiver
	}
	return InputHash(values)
}

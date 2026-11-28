package monitor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Rates are the sampling rates of the operations: the fallback rate, and the
// rates that UNICODE_MONITOR_RATE gives operations by their identity.
//
//uc:realizes monitor.Rates
type Rates struct {
	Fallback float64            `json:"fallback"`
	Ops      map[string]float64 `json:"ops"`
}

// BadRates is the error of a UNICODE_MONITOR_RATE text that is not
// fallback[,operation=rate]... or gives a rate outside 0 to 1.
//
//uc:realizes monitor.BadRates
type BadRates struct {
	Text   string
	Reason string
}

func (e BadRates) Error() string {
	return fmt.Sprintf("UNICODE_MONITOR_RATE %q: %s; write fallback[,operation=rate]... with rates from 0 to 1", e.Text, e.Reason)
}

// defaultRate is the fallback rate when UNICODE_MONITOR_RATE is not set.
const defaultRate = 0.01

// ParseRates reads a UNICODE_MONITOR_RATE text, fallback[,operation=rate]...:
// the first item is the fallback rate, each other item one operation's rate.
//
//uc:realizes monitor.parseRates
func ParseRates(text string) (Rates, error) {
	items := strings.Split(text, ",")
	fallback, err := rate(items[0])
	if err != nil {
		return Rates{}, BadRates{text, "the fallback rate: " + err.Error()}
	}
	out := Rates{Fallback: fallback, Ops: map[string]float64{}}
	for _, item := range items[1:] {
		op, value, ok := strings.Cut(item, "=")
		op = strings.TrimSpace(op)
		if !ok || op == "" {
			return Rates{}, BadRates{text, fmt.Sprintf("the item %q is not operation=rate", item)}
		}
		if _, seen := out.Ops[op]; seen {
			return Rates{}, BadRates{text, "the operation " + op + " has two rates"}
		}
		if out.Ops[op], err = rate(value); err != nil {
			return Rates{}, BadRates{text, "the rate of " + op + ": " + err.Error()}
		}
	}
	return out, nil
}

// rate reads one rate, a finite number from 0 to 1.
func rate(text string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	switch {
	case err != nil:
		return 0, fmt.Errorf("%q is not a number", text)
	case math.IsNaN(value) || value < 0 || value > 1:
		return 0, fmt.Errorf("%q is not from 0 to 1", text)
	}
	return value, nil
}

// Sampled tells whether a call of op is sampled, given draw drawn uniformly
// from [0, 1): it is when draw is below op's rate, which is rates.Ops[op] when
// op has one, and rates.Fallback otherwise.
//
//uc:realizes monitor.sampled
func Sampled(rates Rates, op string, draw float64) bool {
	rate, ok := rates.Ops[op]
	if !ok {
		rate = rates.Fallback
	}
	return draw < rate
}

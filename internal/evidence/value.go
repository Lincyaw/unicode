package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
)

var decimal = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)
var integer = regexp.MustCompile(`^-?[0-9]+$`)

func canonical(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic("evidence: cannot encode internal model data: " + err.Error())
	}
	return data
}

func hash(value any) string {
	sum := sha256.Sum256(canonical(value))
	return hex.EncodeToString(sum[:])
}

// NormalizeInput freezes values, canonicalizes unordered containers and preserves
// arbitrary Go string bytes without JSON's UTF-8 replacement.
func NormalizeInput(input lang.Input) (lang.Input, error) {
	out := lang.Input{Values: map[string]lang.Value{}}
	for name, value := range input.Values {
		if name == "" || !utf8.ValidString(name) {
			return out, fmt.Errorf("the symbol name is empty or not UTF-8")
		}
		normalized, err := normalizeValue(value, 0)
		if err != nil {
			return out, fmt.Errorf("symbol %s: %w", name, err)
		}
		out.Values[name] = normalized
	}
	return out, nil
}

// InputHash hashes only the canonical symbol-to-value object, without an envelope.
func InputHash(input lang.Input) (string, error) {
	normalized, err := NormalizeInput(input)
	if err != nil {
		return "", err
	}
	return hash(normalized.Values), nil
}

func normalizeValue(value lang.Value, depth int) (lang.Value, error) {
	if depth > 128 {
		return lang.Value{}, fmt.Errorf("the model value exceeds the finite nesting budget")
	}
	out := lang.Value{Kind: value.Kind}
	switch value.Kind {
	case "none":
	case "bool":
		if value.Text != "true" && value.Text != "false" {
			return out, fmt.Errorf("a bool needs true or false")
		}
		out.Text = value.Text
	case "int":
		if !integer.MatchString(value.Text) {
			return out, fmt.Errorf("an int needs exact decimal text")
		}
		n, ok := new(big.Int).SetString(value.Text, 10)
		if !ok {
			return out, fmt.Errorf("invalid integer")
		}
		out.Text = n.String()
	case "float":
		if !decimal.MatchString(value.Text) {
			return out, fmt.Errorf("a float needs finite non-exponent decimal text")
		}
		n, ok := new(big.Rat).SetString(value.Text)
		if !ok {
			return out, fmt.Errorf("invalid finite decimal")
		}
		places := 0
		if point := strings.IndexByte(value.Text, '.'); point >= 0 {
			places = len(value.Text) - point - 1
		}
		out.Text = n.FloatString(places)
		if strings.Contains(out.Text, ".") {
			out.Text = strings.TrimRight(strings.TrimRight(out.Text, "0"), ".")
		}
		if n.Sign() == 0 && strings.HasPrefix(value.Text, "-") {
			out.Text = "-0"
		}
	case "string":
		out.Text = value.Text
		if !utf8.ValidString(value.Text) {
			out.Kind = "stringBytes"
			out.Text = base64.StdEncoding.EncodeToString([]byte(value.Text))
		}
	case "stringBytes", "bytes":
		raw, err := base64.StdEncoding.Strict().DecodeString(value.Text)
		if err != nil {
			return out, fmt.Errorf("%s needs canonical Base64 bytes: %w", value.Kind, err)
		}
		out.Text = base64.StdEncoding.EncodeToString(raw)
		if value.Kind == "stringBytes" && utf8.Valid(raw) {
			out.Kind = "string"
			out.Text = string(raw)
		}
	case "time":
		stamp, err := time.Parse(time.RFC3339Nano, value.Text)
		if err != nil {
			return out, fmt.Errorf("invalid time: %w", err)
		}
		out.Text = stamp.UTC().Format(time.RFC3339Nano)
	case "duration":
		span, err := time.ParseDuration(value.Text)
		if err != nil {
			return out, fmt.Errorf("invalid duration: %w", err)
		}
		out.Text = span.String()
	case "enum":
		if value.Text == "" || !utf8.ValidString(value.Text) {
			return out, fmt.Errorf("an enum needs its UTF-8 member name")
		}
		out.Text = value.Text
	case "list", "set", "encoded":
		if value.Kind == "encoded" && len(value.Items) != 1 {
			return out, fmt.Errorf("an encoded value needs exactly one representation")
		}
		out.Items = []lang.Value{}
		for _, child := range value.Items {
			normalized, err := normalizeValue(child, depth+1)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, normalized)
		}
		if value.Kind == "set" {
			slices.SortFunc(out.Items, func(a, b lang.Value) int { return bytes.Compare(canonical(a), canonical(b)) })
			out.Items = slices.CompactFunc(out.Items, func(a, b lang.Value) bool { return bytes.Equal(canonical(a), canonical(b)) })
		}
	case "map", "record", "call":
		if value.Kind == "call" {
			if value.Text == "" || !utf8.ValidString(value.Text) {
				return out, fmt.Errorf("a call needs its operation name")
			}
			out.Text = value.Text
		}
		out.Entries = []lang.ValueEntry{}
		for _, entry := range value.Entries {
			key, err := normalizeValue(entry.Key, depth+1)
			if err != nil {
				return out, err
			}
			if value.Kind != "map" && key.Kind != "string" {
				return out, fmt.Errorf("record fields and call parameters need UTF-8 string keys")
			}
			child, err := normalizeValue(entry.Value, depth+1)
			if err != nil {
				return out, err
			}
			out.Entries = append(out.Entries, lang.ValueEntry{Key: key, Value: child})
		}
		slices.SortFunc(out.Entries, func(a, b lang.ValueEntry) int { return bytes.Compare(canonical(a.Key), canonical(b.Key)) })
		for index := 1; index < len(out.Entries); index++ {
			if bytes.Equal(canonical(out.Entries[index-1].Key), canonical(out.Entries[index].Key)) {
				return out, fmt.Errorf("the model object repeats a key")
			}
		}
	default:
		return out, fmt.Errorf("unknown model value kind %q", value.Kind)
	}
	return out, nil
}

func cloneCase(result lang.CaseResult) (lang.CaseResult, error) {
	input, err := NormalizeInput(result.Input)
	if err != nil {
		return result, err
	}
	fingerprint := hash(input.Values)
	if result.InputHash != "" && result.InputHash != fingerprint {
		return result, fmt.Errorf("the input digest does not match its canonical values")
	}
	result.Input, result.InputHash = input, fingerprint
	if result.Form == "" {
		result.Form = "recorded"
	}
	if result.Form != "declared" && result.Form != "recorded" {
		return result, fmt.Errorf("the case form is neither declared nor recorded")
	}
	if strings.TrimSpace(result.Check) == "" {
		return result, fmt.Errorf("the case has no check identity")
	}
	if result.Id == "" {
		if result.Form == "declared" {
			return result, fmt.Errorf("a declared case needs its identity")
		}
		result.Id = result.Check + ".r-" + fingerprint[:8]
	}
	if result.Form == "recorded" {
		if result.Reason == "" {
			result.Reason = "coverage"
			if !result.Passed && !result.Rejected {
				result.Reason = "counterexample"
			}
		}
		if result.Reason != "coverage" && result.Reason != "counterexample" {
			return result, fmt.Errorf("a recorded case needs a coverage or counterexample reason")
		}
	}
	result.Coverage = append([]lang.Coverage{}, result.Coverage...)
	slices.SortFunc(result.Coverage, func(a, b lang.Coverage) int { return bytes.Compare(canonical(a), canonical(b)) })
	result.Coverage = slices.Compact(result.Coverage)
	result.Trace = append([]lang.TraceEvent{}, result.Trace...)
	result.Observations = append([]string{}, result.Observations...)
	result.Differences = append([]string{}, result.Differences...)
	result.Diagnostics = append([]diag.Diagnostic{}, result.Diagnostics...)
	result.Calls = append([]lang.CallTime{}, result.Calls...)
	if result.Duration < 0 || slices.ContainsFunc(result.Calls, func(c lang.CallTime) bool { return c.Duration < 0 || c.Op == "" }) {
		return result, fmt.Errorf("a case's duration and the durations of its calls cannot be negative, and every call names its operation")
	}
	return result, nil
}

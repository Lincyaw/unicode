package design

import (
	"fmt"
	"strconv"
	"strings"

	"pgregory.net/rapid"
)

// codecProgram writes a component whose type Box declares a codec and whose
// property gives an encoded case value derived from value, changed by
// variant: 0 is valid, with an int representation for odd values and a
// record one for even values; every other variant breaks one rule of
// docs/language.md §5.3.1 and comes with the code it must get.
func codecProgram(value, variant int) (string, string) {
	source := `component model "Codec fixture" {
  focus core
  paths "*.go"
}

type DecodeError error "Invalid representation"

type Box object "A private value" {
  codec encode, decode
  field n: int
  op encode() -> int pure
  op decode(other: string) -> int
}

op decode(value: int) -> Box raises DecodeError

property supplied invariant "A decoded value is checked at runtime" {
  about Box
  forall value: Box
  where value.n in 0..3
  manual "The runtime decodes the representation and checks its limit"
  case input {
    value = encoded(VALUE)
  }
}
`
	source = strings.ReplaceAll(source, "VALUE", strconv.Itoa(value+7001))
	code := "design.invalid-codec"
	switch variant {
	case 0:
		code = ""
		if value%2 == 0 {
			source = strings.Replace(source, "type Box object", "type Repr record \"A representation\" {\n  field amount: int\n}\n\ntype Box object", 1)
			source = strings.Replace(source, "op encode() -> int pure", "op encode() -> Repr pure", 1)
			source = strings.Replace(source, "op decode(value: int)", "op decode(value: Repr)", 1)
			source = strings.Replace(source, "encoded("+strconv.Itoa(value+7001)+")", "encoded({amount: "+strconv.Itoa(value+7001)+"})", 1)
		}
	case 1:
		source = strings.Replace(source, "  codec encode, decode\n", "  codec encode, decode\n  codec encode, decode\n", 1)
	case 2:
		source = strings.Replace(source, "type Box object", "type Box interface", 1)
		source = strings.Replace(source, "  field n: int\n", "", 1)
	case 3:
		source = strings.Replace(source, "codec encode, decode", "codec missing, decode", 1)
	case 4:
		source = strings.Replace(source, "op encode()", "op encode(value: int)", 1)
	case 5:
		source = strings.Replace(source, " -> int pure", " -> int", 1)
	case 6:
		source = strings.Replace(source, " -> int pure", " -> int pure raises DecodeError", 1)
	case 7:
		source = strings.Replace(source, "op decode(value: int)", "op decode(value: string)", 1)
	case 8:
		source = strings.Replace(source, "op decode(value: int) -> Box", "op decode(value: int) -> int", 1)
	case 9:
		source = strings.Replace(source, "codec encode, decode", "codec encode, Box.restore", 1)
		source = strings.Replace(source, "  op encode() -> int pure\n", "  op encode() -> int pure\n  op restore(value: int) -> Box\n", 1)
	case 10:
		source = strings.Replace(source, "op encode() -> int pure", "op encode() -> Box pure", 1)
		source = strings.Replace(source, "op decode(value: int)", "op decode(value: Box)", 1)
	case 11:
		source = strings.Replace(source, "  codec encode, decode\n", "", 1)
		code = "design.value-type-mismatch"
	case 12:
		source = strings.Replace(source, "encoded("+strconv.Itoa(value+7001)+")", "encoded(\"wrong\")", 1)
		code = "design.value-type-mismatch"
	case 13:
		source = strings.Replace(source, "op decode(value: int)", "type Child object \"No implicit codec\" {\n  extends Box\n}\n\nop decode(value: int)", 1)
		source = strings.Replace(source, "forall value: Box", "forall value: Child", 1)
		code = "design.value-type-mismatch"
	case 14:
		source = strings.Replace(source, "op decode(value: int)", "type Other object \"Another codec\" {\n  codec encode, decodeOther\n  op encode() -> Box pure\n}\n\nop decodeOther(value: Box) -> Other\n\nop decode(value: Other)", 1)
		source = strings.Replace(source, "op encode() -> int pure", "op encode() -> Other pure", 1)
	default:
		panic("a codec fixture variant is outside its declared domain")
	}
	return source, code
}

// codecFiles are the system file and the component of a codec fixture.
func codecFiles(component string) map[string]string {
	return map[string]string{"design/system.uc": "system example \"Codec fixture\"\n", "design/model.uc": component}
}

// representation is a representation type of Box's codec, which a codec may
// use when it is finite: when every required field ends in a value that
// does not lead back to it.
type representation struct {
	name, fields, declarations string
	finite                     bool
}

var representations = []representation{
	{"required self cycle", "field next: Repr", "", false},
	{"required indirect cycle", "field next: Link", "type Link record \"An indirect link\" {\n  field next: Repr\n}", false},
	{"optional termination", "field next: optional<Repr>", "", true},
	{"list termination", "field next: list<Repr>", "", true},
	{"set termination", "field next: set<Repr>", "", true},
	{"map termination", "field next: map<string, Repr>", "", true},
	{"codec termination", "codec encode, decodeRepr\n  field next: Repr\n  op encode() -> int pure", "op decodeRepr(value: int) -> Repr", true},
}

// representationSource writes a component whose Box encodes into Repr, of
// the representation r; value only appears in Repr's summary.
func representationSource(value int, r representation) string {
	return fmt.Sprintf(`component model "Codec fixture" {
  focus core
  paths "*.go"
}

type Repr object "Representation %d" {
  %s
}

type Box object "A private value" {
  codec encode, decode
  op encode() -> Repr pure
}

%s

op decode(value: Repr) -> Box
`, value, r.fields, r.declarations)
}

// codecNames are the names of the codec fixtures and the names each may take instead.
var codecNames = map[string][]string{
	"model":       {"model", "codecs", "wire"},
	"DecodeError": {"DecodeError", "BadInput", "Malformed"},
	"Box":         {"Box", "Cell", "Capsule"},
	"encode":      {"encode", "pack", "toWire"},
	"decode":      {"decode", "unpack", "fromWire"},
	"Repr":        {"Repr", "Shape", "Layout"},
	"supplied":    {"supplied", "given", "fixed"},
	"input":       {"input", "sample", "probe"},
	"restore":     {"restore", "rebuild", "revive"},
	"Child":       {"Child", "Sub", "Heir"},
	"Other":       {"Other", "Second", "Alt"},
	"decodeOther": {"decodeOther", "unpackOther", "readOther"},
	"decodeRepr":  {"decodeRepr", "unpackRepr", "readRepr"},
	"Link":        {"Link", "Hop", "Chain"},
	"next":        {"next", "after", "tail"},
	"amount":      {"amount", "total", "units"},
}

// codecExpected is the input of valid-codecs-load for files: Box's encoder
// and decoder, its representation type, and its edges to the two operations.
func codecExpected(r renaming, files map[string]string, representation string) Expected {
	return Expected{
		Sources: sourcesOf(r.files(files)),
		Attrs:   r.attrs(map[string]map[string][]string{"model.Box": {"codec": {"model.Box.encode", "model.decode"}, "codecType": {representation}}}),
		Edges:   []EdgeRef{{Kind: "refers", From: r.apply("model.Box"), To: r.apply("model.Box.encode")}, {Kind: "refers", From: r.apply("model.Box"), To: r.apply("model.decode")}},
	}
}

// genValidCodecs draws a valid codec under drawn names: half the time the
// valid variant of codecProgram, half the time Box encoding into one of the
// five finite representations.
//
//uc:generates design.valid-codecs-load.expected
func genValidCodecs() *rapid.Generator[Expected] {
	return rapid.Custom(func(t *rapid.T) Expected {
		value := rapid.IntRange(-1000, 1000).Draw(t, "value")
		if rapid.Bool().Draw(t, "representation") {
			var finite []representation
			for _, r := range representations {
				if r.finite {
					finite = append(finite, r)
				}
			}
			r := rapid.SampledFrom(finite).Draw(t, "finite")
			return codecExpected(drawRenaming(t, codecNames), codecFiles(representationSource(value, r)), "model.Repr")
		}
		source, _ := codecProgram(value, 0)
		representation := "int"
		if value%2 == 0 {
			representation = "model.Repr"
		}
		return codecExpected(drawRenaming(t, codecNames), codecFiles(source), representation)
	})
}

// genBrokenCodec draws a broken codec under drawn names: half the time one
// of the fourteen invalid variants of codecProgram, half the time Box
// encoding into one of the two representations that lead back to themselves
// through required fields, which leaves Box without a codecType.
//
//uc:generates design.codecs-are-typed.broken
func genBrokenCodec() *rapid.Generator[Broken] {
	return rapid.Custom(func(t *rapid.T) Broken {
		value := rapid.IntRange(-1000, 1000).Draw(t, "value")
		if rapid.Bool().Draw(t, "representation") {
			var cycles []representation
			for _, r := range representations {
				if !r.finite {
					cycles = append(cycles, r)
				}
			}
			r := rapid.SampledFrom(cycles).Draw(t, "cycle")
			return drawRenaming(t, codecNames).broken(brokenFiles{files: codecFiles(representationSource(value, r)), codes: []string{"design.invalid-codec"},
				path: "design/model.uc", absent: map[string][]string{"model.Box": {"codecType"}}}).broken()
		}
		source, code := codecProgram(value, rapid.IntRange(1, 14).Draw(t, "variant"))
		return drawRenaming(t, codecNames).broken(brokenFiles{files: codecFiles(source), codes: []string{code}, path: "design/model.uc"}).broken()
	})
}

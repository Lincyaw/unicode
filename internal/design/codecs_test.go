package design

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

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

//uc:checks design.codecs-are-typed
func checkCodecsAreTyped(t rapid.TB, value, variant int) {
	source, expected := codecProgram(value, variant)
	files := map[string]string{"design/system.uc": "system example \"Codec fixture\"\n", "design/model.uc": source}
	loaded := Load(parseAll(t, files, slices.Sorted(maps.Keys(files))))
	wellFormed(t, loaded.Diagnostics, files)
	if expected != "" {
		if !slices.ContainsFunc(loaded.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == expected && d.Position.Path == "design/model.uc" }) {
			t.Fatalf("codec variant %d did not report %s: %v\n%s", variant, expected, loaded.Diagnostics, source)
		}
		return
	}
	if len(loaded.Diagnostics) != 0 {
		t.Fatalf("a valid codec or a deferred decoded limit was rejected: %v\n%s", loaded.Diagnostics, source)
	}
	if node := loaded.Graph.Node("model.Box"); node == nil || !slices.Equal(node.Attrs["codec"], []string{"model.Box.encode", "model.decode"}) {
		t.Fatalf("the codec did not retain its absolute encoder and free decoder: %+v", node)
	}
	representation := "int"
	if value%2 == 0 {
		representation = "model.Repr"
	}
	if node := loaded.Graph.Node("model.Box"); !slices.Equal(node.Attrs["codecType"], []string{representation}) {
		t.Fatalf("the codec representation type is not canonical: %+v", node)
	}
	for _, target := range []string{"model.Box.encode", "model.decode"} {
		if !slices.ContainsFunc(loaded.Graph.Edges(), func(edge graph.Edge) bool {
			return edge.Kind == graph.EdgeKindRefers && edge.From == "model.Box" && edge.To == target
		}) {
			t.Fatalf("the codec has no refers edge to %s", target)
		}
	}
}

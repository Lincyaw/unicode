package design

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

func (l *loader) invalidCodec(e *entity, at lexer.Position, why string) {
	l.report(e.file, at, "design.invalid-codec", e.id, why,
		"declare one pure zero-argument instance encoder with a finite result and one free decoder from that result to this type")
}

func (l *loader) codecDecl(e, scope *entity, declaration *syntax.Codec) {
	if e.codec != nil {
		l.invalidCodec(e, declaration.Pos, "a type declares more than one codec")
		return
	}
	e.codec = declaration
	l.codecs = append(l.codecs, func() {
		if e.form == "interface" {
			l.invalidCodec(e, declaration.Pos, "an interface uses response scripts instead of a codec")
			return
		}
		resolve := func(reference syntax.OpRef, instance bool) *entity {
			operation := l.resolve(scope, reference.Segments)
			if instance && len(reference.Segments) == 1 && e.children[reference.Segments[0]] != nil {
				operation = e.children[reference.Segments[0]]
			}
			if !operation.is(graph.NodeKindOp) {
				l.invalidCodec(e, reference.Pos, "the codec reference "+strings.Join(reference.Segments, ".")+" does not name an operation")
				return nil
			}
			l.edge(graph.EdgeKindRefers, e, operation, e.file, reference.Pos, graph.SourceDeclared, nil)
			l.depends(e, []ref{{written: strings.Join(reference.Segments, "."), pos: reference.Pos, target: operation}})
			return operation
		}
		encoder, decoder := resolve(declaration.Encode, true), resolve(declaration.Decode, false)
		if encoder == nil || decoder == nil {
			return
		}
		e.setAttr("codec", []string{encoder.id, decoder.id})
		if encoder.parent != e || !encoder.pure || len(encoder.params) != 0 || len(encoder.raises) != 0 || encoder.result == nil {
			l.invalidCodec(e, declaration.Pos, "the encoder must be this type's pure method, with no parameters or declared errors and exactly one result")
			return
		}
		if decoder.parent.is(graph.NodeKindType) || len(decoder.params) != 1 || decoder.result == nil || decoder.result.named() != e || decoder.params[0].typ.text() != encoder.result.text() {
			l.invalidCodec(e, declaration.Pos, "the decoder must be a free operation accepting exactly the encoder's result type and returning the original type")
			return
		}
		e.codecEncoder, e.codecDecoder = encoder, decoder
	})
}

func (l *loader) codecRepresentations() {
	for _, e := range l.entities {
		if e.codecEncoder == nil {
			continue
		}
		if why := codecRepresentation(e.codecEncoder.result, map[*entity]bool{e: true}); why != "" {
			l.invalidCodec(e, e.codec.Pos, fmt.Sprintf("the codec of %s has no finite representation: %s", e.id, why))
		} else {
			e.setAttr("codecType", []string{e.codecEncoder.result.text()})
		}
	}
}

func codecRepresentation(t *typ, codecs map[*entity]bool) string {
	if why := codecShape(t, codecs, map[*entity]bool{}); why != "" {
		return why
	}
	return finiteCodecValue(t, map[*entity]bool{}, map[*entity]bool{})
}

func codecShape(t *typ, codecs, seen map[*entity]bool) string {
	if t == nil || !t.resolved() {
		return "its representation type is unresolved"
	}
	if t.kind == "call" || t.kind == "replies" {
		return "call values and response scripts are not codec representations"
	}
	for _, element := range t.elems {
		if why := codecShape(element, codecs, seen); why != "" {
			return why
		}
	}
	n := t.named()
	if n == nil {
		return ""
	}
	if n.form == "interface" || n.form == "error" {
		return "interfaces and error views are not finite model representations"
	}
	if n.codecEncoder != nil {
		if codecs[n] {
			return "codec representation dependencies form a cycle through " + n.id
		}
		codecs[n] = true
		defer delete(codecs, n)
		return codecShape(n.codecEncoder.result, codecs, map[*entity]bool{})
	}
	if n.form == "enum" || seen[n] {
		return ""
	}
	seen[n] = true
	for _, field := range visibleFields(n) {
		if why := codecShape(field.typ, codecs, seen); why != "" {
			return why
		}
	}
	return ""
}

// finiteCodecValue follows required fields only: containers and optional values
// can terminate with an empty value, while a codec replaces the native fields.
func finiteCodecValue(t *typ, required, finite map[*entity]bool) string {
	switch t.kind {
	case "list", "set", "optional", "map":
		return ""
	}
	n := t.named()
	if n == nil || n.form == "enum" || finite[n] {
		return ""
	}
	if n.codecEncoder != nil {
		return finiteCodecValue(n.codecEncoder.result, required, finite)
	}
	if required[n] {
		return n.id + " contains itself through fields that cannot be empty"
	}
	required[n] = true
	defer delete(required, n)
	for _, field := range visibleFields(n) {
		if why := finiteCodecValue(field.typ, required, finite); why != "" {
			return why
		}
	}
	finite[n] = true
	return ""
}

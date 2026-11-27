package evidence

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/syntax"
)

var lowerName = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
var checkName = regexp.MustCompile(`^[a-z][A-Za-z0-9]*(?:[.-][A-Za-z0-9]+)*$`)

func valueText(value lang.Value) (string, error) {
	switch value.Kind {
	case "none":
		return "none", nil
	case "bool", "int", "float":
		return value.Text, nil
	case "string", "time", "duration":
		return strconv.Quote(value.Text), nil
	case "bytes", "stringBytes":
		data, err := base64.StdEncoding.DecodeString(value.Text)
		if err != nil {
			return "", err
		}
		return strconv.Quote(string(data)), nil
	case "enum":
		if !lowerName.MatchString(value.Text) {
			return "", fmt.Errorf("an enum member is not a model name")
		}
		return value.Text, nil
	case "list", "set":
		children := []string{}
		for _, child := range value.Items {
			text, err := valueText(child)
			if err != nil {
				return "", err
			}
			children = append(children, text)
		}
		return "[" + strings.Join(children, ", ") + "]", nil
	case "encoded":
		if len(value.Items) != 1 {
			return "", fmt.Errorf("the encoded representation is missing")
		}
		text, err := valueText(value.Items[0])
		return "encoded(" + text + ")", err
	case "record", "map", "call":
		children := []string{}
		for _, entry := range value.Entries {
			key, err := valueText(entry.Key)
			if err != nil {
				return "", err
			}
			if value.Kind != "map" {
				if entry.Key.Kind != "string" || !lowerName.MatchString(entry.Key.Text) {
					return "", fmt.Errorf("record fields and call parameters need model names")
				}
				key = entry.Key.Text
			} else {
				switch entry.Key.Kind {
				case "string", "stringBytes", "bytes", "time", "duration", "int", "float", "enum", "bool", "none":
				default:
					return "", fmt.Errorf("this map key has no lossless corpus notation")
				}
			}
			child, err := valueText(entry.Value)
			if err != nil {
				return "", err
			}
			children = append(children, key+": "+child)
		}
		if value.Kind == "call" {
			if !lowerName.MatchString(value.Text) {
				return "", fmt.Errorf("a call needs its short operation name")
			}
			return value.Text + "(" + strings.Join(children, ", ") + ")", nil
		}
		return "{" + strings.Join(children, ", ") + "}", nil
	}
	return "", fmt.Errorf("unknown value kind %s", value.Kind)
}

func splitType(text string) (string, []string) {
	text = strings.TrimSpace(text)
	start := strings.IndexByte(text, '<')
	if start < 0 || !strings.HasSuffix(text, ">") {
		return text, nil
	}
	return text[:start], splitTop(text[start+1 : len(text)-1])
}
func splitTop(text string) []string {
	out := []string{}
	depth, start := 0, 0
	for index, char := range text {
		switch char {
		case '<', '(':
			depth++
		case '>', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(text[start:index]))
				start = index + 1
			}
		}
	}
	if strings.TrimSpace(text[start:]) != "" {
		out = append(out, strings.TrimSpace(text[start:]))
	}
	return out
}
func members(lines []string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		name, typ, ok := strings.Cut(line, ": ")
		if ok {
			out[name] = strings.TrimSuffix(typ, " generated")
		}
	}
	return out
}
func parents(g *graph.Graph, id string) []string {
	edges := []graph.Edge{}
	for _, edge := range g.Edges() {
		if edge.Kind == graph.EdgeKindExtends && edge.From == id {
			edges = append(edges, edge)
		}
	}
	slices.SortStableFunc(edges, func(a, b graph.Edge) int {
		if order := strings.Compare(a.Origin.Path, b.Origin.Path); order != 0 {
			return order
		}
		if a.Origin.Line != b.Origin.Line {
			return a.Origin.Line - b.Origin.Line
		}
		if a.Origin.Column != b.Origin.Column {
			return a.Origin.Column - b.Origin.Column
		}
		return strings.Compare(a.To, b.To)
	})
	out := []string{}
	for _, edge := range edges {
		out = append(out, edge.To)
	}
	return out
}
func fields(g *graph.Graph, id string) map[string]string {
	out, seen := map[string]string{}, map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		n := g.Node(id)
		if n == nil {
			return
		}
		for name, typ := range members(n.Attrs["fields"]) {
			if _, exists := out[name]; !exists {
				out[name] = typ
			}
		}
		for _, parent := range parents(g, id) {
			visit(parent)
		}
	}
	visit(id)
	return out
}
func operationIDs(g *graph.Graph, id string) map[string]string {
	out, seen := map[string]string{}, map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, edge := range g.Edges() {
			if edge.Kind == graph.EdgeKindContains && edge.From == id {
				node := g.Node(edge.To)
				if node != nil && node.Kind == graph.NodeKindOp {
					_, name := lastSegment(node.Id)
					if _, exists := out[name]; !exists {
						out[name] = node.Id
					}
				}
			}
		}
		for _, parent := range parents(g, id) {
			visit(parent)
		}
	}
	visit(id)
	return out
}
func lastSegment(id string) (string, string) {
	at := strings.LastIndexByte(id, '.')
	if at < 0 {
		return "", id
	}
	return id[:at], id[at+1:]
}

type operation struct {
	params map[string]string
	result string
	raises []string
}

func opOf(g *graph.Graph, id string) (operation, error) {
	node := g.Node(id)
	if node == nil || node.Kind != graph.NodeKindOp || len(node.Attrs["signature"]) != 1 {
		return operation{}, fmt.Errorf("operation %s has no complete model signature", id)
	}
	signature := node.Attrs["signature"][0]
	end := strings.IndexByte(signature, ')')
	if !strings.HasPrefix(signature, "(") || end < 0 {
		return operation{}, fmt.Errorf("invalid operation signature")
	}
	op := operation{params: members(splitTop(signature[1:end]))}
	rest := strings.TrimSpace(signature[end+1:])
	result, raises, found := strings.Cut(rest, " raises ")
	if strings.HasPrefix(rest, "raises ") {
		result = ""
		raises = strings.TrimPrefix(rest, "raises ")
		found = true
	}
	if strings.HasPrefix(result, "-> ") {
		op.result = strings.TrimPrefix(result, "-> ")
	}
	if found {
		op.raises = splitTop(raises)
	}
	return op, nil
}

func inputFromAssignments(g *graph.Graph, check string, lines []syntax.CaseLine) (lang.Input, error) {
	node := g.Node(check)
	if node == nil {
		return lang.Input{}, fmt.Errorf("the check no longer exists")
	}
	types := members(node.Attrs["symbols"])
	input := lang.Input{Values: map[string]lang.Value{}}
	for _, line := range lines {
		assignment, ok := line.(*syntax.Assignment)
		if !ok {
			continue
		}
		name := string(assignment.Symbol)
		typ, exists := types[name]
		if !exists {
			return input, fmt.Errorf("the case assigns unknown symbol %s", name)
		}
		if _, duplicate := input.Values[name]; duplicate {
			return input, fmt.Errorf("the case repeats symbol %s", name)
		}
		value, err := fromSyntax(g, typ, assignment.Value, 0)
		if err != nil {
			return input, fmt.Errorf("symbol %s: %w", name, err)
		}
		input.Values[name] = value
	}
	if len(input.Values) != len(types) {
		return input, fmt.Errorf("the case omits input symbols")
	}
	return NormalizeInput(input)
}

func graphCaseInput(g *graph.Graph, node graph.Node, check string) (lang.Input, error) {
	source := "corpus " + check + "\n\ncase r-00000000 coverage {\n"
	for _, assignment := range node.Attrs["values"] {
		source += "  " + assignment + "\n"
	}
	source += "}\n"
	parsed := syntax.Parse("corpus/input.uc", syntax.FileKindCorpus, []byte(source))
	if len(parsed.Diagnostics) > 0 {
		return lang.Input{}, fmt.Errorf("the case values cannot be parsed: %s", parsed.Diagnostics[0].Message)
	}
	for _, declaration := range parsed.File.Decls {
		if recorded, ok := declaration.(*syntax.RecordedCase); ok {
			return inputFromAssignments(g, check, recorded.Lines)
		}
	}
	return lang.Input{}, fmt.Errorf("the case has no assignments")
}

func fromSyntax(g *graph.Graph, typ string, value syntax.Value, depth int) (lang.Value, error) {
	if depth > 128 {
		return lang.Value{}, fmt.Errorf("the corpus value exceeds the finite nesting budget")
	}
	kind, args := splitType(typ)
	if _, none := value.(*syntax.NoneValue); none {
		if kind == "optional" || typ == "" {
			return lang.Value{Kind: "none"}, nil
		}
		return lang.Value{}, fmt.Errorf("none is outside the declared type")
	}
	if kind == "optional" && len(args) == 1 {
		return fromSyntax(g, args[0], value, depth+1)
	}
	if node := g.Node(typ); node != nil && len(node.Attrs["codec"]) > 0 {
		if len(node.Attrs["codecType"]) != 1 {
			return lang.Value{}, fmt.Errorf("the codec representation type is unavailable")
		}
		encoded, ok := value.(*syntax.EncodedValue)
		if !ok {
			return lang.Value{}, fmt.Errorf("a codec-backed type needs encoded representation")
		}
		representation, err := fromSyntax(g, node.Attrs["codecType"][0], encoded.Value, depth+1)
		if err != nil {
			return lang.Value{}, err
		}
		return lang.Value{Kind: "encoded", Items: []lang.Value{representation}}, nil
	}
	switch kind {
	case "bool":
		if literal, ok := value.(*syntax.BoolValue); ok {
			return lang.Value{Kind: kind, Text: strconv.FormatBool(bool(literal.Value))}, nil
		}
	case "int", "float":
		if literal, ok := value.(*syntax.NumberValue); ok {
			return normalizeValue(lang.Value{Kind: kind, Text: string(literal.Text)}, 0)
		}
	case "string", "bytes", "time", "duration":
		if literal, ok := value.(*syntax.StringValue); ok {
			text := string(literal.Text)
			if kind == "bytes" {
				text = base64.StdEncoding.EncodeToString([]byte(text))
			}
			return normalizeValue(lang.Value{Kind: kind, Text: text}, 0)
		}
	case "list", "set":
		if literal, ok := value.(*syntax.ListValue); ok && len(args) == 1 {
			out := lang.Value{Kind: kind, Items: []lang.Value{}}
			for _, item := range literal.Items {
				child, err := fromSyntax(g, args[0], item, depth+1)
				if err != nil {
					return out, err
				}
				out.Items = append(out.Items, child)
			}
			return normalizeValue(out, 0)
		}
	case "map":
		if literal, ok := value.(*syntax.BraceValue); ok && len(args) == 2 {
			out := lang.Value{Kind: "map", Entries: []lang.ValueEntry{}}
			for _, entry := range literal.Entries {
				key, err := fromSyntax(g, args[0], keySyntax(entry.Key, args[0]), depth+1)
				if err != nil {
					return out, err
				}
				child, err := fromSyntax(g, args[1], entry.Value, depth+1)
				if err != nil {
					return out, err
				}
				out.Entries = append(out.Entries, lang.ValueEntry{Key: key, Value: child})
			}
			return normalizeValue(out, 0)
		}
	case "call":
		if literal, ok := value.(*syntax.CallValue); ok {
			operationID := ""
			for _, reference := range args {
				if node := g.Node(reference); node != nil {
					if node.Kind == graph.NodeKindOp {
						_, name := lastSegment(reference)
						if name == string(literal.Op) {
							operationID = reference
						}
					} else if candidate := operationIDs(g, reference)[string(literal.Op)]; candidate != "" {
						operationID = candidate
					}
				}
			}
			if operationID == "" {
				return lang.Value{}, fmt.Errorf("the call operation is outside its declared domain")
			}
			op, err := opOf(g, operationID)
			if err != nil {
				return lang.Value{}, err
			}
			out := lang.Value{Kind: "call", Text: string(literal.Op), Entries: []lang.ValueEntry{}}
			seen := map[string]bool{}
			for _, arg := range literal.Args {
				name := string(arg.Name)
				typ, exists := op.params[name]
				if !exists || seen[name] {
					return out, fmt.Errorf("the call has unknown or repeated arguments")
				}
				seen[name] = true
				child, err := fromSyntax(g, typ, arg.Value, depth+1)
				if err != nil {
					return out, err
				}
				out.Entries = append(out.Entries, lang.ValueEntry{Key: lang.Value{Kind: "string", Text: name}, Value: child})
			}
			if len(seen) != len(op.params) {
				return out, fmt.Errorf("the call omits parameters")
			}
			return normalizeValue(out, 0)
		}
	case "replies":
		if len(args) == 1 {
			return replySyntax(g, args[0], value, depth+1)
		}
	default:
		node := g.Node(typ)
		if node == nil {
			return lang.Value{}, fmt.Errorf("the declared model type %s is missing", typ)
		}
		if node.Form == "enum" {
			if literal, ok := value.(*syntax.NameValue); ok && slices.Contains(node.Attrs["values"], string(literal.Name)) {
				return lang.Value{Kind: "enum", Text: string(literal.Name)}, nil
			}
		}
		if node.Form == "interface" {
			if literal, ok := value.(*syntax.BraceValue); ok {
				out := lang.Value{Kind: "record", Entries: []lang.ValueEntry{}}
				ops := operationIDs(g, typ)
				seen := map[string]bool{}
				for _, entry := range literal.Entries {
					if entry.Key.Name == nil {
						return out, fmt.Errorf("interface scripts need operation-name keys")
					}
					name := string(*entry.Key.Name)
					id := ops[name]
					if id == "" || seen[name] {
						return out, fmt.Errorf("the interface script has an unknown or repeated operation")
					}
					seen[name] = true
					script, err := replySyntax(g, id, entry.Value, depth+1)
					if err != nil {
						return out, err
					}
					out.Entries = append(out.Entries, lang.ValueEntry{Key: lang.Value{Kind: "string", Text: name}, Value: script})
				}
				if len(seen) != len(ops) {
					return out, fmt.Errorf("the interface script omits operations")
				}
				return normalizeValue(out, 0)
			}
		}
		if node.Form == "record" || node.Form == "object" || node.Form == "event" || node.Form == "error" {
			return recordSyntax(g, fields(g, typ), value, depth+1)
		}
	}
	return lang.Value{}, fmt.Errorf("the corpus value does not have its declared type %s", typ)
}

func keySyntax(key syntax.Key, typ string) syntax.Value {
	if key.String != nil {
		return &syntax.StringValue{Text: *key.String}
	}
	if key.Number != nil {
		return &syntax.NumberValue{Text: *key.Number}
	}
	if key.Name != nil {
		name := string(*key.Name)
		if typ == "bool" && (name == "true" || name == "false") {
			return &syntax.BoolValue{Value: syntax.Boolean(name == "true")}
		}
		kind, _ := splitType(typ)
		if kind == "optional" && name == "none" {
			return &syntax.NoneValue{None: true}
		}
		return &syntax.NameValue{Name: *key.Name}
	}
	return &syntax.NoneValue{None: true}
}

func recordSyntax(g *graph.Graph, types map[string]string, value syntax.Value, depth int) (lang.Value, error) {
	literal, ok := value.(*syntax.BraceValue)
	if !ok {
		return lang.Value{}, fmt.Errorf("a record needs its declared fields")
	}
	out := lang.Value{Kind: "record", Entries: []lang.ValueEntry{}}
	seen := map[string]bool{}
	for _, entry := range literal.Entries {
		if entry.Key.Name == nil {
			return out, fmt.Errorf("a record field needs a model name")
		}
		name := string(*entry.Key.Name)
		typ, exists := types[name]
		if !exists || seen[name] {
			return out, fmt.Errorf("the record has an unknown or repeated field")
		}
		seen[name] = true
		child, err := fromSyntax(g, typ, entry.Value, depth+1)
		if err != nil {
			return out, err
		}
		out.Entries = append(out.Entries, lang.ValueEntry{Key: lang.Value{Kind: "string", Text: name}, Value: child})
	}
	if len(seen) != len(types) {
		return out, fmt.Errorf("the record omits declared fields")
	}
	return normalizeValue(out, 0)
}

func replySyntax(g *graph.Graph, id string, value syntax.Value, depth int) (lang.Value, error) {
	op, err := opOf(g, id)
	if err != nil {
		return lang.Value{}, err
	}
	list, ok := value.(*syntax.ListValue)
	if !ok || len(list.Items) == 0 {
		return lang.Value{}, fmt.Errorf("a response script must be nonempty")
	}
	out := lang.Value{Kind: "list", Items: []lang.Value{}}
	for _, item := range list.Items {
		record, ok := item.(*syntax.BraceValue)
		if !ok {
			return out, fmt.Errorf("a response must be a record")
		}
		entries := map[string]syntax.Value{}
		for _, entry := range record.Entries {
			if entry.Key.Name == nil {
				return out, fmt.Errorf("a response field must be named")
			}
			name := string(*entry.Key.Name)
			if entries[name] != nil {
				return out, fmt.Errorf("a response repeats a field")
			}
			entries[name] = entry.Value
		}
		response := lang.Value{Kind: "record", Entries: []lang.ValueEntry{}}
		if returned, exists := entries["returns"]; exists {
			if len(entries) != 1 {
				return out, fmt.Errorf("a successful response has only returns")
			}
			child, err := fromSyntax(g, op.result, returned, depth+1)
			if err != nil {
				return out, err
			}
			response.Entries = append(response.Entries, lang.ValueEntry{Key: lang.Value{Kind: "string", Text: "returns"}, Value: child})
		} else {
			name, ok := entries["raises"].(*syntax.StringValue)
			if !ok || entries["fields"] == nil || len(entries) != 2 {
				return out, fmt.Errorf("an error response needs raises and fields")
			}
			failure := string(name.Text)
			node := g.Node(failure)
			if node == nil || node.Form != "error" {
				return out, fmt.Errorf("the response names an unknown error")
			}
			allowed := false
			seen := map[string]bool{}
			queue := []string{failure}
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				if seen[current] {
					continue
				}
				seen[current] = true
				allowed = allowed || slices.Contains(op.raises, current)
				queue = append(queue, parents(g, current)...)
			}
			if !allowed {
				return out, fmt.Errorf("the response error is not declared by the operation")
			}
			child, err := recordSyntax(g, fields(g, failure), entries["fields"], depth+1)
			if err != nil {
				return out, err
			}
			response.Entries = append(response.Entries, lang.ValueEntry{Key: lang.Value{Kind: "string", Text: "raises"}, Value: lang.Value{Kind: "string", Text: failure}}, lang.ValueEntry{Key: lang.Value{Kind: "string", Text: "fields"}, Value: child})
		}
		normalized, err := normalizeValue(response, 0)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, normalized)
	}
	return out, nil
}

func validateRoundtrip(g *graph.Graph, check string, input lang.Input, lines []syntax.CaseLine) error {
	restored, err := inputFromAssignments(g, check, lines)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical(restored.Values), canonical(input.Values)) {
		return fmt.Errorf("the corpus representation does not preserve the frozen model input")
	}
	return nil
}

package design

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/syntax"
)

// This file breaks a valid rich design in one place for each design code: it
// changes the syntax trees of the skeleton every rich design has (skeleton in
// sample_test.go), and marks the node of the tree whose line the diagnostic
// must stand on.

// Broken is a valid design changed in one place to break one constraint: the
// files, the path and line of the change, and the code of its diagnostic.
//
//uc:realizes design.Broken
type Broken struct {
	Files map[string]string
	Path  string
	Line  int
	Code  string
}

// breakage changes a rich design and returns the path of the file and the
// node of its tree where the diagnostic stands; a nil node stands for line 1.
type breakage func(d *designer) (string, any)

var breakages = map[string]breakage{
	"design.invalid-codec": func(d *designer) (string, any) {
		record := d.skel.record
		codec := &syntax.Codec{Encode: syntax.OpRef{Segments: []string{d.lower("missingEncoder")}}, Decode: syntax.OpRef{Segments: []string{d.lower("missingDecoder")}}}
		insertLine(&record.typeDecl.Lines, syntax.Member(codec), func(member syntax.Member) bool {
			switch member.(type) {
			case *syntax.Label, *syntax.Extends, *syntax.Implements:
				return true
			}
			return false
		})
		return record.file, codec
	},
	"design.missing-system-file": func(d *designer) (string, any) {
		delete(d.files, systemPath)
		return systemPath, nil
	},
	"design.component-path-mismatch": func(d *designer) (string, any) {
		c := d.skel.child
		path := "design/" + strings.ReplaceAll(c.parent.id, ".", "/") + "/" + d.kebab("moved") + ".uc"
		d.files[path] = d.files[c.file]
		d.files[path].Path = path
		delete(d.files, c.file)
		return path, c.component
	},
	"design.corpus-path-mismatch": func(d *designer) (string, any) {
		f := d.files[d.skel.corpusPath]
		delete(d.files, f.Path)
		f.Path = "corpus/" + d.kebab("moved") + ".uc"
		d.files[f.Path] = f
		return f.Path, f.Decls[0]
	},
	"design.unresolved-reference": func(d *designer) (string, any) {
		k := &d.skel
		if rapid.Bool().Draw(d.t, "anywhere?") {
			var all []refSite
			paths := d.sortedFiles()
			for _, p := range paths {
				refSites(reflect.ValueOf(d.files[p]), p, nil, &all)
			}
			site := rapid.SampledFrom(all).Draw(d.t, "reference")
			switch site.kind {
			case reflect.TypeFor[syntax.TypeRef]():
				site.segs[len(site.segs)-1] = "Undeclared"
			case reflect.TypeFor[syntax.OpRef]():
				site.segs[len(site.segs)-1] = d.lower("missing")
			default:
				site.segs[len(site.segs)-1] = d.kebab("missing")
			}
			return site.path, site.holder
		}
		switch count(d, "site", 0, 4) {
		case 0:
			segs := k.call.Op.Segments
			segs[len(segs)-1] = d.lower("missing")
			return k.flow.file, k.call
		case 1:
			uses := usesLine(k.top)
			uses.Refs[0].Segments = []string{d.kebab("missing")}
			return k.top.file, uses
		case 2:
			about := k.prop.propDecl.Lines[0].(*syntax.About)
			about.Refs[0].Segments = []string{k.top.name, "Undeclared"}
			return k.prop.file, about
		case 3:
			decl := d.files[k.corpusPath].Decls[0].(*syntax.CorpusDecl)
			decl.Target.Segments = []string{k.top.name, d.kebab("missing")}
			return k.corpusPath, decl
		}
		k.on.Error.Segments = []string{"Undeclared"}
		return k.flow.file, k.on
	},
	"design.wrong-reference-kind": func(d *designer) (string, any) {
		k := &d.skel
		switch count(d, "site", 0, 4) {
		case 0:
			uses := usesLine(k.top)
			uses.Refs = append(uses.Refs, syntax.Ref{Segments: strings.Split(k.record.id, ".")})
			return k.top.file, uses
		case 1:
			about := k.prop.propDecl.Lines[0].(*syntax.About)
			about.Refs = append(about.Refs, syntax.Ref{Segments: []string{d.persons[0].name}})
			return k.prop.file, about
		case 2:
			k.raising.opDecl.Raises = append(k.raising.opDecl.Raises, syntax.TypeRef{Segments: strings.Split(k.record.id, ".")})
			return k.top.file, k.raising.opDecl
		case 3:
			k.emit.From = syntax.Participant{Segments: []string{d.persons[0].name}}
			return k.flow.file, k.emit
		}
		decl := d.files[k.corpusPath].Decls[0].(*syntax.CorpusDecl)
		decl.Target.Segments = strings.Split(k.record.id, ".")
		return k.corpusPath, decl
	},
	"design.uses-person": func(d *designer) (string, any) {
		uses := usesLine(d.skel.top)
		at := rapid.IntRange(0, len(uses.Refs)).Draw(d.t, "at")
		uses.Refs = slices.Insert(uses.Refs, at, syntax.Ref{Segments: []string{pick(d, "person", d.persons).name}})
		return d.skel.top.file, uses
	},
	"design.missing-parent": func(d *designer) (string, any) {
		parent, name := d.kebab("missing"), d.kebab("orphan")
		decl := &syntax.ComponentDecl{Path: syntax.Path{Segments: []string{parent, name}}, Summary: "orphan",
			Lines: []syntax.ComponentLine{&syntax.Paths{Patterns: []syntax.Text{"x/**"}}}}
		path := componentPath(parent + "." + name)
		d.file(path, syntax.FileKindComponent).Decls = []syntax.Decl{decl}
		return path, decl
	},
	"design.duplicate-name": func(d *designer) (string, any) {
		k := &d.skel
		switch count(d, "site", 0, 2) {
		case 0:
			copied := *k.record.typeDecl
			f := d.componentFile(k.top)
			at := slices.Index(f.Decls, syntax.Decl(k.record.typeDecl))
			f.Decls = slices.Insert(f.Decls, at+1, syntax.Decl(&copied))
			return k.top.file, &copied
		case 1:
			name := d.persons[0].name
			decl := &syntax.ComponentDecl{Path: syntax.Path{Segments: []string{name}}, Summary: "clash",
				Lines: []syntax.ComponentLine{&syntax.Focus{Level: "core"}, &syntax.Paths{Patterns: []syntax.Text{"x/**"}}}}
			path := componentPath(name)
			d.file(path, syntax.FileKindComponent).Decls = []syntax.Decl{decl}
			return path, decl
		}
		field := &syntax.Field{Name: syntax.Lower(k.record.fields[0].name), Type: &syntax.Builtin{Name: "int"}}
		insertLine(&k.record.typeDecl.Lines, syntax.Member(field), func(m syntax.Member) bool { _, op := m.(*syntax.OpDecl); return !op })
		return k.top.file, field
	},
	"design.name-collision": func(d *designer) (string, any) {
		k := &d.skel
		f := d.componentFile(k.top)
		switch count(d, "site", 0, 2) {
		case 0:
			// The type T and the operation t both take the Go name T.
			name := d.freeName(k.top, "op", func() string { return d.lower("clash") })
			op := &syntax.OpDecl{Name: syntax.Lower(name)}
			insertDecl(f, &syntax.TypeDecl{Name: syntax.Upper(strings.ToUpper(name[:1]) + name[1:]), Form: "record", Summary: "clash"})
			insertDecl(f, op)
			return k.top.file, op
		case 1:
			// The predicates of the flow f and the property flow-f are both checkFlowF.
			name := d.fresh(func(name string) bool { return !d.used["flow-"+name] && !d.clashes(k.top, "flow", name) })
			second := d.extraProperty("flow-" + name)
			insertDecl(f, &syntax.FlowDecl{Name: syntax.Kebab(name), Summary: "clash"})
			insertDecl(f, second)
			return k.top.file, second
		}
		// The predicates of the properties p-7 and p7 are both checkP7.
		digit := strconv.Itoa(rapid.IntRange(0, 9).Draw(d.t, "digit"))
		base := d.fresh(func(name string) bool {
			return !d.used[name+"-"+digit] && !d.used[name+digit] && !d.clashes(k.top, "property", name+digit)
		})
		second := d.extraProperty(base + digit)
		insertDecl(f, d.extraProperty(base+"-"+digit))
		insertDecl(f, second)
		return k.top.file, second
	},
	"design.irreversible-name": func(d *designer) (string, any) {
		k := &d.skel
		f := d.componentFile(k.top)
		switch count(d, "site", 0, 2) {
		case 0:
			// The property flow-x, whose predicate checkFlowX a flow x's would be.
			p := d.extraProperty("flow-" + d.fresh(func(name string) bool {
				return !d.used["flow-"+name] && !d.clashes(k.top, "property", "flow-"+name)
			}))
			insertDecl(f, p)
			return k.top.file, p
		case 1:
			// The property a-7, whose predicate checkA7 the property a7's would be.
			name := d.digitAfterDash("property")
			p := d.extraProperty(name)
			insertDecl(f, p)
			return k.top.file, p
		}
		fl := &syntax.FlowDecl{Name: syntax.Kebab(d.digitAfterDash("flow")), Summary: "irreversible"}
		insertDecl(f, fl)
		return k.top.file, fl
	},
	"design.missing-focus": func(d *designer) (string, any) {
		c := d.skel.top.component
		c.Lines = slices.DeleteFunc(c.Lines, func(l syntax.ComponentLine) bool { _, ok := l.(*syntax.Focus); return ok })
		return d.skel.top.file, c
	},
	"design.unknown-label": func(d *designer) (string, any) {
		label := &syntax.Label{Name: "no-such-label"}
		t := d.skel.record.typeDecl
		t.Lines = slices.Insert(t.Lines, 0, syntax.Member(label))
		return d.skel.top.file, label
	},
	"design.undeclared-notation": func(d *designer) (string, any) {
		if slices.Contains(d.notations, "ddd") && slices.Contains(d.notations, "c4") {
			f := d.files[systemPath]
			f.Decls = slices.DeleteFunc(f.Decls, func(decl syntax.Decl) bool {
				n, ok := decl.(*syntax.NotationDecl)
				return ok && n.Name == "ddd"
			})
			d.notations = []string{"c4"}
		}
		if !slices.Contains(d.notations, "ddd") {
			label := &syntax.Label{Name: "aggregate"}
			t := d.skel.record.typeDecl
			t.Lines = slices.Insert(t.Lines, 0, syntax.Member(label))
			return d.skel.top.file, label
		}
		label := &syntax.Label{Name: "software-system"}
		c := d.skel.top.component
		c.Lines = slices.Insert(c.Lines, 0, syntax.ComponentLine(label))
		return d.skel.top.file, label
	},
	"design.label-not-allowed": func(d *designer) (string, any) {
		if !slices.Contains(d.notations, "ddd") {
			f := d.files[systemPath]
			f.Decls = slices.Insert(f.Decls, 1, syntax.Decl(&syntax.NotationDecl{Name: "ddd"}))
		}
		label := &syntax.Label{Name: "bounded-context"}
		t := d.skel.record.typeDecl
		t.Lines = slices.Insert(t.Lines, 0, syntax.Member(label))
		return d.skel.top.file, label
	},
	"design.undeclared-dependency": func(d *designer) (string, any) {
		k := &d.skel
		isolated := d.kebab("isolated")
		op := d.lower("isolate")
		path := componentPath(isolated)
		d.file(path, syntax.FileKindComponent).Decls = []syntax.Decl{
			&syntax.ComponentDecl{Path: syntax.Path{Segments: []string{isolated}}, Summary: "isolated",
				Lines: []syntax.ComponentLine{&syntax.Focus{Level: "utility"}, &syntax.Paths{Patterns: []syntax.Text{"iso/**"}}}},
			&syntax.TypeDecl{Name: "Iso", Form: "record", Summary: "isolated"},
			&syntax.OpDecl{Name: syntax.Lower(op)},
		}
		if rapid.Bool().Draw(d.t, "step?") {
			call := &syntax.Call{From: syntax.Participant{Segments: strings.Split(k.service.id, ".")}, Op: syntax.OpRef{Segments: []string{isolated, op}}}
			insertStep(k.flow.flowDecl, call, false)
			return k.flow.file, call
		}
		field := &syntax.Field{Name: syntax.Lower(d.lower("iso")), Type: &syntax.NamedType{Ref: syntax.TypeRef{Segments: []string{isolated, "Iso"}}}}
		insertLine(&k.service.typeDecl.Lines, syntax.Member(field), func(m syntax.Member) bool { _, op := m.(*syntax.OpDecl); return !op })
		return k.top.file, field
	},
	"design.invalid-subscription": func(d *designer) (string, any) {
		op := d.skel.sub.opDecl
		op.Params = append(op.Params, syntax.Param{Name: syntax.Lower(d.lower("extra")), Type: &syntax.Builtin{Name: "int"}})
		return d.skel.top.file, op
	},
	"design.not-an-event": func(d *designer) (string, any) {
		k := &d.skel
		record := syntax.TypeRef{Segments: strings.Split(k.record.id, ".")}
		if rapid.Bool().Draw(d.t, "emit?") {
			k.emit.Event = record
			return k.flow.file, k.emit
		}
		op := k.sub.opDecl
		op.Handles = &record
		op.Params[0].Type = &syntax.NamedType{Ref: record}
		return k.top.file, op
	},
	"design.nested-person-sender": func(d *designer) (string, any) {
		d.skel.inner.From = syntax.Participant{Segments: []string{d.persons[0].name}}
		return d.skel.flow.file, d.skel.inner
	},
	"design.argument-count": func(d *designer) (string, any) {
		k := &d.skel
		if rapid.Bool().Draw(d.t, "emit?") {
			k.emit.Args = append(k.emit.Args, "extra")
			return k.flow.file, k.emit
		}
		k.call.Args = append(k.call.Args, "extra")
		return k.flow.file, k.call
	},
	"design.sender-not-receiver": func(d *designer) (string, any) {
		d.skel.inner.From = syntax.Participant{Segments: strings.Split(d.skel.top.id, ".")}
		return d.skel.flow.file, d.skel.inner
	},
	"design.unraised-error": func(d *designer) (string, any) {
		k := &d.skel
		f := d.componentFile(k.top)
		f.Decls = slices.Insert(f.Decls, 1, syntax.Decl(&syntax.TypeDecl{Name: "Unraised", Form: "error", Summary: "unraised"}))
		k.on.Error = syntax.TypeRef{Segments: []string{k.top.name, "Unraised"}}
		return k.flow.file, k.on
	},
	"design.async-without-trigger": func(d *designer) (string, any) {
		k := &d.skel
		async := &syntax.Async{Op: syntax.OpRef{Segments: strings.Split(k.helper.id, ".")}}
		insertStep(k.flow.flowDecl, async, true)
		return k.flow.file, async
	},
	"design.ungenerable-symbol": func(d *designer) (string, any) {
		k := &d.skel
		s := &syntax.Symbol{Name: syntax.Lower(d.lower("symbol")), Type: &syntax.NamedType{Ref: syntax.TypeRef{Segments: strings.Split(k.iface.id, ".")}}}
		insertLine(&k.prop.propDecl.Lines, syntax.PropertyLine(s), func(l syntax.PropertyLine) bool { _, ok := l.(*syntax.Symbol); return ok })
		return k.prop.file, s
	},
	"design.limited-generated-symbol": func(d *designer) (string, any) {
		k := &d.skel
		lim := &syntax.IntervalLimit{Path: []syntax.Lower{syntax.Lower(k.flow.symbols[1].name)}, Op: "size", Low: "0", High: "3"}
		insertLine(&k.flow.flowDecl.Lines, syntax.FlowLine(lim), isInput[syntax.FlowLine])
		return k.flow.file, lim
	},
	"design.invalid-limit-path": func(d *designer) (string, any) {
		k := &d.skel
		path := []syntax.Lower{syntax.Lower(d.lower("missing"))}
		if rapid.Bool().Draw(d.t, "field?") {
			path = []syntax.Lower{syntax.Lower(k.prop.symbols[0].name), syntax.Lower(d.lower("missing"))}
		}
		lim := &syntax.IntervalLimit{Path: path, Op: "in", Low: "0", High: "1"}
		insertLine(&k.prop.propDecl.Lines, syntax.PropertyLine(lim), isInput[syntax.PropertyLine])
		return k.prop.file, lim
	},
	"design.invalid-limit": func(d *designer) (string, any) {
		k := &d.skel
		number := syntax.Lower(k.prop.symbols[0].name)
		var lim syntax.PropertyLine = &syntax.IntervalLimit{Path: []syntax.Lower{number}, Op: "size", Low: "0", High: "3"}
		if rapid.Bool().Draw(d.t, "subset?") {
			lim = &syntax.SubsetLimit{Path: []syntax.Lower{number}, Members: []syntax.Lower{"one"}}
		}
		insertLine(&k.prop.propDecl.Lines, lim, isInput[syntax.PropertyLine])
		return k.prop.file, lim
	},
	"design.empty-interval": func(d *designer) (string, any) {
		n := pick(d, "owner", []*gNode{d.skel.flow, d.skel.prop})
		lim := n.limits[count(d, "limit", 0, 1)].decl.(*syntax.IntervalLimit)
		high := rapid.IntRange(0, 20).Draw(d.t, "high")
		if lim.Op == "in" {
			high -= 10
		}
		lim.Low = syntax.Number(strconv.Itoa(high + rapid.IntRange(1, 10).Draw(d.t, "excess")))
		lim.High = syntax.Number(strconv.Itoa(high))
		return n.file, lim
	},
	"design.missing-value": func(d *designer) (string, any) {
		k := &d.skel
		if rapid.Bool().Draw(d.t, "recorded?") {
			k.recorded.Lines = k.recorded.Lines[1:]
			return k.corpusPath, k.recorded
		}
		k.propCase.Lines = k.propCase.Lines[1:]
		return k.prop.file, k.propCase
	},
	"design.unknown-symbol": func(d *designer) (string, any) {
		k := &d.skel
		a := &syntax.Assignment{Symbol: syntax.Lower(d.lower("missing")), Value: &syntax.NumberValue{Text: "1"}}
		k.propCase.Lines = append(k.propCase.Lines, a)
		return k.prop.file, a
	},
	"design.duplicate-value": func(d *designer) (string, any) {
		k := &d.skel
		copied := *k.number
		k.propCase.Lines = append(k.propCase.Lines, &copied)
		return k.prop.file, &copied
	},
	"design.value-type-mismatch": func(d *designer) (string, any) {
		k := &d.skel
		k.number.Value = pick(d, "wrong", []syntax.Value{
			&syntax.StringValue{Text: "5"}, &syntax.NumberValue{Text: "1.5"}, &syntax.NoneValue{None: true},
			&syntax.ListValue{}, &syntax.NameValue{Name: "five"},
		})
		return k.prop.file, k.number
	},
	"design.value-out-of-limit": func(d *designer) (string, any) {
		k := &d.skel
		value := &syntax.NumberValue{Text: syntax.Number(pick(d, "outside", []string{"-1", "11", "1000"}))}
		if rapid.Bool().Draw(d.t, "recorded?") {
			for _, line := range k.recorded.Lines {
				if a := line.(*syntax.Assignment); string(a.Symbol) == k.prop.symbols[0].name {
					a.Value = value
					return k.corpusPath, a
				}
			}
		}
		k.number.Value = value
		return k.prop.file, k.number
	},
	"design.recorded-case-name": func(d *designer) (string, any) {
		d.skel.propCase.Name = "r-0123abcd"
		return d.skel.prop.file, d.skel.propCase
	},
}

// fresh draws a kebab name for which free holds.
func (d *designer) fresh(free func(name string) bool) string {
	for {
		if name := d.kebab("clash"); free(name) {
			return name
		}
	}
}

// extraProperty declares a property of the skeleton's top component, about its service.
func (d *designer) extraProperty(name string) *syntax.PropertyDecl {
	d.used[name] = true
	about := &syntax.About{Refs: []syntax.Ref{{Segments: strings.Split(d.skel.service.id, ".")}}}
	return &syntax.PropertyDecl{Name: syntax.Kebab(name), Form: "invariant", Summary: "clash", Lines: []syntax.PropertyLine{about, &syntax.Manual{Reason: "Generated graph fixture"}}}
}

// digitAfterDash draws an unused name for a property or a flow of the
// skeleton's top component, with a digit right after a dash, like a-7 or a-7b.
func (d *designer) digitAfterDash(kind string) string {
	digit := strconv.Itoa(rapid.IntRange(0, 9).Draw(d.t, "digit"))
	tail := rapid.StringMatching(`[a-z0-9]{0,2}`).Draw(d.t, "tail")
	base := d.fresh(func(name string) bool {
		return !d.used[name+"-"+digit+tail] && !d.clashes(d.skel.top, kind, name+"-"+digit+tail)
	})
	d.used[base+"-"+digit+tail] = true
	return base + "-" + digit + tail
}

// refSite is a reference in a file's tree: its segments, which kind of
// reference it is, and the innermost node that holds it, on the same line.
type refSite struct {
	path   string
	segs   []string
	kind   reflect.Type
	holder any
}

var refKinds = []reflect.Type{reflect.TypeFor[syntax.Ref](), reflect.TypeFor[syntax.TypeRef](), reflect.TypeFor[syntax.OpRef](), reflect.TypeFor[syntax.Participant]()}

// refSites lists the references under v, the tree of the file at path.
func refSites(v reflect.Value, path string, holder any, out *[]refSite) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		if v.Elem().Kind() == reflect.Struct && v.Elem().FieldByName("Pos").IsValid() {
			holder = v.Interface()
		}
		refSites(v.Elem(), path, holder, out)
	case reflect.Interface:
		if !v.IsNil() {
			refSites(v.Elem(), path, holder, out)
		}
	case reflect.Struct:
		if slices.Contains(refKinds, v.Type()) {
			*out = append(*out, refSite{path, v.FieldByName("Segments").Interface().([]string), v.Type(), holder})
			return
		}
		for i := range v.NumField() {
			refSites(v.Field(i), path, holder, out)
		}
	case reflect.Slice:
		for i := range v.Len() {
			refSites(v.Index(i), path, holder, out)
		}
	}
}

func usesLine(c *gNode) *syntax.Uses {
	for _, line := range c.component.Lines {
		if u, ok := line.(*syntax.Uses); ok {
			return u
		}
	}
	panic("the skeleton's top component uses every external actor")
}

func isInput[L any](l L) bool {
	switch any(l).(type) {
	case *syntax.Symbol, *syntax.IntervalLimit, *syntax.SubsetLimit, *syntax.About:
		return true
	}
	return false
}

// insertLine puts line after the last of lines that before accepts.
func insertLine[L any](lines *[]L, line L, before func(L) bool) {
	at := 0
	for i, l := range *lines {
		if before(l) {
			at = i + 1
		}
	}
	*lines = slices.Insert(*lines, at, line)
}

// insertDecl puts a declaration of a component file last in its group
// (docs/language.md §3: types, operations, flows, then properties).
func insertDecl(f *syntax.File, decl syntax.Decl) {
	group := func(d syntax.Decl) int {
		switch d.(type) {
		case *syntax.TypeDecl:
			return 1
		case *syntax.OpDecl:
			return 2
		case *syntax.FlowDecl:
			return 3
		case *syntax.PropertyDecl:
			return 4
		}
		return 0
	}
	insertLine(&f.Decls, decl, func(d syntax.Decl) bool { return group(d) <= group(decl) })
}

// insertStep puts a step first or last among a flow's steps.
func insertStep(f *syntax.FlowDecl, step syntax.FlowLine, first bool) {
	if first {
		insertLine(&f.Lines, step, isInput[syntax.FlowLine])
		return
	}
	insertLine(&f.Lines, step, func(l syntax.FlowLine) bool {
		_, isStep := l.(syntax.Step)
		return isInput(l) || isStep
	})
}

var positionType = reflect.TypeFor[lexer.Position]()

// lineOf finds mark among the nodes of built and returns the line of the
// same node in parsed, the tree that built's print parses into.
func lineOf(built, parsed reflect.Value, mark any) (int, bool) {
	switch built.Kind() {
	case reflect.Pointer:
		if built.IsNil() {
			return 0, false
		}
		if built.Interface() == mark {
			return parsed.Elem().FieldByName("Pos").Interface().(lexer.Position).Line, true
		}
		return lineOf(built.Elem(), parsed.Elem(), mark)
	case reflect.Interface:
		if built.IsNil() {
			return 0, false
		}
		return lineOf(built.Elem(), parsed.Elem(), mark)
	case reflect.Struct:
		if built.Type() == positionType {
			return 0, false
		}
		for i := range built.NumField() {
			if line, ok := lineOf(built.Field(i), parsed.Field(i), mark); ok {
				return line, true
			}
		}
	case reflect.Slice:
		for i := range built.Len() {
			if line, ok := lineOf(built.Index(i), parsed.Index(i), mark); ok {
				return line, true
			}
		}
	}
	return 0, false
}

// genBroken draws a rich valid design and breaks it in one place for one design code.
//
//uc:generates design.rejects-broken-design.broken
func genBroken() *rapid.Generator[Broken] {
	codes := slices.Sorted(func(yield func(string) bool) {
		for code := range breakages {
			if !yield(code) {
				return
			}
		}
	})
	return rapid.Custom(func(t *rapid.T) Broken {
		if rapid.IntRange(0, 2).Draw(t, "contract") == 0 {
			return brokenContractFixture(t)
		}
		d := newDesign(t, true)
		code := rapid.SampledFrom(codes).Draw(t, "code")
		path, mark := breakages[code](d)
		files := d.sample().Files
		line := 1
		if mark != nil {
			kind, err := syntax.KindOf(path)
			parsed := syntax.Parse(path, kind, []byte(files[path]))
			if err != nil || parsed.File == nil {
				t.Fatalf("setup failed: breaking %s left %s unparsable:%s\n%s", code, path, listDiagnostics(parsed.Diagnostics), files[path])
			}
			var ok bool
			if line, ok = lineOf(reflect.ValueOf(d.files[path]), reflect.ValueOf(parsed.File), mark); !ok {
				t.Fatalf("setup failed: the changed node of %s is not in the tree of %s", code, path)
			}
		}
		return Broken{Files: files, Path: path, Line: line, Code: code}
	})
}

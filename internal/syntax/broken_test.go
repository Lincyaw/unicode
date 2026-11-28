package syntax

import (
	"fmt"
	"math/bits"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/alecthomas/participle/v2/lexer"
	"pgregory.net/rapid"
)

// Broken is a valid file with one edit that breaks a rule of docs/language.md:
// its kind, a path that fits the kind, its bytes, the line and byte column of
// the edit, and the code of the rule's diagnostic.
//
//uc:realizes syntax.Broken
type Broken struct {
	Kind   FileKind
	Path   string
	Text   []byte
	Line   int
	Column int
	Code   string
}

// A breakage draws a valid file and breaks one rule at a random site of it.
type breakage func(t *rapid.T) Broken

func oneOf(bs ...breakage) breakage {
	return func(t *rapid.T) Broken { return bs[uniform(t, len(bs))](t) }
}

// uniform draws an index below n with equal chances, from unbiased bits:
// rapid's integers favour small values and the largest one.
func uniform(t *rapid.T, n int) int {
	for {
		i := 0
		for range bits.Len(uint(n - 1)) {
			i <<= 1
			if rapid.Bool().Draw(t, "bit") {
				i++
			}
		}
		if i < n {
			return i
		}
	}
}

// inserted puts a character that no file may contain between two characters
// of a valid sample, or at one of its ends.
func inserted(code string, chars *rapid.Generator[string]) breakage {
	return func(t *rapid.T) Broken {
		s := genSample().Draw(t, "sample")
		text := string(s.Text)
		var bounds []int
		for i := range text {
			bounds = append(bounds, i)
		}
		at := rapid.SampledFrom(append(bounds, len(text))).Draw(t, "at")
		line, column := lineColumn(text, at)
		return newBroken(t, s.Kind, text[:at]+chars.Draw(t, "char")+text[at:], line, column, code)
	}
}

// lineColumn is the line and byte column of offset in text.
func lineColumn(text string, offset int) (int, int) {
	return strings.Count(text[:offset], "\n") + 1, offset - strings.LastIndexByte(text[:offset], '\n')
}

// controls are the control characters other than the tab, LF and CR, which have codes of their own.
var controls = func() []string {
	var cs []string
	for r := range rune(0xa0) {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			cs = append(cs, string(r))
		}
	}
	return cs
}()

// A byte from 0x80 up, put anywhere in valid UTF-8, leaves a sequence that is not UTF-8.
var nonUTF8 = rapid.Custom(func(t *rapid.T) string { return string([]byte{byte(rapid.IntRange(0x80, 0xff).Draw(t, "byte"))}) })

// An edit breaks a rule at one site of a tree and returns where the rule's
// diagnostic stands, as the writer's mark: a node, the address of a value in
// the tree, or nil for the start of the file.
type edit func(t *rapid.T) any

// edited draws a tree of one of kinds that has a site for the edit, makes it
// at one of the sites, and writes the tree with random layout.
func edited(code string, kinds []FileKind, sites func(f *File) []edit) breakage {
	return func(t *rapid.T) Broken {
		f, mark := editedTree(t, kinds, sites)
		text, line, column := written(&writer{t: t, mark: mark}, f)
		return newBroken(t, f.Kind, text, line, column, code)
	}
}

func editedTree(t *rapid.T, kinds []FileKind, sites func(f *File) []edit) (*File, any) {
	f := rapid.Custom(func(t *rapid.T) *File { return genFileOf(t, rapid.SampledFrom(kinds).Draw(t, "kind")) }).
		Filter(func(f *File) bool { return len(sites(f)) > 0 }).Draw(t, "file")
	es := sites(f)
	return f, es[rapid.IntRange(0, len(es)-1).Draw(t, "site")](t)
}

// written writes f with w, and returns the text and the line and column of w's mark.
func written(w *writer, f *File) (string, int, int) {
	w.file(f)
	switch {
	case w.mark == nil:
		return w.b.String(), 1, 1
	case w.markColumn == 0:
		panic(fmt.Sprintf("the writer never wrote the edited %T", w.mark))
	}
	return w.b.String(), w.markLine, w.markColumn
}

var posType = reflect.TypeFor[lexer.Position]()

// site is a value in a tree and the type of the struct that holds it.
type site struct {
	v     reflect.Value
	owner reflect.Type
}

var (
	flowLinesType = reflect.TypeFor[[]FlowLine]()
	stepsType     = reflect.TypeFor[[]Step]()
	// lineLists are the lists of lines of every level.
	lineLists = []reflect.Type{
		reflect.TypeFor[[]Decl](), reflect.TypeFor[[]LanguageLine](), reflect.TypeFor[[]ExternalLine](),
		reflect.TypeFor[[]ComponentLine](), reflect.TypeFor[[]Member](), flowLinesType,
		reflect.TypeFor[[]PropertyLine](), reflect.TypeFor[[]CaseLine](), stepsType,
	}
)

// sitesWhere returns the values in f's tree whose type match accepts.
func sitesWhere(f *File, match func(reflect.Type) bool) []site {
	var out []site
	var walk func(v reflect.Value, owner reflect.Type)
	walk = func(v reflect.Value, owner reflect.Type) {
		if match(v.Type()) {
			out = append(out, site{v, owner})
		}
		switch v.Kind() {
		case reflect.Interface, reflect.Pointer:
			if !v.IsNil() {
				walk(v.Elem(), owner)
			}
		case reflect.Struct:
			if v.Type() == posType {
				return
			}
			for i := range v.NumField() {
				walk(v.Field(i), v.Type())
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i), owner)
			}
		}
	}
	walk(reflect.ValueOf(f), nil)
	return out
}

func sitesOf[T any](f *File) []site {
	want := reflect.TypeFor[T]()
	return sitesWhere(f, func(ty reflect.Type) bool { return ty == want })
}

func each(ss []site, change func(t *rapid.T, s site) any) []edit {
	es := make([]edit, len(ss))
	for i, s := range ss {
		es[i] = func(t *rapid.T) any { return change(t, s) }
	}
	return es
}

// insert puts x at index i of the list v.
func insert(v reflect.Value, i int, x any) {
	grown := reflect.Append(v, reflect.Zero(v.Type().Elem()))
	reflect.Copy(grown.Slice(i+1, grown.Len()), grown.Slice(i, grown.Len()-1))
	grown.Index(i).Set(reflect.ValueOf(x))
	v.Set(grown)
}

func clone(x any) any {
	v := reflect.ValueOf(x)
	c := reflect.New(v.Elem().Type())
	c.Elem().Set(v.Elem())
	return c.Interface()
}

func isComment(l any) bool { _, ok := l.(*Comment); return ok }

// The ranks of the lines of each level, in the order of docs/language.md §3;
// a comment has none.
func systemRank(l any) int {
	switch l.(type) {
	case *SystemDecl:
		return 0
	case *NotationDecl:
		return 1
	case *LanguageDecl:
		return 2
	case *PersonDecl, *ExternalDecl:
		return 3
	}
	return -1
}

func componentFileRank(l any) int {
	switch l.(type) {
	case *ComponentDecl:
		return 0
	case *TypeDecl:
		return 1
	case *OpDecl:
		return 2
	case *FlowDecl:
		return 3
	case *PropertyDecl:
		return 4
	}
	return -1
}

func corpusRank(l any) int {
	switch l.(type) {
	case *CorpusDecl:
		return 0
	case *RecordedCase:
		return 1
	}
	return -1
}

func componentRank(l any) int {
	switch l.(type) {
	case *Label:
		return 0
	case *Focus:
		return 1
	case *Paths:
		return 2
	case *Uses:
		return 3
	}
	return -1
}

func externalRank(l any) int {
	switch l.(type) {
	case *TypeDecl:
		return 0
	case *OpDecl:
		return 1
	}
	return -1
}

func memberRank(l any) int {
	switch l.(type) {
	case *Label:
		return 0
	case *Extends:
		return 1
	case *Implements:
		return 2
	case *Field, *EnumValue:
		return 3
	case *OpDecl:
		return 4
	}
	return -1
}

// flowRank gives the steps rank 2.
func flowRank(l any) int {
	switch l.(type) {
	case *Comment:
		return -1
	case *Symbol:
		return 0
	case *SubsetLimit, *IntervalLimit:
		return 1
	case *CaseDecl:
		return 3
	}
	return 2
}

func propertyRank(l any) int {
	switch l.(type) {
	case *About:
		return 0
	case *Symbol:
		return 1
	case *SubsetLimit, *IntervalLimit:
		return 2
	case *Manual:
		return 3
	case *CaseDecl:
		return 4
	}
	return -1
}

var fileRanks = map[FileKind]func(any) int{FileKindSystem: systemRank, FileKindComponent: componentFileRank, FileKindCorpus: corpusRank}

// span is where a line of rank r may go among ls in order: after every line
// of a lower rank and before every line of a higher one.
func span[L any](ls []L, rank func(any) int, r int) (int, int) {
	lo, hi := 0, len(ls)
	for i, l := range ls {
		switch k := rank(l); {
		case k < 0:
		case k < r:
			lo = i + 1
		case k > r && hi == len(ls):
			hi = i
		}
	}
	return lo, hi
}

func items(v reflect.Value) []any {
	out := make([]any, v.Len())
	for i := range out {
		out[i] = v.Index(i).Interface()
	}
	return out
}

// level is one ordered list of lines: the file's declarations or a block's members.
type level struct {
	lines reflect.Value
	rank  func(any) int
	file  bool
}

func levels(f *File) []level {
	ls := []level{{reflect.ValueOf(&f.Decls).Elem(), fileRanks[f.Kind], true}}
	add := func(lines any, rank func(any) int) {
		ls = append(ls, level{reflect.ValueOf(lines).Elem(), rank, false})
	}
	for _, s := range sitesOf[*ComponentDecl](f) {
		add(&s.v.Interface().(*ComponentDecl).Lines, componentRank)
	}
	for _, s := range sitesOf[*ExternalDecl](f) {
		add(&s.v.Interface().(*ExternalDecl).Lines, externalRank)
	}
	for _, s := range sitesOf[*TypeDecl](f) {
		add(&s.v.Interface().(*TypeDecl).Lines, memberRank)
	}
	for _, s := range sitesOf[*FlowDecl](f) {
		add(&s.v.Interface().(*FlowDecl).Lines, flowRank)
	}
	for _, s := range sitesOf[*PropertyDecl](f) {
		add(&s.v.Interface().(*PropertyDecl).Lines, propertyRank)
	}
	return ls
}

// reordered moves a line after one of a higher rank, now and then with
// other lines of a lower rank that lie between them, in the file's
// declarations or, when blocks is set, in a block. The diagnostic of the
// last line moved is the one to find, so that every misplaced line must get one.
func reordered(blocks bool) func(f *File) []edit {
	return func(f *File) []edit {
		var es []edit
		for _, lv := range levels(f) {
			if lv.file == blocks {
				continue
			}
			ls := items(lv.lines)
			for i := range ls {
				for j := i + 1; j < len(ls); j++ {
					if ri, rj := lv.rank(ls[i]), lv.rank(ls[j]); ri >= 0 && ri < rj {
						es = append(es, func(t *rapid.T) any {
							moved, kept := []any{ls[i]}, []any{}
							for _, l := range ls[i+1 : j] {
								if r := lv.rank(l); r >= 0 && r < rj && rapid.Bool().Draw(t, "moved") {
									moved = append(moved, l)
								} else {
									kept = append(kept, l)
								}
							}
							for k, l := range slices.Concat(kept, ls[j:j+1], moved) {
								lv.lines.Index(i + k).Set(reflect.ValueOf(l))
							}
							return moved[len(moved)-1]
						})
					}
				}
			}
		}
		return es
	}
}

// duplicated repeats a member that a level may hold only once, somewhere after it.
func duplicated(f *File) []edit {
	var es []edit
	add := func(lines any, single func(any) bool) {
		ls := reflect.ValueOf(lines).Elem()
		for j, l := range items(ls) {
			if single(l) {
				es = append(es, func(t *rapid.T) any {
					c := clone(l)
					insert(ls, rapid.IntRange(j+1, ls.Len()).Draw(t, "at"), c)
					return c
				})
			}
		}
	}
	add(&f.Decls, func(l any) bool { return slices.Contains([]int{systemRank(l), componentFileRank(l), corpusRank(l)}, 0) })
	for _, s := range sitesOf[*ComponentDecl](f) {
		add(&s.v.Interface().(*ComponentDecl).Lines, func(l any) bool { return componentRank(l) > 0 })
	}
	for _, s := range sitesOf[*PropertyDecl](f) {
		add(&s.v.Interface().(*PropertyDecl).Lines, func(l any) bool { return propertyRank(l) == 0 })
	}
	return es
}

// headless removes the file's first declaration; the diagnostic stands at the next one.
func headless(f *File) []edit {
	return []edit{func(*rapid.T) any {
		i := slices.IndexFunc(f.Decls, func(d Decl) bool { return !isComment(d) })
		f.Decls = slices.Delete(f.Decls, i, i+1)
		if j := slices.IndexFunc(f.Decls, func(d Decl) bool { return !isComment(d) }); j >= 0 {
			return f.Decls[j]
		}
		return nil
	}}
}

// foreign puts a declaration of another kind of file anywhere among the file's.
func foreign(f *File) []edit {
	return []edit{func(t *rapid.T) any {
		others := slices.DeleteFunc(slices.Clone(fileKinds), func(k FileKind) bool { return k == f.Kind })
		ds := slices.DeleteFunc(genFileOf(t, rapid.SampledFrom(others).Draw(t, "other")).Decls, func(d Decl) bool { return isComment(d) })
		d := ds[rapid.IntRange(0, len(ds)-1).Draw(t, "decl")]
		f.Decls = slices.Insert(f.Decls, rapid.IntRange(0, len(f.Decls)).Draw(t, "at"), d)
		return d
	}}
}

// without removes the required members from a random block of type D.
func without[D any, L any](linesOf func(D) *[]L, required func(L) bool) func(f *File) []edit {
	return func(f *File) []edit {
		return each(sitesOf[D](f), func(_ *rapid.T, s site) any {
			d := s.v.Interface().(D)
			*linesOf(d) = slices.DeleteFunc(*linesOf(d), required)
			return d
		})
	}
}

// Names outside each class that still lex as one word. Keywords are
// lowercase letters only, so none of these is a keyword.
var (
	notKebab = rapid.OneOf(rapid.StringMatching(`[a-z][a-z0-9]{0,3}[A-Z][A-Za-z0-9]{0,3}`), rapid.StringMatching(`[A-Z][A-Za-z0-9]{0,4}`),
		rapid.StringMatching(`[a-z][a-z0-9]{0,3}-[a-z0-9]{0,2}[A-Z][A-Za-z0-9]{0,2}`))
	notUpper = rapid.OneOf(rapid.StringMatching(`[a-z][a-z0-9]{0,3}[0-9]`), rapid.StringMatching(`[A-Z][A-Za-z0-9]{0,3}-[A-Za-z0-9]{1,3}`))
	notLower = rapid.OneOf(rapid.StringMatching(`[A-Z][A-Za-z0-9]{0,4}`), rapid.StringMatching(`[a-z][A-Za-z0-9]{0,3}-[a-z0-9]{1,3}`))
	notAName = rapid.OneOf(rapid.StringMatching(`[A-Z][A-Za-z0-9]{0,3}-[A-Za-z0-9]{1,3}`),
		rapid.StringMatching(`[a-z][a-z0-9]{0,2}[A-Z][A-Za-z0-9]{0,2}-[a-z0-9]{1,3}`))
	notKebabOrUpper = rapid.OneOf(rapid.StringMatching(`[a-z][a-z0-9]{0,3}[A-Z][A-Za-z0-9]{0,3}`), notAName)

	// badLast are the last segments that make each form of reference invalid.
	badLast = map[reflect.Type]*rapid.Generator[string]{
		reflect.TypeFor[TypeRef](): notUpper, reflect.TypeFor[OpRef](): notLower,
		reflect.TypeFor[Participant](): notKebabOrUpper, reflect.TypeFor[Path](): notKebab,
	}
)

// renamed gives a random name of class N a name outside its class.
func renamed[N ~string](names *rapid.Generator[string]) func(f *File) []edit {
	return func(f *File) []edit {
		return each(sitesOf[N](f), func(t *rapid.T, s site) any {
			s.v.SetString(names.Draw(t, "name"))
			return s.v.Addr().Interface()
		})
	}
}

// badSegment makes a random segment of a random plain reference a name of no class.
func badSegment(f *File) []edit {
	return each(sitesOf[Ref](f), func(t *rapid.T, s site) any {
		segs := s.v.FieldByName("Segments")
		seg := segs.Index(rapid.IntRange(0, segs.Len()-1).Draw(t, "segment"))
		seg.SetString(notAName.Draw(t, "name"))
		return seg.Addr().Interface()
	})
}

func typedRefs(f *File) []site {
	return sitesWhere(f, func(ty reflect.Type) bool { _, ok := badLast[ty]; return ok })
}

// start is where the reference at s starts: its first segment.
func start(s site) any { return s.v.FieldByName("Segments").Index(0).Addr().Interface() }

// badEarlier puts a segment that is neither kebab-case nor a type before the
// last segment of a random reference, in place of one or added.
func badEarlier(f *File) []edit {
	return each(typedRefs(f), func(t *rapid.T, s site) any {
		segs := slices.Clone(s.v.FieldByName("Segments").Interface().([]string))
		bad := notKebabOrUpper.Draw(t, "name")
		if len(segs) > 1 && rapid.Bool().Draw(t, "replace") {
			segs[rapid.IntRange(0, len(segs)-2).Draw(t, "segment")] = bad
		} else {
			segs = slices.Insert(segs, rapid.IntRange(0, len(segs)-1).Draw(t, "segment"), bad)
		}
		s.v.FieldByName("Segments").Set(reflect.ValueOf(segs))
		return start(s)
	})
}

// badLastSegment gives a random reference a last segment its form does not allow.
func badLastSegment(f *File) []edit {
	return each(typedRefs(f), func(t *rapid.T, s site) any {
		segs := slices.Clone(s.v.FieldByName("Segments").Interface().([]string))
		segs[len(segs)-1] = badLast[s.v.Type()].Draw(t, "name")
		s.v.FieldByName("Segments").Set(reflect.ValueOf(segs))
		return start(s)
	})
}

// reservedName names a random lower-camel slot true, false or none. A bare
// value of that spelling is a literal, not a name, so values are left out.
func reservedName(f *File) []edit {
	ss := slices.DeleteFunc(sitesOf[Lower](f), func(s site) bool { return s.owner == reflect.TypeFor[NameValue]() })
	return each(ss, func(t *rapid.T, s site) any {
		s.v.SetString([]string{"true", "false", "none"}[uniform(t, 3)])
		return s.v.Addr().Interface()
	})
}

// summary changes a random summary, domain or label.
func summary(change func(t *rapid.T, s string) string) func(f *File) []edit {
	return func(f *File) []edit {
		return each(sitesOf[Summary](f), func(t *rapid.T, s site) any {
			s.v.SetString(change(t, s.v.String()))
			return s.v.Addr().Interface()
		})
	}
}

// spaces are white space by Unicode that is neither a tab nor a newline.
var spaces = rapid.SampledFrom([]string{" ", " ", " ", "　"})

// twoSentences makes a summary of two sentences: a mark that ends a sentence
// before its end, or a second ；.
func twoSentences(t *rapid.T, s string) string {
	if rapid.Bool().Draw(t, "semicolons") {
		return s + "；" + s + "；" + s
	}
	return s + rapid.SampledFrom([]string{"。", "！", "？"}).Draw(t, "end") + s
}

func withTabOrNewline(t *rapid.T, s string) string {
	r := []rune(s)
	i := rapid.IntRange(0, len(r)).Draw(t, "at")
	return string(r[:i]) + rapid.SampledFrom([]string{"\t", "\n"}).Draw(t, "char") + string(r[i:])
}

// member gives a random type a form that forms draws and a new field or value among its fields and values.
func member(forms []string, line func(t *rapid.T) Member) func(f *File) []edit {
	return func(f *File) []edit {
		return each(sitesOf[*TypeDecl](f), func(t *rapid.T, s site) any {
			d := s.v.Interface().(*TypeDecl)
			d.Form = rapid.SampledFrom(forms).Draw(t, "form")
			l := line(t)
			lo, hi := span(d.Lines, memberRank, 3)
			d.Lines = slices.Insert(d.Lines, rapid.IntRange(lo, hi).Draw(t, "at"), l)
			return l
		})
	}
}

func newField(t *rapid.T) Member { return &Field{Name: genLower(t, "field"), Type: genTypeExpr(t, 2)} }

func newValue(t *rapid.T) Member { return &EnumValue{Name: genLower(t, "value")} }

// stepLists are a flow's lines and every block of steps.
func stepLists(f *File) []site {
	return sitesWhere(f, func(ty reflect.Type) bool { return ty == flowLinesType || ty == stepsType })
}

// stepSpan is where a step may go in a list of steps or of a flow's lines.
func stepSpan(ls reflect.Value) (int, int) {
	if ls.Type() == flowLinesType {
		return span(items(ls), flowRank, 2)
	}
	return 0, ls.Len()
}

// onSpots are the places of the list ls where an on block has no call or on before it.
func onSpots(ls reflect.Value) []int {
	lo, hi := stepSpan(ls)
	var spots []int
	var prev any
	for j, l := range append(items(ls), nil) {
		switch prev.(type) {
		case *Call, *OnBlock:
		default:
			if j >= lo && j <= hi {
				spots = append(spots, j)
			}
		}
		if l != nil && !isComment(l) {
			prev = l
		}
	}
	return spots
}

func newOn(t *rapid.T) *OnBlock { return &OnBlock{Error: genTypeRef(t), Lines: genSteps[Step](t, 1)} }

// misplacedOn puts an on block at a random place of a random list of steps
// that has no call or on before it.
func misplacedOn(f *File) []edit {
	return each(stepLists(f), func(t *rapid.T, s site) any {
		spots := onSpots(s.v)
		on := newOn(t)
		insert(s.v, spots[rapid.IntRange(0, len(spots)-1).Draw(t, "spot")], on)
		return on
	})
}

// onInCall puts an on block among the steps of a call, where no call or on
// comes before it: in a random call, or in a new one put among random steps.
func onInCall(f *File) []edit {
	es := each(sitesOf[*Call](f), func(t *rapid.T, s site) any {
		ls := reflect.ValueOf(&s.v.Interface().(*Call).Lines).Elem()
		spots := onSpots(ls)
		on := newOn(t)
		insert(ls, spots[rapid.IntRange(0, len(spots)-1).Draw(t, "spot")], on)
		return on
	})
	return append(es, each(stepLists(f), func(t *rapid.T, s site) any {
		on := newOn(t)
		call := &Call{From: genParticipant(t), Op: genOpRef(t), Args: genArgs(t), Lines: []Step{on}}
		lo, hi := stepSpan(s.v)
		insert(s.v, rapid.IntRange(lo, hi).Draw(t, "at"), call)
		return on
	})...)
}

var fraction = rapid.Custom(func(t *rapid.T) Number {
	return Number(rapid.StringMatching(`-?(0|[1-9][0-9]{0,3})\.[0-9]{0,2}[1-9]`).Draw(t, "fraction"))
})

// fractionalSize puts a size limit among the limits of a random flow or
// property, with a fraction as its low or, when high is set, its high bound.
func fractionalSize(high bool) func(f *File) []edit {
	// limit returns the limit and the address of its fraction.
	limit := func(t *rapid.T) (*IntervalLimit, any) {
		l := &IntervalLimit{Path: []Lower{genLower(t, "path")}, Op: "size", Low: genNumber(t, true), High: genNumber(t, true)}
		if high {
			l.High = fraction.Draw(t, "high")
			return l, &l.High
		}
		l.Low = fraction.Draw(t, "low")
		return l, &l.Low
	}
	return func(f *File) []edit {
		es := each(sitesOf[*FlowDecl](f), func(t *rapid.T, s site) any {
			d := s.v.Interface().(*FlowDecl)
			l, bound := limit(t)
			lo, hi := span(d.Lines, flowRank, 1)
			d.Lines = slices.Insert(d.Lines, rapid.IntRange(lo, hi).Draw(t, "at"), FlowLine(l))
			return bound
		})
		return append(es, each(sitesOf[*PropertyDecl](f), func(t *rapid.T, s site) any {
			d := s.v.Interface().(*PropertyDecl)
			l, bound := limit(t)
			lo, hi := span(d.Lines, propertyRank, 2)
			d.Lines = slices.Insert(d.Lines, rapid.IntRange(lo, hi).Draw(t, "at"), PropertyLine(l))
			return bound
		})...)
	}
}

// trailingComment ends the first line of a random line of a random tree with a comment.
func trailingComment(t *rapid.T) Broken {
	isLine := func(ty reflect.Type) bool {
		return ty.Kind() == reflect.Pointer && ty != reflect.TypeFor[*Comment]() &&
			slices.ContainsFunc(lineLists, func(list reflect.Type) bool { return ty.Implements(list.Elem()) })
	}
	f, node := editedTree(t, fileKinds, func(f *File) []edit {
		return each(sitesWhere(f, isLine), func(_ *rapid.T, s site) any { return s.v.Interface() })
	})
	text, line, column := written(&writer{t: t, mark: node, markComment: "//" + rapid.StringOfN(genRune(), 0, 12, -1).Draw(t, "comment")}, f)
	return newBroken(t, f.Kind, text, line, column, "syntax.trailing-comment")
}

// misplacedElse writes the else of a random branch of a random alt on a line
// of its own, below the } of the branch before it, with comments or blank
// lines between them. The alt is one of the tree's or a new one put among
// random steps; its branches hold steps, alts among them.
func misplacedElse(t *rapid.T) Broken {
	branch := func(t *rapid.T, a *Alt) any {
		return &a.Branches[rapid.IntRange(1, len(a.Branches)-1).Draw(t, "branch")]
	}
	f, mark := editedTree(t, component, func(f *File) []edit {
		alts := slices.DeleteFunc(sitesOf[*Alt](f), func(s site) bool { return len(s.v.Interface().(*Alt).Branches) < 2 })
		es := each(alts, func(t *rapid.T, s site) any { return branch(t, s.v.Interface().(*Alt)) })
		return append(es, each(stepLists(f), func(t *rapid.T, s site) any {
			a := genAlt(t, 3, 2)
			lo, hi := stepSpan(s.v)
			insert(s.v, rapid.IntRange(lo, hi).Draw(t, "at"), a)
			return branch(t, a)
		})...)
	})
	text, line, column := written(&writer{t: t, mark: mark, elseApart: true}, f)
	return newBroken(t, f.Kind, text, line, column, "syntax.misplaced-else")
}

// rawLines are lines the writer writes as they are, for edits that no tree
// can hold; the rule is broken on the line of index at, col bytes into it.
// They may stand in a list of lines of any level.
type rawLines struct {
	lines   []string
	at, col int
}

func (*rawLines) decl()          {}
func (*rawLines) languageLine()  {}
func (*rawLines) externalLine()  {}
func (*rawLines) componentLine() {}
func (*rawLines) member()        {}
func (*rawLines) flowLine()      {}
func (*rawLines) propertyLine()  {}
func (*rawLines) caseLine()      {}
func (*rawLines) step()          {}

var (
	_ LanguageLine  = (*rawLines)(nil)
	_ ExternalLine  = (*rawLines)(nil)
	_ ComponentLine = (*rawLines)(nil)
	_ Member        = (*rawLines)(nil)
	_ PropertyLine  = (*rawLines)(nil)
	_ CaseLine      = (*rawLines)(nil)
	_ Step          = (*rawLines)(nil)
)

func (w *writer) raw(r *rawLines) {
	for i, l := range r.lines {
		if i == r.at && any(r) == w.mark {
			w.at(r)
			w.markOffset = r.col
		}
		w.tok(l)
		w.flush()
	}
}

// insertedLines puts the lines that raw draws at a random place of a random
// list of a tree of one of kinds; lists are the kinds of list they may join.
func insertedLines(code string, kinds []FileKind, lists []reflect.Type, raw func(t *rapid.T) *rawLines) breakage {
	return edited(code, kinds, func(f *File) []edit {
		return each(sitesWhere(f, func(ty reflect.Type) bool { return slices.Contains(lists, ty) }), func(t *rapid.T, s site) any {
			r := raw(t)
			insert(s.v, rapid.IntRange(0, s.v.Len()).Draw(t, "at"), r)
			return r
		})
	})
}

func rawAt(at, col int, ls ...string) *rawLines { return &rawLines{lines: ls, at: at, col: col} }

var (
	declLists = []reflect.Type{reflect.TypeFor[[]Decl]()}
	caseLists = []reflect.Type{reflect.TypeFor[[]CaseLine]()}
	stepTypes = []reflect.Type{flowLinesType, stepsType}
	words     = rapid.StringMatching(`[a-z][a-z0-9]{1,8}`).Filter(func(s string) bool { return !slices.Contains(keywords, s) })
	plain     = rapid.StringMatching(`[a-z 中文]{0,6}`)
)

func label(t *rapid.T) string { return quote(string(genSummary(t, "label"))) }

// A string with an escape other than the four; the rule is broken at its backslash.
func badEscape(t *rapid.T) *rawLines {
	x := rapid.SampledFrom([]string{"q", "x", "u", "a", "0", " ", "{", "中"}).Draw(t, "escape")
	before := words.Draw(t, "word") + ` "` + plain.Draw(t, "before")
	return rawAt(0, len(before), before+`\`+x+plain.Draw(t, "after")+`"`)
}

func unterminated(t *rapid.T) *rawLines {
	word := words.Draw(t, "word")
	return rawAt(0, len(word)+1, word+` "`+plain.Draw(t, "text"))
}

// A character that no token holds, outside strings and comments: + - * / are
// punctuation of expressions, and ! starts only !=.
func strayCharacter(t *rapid.T) *rawLines {
	c := rapid.SampledFrom([]string{"@", "#", "$", "%", "&", "!", "?", ";", "~", "^", "|", "_", "'", "`", `\`, "中", "é", " ", "　"}).Draw(t, "char")
	word := words.Draw(t, "word")
	return rawAt(0, len(word)+1, word+" "+c+" "+words.Draw(t, "word"))
}

// An assignment whose value breaks its line for a comment.
func commentInValue(t *rapid.T) *rawLines {
	open, closer := "[", "]"
	switch rapid.IntRange(0, 2).Draw(t, "bracket") {
	case 1:
		open, closer = "{", "}"
	case 2:
		open, closer = string(genLower(t, "op"))+"(", ")"
	}
	head := string(genLower(t, "symbol")) + " = " + open
	if rapid.Bool().Draw(t, "item") {
		head += map[string]string{"]": "1,", "}": "a: 1,", ")": "a: 1,"}[closer]
	}
	return rawAt(1, 0, head, "//"+plain.Draw(t, "comment"), closer)
}

// What follows a { on its line; the rule is broken there.
func openBrace(t *rapid.T) *rawLines {
	head := "type " + string(genUpper(t, "type")) + " record " + label(t) + " { "
	return rawAt(0, len(head), head+"label "+string(genKebab(t, "label")), "}")
}

// A } after a member on its line; the rule is broken at the }.
func closeBrace(t *rapid.T) *rawLines {
	member := "label " + string(genKebab(t, "label")) + " "
	return rawAt(1, len(member), "type "+string(genUpper(t, "type"))+" record "+label(t)+" {", member+"}")
}

// An else on its own line after the } of a block that is no branch of an alt.
func elseAfterBlock(t *rapid.T) *rawLines {
	var block []string
	switch rapid.IntRange(0, 3).Draw(t, "block") {
	case 0:
		block = []string{string(genKebab(t, "sender")) + " -> " + string(genLower(t, "op")) + "()", "on " + string(genUpper(t, "error")) + " {"}
	case 1:
		block = []string{"opt " + label(t) + " {"}
	case 2:
		block = []string{"loop " + label(t) + " {"}
	default:
		block = []string{"async " + string(genLower(t, "op")) + " {"}
	}
	ls := append(block, "}", "else "+label(t)+" {", "}")
	return rawAt(len(ls)-2, 0, ls...)
}

// A word alone on a line, outside any value.
func loneWord(t *rapid.T) *rawLines { return rawAt(0, 0, words.Draw(t, "word")) }

// Top-level declarations that break the grammar, each at the token where it departs from it.
func badDecl(t *rapid.T) *rawLines {
	switch rapid.IntRange(0, 3).Draw(t, "decl") {
	case 0:
		head := "type " + string(genUpper(t, "type")) + " "
		return rawAt(0, len(head), head+"recor "+label(t))
	case 1:
		head := "op " + string(genLower(t, "op")) + "(" + string(genLower(t, "param")) + " "
		return rawAt(0, len(head), head+"int)")
	case 2:
		head := "actor " + string(genKebab(t, "actor")) + " person " + label(t) + " "
		return rawAt(0, len(head), head+"{", "}")
	}
	return rawAt(1, 0, "flow "+string(genKebab(t, "flow"))+" "+label(t)+" {", "field "+string(genLower(t, "field"))+": int", "}")
}

// Lines that the grammar breaks at one token, one for each class of line
// that no keyword starts: a step, an assignment, an item of a value broken
// across lines, a } that closes no block, and a } with a word after it.
func badStep(t *rapid.T) *rawLines {
	head := string(genKebab(t, "sender")) + " -> " + string(genLower(t, "op")) + "(" + string(genLower(t, "arg")) + " "
	return rawAt(0, len(head), head+string(genLower(t, "arg"))+")")
}

func badAssignment(t *rapid.T) *rawLines {
	head := string(genLower(t, "symbol")) + " = "
	if rapid.Bool().Draw(t, "list?") {
		head += "[" + string(genNumber(t, false)) + " "
		return rawAt(0, len(head), head+string(genNumber(t, false))+"]")
	}
	return rawAt(0, len(head), head+",")
}

func badItem(t *rapid.T) *rawLines {
	item := string(genNumber(t, false)) + " "
	return rawAt(1, len(item), string(genLower(t, "symbol"))+" = [", item+string(genNumber(t, false)), "]")
}

func strayClose(*rapid.T) *rawLines { return rawAt(0, 0, "}") }

func afterClose(t *rapid.T) *rawLines {
	return rawAt(2, 2, "type "+string(genUpper(t, "type"))+" record "+label(t)+" {", "label "+string(genKebab(t, "label")), "} "+words.Draw(t, "word"))
}

// unexpectedEnd ends a valid sample with a block that the file ends inside.
func unexpectedEnd(t *rapid.T) Broken {
	s := genSample().Draw(t, "sample")
	text := strings.TrimRight(string(s.Text), "\n ") + "\n\ntype " + string(genUpper(t, "type")) + " record " + label(t) + " {\n  label " + string(genKebab(t, "label")) + "\n"
	return newBroken(t, s.Kind, text, strings.Count(text, "\n")+1, 1, "syntax.unexpected-end")
}

var (
	designKinds = []FileKind{FileKindSystem, FileKindComponent}
	component   = []FileKind{FileKindComponent}
)

// breakages are drawn with equal weight. Each breaks one rule at one kind of
// site, or one of a few rules whose sites are alike.
var breakages = []breakage{
	oneOf(inserted("syntax.tab", rapid.Just("\t")), inserted("syntax.carriage-return", rapid.Just("\r")),
		inserted("syntax.control-character", rapid.SampledFrom(controls)), inserted("syntax.invalid-utf8", nonUTF8)),
	oneOf(insertedLines("syntax.invalid-escape", fileKinds, lineLists, badEscape),
		insertedLines("syntax.unterminated-string", fileKinds, lineLists, unterminated),
		insertedLines("syntax.unexpected-character", fileKinds, lineLists, strayCharacter)),

	oneOf(trailingComment, insertedLines("syntax.comment-in-value", []FileKind{FileKindComponent, FileKindCorpus}, caseLists, commentInValue),
		insertedLines("syntax.brace-position", fileKinds, declLists, openBrace), insertedLines("syntax.brace-position", fileKinds, declLists, closeBrace),
		misplacedElse),
	oneOf(insertedLines("syntax.unexpected-token", component, stepTypes, elseAfterBlock),
		insertedLines("syntax.unexpected-token", fileKinds, lineLists, loneWord),
		insertedLines("syntax.unexpected-token", fileKinds, declLists, badDecl), unexpectedEnd),
	oneOf(insertedLines("syntax.unexpected-token", component, stepTypes, badStep),
		insertedLines("syntax.unexpected-token", []FileKind{FileKindComponent, FileKindCorpus}, caseLists, badAssignment),
		insertedLines("syntax.unexpected-token", []FileKind{FileKindComponent, FileKindCorpus}, caseLists, badItem),
		insertedLines("syntax.unexpected-token", fileKinds, declLists, strayClose),
		insertedLines("syntax.unexpected-token", fileKinds, declLists, afterClose)),

	oneOf(edited("syntax.declaration-order", fileKinds, reordered(false)), edited("syntax.member-order", designKinds, reordered(true))),
	oneOf(edited("syntax.duplicate-member", fileKinds, duplicated), edited("syntax.missing-header", fileKinds, headless),
		edited("syntax.unknown-declaration", fileKinds, foreign)),
	edited("syntax.missing-member", component, without(func(d *ComponentDecl) *[]ComponentLine { return &d.Lines },
		func(l ComponentLine) bool { _, ok := l.(*Paths); return ok })),
	edited("syntax.missing-member", component, without(func(d *PropertyDecl) *[]PropertyLine { return &d.Lines },
		func(l PropertyLine) bool { _, ok := l.(*About); return ok })),
	edited("syntax.missing-member", []FileKind{FileKindSystem}, without(func(d *LanguageDecl) *[]LanguageLine { return &d.Lines },
		func(l LanguageLine) bool { _, ok := l.(*Root); return ok })),

	edited("syntax.invalid-name", fileKinds, renamed[Kebab](notKebab)),
	oneOf(edited("syntax.invalid-name", fileKinds, renamed[Upper](notUpper)), edited("syntax.invalid-name", fileKinds, renamed[Lower](notLower)),
		edited("syntax.invalid-name", fileKinds, badSegment)),
	edited("syntax.invalid-reference", fileKinds, badEarlier),
	edited("syntax.invalid-reference", fileKinds, badLastSegment),
	edited("syntax.reserved-name", fileKinds, reservedName),

	edited("syntax.summary-surrounding-space", designKinds, summary(func(t *rapid.T, s string) string { return spaces.Draw(t, "space") + s })),
	edited("syntax.summary-surrounding-space", designKinds, summary(func(t *rapid.T, s string) string { return s + spaces.Draw(t, "space") })),
	oneOf(edited("syntax.empty-summary", designKinds, summary(func(*rapid.T, string) string { return "" })),
		edited("syntax.summary-tab-or-newline", designKinds, summary(withTabOrNewline))),
	edited("syntax.summary-not-one-sentence", designKinds, summary(twoSentences)),

	edited("syntax.field-not-allowed", designKinds, member([]string{"enum"}, newField)),
	edited("syntax.field-not-allowed", designKinds, member([]string{"interface"}, newField)),
	edited("syntax.value-outside-enum", designKinds, member(slices.DeleteFunc(slices.Clone(typeForms), func(f string) bool { return f == "enum" }), newValue)),

	edited("syntax.on-without-call", component, onInCall),
	edited("syntax.on-without-call", component, misplacedOn),
	edited("syntax.not-an-integer", component, fractionalSize(false)),
	edited("syntax.not-an-integer", component, fractionalSize(true)),
}

// genBroken draws a random valid file and breaks one rule of docs/language.md
// at a random site of it.
//
//uc:generates syntax.rejects-broken-text.broken
func genBroken() *rapid.Generator[Broken] {
	return rapid.Custom(func(t *rapid.T) Broken {
		if rapid.IntRange(0, 2).Draw(t, "contract") == 0 {
			return brokenContractSyntax(t)
		}
		return breakages[uniform(t, len(breakages))](t)
	})
}

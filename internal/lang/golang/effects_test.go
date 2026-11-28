package golang

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// EffectModule is a Go module that calls functions of the standard library,
// with the files of a module it depends on, its source roots, and the
// direct effects each of its functions and methods has.
//
//uc:realizes lang.golang.EffectModule
type EffectModule struct {
	Files    map[string]string
	Roots    []string
	Expected map[string][]string
}

// stdCall is a statement that calls the standard library: the package it
// imports, the statement, and the kind of effect the table gives the call,
// or "" for a function the table leaves out.
type stdCall struct {
	pkg, code string
	kind      graph.Effect
}

// stdCalls draw on every row of the table, on functions and methods, on a
// method called through an interface of net, and on functions of the same
// packages that the table leaves out.
var stdCalls = []stdCall{
	{"time", "_ = time.Now()", graph.EffectTime},
	{"time", "time.Sleep(0)", graph.EffectTime},
	{"time", "{ t := time.NewTimer(0); t.Stop() }", graph.EffectTime},
	{"time", `_, _ = time.ParseDuration("1s")`, ""},
	{"math/rand", "_ = rand.Int()", graph.EffectTime},
	{"os", `_, _ = os.ReadFile("x")`, graph.EffectFs},
	{"os", "{ var f *os.File; if f != nil { _ = f.Close() } }", graph.EffectFs},
	{"os", "_ = os.IsNotExist(nil)", ""},
	{"os", `_ = os.Getenv("X")`, graph.EffectProcess},
	{"path/filepath", `_, _ = filepath.Abs(".")`, graph.EffectFs},
	{"path/filepath", `_ = filepath.Join("a", "b")`, ""},
	{"os/exec", `_ = exec.Command("true")`, graph.EffectProcess},
	{"fmt", "fmt.Println()", graph.EffectProcess},
	{"fmt", "_ = fmt.Sprint()", ""},
	{"strings", `_ = strings.ToUpper("a")`, ""},
	{"net", `_, _ = net.Dial("tcp", "x")`, graph.EffectNetwork},
	{"net", "{ var c net.Conn; if c != nil { _ = c.Close() } }", graph.EffectNetwork},
	{"net/http", `_, _ = http.Get("http://x")`, graph.EffectNetwork},
}

// goFile collects the statements of a Go file and the packages they import.
type goFile struct {
	imports map[string]bool
	decls   strings.Builder
}

func (f *goFile) text(pkg string) string {
	var b strings.Builder
	b.WriteString("package " + pkg + "\n\n")
	for _, path := range slices.Sorted(maps.Keys(f.imports)) {
		b.WriteString("import " + `"` + path + `"` + "\n")
	}
	b.WriteString("\n" + f.decls.String())
	return b.String()
}

// effectSet is a set of kinds of effect.
type effectSet map[graph.Effect]bool

func (s effectSet) add(other effectSet) {
	for k := range other {
		s[k] = true
	}
}

func (s effectSet) written() []string {
	out := []string{}
	for _, k := range []graph.Effect{graph.EffectTime, graph.EffectFs, graph.EffectProcess, graph.EffectNetwork} {
		if s[k] {
			out = append(out, string(k))
		}
	}
	return out
}

// statements draws zero to two calls of the standard library for a body in
// f, and gives the kinds of effect they have.
func statements(t *rapid.T, f *goFile, label string) (string, effectSet) {
	kinds := effectSet{}
	var body strings.Builder
	for range rapid.IntRange(0, 2).Draw(t, label+"Calls") {
		c := rapid.SampledFrom(stdCalls).Draw(t, label+"Call")
		f.imports[c.pkg] = true
		body.WriteString("\t" + c.code + "\n")
		if c.kind != "" {
			kinds[c.kind] = true
		}
	}
	return body.String(), kinds
}

// genEffectModule draws a module example.test/m at the root, its source
// root, and a module example.test/dep it requires and replaces with the
// directory dep. The dependency has an unexported helper, a package-level
// variable holding a function literal, and one to three exported functions,
// each calling the standard library and maybe the helper, the variable's
// function and earlier exported functions. The module has an interface
// Doer, a type Impl whose method Do calls the standard library, a
// package-level variable holding a function literal, and one to four
// functions, each calling the standard library directly and in a function
// literal, and maybe an exported function of the dependency, the
// variable's function, another of its functions, and Do through Doer.
//
//uc:generates lang.golang.extracts-direct-effects.module
func genEffectModule() *rapid.Generator[EffectModule] {
	return rapid.Custom(func(t *rapid.T) EffectModule {
		dep := &goFile{imports: map[string]bool{}}
		helperBody, helperKinds := statements(t, dep, "helper")
		fmt.Fprintf(&dep.decls, "func helper() {\n%s}\n\n", helperBody)
		hookBody, hookKinds := statements(t, dep, "depHook")
		fmt.Fprintf(&dep.decls, "var hook = func() {\n%s}\n\n", hookBody)
		var depKinds []effectSet
		for i := range rapid.IntRange(1, 3).Draw(t, "depFuncs") {
			body, kinds := statements(t, dep, "dep")
			if rapid.Bool().Draw(t, "callsHelper") {
				body += "\thelper()\n"
				kinds.add(helperKinds)
			}
			if rapid.Bool().Draw(t, "callsHook") {
				body += "\thook()\n"
				kinds.add(hookKinds)
			}
			if i > 0 && rapid.Bool().Draw(t, "callsEarlier") {
				j := rapid.IntRange(0, i-1).Draw(t, "earlier")
				body += fmt.Sprintf("\tD%d()\n", j)
				kinds.add(depKinds[j])
			}
			fmt.Fprintf(&dep.decls, "func D%d() {\n%s}\n\n", i, body)
			depKinds = append(depKinds, kinds)
		}

		m := &goFile{imports: map[string]bool{}}
		expected := map[string][]string{}
		doBody, doKinds := statements(t, m, "do")
		fmt.Fprintf(&m.decls, "type Doer interface {\n\tDo()\n}\n\ntype Impl struct{}\n\nfunc (Impl) Do() {\n%s}\n\n", doBody)
		expected["go:example.test/m.Doer.Do"] = []string{}
		expected["go:example.test/m.Impl.Do"] = doKinds.written()
		mainHookBody, mainHookKinds := statements(t, m, "hook")
		fmt.Fprintf(&m.decls, "var hook = func() {\n%s}\n\n", mainHookBody)
		count := rapid.IntRange(1, 4).Draw(t, "funcs")
		for i := range count {
			body, kinds := statements(t, m, "func")
			literal, literalKinds := statements(t, m, "literal")
			if literal != "" {
				body += "\tfunc() {\n" + literal + "\t}()\n"
				kinds.add(literalKinds)
			}
			if rapid.Bool().Draw(t, "callsDependency") {
				j := rapid.IntRange(0, len(depKinds)-1).Draw(t, "dependency")
				m.imports["example.test/dep"] = true
				body += fmt.Sprintf("\tdep.D%d()\n", j)
				kinds.add(depKinds[j])
			}
			if rapid.Bool().Draw(t, "callsMainHook") {
				body += "\thook()\n"
				kinds.add(mainHookKinds)
			}
			if rapid.Bool().Draw(t, "callsFunc") {
				body += fmt.Sprintf("\tF%d()\n", rapid.IntRange(0, count-1).Draw(t, "callee"))
			}
			if rapid.Bool().Draw(t, "callsInterface") {
				body += "\tvar d Doer = Impl{}\n\td.Do()\n"
			}
			fmt.Fprintf(&m.decls, "func F%d() {\n%s}\n\n", i, body)
			expected[fmt.Sprintf("go:example.test/m.F%d", i)] = kinds.written()
		}
		return EffectModule{
			Files: map[string]string{
				"go.mod":     "module example.test/m\n\ngo 1.26\n\nrequire example.test/dep v0.0.0\n\nreplace example.test/dep => ./dep\n",
				"m.go":       m.text("m"),
				"dep/go.mod": "module example.test/dep\n\ngo 1.26\n",
				"dep/dep.go": dep.text("dep"),
			},
			Roots:    []string{"."},
			Expected: expected,
		}
	})
}

// deriveEffectRoot writes the files of module into a new directory and
// gives its path.
//
//uc:generates lang.golang.extracts-direct-effects.root
func deriveEffectRoot(module EffectModule) (string, error) {
	root, err := os.MkdirTemp("", "effects-")
	if err != nil {
		return "", err
	}
	for name, content := range module.Files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	return root, nil
}

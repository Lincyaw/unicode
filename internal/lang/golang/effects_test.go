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

// stdCall is a statement that calls the standard library: the import specs
// it needs, the statement, and the kinds the table of docs/design.md §4
// gives it.
type stdCall struct {
	imports []string
	code    string
	kinds   []graph.Effect
}

// stdCalls draw on every package the table classifies with an effect, on
// the functions of those packages that it excepts, on pure packages, on
// methods and on a method called through an interface of net, and on the
// variables it lists, alone and passed to a pure function.
var stdCalls = []stdCall{
	{[]string{`zip "archive/zip"`}, `_, _ = zip.OpenReader("x")`, []graph.Effect{graph.EffectFs}},
	{[]string{`zip "archive/zip"`}, `_ = zip.NewWriter(nil)`, nil},
	{[]string{`"context"`}, `_, _ = context.WithTimeout(context.Background(), 0)`, []graph.Effect{graph.EffectTime}},
	{[]string{`"context"`}, `_ = context.Background()`, nil},
	{[]string{`crand "crypto/rand"`}, `_, _ = crand.Read(nil)`, []graph.Effect{graph.EffectTime}},
	{[]string{`crand "crypto/rand"`, `"io"`}, `_, _ = io.ReadFull(crand.Reader, nil)`, []graph.Effect{graph.EffectTime}},
	{[]string{`"crypto/ed25519"`}, `_, _, _ = ed25519.GenerateKey(nil)`, []graph.Effect{graph.EffectTime}},
	{[]string{`"crypto/ed25519"`}, `_ = ed25519.Verify(nil, nil, nil)`, nil},
	{[]string{`"crypto/sha256"`}, `_ = sha256.Sum256(nil)`, nil},
	{[]string{`"crypto/tls"`}, `_, _ = tls.Dial("tcp", "x", nil)`, []graph.Effect{graph.EffectNetwork}},
	{[]string{`"crypto/tls"`}, `_ = tls.CipherSuites()`, nil},
	{[]string{`"crypto/tls"`}, `_, _ = tls.LoadX509KeyPair("a", "b")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"crypto/x509"`}, `_, _ = x509.SystemCertPool()`, []graph.Effect{graph.EffectFs}},
	{[]string{`"crypto/x509"`}, `_, _ = x509.ParseCertificate(nil)`, nil},
	{[]string{`"database/sql"`}, `_, _ = sql.Open("x", "y")`, []graph.Effect{graph.EffectTime, graph.EffectFs, graph.EffectNetwork}},
	{[]string{`"debug/elf"`}, `_, _ = elf.Open("x")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"flag"`}, `flag.Parse()`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"fmt"`}, `fmt.Println()`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"fmt"`}, `_ = fmt.Sprint()`, nil},
	{[]string{`"fmt"`, `"os"`}, `fmt.Fprintln(os.Stderr)`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"go/ast"`}, `_ = ast.Print(nil, nil)`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"go/build"`}, `_, _ = build.Import("x", "", 0)`, []graph.Effect{graph.EffectFs, graph.EffectProcess}},
	{[]string{`"go/parser"`}, `_, _ = parser.ParseExpr("1")`, nil},
	{[]string{`"go/parser"`}, `_, _ = parser.ParseFile(nil, "x", nil, 0)`, []graph.Effect{graph.EffectFs}},
	{[]string{`"hash/maphash"`}, `_ = maphash.MakeSeed()`, []graph.Effect{graph.EffectTime}},
	{[]string{`"io"`}, `_, _ = io.ReadFull(nil, nil)`, nil},
	{[]string{`"io/ioutil"`}, `_, _ = ioutil.ReadFile("x")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"io/ioutil"`}, `_ = ioutil.NopCloser(nil)`, nil},
	{[]string{`"log"`}, `log.Print()`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"log/slog"`}, `slog.Info("x")`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"log/slog"`}, `_ = slog.String("a", "b")`, nil},
	{[]string{`mrand "math/rand"`}, `_ = mrand.Int()`, []graph.Effect{graph.EffectTime}},
	{[]string{`rand2 "math/rand/v2"`}, `_ = rand2.N(3)`, []graph.Effect{graph.EffectTime}},
	{[]string{`"mime"`}, `_ = mime.TypeByExtension(".x")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"mime"`}, `_, _, _ = mime.ParseMediaType("x")`, nil},
	{[]string{`"net"`}, `_, _ = net.Dial("tcp", "x")`, []graph.Effect{graph.EffectNetwork}},
	{[]string{`"net"`}, `_ = net.ParseIP("1")`, nil},
	{[]string{`"net"`}, `{ var c net.Conn; if c != nil { _ = c.Close() } }`, []graph.Effect{graph.EffectNetwork}},
	{[]string{`"net/http"`}, `_, _ = http.Get("http://x")`, []graph.Effect{graph.EffectNetwork}},
	{[]string{`"net/http"`}, `_ = http.StatusText(200)`, nil},
	{[]string{`"net/http"`}, `http.ServeFile(nil, nil, "x")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"net/http"`}, `_ = http.DefaultClient`, []graph.Effect{graph.EffectNetwork}},
	{[]string{`"net/http/httptest"`}, `_ = httptest.NewRecorder()`, nil},
	{[]string{`"net/http/httptest"`}, `_ = httptest.NewServer(nil)`, []graph.Effect{graph.EffectNetwork}},
	{[]string{`"net/url"`}, `_, _ = url.Parse("x")`, nil},
	{[]string{`"os"`}, `_, _ = os.ReadFile("x")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"os"`}, `_, _ = os.Getwd()`, []graph.Effect{graph.EffectFs}},
	{[]string{`"os"`}, `{ var f *os.File; if f != nil { _ = f.Close() } }`, []graph.Effect{graph.EffectFs}},
	{[]string{`"os"`}, `_ = os.IsNotExist(nil)`, nil},
	{[]string{`"os"`}, `_ = os.Getenv("X")`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"os"`}, `_ = os.Args`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"os/exec"`}, `_ = exec.Command("true")`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"os/signal"`}, `signal.Ignore()`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"os/user"`}, `_, _ = user.Current()`, []graph.Effect{graph.EffectFs, graph.EffectProcess}},
	{[]string{`"path/filepath"`}, `_, _ = filepath.Abs(".")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"path/filepath"`}, `_ = filepath.Join("a", "b")`, nil},
	{[]string{`"runtime"`}, `_ = runtime.GOROOT()`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"runtime"`}, `_ = runtime.NumCPU()`, nil},
	{[]string{`"runtime/debug"`}, `debug.PrintStack()`, []graph.Effect{graph.EffectProcess}},
	{[]string{`"runtime/debug"`}, `_ = debug.SetGCPercent(100)`, nil},
	{[]string{`"strings"`}, `_ = strings.ToUpper("a")`, nil},
	{[]string{`"syscall"`}, `_ = syscall.Getpid()`, []graph.Effect{graph.EffectFs, graph.EffectProcess, graph.EffectNetwork}},
	{[]string{`"testing"`}, `_ = testing.Short()`, nil},
	{[]string{`"testing"`}, `_ = testing.Benchmark(nil)`, []graph.Effect{graph.EffectTime}},
	{[]string{`"text/template"`}, `_, _ = template.ParseFiles("x")`, []graph.Effect{graph.EffectFs}},
	{[]string{`"time"`}, `_ = time.Now()`, []graph.Effect{graph.EffectTime}},
	{[]string{`"time"`}, `time.Sleep(0)`, []graph.Effect{graph.EffectTime}},
	{[]string{`"time"`}, `{ t := time.NewTimer(0); t.Stop() }`, []graph.Effect{graph.EffectTime}},
	{[]string{`"time"`}, `_, _ = time.ParseDuration("1s")`, nil},
	{[]string{`"time"`}, `_ = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)`, nil},
	{[]string{`"time"`}, `_, _ = time.LoadLocation("x")`, []graph.Effect{graph.EffectFs}},
}

// goFile collects the statements of a Go file and the import specs they need.
type goFile struct {
	imports map[string]bool
	decls   strings.Builder
}

func (f *goFile) text(pkg string) string {
	var b strings.Builder
	b.WriteString("package " + pkg + "\n\n")
	for _, spec := range slices.Sorted(maps.Keys(f.imports)) {
		b.WriteString("import " + spec + "\n")
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
		for _, spec := range c.imports {
			f.imports[spec] = true
		}
		body.WriteString("\t{\n\t\t" + c.code + "\n\t}\n")
		for _, k := range c.kinds {
			kinds[k] = true
		}
	}
	return body.String(), kinds
}

// genEffectModule draws a module example.test/m at the root that requires
// golang.org/x/mod at its version, its source root, and with even chance a
// module example.test/dep it requires and replaces with the directory dep. The dependency has an unexported helper, a package-level
// variable holding a function literal, and one to three exported functions,
// each calling the standard library and maybe the helper, the variable's
// function and earlier exported functions. The module has an interface
// Doer, a type Impl whose method Do calls the standard library, a
// package-level variable holding a function literal, and one to four
// functions, each calling the standard library directly and in a function
// literal, and maybe semver.IsValid of the versioned module, an exported
// function of the local dependency, the variable's function, another of its
// functions, and Do through Doer.
//
//uc:generates lang.golang.extracts-direct-effects.module
func genEffectModule() *rapid.Generator[EffectModule] {
	return rapid.Custom(func(t *rapid.T) EffectModule {
		local := rapid.Bool().Draw(t, "local")
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
			if rapid.Bool().Draw(t, "callsVersioned") {
				// semver only computes on strings.
				m.imports[`"golang.org/x/mod/semver"`] = true
				body += "\t_ = semver.IsValid(\"v1\")\n"
			}
			if local && rapid.Bool().Draw(t, "callsDependency") {
				j := rapid.IntRange(0, len(depKinds)-1).Draw(t, "dependency")
				m.imports[`"example.test/dep"`] = true
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
		files := map[string]string{
			"go.mod": "module example.test/m\n\ngo 1.26.2\n\nrequire golang.org/x/mod v0.43.0\n",
			"m.go":   m.text("m"),
		}
		if local {
			files["go.mod"] += "\nrequire example.test/dep v0.0.0\n\nreplace example.test/dep => ./dep\n"
			files["dep/go.mod"] = "module example.test/dep\n\ngo 1.26\n"
			files["dep/dep.go"] = dep.text("dep")
		}
		return EffectModule{Files: files, Roots: []string{"."}, Expected: expected}
	})
}

// deriveEffectRoot writes the files of module and the repository's go.sum
// into a new directory and gives its path.
//
//uc:generates lang.golang.extracts-direct-effects.root
func deriveEffectRoot(module EffectModule) (string, error) {
	root, err := os.MkdirTemp("", "effects-")
	if err != nil {
		return "", err
	}
	// The repository's go.sum holds the sums of the versioned dependency.
	sum, err := os.ReadFile(filepath.Join("..", "..", "..", "go.sum"))
	if err != nil {
		return "", err
	}
	files := maps.Clone(module.Files)
	files["go.sum"] = string(sum)
	for name, content := range files {
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

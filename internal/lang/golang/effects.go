package golang

import (
	"cmp"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
)

// The direct effects of a function (docs/design.md §4, docs/model.md §3.1)
// are the kinds of effect of the standard library functions and methods its
// body calls, by the table below, and the effects of the functions of
// dependency modules it calls. A call here is what a calls edge is: the
// calls in its body and function literals, and those its body gets through
// the package-level variables it uses. A standard library function outside
// the table has no effect.

// stdEffects is the table of docs/design.md §4, by package. A package that
// is all of one kind has an entry in whole; the others list their functions
// and the types whose methods count.
var stdEffects = map[string]stdPackage{
	"time": {kind: graph.EffectTime,
		funcs:   names("Now", "Since", "Until", "Sleep", "After", "AfterFunc", "Tick", "NewTimer", "NewTicker"),
		methods: names("Timer", "Ticker")},
	"math/rand":    {kind: graph.EffectTime, whole: true},
	"math/rand/v2": {kind: graph.EffectTime, whole: true},
	"crypto/rand":  {kind: graph.EffectTime, whole: true},
	"path/filepath": {kind: graph.EffectFs,
		funcs: names("Abs", "EvalSymlinks", "Glob", "Walk", "WalkDir")},
	"io/ioutil": {kind: graph.EffectFs, allFuncs: true},
	"os/exec":   {kind: graph.EffectProcess, whole: true},
	"os/signal": {kind: graph.EffectProcess, whole: true},
	"fmt": {kind: graph.EffectProcess,
		funcs: names("Print", "Printf", "Println", "Scan", "Scanf", "Scanln")},
	"log":      {kind: graph.EffectProcess, allFuncs: true},
	"log/slog": {kind: graph.EffectProcess, allFuncs: true},
	"net/http": {kind: graph.EffectNetwork,
		funcs:   names("Get", "Head", "Post", "PostForm", "ListenAndServe", "ListenAndServeTLS", "Serve", "ServeTLS"),
		methods: names("Client", "Server", "Transport")},
	"crypto/tls": {kind: graph.EffectNetwork,
		funcs: names("Dial", "DialWithDialer", "Listen"), methods: names("Conn", "Dialer")},
	"net/rpc":  {kind: graph.EffectNetwork, whole: true},
	"net/smtp": {kind: graph.EffectNetwork, whole: true},
}

// stdPackage is one package's row of the table: every function and method
// of it, all its package-level functions, or the functions and the methods
// of the types listed, are of the kind.
type stdPackage struct {
	kind     graph.Effect
	whole    bool
	allFuncs bool
	funcs    map[string]bool
	methods  map[string]bool
}

// The package os is split between the file system and the process: the
// functions and methods that read or change the process or its
// environment are of the process, a few that touch neither are of no kind,
// and the rest are of the file system.
var (
	osProcessFuncs = names("StartProcess", "FindProcess", "Exit", "Executable", "Hostname", "Getpagesize", "Getpid",
		"Getppid", "Getuid", "Geteuid", "Getgid", "Getegid", "Getgroups", "Getenv", "LookupEnv", "Setenv", "Unsetenv",
		"Clearenv", "Environ", "ExpandEnv", "TempDir", "UserHomeDir", "UserCacheDir", "UserConfigDir")
	osProcessMethods = names("Process", "ProcessState")
	osPureFuncs      = names("Expand", "IsExist", "IsNotExist", "IsPathSeparator", "IsPermission", "IsTimeout",
		"NewSyscallError", "SameFile")
	osPureMethods = names("LinkError", "SyscallError")
	// In net, the functions whose names start with these, the methods of
	// these types, and the methods of the types that implement one of the
	// interfaces netConns are of the network.
	netFuncPrefixes = []string{"Dial", "Listen", "Lookup"}
	netMethods      = names("Dialer", "ListenConfig", "Resolver")
	netConns        = []string{"Conn", "PacketConn", "Listener"}
)

func names(list ...string) map[string]bool {
	out := map[string]bool{}
	for _, n := range list {
		out[n] = true
	}
	return out
}

// stdEffect is the kind of effect of a standard library function or method
// by the table, or "" when it has none.
func stdEffect(fn *types.Func) graph.Effect {
	pkg := fn.Pkg()
	recv := ""
	if r := receiver(fn); r != nil {
		recv = r.Obj().Name()
	}
	method := fn.Signature().Recv() != nil
	switch path := pkg.Path(); path {
	case "os":
		switch {
		case !method && osProcessFuncs[fn.Name()], method && osProcessMethods[recv]:
			return graph.EffectProcess
		case !method && osPureFuncs[fn.Name()], method && osPureMethods[recv]:
			return ""
		}
		return graph.EffectFs
	case "net":
		switch {
		case !method && slices.ContainsFunc(netFuncPrefixes, func(p string) bool { return strings.HasPrefix(fn.Name(), p) }),
			method && netMethods[recv],
			method && implementsNetConn(fn, pkg):
			return graph.EffectNetwork
		}
		return ""
	default:
		row, ok := stdEffects[path]
		switch {
		case !ok:
			return ""
		case row.whole, row.allFuncs && !method, !method && row.funcs[fn.Name()], method && row.methods[recv]:
			return row.kind
		}
		return ""
	}
}

// implementsNetConn tells whether a method of net belongs to a type that
// implements one of the interfaces netConns, an interface itself included.
func implementsNetConn(fn *types.Func, net *types.Package) bool {
	r := receiver(fn)
	if r == nil {
		return false
	}
	for _, name := range netConns {
		iface, ok := net.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		i, ok := iface.Type().Underlying().(*types.Interface)
		if ok && (types.Implements(r, i) || types.Implements(types.NewPointer(r), i)) {
			return true
		}
	}
	return false
}

// effectsRules names the table and the walk that give direct effects, so
// that what a cache holds changes with them: the table enters by its
// content, the walk by this version, which a change to the calls a body
// gives must raise.
const effectsWalkVersion = "2"

func effectsRules() string {
	var b strings.Builder
	b.WriteString("walk " + effectsWalkVersion + "\n")
	for _, path := range slices.Sorted(maps.Keys(stdEffects)) {
		row := stdEffects[path]
		b.WriteString(path + " " + string(row.kind))
		if row.whole {
			b.WriteString(" whole")
		}
		if row.allFuncs {
			b.WriteString(" funcs")
		}
		b.WriteString(" " + strings.Join(slices.Sorted(maps.Keys(row.funcs)), ",") + " " + strings.Join(slices.Sorted(maps.Keys(row.methods)), ",") + "\n")
	}
	for _, set := range []map[string]bool{osProcessFuncs, osProcessMethods, osPureFuncs, osPureMethods, netMethods} {
		b.WriteString(strings.Join(slices.Sorted(maps.Keys(set)), ",") + "\n")
	}
	b.WriteString(strings.Join(netFuncPrefixes, ",") + " " + strings.Join(netConns, ",") + "\n")
	return b.String()
}

// external is a function outside the code layer that the code calls, and
// the package it belongs to.
type external struct {
	fn  *types.Func
	pkg *packages.Package
}

// effect is the kind of a function of the standard library by the table,
// or "" when it has none or is not of the standard library.
func (e external) effect() graph.Effect {
	if !standard(e.pkg) {
		return ""
	}
	return stdEffect(e.fn)
}

// usedFuncs indexes by identity the package-level functions and methods
// that the packages name: those their calls and references name, with the
// types their packages give them, which an identity alone does not reach.
func usedFuncs(pkgs []*packages.Package) map[string]*types.Func {
	out := map[string]*types.Func{}
	for _, p := range pkgs {
		for _, obj := range p.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok && packageLevel(fn) {
				out[funcID(fn.Origin())] = fn.Origin()
			}
		}
	}
	return out
}

// externals lists, for each function of an extraction, the functions
// outside it that it calls, as the calls edges give them, by identity.
func (x *extraction) externals(funcs map[string]*types.Func, known map[string]*packages.Package) map[string][]external {
	out := map[string][]external{}
	for _, key := range slices.SortedFunc(maps.Keys(x.edges), compareEdgeKeys) {
		if key.kind != graph.EdgeKindCalls {
			continue
		}
		fn := funcs[key.to]
		if _, isNode := x.nodes[key.to]; isNode || fn == nil || known[fn.Pkg().Path()] == nil {
			continue
		}
		out[key.from] = append(out[key.from], external{fn: fn, pkg: known[fn.Pkg().Path()]})
	}
	return out
}

// addDirectEffects writes the directEffects attr of each function of the
// code layer: the kinds of the standard library functions it calls, and the
// effects of the functions of dependency modules it calls, in the order of
// graph.Effect. Functions of the main module outside the source roots are
// not counted.
func (x *extraction) addDirectEffects(l loaded) error {
	known := importClosure(l.packages)
	calls := x.externals(usedFuncs(l.packages), known)
	var deps map[string][]graph.Effect
	for _, callees := range calls {
		if deps == nil && slices.ContainsFunc(callees, func(e external) bool { return dependency(e.pkg) }) {
			var err error
			if deps, err = dependencyEffects(l.root, known); err != nil {
				return err
			}
		}
	}
	for from, callees := range calls {
		n, isFunction := x.nodes[from]
		if !isFunction || n.Kind != graph.NodeKindOp {
			continue
		}
		has := map[graph.Effect]bool{}
		for _, c := range callees {
			if k := c.effect(); k != "" {
				has[k] = true
			}
			for _, k := range deps[funcID(c.fn)] {
				has[k] = true
			}
		}
		if len(has) == 0 {
			continue
		}
		var written []string
		for _, k := range effectKinds {
			if has[k] {
				written = append(written, string(k))
			}
		}
		n.Attrs["directEffects"] = written
		x.nodes[from] = n
	}
	return nil
}

// effectKinds are the kinds of effect in the order they are written in
// (docs/design.md §5.2).
var effectKinds = []graph.Effect{graph.EffectTime, graph.EffectFs, graph.EffectProcess, graph.EffectNetwork}

func compareEdgeKeys(a, b edgeKey) int {
	return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.from, b.from), cmp.Compare(a.to, b.to))
}

// standard tells whether a package is of the standard library: it belongs
// to no module.
func standard(p *packages.Package) bool { return p.Module == nil }

// dependency tells whether a package belongs to a module that is not a main
// module.
func dependency(p *packages.Package) bool { return p.Module != nil && !p.Module.Main }

// importClosure indexes by path the packages loaded and every package they
// import, directly or not.
func importClosure(roots []*packages.Package) map[string]*packages.Package {
	out := map[string]*packages.Package{}
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if _, seen := out[p.PkgPath]; seen {
			return
		}
		out[p.PkgPath] = p
		for _, path := range slices.Sorted(maps.Keys(p.Imports)) {
			visit(p.Imports[path])
		}
	}
	for _, p := range roots {
		visit(p)
	}
	return out
}

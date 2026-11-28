package golang

import (
	"fmt"
	"go/ast"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// The direct effects of a function (docs/design.md §4, docs/model.md §3.1)
// are the kinds of effect of the standard library functions and methods its
// calls name, and of the standard library variables its body uses, by the
// table below, and the effects of the functions outside the source roots it
// calls: those of dependency modules and of the main modules' other
// packages, which D8 gives on their code. A call is what a calls edge is:
// the calls and references of its body and function literals, and those it
// gets through the package-level variables it uses. The table fails safe: a
// standard library package it does not classify has every kind, and is a
// diagnostic.

// effectMask holds a set of kinds of effect, one bit for each in
// graph.EffectOrder.
type effectMask uint8

var (
	none      effectMask
	onTime    = mask(graph.EffectTime)
	onFs      = mask(graph.EffectFs)
	onProcess = mask(graph.EffectProcess)
	onNetwork = mask(graph.EffectNetwork)
	onEvery   = onTime | onFs | onProcess | onNetwork
)

func mask(kinds ...graph.Effect) effectMask {
	var m effectMask
	for _, k := range kinds {
		m |= 1 << slices.Index(graph.EffectOrder, k)
	}
	return m
}

func (m effectMask) written() []string {
	var out []string
	for i, k := range graph.EffectOrder {
		if m&(1<<i) != 0 {
			out = append(out, string(k))
		}
	}
	return out
}

// stdRule classifies a package: its functions and methods have the kinds of
// base, except the names in by, which have the kinds given there. A name is
// a function's, `Type.Method` for a method, or `Type.` for every method of
// the type; a name ending in `*` stands for every name it starts.
type stdRule struct {
	base effectMask
	by   map[string]effectMask
}

func pure(by map[string]effectMask) stdRule { return stdRule{by: by} }

// pureNames gives each of the names no effect.
func pureNames(list ...string) map[string]effectMask {
	out := map[string]effectMask{}
	for _, n := range list {
		out[n] = none
	}
	return out
}

func with(m effectMask, list ...string) map[string]effectMask {
	out := map[string]effectMask{}
	for _, n := range list {
		out[n] = m
	}
	return out
}

func merge(parts ...map[string]effectMask) map[string]effectMask {
	out := map[string]effectMask{}
	for _, p := range parts {
		maps.Copy(out, p)
	}
	return out
}

// purePackages are the packages of the standard library none of whose
// functions and methods have an effect of their own: they compute, or reach
// the world only through the readers, writers and file systems they are
// given, whose effects D8 finds at the caller.
var purePackages = []string{
	"archive/tar", "bufio", "bytes", "cmp", "compress/bzip2", "compress/flate", "compress/gzip", "compress/lzw",
	"compress/zlib", "container/heap", "container/list", "container/ring", "crypto", "crypto/aes", "crypto/cipher",
	"crypto/des", "crypto/elliptic", "crypto/fips140", "crypto/hkdf", "crypto/hmac", "crypto/md5",
	"crypto/mlkem/mlkemtest", "crypto/pbkdf2", "crypto/rc4", "crypto/sha1", "crypto/sha256", "crypto/sha3",
	"crypto/sha512", "crypto/subtle", "crypto/x509/pkix", "database/sql/driver", "debug/dwarf", "debug/gosym",
	"embed", "encoding", "encoding/ascii85", "encoding/asn1", "encoding/base32", "encoding/base64",
	"encoding/binary", "encoding/csv", "encoding/gob", "encoding/hex", "encoding/json", "encoding/pem",
	"encoding/xml", "errors", "expvar", "go/build/constraint", "go/constant", "go/doc", "go/doc/comment",
	"go/format", "go/printer", "go/scanner", "go/token", "go/types", "go/version", "hash", "hash/adler32",
	"hash/crc32", "hash/crc64", "hash/fnv", "html", "image", "image/color", "image/color/palette", "image/draw",
	"image/gif", "image/jpeg", "image/png", "index/suffixarray", "io", "io/fs", "iter", "maps", "math",
	"math/big", "math/bits", "math/cmplx", "mime/quotedprintable", "net/http/cookiejar", "net/http/httptrace",
	"net/mail", "net/netip", "net/url", "path", "reflect", "regexp", "regexp/syntax", "runtime/cgo",
	"runtime/metrics", "runtime/race", "runtime/trace", "slices", "sort", "strconv", "strings", "structs", "sync",
	"sync/atomic", "testing/cryptotest", "testing/fstest", "testing/iotest", "testing/slogtest", "testing/synctest",
	"text/scanner", "text/tabwriter", "text/template/parse", "time/tzdata", "unicode", "unicode/utf16",
	"unicode/utf8", "unique", "unsafe", "weak",
}

// stdRules are the packages with effects, and the pure packages with a few
// functions that have one. A key ending in `/` stands for every package it
// starts, as golang.org/x/sys/, whose code is a thin layer over system calls.
var stdRules = map[string]stdRule{
	"archive/zip":          pure(with(onFs, "OpenReader")),
	"context":              pure(with(onTime, "WithTimeout", "WithDeadline", "WithTimeoutCause", "WithDeadlineCause")),
	"crypto/rand":          {base: onTime},
	"crypto/dsa":           {base: onTime, by: pureNames("Verify*", "PublicKey.")},
	"crypto/ecdh":          {base: onTime, by: pureNames("PublicKey.")},
	"crypto/ecdsa":         {base: onTime, by: pureNames("Verify*", "PublicKey.")},
	"crypto/ed25519":       {base: onTime, by: pureNames("Verify*", "PublicKey.", "NewKeyFromSeed")},
	"crypto/hpke":          {base: onTime},
	"crypto/mlkem":         {base: onTime, by: pureNames("EncapsulationKey768.", "EncapsulationKey1024.", "NewEncapsulationKey*")},
	"crypto/rsa":           {base: onTime, by: pureNames("Verify*", "PublicKey.")},
	"crypto/tls":           {base: onNetwork, by: merge(pureNames("Config.", "CipherSuites", "CipherSuiteName", "InsecureCipherSuites", "VersionName", "X509KeyPair", "Certificate.", "ConnectionState.", "QUICConn."), with(onFs, "LoadX509KeyPair"))},
	"crypto/x509":          pure(with(onFs, "SystemCertPool", "Certificate.Verify")),
	"database/sql":         {base: onTime | onFs | onNetwork},
	"debug/buildinfo":      pure(with(onFs, "ReadFile")),
	"debug/elf":            pure(with(onFs, "Open")),
	"debug/macho":          pure(with(onFs, "Open", "OpenFat")),
	"debug/pe":             pure(with(onFs, "Open")),
	"debug/plan9obj":       pure(with(onFs, "Open")),
	"flag":                 {base: onProcess},
	"fmt":                  pure(with(onProcess, "Print", "Printf", "Println", "Scan", "Scanf", "Scanln")),
	"go/ast":               pure(with(onProcess, "Print")),
	"go/build":             {base: onFs | onProcess},
	"go/importer":          {base: onFs | onProcess},
	"go/parser":            pure(with(onFs, "ParseFile", "ParseDir")),
	"hash/maphash":         pure(with(onTime, "MakeSeed")),
	"html/template":        pure(with(onFs, "ParseFiles", "ParseGlob", "Template.ParseFiles", "Template.ParseGlob")),
	"io/ioutil":            {base: onFs, by: pureNames("NopCloser", "ReadAll")},
	"log":                  {base: onProcess},
	"log/slog":             {base: onProcess, by: pureNames("Any", "Bool", "Duration", "Float64", "Group", "Int", "Int64", "String", "Time", "Uint64", "*Value", "NewRecord", "NewTextHandler", "NewJSONHandler", "New", "Value.", "Attr.", "Record.", "Level.", "LevelVar.", "Kind.", "Source.", "TextHandler.", "JSONHandler.")},
	"log/syslog":           {base: onNetwork | onProcess},
	"math/rand":            {base: onTime},
	"math/rand/v2":         {base: onTime},
	"mime":                 pure(with(onFs, "TypeByExtension", "ExtensionsByType", "AddExtensionType")),
	"mime/multipart":       pure(with(onFs, "Reader.ReadForm", "Form.RemoveAll", "FileHeader.Open")),
	"net":                  {base: onNetwork, by: pureNames("ParseIP", "ParseCIDR", "ParseMAC", "IPv4", "IPv4Mask", "CIDRMask", "JoinHostPort", "SplitHostPort", "TCPAddrFromAddrPort", "UDPAddrFromAddrPort", "IP.", "IPMask.", "IPNet.", "HardwareAddr.", "TCPAddr.", "UDPAddr.", "IPAddr.", "UnixAddr.", "OpError.", "AddrError.", "DNSError.", "ParseError.", "UnknownNetworkError.", "InvalidAddrError.", "DNSConfigError.", "Flags.", "Buffers.")},
	"net/http":             {base: onNetwork, by: merge(pureNames("CanonicalHeaderKey", "DetectContentType", "Error", "NewRequest", "NewRequestWithContext", "NewResponseController", "NotFound", "NotFoundHandler", "ParseHTTPVersion", "ParseTime", "ParseCookie", "ParseSetCookie", "MaxBytesReader", "MaxBytesHandler", "Redirect", "RedirectHandler", "StatusText", "StripPrefix", "TimeoutHandler", "AllowQuerySemicolons", "NewServeMux", "FileServer", "FileServerFS", "FS", "ServeContent", "ServeFileFS", "Header.", "Request.", "Response.", "Cookie.", "ServeMux.", "HandlerFunc.", "MaxBytesError.", "ProtocolError."), with(onFs, "Dir.", "ServeFile", "NewFileTransport"))},
	"net/http/cgi":         {base: onProcess},
	"net/http/fcgi":        {base: onNetwork},
	"net/http/httptest":    pure(with(onNetwork, "NewServer", "NewTLSServer", "NewUnstartedServer", "Server.")),
	"net/http/httputil":    {base: onNetwork, by: pureNames("DumpRequest", "DumpResponse", "NewChunkedReader", "NewChunkedWriter")},
	"net/http/pprof":       {base: onProcess | onTime},
	"net/rpc":              {base: onNetwork},
	"net/rpc/jsonrpc":      {base: onNetwork},
	"net/smtp":             {base: onNetwork},
	"net/textproto":        pure(with(onNetwork, "Dial", "Conn.")),
	"os":                   {base: onFs, by: merge(osProcess, pureNames("Expand", "IsExist", "IsNotExist", "IsPathSeparator", "IsPermission", "IsTimeout", "NewSyscallError", "SameFile", "LinkError.", "SyscallError.", "Signal."))},
	"os/exec":              {base: onProcess},
	"os/signal":            {base: onProcess},
	"os/user":              {base: onFs | onProcess},
	"path/filepath":        pure(with(onFs, "Abs", "EvalSymlinks", "Glob", "Walk", "WalkDir")),
	"plugin":               {base: onFs | onProcess},
	"runtime":              pure(with(onProcess, "GOROOT")),
	"runtime/coverage":     {base: onFs},
	"runtime/debug":        pure(merge(with(onFs, "WriteHeapDump", "SetCrashOutput"), with(onProcess, "PrintStack"))),
	"runtime/pprof":        {base: onTime},
	"syscall":              {base: onFs | onProcess | onNetwork, by: pureNames("Errno.", "Signal.", "WaitStatus.", "BytePtrFromString", "ByteSliceFromString", "StringBytePtr", "StringByteSlice")},
	"testing":              pure(merge(with(onFs, "common.TempDir", "common.Chdir", "T.Chdir"), with(onProcess, "common.Setenv", "T.Setenv", "Main", "MainStart", "RunTests", "RunBenchmarks", "RunExamples", "M.Run"), with(onTime, "B.", "Benchmark"))),
	"testing/quick":        {base: onTime},
	"text/template":        pure(with(onFs, "ParseFiles", "ParseGlob", "Template.ParseFiles", "Template.ParseGlob")),
	"time":                 {base: onTime, by: merge(pureNames("Date", "Unix", "UnixMilli", "UnixMicro", "Parse", "ParseInLocation", "ParseDuration", "FixedZone", "LoadLocationFromTZData", "Time.", "Duration.", "Month.", "Weekday.", "Location.", "ParseError."), with(onFs, "LoadLocation"))},
	"golang.org/x/sys/":    {base: onFs | onProcess | onNetwork},
	"golang.org/x/sys/cpu": pure(nil),
}

// osProcess are the functions and methods of os that read or change the
// process or its environment, the standard streams included.
var osProcess = with(onProcess, "StartProcess", "FindProcess", "Exit", "Executable", "Hostname", "Getpagesize", "Getpid",
	"Getppid", "Getuid", "Geteuid", "Getgid", "Getegid", "Getgroups", "Getenv", "LookupEnv", "Setenv", "Unsetenv",
	"Clearenv", "Environ", "ExpandEnv", "TempDir", "UserHomeDir", "UserCacheDir", "UserConfigDir", "Process.",
	"ProcessState.")

// stdVars are the variables of the standard library whose use reaches the
// world: the standard streams and arguments, the source of randomness, and
// the default clients of the network.
var stdVars = map[string]effectMask{
	"os.Stdin": onProcess, "os.Stdout": onProcess, "os.Stderr": onProcess, "os.Args": onProcess,
	"crypto/rand.Reader":     onTime,
	"net/http.DefaultClient": onNetwork, "net/http.DefaultTransport": onNetwork,
}

// rule finds the rule of a package, a key ending in / covering the packages
// it starts; ok is false for a package of the standard library that the
// table does not classify.
func rule(path string) (stdRule, bool) {
	if r, ok := stdRules[path]; ok {
		return r, true
	}
	if slices.Contains(purePackages, path) {
		return stdRule{}, true
	}
	for prefix, r := range stdRules {
		if strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix) {
			return r, true
		}
	}
	return stdRule{}, false
}

// classified tells whether the table covers a package that is not walked
// as code: the standard library, and the packages a prefix rule covers.
func classified(path string) bool {
	for prefix := range stdRules {
		if strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// stdEffect is the kinds of a function or method of a classified package,
// and whether the table classifies its package. A name in the rule's by
// is matched exactly first, then as a pattern: `Prefix*` stands for every
// name that starts with Prefix, `*Suffix` for every function whose name
// ends with Suffix.
func stdEffect(fn *types.Func) (effectMask, bool) {
	r, ok := rule(fn.Pkg().Path())
	if !ok {
		return onEvery, false
	}
	candidates := []string{fn.Name()}
	if recv := receiver(fn); recv != nil {
		candidates = []string{recv.Obj().Name() + "." + fn.Name(), recv.Obj().Name() + "."}
	}
	for _, c := range candidates {
		if m, ok := r.by[c]; ok {
			return m, true
		}
	}
	for _, pattern := range slices.Sorted(maps.Keys(r.by)) {
		if head, ok := strings.CutSuffix(pattern, "*"); ok && strings.HasPrefix(candidates[0], head) {
			return r.by[pattern], true
		}
		if tail, ok := strings.CutPrefix(pattern, "*"); ok && len(candidates) == 1 && strings.HasSuffix(candidates[0], tail) {
			return r.by[pattern], true
		}
	}
	return r.base, true
}

// outside is the package a function outside the code layer belongs to, by
// how D8 reaches its effects.
type outside int

const (
	inTable    outside = iota // the standard library or a prefix rule: the table
	walked                    // a dependency module or a main module's package outside the roots: its code
	unknownPkg                // a package the loaded code does not reach
)

func place(p *packages.Package) outside {
	switch {
	case p == nil:
		return unknownPkg
	case p.Module == nil || classified(p.PkgPath):
		return inTable
	}
	return walked
}

// usedFuncs indexes by identity the package-level functions and methods
// that the packages name, with the types their packages give them, which an
// identity alone does not reach.
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

// tableEffects gives each function of the extraction the kinds of the
// table's functions it calls and of the table's variables its body uses,
// and the walked functions it calls; it reports, at the call, each function
// of a standard library package the table does not classify, which then
// has every kind.
func (x *extraction) tableEffects(pkgs []*packages.Package, known map[string]*packages.Package) (map[string]effectMask, map[string][]string) {
	funcs := usedFuncs(pkgs)
	direct := map[string]effectMask{}
	calls := map[string][]string{}
	for _, key := range slices.SortedFunc(maps.Keys(x.edges), compareEdgeKeys) {
		fn := funcs[key.to]
		if _, isNode := x.nodes[key.to]; key.kind != graph.EdgeKindCalls || isNode || fn == nil {
			continue
		}
		switch place(known[fn.Pkg().Path()]) {
		case inTable:
			m, ok := stdEffect(fn)
			if !ok {
				site := x.sites[key]
				at := diag.Position{}
				if len(site) > 0 {
					at = site[0]
				}
				x.diagnostics = append(x.diagnostics, diag.Diagnostic{
					Code: "lang.golang.unclassified-package", Subject: key.from, Position: at,
					Message: fmt.Sprintf("%s calls %s of the standard library package %s, which the effect table does not classify, so it counts as having every effect", key.from, fn.Name(), fn.Pkg().Path()),
					Fix:     fmt.Sprintf("add %s to the effect table of the Go backend (docs/design.md §4)", fn.Pkg().Path()),
				})
			}
			direct[key.from] |= m
		case walked:
			calls[key.from] = append(calls[key.from], key.to)
		}
	}
	for _, d := range x.declared {
		if d.decl != nil && d.decl.Body != nil {
			direct[d.id] |= x.varEffects(d.pkg, d.decl.Body)
		}
	}
	return direct, calls
}

// varEffects is the kinds of the table's variables that a body uses, and
// that the values of the package-level variables it uses use in turn: a
// value's function literals run in whoever uses the variable, as its calls
// do (calls.go).
func (x *extraction) varEffects(p *packages.Package, body ast.Node) effectMask {
	var m effectMask
	seen := map[string]bool{}
	var use func(p *packages.Package, n ast.Node)
	use = func(p *packages.Package, n ast.Node) {
		ast.Inspect(n, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			v, ok := p.TypesInfo.Uses[ident].(*types.Var)
			if !ok || v.Pkg() == nil || v.Parent() != v.Pkg().Scope() {
				return true
			}
			m |= stdVars[v.Pkg().Path()+"."+v.Name()]
			if id := varID(v); !seen[id] {
				seen[id] = true
				for _, init := range x.vars[id] {
					use(init.pkg, init.expr)
				}
			}
			return true
		})
	}
	use(p, body)
	return m
}

// addDirectEffects writes the directEffects attr of each function of the
// code layer: the kinds the table gives the calls and variables of its
// body, and the effects that D8 gives the walked functions it calls.
func (x *extraction) addDirectEffects(l loaded) error {
	known := importClosure(l.packages)
	direct, calls := x.tableEffects(l.packages, known)
	if len(calls) > 0 {
		walkedEffects, problems, err := outsideEffects(l, known)
		if err != nil {
			return err
		}
		x.diagnostics = append(x.diagnostics, problems...)
		for from, callees := range calls {
			for _, c := range callees {
				direct[from] |= walkedEffects[c]
			}
		}
	}
	for id, m := range direct {
		n, ok := x.nodes[id]
		if !ok || n.Kind != graph.NodeKindOp || m == none {
			continue
		}
		n.Attrs["directEffects"] = m.written()
		x.nodes[id] = n
	}
	return nil
}

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

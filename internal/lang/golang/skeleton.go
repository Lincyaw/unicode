package golang

import (
	"crypto/sha256"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/mod/modfile"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

type skeletonPackage struct{ dir, name, importPath string }
type skeletonPlanner struct {
	plan        lang.GenPlan
	nodes       map[string]graph.Node
	parents     map[string]string
	realized    map[string][]string
	checks      map[string][]string
	generators  map[string]bool
	packages    map[string]skeletonPackage
	module      string
	diagnostics []diag.Diagnostic
}

// prepareSkeletons loads planned human-owned declarations in memory, then
// links their real bindings before compiling the generated contracts.
func prepareSkeletons(plan lang.GenPlan) (lang.GenPlan, loaded, []lang.Edit, []diag.Diagnostic) {
	p := skeletonPlanner{plan: plan, nodes: map[string]graph.Node{}, parents: map[string]string{}, realized: map[string][]string{}, checks: map[string][]string{}, generators: map[string]bool{}, packages: map[string]skeletonPackage{}}
	for _, n := range plan.Graph.Nodes() {
		p.nodes[n.Id] = n
	}
	for _, e := range plan.Graph.Edges() {
		switch e.Kind {
		case graph.EdgeKindContains:
			p.parents[e.To] = e.From
		case graph.EdgeKindRealizes:
			p.realized[e.To] = append(p.realized[e.To], e.From)
		case graph.EdgeKindChecks:
			p.checks[e.To] = append(p.checks[e.To], e.From)
		case graph.EdgeKindGenerates:
			for _, symbol := range e.Attrs["symbols"] {
				p.generators[e.To+"."+symbol] = true
			}
		}
	}
	edits := p.edits()
	for _, edit := range edits {
		if _, err := os.Lstat(filepath.Join(plan.Root, filepath.FromSlash(edit.Path))); err == nil {
			p.diagnostics = append(p.diagnostics, diag.Diagnostic{Code: "lang.golang.skeleton-failed", Subject: edit.Path, Position: diag.Position{Path: edit.Path, Line: 1, Column: 1}, Message: "the create-only skeleton target already exists", Fix: "restore its intended binding or move the conflicting file before rebuilding the generation plan"})
		} else if !os.IsNotExist(err) {
			p.diagnostics = append(p.diagnostics, runDiagnostic("lang.golang.skeleton-failed", err.Error(), "make prospective skeleton paths readable before generation"))
		}
	}
	if len(p.diagnostics) > 0 {
		return plan, loaded{}, nil, p.diagnostics
	}
	var l loaded
	var err error
	if len(edits) == 0 {
		l, err = loadForGeneration(plan.Root, plan.Roots)
	} else {
		l, err = loadWithSkeletons(plan, edits)
	}
	if err != nil {
		return plan, loaded{}, nil, []diag.Diagnostic{runDiagnostic("lang.golang.generate-failed", err.Error(), "make the handwritten packages and planned skeleton declarations load before generation")}
	}
	if len(edits) == 0 {
		return plan, l, nil, nil
	}
	g := graph.NewGraph()
	var fragment graph.Fragment
	for _, n := range plan.Graph.Nodes() {
		if n.Layer == graph.LayerDesign {
			fragment.Nodes = append(fragment.Nodes, n)
		}
	}
	for _, e := range plan.Graph.Edges() {
		if p.nodes[e.From].Layer == graph.LayerDesign && p.nodes[e.To].Layer == graph.LayerDesign {
			fragment.Edges = append(fragment.Edges, e)
		}
	}
	if err = g.AddFragment(fragment); err != nil {
		return plan, loaded{}, nil, []diag.Diagnostic{runDiagnostic("lang.golang.generate-failed", err.Error(), "correct the design graph before planning skeletons")}
	}
	extracted := extract(l)
	if err = g.AddFragment(extracted.Fragment); err != nil {
		return plan, loaded{}, nil, []diag.Diagnostic{runDiagnostic("lang.golang.generate-failed", err.Error(), "give planned declarations distinct bindings")}
	}
	ds := append(extracted.Diagnostics, g.Derive()...)
	if len(ds) > 0 {
		return plan, loaded{}, nil, ds
	}
	plan.Graph = &g
	return plan, l, edits, nil
}
func (p *skeletonPlanner) fail(n graph.Node, message, fix string) {
	p.diagnostics = append(p.diagnostics, diag.Diagnostic{Code: "lang.golang.skeleton-failed", Subject: n.Id, Position: n.Origin, Message: message, Fix: fix})
}
func (p *skeletonPlanner) home(id string) string {
	for id != "" {
		n := p.nodes[id]
		if n.Kind == graph.NodeKindContainer && n.Form != "system" || n.Kind == graph.NodeKindActor && n.Form == "external" {
			return id
		}
		id = p.parents[id]
	}
	return ""
}
func (p *skeletonPlanner) owner(id string) string {
	owner := p.home(id)
	if p.nodes[owner].Kind != graph.NodeKindActor {
		return owner
	}
	var candidates []string
	for _, e := range p.plan.Graph.Edges() {
		if e.Kind == graph.EdgeKindUses && e.To == owner && p.nodes[e.From].Kind == graph.NodeKindContainer {
			candidates = append(candidates, e.From)
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	if len(candidates) != 1 {
		p.fail(p.nodes[id], "an external declaration has no unique client component", "give the external actor exactly one using component for initial generation, or realize its client explicitly")
		return ""
	}
	return candidates[0]
}
func (p *skeletonPlanner) modulePath() string {
	if p.module != "" {
		return p.module
	}
	src, err := os.ReadFile(filepath.Join(p.plan.Root, "go.mod"))
	if err != nil {
		return ""
	}
	file, err := modfile.Parse("go.mod", src, nil)
	if err != nil || file.Module == nil {
		return ""
	}
	p.module = file.Module.Mod.Path
	return p.module
}
func (p *skeletonPlanner) packageOf(id string) (skeletonPackage, bool) {
	owner := p.owner(id)
	if owner == "" {
		return skeletonPackage{}, false
	}
	// A method belongs beside its actual native receiver even when the
	// component owns several packages.
	if parent := p.nodes[p.parents[id]]; parent.Kind == graph.NodeKindType {
		if bindings := p.realized[parent.Id]; len(bindings) == 1 {
			declaration := p.nodes[bindings[0]]
			if declaration.Origin.Path != "" {
				file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.plan.Root, filepath.FromSlash(declaration.Origin.Path)), nil, parser.PackageClauseOnly)
				if err != nil {
					p.fail(p.nodes[id], err.Error(), "make the receiver declaration readable")
					return skeletonPackage{}, false
				}
				dir := path.Dir(declaration.Origin.Path)
				importPath := p.modulePath()
				if dir != "." {
					importPath += "/" + dir
				}
				return skeletonPackage{dir, file.Name.Name, importPath}, true
			}
		}
	}
	if pkg, ok := p.packages[owner]; ok {
		return pkg, true
	}
	n := p.nodes[owner]
	ownedFiles := map[string]bool{}
	for _, edge := range p.plan.Graph.Edges() {
		if edge.Kind == graph.EdgeKindOwns && edge.From == owner {
			ownedFiles[edge.To] = true
		}
	}
	// Existing declarations decide the directory and package clause. Multiple
	// packages are an explicit placement ambiguity, never a filename heuristic.
	dirs := map[string]string{}
	for _, file := range p.plan.Graph.Nodes() {
		if file.Layer != graph.LayerCode || file.Kind != graph.NodeKindContainer || file.Form != "file" {
			continue
		}
		if !ownedFiles[file.Id] {
			continue
		}
		rel := first(file.Attrs["path"])
		matched := false
		for _, pattern := range n.Attrs["paths"] {
			ok, err := doublestar.Match(pattern, rel)
			if err == nil && ok {
				matched = true
			}
		}
		if !matched {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.plan.Root, filepath.FromSlash(rel)), nil, parser.PackageClauseOnly)
		if err != nil {
			p.fail(n, err.Error(), "make the component source package readable")
			return skeletonPackage{}, false
		}
		// External test packages do not own production declarations.
		if strings.HasSuffix(parsed.Name.Name, "_test") {
			continue
		}
		dirs[path.Dir(rel)] = parsed.Name.Name
	}
	if len(dirs) == 0 {
		for _, pattern := range n.Attrs["paths"] {
			dir := strings.TrimSuffix(pattern, "/**")
			if dir == pattern || strings.ContainsAny(dir, "*?[]{}\\") || !filepath.IsLocal(dir) {
				continue
			}
			dirs[path.Clean(dir)] = ""
		}
	}
	if len(dirs) != 1 {
		p.fail(n, "the component has no unique Go package directory", "use one literal directory/** path or realize a declaration in the intended package first")
		return skeletonPackage{}, false
	}
	module := p.modulePath()
	if module == "" {
		p.fail(n, "the repository has no readable module declaration", "initialize a Go module with go.mod before generating skeletons")
		return skeletonPackage{}, false
	}
	dir := slices.Sorted(maps.Keys(dirs))[0]
	name := dirs[dir]
	if name == "" {
		name = path.Base(dir)
		if dir == "." {
			name = path.Base(module)
		}
		name = strings.ReplaceAll(name, "-", "_")
		if !token.IsIdentifier(name) || token.Lookup(name).IsKeyword() {
			p.fail(n, "the directory does not determine a valid Go package name", "create an ordinary package clause in the intended directory")
			return skeletonPackage{}, false
		}
	}
	importPath := module
	if dir != "." {
		importPath += "/" + dir
	}
	pkg := skeletonPackage{dir, name, importPath}
	p.packages[owner] = pkg
	return pkg, true
}
func skeletonFileName(id string, test bool) string {
	suffix := ".go"
	if test {
		suffix = "_test.go"
	}
	return fmt.Sprintf("unicode_skeleton_%x%s", sha256.Sum256([]byte(id)), suffix)
}
func (p *skeletonPlanner) edits() []lang.Edit {
	var edits []lang.Edit
	missingTypes := map[string]bool{}
	for _, n := range p.plan.Graph.Nodes() {
		if n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindType && len(p.realized[n.Id]) == 0 {
			missingTypes[n.Id] = true
		}
	}
	for _, n := range p.plan.Graph.Nodes() {
		if n.Layer != graph.LayerDesign {
			continue
		}
		manual := (n.Kind == graph.NodeKindProperty || n.Kind == graph.NodeKindFlow) && len(n.Attrs["manual"]) > 0 && len(p.checks[n.Id]) == 0
		if n.Kind != graph.NodeKindType && n.Kind != graph.NodeKindOp && !manual {
			continue
		}
		if n.Kind == graph.NodeKindType && !missingTypes[n.Id] || n.Kind == graph.NodeKindOp && (len(p.realized[n.Id]) > 0 || missingTypes[p.parents[n.Id]]) {
			continue
		}
		pkg, ok := p.packageOf(n.Id)
		if !ok {
			continue
		}
		imports := map[string]string{}
		var body string
		switch {
		case manual:
			body = p.predicate(n, pkg, imports)
		case n.Kind == graph.NodeKindType:
			body = p.typeDeclaration(n, pkg, imports)
		case n.Kind == graph.NodeKindOp:
			body = p.operation(n, pkg, imports)
		}
		if body == "" {
			continue
		}
		var src strings.Builder
		fmt.Fprintf(&src, "// Unicode skeleton. Implement the declarations in this file.\npackage %s\n", pkg.name)
		if len(imports) > 0 {
			src.WriteString("import (\n")
			for _, key := range slices.Sorted(maps.Keys(imports)) {
				fmt.Fprintf(&src, "%s %s\n", imports[key], strconv.Quote(key))
			}
			src.WriteString(")\n")
		}
		src.WriteString(body)
		formatted, err := format.Source([]byte(src.String()))
		if err != nil {
			p.fail(n, err.Error(), "correct names or conflicting signatures in the design before generating skeletons")
			continue
		}
		edits = append(edits, lang.Edit{Path: path.Join(pkg.dir, skeletonFileName(n.Id, manual)), Content: formatted, CreateOnly: true})
	}
	for _, n := range p.plan.Graph.Nodes() {
		if n.Layer != graph.LayerDesign || (n.Kind != graph.NodeKindProperty && n.Kind != graph.NodeKindFlow) {
			continue
		}
		var prior []ucSymbol
		for _, symbol := range n.Attrs["symbols"] {
			name, model, _ := strings.Cut(symbol, ": ")
			model = strings.TrimSuffix(model, " generated")
			if strings.HasSuffix(symbol, " generated") && !p.generators[n.Id+"."+name] {
				pkg, ok := p.packageOf(n.Id)
				if !ok {
					continue
				}
				imports := map[string]string{"pgregory.net/rapid": "rapid"}
				var parameters []string
				for _, input := range prior {
					parameters = append(parameters, input.Name+" "+p.goType(input.Type, pkg, imports))
				}
				prefix := "gen"
				if n.Kind == graph.NodeKindFlow {
					prefix += "Flow"
				}
				factory := prefix + upperCamel(lastName(n.Id)) + upperFirst(name)
				body := "//uc:generates " + n.Id + "." + name + "\nfunc " + factory + "(" + strings.Join(parameters, ",") + ") *rapid.Generator[" + p.goType(model, pkg, imports) + "] { panic(\"" + unimplemented + "\") }\n"
				var source strings.Builder
				fmt.Fprintf(&source, "// Unicode skeleton. Implement the declarations in this file.\npackage %s\nimport (\n", pkg.name)
				for _, key := range slices.Sorted(maps.Keys(imports)) {
					fmt.Fprintf(&source, "%s %s\n", imports[key], strconv.Quote(key))
				}
				source.WriteString(")\n" + body)
				formatted, err := format.Source([]byte(source.String()))
				if err != nil {
					p.fail(n, err.Error(), "correct the generated symbol and factory names before generation")
				} else {
					edits = append(edits, lang.Edit{Path: path.Join(pkg.dir, skeletonFileName(n.Id+"."+name, true)), Content: formatted, CreateOnly: true})
				}
			}
			prior = append(prior, ucSymbol{Name: name, Type: model})
		}
	}
	slices.SortFunc(edits, func(a, b lang.Edit) int { return strings.Compare(a.Path, b.Path) })
	return edits
}
func (p *skeletonPlanner) typeName(id string, pkg skeletonPackage, imports map[string]string) string {
	if realized := p.realized[id]; len(realized) == 1 {
		code := p.nodes[realized[0]]
		name := lastName(code.Id)
		// Type identities are go:<package>.<name>; dots in package paths are fine.
		imported, _, _ := cutLast(strings.TrimPrefix(code.Id, "go:"), ".")
		if imported == pkg.importPath {
			return name
		}
		alias := imports[imported]
		if alias == "" {
			alias = "skeletonpkg" + strconv.Itoa(len(imports))
			imports[imported] = alias
		}
		return alias + "." + name
	}
	target, ok := p.packageOf(id)
	if !ok {
		return ""
	}
	name := lastName(id)
	if target.importPath == pkg.importPath {
		return name
	}
	alias := imports[target.importPath]
	if alias == "" {
		alias = "skeletonpkg" + strconv.Itoa(len(imports))
		imports[target.importPath] = alias
	}
	return alias + "." + name
}
func (p *skeletonPlanner) goType(model string, pkg skeletonPackage, imports map[string]string) string {
	kind, args := ucSplitType(model)
	switch kind {
	case "bool", "int", "string":
		return kind
	case "float":
		return "float64"
	case "bytes":
		return "[]byte"
	case "time", "duration":
		imports["time"] = "time"
		if kind == "time" {
			return "time.Time"
		}
		return "time.Duration"
	case "list":
		return "[]" + p.goType(args[0], pkg, imports)
	case "set":
		return "map[" + p.goType(args[0], pkg, imports) + "]struct{}"
	case "map":
		return "map[" + p.goType(args[0], pkg, imports) + "]" + p.goType(args[1], pkg, imports)
	case "optional":
		t := p.goType(args[0], pkg, imports)
		if strings.HasPrefix(t, "*") {
			return t
		}
		return "*" + t
	case "call", "replies":
		return modelCarrierName(model)
	}
	name := p.typeName(model, pkg, imports)
	if p.nodes[model].Form == "object" {
		return "*" + name
	}
	return name
}
func (p *skeletonPlanner) signature(n graph.Node, pkg skeletonPackage, imports map[string]string) string {
	params, result, raises := ucSignature(first(n.Attrs["signature"]))
	var args []string
	for _, param := range params {
		args = append(args, param.Name+" "+p.goType(param.Type, pkg, imports))
	}
	var results []string
	if result != "" {
		results = append(results, p.goType(result, pkg, imports))
	}
	if len(raises) > 0 {
		results = append(results, "error")
	}
	text := "(" + strings.Join(args, ",") + ")"
	switch len(results) {
	case 0:
	case 1:
		text += " " + results[0]
	default:
		text += " (" + strings.Join(results, ",") + ")"
	}
	return text
}
func (p *skeletonPlanner) operation(n graph.Node, pkg skeletonPackage, imports map[string]string) string {
	owner := p.nodes[p.parents[n.Id]]
	name := upperFirst(lastName(n.Id))
	sig := p.signature(n, pkg, imports)
	if owner.Kind != graph.NodeKindType {
		return "//uc:realizes " + n.Id + "\nfunc " + name + sig + " { panic(\"" + unimplemented + "\") }\n"
	}
	if owner.Form == "interface" {
		p.fail(n, "an existing interface is missing a declared operation", "add the method to the handwritten interface; a separate Go file cannot extend its method set")
		return ""
	}
	receiver := p.typeName(owner.Id, pkg, imports)
	if owner.Form == "object" {
		receiver = "*" + receiver
	}
	return "func (receiver " + receiver + ") " + name + sig + " { panic(\"" + unimplemented + "\") }\n"
}
func (p *skeletonPlanner) methods(id string, seen map[string]bool) []graph.Node {
	if seen[id] {
		return nil
	}
	seen[id] = true
	var out []graph.Node
	for _, n := range p.plan.Graph.Nodes() {
		if n.Kind == graph.NodeKindOp && p.parents[n.Id] == id {
			out = append(out, n)
		}
	}
	for _, e := range p.plan.Graph.Edges() {
		if (e.Kind == graph.EdgeKindImplements || e.Kind == graph.EdgeKindExtends) && e.From == id {
			out = append(out, p.methods(e.To, seen)...)
		}
	}
	return out
}
func (p *skeletonPlanner) typeDeclaration(n graph.Node, pkg skeletonPackage, imports map[string]string) string {
	var body strings.Builder
	name := lastName(n.Id)
	fmt.Fprintf(&body, "//uc:realizes %s\ntype %s ", n.Id, name)
	if n.Form == "enum" {
		body.WriteString("string\nconst (\n")
		for _, value := range n.Attrs["values"] {
			fmt.Fprintf(&body, "%s%s %s = %s\n", name, upperFirst(value), name, strconv.Quote(value))
		}
		body.WriteString(")\n")
		return body.String()
	}
	if n.Form == "interface" {
		body.WriteString("interface {\n")
		for _, e := range p.plan.Graph.Edges() {
			if e.Kind == graph.EdgeKindExtends && e.From == n.Id {
				body.WriteString(p.typeName(e.To, pkg, imports) + "\n")
			}
		}
		for _, op := range p.plan.Graph.Nodes() {
			if op.Kind == graph.NodeKindOp && p.parents[op.Id] == n.Id {
				body.WriteString(upperFirst(lastName(op.Id)) + p.signature(op, pkg, imports) + "\n")
			}
		}
		body.WriteString("}\n")
		return body.String()
	}
	body.WriteString("struct {\n")
	for _, e := range p.plan.Graph.Edges() {
		if e.Kind == graph.EdgeKindExtends && e.From == n.Id {
			body.WriteString(p.typeName(e.To, pkg, imports) + "\n")
		}
	}
	for _, field := range n.Attrs["fields"] {
		fieldName, typ, _ := strings.Cut(field, ": ")
		fmt.Fprintf(&body, "%s %s\n", upperFirst(fieldName), p.goType(typ, pkg, imports))
	}
	body.WriteString("}\n")
	if n.Form == "error" {
		fmt.Fprintf(&body, "func (value %s) Error() string { return %s }\n", name, strconv.Quote(n.Id))
	}
	seen := map[string]bool{}
	for _, op := range p.methods(n.Id, map[string]bool{}) {
		method := upperFirst(lastName(op.Id))
		sig := p.signature(op, pkg, imports)
		if seen[method] {
			continue
		}
		seen[method] = true
		receiver := name
		if n.Form == "object" {
			receiver = "*" + name
		}
		fmt.Fprintf(&body, "func (receiver %s) %s%s { panic(%s) }\n", receiver, method, sig, strconv.Quote(unimplemented))
	}
	return body.String()
}
func (p *skeletonPlanner) predicate(n graph.Node, pkg skeletonPackage, imports map[string]string) string {
	imports["pgregory.net/rapid"] = "rapid"
	name := (&Backend{}).MapName(lang.NameRolePredicate, lastName(n.Id))
	if n.Kind == graph.NodeKindFlow {
		name = (&Backend{}).MapName(lang.NameRoleFlowPredicate, lastName(n.Id))
	}
	params := []string{}
	symbols := map[string]bool{}
	for _, text := range n.Attrs["symbols"] {
		symbol, typ, _ := strings.Cut(text, ": ")
		typ = strings.TrimSuffix(typ, " generated")
		symbols[symbol] = true
		params = append(params, symbol+" "+p.goType(typ, pkg, imports))
	}
	handle := "t"
	for symbols[handle] {
		handle += "_"
	}
	params = append([]string{handle + " rapid.TB"}, params...)
	return "//uc:checks " + n.Id + "\nfunc " + name + "(" + strings.Join(params, ",") + ") { panic(\"" + unimplemented + "\") }\n"
}

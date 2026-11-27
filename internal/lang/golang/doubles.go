package golang

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

//go:embed doubles_runtime.go
var contractDoubleRuntime string

func (c *contractCompiler) doubleModels(unit *contractUnit) []string {
	seen := map[string]bool{}
	found := map[string]bool{}
	var visit func(string)
	visit = func(model string) {
		if seen[model] {
			return
		}
		seen[model] = true
		kind, args := ucSplitType(model)
		switch kind {
		case "replies":
			found[model] = true
			for _, id := range args {
				op := unit.spec.Operations[id]
				visit(op.Result)
				for _, failure := range op.Raises {
					visit(failure)
				}
			}
			return
		case "map", "list", "set", "optional":
			for _, arg := range args {
				visit(arg)
			}
			return
		case "call":
			for _, op := range unit.spec.Types[model].Calls {
				for _, parameter := range op.Params {
					visit(parameter)
				}
				visit(op.Result)
				for _, failure := range op.Raises {
					visit(failure)
				}
			}
			return
		}
		ty := unit.spec.Types[model]
		if ty.Form == "interface" {
			found[model] = true
			for _, op := range unit.spec.Operations {
				selected, visible := ucOperationFor(&unit.spec, model, lastName(op.Name))
				if visible && selected.Name == op.Name {
					visit(op.Result)
					for _, failure := range op.Raises {
						visit(failure)
					}
				}
			}
		}
		for _, f := range ucFields(unit.spec.Types, model) {
			visit(f.Type)
		}
	}
	for _, symbol := range unit.spec.Symbols {
		visit(symbol.Type)
	}
	return slices.Sorted(maps.Keys(found))
}
func (c *contractCompiler) replySignature(unit *contractUnit, model string) (map[string]ucOperation, map[string]*types.Signature) {
	ops := map[string]ucOperation{}
	signatures := map[string]*types.Signature{}
	if kind, args := ucSplitType(model); kind == "replies" {
		if d, ok := c.bound(graph.EdgeKindRealizes, args[0]); ok {
			if fn, ok := d.obj.(*types.Func); ok {
				ops[args[0]] = unit.spec.Operations[args[0]]
				signatures[args[0]] = fn.Signature()
			}
		}
		return ops, signatures
	}
	d, ok := c.bound(graph.EdgeKindRealizes, model)
	if !ok {
		return ops, signatures
	}
	iface, ok := d.obj.Type().Underlying().(*types.Interface)
	if !ok {
		return ops, signatures
	}
	iface.Complete()
	for i := 0; i < iface.NumMethods(); i++ {
		method := iface.Method(i)
		id := ""
		if operation, ok := ucOperationFor(&unit.spec, model, designName(method.Name())); ok {
			id = operation.Name
		}
		if id != "" {
			key := lastName(id)
			ops[key] = unit.spec.Operations[id]
			signatures[key] = method.Signature()
		}
	}
	return ops, signatures
}
func (c *contractCompiler) configureDoubles(unit *contractUnit) bool {
	valid := true
	for _, model := range c.doubleModels(unit) {
		ops, sigs := c.replySignature(unit, model)
		if kind, _ := ucSplitType(model); kind == "replies" {
			unit.spec.Types[model] = ucType{Form: "replies"}
		}
		complete := true
		if unit.spec.Types[model].Form == "interface" {
			complete = false
		}
		if d, ok := c.bound(graph.EdgeKindRealizes, model); ok {
			if iface, ok := d.obj.Type().Underlying().(*types.Interface); ok {
				complete = iface.Complete().NumMethods() == len(ops)
			}
		}
		if !complete || len(ops) == 0 && unit.spec.Types[model].Form != "interface" {
			c.diagnostic(unit.node, "lang.golang.unsupported-contract", "the response script has no complete native operation binding: "+model, "realize its interface or external operation with matching Go signatures")
			valid = false
			continue
		}
		if kind, args := ucSplitType(model); kind == "replies" {
			d, ok := c.bound(graph.EdgeKindSubstitutes, args[0])
			fn, fnok := d.obj.(*types.Func)
			if !ok || !fnok || !installerMatches(fn.Signature(), sigs[args[0]]) || !c.substituteOwnersMatch(args[0]) {
				c.diagnostic(unit.node, "lang.golang.missing-substitute", "the external operation needs one installer with its native signature in the client's owning component: "+args[0], "bind a free installer in the client's component using //uc:substitutes; accept the client function signature and return func()")
				valid = false
			}
		}
	}
	// A replacement is required for every external operation reachable from the
	// tested operation or a formal expression. Manual predicates still execute
	// their formal preconditions, so their expression dependencies count too.
	seeds := map[string]bool{}
	external := map[string]string{}
	for _, e := range c.plan.Graph.Edges() {
		if e.Kind == graph.EdgeKindRealizes && c.nodes[c.parents[e.To]].Form == "external" {
			external[e.From] = e.To
		}
		if e.From == unit.node.Id && c.nodes[e.To].Kind == graph.NodeKindOp && (e.Kind == graph.EdgeKindRefers || unit.predicate == nil && e.Kind == graph.EdgeKindAbout) {
			for _, r := range c.plan.Graph.Edges() {
				if r.Kind == graph.EdgeKindRealizes && r.To == e.To {
					seeds[r.From] = true
				}
			}
		}
	}
	if unit.spec.Call != nil {
		env := ucEnvironment{spec: &unit.spec, values: map[string]ucValue{}}
		for _, s := range unit.spec.Symbols {
			env.values[s.Name] = ucValue{Type: s.Type}
		}
		callee := unit.spec.Call.Children[0]
		id := ucName(callee)
		if _, ok := unit.spec.Operations[id]; !ok && callee.Kind == "field" {
			id = env.typeOf(callee.Children[0], nil) + "." + callee.Text
		}
		for _, e := range c.plan.Graph.Edges() {
			if e.Kind == graph.EdgeKindRealizes && e.To == id {
				seeds[e.From] = true
			}
		}
	}
	queue := slices.Sorted(maps.Keys(seeds))
	required := map[string]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if target := external[id]; target != "" {
			required[target] = true
		}
		for _, e := range c.plan.Graph.Edges() {
			if (e.Kind == graph.EdgeKindCalls || e.Kind == graph.EdgeKindReaches) && e.From == id && !seeds[e.To] {
				seeds[e.To] = true
				queue = append(queue, e.To)
			}
		}
	}
	provided := map[string]int{}
	for _, symbol := range unit.spec.Symbols {
		if kind, args := ucSplitType(symbol.Type); kind == "replies" {
			provided[args[0]]++
		}
	}
	for id := range required {
		if provided[id] != 1 {
			c.diagnostic(unit.node, "lang.golang.missing-substitute", "a contract must provide exactly one response script for external operation reachable from a formal expression or tested call: "+id, "declare one forall answers: replies<"+id+"> and bind its substitute installer")
			valid = false
		}
	}
	return valid
}

func (c *contractCompiler) substituteOwnersMatch(target string) bool {
	client, clientOK := c.bound(graph.EdgeKindRealizes, target)
	installer, installerOK := c.bound(graph.EdgeKindSubstitutes, target)
	if !clientOK || !installerOK {
		return false
	}
	clientFn, clientIsFn := client.obj.(*types.Func)
	installerFn, installerIsFn := installer.obj.(*types.Func)
	if !clientIsFn || !installerIsFn || clientFn.Signature().Recv() != nil {
		return false
	}
	clientPath := c.nodes[funcID(clientFn)].Origin.Path
	installerPath := c.nodes[funcID(installerFn)].Origin.Path
	if clientPath == "" || installerPath == "" {
		return false
	}
	clientFile := "file:" + clientPath
	installerFile := "file:" + installerPath
	owners := map[string]bool{}
	for _, e := range c.plan.Graph.Edges() {
		if e.Kind == graph.EdgeKindOwns && e.To == clientFile {
			owners[e.From] = true
		}
	}
	for _, e := range c.plan.Graph.Edges() {
		if e.Kind != graph.EdgeKindOwns || e.To != installerFile || !owners[e.From] {
			continue
		}
		seen := map[string]bool{}
		for owner := e.From; owner != "" && !seen[owner]; owner = c.parents[owner] {
			seen[owner] = true
			for _, use := range c.plan.Graph.Edges() {
				if use.Kind == graph.EdgeKindUses && use.From == owner && use.To == c.parents[target] {
					return true
				}
			}
		}
	}
	return false
}
func doubleName(model string) string {
	return fmt.Sprintf("unicodeGeneratedDouble_%x", sha256.Sum256([]byte(model)))
}
func (c *contractCompiler) nativeTypeText(typ types.Type, pkg *packages.Package, imports map[string]string) string {
	return types.TypeString(typ, func(p *types.Package) string {
		if p.Path() == pkg.PkgPath {
			return ""
		}
		if alias := imports[p.Path()]; alias != "" {
			return alias
		}
		alias := "ucpkg" + strconv.Itoa(len(imports))
		imports[p.Path()] = alias
		return alias
	})
}
func (c *contractCompiler) renderDoubles(file *contractFile) string {
	var out strings.Builder
	seen := map[string]bool{}
	for _, unit := range file.units {
		for _, model := range c.doubleModels(unit) {
			if seen[model] {
				continue
			}
			seen[model] = true
			if kind, _ := ucSplitType(model); kind == "replies" {
				out.WriteString(modelCarrierDeclaration(model))
				continue
			}
			name := doubleName(model)
			fmt.Fprintf(&out, "type %s struct{_scripts map[string][]map[string]any;_cursor map[string]int;_types map[string]ucType;_ops map[string]ucReplyOperation}\n", name)
			fmt.Fprintf(&out, "func(d *%s)_unicodeCloneDouble()reflect.Value{return reflect.ValueOf(&%s{_scripts:d._scripts,_cursor:map[string]int{},_types:d._types,_ops:d._ops})}\n", name, name)
			ops, sigs := c.replySignature(unit, model)
			for _, key := range slices.Sorted(maps.Keys(ops)) {
				sig := sigs[key]
				var params, returns, locals []string
				for i := 0; i < sig.Params().Len(); i++ {
					t := c.nativeTypeText(sig.Params().At(i).Type(), file.pkg, file.imports)
					if sig.Variadic() && i == sig.Params().Len()-1 {
						t = "..." + strings.TrimPrefix(t, "[]")
					}
					params = append(params, "_ "+t)
				}
				for i := 0; i < sig.Results().Len(); i++ {
					t := c.nativeTypeText(sig.Results().At(i).Type(), file.pkg, file.imports)
					returns = append(returns, t)
					locals = append(locals, "r"+strconv.Itoa(i))
				}
				fmt.Fprintf(&out, "func(d *%s)%s(%s)", name, ops[key].GoName, strings.Join(params, ","))
				if len(returns) > 0 {
					out.WriteString("(" + strings.Join(returns, ",") + ")")
				}
				fmt.Fprintf(&out, "{cursor:=d._cursor[%s];reply:=ucNextReply(d._scripts[%s],&cursor);d._cursor[%s]=cursor;values:=ucReplyValues(d._ops[%s],reply,d._types);", strconv.Quote(key), strconv.Quote(key), strconv.Quote(key), strconv.Quote(key))
				for i, t := range returns {
					fmt.Fprintf(&out, "var r%d %s;if v:=values[%d].Interface();v!=nil{r%d=v.(%s)};", i, t, i, i, t)
				}
				if len(returns) == 0 {
					out.WriteString("_=values;")
				} else {
					out.WriteString("return " + strings.Join(locals, ","))
				}
				out.WriteString("}\n")
			}
			if d, ok := c.bound(graph.EdgeKindRealizes, model); ok {
				fmt.Fprintf(&out, "var _ %s=(*%s)(nil)\n", c.qualify(d.obj, file.pkg, file.imports), name)
			}
		}
	}
	return out.String()
}
func (c *contractCompiler) replyErrors(unit *contractUnit, op ucOperation) map[string]string {
	out := map[string]string{}
	for _, id := range slices.Sorted(maps.Keys(unit.spec.Types)) {
		ty := unit.spec.Types[id]
		if ty.Form != "error" {
			continue
		}
		allowed := slices.Contains(op.Raises, id)
		for _, parent := range ucAncestors(unit.spec.Types, id) {
			allowed = allowed || slices.Contains(op.Raises, parent)
		}
		if !allowed {
			continue
		}
		d, ok := c.bound(graph.EdgeKindRealizes, id)
		if !ok {
			continue
		}
		native := d.obj.Type()
		if !types.Implements(native, errorType.Underlying().(*types.Interface)) {
			native = types.NewPointer(native)
		}
		if types.Implements(native, errorType.Underlying().(*types.Interface)) {
			out[id] = c.nativeTypeText(native, unit.pkg, c.literalImports)
		}
	}
	return out
}
func (c *contractCompiler) renderDoubleSpec(out *strings.Builder, file *contractFile, unit *contractUnit) {
	out.WriteString("s.Installers=map[string]reflect.Value{};")
	for _, model := range c.doubleModels(unit) {
		ops, sigs := c.replySignature(unit, model)
		out.WriteString("{factory:=&ucDoubleFactory{Operations:map[string]ucReplyOperation{")
		for _, key := range slices.Sorted(maps.Keys(ops)) {
			op := ops[key]
			fmt.Fprintf(out, "%s:{Name:%s,Result:%s,Signature:reflect.TypeFor[%s](),Errors:map[string]reflect.Type{", strconv.Quote(key), strconv.Quote(op.Name), strconv.Quote(op.Result), c.nativeTypeText(sigs[key], file.pkg, file.imports))
			for _, id := range slices.Sorted(maps.Keys(c.replyErrors(unit, op))) {
				fmt.Fprintf(out, "%s:reflect.TypeFor[%s](),", strconv.Quote(id), c.replyErrors(unit, op)[id])
			}
			out.WriteString("}},")
		}
		out.WriteString("}};")
		if kind, args := ucSplitType(model); kind == "replies" {
			if d, ok := c.bound(graph.EdgeKindSubstitutes, args[0]); ok {
				selector := c.qualify(d.obj, file.pkg, file.imports)
				if selector == "" {
					c.diagnostic(unit.node, "lang.golang.missing-substitute", "the substitute installer is inaccessible from the contract package", "export the installer or place it in the contract package")
				} else {
					fmt.Fprintf(out, "s.Installers[%s]=reflect.ValueOf(%s);", strconv.Quote(args[0]), selector)
				}
			}
		} else {
			fmt.Fprintf(out, "factory.New=func(scripts map[string][]map[string]any)reflect.Value{return reflect.ValueOf(&%s{_scripts:scripts,_cursor:map[string]int{},_types:s.Types,_ops:factory.Operations})};", doubleName(model))
		}
		fmt.Fprintf(out, "ty:=s.Types[%s];ty.Double=factory;s.Types[%s]=ty;};", strconv.Quote(model), strconv.Quote(model))
	}
}
func (c *contractCompiler) doubleLiteral(value syntax.Value, t string, pkg *packages.Package) (string, bool, error) {
	for _, node := range c.nodes {
		for _, symbol := range node.Attrs["symbols"] {
			_, model, _ := strings.Cut(strings.TrimSuffix(symbol, " generated"), ": ")
			if kind, args := ucSplitType(model); kind == "replies" && t == modelCarrierName(model) {
				d, ok := c.bound(graph.EdgeKindRealizes, args[0])
				if !ok {
					return "", true, fmt.Errorf("the external operation is unrealized")
				}
				fn := d.obj.(*types.Func)
				op := c.operationOf(args[0])
				text, err := c.replyLiteral(value, op, fn.Signature(), pkg)
				return t + "{Replies:" + text + "}", true, err
			}
		}
	}
	object := c.literalTypeObject(t, pkg)
	if object == nil {
		return "", false, nil
	}
	named, ok := types.Unalias(object.Type()).(*types.Named)
	if !ok {
		return "", false, nil
	}
	object = named.Origin().Obj()
	for _, id := range slices.Sorted(maps.Keys(c.nodes)) {
		node := c.nodes[id]
		if node.Form != "interface" || node.Kind != graph.NodeKindType {
			continue
		}
		d, ok := c.bound(graph.EdgeKindRealizes, id)
		if !ok {
			continue
		}
		boundNamed, ok := types.Unalias(d.obj.Type()).(*types.Named)
		if !ok {
			continue
		}
		boundObject := boundNamed.Origin().Obj()
		if boundObject.Pkg() == nil || object.Pkg() == nil || boundObject.Pkg().Path() != object.Pkg().Path() || boundObject.Name() != object.Name() {
			continue
		}
		unit := &contractUnit{pkg: pkg, spec: ucContract{Types: map[string]ucType{}, Operations: map[string]ucOperation{}}}
		for key, n := range c.nodes {
			if n.Kind == graph.NodeKindOp {
				unit.spec.Operations[key] = c.operationOf(key)
			}
			if n.Kind == graph.NodeKindType {
				ty := ucType{Form: n.Form}

				ty.Parents = c.modelParents(key)
				ty.Parent = first(ty.Parents)
				unit.spec.Types[key] = ty
			}
		}
		ops, sigs := c.replySignature(unit, id)
		b, ok := value.(*syntax.BraceValue)
		if !ok {
			return "", true, fmt.Errorf("an interface literal needs response scripts")
		}
		var entries []string
		for _, entry := range b.Entries {
			if entry.Key.Name == nil {
				return "", true, fmt.Errorf("an interface operation key must be a name")
			}
			key := string(*entry.Key.Name)
			op, ok := ops[key]
			if !ok {
				return "", true, fmt.Errorf("unknown interface operation %s", key)
			}
			text, err := c.replyLiteral(entry.Value, op, sigs[key], pkg)
			if err != nil {
				return "", true, err
			}
			entries = append(entries, strconv.Quote(key)+":"+text)
		}
		return "s.Types[" + strconv.Quote(id) + "].Double.New(map[string][]map[string]any{" + strings.Join(entries, ",") + "}).Interface().(" + t + ")", true, nil
	}
	return "", false, nil
}

// The literal's qualifier is an emitted import alias, not a package basename.
// Resolve that alias before consulting the complete native declaration ID.
func (c *contractCompiler) literalTypeObject(text string, pkg *packages.Package) *types.TypeName {
	name := strings.TrimPrefix(text, "*")
	if original := c.foreignOriginal[name]; original != "" {
		name = original
	}
	path := pkg.PkgPath
	if alias, member, qualified := strings.Cut(name, "."); qualified {
		path = ""
		for imported, current := range c.literalImports {
			if current == alias {
				if path != "" {
					return nil
				}
				path = imported
			}
		}
		if path == "" {
			return nil
		}
		name = member
	}
	if declaration, ok := c.declarations["go:"+path+"."+name]; ok {
		object, _ := declaration.obj.(*types.TypeName)
		return object
	}
	if path == pkg.PkgPath && pkg.Types != nil {
		object, _ := pkg.Types.Scope().Lookup(name).(*types.TypeName)
		return object
	}
	return nil
}
func (c *contractCompiler) operationOf(id string) ucOperation {
	node := c.nodes[id]
	params, result, raises := ucSignature(first(node.Attrs["signature"]))
	op := ucOperation{Name: id, Owner: c.parents[id], GoName: upperFirst(lastName(id)), Result: result, Raises: raises}
	for _, p := range params {
		op.Params = append(op.Params, p.Type)
	}
	return op
}
func (c *contractCompiler) replyLiteral(value syntax.Value, op ucOperation, sig *types.Signature, pkg *packages.Package) (string, error) {
	list, ok := value.(*syntax.ListValue)
	if !ok || len(list.Items) == 0 {
		return "", fmt.Errorf("a response script must be a nonempty list")
	}
	var entries []string
	for _, item := range list.Items {
		b, ok := item.(*syntax.BraceValue)
		if !ok {
			return "", fmt.Errorf("a response must be a record")
		}
		fields := map[string]syntax.Value{}
		for _, e := range b.Entries {
			if e.Key.Name != nil {
				fields[string(*e.Key.Name)] = e.Value
			}
		}
		if v, ok := fields["returns"]; ok {
			text := "nil"
			if op.Result != "" {
				var err error
				text, err = c.valueLiteral(v, c.nativeTypeText(sig.Results().At(0).Type(), pkg, c.literalImports), pkg)
				if err != nil {
					return "", err
				}
			}
			entries = append(entries, "{\"returns\":"+text+"}")
			continue
		}
		id, ok := fields["raises"].(*syntax.StringValue)
		if !ok {
			return "", fmt.Errorf("an error response needs raises and fields")
		}
		d, ok := c.bound(graph.EdgeKindRealizes, string(id.Text))
		if !ok {
			return "", fmt.Errorf("the response error is unrealized")
		}
		native := d.obj.Type()
		if !types.Implements(native, errorType.Underlying().(*types.Interface)) {
			native = types.NewPointer(native)
		}
		text, _, err := c.replyRecordLiteral(fields["fields"], native, pkg)
		if err != nil {
			return "", err
		}
		entries = append(entries, "{\"raises\":"+strconv.Quote(string(id.Text))+",\"fields\":"+text+"}")
	}
	return "[]map[string]any{" + strings.Join(entries, ",") + "}", nil
}

// Error cases name inherited model fields directly. Go may represent them
// through embedded values or pointers, so construct that native nesting.
func (c *contractCompiler) replyRecordLiteral(value syntax.Value, native types.Type, pkg *packages.Package) (string, map[string]bool, error) {
	pointer := false
	typ := native
	if p, ok := types.Unalias(typ).(*types.Pointer); ok {
		pointer = true
		typ = p.Elem()
	}
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok {
		return "", nil, fmt.Errorf("the response error is not a named record")
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", nil, fmt.Errorf("the response error is not a struct")
	}
	b, ok := value.(*syntax.BraceValue)
	if !ok {
		return "", nil, fmt.Errorf("response error fields must be a record")
	}
	given := map[string]syntax.Value{}
	for _, entry := range b.Entries {
		if entry.Key.Name != nil {
			given[upperFirst(string(*entry.Key.Name))] = entry.Value
		}
	}
	used := map[string]bool{}
	var fields []string
	for i := 0; i < structure.NumFields(); i++ {
		field := structure.Field(i)
		if v := given[field.Name()]; v != nil {
			text, err := c.valueLiteral(v, c.nativeTypeText(field.Type(), pkg, c.literalImports), pkg)
			if err != nil {
				return "", nil, err
			}
			fields = append(fields, field.Name()+":"+text)
			used[field.Name()] = true
			continue
		}
		if field.Embedded() {
			text, keys, err := c.replyRecordLiteral(value, field.Type(), pkg)
			if err != nil {
				return "", nil, err
			}
			if len(keys) > 0 {
				fields = append(fields, field.Name()+":"+text)
				for key := range keys {
					used[key] = true
				}
			}
		}
	}
	prefix := ""
	if pointer {
		prefix = "&"
	}
	return prefix + c.nativeTypeText(typ, pkg, c.literalImports) + "{" + strings.Join(fields, ",") + "}", used, nil
}

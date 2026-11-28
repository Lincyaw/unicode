package golang

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

type measuredEdit struct {
	start, end int
	text       string
}
type measuredSpan struct{ start, end, originalStart, originalEnd int }
type measuredSource struct {
	original, modified []byte
	spans              []measuredSpan
}
type measuredProgram struct {
	profileSources      map[string]string
	functionEnds        map[string]int
	root, temp, overlay string
	tool                string
	sources             map[string]measuredSource
	packages            []string
	functions           map[string]graph.Node
	nativeErrors        map[string][]string
	graph               *graph.Graph
}

func prepareMeasuredProgram(plan lang.RunPlan, generation lang.Generation, temp string) (*measuredProgram, error) {
	loader := load
	if plan.Explore > 0 {
		loader = loadForGeneration
	}
	loaded, err := loader(plan.Root, plan.Roots)
	if err != nil {
		return nil, err
	}
	program := &measuredProgram{root: loaded.root, temp: temp, sources: map[string]measuredSource{}, functions: map[string]graph.Node{}, nativeErrors: map[string][]string{}, graph: plan.Graph, profileSources: map[string]string{}, functionEnds: map[string]int{}}
	physicalRoot, err := filepath.EvalSymlinks(loaded.root)
	if err != nil {
		return nil, err
	}
	program.root = physicalRoot
	nodes := map[string]graph.Node{}
	for _, node := range plan.Graph.Nodes() {
		nodes[node.Id] = node
		if node.Layer == graph.LayerCode && node.Kind == graph.NodeKindOp {
			program.functions[node.Id] = node
		}
	}
	publishers := map[string][]string{}
	for _, edge := range plan.Graph.Edges() {
		if edge.Kind == graph.EdgeKindPublishes {
			publishers[edge.From] = append(publishers[edge.From], edge.To)
		}
		if edge.Kind == graph.EdgeKindRealizes && nodes[edge.To].Form == "error" {
			program.nativeErrors[strings.TrimPrefix(edge.From, "go:")] = append(program.nativeErrors[strings.TrimPrefix(edge.From, "go:")], edge.To)
		}
	}
	replacements := map[string]string{}
	seen := map[string]bool{}
	for _, pkg := range loaded.packages {
		program.packages = append(program.packages, pkg.PkgPath)
		for _, file := range filesUnder(loaded.root, pkg) {
			original := filepath.Join(loaded.root, filepath.FromSlash(file.rel))
			if seen[original] {
				continue
			}
			seen[original] = true
			if unicodeGenerated(file.syntax) {
				continue
			}
			data, err := os.ReadFile(original)
			if err != nil {
				return nil, err
			}
			tokenFile := pkg.Fset.File(file.syntax.Pos())
			edits := []measuredEdit{}
			names := map[string]bool{}
			ast.Inspect(file.syntax, func(node ast.Node) bool {
				if id, ok := node.(*ast.Ident); ok {
					names[id.Name] = true
				}
				return true
			})
			unique := func(base string) string {
				for names[base] {
					base += "_"
				}
				names[base] = true
				return base
			}
			importNames := map[string]string{}
			var imports []string
			importAs := func(path, base string) string {
				if name := importNames[path]; name != "" {
					return name
				}
				for _, spec := range file.syntax.Imports {
					value, _ := strconv.Unquote(spec.Path.Value)
					if value != path {
						continue
					}
					name := filepath.Base(path)
					if spec.Name != nil {
						name = spec.Name.Name
					}
					if name != "_" && name != "." {
						importNames[path] = name
						return name
					}
				}
				name := unique(base)
				importNames[path] = name
				imports = append(imports, "import "+name+" "+strconv.Quote(path)+";")
				return name
			}
			offset := func(position token.Pos) int { return tokenFile.Offset(position) }
			for _, decl := range file.syntax.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				obj, ok := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
				if !ok {
					continue
				}
				id := funcID(obj)
				program.functionEnds[id] = pkg.Fset.Position(fn.End()).Line
				if _, ok := program.functions[id]; !ok {
					continue
				}
				tr := importAs("runtime/trace", "unicodeMeasuredTrace")
				ctx := importAs("context", "unicodeMeasuredContext")
				log := func(message string) string {
					return tr + ".Log(" + ctx + ".Background(),\"unicode\"," + strconv.Quote(message) + ");"
				}
				prefix := log("enter:"+id) + "defer " + tr + ".Log(" + ctx + ".Background(),\"unicode\"," + strconv.Quote("exit:"+id) + ");"
				if len(publishers[id]) > 0 {
					complete := unique("unicodeMeasuredPublished")
					for _, event := range publishers[id] {
						prefix += log("publication:" + event)
					}
					prefix += complete + ":=false;defer func(){if problem:=recover();problem!=nil{panic(problem)};if " + complete + "{"
					for _, event := range publishers[id] {
						prefix += log("publish:" + event)
					}
					prefix += "}}();"
					ast.Inspect(fn.Body, func(node ast.Node) bool {
						if _, ok := node.(*ast.FuncLit); ok {
							return false
						}
						if ret, ok := node.(*ast.ReturnStmt); ok {
							edits = append(edits, measuredEdit{offset(ret.Pos()), offset(ret.Pos()), complete + "=true;"})
						}
						return true
					})
					edits = append(edits, measuredEdit{offset(fn.Body.Rbrace), offset(fn.Body.Rbrace), ";" + complete + "=true;"})
				}
				results := obj.Signature().Results()
				if results.Len() > 0 && types.Implements(results.At(results.Len()-1).Type(), types.Universe.Lookup("error").Type().Underlying().(*types.Interface)) {
					resultNames := []string{}
					for _, field := range fn.Type.Results.List {
						if len(field.Names) == 0 {
							name := unique("unicodeMeasuredResult")
							resultNames = append(resultNames, name)
							text := name + " "
							if !fn.Type.Results.Opening.IsValid() {
								text = "(" + text
								edits = append(edits, measuredEdit{offset(field.Type.End()), offset(field.Type.End()), ")"})
							}
							edits = append(edits, measuredEdit{offset(field.Type.Pos()), offset(field.Type.Pos()), text})
						} else {
							for _, name := range field.Names {
								if name.Name == "_" {
									replacement := unique("unicodeMeasuredResult")
									edits = append(edits, measuredEdit{offset(name.Pos()), offset(name.End()), replacement})
									resultNames = append(resultNames, replacement)
								} else {
									resultNames = append(resultNames, name.Name)
								}
							}
						}
					}
					reflectName := importAs("reflect", "unicodeMeasuredReflect")
					errorName := unique("unicodeMeasuredError")
					walk := unique("unicodeMeasuredWalk")
					typ := unique("unicodeMeasuredType")
					errResult := resultNames[len(resultNames)-1]
					prefix += "defer func(){var " + walk + " func(error,int);" + walk + "=func(" + errorName + " error,depth int){if " + errorName + "==nil{return};if depth>128{" + log("invalid:error-unwrapping-depth") + "return};" + typ + ":=" + reflectName + ".TypeOf(" + errorName + ");for " + typ + ".Kind()==" + reflectName + ".Pointer{" + typ + "=" + typ + ".Elem()};" + tr + ".Log(" + ctx + ".Background(),\"unicode\",\"error:\"+" + typ + ".PkgPath()+\".\"+" + typ + ".Name());if u,ok:=" + errorName + ".(interface{Unwrap() error});ok{" + walk + "(u.Unwrap(),depth+1)};if u,ok:=" + errorName + ".(interface{Unwrap() []error});ok{for _,child:=range u.Unwrap(){" + walk + "(child,depth+1)}}};" + walk + "(" + errResult + ",0)}();"
				}
				edits = append(edits, measuredEdit{offset(fn.Body.Lbrace) + 1, offset(fn.Body.Lbrace) + 1, prefix})
			}
			if len(edits) == 0 {
				continue
			}
			edits = append(edits, measuredEdit{offset(file.syntax.Name.End()), offset(file.syntax.Name.End()), ";" + strings.Join(imports, "")})
			modified, spans, err := applyMeasuredEdits(data, edits)
			if err != nil {
				return nil, fmt.Errorf("instrument %s: %w", file.rel, err)
			}
			source := measuredSource{data, modified, spans}
			program.sources[file.rel] = source
			program.profileSources[pkg.PkgPath+"/"+filepath.Base(file.rel)] = file.rel
			program.profileSources[filepath.ToSlash(original)] = file.rel
			name := filepath.Join(temp, "source", strconv.Itoa(len(replacements))+".go")
			program.profileSources[pkg.PkgPath+"/"+filepath.Base(name)] = file.rel
			program.profileSources[filepath.ToSlash(name)] = file.rel
			if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
				return nil, err
			}
			if err = os.WriteFile(name, modified, 0600); err != nil {
				return nil, err
			}
			replacements[original] = name
			replacements[filepath.Join(physicalRoot, filepath.FromSlash(file.rel))] = name
		}
	}
	if plan.Explore > 0 {
		for index, edit := range generation.Edits {
			if edit.CreateOnly {
				continue
			}
			name := filepath.Join(temp, "generated", strconv.Itoa(index)+".go")
			if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
				return nil, err
			}
			if err := os.WriteFile(name, edit.Content, 0600); err != nil {
				return nil, err
			}
			replacements[filepath.Join(loaded.root, edit.Path)] = name
			replacements[filepath.Join(physicalRoot, edit.Path)] = name
		}
	}
	slices.Sort(program.packages)
	program.packages = slices.Compact(program.packages)
	encoded, err := json.Marshal(struct{ Replace map[string]string }{replacements})
	if err != nil {
		return nil, err
	}
	program.overlay = filepath.Join(temp, "overlay.json")
	if err = os.WriteFile(program.overlay, encoded, 0600); err != nil {
		return nil, err
	}
	return program, nil
}
func applyMeasuredEdits(original []byte, edits []measuredEdit) ([]byte, []measuredSpan, error) {
	slices.SortStableFunc(edits, func(a, b measuredEdit) int { return a.start - b.start })
	var out []byte
	var spans []measuredSpan
	cursor := 0
	for _, edit := range edits {
		if edit.start < cursor || edit.start < 0 || edit.end < edit.start || edit.end > len(original) {
			return nil, nil, fmt.Errorf("overlapping or invalid source edit")
		}
		out = append(out, original[cursor:edit.start]...)
		start := len(out)
		out = append(out, edit.text...)
		spans = append(spans, measuredSpan{start, len(out), edit.start, edit.end})
		cursor = edit.end
	}
	out = append(out, original[cursor:]...)
	return out, spans, nil
}
func (p *measuredProgram) compile(ctx context.Context, target string, index int) (string, error) {
	binary := filepath.Join(p.temp, fmt.Sprintf("contracts-%d.test", index))
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-buildvcs=false", "-mod=readonly", "-overlay="+p.overlay, "-toolexec="+strconv.Quote(p.tool), "-covermode=atomic", "-coverpkg="+strings.Join(p.packages, ","), "-o", binary, target)
	cmd.Dir = p.root
	cmd.Env = append(os.Environ(), p.coverToolEnvironment()...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("compile measured contracts: %w: %s", err, output)
	}
	return binary, nil
}

func measuredOffset(data []byte, line, column int) (int, bool) {
	if line < 1 || column < 1 {
		return 0, false
	}
	offset := 0
	for n := 1; n < line; n++ {
		next := strings.IndexByte(string(data[offset:]), '\n')
		if next < 0 {
			return 0, false
		}
		offset += next + 1
	}
	offset += column - 1
	return offset, offset <= len(data)
}
func measuredPosition(data []byte, offset int) (int, int) {
	offset = max(0, min(offset, len(data)))
	line := 1
	start := 0
	for i, b := range data[:offset] {
		if b == '\n' {
			line++
			start = i + 1
		}
	}
	return line, offset - start + 1
}
func (s measuredSource) originalRange(start, end int) (int, int, bool) {
	mapOffset := func(offset int, isEnd bool) (int, bool) {
		delta := 0
		for _, span := range s.spans {
			if offset < span.start {
				break
			}
			if offset < span.end {
				return span.originalStart, false
			}
			delta += (span.end - span.start) - (span.originalEnd - span.originalStart)
		}
		return offset - delta, true
	}
	a, validA := mapOffset(start, false)
	b, validB := mapOffset(end, true)
	if !validA {
		for _, span := range s.spans {
			if start >= span.start && start < span.end {
				if end <= span.end {
					return 0, 0, false
				}
				a = span.originalEnd
				break
			}
		}
	}
	if !validB {
		for _, span := range s.spans {
			if end >= span.start && end < span.end {
				b = span.originalStart
				break
			}
		}
	}
	for a < b && (s.original[a] == ' ' || s.original[a] == '\t' || s.original[a] == '\r' || s.original[a] == '\n') {
		a++
	}
	return a, b, a < b
}

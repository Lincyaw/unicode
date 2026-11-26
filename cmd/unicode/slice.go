package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
)

// sliceCommand is `unicode slice <id>... [--expand ID]... [--with-code]`.
func sliceCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	var expand []string
	withCode := false
	c := &cobra.Command{
		Use:   "slice <id>...",
		Short: "Give the least context that answers a question about the nodes (docs/design.md §7.3)",
		Long: "slice gives the nodes, their ancestors, the salient nodes one edge away, what each node's kind adds, and\n" +
			"the number of detail nodes it leaves out; --expand lists those of a node. Code is given by its position;\n" +
			"with --with-code, the JSON's code maps each code type and function of the slice to the source of its declaration.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: queryAnnotations(),
		RunE: func(_ *cobra.Command, ids []string) error {
			return withGraph(g, func(root string, gr *graph.Graph) error {
				s, err := query.Slice(gr, ids, expand)
				if err != nil {
					return queryError(err)
				}
				out := sliceOutput{Context: s}
				if withCode {
					if out.Code, err = codeOf(root, sliceNodes(s)); err != nil {
						return &exitError{code: exitUnexpected, err: err}
					}
				}
				return writeResult(g.format, out, sliceText(out), stdout)
			})
		},
	}
	c.Flags().StringArrayVar(&expand, "expand", nil, "list the detail nodes of this node too (repeatable)")
	c.Flags().BoolVar(&withCode, "with-code", false, "attach the source of each code type and function")
	return c
}

// sliceOutput is the JSON of slice: the slice, and with --with-code the
// source of each code type and function in it, keyed by identity.
type sliceOutput struct {
	query.Context
	Code map[string]string `json:"code,omitempty"`
}

// sliceNodes are every node a slice lists, once each, sorted by identity.
func sliceNodes(s query.Context) []graph.Node {
	byID := map[string]graph.Node{}
	for _, list := range [][]graph.Node{s.Focus, s.Ancestors, s.Extras} {
		for _, n := range list {
			byID[n.Id] = n
		}
	}
	for _, n := range s.Neighbors {
		byID[n.Node.Id] = n.Node
	}
	nodes := make([]graph.Node, 0, len(byID))
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		nodes = append(nodes, byID[id])
	}
	return nodes
}

// codeOf gives the source of the declaration of each code type and function
// among nodes, keyed by identity. Only Go files are read; the source is the
// top-level declaration that holds the node's origin, with its doc comment.
func codeOf(root string, nodes []graph.Node) (map[string]string, error) {
	code := map[string]string{}
	files := map[string]*ast.File{}
	sources := map[string][]byte{}
	fset := token.NewFileSet()
	for _, n := range nodes {
		path := n.Origin.Path
		if n.Layer != graph.LayerCode || n.Kind != graph.NodeKindType && n.Kind != graph.NodeKindOp || !strings.HasSuffix(path, ".go") {
			continue
		}
		if _, read := files[path]; !read {
			src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				return nil, fmt.Errorf("cannot read the source of %s: %w", n.Id, err)
			}
			f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("cannot parse the source of %s: %w", n.Id, err)
			}
			files[path], sources[path] = f, src
		}
		if text, ok := declarationAt(fset, files[path], sources[path], n.Origin.Line, n.Origin.Column); ok {
			code[n.Id] = text
		}
	}
	return code, nil
}

// declarationAt is the source of the top-level declaration of f that holds
// the position line:column, from its doc comment to its end; in a grouped
// type declaration, of the type spec that holds it.
func declarationAt(fset *token.FileSet, f *ast.File, src []byte, line, column int) (string, bool) {
	tf := fset.File(f.Pos())
	if line < 1 || line > tf.LineCount() {
		return "", false
	}
	pos := tf.LineStart(line) + token.Pos(column-1)
	text := func(doc *ast.CommentGroup, from, to token.Pos) string {
		if doc != nil {
			from = doc.Pos()
		}
		return string(src[tf.Offset(from):tf.Offset(to)])
	}
	for _, d := range f.Decls {
		if pos < d.Pos() || pos >= d.End() {
			continue
		}
		switch d := d.(type) {
		case *ast.FuncDecl:
			return text(d.Doc, d.Pos(), d.End()), true
		case *ast.GenDecl:
			if d.Lparen.IsValid() {
				for _, s := range d.Specs {
					if s, ok := s.(*ast.TypeSpec); ok && pos >= s.Pos() && pos < s.End() {
						return text(s.Doc, s.Pos(), s.End()), true
					}
				}
			}
			return text(d.Doc, d.Pos(), d.End()), true
		}
	}
	return "", false
}

func sliceText(s sliceOutput) []byte {
	var b strings.Builder
	section(&b, "focus", nodeLines(s.Focus))
	section(&b, "ancestors", nodeLines(s.Ancestors))
	var neighbors []string
	for _, n := range s.Neighbors {
		neighbors = append(neighbors, fmt.Sprintf("%s, along %s: %s", n.Focus, edgeText(n.Edge), nodeText(n.Node)))
	}
	section(&b, "neighbors", neighbors)
	section(&b, "extras", nodeLines(s.Extras))
	var details []string
	for _, f := range slices.Sorted(maps.Keys(s.Details)) {
		details = append(details, fmt.Sprintf("%s: %d detail nodes not listed; --expand %s lists them", f, s.Details[f], f))
	}
	section(&b, "details", details)
	for _, id := range slices.Sorted(maps.Keys(s.Code)) {
		fmt.Fprintf(&b, "code of %s:\n%s\n", id, s.Code[id])
	}
	return []byte(b.String())
}

package serve

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang/golang"
)

// files are the files the interface may read, each with its language, ""
// for a design or corpus file: those the snapshot's nodes are written in,
// and its code files.
func files(nodes map[string]graph.Node) map[string]string {
	out := map[string]string{}
	for _, n := range nodes {
		if p := n.Origin.Path; p != "" {
			if _, known := out[p]; !known {
				out[p] = ""
			}
		}
		if n.Layer == graph.LayerCode && n.Kind == graph.NodeKindContainer && n.Form == "file" {
			if p := n.Attrs["path"]; len(p) > 0 {
				out[p[0]] = strings.Join(n.Attrs["language"], "")
			}
		}
	}
	return out
}

// read reads a file the interface may read from the working tree.
func (st *state) read(path string) ([]byte, error) {
	if _, known := st.files[path]; !known || !filepath.IsLocal(filepath.FromSlash(path)) {
		return nil, refuse(http.StatusBadRequest, "serve.bad-path", "give the path of a design or corpus file, or of a code file of the snapshot",
			"%q is not a file the snapshot describes", path)
	}
	src, err := os.ReadFile(filepath.Join(st.root, filepath.FromSlash(path)))
	if err != nil {
		why := "cannot be read"
		if errors.Is(err, fs.ErrNotExist) {
			why = "no longer exists in the working tree"
		}
		return nil, refuse(http.StatusNotFound, "serve.not-found", "run unicode build to read the working tree again",
			"the file %s %s: %v", path, why, err)
	}
	return src, nil
}

// lines splits a file into its lines, each with its newline; the last one
// gets one when the file does not end in a newline.
func lines(src []byte) []string {
	out := strings.SplitAfter(string(src), "\n")
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if n := len(out); n > 0 && !strings.HasSuffix(out[n-1], "\n") {
		out[n-1] += "\n"
	}
	return out
}

func (st *state) excerpt(path string, all []string, from, to int) CodeExcerpt {
	e := CodeExcerpt{Path: path, From: int32(from), To: int32(to), Text: strings.Join(all[from-1:to], "")}
	if language := st.files[path]; language != "" {
		e.Language = &language
	}
	return e
}

// code is the lines from to to of a file, the whole file when they are not given.
func (st *state) code(path string, from, to *int32) (CodeExcerpt, error) {
	src, err := st.read(path)
	if err != nil {
		return CodeExcerpt{}, err
	}
	all := lines(src)
	first, last := 1, len(all)
	if from != nil {
		first = int(*from)
	}
	if to != nil {
		last = min(int(*to), len(all))
	}
	switch {
	case first < 1 || first > max(len(all), 1):
		return CodeExcerpt{}, refuse(http.StatusBadRequest, "serve.bad-parameter", "give from between 1 and the number of lines",
			"the file %s has %d lines, so it has no line %d", path, len(all), first)
	case last < first && len(all) > 0:
		return CodeExcerpt{}, refuse(http.StatusBadRequest, "serve.bad-parameter", "give a to that is not before from",
			"the lines %d to %d of %s are none", first, last, path)
	}
	return st.excerpt(path, all, first, last), nil
}

// declaration is the source of a Go code type's or function's declaration.
func (st *state) declaration(id string) (CodeExcerpt, bool) {
	n := st.nodes[id]
	if n.Layer != graph.LayerCode || n.Kind != graph.NodeKindType && n.Kind != graph.NodeKindOp || !strings.HasSuffix(n.Origin.Path, ".go") {
		return CodeExcerpt{}, false
	}
	src, err := st.read(n.Origin.Path)
	if err != nil {
		return CodeExcerpt{}, false
	}
	l := golang.Declaration(src, n.Origin.Line, n.Origin.Column)
	if l == nil {
		return CodeExcerpt{}, false
	}
	return st.excerpt(n.Origin.Path, lines(src), l.From, l.To), true
}

// line is the line n of a file the interface may read.
func (st *state) line(path string, n int) (string, bool) {
	src, err := st.read(path)
	if err != nil {
		return "", false
	}
	all := lines(src)
	if n < 1 || n > len(all) {
		return "", false
	}
	return all[n-1], true
}

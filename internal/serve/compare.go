package serve

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang/golang"
	"github.com/lincyaw/unicode/internal/repo"
)

// compared is the served snapshot compared with the graph of a base commit,
// as unicode diff compares them.
type compared struct {
	base  repo.Revision
	graph *state
	c     graph.Comparison
}

// baseEntry is the graph of one base commit, built once.
type baseEntry struct {
	once sync.Once
	st   *state
	err  error
}

// compare compares the served snapshot with the graph of the commit rev,
// which the builder builds once for the server's life; the comparison is
// kept with the snapshot. A rev that names no commit is refused with
// serve.bad-revision.
func (s *Server) compare(st *state, rev string) (*compared, error) {
	r, err := repo.ReadRevision(st.root, rev)
	switch {
	case errors.As(err, new(repo.BadRevision)) || errors.As(err, new(repo.NoGit)):
		return nil, refuse(http.StatusBadRequest, "serve.bad-revision", "give a commit of the repository, such as HEAD~1 or a hash from /api/revisions", "%v", err)
	case err != nil:
		return nil, err
	}
	e, _ := s.bases.LoadOrStore(r.Commit, &baseEntry{})
	entry := e.(*baseEntry)
	entry.once.Do(func() {
		g, err := s.builder.Build(st.root, r.Commit)
		if err != nil {
			entry.err = Unbuildable{Commit: r.Commit, Cause: err}
			return
		}
		entry.st = indexed(st.root, g)
	})
	if entry.err != nil {
		return nil, entry.err
	}
	c, ok := st.comparisons.Load(r.Commit)
	if !ok {
		c, _ = st.comparisons.LoadOrStore(r.Commit, graph.Compare(&entry.st.g, &st.g))
	}
	return &compared{base: r, graph: entry.st, c: c.(graph.Comparison)}, nil
}

// comparison writes a comparison as the interface gives it: the base
// commit, the working tree of the snapshot, and each changed node and edge
// as it is in the base and in the snapshot.
func (st *state) comparison(c *compared) Comparison {
	out := Comparison{Base: revision(c.base), Head: st.head, Nodes: []NodeChange{}, Edges: []EdgeChange{}}
	for _, n := range c.c.Nodes {
		out.Nodes = append(out.Nodes, st.nodeChange(c, n.Id, n.Change, n.Attrs))
	}
	for _, e := range c.c.Edges {
		change := EdgeChange{Kind: EdgeKind(e.Kind), From: e.From, To: e.To, Change: ChangeKind(e.Change)}
		if e.Before != nil {
			before := c.graph.edge(*e.Before)
			change.Before = &before
		}
		if e.After != nil {
			after := st.edge(*e.After)
			change.After = &after
		}
		out.Edges = append(out.Edges, change)
	}
	return out
}

func (st *state) nodeChange(c *compared, id string, kind graph.ChangeKind, attrs []graph.AttrChange) NodeChange {
	change := NodeChange{Id: id, Change: ChangeKind(kind), Attrs: []AttrChange{}}
	if _, ok := c.graph.nodes[id]; ok {
		before := c.graph.node(id)
		change.Before = &before
	}
	if _, ok := st.nodes[id]; ok {
		after := st.node(id)
		change.After = &after
	}
	for _, a := range attrs {
		change.Attrs = append(change.Attrs, AttrChange{Key: a.Key, Removed: append([]string{}, a.Removed...), Added: append([]string{}, a.Added...)})
	}
	return change
}

// changes are the changes of a node and of the code elements that realize
// it in the base or in the snapshot, the node first: each node changed,
// added, removed or unchanged, and a code type or function with the lines
// of its declaration that differ.
func (st *state) changes(id string, c *compared) []NodeChange {
	byID := map[string]graph.NodeChange{}
	for _, n := range c.c.Nodes {
		byID[n.Id] = n
	}
	var realizers []string
	for _, side := range []*state{c.graph, st} {
		for _, r := range side.sources(id, graph.EdgeKindRealizes) {
			if side.nodes[r].Layer == graph.LayerCode && r != id {
				realizers = append(realizers, r)
			}
		}
	}
	out := []NodeChange{}
	for _, x := range append([]string{id}, sorted(realizers)...) {
		n, changed := byID[x]
		if !changed {
			n = graph.NodeChange{Id: x, Change: graph.ChangeKindUnchanged}
		}
		change := st.nodeChange(c, x, n.Change, n.Attrs)
		change.LinesChanged = st.linesChanged(c, x)
		out = append(out, change)
	}
	return out
}

// linesChanged counts the lines that differ between a code type's or
// function's declaration in the base commit and in the working tree; a
// declaration missing on one side counts as empty. It is nil for any other
// node, or when a side cannot be read.
func (st *state) linesChanged(c *compared, id string) *int32 {
	var texts [2]string
	for i, side := range []*state{c.graph, st} {
		n, ok := side.nodes[id]
		switch {
		case !ok:
			continue
		case n.Layer != graph.LayerCode || n.Kind != graph.NodeKindType && n.Kind != graph.NodeKindOp || !strings.HasSuffix(n.Origin.Path, ".go"):
			return nil
		}
		var src []byte
		var err error
		if side == st {
			src, err = st.read(n.Origin.Path)
		} else {
			src, err = repo.Show(st.root, c.base.Commit, n.Origin.Path)
		}
		if err != nil {
			return nil
		}
		l := golang.Declaration(src, n.Origin.Line, n.Origin.Column)
		if l == nil {
			return nil
		}
		texts[i] = strings.Join(lines(src)[l.From-1:l.To], "")
	}
	count, err := repo.LinesChanged(texts[0], texts[1])
	if err != nil {
		return nil
	}
	n := int32(count)
	return &n
}

// revision writes a commit as the interface gives it.
func revision(r repo.Revision) Revision {
	return Revision{Commit: r.Commit, Short: r.Short, Branch: optional(r.Branch), Subject: r.Subject, Time: r.Time}
}

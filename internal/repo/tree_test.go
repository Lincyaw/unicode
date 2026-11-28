package repo

import (
	"pgregory.net/rapid"
)

// genTree draws a small tree: each entry under a directory of the tree, a
// third directories, a third files with any bytes, the rest links.
//
//uc:generates repo.read-tree-inverts-materialize.tree
func genTree() *rapid.Generator[Tree] {
	return rapid.Custom(func(t *rapid.T) Tree {
		tree := Tree{Entries: map[string]Entry{}}
		dirs := []string{""}
		for range rapid.IntRange(0, 10).Draw(t, "entries") {
			parent := rapid.SampledFrom(dirs).Draw(t, "parent")
			name := rapid.SampledFrom([]string{"a", "b", "c"}).Draw(t, "name")
			p := name
			if parent != "" {
				p = parent + "/" + name
			}
			if _, taken := tree.Entries[p]; taken || len(p) > len("a/b/c") {
				continue
			}
			switch rapid.IntRange(0, 2).Draw(t, "kind") {
			case 0:
				tree.Entries[p] = Entry{Kind: EntryKindDir, Mode: rapid.SampledFrom([]int{0o700, 0o750, 0o755}).Draw(t, "dirMode")}
				dirs = append(dirs, p)
			case 1:
				tree.Entries[p] = Entry{Kind: EntryKindFile, Content: rapid.SliceOfN(rapid.Byte(), 0, 16).Draw(t, "content"),
					Mode: rapid.SampledFrom([]int{0o600, 0o640, 0o644, 0o700, 0o755}).Draw(t, "fileMode")}
			default:
				tree.Entries[p] = Entry{Kind: EntryKindLink, Target: rapid.SampledFrom([]string{"a", "../x", "b/c", "/nonexistent"}).Draw(t, "target")}
			}
		}
		return tree
	})
}

//uc:generates repo.read-tree-inverts-materialize.dir
func deriveTreeDir(tree Tree) (string, error) { return Materialize(tree) }

package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
)

// EntryKind is what a directory entry is.
//
//uc:realizes repo.EntryKind
type EntryKind string

const (
	EntryKindFile EntryKind = "file"
	EntryKindDir  EntryKind = "dir"
	EntryKindLink EntryKind = "link"
)

// Entry is one entry of a directory tree: a regular file with its content, a
// directory, or a symbolic link with its target; Mode is the permission bits,
// 0 for a link.
//
//uc:realizes repo.Entry
type Entry struct {
	Kind    EntryKind
	Content []byte
	Target  string
	Mode    int
}

// Tree is a directory tree: slash-separated relative paths to entries.
// Directories and links are entries; links are not followed.
//
//uc:realizes repo.Tree
type Tree struct {
	Entries map[string]Entry
}

// BadTree is the error of materializing a tree that has an entry whose path
// is not a clean relative path, whose parent is not a directory of the tree,
// or that cannot be written.
//
//uc:realizes repo.BadTree
type BadTree struct {
	Path  string
	Cause error
}

func (e BadTree) Error() string {
	return fmt.Sprintf("cannot write the entry %q of the tree: %v", e.Path, e.Cause)
}

func (e BadTree) Unwrap() error { return e.Cause }

// Unreadable is the error of reading a directory, or an entry under it,
// that cannot be read.
//
//uc:realizes repo.Unreadable
type Unreadable struct {
	Path  string
	Cause error
}

func (e Unreadable) Error() string { return fmt.Sprintf("cannot read %s: %v", e.Path, e.Cause) }

func (e Unreadable) Unwrap() error { return e.Cause }

// Materialize makes a new temporary directory and writes the tree into it:
// parents first, each directory, file and link at its path with its mode,
// directories' modes set after their contents, so that a read-only directory
// still receives its entries. It returns the directory's path.
//
//uc:realizes repo.materialize
func Materialize(tree Tree) (string, error) {
	dir, err := os.MkdirTemp("", "unicode-tree-")
	if err != nil {
		return "", BadTree{Path: ".", Cause: err}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", BadTree{Path: ".", Cause: err}
	}
	defer func() { _ = root.Close() }()
	paths := slices.Sorted(maps.Keys(tree.Entries))
	for _, p := range paths {
		if err := writeEntry(root, tree, p); err != nil {
			return "", BadTree{Path: p, Cause: err}
		}
	}
	slices.Reverse(paths)
	for _, p := range paths {
		if e := tree.Entries[p]; e.Kind == EntryKindDir {
			if err := root.Chmod(p, fs.FileMode(e.Mode).Perm()); err != nil {
				return "", BadTree{Path: p, Cause: err}
			}
		}
	}
	return dir, nil
}

func writeEntry(root *os.Root, tree Tree, p string) error {
	if p == "" || p == "." || !fs.ValidPath(p) {
		return errors.New("the path is not a clean slash-separated relative path")
	}
	if parent := path.Dir(p); parent != "." {
		if tree.Entries[parent].Kind != EntryKindDir {
			return errors.New("the parent of the entry is not a directory of the tree")
		}
	}
	e := tree.Entries[p]
	switch e.Kind {
	case EntryKindDir:
		return root.Mkdir(p, 0o700)
	case EntryKindFile:
		if err := root.WriteFile(p, e.Content, 0o600); err != nil {
			return err
		}
		return root.Chmod(p, fs.FileMode(e.Mode).Perm())
	case EntryKindLink:
		return root.Symlink(e.Target, p)
	}
	return fmt.Errorf("the entry kind %q is not file, dir or link", e.Kind)
}

// ReadTree reads every entry under dir without following links: a directory
// with its permission bits, a regular file with its content and permission
// bits, and a link with its target. Anything else, or an entry it cannot
// read, is Unreadable.
//
//uc:realizes repo.readTree
func ReadTree(dir string) (Tree, error) {
	tree := Tree{Entries: map[string]Entry{}}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Tree{}, Unreadable{Path: dir, Cause: err}
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		info, err := root.Lstat(p)
		if err != nil {
			return err
		}
		mode := int(info.Mode().Perm())
		switch {
		case info.IsDir():
			tree.Entries[p] = Entry{Kind: EntryKindDir, Mode: mode}
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := root.Readlink(p)
			if err != nil {
				return err
			}
			tree.Entries[p] = Entry{Kind: EntryKindLink, Target: target}
		case info.Mode().IsRegular():
			content, err := root.ReadFile(p)
			if err != nil {
				return err
			}
			tree.Entries[p] = Entry{Kind: EntryKindFile, Content: content, Mode: mode}
		default:
			return fmt.Errorf("%s is neither a directory, a regular file nor a symbolic link", filepath.FromSlash(p))
		}
		return nil
	})
	if err != nil {
		return Tree{}, Unreadable{Path: dir, Cause: err}
	}
	return tree, nil
}

package golang

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"pgregory.net/rapid"
)

// The template properties call the derivation runtime of generated drivers
// (ucDerive) directly, as a driver does for a derived input dir: string
// computed from files: map<string, string>, with a content cache of their own.

// templateFixture is the contract of the derived input, and the setting of
// a private content cache that holds the templates.
func templateFixture(t rapid.TB) (*ucContract, func()) {
	cache, err := os.MkdirTemp("", "unicode-template-cache-")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	previous, had := os.LookupEnv("UNICODE_CACHE")
	if err := os.Setenv("UNICODE_CACHE", cache); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	s := &ucContract{ID: "lang.golang.template-fixture", Symbols: []ucSymbol{{Name: "files", Type: "map<string, string>"}, {Name: "dir", Type: "string"}}, Derived: []string{"dir"}, Types: map[string]ucType{}}
	return s, func() {
		ucCleanDerived()
		if had {
			_ = os.Setenv("UNICODE_CACHE", previous)
		} else {
			_ = os.Unsetenv("UNICODE_CACHE")
		}
		_ = os.RemoveAll(cache)
	}
}

// materializeFiles writes files into a new directory under the temporary
// directory, with a relative link to the first directory and an executable
// copy of the first file in the order of their names, and returns the
// directory. With naming, it also
// writes its own path into where.txt.
func materializeFiles(files map[string]string, naming bool) (string, error) {
	dir, err := os.MkdirTemp("", "materialized-")
	if err != nil {
		return "", err
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	if names := slices.Sorted(maps.Keys(files)); len(names) > 0 {
		if err := os.Symlink(strings.SplitN(names[0], "/", 2)[0], filepath.Join(dir, "link")); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, "tool.sh"), []byte(files[names[0]]), 0o755); err != nil {
			return "", err
		}
	}
	if naming {
		if err := os.WriteFile(filepath.Join(dir, "where.txt"), []byte(dir+"\n"), 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}

//uc:checks lang.golang.derived-template-matches-fresh-derivation
func checkDerivedTemplateMatchesFreshDerivation(t rapid.TB, module Module) {
	s, restore := templateFixture(t)
	defer restore()
	calls := 0
	derive := func() (any, error) { calls++; return materializeFiles(module.Files, false) }
	values := map[string]reflect.Value{"files": reflect.ValueOf(module.Files)}
	first := ucDerive(s, "dir", []string{"files"}, values, reflect.TypeFor[string](), derive).String()
	firstRoot := ucDerivedRoots[len(ucDerivedRoots)-1]
	second := ucDerive(s, "dir", []string{"files"}, values, reflect.TypeFor[string](), derive).String()
	secondRoot := ucDerivedRoots[len(ucDerivedRoots)-1]
	if calls != 1 {
		t.Fatalf("deriving twice from the same files called the derivation %d times instead of copying its template", calls)
	}
	firstRel, err1 := filepath.Rel(firstRoot, first)
	secondRel, err2 := filepath.Rel(secondRoot, second)
	if err1 != nil || err2 != nil || firstRel != secondRel || !filepath.IsLocal(secondRel) {
		t.Fatalf("the copied value %s is not at %s within its own directory %s", second, firstRel, secondRoot)
	}
	freshRoot, err := os.MkdirTemp("", "unicode-template-fresh-")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(freshRoot) }()
	previous := os.Getenv("TMPDIR")
	_ = os.Setenv("TMPDIR", freshRoot)
	fresh, err := materializeFiles(module.Files, false)
	_ = os.Setenv("TMPDIR", previous)
	if err != nil {
		t.Fatalf("setup failed: the fresh derivation failed: %v", err)
	}
	got, want := templateTree(t, second), templateTree(t, fresh)
	if !maps.Equal(got, want) {
		t.Fatalf("the template copy differs from a fresh derivation\ncopy:  %v\nfresh: %v", got, want)
	}
}

//uc:checks lang.golang.derived-template-skips-self-naming-state
func checkDerivedTemplateSkipsSelfNamingState(t rapid.TB, module Module) {
	s, restore := templateFixture(t)
	defer restore()
	calls := 0
	derive := func() (any, error) { calls++; return materializeFiles(module.Files, true) }
	values := map[string]reflect.Value{"files": reflect.ValueOf(module.Files)}
	first := ucDerive(s, "dir", []string{"files"}, values, reflect.TypeFor[string](), derive).String()
	second := ucDerive(s, "dir", []string{"files"}, values, reflect.TypeFor[string](), derive).String()
	if calls != 2 {
		t.Fatalf("a state naming its own directory was derived %d times for two cases, so the second used a template", calls)
	}
	for _, dir := range []string{first, second} {
		where, err := os.ReadFile(filepath.Join(dir, "where.txt"))
		if err != nil || strings.TrimSpace(string(where)) != dir {
			t.Fatalf("the state at %s does not name its own directory: %q %v", dir, where, err)
		}
	}
}

// templateTree describes the tree at root: each path relative to it with
// its kind, permission bits, and content or link target.
func templateTree(t rapid.TB, root string) map[string]string {
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			tree[rel] = "link " + target
			return err
		case d.IsDir():
			tree[rel] = "dir " + info.Mode().Perm().String()
		default:
			content, err := os.ReadFile(path)
			tree[rel] = "file " + info.Mode().Perm().String() + " " + string(content)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cannot read the tree at %s: %v", root, err)
	}
	return tree
}

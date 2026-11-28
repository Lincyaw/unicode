package golang

import (
	"fmt"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"pgregory.net/rapid"
)

// CacheChange is one change to a module after its first extraction.
//
//uc:realizes lang.golang.CacheChange
type CacheChange struct{ Kind, Pick, Value int }

//uc:generates lang.golang.extraction-cache-follows-changes.change
func genCacheChange() *rapid.Generator[CacheChange] {
	return rapid.Custom(func(t *rapid.T) CacheChange {
		return CacheChange{rapid.IntRange(0, 3).Draw(t, "kind"), rapid.IntRange(0, 1000).Draw(t, "pick"), rapid.IntRange(0, 1000).Draw(t, "value")}
	})
}

//uc:checks lang.golang.extraction-cache-hits-moved-module
func checkExtractionCacheHitsMovedModule(t rapid.TB, module Module, forGeneration bool) {
	cache, removeCache := privateCache(t)
	defer removeCache()
	first, removeFirst := writeModule(t, module)
	defer removeFirst()
	second, removeSecond := writeModule(t, module)
	defer removeSecond()
	if _, hit, err := cache.extraction(first, []string{"."}, forGeneration); err != nil || hit {
		t.Fatalf("the first extraction into an empty cache failed or hit (hit=%t): %v\n%s", hit, err, listing(module))
	}
	got, hit, err := cache.extraction(second, []string{"."}, forGeneration)
	if err != nil {
		t.Fatalf("the cached extraction of the moved module failed: %v", err)
	}
	if !hit {
		t.Fatalf("the same module in another directory missed the content cache\n%s", listing(module))
	}
	want, err := freshExtraction(second, []string{"."}, forGeneration)
	if err != nil {
		t.Fatalf("the uncached extraction of the moved module failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the cached extraction differs from the uncached one in the same directory\ncached:   %+v\nuncached: %+v", got, want)
	}
}

//uc:checks lang.golang.extraction-cache-follows-changes
func checkExtractionCacheFollowsChanges(t rapid.TB, module Module, change CacheChange) {
	cache, removeCache := privateCache(t)
	defer removeCache()
	dir, remove := writeModule(t, module)
	defer remove()
	if _, _, err := cache.extraction(dir, []string{"."}, false); err != nil {
		t.Fatalf("extracting the generated module failed: %v\n%s", err, listing(module))
	}
	applyCacheChange(t, dir, module, change)
	got, _, gotErr := cache.extraction(dir, []string{"."}, false)
	want, wantErr := freshExtraction(dir, []string{"."}, false)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("after change %+v the cached extraction gives error %v, the uncached one %v", change, gotErr, wantErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after change %+v the cached extraction differs from the uncached one\ncached:   %+v\nuncached: %+v", change, got, want)
	}
}

// applyCacheChange makes the change in the module written at dir: kind 0
// appends a function to a source file, 1 adds a source file to a package,
// 2 deletes a test file (none when the module has none), 3 rewrites the go
// line of go.mod.
func applyCacheChange(t rapid.TB, dir string, module Module, change CacheChange) {
	var sources, tests []string
	for _, name := range slices.Sorted(maps.Keys(module.Files)) {
		if !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "rapid/") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			tests = append(tests, name)
		} else {
			sources = append(sources, name)
		}
	}
	if len(sources) == 0 {
		t.Fatalf("setup failed: the generated module has no source file\n%s", listing(module))
	}
	probe := fmt.Sprintf("\nfunc UcCacheProbe%d() int { return %d }\n", change.Value, change.Value)
	switch change.Kind {
	case 0:
		name := sources[change.Pick%len(sources)]
		writeModuleFile(t, dir, name, module.Files[name]+probe)
	case 1:
		name := sources[change.Pick%len(sources)]
		file, err := parser.ParseFile(token.NewFileSet(), name, module.Files[name], parser.PackageClauseOnly)
		if err != nil {
			t.Fatalf("setup failed: cannot read the package of %s: %v", name, err)
		}
		writeModuleFile(t, dir, path.Join(path.Dir(name), "cache_probe.go"), "package "+file.Name.Name+"\n"+probe)
	case 2:
		if len(tests) == 0 {
			return
		}
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(tests[change.Pick%len(tests)]))); err != nil {
			t.Fatalf("setup failed: cannot delete a test file: %v", err)
		}
	case 3:
		mod := module.Files["go.mod"]
		if !strings.Contains(mod, "\ngo 1.26\n") {
			t.Fatalf("setup failed: go.mod has no go 1.26 line:\n%s", mod)
		}
		writeModuleFile(t, dir, "go.mod", strings.Replace(mod, "\ngo 1.26\n", fmt.Sprintf("\ngo 1.%d\n", 24+change.Value%3), 1))
	default:
		t.Fatalf("setup failed: unknown change kind %d", change.Kind)
	}
}

// privateCache is a content cache in a new temporary directory, so that a
// property sees only its own entries.
func privateCache(t rapid.TB) (contentCache, func()) {
	dir, err := os.MkdirTemp("", "unicode-cache-")
	if err != nil {
		t.Fatalf("setup failed: cannot make a cache directory: %v", err)
	}
	return contentCache{dir: dir}, func() { _ = os.RemoveAll(dir) }
}

// writeModule writes the module's files into a new temporary directory.
func writeModule(t rapid.TB, module Module) (string, func()) {
	dir, err := os.MkdirTemp("", "unicode-golang-")
	if err != nil {
		t.Fatalf("setup failed: cannot make a directory to write the module in: %v", err)
	}
	for name, content := range module.Files {
		writeModuleFile(t, dir, name, content)
	}
	return dir, func() { _ = os.RemoveAll(dir) }
}

func writeModuleFile(t rapid.TB, dir, name, content string) {
	file := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("setup failed: cannot make the directory of %s: %v", name, err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("setup failed: cannot write %s: %v", name, err)
	}
}

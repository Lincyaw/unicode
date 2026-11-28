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

	"github.com/lincyaw/unicode/internal/lang"
)

// CacheChange is one change to a module after its first extraction.
//
//uc:realizes lang.golang.CacheChange
type CacheChange struct{ Kind, Pick, Value int }

//uc:generates lang.golang.extraction-cache-follows-changes.change
//uc:generates lang.golang.planned-check-cache-follows-changes.change
func genCacheChange() *rapid.Generator[CacheChange] {
	return rapid.Custom(func(t *rapid.T) CacheChange {
		return CacheChange{rapid.IntRange(0, cacheChangeKinds-1).Draw(t, "kind"), rapid.IntRange(0, 1000).Draw(t, "pick"), rapid.IntRange(0, 1000).Draw(t, "value")}
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
func checkExtractionCacheFollowsChanges(t rapid.TB, module Module, change CacheChange, forGeneration bool) {
	cache, removeCache := privateCache(t)
	defer removeCache()
	setup := setUpCacheChange(t, module, change)
	defer setup.remove()
	if _, _, err := cache.extraction(setup.root, []string{"."}, forGeneration); err != nil {
		t.Fatalf("extracting the generated module before change %+v failed: %v\n%s", change, err, listing(module))
	}
	restore := setup.change(t)
	defer restore()
	got, _, gotErr := cache.extraction(setup.root, []string{"."}, forGeneration)
	want, wantErr := freshExtraction(setup.root, []string{"."}, forGeneration)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("after change %+v the cached extraction gives error %v, the uncached one %v", change, gotErr, wantErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after change %+v the cached extraction differs from the uncached one\ncached:   %+v\nuncached: %+v", change, got, want)
	}
}

//uc:checks lang.golang.planned-check-cache-follows-changes
func checkPlannedCheckCacheFollowsChanges(t rapid.TB, module Module, change CacheChange) {
	cache, removeCache := privateCache(t)
	defer removeCache()
	setup := setUpCacheChange(t, module, change)
	defer setup.remove()
	plan := lang.GenPlan{Root: setup.root, Roots: []string{"."}}
	edits := []lang.Edit{{Path: path.Join(setup.pkgDir, "cache_planned.go"), Content: []byte("package " + setup.pkgName + "\n\nfunc ucCachePlanned() int { return 1 }\n")}}
	if err := cache.plannedLoads(plan, edits); err != nil {
		t.Fatalf("checking the planned file before change %+v failed: %v\n%s", change, err, listing(module))
	}
	restore := setup.change(t)
	defer restore()
	gotErr := cache.plannedLoads(plan, edits)
	_, wantErr := loadPlannedGo(plan, edits, false)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("after change %+v the cached check of the planned file gives error %v, the uncached load %v", change, gotErr, wantErr)
	}
}

// cacheChangeKinds is how many kinds of change setUpCacheChange knows.
const cacheChangeKinds = 10

// cacheChangeSetup is a module written for one change: the root to load,
// a package directory under it (slash, relative to the root) with its name,
// the change itself, and the removal of everything written.
type cacheChangeSetup struct {
	root, pkgDir, pkgName string
	change                func(rapid.TB) (restore func())
	remove                func()
}

// setUpCacheChange writes the module for change.Kind and returns how to
// make the change after a first load. Kinds 0 to 3 edit what the walk of
// the root keys; the others change what the go command reads beyond it:
//
//	0 appends a function to a source file;
//	1 adds a source file to a package;
//	2 deletes a test file (none when the module has none);
//	3 sets the go line of go.mod to 1.24, 1.25 or 1.26;
//	4 deletes the file a package embeds;
//	5 renames TB in the rapid stand-in, replaced from a directory beside the root;
//	6 renames a function of a sibling package, beside the root below go.mod;
//	7 renames TB in the rapid stand-in behind a directory link inside the root;
//	8 sets GOFLAGS=-tags=uccacheprobe, which brings in a file with a type error;
//	9 sets GOARCH=386, which brings in a file with a type error.
func setUpCacheChange(t rapid.TB, module Module, change CacheChange) cacheChangeSetup {
	base, err := os.MkdirTemp("", "unicode-cache-change-")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	setup := cacheChangeSetup{root: filepath.Join(base, "m"), remove: func() { _ = os.RemoveAll(base) }}
	files := maps.Clone(module.Files)
	var sources, tests []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
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
	source := sources[change.Pick%len(sources)]
	file, err := parser.ParseFile(token.NewFileSet(), source, files[source], parser.PackageClauseOnly)
	if err != nil {
		t.Fatalf("setup failed: cannot read the package of %s: %v", source, err)
	}
	setup.pkgDir, setup.pkgName = path.Dir(source), file.Name.Name
	inPackage := func(name string) string { return path.Join(setup.pkgDir, name) }
	probe := fmt.Sprintf("\nfunc UcCacheProbe%d() int { return %d }\n", change.Value, change.Value)
	rapidUser := "package " + setup.pkgName + "\n\nimport \"pgregory.net/rapid\"\n\nvar _ rapid.TB\n"
	breakRapid := func(dir string) func(rapid.TB) func() {
		return func(t rapid.TB) func() {
			name := filepath.Join(dir, "rapid.go")
			content, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			renamed := strings.Replace(string(content), "type TB interface", "type TBGone interface", 1)
			if renamed == string(content) {
				t.Fatalf("setup failed: the rapid stand-in declares no TB:\n%s", content)
			}
			writeModuleFile(t, dir, "rapid.go", renamed)
			return func() {}
		}
	}
	setEnv := func(name, value string) func(rapid.TB) func() {
		return func(t rapid.TB) func() {
			old, had := os.LookupEnv(name)
			if err := os.Setenv(name, value); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			return func() {
				if had {
					_ = os.Setenv(name, old)
				} else {
					_ = os.Unsetenv(name)
				}
			}
		}
	}
	nothing := func() {}
	switch change.Kind {
	case 0:
		setup.change = func(t rapid.TB) func() {
			writeModuleFile(t, setup.root, source, files[source]+probe)
			return nothing
		}
	case 1:
		setup.change = func(t rapid.TB) func() {
			writeModuleFile(t, setup.root, inPackage("cache_probe.go"), "package "+setup.pkgName+"\n"+probe)
			return nothing
		}
	case 2:
		setup.change = func(t rapid.TB) func() {
			if len(tests) > 0 {
				if err := os.Remove(filepath.Join(setup.root, filepath.FromSlash(tests[change.Pick%len(tests)]))); err != nil {
					t.Fatalf("setup failed: cannot delete a test file: %v", err)
				}
			}
			return nothing
		}
	case 3:
		if !strings.Contains(files["go.mod"], "\ngo 1.26\n") {
			t.Fatalf("setup failed: go.mod has no go 1.26 line:\n%s", files["go.mod"])
		}
		setup.change = func(t rapid.TB) func() {
			writeModuleFile(t, setup.root, "go.mod", strings.Replace(files["go.mod"], "\ngo 1.26\n", fmt.Sprintf("\ngo 1.%d\n", 24+change.Value%3), 1))
			return nothing
		}
	case 4:
		files[inPackage("cache_embed.go")] = "package " + setup.pkgName + "\n\nimport _ \"embed\"\n\n//go:embed cache_embed.txt\nvar ucEmbedded string\n"
		files[inPackage("cache_embed.txt")] = "embedded\n"
		setup.change = func(t rapid.TB) func() {
			if err := os.Remove(filepath.Join(setup.root, filepath.FromSlash(inPackage("cache_embed.txt")))); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			return nothing
		}
	case 5, 7:
		outside := filepath.Join(base, "rapid")
		if change.Kind == 7 {
			outside = filepath.Join(base, "rapid-real")
		}
		for _, name := range []string{"rapid/go.mod", "rapid/rapid.go"} {
			writeModuleFile(t, outside, path.Base(name), files[name])
			delete(files, name)
		}
		files[inPackage("cache_rapid.go")] = rapidUser
		if change.Kind == 5 {
			files["go.mod"] = strings.Replace(files["go.mod"], "=> ./rapid", "=> ../rapid", 1)
		} else {
			if err := os.MkdirAll(setup.root, 0o755); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			if err := os.Symlink(filepath.Join("..", "rapid-real"), filepath.Join(setup.root, "rapid")); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
		}
		setup.change = breakRapid(outside)
	case 6:
		moduleDir := filepath.Join(base, "m")
		setup.root = filepath.Join(moduleDir, "ucsub")
		setup.pkgDir, setup.pkgName = ".", "ucsub"
		files["ucsub/ucsub.go"] = "package ucsub\n\nimport \"" + modulePath + "/ucsib\"\n\nfunc Use() int { return ucsib.Value() }\n"
		files["ucsib/ucsib.go"] = "package ucsib\n\nfunc Value() int { return 1 }\n"
		setup.change = func(t rapid.TB) func() {
			writeModuleFile(t, moduleDir, "ucsib/ucsib.go", "package ucsib\n\nfunc Other() int { return 1 }\n")
			return nothing
		}
	case 8:
		files[inPackage("cache_tagged.go")] = "//go:build uccacheprobe\n\npackage " + setup.pkgName + "\n\nvar _ = undefinedProbe\n"
		setup.change = setEnv("GOFLAGS", "-tags=uccacheprobe")
	case 9:
		files[inPackage("cache_probe_386.go")] = "package " + setup.pkgName + "\n\nvar _ = undefinedProbe\n"
		setup.change = setEnv("GOARCH", "386")
	default:
		t.Fatalf("setup failed: unknown change kind %d", change.Kind)
	}
	for name, content := range files {
		writeModuleFile(t, filepath.Join(base, "m"), name, content)
	}
	return setup
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

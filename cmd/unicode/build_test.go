package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// headRecord reads build.json in dir as {commit, dirty}, and fails unless it
// has exactly those two keys.
func headRecord(t rapid.TB, dir string) map[string]any {
	data, err := os.ReadFile(filepath.Join(dir, "build.json"))
	if err != nil {
		t.Fatalf("unicode build wrote no build.json beside graph.json in %s: %v", dir, err)
	}
	record, ok := decodeJSON(t, "build.json", data).(map[string]any)
	if _, isString := record["commit"].(string); !ok || len(record) != 2 || (record["commit"] != nil && !isString) {
		t.Fatalf("build.json is not {\"commit\": <hash or null>, \"dirty\": <bool>}: %s", data)
	}
	if _, ok := record["dirty"].(bool); !ok {
		t.Fatalf("build.json gives no boolean dirty: %s", data)
	}
	return record
}

//uc:checks cli.build-snapshot
func checkFlowBuildSnapshot(t rapid.TB, files map[string]RepoFile, committed bool, edited bool) {
	first, second := writeRepo(t, files), writeRepo(t, files)
	defer removeAll(t, filepath.Dir(first))
	defer removeAll(t, filepath.Dir(second))
	direct := buildDirectly(t, first, files)
	if len(direct.diags) > 0 {
		t.Fatalf("setup failed: the drawn repository is not legal, building it gives %+v", direct.diags)
	}
	want := direct.graph.Encode()
	head := map[string]any{"commit": nil, "dirty": true}
	if committed {
		gitCommit(t, first)
		head = map[string]any{"commit": runGit(t, first, "rev-parse", "HEAD"), "dirty": edited}
		// What an earlier build left behind is not a change of the working tree.
		writeFiles(t, first, map[string]RepoFile{".unicode/graph.json": {Content: "{}", Mode: 0o644}, ".unicode/build.json": {Content: "{}", Mode: 0o644}})
	}
	if edited {
		writeFiles(t, first, map[string]RepoFile{"notes.txt": {Content: "not committed", Mode: 0o644}})
	}

	code, stdout, stderr := runMain(t, []string{"--root", first, "--format", "json", "build"})
	if code != exitOK || stderr != "" {
		t.Fatalf("unicode build --format json exited with %d instead of %d, or wrote to stderr\nstdout: %s\nstderr: %s", code, exitOK, stdout, stderr)
	}
	snapshot := filepath.Join(first, ".unicode", "graph.json")
	var report buildReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.Diagnostics == nil || len(report.Diagnostics) > 0 || report.Snapshot != snapshot {
		t.Fatalf("stdout is not build's JSON report {\"diagnostics\": [], \"snapshot\": %q}: %v\n%s", snapshot, err, stdout)
	}
	written, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatalf("unicode build wrote no snapshot at .unicode/graph.json: %v", err)
	}
	if !bytes.Equal(written, want) {
		t.Fatalf("the snapshot unicode build wrote differs from the Encode of the graph that Load, Extract, AddFragment and Derive give\nwritten:\n%s\nEncode:\n%s", written, want)
	}
	if got := headRecord(t, filepath.Dir(snapshot)); !reflect.DeepEqual(got, head) {
		t.Fatalf("build.json records %v, yet the repository (committed: %t, changed since: %t) is at %v", got, committed, edited, head)
	}
	after := readRepo(t, first)
	maps.DeleteFunc(after, func(p string, _ RepoFile) bool { return strings.HasPrefix(p, ".git/") || (edited && p == "notes.txt") })
	_, wroteSnapshot := after[".unicode/graph.json"]
	_, wroteHead := after[".unicode/build.json"]
	if !wroteSnapshot || !wroteHead || len(after) != len(files)+2 {
		t.Fatalf("unicode build should add only .unicode/graph.json and .unicode/build.json to the repository, yet it holds %v", sortedKeys(after))
	}
	delete(after, ".unicode/graph.json")
	delete(after, ".unicode/build.json")
	if !maps.Equal(after, files) {
		t.Fatalf("unicode build changed the files of the repository")
	}

	out := filepath.Join(filepath.Dir(second), "out")
	code, stdout, stderr = runMain(t, []string{"--root", second, "build", "--out", out})
	if code != exitOK || stdout != filepath.Join(out, "graph.json")+"\n" || stderr != "" {
		t.Fatalf("unicode build --out %s should print the snapshot's path and exit with %d, yet it exited with %d\nstdout: %s\nstderr: %s", out, exitOK, code, stdout, stderr)
	}
	again, err := os.ReadFile(filepath.Join(out, "graph.json"))
	if err != nil {
		t.Fatalf("unicode build --out wrote no graph.json in its directory: %v", err)
	}
	if !bytes.Equal(again, written) {
		t.Fatalf("the same repository, written again elsewhere, builds to another snapshot\nfirst:\n%s\nagain:\n%s", written, again)
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 2 {
		t.Fatalf("unicode build --out should leave only graph.json and build.json in its directory, yet it holds %v (%v)", entries, err)
	}
	if got, outside := headRecord(t, out), (map[string]any{"commit": nil, "dirty": true}); !reflect.DeepEqual(got, outside) {
		t.Fatalf("build.json records %v for a repository outside any Git work tree, not %v", got, outside)
	}
}

func TestFlowBuildSnapshot(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFlowBuildSnapshot(t, genRepo().Draw(t, "files"), rapid.Bool().Draw(t, "committed"), rapid.Bool().Draw(t, "edited"))
	})
}

func FuzzFlowBuildSnapshot(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFlowBuildSnapshot(t, genRepo().Draw(t, "files"), rapid.Bool().Draw(t, "committed"), rapid.Bool().Draw(t, "edited"))
	}))
}

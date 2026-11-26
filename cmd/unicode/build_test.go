package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

//uc:checks cli.build-snapshot
func checkFlowBuildSnapshot(t rapid.TB, files map[string]RepoFile) {
	first, second := writeRepo(t, files), writeRepo(t, files)
	defer removeAll(t, filepath.Dir(first))
	defer removeAll(t, filepath.Dir(second))
	direct := buildDirectly(t, first, files)
	if len(direct.diags) > 0 {
		t.Fatalf("setup failed: the drawn repository is not legal, building it gives %+v", direct.diags)
	}
	want := direct.graph.Encode()

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
	after := readRepo(t, first)
	if _, ok := after[".unicode/graph.json"]; !ok || len(after) != len(files)+1 {
		t.Fatalf("unicode build should add only .unicode/graph.json to the repository, yet it holds %v", sortedKeys(after))
	}
	delete(after, ".unicode/graph.json")
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
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 1 {
		t.Fatalf("unicode build --out should leave only graph.json in its directory, yet it holds %v (%v)", entries, err)
	}
}

func TestFlowBuildSnapshot(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFlowBuildSnapshot(t, genRepo().Draw(t, "files"))
	})
}

func FuzzFlowBuildSnapshot(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFlowBuildSnapshot(t, genRepo().Draw(t, "files"))
	}))
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/serve"
)

// StoreWrite is a viewer opened on a repository's snapshot, a request, and
// its answers before and after a record was written to a store; unicode
// build has run again since.
//
//uc:realizes cli.StoreWrite
type StoreWrite struct {
	Server  *serve.Server
	Request serve.Request
	Before  serve.Response
	Written serve.Response
}

// storeRepos are the repositories genStoreWrite wrote. They outlive the
// generator, since the predicate answers from them, so TestMain removes them.
var storeRepos struct {
	sync.Mutex
	dirs []string
}

// TestMain removes the repositories genStoreWrite wrote, after the tests.
func TestMain(m *testing.M) {
	code := m.Run()
	for _, dir := range storeRepos.dirs {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

// genStoreWrite builds a small repository whose build gives no diagnostic,
// opens a viewer on its snapshot and answers one request before and after
// writing a record about the requested node: about half the time an
// approved review of a component that unicode review record writes, asked
// for by /api/node or /api/entity; otherwise a run report for the
// component's property keeps that evidence.Save writes, asked for by
// /api/node. Then it runs unicode build again.
//
//uc:generates cli.stores-reach-viewer-through-build.change
func genStoreWrite() *rapid.Generator[StoreWrite] {
	return rapid.Custom(func(t *rapid.T) StoreWrite {
		model := drawRepo(t)
		root := writeRepo(t, repoFiles(model.texts(t)))
		storeRepos.Lock()
		storeRepos.dirs = append(storeRepos.dirs, filepath.Dir(root))
		storeRepos.Unlock()
		run := func(args ...string) {
			if code, stdout, stderr := runMain(t, append([]string{"--root", root}, args...)); code != exitOK {
				t.Fatalf("setup failed: unicode %v exited with %d\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
			}
		}
		run("build")
		s, err := serve.Open(root, filepath.Join(root, ".unicode", "graph.json"), commitGraphs{})
		if err != nil {
			t.Fatalf("setup failed: serve.Open refused the snapshot that unicode build wrote: %v", err)
		}
		component := rapid.SampledFrom(model.components).Draw(t, "component").name
		reviewed := rapid.Bool().Draw(t, "review?")
		request := serve.Request{Path: "/api/node", Query: map[string][]string{"id": {component}}}
		if reviewed && rapid.Bool().Draw(t, "entity?") {
			request.Path = "/api/entity"
		}
		if !reviewed {
			request.Query["id"] = []string{component + ".keeps"}
		}
		before := s.Answer(request)
		if reviewed {
			run("review", "record", component, "--reviewer", "reviewer", "--decision", "approved")
		} else {
			report := lang.RunReport{Version: 1, Snapshot: strings.Repeat("ab", 32), At: "2026-11-27T10:00:00Z", Mode: "replay",
				Results: []lang.RunResult{{Id: component + ".keeps", Passed: true, Accepted: 1}}, Diagnostics: []diag.Diagnostic{}}
			if problems := evidence.Save(root, report); len(problems) > 0 {
				t.Fatalf("setup failed: evidence.Save refused a complete run report: %v", problems)
			}
		}
		written := s.Answer(request)
		run("build")
		return StoreWrite{Server: s, Request: request, Before: before, Written: written}
	})
}

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2/maybe"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/repo"
)

// runtimeDigests gives each runtime run record built from a clean commit the
// execution-input digest of that commit (docs/design.md §6.6): a monitored
// program has no graph to compute it, so its record names only the commit.
// The working tree's own digest serves its clean HEAD; any other commit's
// graph is built as a query's --rev builds it, once, and its digest kept in
// .unicode/cache/runtime/<commit>. A record of a dirty or unknown build, or of
// a commit that cannot be built, keeps no digest and stays history.
func runtimeDigests(root string, g *graph.Graph, reports []lang.RunReport) {
	digests := map[string]string{}
	for i := range reports {
		report := &reports[i]
		if report.Venue != lang.VenueRuntime || report.Commit == "" || report.Dirty || report.Snapshot != "" {
			continue
		}
		if _, known := digests[report.Commit]; !known {
			digests[report.Commit] = commitDigest(root, g, report.Commit)
		}
		report.Snapshot = digests[report.Commit]
	}
}

// commitDigest is the execution-input digest of the graph at commit, or ""
// when it cannot be built.
func commitDigest(root string, g *graph.Graph, commit string) string {
	if head, err := repo.ReadHead(root, filepath.Join(root, ".unicode")); err == nil && !head.Dirty && head.Commit != nil && *head.Commit == commit {
		return graph.InputDigest(g)
	}
	cache := filepath.Join(root, ".unicode", "cache", "runtime", commit)
	if data, err := os.ReadFile(cache); err == nil && len(strings.TrimSpace(string(data))) == 64 {
		return strings.TrimSpace(string(data))
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	built, _, err := revisionGraph(root, commit)
	if err != nil {
		return ""
	}
	digest := graph.InputDigest(&built)
	if os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
		_ = maybe.WriteFile(cache, []byte(digest+"\n"), 0o644)
	}
	return digest
}

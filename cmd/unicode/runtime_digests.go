package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"sync"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/lang/golang"
	"github.com/lincyaw/unicode/internal/repo"
)

// runtimeDigests gives, for unicode build, each runtime run record built from
// a clean commit the execution-input digest of that commit (docs/design.md
// §6.6): a monitored program has no graph to compute it, so its record names
// only the commit. The working tree's own digest serves its clean HEAD; any
// other commit's graph is built as a query's --rev builds it, once for each
// distinct commit and several at a time, through the content cache. A record
// of a dirty or unknown build keeps no digest and stays history; a commit
// that cannot be built leaves its records history too, and is reported.
func runtimeDigests(root string, g *graph.Graph, reports []lang.RunReport) []diag.Diagnostic {
	var commits []string
	for _, report := range reports {
		if report.Venue == lang.VenueRuntime && report.Commit != "" && !report.Dirty && report.Snapshot == "" && !slices.Contains(commits, report.Commit) {
			commits = append(commits, report.Commit)
		}
	}
	if len(commits) == 0 {
		return nil
	}
	head, headErr := repo.ReadHead(root, filepath.Join(root, ".unicode"))
	digests := make([]string, len(commits))
	problems := make([]error, len(commits))
	slots := make(chan struct{}, max(1, runtime.GOMAXPROCS(0)/2))
	var wait sync.WaitGroup
	for i, commit := range commits {
		if headErr == nil && !head.Dirty && head.Commit != nil && *head.Commit == commit {
			digests[i] = graph.InputDigest(g)
			continue
		}
		wait.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			digests[i], problems[i] = golang.CommitDigest(commit, func() (string, error) {
				built, _, err := revisionGraph(root, commit)
				if err != nil {
					return "", err
				}
				return graph.InputDigest(&built), nil
			})
		})
	}
	wait.Wait()
	var out []diag.Diagnostic
	for i, commit := range commits {
		if problems[i] != nil {
			out = append(out, diag.Diagnostic{Code: "cli.unbuildable-runtime-commit", Subject: commit, Position: diag.Position{Path: ".unicode/runs", Line: 1, Column: 1},
				Message: fmt.Sprintf("the commit %s of runtime run records builds no graph: %v; its records stay history", commit, problems[i]),
				Fix:     "fetch the commit the monitored program was built from, or remove its records from .unicode/runs"})
		}
	}
	for i := range reports {
		if index := slices.Index(commits, reports[i].Commit); reports[i].Venue == lang.VenueRuntime && index >= 0 && !reports[i].Dirty && problems[index] == nil {
			reports[i].Snapshot = digests[index]
		}
	}
	return out
}

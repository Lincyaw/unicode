package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"pgregory.net/rapid"
)

// fixtureTemp is the process's temporary directory, taken before any case
// points TMPDIR at a directory of its own.
var fixtureTemp = os.TempDir()

// fixtureRepository is a git repository holding only a system file,
// committed once, made once per test process.
var fixtureRepository = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp(fixtureTemp, "commit-graphs-")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "design"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "design", "system.uc"), []byte("system fixture \"只有系统文件的仓库\"\n"), 0o644); err != nil {
		return "", err
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-q", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.test",
			"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.test", "GIT_CONFIG_GLOBAL=/dev/null")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", errors.Join(err, errors.New(string(output)))
		}
	}
	return dir, nil
})

// genCommitGraphsRoot draws, about half the time, the fixture repository,
// and otherwise a relative path of absent- and one to twelve lowercase
// letters, which the test's working directory does not hold.
//
//uc:generates cli.commit-graphs-have-one-system.root
func genCommitGraphsRoot() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if rapid.Bool().Draw(t, "present") {
			dir, err := fixtureRepository()
			if err != nil {
				t.Fatalf("the fixture repository cannot be made: %v", err)
			}
			return dir
		}
		return "absent-" + rapid.StringMatching(`[a-z]{1,12}`).Draw(t, "name")
	})
}

// genCommitGraphsCommit draws HEAD about half the time, and otherwise a name
// of missing- and one to twelve lowercase letters, which names no commit.
//
//uc:generates cli.commit-graphs-have-one-system.commit
func genCommitGraphsCommit() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if rapid.Bool().Draw(t, "head") {
			return "HEAD"
		}
		return "missing-" + rapid.StringMatching(`[a-z]{1,12}`).Draw(t, "name")
	})
}

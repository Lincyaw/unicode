package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// replaceFiles leaves the working tree at root holding exactly the files,
// beside its .git.
func replaceFiles(t rapid.TB, root string, files map[string]RepoFile) {
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("setup failed: cannot list %s: %v", root, err)
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
			t.Fatalf("setup failed: cannot remove %s from the working tree: %v", e.Name(), err)
		}
	}
	writeFiles(t, root, files)
}

// endMismatch returns why the end of a comparison that diff wrote is not the
// commit, or "": its hash, abbreviated hash, branch, subject and UTC time,
// and for the head whether the working tree has changes.
func endMismatch(got any, name, commit, branch, subject string, dirty *bool) string {
	end, ok := got.(map[string]any)
	if !ok {
		return fmt.Sprintf("%s is not an object: %v", name, got)
	}
	short, _ := end["short"].(string)
	at, _ := end["time"].(string)
	parsed, err := time.Parse(time.RFC3339, at)
	want := map[string]any{"commit": commit, "short": short, "subject": subject, "time": at}
	if branch != "" {
		want["branch"] = branch
	}
	if dirty != nil {
		want["dirty"] = *dirty
	}
	switch {
	case short == "" || !strings.HasPrefix(commit, short):
		return fmt.Sprintf("%s gives the abbreviated hash %q of the commit %s", name, short, commit)
	case err != nil || parsed.Location() != time.UTC:
		return fmt.Sprintf("%s gives the time %q, which is not an RFC 3339 time in UTC", name, at)
	case !reflect.DeepEqual(end, want):
		return fmt.Sprintf("%s is %v, yet the commit is %v", name, end, want)
	}
	return ""
}

//uc:checks cli.diff-snapshots
func checkFlowDiffSnapshots(t rapid.TB, base map[string]RepoFile, head map[string]RepoFile, toCommit bool, jsonOutput bool, readable bool) {
	root := writeRepo(t, base)
	defer removeAll(t, filepath.Dir(root))
	var baseGraph, headGraph graph.Graph
	if readable {
		direct := buildDirectly(t, root, base)
		if len(direct.diags) > 0 {
			t.Fatalf("setup failed: the drawn base repository is not legal, building it gives %+v", direct.diags)
		}
		baseGraph = direct.graph
	}
	gitCommit(t, root)
	first := runGit(t, root, "rev-parse", "HEAD")
	replaceFiles(t, root, head)
	if readable {
		direct := buildDirectly(t, root, head)
		if len(direct.diags) > 0 {
			t.Fatalf("setup failed: the drawn head repository is not legal, building it gives %+v", direct.diags)
		}
		headGraph = direct.graph
	}
	command := []string{"diff", first}
	second := ""
	if toCommit {
		commitAll(t, root, "the second repository")
		second = runGit(t, root, "rev-parse", "HEAD")
		// The working tree no longer holds a design, so only the second commit's graph answers.
		if err := os.RemoveAll(filepath.Join(root, "design")); err != nil {
			t.Fatalf("setup failed: cannot remove design/ from the working tree: %v", err)
		}
		command = append(command, "--to", second)
	}
	if !readable {
		command[len(command)-1] = "no-such-commit"
	}
	args := append([]string{"--root", root}, command...)
	if jsonOutput {
		args = append(args, "--format", "json")
	}
	code, stdout, stderr := runMain(t, args)
	if !readable {
		if code != exitUsage || stdout != "" || stderr == "" {
			t.Fatalf("unicode %s names a revision that is no commit, so it should exit with %d and say so on stderr, yet it exited with %d\nstdout: %s\nstderr: %s",
				strings.Join(command, " "), exitUsage, code, stdout, stderr)
		}
		return
	}
	if code != exitOK || stderr != "" {
		t.Fatalf("unicode %s exited with %d instead of %d, or wrote to stderr\nstdout: %s\nstderr: %s", strings.Join(command, " "), code, exitOK, stdout, stderr)
	}
	want := graph.Compare(&baseGraph, &headGraph)
	if !jsonOutput {
		short := runGit(t, root, "rev-parse", "--short", first)
		if !strings.HasPrefix(stdout, "base "+short+" the drawn repository\n") {
			t.Fatalf("the text of unicode %s does not start with the base commit %s\n%s", strings.Join(command, " "), short, stdout)
		}
		for _, c := range want.Nodes {
			if !strings.Contains(stdout, string(c.Change)+" "+c.Id+" ") {
				t.Fatalf("the text of unicode %s leaves out the node %s, which is %s\n%s", strings.Join(command, " "), c.Id, c.Change, stdout)
			}
		}
		for _, c := range want.Edges {
			if line := fmt.Sprintf("%s %s %s -> %s\n", c.Change, c.Kind, c.From, c.To); !strings.Contains(stdout, line) {
				t.Fatalf("the text of unicode %s leaves out the line %q\n%s", strings.Join(command, " "), line, stdout)
			}
		}
		return
	}
	got, ok := decodeJSON(t, "the output of unicode "+strings.Join(command, " "), []byte(stdout)).(map[string]any)
	if !ok {
		t.Fatalf("the output of unicode %s is not a JSON object\n%s", strings.Join(command, " "), stdout)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("cannot encode the result of calling graph.Compare directly: %v", err)
	}
	direct := decodeJSON(t, "the direct comparison", encoded).(map[string]any)
	for _, key := range []string{"nodes", "edges"} {
		if _, isList := got[key].([]any); !isList || !reflect.DeepEqual(got[key], direct[key]) {
			t.Fatalf("the %s of unicode %s differ from those of calling graph.Compare on the two graphs directly\ngot:\n%s\nwant:\n%s", key, strings.Join(command, " "), stdout, encoded)
		}
	}
	branch := runGit(t, root, "symbolic-ref", "--short", "HEAD")
	headCommit, headSubject, dirty := first, "the drawn repository", !maps.Equal(base, head)
	baseBranch := branch
	if toCommit {
		headCommit, headSubject, dirty, baseBranch = second, "the second repository", false, ""
	}
	if why := endMismatch(got["base"], "base", first, baseBranch, "the drawn repository", nil); why != "" {
		t.Fatalf("unicode %s: %s", strings.Join(command, " "), why)
	}
	if why := endMismatch(got["head"], "head", headCommit, branch, headSubject, &dirty); why != "" {
		t.Fatalf("unicode %s: %s", strings.Join(command, " "), why)
	}
	if len(got) != 4 {
		t.Fatalf("the output of unicode %s holds the keys %v, not only base, head, nodes and edges", strings.Join(command, " "), sortedKeys(got))
	}
}

func TestFlowDiffSnapshots(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkFlowDiffSnapshots(t, genRepo().Draw(t, "base"), genRepo().Draw(t, "head"),
			rapid.Bool().Draw(t, "toCommit"), rapid.Bool().Draw(t, "jsonOutput"), rapid.Bool().Draw(t, "readable"))
	})
}

func FuzzFlowDiffSnapshots(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkFlowDiffSnapshots(t, genRepo().Draw(t, "base"), genRepo().Draw(t, "head"),
			rapid.Bool().Draw(t, "toCommit"), rapid.Bool().Draw(t, "jsonOutput"), rapid.Bool().Draw(t, "readable"))
	}))
}

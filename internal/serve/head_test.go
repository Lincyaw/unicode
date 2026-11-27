package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/repo"
)

// genRecord draws no record, a record without a commit, or one with a
// random commit hash, which git cannot read.
//
//uc:generates serve.snapshot-head-follows-record.record
func genRecord() *rapid.Generator[*repo.Head] {
	return rapid.Custom(func(t *rapid.T) *repo.Head {
		switch rapid.IntRange(0, 2).Draw(t, "record") {
		case 0:
			return nil
		case 1:
			return &repo.Head{Dirty: true}
		}
		commit := rapid.StringMatching(`[0-9a-f]{40}`).Draw(t, "commit")
		return &repo.Head{Commit: &commit, Dirty: rapid.Bool().Draw(t, "dirty")}
	})
}

// headOf opens a server on the repository at root and reads its snapshot's head.
func headOf(t rapid.TB, root string, x Exchange) WorkingTree {
	s, err := Open(root, filepath.Join(root, ".unicode", "graph.json"), baseBuilder{graphOf(t, baseOf(x.Fragment))})
	if err != nil {
		t.Fatalf("Open refused the snapshot that Encode wrote, beside a well-formed record or none: %v", err)
	}
	return snapshotOf(t, s, "the snapshot").Head
}

func writeRecord(t rapid.TB, root string, record repo.Head) {
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("setup failed: cannot encode the record: %v", err)
	}
	writeFile(t, filepath.Join(root, ".unicode", repo.RecordFile), data)
}

//uc:checks serve.snapshot-head-follows-record
func checkSnapshotHeadFollowsRecord(t rapid.TB, exchange Exchange, record *repo.Head) {
	root, _ := written(t, exchange)
	defer removeAll(t, root)
	committed := slices.ContainsFunc(exchange.Requests, readsRevision)
	// What git says of the working tree: its commit, unchanged but for the snapshot, or nothing.
	var atHead WorkingTree
	if committed {
		commit := runGit(t, root, "rev-parse", "HEAD")
		at, err := time.Parse(time.RFC3339, runGit(t, root, "show", "-s", "--format=%cI", "HEAD"))
		if err != nil {
			t.Fatalf("setup failed: git gives no commit time: %v", err)
		}
		branch := runGit(t, root, "symbolic-ref", "--short", "HEAD")
		atHead = WorkingTree{Commit: commit, Short: runGit(t, root, "rev-parse", "--short", "HEAD"), Branch: &branch,
			Subject: "the drawn repository", Time: at.UTC()}
	}
	want := atHead
	if record != nil {
		writeRecord(t, root, *record)
		want = WorkingTree{Dirty: record.Dirty}
		if record.Commit != nil {
			want.Commit = *record.Commit
		}
	}
	if got := headOf(t, root, exchange); !reflect.DeepEqual(got, want) {
		t.Fatalf("/api/snapshot gives the head %+v, yet with the record %+v beside the snapshot (the repository committed: %t) it is %+v", got, record, committed, want)
	}
	if committed {
		dirty := record == nil || !record.Dirty
		writeRecord(t, root, repo.Head{Commit: &atHead.Commit, Dirty: dirty})
		want = atHead
		want.Dirty = dirty
		if got := headOf(t, root, exchange); !reflect.DeepEqual(got, want) {
			t.Fatalf("with a record of the commit checked out, /api/snapshot gives the head %+v, not %+v as git gives the commit", got, want)
		}
	}
}

// genMalformedRecord draws the text of a build record that is not
// {"commit": <hash or null>, "dirty": <bool>}: text that is not JSON, a JSON
// value that is not an object, an object without commit or dirty, one with a
// third field, or one whose commit or dirty has another type.
//
//uc:generates serve.open-refuses-malformed-record.record
func genMalformedRecord() *rapid.Generator[string] {
	commit := rapid.SampledFrom([]string{`null`, `"0123456789abcdef0123456789abcdef01234567"`})
	dirty := rapid.SampledFrom([]string{`true`, `false`})
	object := func(fields ...string) string { return "{" + strings.Join(fields, ", ") + "}" }
	return rapid.OneOf(
		rapid.Map(rapid.String(), func(s string) string { return "x" + s }),
		rapid.SampledFrom([]string{`[]`, `7`, `"commit"`, `null`, `true`}),
		rapid.Map(commit, func(c string) string { return object(`"commit": ` + c) }),
		rapid.Map(dirty, func(d string) string { return object(`"dirty": ` + d) }),
		rapid.Custom(func(t *rapid.T) string {
			return object(`"commit": `+commit.Draw(t, "commit"), `"dirty": `+dirty.Draw(t, "dirty"), `"branch": "main"`)
		}),
		rapid.Custom(func(t *rapid.T) string {
			return object(`"commit": `+rapid.SampledFrom([]string{`7`, `true`, `[]`, `{}`}).Draw(t, "commit"), `"dirty": `+dirty.Draw(t, "dirty"))
		}),
		rapid.Custom(func(t *rapid.T) string {
			return object(`"commit": `+commit.Draw(t, "commit"), `"dirty": `+rapid.SampledFrom([]string{`"yes"`, `1`, `[]`}).Draw(t, "dirty"))
		}),
	)
}

//uc:checks serve.open-refuses-malformed-record
func checkOpenRefusesMalformedRecord(t rapid.TB, exchange Exchange, record string) {
	root, _ := written(t, exchange)
	defer removeAll(t, root)
	writeFile(t, filepath.Join(root, ".unicode", repo.RecordFile), []byte(record))
	if _, err := Open(root, filepath.Join(root, ".unicode", "graph.json"), baseBuilder{}); !errors.As(err, new(Unreadable)) {
		t.Fatalf("beside the record %q, which is not {commit, dirty}, Open should refuse the snapshot as unreadable, yet it gave %v", record, err)
	}
}

// snapshotOf reads the answer of /api/snapshot.
func snapshotOf(t rapid.TB, s *Server, when string) Snapshot {
	resp := s.Answer(Request{Path: "/api/snapshot"})
	if resp.Status != http.StatusOK {
		t.Fatalf("%s: /api/snapshot got %d\n%s", when, resp.Status, resp.Body)
	}
	var snap Snapshot
	if err := json.Unmarshal(resp.Body, &snap); err != nil {
		t.Fatalf("%s: /api/snapshot does not decode: %v\n%s", when, err, resp.Body)
	}
	return snap
}

package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/repo"
)

// WriteSample is a graph with an optional base, the requests written to the
// store first, the request written after them, whether that request repeats
// the last earlier request about its target, and the code it should get.
//
//uc:realizes review.WriteSample
type WriteSample struct {
	Graph   *graph.Graph
	Base    *graph.Graph
	History []Request
	Request Request
	Repeat  bool
	Code    string
}

// LatestSample is a graph, no base, records about some of its targets, and
// the latest record of each of those targets.
//
//uc:realizes review.LatestSample
type LatestSample struct {
	Graph   *graph.Graph
	Base    *graph.Graph
	Records []Record
	Latest  []Record
}

// requestTargets are the targets a request may name: the current candidates
// of the sample's graph and the targets its comparison removes.
func requestTargets(s Sample) []Target {
	targets := sortedOracleTargets(oracleTargets(s.Graph))
	for _, target := range s.Targets {
		if target.Removed {
			targets = append(targets, target)
		}
	}
	return targets
}

func drawRequest(t *rapid.T, targets []Target) Request {
	commit := ""
	if rapid.Bool().Draw(t, "committed") {
		commit = rapid.StringOfN(rapid.RuneFrom([]rune("0123456789abcdef")), 40, 40, -1).Draw(t, "commit")
	}
	return Request{
		Target:   rapid.SampledFrom(targets).Draw(t, "target"),
		Reviewer: rapid.SampledFrom([]string{"reviewer", "张三", "review-bot"}).Draw(t, "reviewer"),
		Decision: rapid.SampledFrom([]string{"approved", "changes-requested"}).Draw(t, "decision"),
		Comment:  rapid.SampledFrom([]string{"", "looks right", "字段名与代码一致 ✅"}).Draw(t, "comment"),
		Commit:   commit,
		Dirty:    rapid.Bool().Draw(t, "dirty"),
	}
}

// drawWriteSample draws a sample's graph and base, earlier requests and a
// request; repeat makes the request the last earlier one.
func drawWriteSample(t *rapid.T, repeat bool) WriteSample {
	s := genSample().Draw(t, "sample")
	targets := requestTargets(s)
	out := WriteSample{Graph: s.Graph, Base: s.Base}
	least := 0
	if repeat {
		least = 1
	}
	for range rapid.IntRange(least, 4).Draw(t, "history") {
		out.History = append(out.History, drawRequest(t, targets))
	}
	if repeat {
		out.Request = out.History[len(out.History)-1]
	} else {
		out.Request = drawRequest(t, targets)
	}
	for _, earlier := range slices.Backward(out.History) {
		if earlier.Target == out.Request.Target {
			out.Repeat = earlier == out.Request
			break
		}
	}
	return out
}

//uc:generates review.write-records-the-request.sample
//uc:generates review.write-stores-the-record.sample
//uc:generates review.write-keeps-history.sample
//uc:generates review.write-adds-a-new-record.sample
//uc:generates review.read-is-repeatable.sample
func genWriteSample() *rapid.Generator[WriteSample] {
	return rapid.Custom(func(t *rapid.T) WriteSample { return drawWriteSample(t, false) })
}

//uc:generates review.write-is-idempotent.sample
//uc:generates review.read-rejects-corrupt-records.sample
//uc:generates review.read-rejects-symlinks.sample
//uc:generates review.write-rejects-symlinks.sample
//uc:generates review.read-diagnostics-are-described.sample
func genRepeatSample() *rapid.Generator[WriteSample] {
	return rapid.Custom(func(t *rapid.T) WriteSample { return drawWriteSample(t, true) })
}

// genInvalidSample breaks the request in one of six ways, with the code each
// way should get.
//
//uc:generates review.write-rejects-invalid-requests.sample
//uc:generates review.write-diagnostics-are-described.sample
func genInvalidSample() *rapid.Generator[WriteSample] {
	return rapid.Custom(func(t *rapid.T) WriteSample {
		sample := drawWriteSample(t, false)
		sample.Repeat = false
		switch rapid.IntRange(0, 5).Draw(t, "invalid") {
		case 0:
			sample.Request.Reviewer, sample.Code = " ", "review.invalid-reviewer"
		case 1:
			sample.Request.Decision, sample.Code = "unknown", "review.invalid-decision"
		case 2:
			sample.Request.Target, sample.Code = Target{Node: "unknown"}, "review.unknown-target"
		case 3:
			sample.Request.Target, sample.Code = Target{Node: "domain", EdgeKind: "calls", From: "domain", To: "domain"}, "review.invalid-target"
		case 4:
			sample.Request.ExpectedDigest, sample.Code = "xyz", "review.invalid-record"
		default:
			sample.Request.ExpectedDigest, sample.Code = strings.Repeat("0", 64), "review.stale-target"
		}
		return sample
	})
}

// writeHistory writes the sample's earlier requests into the store at dir.
func writeHistory(dir string, sample WriteSample) error {
	for _, request := range sample.History {
		if written := Write(dir, sample.Graph, sample.Base, request); written.Record == nil || len(written.Diagnostics) > 0 {
			return fmt.Errorf("a valid earlier request was refused: %v", written.Diagnostics)
		}
	}
	return nil
}

//uc:generates review.write-records-the-request.dir
//uc:generates review.write-stores-the-record.dir
//uc:generates review.write-keeps-history.dir
//uc:generates review.write-adds-a-new-record.dir
//uc:generates review.write-is-idempotent.dir
//uc:generates review.read-is-repeatable.dir
//uc:generates review.write-rejects-invalid-requests.dir
//uc:generates review.write-diagnostics-are-described.dir
func deriveHistoryDir(sample WriteSample) (string, error) {
	dir, err := repo.Materialize(repo.Tree{})
	if err != nil {
		return "", err
	}
	return dir, writeHistory(dir, sample)
}

// firstRecord is the name of the record file whose name sorts first.
func firstRecord(reviews string) (string, error) {
	entries, err := os.ReadDir(reviews)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("the store at %s holds no record", reviews)
	}
	return entries[0].Name(), nil
}

// deriveDamagedDir writes the earlier requests, then adds a record of
// version 42 (damage 0), adds a field to the first record (1), or adds a
// repeated comment key at its start (2).
//
//uc:generates review.read-rejects-corrupt-records.dir
//uc:generates review.read-diagnostics-are-described.dir
func deriveDamagedDir(sample WriteSample, damage int) (string, error) {
	dir, err := deriveHistoryDir(sample)
	if err != nil {
		return "", err
	}
	reviews := filepath.Join(dir, ".unicode", "reviews")
	if damage == 0 {
		if err := os.MkdirAll(reviews, 0o755); err != nil {
			return "", err
		}
		return dir, os.WriteFile(filepath.Join(reviews, strings.Repeat("0", 64)+".json"), []byte(`{"version":42}`), 0o644)
	}
	name, err := firstRecord(reviews)
	if err != nil {
		return "", err
	}
	path := filepath.Join(reviews, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if damage == 1 {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			return "", err
		}
		object["unexpected"] = json.RawMessage("true")
		if data, err = json.Marshal(object); err != nil {
			return "", err
		}
	} else {
		data = append([]byte(`{"comment":"ignored",`), data[1:]...)
	}
	return dir, os.WriteFile(path, data, 0o644)
}

// deriveLinkedDir writes the earlier requests into dir/store, then links a
// record file to the first record, or makes dir/repo/.unicode a link to the
// store's.
//
//uc:generates review.read-rejects-symlinks.dir
//uc:generates review.write-rejects-symlinks.dir
func deriveLinkedDir(sample WriteSample, linkRecord bool) (string, error) {
	dir, err := repo.Materialize(repo.Tree{})
	if err != nil {
		return "", err
	}
	store := filepath.Join(dir, "store")
	if err := os.Mkdir(store, 0o755); err != nil {
		return "", err
	}
	if err := writeHistory(store, sample); err != nil {
		return "", err
	}
	if linkRecord {
		reviews := filepath.Join(store, ".unicode", "reviews")
		name, err := firstRecord(reviews)
		if err != nil {
			return "", err
		}
		return dir, os.Symlink(name, filepath.Join(reviews, strings.Repeat("1", 64)+".json"))
	}
	if err := os.Mkdir(filepath.Join(dir, "repo"), 0o755); err != nil {
		return "", err
	}
	return dir, os.Symlink(filepath.Join("..", "store", ".unicode"), filepath.Join(dir, "repo", ".unicode"))
}

//uc:generates review.read-rejects-symlinks.root
//uc:generates review.write-rejects-symlinks.root
func deriveLinkedRoot(dir string, linkRecord bool) string {
	if linkRecord {
		return filepath.Join(dir, "store")
	}
	return filepath.Join(dir, "repo")
}

// genEmptyStoreTree draws, with even chances, an empty tree, a tree with only
// .unicode, or a tree with only design/system.uc.
//
//uc:generates review.read-is-empty-without-store.tree
func genEmptyStoreTree() *rapid.Generator[repo.Tree] {
	return rapid.Custom(func(t *rapid.T) repo.Tree {
		dir := repo.Entry{Kind: repo.EntryKindDir, Mode: 0o755}
		switch rapid.IntRange(0, 2).Draw(t, "store") {
		case 0:
			return repo.Tree{Entries: map[string]repo.Entry{}}
		case 1:
			return repo.Tree{Entries: map[string]repo.Entry{".unicode": dir}}
		}
		return repo.Tree{Entries: map[string]repo.Entry{"design": dir, "design/system.uc": {Kind: repo.EntryKindFile, Content: []byte("system s\n"), Mode: 0o644}}}
	})
}

//uc:generates review.read-is-empty-without-store.dir
func deriveTreeDir(tree repo.Tree) (string, error) { return repo.Materialize(tree) }

// The observation adapters read the store or the tree under dir, each for
// one named observation (docs/model.md §10 rule 15).

//uc:observes review.write-records-the-request.stored
func observeRecordedStore(dir string) Loaded { return Read(dir) }

//uc:observes review.write-stores-the-record.stored
func observeStoredStore(dir string) Loaded { return Read(dir) }

//uc:observes review.write-adds-a-new-record.stored
func observeAddedStore(dir string) Loaded { return Read(dir) }

//uc:observes review.write-is-idempotent.stored
func observeRepeatedStore(dir string) Loaded { return Read(dir) }

//uc:observes review.read-is-repeatable.stored
func observeReadStore(dir string) Loaded { return Read(dir) }

//uc:observes review.write-keeps-history.files
func observeHistoryTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes review.write-is-idempotent.files
func observeRepeatedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes review.read-is-repeatable.files
func observeReadTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes review.write-rejects-invalid-requests.files
func observeRejectedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes review.read-rejects-corrupt-records.files
func observeDamagedTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes review.read-rejects-symlinks.files
func observeLinkedReadTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

//uc:observes review.write-rejects-symlinks.files
func observeLinkedWriteTree(dir string) (repo.Tree, error) { return repo.ReadTree(dir) }

// genLatestSample draws records about one to three current targets of the
// sample graph, at one of three instants of 2026-11-27 each, and the latest
// record of each target: the latest instant, the greatest identity among
// records of one instant.
//
//uc:generates review.evaluation-picks-latest.sample
func genLatestSample() *rapid.Generator[LatestSample] {
	return rapid.Custom(func(t *rapid.T) LatestSample {
		g := sampleGraph(t, rapid.IntRange(-20, 20).Draw(t, "value"))
		targets := rapid.SliceOfNDistinct(rapid.SampledFrom(sortedOracleTargets(oracleTargets(g))), 1, 3, targetKey).Draw(t, "targets")
		instants := []string{"2026-11-27T10:00:00Z", "2026-11-27T10:00:00.5Z", "2026-11-27T10:00:01Z"}
		out := LatestSample{Graph: g, Records: []Record{}, Latest: []Record{}}
		for _, target := range targets {
			var latest Record
			for i := range rapid.IntRange(1, 4).Draw(t, "records") {
				r := Record{Version: 1, Reviewer: rapid.SampledFrom([]string{"reviewer", "张三", "review-bot"}).Draw(t, "reviewer"),
					At: rapid.SampledFrom(instants).Draw(t, "at"), Snapshot: graph.InputDigest(g), Target: target,
					Digest: indexContent(g).item(target, nil).Digest, Decision: rapid.SampledFrom([]string{"approved", "changes-requested"}).Draw(t, "decision"),
					Comment: rapid.SampledFrom([]string{"", "looks right", "字段名与代码一致 ✅"}).Draw(t, "comment")}
				r.Id = recordId(r)
				out.Records = append(out.Records, r)
				at, _ := time.Parse(time.RFC3339Nano, r.At)
				was, _ := time.Parse(time.RFC3339Nano, latest.At)
				if i == 0 || at.After(was) || at.Equal(was) && r.Id > latest.Id {
					latest = r
				}
			}
			out.Latest = append(out.Latest, latest)
		}
		return out
	})
}

//uc:checks review.concurrent-writes-keep-both
func checkConcurrentWritesKeepBoth(t rapid.TB, sample Sample) {
	root, err := os.MkdirTemp("", "unicode-review-parallel-")
	if err != nil {
		t.Fatalf("no temporary directory for the store: %v", err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	replies := make(chan Written, 2)
	for _, comment := range []string{"parallel first", "parallel second"} {
		go func() {
			replies <- Write(root, sample.Graph, sample.Base, Request{Target: sample.Targets[0], Reviewer: "reviewer", Decision: "approved", Comment: comment})
		}()
	}
	var written []Record
	for range 2 {
		reply := <-replies
		if reply.Record == nil || len(reply.Diagnostics) != 0 {
			t.Fatalf("a write that ran beside another returned no record: %+v", reply)
		}
		written = append(written, *reply.Record)
	}
	loaded := Read(root)
	if len(loaded.Diagnostics) != 0 || len(loaded.Records) != 2 || !slices.Contains(loaded.Records, written[0]) || !slices.Contains(loaded.Records, written[1]) {
		t.Fatalf("two writes that ran side by side did not both keep their records: wrote %+v, read %+v", written, loaded)
	}
}

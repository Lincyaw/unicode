package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

//uc:checks review.records-preserve-history
func checkRecordsPreserveHistory(t rapid.TB, sample Sample) {
	root, err := os.MkdirTemp("", "unicode-review-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	if empty := Read(root); len(empty.Records) != 0 || len(empty.Diagnostics) != 0 {
		t.Fatalf("missing store is not empty: %+v", empty)
	}
	if len(sample.Targets) == 0 {
		return
	}
	target := sample.Targets[0]
	request := Request{Target: target, Reviewer: "reviewer", Decision: "approved", Comment: "first review"}
	written := Write(root, sample.Graph, sample.Base, request)
	if written.Record == nil || len(written.Diagnostics) > 0 {
		t.Fatalf("cannot write a legal review: %+v", written)
	}
	first := *written.Record
	path := filepath.Join(root, recordPath(first.Id))
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := Write(root, sample.Graph, sample.Base, request)
	if duplicate.Record == nil || *duplicate.Record != first || len(duplicate.Diagnostics) > 0 {
		t.Fatalf("same review request created another record: %+v", duplicate)
	}
	request.Comment = "second independent review"
	second := Write(root, sample.Graph, sample.Base, request)
	if second.Record == nil || second.Record.Id == first.Id || len(second.Diagnostics) > 0 {
		t.Fatalf("distinct review did not preserve another record: %+v", second)
	}
	loaded := Read(root)
	if len(loaded.Diagnostics) > 0 || len(loaded.Records) != 2 || !slices.Contains(loaded.Records, first) || !slices.Contains(loaded.Records, *second.Record) {
		t.Fatalf("records were lost or changed: %+v", loaded)
	}
	expectedLatest := first
	if firstAt, firstErr := time.Parse(time.RFC3339Nano, first.At); firstErr != nil {
		t.Fatal(firstErr)
	} else {
		secondAt, secondErr := time.Parse(time.RFC3339Nano, second.Record.At)
		if secondErr != nil {
			t.Fatal(secondErr)
		}
		if secondAt.After(firstAt) || secondAt.Equal(firstAt) && second.Record.Id > first.Id {
			expectedLatest = *second.Record
		}
	}
	evaluation := Evaluate(sample.Graph, sample.Base, loaded.Records)
	if !slices.ContainsFunc(evaluation.Items, func(item Item) bool {
		return item.Target == target && item.Record != nil && *item.Record == expectedLatest
	}) {
		t.Fatalf("the latest matching review was not selected: %+v", evaluation)
	}
	currentBytes, err := os.ReadFile(path)
	if err != nil || string(currentBytes) != string(bytes) {
		t.Fatalf("another write changed immutable record %s: %v", first.Id, err)
	}
	for range 2 {
		if again := Read(root); !reflect.DeepEqual(again, loaded) {
			t.Fatalf("reading the store changed its records: %+v", again)
		}
	}
	attached := Attach(sample.Graph, loaded.Records)
	if len(attached.Diagnostics) > 0 || len(attached.Fragment.Nodes) != 2 {
		t.Fatalf("record projection lost history: %+v", attached)
	}
	joined := sampleBuild(t, append(sample.Graph.Nodes(), attached.Fragment.Nodes...), append(sample.Graph.Edges(), attached.Fragment.Edges...))
	if graph.InputDigest(joined) != graph.InputDigest(sample.Graph) {
		t.Fatal("attaching reviews changed the execution input digest")
	}
	request.ExpectedDigest = strings.Repeat("0", 64)
	stale := Write(root, sample.Graph, sample.Base, request)
	checkDiagnostic(t, stale.Diagnostics, "review.stale-target")
	if stale.Record != nil || !reflect.DeepEqual(Read(root), loaded) {
		t.Fatal("an outdated expectation changed the store")
	}
	request.ExpectedDigest = ""
	request.Reviewer = " "
	checkDiagnostic(t, Write(root, sample.Graph, sample.Base, request).Diagnostics, "review.invalid-reviewer")
	request.Reviewer = "reviewer"
	request.Decision = "unknown"
	checkDiagnostic(t, Write(root, sample.Graph, sample.Base, request).Diagnostics, "review.invalid-decision")
	request.Decision = "approved"
	request.Target = Target{Node: "unknown"}
	checkDiagnostic(t, Write(root, sample.Graph, sample.Base, request).Diagnostics, "review.unknown-target")
	request.Target = Target{Node: "domain", EdgeKind: "calls", From: "domain", To: "domain"}
	checkDiagnostic(t, Write(root, sample.Graph, sample.Base, request).Diagnostics, "review.invalid-target")
	parallelRoot := filepath.Join(root, "parallel")
	if err := os.Mkdir(parallelRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	replies := make(chan Written, 2)
	for _, comment := range []string{"parallel first", "parallel second"} {
		go func() {
			replies <- Write(parallelRoot, sample.Graph, sample.Base, Request{Target: target, Reviewer: "reviewer", Decision: "approved", Comment: comment})
		}()
	}
	parallel := []Record{}
	for range 2 {
		response := <-replies
		if response.Record == nil || len(response.Diagnostics) != 0 {
			t.Fatalf("parallel review failed: %+v", response)
		}
		parallel = append(parallel, *response.Record)
	}
	parallelRead := Read(parallelRoot)
	if len(parallelRead.Diagnostics) != 0 || len(parallelRead.Records) != 2 || !slices.Contains(parallelRead.Records, parallel[0]) || !slices.Contains(parallelRead.Records, parallel[1]) {
		t.Fatalf("parallel writers lost history: %+v", parallelRead)
	}
	corrupt := filepath.Join(root, ".unicode", "reviews", strings.Repeat("0", 64)+".json")
	if err := os.WriteFile(corrupt, []byte("{\"version\":42}"), 0o644); err != nil {
		t.Fatal(err)
	}
	checkDiagnostic(t, Read(root).Diagnostics, "review.invalid-record")
	if err := os.Remove(corrupt); err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(bytes, &object); err != nil {
		t.Fatal(err)
	}
	object["unexpected"] = json.RawMessage("true")
	changed, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corrupt, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	checkDiagnostic(t, Read(root).Diagnostics, "review.invalid-record")
	if err := os.Remove(corrupt); err != nil {
		t.Fatal(err)
	}
	duplicated := append([]byte("{\"comment\":\"ignored\","), bytes[1:]...)
	if err := os.WriteFile(path, duplicated, 0o644); err != nil {
		t.Fatal(err)
	}
	checkDiagnostic(t, Read(root).Diagnostics, "review.invalid-record")
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".unicode", "reviews", strings.Repeat("1", 64)+".json")
	if reviewSymlink(t, path, link) {
		checkDiagnostic(t, Read(root).Diagnostics, "review.symlink")
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
	outside, err := os.MkdirTemp("", "unicode-review-outside-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(outside) }()
	if reviewSymlink(t, filepath.Join(root, ".unicode"), filepath.Join(outside, ".unicode")) {
		checkDiagnostic(t, Read(outside).Diagnostics, "review.symlink")
		request.Target = target
		checkDiagnostic(t, Write(outside, sample.Graph, sample.Base, request).Diagnostics, "review.symlink")
	}
}

func checkDiagnostic(t rapid.TB, diagnostics []diag.Diagnostic, code string) {
	if !slices.ContainsFunc(diagnostics, func(d diag.Diagnostic) bool { return d.Code == code }) {
		t.Fatalf("missing diagnostic %s: %+v", code, diagnostics)
	}
	for _, d := range diagnostics {
		if d.Code == "" || d.Position.Path == "" || d.Position.Line < 1 || d.Position.Column < 1 || d.Message == "" || d.Fix == "" {
			t.Fatalf("incomplete review diagnostic: %+v", d)
		}
	}
}

// Unprivileged Windows processes may lack permission to create symlinks.
func reviewSymlink(t rapid.TB, target, path string) bool {
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		t.Logf("symlink creation is unavailable to this Windows process: %v", err)
		return false
	}
	return true
}

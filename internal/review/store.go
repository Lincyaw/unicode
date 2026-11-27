package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

func recordPath(id string) string {
	return filepath.ToSlash(filepath.Join(".unicode", "reviews", id+".json"))
}

func recordId(record Record) string {
	return digest(map[string]any{"version": record.Version, "reviewer": record.Reviewer, "at": record.At, "snapshot": record.Snapshot, "commit": record.Commit, "dirty": record.Dirty, "target": map[string]any{"node": record.Target.Node, "edgeKind": record.Target.EdgeKind, "from": record.Target.From, "to": record.Target.To, "removed": record.Target.Removed}, "digest": record.Digest, "decision": record.Decision, "comment": record.Comment})
}

func hexString(text string, size int) bool {
	if len(text) != size {
		return false
	}
	for _, char := range text {
		if char < '0' || char > '9' && char < 'a' || char > 'f' {
			return false
		}
	}
	return true
}

func targetReason(target Target) string {
	if target.Node != "" {
		if strings.TrimSpace(target.Node) == "" || target.EdgeKind != "" || target.From != "" || target.To != "" {
			return "a node target gives only its nonempty node identity and removed flag"
		}
		return ""
	}
	if strings.TrimSpace(target.From) == "" || strings.TrimSpace(target.To) == "" {
		return "an edge target gives its kind and two nonempty endpoint identities"
	}
	kinds := []graph.EdgeKind{graph.EdgeKindContains, graph.EdgeKindUses, graph.EdgeKindExtends, graph.EdgeKindImplements, graph.EdgeKindRefers, graph.EdgeKindSubscribes, graph.EdgeKindImports, graph.EdgeKindCalls, graph.EdgeKindFrom, graph.EdgeKindEmits, graph.EdgeKindHandles, graph.EdgeKindAbout, graph.EdgeKindRealizes, graph.EdgeKindChecks, graph.EdgeKindGenerates, graph.EdgeKindObserves, graph.EdgeKindSubstitutes, graph.EdgeKindOwns, graph.EdgeKindDepends, graph.EdgeKindTraces, graph.EdgeKindExercises, graph.EdgeKindReaches, graph.EdgeKindCovers}
	if !slices.Contains(kinds, graph.EdgeKind(target.EdgeKind)) {
		return "the edge target has an unknown edge kind"
	}
	return ""
}

func validateRecord(record Record) string {
	if record.Version != 1 {
		return "the review record version is not 1"
	}
	if why := targetReason(record.Target); why != "" {
		return why
	}
	if strings.TrimSpace(record.Reviewer) == "" || record.Reviewer != strings.TrimSpace(record.Reviewer) {
		return "the reviewer must be a nonempty name without surrounding whitespace"
	}
	if record.Decision != "approved" && record.Decision != "changes-requested" {
		return "the decision must be approved or changes-requested"
	}
	at, err := time.Parse(time.RFC3339Nano, record.At)
	if err != nil || at.UTC().Format(time.RFC3339Nano) != record.At {
		return "the review time must be canonical UTC RFC3339Nano"
	}
	if !hexString(record.Snapshot, 64) || !hexString(record.Digest, 64) || !hexString(record.Id, 64) {
		return "record, snapshot and content digests must be 64 lowercase hexadecimal characters"
	}
	if record.Commit != "" && !hexString(record.Commit, 40) && !hexString(record.Commit, 64) {
		return "the commit must be empty or a complete lowercase Git commit hash"
	}
	for _, text := range []string{record.Reviewer, record.Comment, record.Target.Node, record.Target.EdgeKind, record.Target.From, record.Target.To} {
		if !utf8.ValidString(text) {
			return "review record strings must be valid UTF-8"
		}
	}
	if record.Id != recordId(record) {
		return "the record identity does not match the canonical hash of its content"
	}
	return ""
}

func requiredFields(data []byte, keys []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != len(keys) {
		return errors.New("the record has missing or unknown fields")
	}
	for _, key := range keys {
		value, present := fields[key]
		if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("the field %s is missing or null", key)
		}
	}
	return nil
}

func uniqueKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if delimiter == '{' {
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name := key.(string)
			if seen[name] {
				return fmt.Errorf("the JSON object repeats field %s", name)
			}
			seen[name] = true
			if err := uniqueKeys(decoder); err != nil {
				return err
			}
		}
	} else {
		for decoder.More() {
			if err := uniqueKeys(decoder); err != nil {
				return err
			}
		}
	}
	_, err = decoder.Token()
	return err
}

func decodeRecord(data []byte) (Record, error) {
	var record Record
	if !utf8.Valid(data) {
		return record, errors.New("the JSON is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return record, errors.New("more data follows the review record")
	}
	if err := uniqueKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return record, err
	}
	if err := requiredFields(data, []string{"version", "id", "reviewer", "at", "snapshot", "commit", "dirty", "target", "digest", "decision", "comment"}); err != nil {
		return record, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return record, err
	}
	if err := requiredFields(fields["target"], []string{"node", "edgeKind", "from", "to", "removed"}); err != nil {
		return record, err
	}
	if why := validateRecord(record); why != "" {
		return record, errors.New(why)
	}
	return record, nil
}

// storage checks each path component below the repository before creating or
// reading it. In particular, a symlink is never treated as an absent store.
func storage(root string, create bool) (string, *diag.Diagnostic) {
	if strings.TrimSpace(root) == "" {
		d := issue("review.unreadable-store", "design/system.uc", "", "the repository root is empty", "give an existing repository root")
		return "", &d
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		d := issue("review.unreadable-store", root, "", err.Error(), "give an existing repository root")
		return "", &d
	}
	paths := []string{absolute, filepath.Join(absolute, ".unicode"), filepath.Join(absolute, ".unicode", "reviews")}
	positions := []string{"design/system.uc", ".unicode", ".unicode/reviews"}
	for index, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) && index > 0 {
			if !create {
				return "", nil
			}
			if err = os.Mkdir(path, 0o755); err == nil || errors.Is(err, fs.ErrExist) {
				info, err = os.Lstat(path)
			}
		}
		if err != nil {
			code := "review.unreadable-store"
			if create {
				code = "review.write-failed"
			}
			d := issue(code, positions[index], "", err.Error(), "restore a readable regular review-store directory inside the repository")
			return "", &d
		}
		if info.Mode()&os.ModeSymlink != 0 {
			d := issue("review.symlink", positions[index], "", "the review-store path is a symbolic link", "replace the symbolic link with a regular directory inside the repository")
			return "", &d
		}
		if !info.IsDir() {
			d := issue("review.unreadable-store", positions[index], "", "the review-store path is not a directory", "replace it with a directory or choose the intended repository root")
			return "", &d
		}
	}
	return paths[2], nil
}

// Read reads immutable records. A missing store is an empty collection.
//
//uc:realizes review.read
func Read(root string) Loaded {
	out := Loaded{Records: []Record{}, Diagnostics: []diag.Diagnostic{}}
	dir, problem := storage(root, false)
	if problem != nil {
		out.Diagnostics = append(out.Diagnostics, *problem)
		return out
	}
	if dir == "" {
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, issue("review.unreadable-store", ".unicode/reviews", "", err.Error(), "restore read access to the review store"))
		return out
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		position := ".unicode/reviews/" + entry.Name()
		info, err := os.Lstat(path)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, issue("review.unreadable-store", position, "", err.Error(), "restore read access to this review record"))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			out.Diagnostics = append(out.Diagnostics, issue("review.symlink", position, "", "a review record is a symbolic link", "replace it with the immutable regular JSON record"))
			continue
		}
		id, suffix := strings.CutSuffix(entry.Name(), ".json")
		if !suffix || !hexString(id, 64) || !info.Mode().IsRegular() {
			out.Diagnostics = append(out.Diagnostics, issue("review.invalid-record", position, "", "the store entry is not a regular lowercase-digest.json record", "restore the correctly named immutable review record or remove this entry"))
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, issue("review.unreadable-store", position, id, err.Error(), "restore read access to this review record"))
			continue
		}
		record, err := decodeRecord(data)
		if err == nil && record.Id != id {
			err = errors.New("the filename does not match the record identity")
		}
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, issue("review.invalid-record", position, id, err.Error(), "restore the immutable review record with its matching filename and canonical content"))
			continue
		}
		out.Records = append(out.Records, record)
	}
	return out
}

func knownTarget(g, base *graph.Graph, target Target) bool {
	current := target
	current.Removed = false
	if target.Removed {
		if base == nil {
			return false
		}
		_, before := candidates(base)[targetKey(current)]
		if !before {
			return false
		}
		if target.Node != "" {
			return g.Node(target.Node) == nil
		}
		_, exists := edgeOf(g, current)
		return !exists
	}
	_, exists := candidates(g)[targetKey(current)]
	return exists
}

// Write records the content actually reviewed, refusing an outdated expectation.
//
//uc:realizes review.write
func Write(root string, g *graph.Graph, base *graph.Graph, request Request) Written {
	out := Written{Diagnostics: []diag.Diagnostic{}}
	fail := func(code, cause, fix string) Written {
		problem := issue(code, "design/system.uc", targetSubject(request.Target), cause, fix)
		for _, source := range []*graph.Graph{g, base} {
			if source == nil {
				continue
			}
			if node := source.Node(request.Target.Node); node != nil && node.Origin.Path != "" {
				problem.Position = node.Origin
				break
			}
			if edge, found := edgeOf(source, request.Target); found && edge.Origin.Path != "" {
				problem.Position = edge.Origin
				break
			}
		}
		if problem.Position.Line < 1 {
			problem.Position.Line = 1
		}
		if problem.Position.Column < 1 {
			problem.Position.Column = 1
		}
		out.Diagnostics = append(out.Diagnostics, problem)
		return out
	}
	if why := targetReason(request.Target); why != "" {
		return fail("review.invalid-target", why, "give exactly one node identity or one complete edge identity")
	}
	if strings.TrimSpace(request.Reviewer) == "" {
		return fail("review.invalid-reviewer", "the reviewer is empty", "give the person or agent that reviewed this content")
	}
	if request.Decision != "approved" && request.Decision != "changes-requested" {
		return fail("review.invalid-decision", "the decision is neither approved nor changes-requested", "choose approved or changes-requested")
	}
	if request.ExpectedDigest != "" && !hexString(request.ExpectedDigest, 64) {
		return fail("review.invalid-record", "the expected digest is not a lowercase SHA-256 digest", "give the 64-character digest listed for the target")
	}
	if !knownTarget(g, base, request.Target) {
		return fail("review.unknown-target", "the graph or comparison has no such reviewable target", "select a current target or a deletion from a comparison with its base")
	}
	view := g
	if request.Target.Removed {
		view = base
	}
	current := item(view, request.Target, nil)
	if request.ExpectedDigest != "" && request.ExpectedDigest != current.Digest {
		return fail("review.stale-target", "the target content changed since the expected review digest", "read the current target and review the changed content before recording it")
	}
	loaded := Read(root)
	if len(loaded.Diagnostics) > 0 {
		out.Diagnostics = loaded.Diagnostics
		return out
	}
	record := Record{Version: 1, Reviewer: strings.TrimSpace(request.Reviewer), At: time.Now().UTC().Format(time.RFC3339Nano), Snapshot: graph.InputDigest(g), Commit: request.Commit, Dirty: request.Dirty, Target: request.Target, Digest: current.Digest, Decision: request.Decision, Comment: request.Comment}
	record.Id = recordId(record)
	if why := validateRecord(record); why != "" {
		return fail("review.invalid-record", why, "give valid UTF-8 review metadata and a complete commit hash when supplied")
	}
	for _, existing := range loaded.Records {
		copy := record
		copy.At, copy.Id = existing.At, existing.Id
		if copy == existing {
			out.Record = &existing
			return out
		}
	}
	dir, problem := storage(root, true)
	if problem != nil {
		out.Diagnostics = append(out.Diagnostics, *problem)
		return out
	}
	path := filepath.Join(dir, record.Id+".json")
	data := append(mustJSON(record), '\n')
	temp, err := os.CreateTemp(filepath.Dir(dir), ".review-pending-")
	if err != nil {
		return fail("review.write-failed", err.Error(), "restore write access to the local review store")
	}
	tempPath := temp.Name()
	// A leftover staging file is outside reviews and cannot be read as a record.
	defer func() { _ = os.Remove(tempPath) }()
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fail("review.write-failed", err.Error(), "restore write access and retry the immutable review record")
	}
	if err = os.Link(tempPath, path); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return fail("review.write-failed", err.Error(), "restore write access on a filesystem supporting immutable review records")
		}
		loaded = Read(root)
		if len(loaded.Diagnostics) > 0 {
			out.Diagnostics = loaded.Diagnostics
			return out
		}
		for _, existing := range loaded.Records {
			if existing == record {
				out.Record = &existing
				return out
			}
		}
		return fail("review.write-failed", "a different record already occupies the immutable path", "restore the original immutable review record before retrying")
	}
	out.Record = &record
	return out
}

package repo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The files that build writes to its directory: the snapshot, and the
// record of the repository's head beside it.
const (
	SnapshotFile = "graph.json"
	RecordFile   = "build.json"
)

// NoGit is the error of reading a repository without the command git.
//
//uc:realizes repo.NoGit
type NoGit struct{ Cause error }

func (e NoGit) Error() string {
	return fmt.Sprintf("reading the repository needs the command git on PATH: %v", e.Cause)
}

func (e NoGit) Unwrap() error { return e.Cause }

// BadRevision is the error of reading a revision that names no commit git can read.
//
//uc:realizes repo.BadRevision
type BadRevision struct {
	Rev   string
	Cause error
}

func (e BadRevision) Error() string {
	return fmt.Sprintf("%s is not a commit of the repository: %v", e.Rev, e.Cause)
}

func (e BadRevision) Unwrap() error { return e.Cause }

// GitFailed is the error of a git command that failed for another reason
// than an unreadable revision.
//
//uc:realizes repo.GitFailed
type GitFailed struct {
	Args  []string
	Cause error
}

func (e GitFailed) Error() string {
	return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Cause)
}

func (e GitFailed) Unwrap() error { return e.Cause }

// BadRecord is the error of reading a record that does not exist, cannot be
// read, or is not {commit, dirty}.
//
//uc:realizes repo.BadRecord
type BadRecord struct {
	Path  string
	Cause error
}

func (e BadRecord) Error() string {
	return fmt.Sprintf("cannot read the record %s: %v", e.Path, e.Cause)
}

func (e BadRecord) Unwrap() error { return e.Cause }

// Revision is a commit as the viewer's interface writes it (api/snapshot.tsp):
// its hash, the abbreviated hash, the branch checked out at it, the first
// line of its message and its committer time in UTC.
//
//uc:realizes repo.Revision
type Revision struct {
	Commit  string    `json:"commit"`
	Short   string    `json:"short"`
	Branch  string    `json:"branch,omitempty"`
	Subject string    `json:"subject"`
	Time    time.Time `json:"time"`
}

// WorkingTree is the commit a working tree is at, written as a Revision
// is, and whether the working tree differs from it.
//
//uc:realizes repo.WorkingTree
type WorkingTree struct {
	Commit  string    `json:"commit"`
	Short   string    `json:"short"`
	Branch  string    `json:"branch,omitempty"`
	Subject string    `json:"subject"`
	Time    time.Time `json:"time"`
	Dirty   bool      `json:"dirty"`
}

// At is the working tree at the commit r, dirty or not.
func (r Revision) At(dirty bool) WorkingTree {
	return WorkingTree{Commit: r.Commit, Short: r.Short, Branch: r.Branch, Subject: r.Subject, Time: r.Time, Dirty: dirty}
}

// Head is the record build writes beside the snapshot: the commit the
// repository is at, nil outside a Git work tree with a commit, and whether
// the working tree differs from it, which it always does when there is no commit.
//
//uc:realizes repo.Head
type Head struct {
	Commit *string `json:"commit"`
	Dirty  bool    `json:"dirty"`
}

// run runs git in dir and returns its standard output; its error is what
// git wrote to standard error.
func run(dir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, NoGit{Cause: err}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			err = errors.New(msg)
		}
		return nil, GitFailed{Args: args, Cause: err}
	}
	return out, nil
}

// git runs git in dir and returns its standard output without the final newline.
func git(dir string, args ...string) (string, error) {
	out, err := run(dir, args...)
	return strings.TrimSuffix(string(out), "\n"), err
}

// revisionFormat writes a commit one field after another, separated by NUL;
// the subject, last, holds no newline.
const revisionFormat = "--format=%H%x00%h%x00%cI%x00%s"

func parseRevision(line string) (Revision, error) {
	fields := strings.SplitN(line, "\x00", 4)
	if len(fields) != 4 {
		return Revision{}, fmt.Errorf("git gives the commit as %q", line)
	}
	at, err := time.Parse(time.RFC3339, fields[2])
	if err != nil {
		return Revision{}, fmt.Errorf("git gives the time of %s as %q: %w", fields[0], fields[2], err)
	}
	return Revision{Commit: fields[0], Short: fields[1], Subject: fields[3], Time: at.UTC()}, nil
}

// revisionError tells a git failure about rev apart: no git, or a revision
// git cannot read.
func revisionError(rev string, err error) error {
	var failed GitFailed
	if errors.As(err, &failed) {
		return BadRevision{Rev: rev, Cause: failed.Cause}
	}
	return err
}

// ReadRevision reads the commit that rev names in the repository at root,
// with the branch when it is the commit checked out on one.
//
//uc:realizes repo.readRevision
func ReadRevision(root, rev string) (Revision, error) {
	if strings.HasPrefix(rev, "-") || rev == "" {
		return Revision{}, BadRevision{Rev: rev, Cause: errors.New("it is not a revision")}
	}
	out, err := git(root, "-c", "log.showSignature=false", "show", "--no-patch", revisionFormat, rev+"^{commit}", "--")
	if err != nil {
		return Revision{}, revisionError(rev, err)
	}
	r, err := parseRevision(out)
	if err != nil {
		return Revision{}, BadRevision{Rev: rev, Cause: err}
	}
	r.Branch = branchAt(root, r.Commit)
	return r, nil
}

// branchAt is the branch checked out at the commit, "" when the commit is
// not checked out or HEAD is detached.
func branchAt(root, commit string) string {
	if head, err := git(root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err == nil && head == commit {
		branch, _ := git(root, "symbolic-ref", "--quiet", "--short", "HEAD")
		return branch
	}
	return ""
}

// Log returns the latest commits of rev's history in the repository at
// root, newest first, at most limit of them; none when limit is below 1.
// The newest has its branch when it is the commit checked out on one.
//
//uc:realizes repo.log
func Log(root, rev string, limit int) ([]Revision, error) {
	if limit < 1 {
		return []Revision{}, nil
	}
	if strings.HasPrefix(rev, "-") || rev == "" {
		return nil, BadRevision{Rev: rev, Cause: errors.New("it is not a revision")}
	}
	out, err := git(root, "-c", "log.showSignature=false", "log", revisionFormat, "-n", strconv.Itoa(limit), rev, "--")
	if err != nil {
		return nil, revisionError(rev, err)
	}
	revs := []Revision{}
	for line := range strings.Lines(out) {
		r, err := parseRevision(strings.TrimSuffix(line, "\n"))
		if err != nil {
			return nil, BadRevision{Rev: rev, Cause: err}
		}
		revs = append(revs, r)
	}
	if len(revs) > 0 {
		revs[0].Branch = branchAt(root, revs[0].Commit)
	}
	return revs, nil
}

// Show returns the content of the file path, relative to root, in the commit.
//
//uc:realizes repo.show
func Show(root, commit, path string) ([]byte, error) {
	if strings.HasPrefix(commit, "-") || commit == "" {
		return nil, BadRevision{Rev: commit, Cause: errors.New("it is not a revision")}
	}
	out, err := run(root, "show", commit+":./"+filepath.ToSlash(path))
	if err != nil {
		return nil, revisionError(commit+":"+path, err)
	}
	return out, nil
}

// ReadWorkingTree reads the commit that the working tree at root is at, and
// whether the files under root differ from it, apart from the snapshot and
// the record that build writes to the directory out.
//
//uc:realizes repo.readWorkingTree
func ReadWorkingTree(root, out string) (WorkingTree, error) {
	r, err := ReadRevision(root, "HEAD")
	if err != nil {
		return WorkingTree{}, err
	}
	dirty, err := isDirty(root, out)
	if err != nil {
		return WorkingTree{}, err
	}
	return r.At(dirty), nil
}

// ReadHead reads what build records of the repository at root, which writes
// to the directory out.
//
//uc:realizes repo.readHead
func ReadHead(root, out string) (Head, error) {
	commit, err := git(root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return Head{Dirty: true}, nil
	}
	dirty, err := isDirty(root, out)
	return Head{Commit: &commit, Dirty: dirty}, err
}

// isDirty tells whether git status lists a change or an untracked file
// under root, leaving out the snapshot and the record that build writes to
// the directory out.
func isDirty(root, out string) (bool, error) {
	args := []string{"status", "--porcelain", "--untracked-files=all", "--", "."}
	abs, err := filepath.Abs(out)
	if err != nil {
		return false, GitFailed{Args: args, Cause: fmt.Errorf("cannot resolve the directory %s: %w", out, err)}
	}
	if rel, err := filepath.Rel(root, abs); err == nil && filepath.IsLocal(rel) {
		for _, name := range []string{SnapshotFile, RecordFile} {
			args = append(args, ":(exclude,literal)"+filepath.ToSlash(filepath.Join(rel, name)))
		}
	}
	status, err := git(root, args...)
	if err != nil {
		return false, err
	}
	return status != "", nil
}

// ReadRecord reads the record build wrote to the directory dir.
//
//uc:realizes repo.readRecord
func ReadRecord(dir string) (Head, error) {
	path := filepath.Join(dir, RecordFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return Head{}, BadRecord{Path: path, Cause: err}
	}
	var fields map[string]json.RawMessage
	var h Head
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 2 || fields["commit"] == nil || fields["dirty"] == nil {
		return Head{}, BadRecord{Path: path, Cause: fmt.Errorf("it is not {\"commit\": <hash or null>, \"dirty\": <bool>}: %s", bytes.TrimSpace(data))}
	}
	if err := dec.Decode(&h); err != nil {
		return Head{}, BadRecord{Path: path, Cause: err}
	}
	return h, nil
}

// LinesChanged counts the lines that differ between two texts as git diff
// counts them: the lines added and the lines removed.
//
//uc:realizes repo.linesChanged
func LinesChanged(before, after string) (int, error) {
	if before == after {
		return 0, nil
	}
	dir, err := os.MkdirTemp("", "unicode-lines-")
	if err != nil {
		return 0, GitFailed{Args: []string{"diff"}, Cause: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for name, text := range map[string]string{"before": before, "after": after} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			return 0, GitFailed{Args: []string{"diff"}, Cause: err}
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return 0, NoGit{Cause: err}
	}
	args := []string{"diff", "--no-index", "--numstat", "--", "before", "after"}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	// git diff --no-index exits with 1 when the files differ.
	if exit := (*exec.ExitError)(nil); err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return 0, GitFailed{Args: args, Cause: err}
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, GitFailed{Args: args, Cause: fmt.Errorf("git diff --numstat gives %q", out)}
	}
	added, errAdded := strconv.Atoi(fields[0])
	removed, errRemoved := strconv.Atoi(fields[1])
	if errAdded != nil || errRemoved != nil {
		return 0, GitFailed{Args: args, Cause: fmt.Errorf("git diff --numstat gives %q", out)}
	}
	return added + removed, nil
}

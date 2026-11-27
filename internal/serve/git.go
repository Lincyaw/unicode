package serve

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// git runs git in the repository at root and returns its standard output.
func git(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// revisionFormat writes a commit as git log --format gives it, one field
// after another, separated by NUL; the subject, last, holds no newline.
const revisionFormat = "--format=%H%x00%h%x00%cI%x00%s"

// revisions returns the latest commits of rev's history, newest first, at
// most limit of them; none when git cannot read them.
func revisions(root, rev string, limit int) []Revision {
	out := []Revision{}
	if limit < 1 {
		return out
	}
	text, err := git(root, "log", revisionFormat, "-n", strconv.Itoa(limit), "--end-of-options", rev, "--")
	if err != nil {
		return out
	}
	for line := range strings.Lines(string(text)) {
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), "\x00", 4)
		if len(fields) != 4 {
			continue
		}
		at, _ := time.Parse(time.RFC3339, fields[2])
		out = append(out, Revision{Commit: fields[0], Short: fields[1], Time: at.UTC(), Subject: fields[3]})
	}
	return out
}

// branch is the branch checked out in the repository at root, or nil.
func branch(root string) *string {
	out, err := git(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(string(out))
	return &name
}

// readHead reads the working tree of the repository at root: the commit it
// is checked out at, its branch, and whether any file differs from the
// commit, untracked files included. Without git or a commit, it is empty.
func readHead(root string) WorkingTree {
	head := WorkingTree{}
	if rs := revisions(root, "HEAD", 1); len(rs) == 1 {
		r := rs[0]
		head = WorkingTree{Commit: r.Commit, Short: r.Short, Subject: r.Subject, Time: r.Time, Branch: branch(root)}
	}
	if status, err := git(root, "status", "--porcelain"); err == nil {
		head.Dirty = len(bytes.TrimSpace(status)) > 0
	}
	return head
}

// revision reads the commit that rev names in the repository at root; a
// rev that names no commit is refused with serve.bad-revision.
func revision(root, rev string) (Revision, error) {
	bad := func(why string) (Revision, error) {
		return Revision{}, refuse(http.StatusBadRequest, "serve.bad-revision",
			"give a commit of the repository, such as HEAD~1 or a hash from /api/revisions", "the revision %q %s", rev, why)
	}
	if rev == "" || strings.HasPrefix(rev, "-") {
		return bad("is not a revision")
	}
	out, err := git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return bad("names no commit git can read")
	}
	rs := revisions(root, strings.TrimSpace(string(out)), 1)
	if len(rs) != 1 {
		return bad("names no commit git can read")
	}
	return rs[0], nil
}

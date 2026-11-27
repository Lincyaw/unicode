package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// checkoutRev writes the tree of the commit rev under root, as git archive
// gives it, into a new temporary directory and returns that directory. A
// revision that git cannot archive is a usage error; no git is an
// environment error.
func checkoutRev(root, rev string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", &exitError{code: exitEnvironment, err: fmt.Errorf("--rev needs the command git on PATH: %w", err)}
	}
	if strings.HasPrefix(rev, "-") {
		return "", usageError("--rev %s is not a revision", rev)
	}
	dir, err := os.MkdirTemp("", "unicode-rev-")
	if err != nil {
		return "", &exitError{code: exitUnexpected, err: fmt.Errorf("cannot make a directory for the commit %s: %w", rev, err)}
	}
	// git archive run in root holds root's subtree of the commit, with paths relative to root.
	cmd := exec.Command("git", "archive", "--format=tar", rev)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", &exitError{code: exitUnexpected, err: fmt.Errorf("cannot run git archive: %w", err)}
	}
	written := untar(dir, out)
	_, _ = io.Copy(io.Discard, out)
	if err := cmd.Wait(); err != nil {
		_ = os.RemoveAll(dir)
		return "", usageError("--rev %s: git archive cannot read the commit: %s", rev, strings.TrimSpace(stderr.String()))
	}
	if written != nil {
		_ = os.RemoveAll(dir)
		return "", &exitError{code: exitUnexpected, err: fmt.Errorf("cannot write the files of the commit %s: %w", rev, written)}
	}
	return dir, nil
}

// untar writes the directories, files and symbolic links of a tar stream
// under dir, each file with its mode.
func untar(dir string, r io.Reader) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		case !filepath.IsLocal(h.Name):
			return fmt.Errorf("the archive holds the path %s, which is outside its root", h.Name)
		}
		full := filepath.Join(dir, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(full, 0o755)
		case tar.TypeReg:
			err = writeEntry(full, tr, os.FileMode(h.Mode).Perm())
		case tar.TypeSymlink:
			if err = os.MkdirAll(filepath.Dir(full), 0o755); err == nil {
				err = os.Symlink(h.Linkname, full)
			}
		}
		if err != nil {
			return err
		}
	}
}

func writeEntry(full string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// revision is a commit as the viewer's interface writes it (api/snapshot.tsp):
// its hash, the abbreviated hash, the branch checked out at it, the first
// line of its message and its committer time in UTC.
type revision struct {
	Commit  string    `json:"commit"`
	Short   string    `json:"short"`
	Branch  string    `json:"branch,omitempty"`
	Subject string    `json:"subject"`
	Time    time.Time `json:"time"`
}

// workingTree is the commit a working tree is at, and whether the working
// tree differs from it.
type workingTree struct {
	revision
	Dirty bool `json:"dirty"`
}

// git runs git in dir and returns its standard output without the final
// newline; its error is what git wrote to standard error.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// readRevision reads the commit that rev names in the repository at root. A
// rev that names no commit is a usage error; no git is an environment error.
func readRevision(root, rev string) (revision, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return revision{}, &exitError{code: exitEnvironment, err: fmt.Errorf("reading the commit %s needs the command git on PATH: %w", rev, err)}
	}
	if strings.HasPrefix(rev, "-") {
		return revision{}, usageError("%s is not a revision", rev)
	}
	out, err := git(root, "-c", "log.showSignature=false", "show", "--no-patch", "--format=%H%x00%h%x00%cI%x00%s", rev+"^{commit}", "--")
	if err != nil {
		return revision{}, usageError("%s is not a commit of the repository at %s: %v", rev, root, err)
	}
	fields := strings.SplitN(out, "\x00", 4)
	if len(fields) != 4 {
		return revision{}, &exitError{code: exitUnexpected, err: fmt.Errorf("git show gives the commit %s as %q", rev, out)}
	}
	at, err := time.Parse(time.RFC3339, fields[2])
	if err != nil {
		return revision{}, &exitError{code: exitUnexpected, err: fmt.Errorf("git show gives the time of %s as %q: %w", rev, fields[2], err)}
	}
	r := revision{Commit: fields[0], Short: fields[1], Subject: fields[3], Time: at.UTC()}
	// The branch is given only for the commit checked out on one; a detached HEAD has none.
	if head, err := git(root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err == nil && head == r.Commit {
		r.Branch, _ = git(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	}
	return r, nil
}

// readWorkingTree reads the commit that the working tree at root is at, and
// whether the files under root differ from it, apart from the files that
// build writes to the directory out.
func readWorkingTree(root, out string) (workingTree, error) {
	r, err := readRevision(root, "HEAD")
	if err != nil {
		return workingTree{}, err
	}
	dirty, err := isDirty(root, out)
	if err != nil {
		return workingTree{}, err
	}
	return workingTree{revision: r, Dirty: dirty}, nil
}

// isDirty tells whether git status lists a change or an untracked file
// under root, leaving out the snapshot and the head record that build
// writes to the directory out.
func isDirty(root, out string) (bool, error) {
	args := []string{"status", "--porcelain", "--untracked-files=all", "--", "."}
	abs, err := filepath.Abs(out)
	if err != nil {
		return false, &exitError{code: exitUnexpected, err: fmt.Errorf("cannot resolve the directory %s: %w", out, err)}
	}
	if rel, err := filepath.Rel(root, abs); err == nil && filepath.IsLocal(rel) {
		for _, name := range []string{snapshotFile, headFile} {
			args = append(args, ":(exclude,literal)"+filepath.ToSlash(filepath.Join(rel, name)))
		}
	}
	status, err := git(root, args...)
	if err != nil {
		return false, &exitError{code: exitUnexpected, err: err}
	}
	return status != "", nil
}

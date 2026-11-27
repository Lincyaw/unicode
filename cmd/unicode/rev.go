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

	"github.com/lincyaw/unicode/internal/repo"
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

// repoError ends a command whose reading of the repository failed: without
// git it is an environment error, a revision that names no commit is a usage
// error, and any other failure is unexpected.
func repoError(err error) error {
	switch {
	case errors.As(err, new(repo.NoGit)):
		return &exitError{code: exitEnvironment, err: err}
	case errors.As(err, new(repo.BadRevision)):
		return usageError("%v", err)
	}
	return &exitError{code: exitUnexpected, err: err}
}

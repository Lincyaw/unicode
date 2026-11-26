package main

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

	"github.com/google/renameio/v2"
	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/syntax"
)

// fmtCommand is `unicode fmt [--check] [path...]`.
func fmtCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	check := false
	c := &cobra.Command{
		Use:   "fmt [path...]",
		Short: "Rewrite design and corpus files in the canonical format; with --check, only report the files that are not",
		Long: "fmt rewrites design and corpus files in the canonical format of docs/format.md. With --check it rewrites\n" +
			"nothing and reports each file that is not canonical. Without paths it takes every file under design/ and corpus/;\n" +
			"a path names a file or a directory inside them.",
		RunE: func(_ *cobra.Command, paths []string) error {
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			return runFmt(root, g.format, check, paths, stdout, stderr)
		},
	}
	c.Flags().BoolVar(&check, "check", false, "report the files that are not canonical, and rewrite nothing")
	return c
}

// The statuses of a file in the report of fmt.
const (
	statusUnchanged    = "unchanged"
	statusRewritten    = "rewritten"
	statusNotCanonical = "not-canonical"
	statusInvalid      = "invalid"
)

type fileReport struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type diagnosticJSON struct {
	Code    string `json:"code"`
	Subject string `json:"subject"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

func diagnosticsJSON(diags []diag.Diagnostic) []diagnosticJSON {
	out := []diagnosticJSON{}
	for _, d := range diags {
		out = append(out, diagnosticJSON{
			Code: d.Code, Subject: d.Subject, Path: d.Position.Path, Line: d.Position.Line, Column: d.Position.Column,
			Message: d.Message, Fix: d.Fix,
		})
	}
	return out
}

// diagnosticsText writes one line per diagnostic, for standard error.
func diagnosticsText(diags []diag.Diagnostic) []byte {
	var out []byte
	for _, d := range diags {
		out = fmt.Appendf(out, "%s:%d:%d: %s: %s (fix: %s)\n", d.Position.Path, d.Position.Line, d.Position.Column, d.Code, d.Message, d.Fix)
	}
	return out
}

type fmtReport struct {
	Files       []fileReport     `json:"files"`
	Diagnostics []diagnosticJSON `json:"diagnostics"`
}

// runFmt rewrites, or with --check only reports, every design and corpus file
// that is not in the canonical format. A file with diagnostics is never written.
func runFmt(root, format string, check bool, args []string, stdout, stderr io.Writer) error {
	paths, err := fmtPaths(root, args)
	if err != nil {
		return err
	}
	report := fmtReport{Files: []fileReport{}}
	var diags []diag.Diagnostic
	for _, rel := range paths {
		status, ds, err := formatFile(root, rel, check)
		if err != nil {
			return &exitError{code: exitUnexpected, err: err}
		}
		report.Files = append(report.Files, fileReport{Path: rel, Status: status})
		diags = append(diags, ds...)
	}
	report.Diagnostics = diagnosticsJSON(diags)
	var out, errOut []byte
	if format == "json" {
		if out, err = json.MarshalIndent(report, "", "  "); err != nil {
			return &exitError{code: exitUnexpected, err: fmt.Errorf("cannot encode the JSON report: %w", err)}
		}
		out = append(out, '\n')
	} else {
		for _, f := range report.Files {
			if f.Status == statusRewritten || f.Status == statusNotCanonical {
				out = fmt.Appendln(out, f.Path)
			}
		}
		errOut = diagnosticsText(diags)
	}
	if _, err := stdout.Write(out); err != nil {
		return &exitError{code: exitUnexpected}
	}
	if _, err := stderr.Write(errOut); err != nil {
		return &exitError{code: exitUnexpected}
	}
	if len(diags) > 0 {
		return &exitError{code: exitDiagnostics}
	}
	return nil
}

// fmtPaths lists the files to format as sorted slash paths relative to root:
// those under the given paths, or every file under design/ and corpus/. A
// path must name a file or directory inside design/ or corpus/.
func fmtPaths(root string, args []string) ([]string, error) {
	var tops []string
	if len(args) == 0 {
		for _, dir := range []string{"design", "corpus"} {
			if info, err := os.Stat(filepath.Join(root, dir)); err == nil && info.IsDir() {
				tops = append(tops, filepath.Join(root, dir))
			}
		}
	}
	for _, a := range args {
		abs, err := filepath.Abs(a)
		if err != nil {
			return nil, usageError("path %s: %v", a, err)
		}
		rel, err := filepath.Rel(root, abs)
		top, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		if err != nil || top != "design" && top != "corpus" {
			return nil, usageError("path %s is not inside design/ or corpus/ of the repository %s", a, root)
		}
		if _, err := os.Lstat(abs); err != nil {
			return nil, usageError("path %s does not exist", a)
		}
		tops = append(tops, abs)
	}
	var paths []string
	for _, top := range tops {
		err := filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				paths = append(paths, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			return nil, &exitError{code: exitUnexpected, err: fmt.Errorf("cannot list the files under %s: %w", top, err)}
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// formatFile formats one file and returns its status and diagnostics; the
// error is an unexpected failure to write it.
func formatFile(root, rel string, check bool) (string, []diag.Diagnostic, error) {
	src, f, diags := readFile(root, rel)
	if f == nil {
		return statusInvalid, diags, nil
	}
	out := f.Print()
	switch {
	case bytes.Equal(out, src):
		return statusUnchanged, nil, nil
	case check:
		return statusNotCanonical, []diag.Diagnostic{notCanonical(rel, src, out)}, nil
	}
	if err := renameio.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), out, 0o644); err != nil {
		return "", nil, fmt.Errorf("cannot write %s: %w", rel, err)
	}
	return statusRewritten, nil, nil
}

// readFile reads and parses one file of design/ or corpus/. A symbolic link,
// which is neither followed nor rewritten, a path that KindOf rejects, a file
// that cannot be read and one with syntax errors give diagnostics and no tree.
func readFile(root, rel string) ([]byte, *syntax.File, []diag.Diagnostic) {
	at := diag.Position{Path: rel, Line: 1, Column: 1}
	full := filepath.Join(root, filepath.FromSlash(rel))
	if info, err := os.Lstat(full); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, []diag.Diagnostic{{
			Code: "cli.symlink", Subject: rel, Position: at,
			Message: "the file is a symbolic link, which unicode neither follows nor rewrites",
			Fix:     "replace the link with the file it points to, or move the link out of design/ and corpus/",
		}}
	}
	kind, err := syntax.KindOf(rel)
	if err != nil {
		return nil, nil, []diag.Diagnostic{{
			Code: "cli.not-a-design-file", Subject: rel, Position: at,
			Message: "design/ and corpus/ may hold only .uc files, and this file is not one",
			Fix:     "move the file out of design/ and corpus/, or rename it",
		}}
	}
	src, err := os.ReadFile(full)
	if err != nil {
		// The cause alone, without the absolute path that the error repeats.
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			err = pathErr.Err
		}
		return nil, nil, []diag.Diagnostic{{
			Code: "cli.unreadable-file", Subject: rel, Position: at,
			Message: "cannot read the file: " + err.Error(), Fix: "make the file readable, or remove it",
		}}
	}
	parsed := syntax.Parse(rel, kind, src)
	return src, parsed.File, parsed.Diagnostics
}

// notCanonical is the diagnostic of a file src whose canonical form is out.
func notCanonical(rel string, src, out []byte) diag.Diagnostic {
	line, column := firstDifference(src, out)
	return diag.Diagnostic{
		Code: "cli.not-canonical", Subject: rel, Position: diag.Position{Path: rel, Line: line, Column: column},
		Message: "the file is not in the canonical format: it differs from the output of unicode fmt from here on",
		Fix:     "run unicode fmt",
	}
}

// firstDifference returns the line and byte column in a of the first byte where a and b differ.
func firstDifference(a, b []byte) (int, int) {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	lineStart := bytes.LastIndexByte(a[:n], '\n') + 1
	return bytes.Count(a[:n], []byte{'\n'}) + 1, n - lineStart + 1
}

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

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/syntax"
)

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

type fmtReport struct {
	Files       []fileReport     `json:"files"`
	Diagnostics []diagnosticJSON `json:"diagnostics"`
}

// runFmt rewrites, or with --check only reports, every design and corpus file
// that is not in the canonical format. A file with diagnostics is never written.
func runFmt(root string, o options, stdout, stderr io.Writer) int {
	paths, err := fmtPaths(root, o.paths)
	if err != nil {
		return usageError(stderr, err)
	}
	report := fmtReport{Files: []fileReport{}, Diagnostics: []diagnosticJSON{}}
	var diags []diag.Diagnostic
	for _, rel := range paths {
		status, ds, err := formatFile(root, rel, o.check)
		if err != nil {
			return write(stderr, fmt.Appendf(nil, "unicode：%v\n", err), exitUnexpected)
		}
		report.Files = append(report.Files, fileReport{Path: rel, Status: status})
		diags = append(diags, ds...)
	}
	for _, d := range diags {
		report.Diagnostics = append(report.Diagnostics, diagnosticJSON{
			Code: d.Code, Subject: d.Subject, Path: d.Position.Path, Line: d.Position.Line, Column: d.Position.Column,
			Message: d.Message, Fix: d.Fix,
		})
	}
	code := exitOK
	if len(diags) > 0 {
		code = exitDiagnostics
	}
	if o.format == "json" {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return write(stderr, fmt.Appendf(nil, "unicode：%v\n", err), exitUnexpected)
		}
		return write(stdout, append(out, '\n'), code)
	}
	var out, errOut []byte
	for _, f := range report.Files {
		if f.Status == statusRewritten || f.Status == statusNotCanonical {
			out = fmt.Appendln(out, f.Path)
		}
	}
	for _, d := range diags {
		errOut = fmt.Appendf(errOut, "%s:%d:%d: %s: %s（修正：%s）\n", d.Position.Path, d.Position.Line, d.Position.Column, d.Code, d.Message, d.Fix)
	}
	if write(stdout, out, code) == exitUnexpected {
		return exitUnexpected
	}
	return write(stderr, errOut, code)
}

// fmtPaths lists the files to format as sorted slash paths relative to root:
// those under the given paths, or every file under design/ and corpus/.
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
			return nil, fmt.Errorf("路径 %s 不可用：%v", a, err)
		}
		if rel, err := filepath.Rel(root, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("路径 %s 不在仓库根目录 %s 之内", a, root)
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("路径 %s 不存在", a)
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
			return nil, fmt.Errorf("读不出 %s 下的文件：%v", top, err)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// formatFile formats one file and returns its status and diagnostics; the
// error is an unexpected failure to write it.
func formatFile(root, rel string, check bool) (string, []diag.Diagnostic, error) {
	at := diag.Position{Path: rel, Line: 1, Column: 1}
	kind, err := syntax.KindOf(rel)
	if err != nil {
		return statusInvalid, []diag.Diagnostic{{
			Code: "cli.not-a-design-file", Subject: rel, Position: at,
			Message: "design/ 和 corpus/ 中只能有 .uc 文件，参数中的路径也只能指向它们：" + err.Error(),
			Fix:     "把这个文件移出 design/ 和 corpus/，或改正路径",
		}}, nil
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	src, err := os.ReadFile(full)
	if err != nil {
		return statusInvalid, []diag.Diagnostic{{
			Code: "cli.unreadable-file", Subject: rel, Position: at,
			Message: fmt.Sprintf("读不出这个文件：%v", err),
			Fix:     "检查文件的权限，或删去它",
		}}, nil
	}
	parsed := syntax.Parse(rel, kind, src)
	if len(parsed.Diagnostics) > 0 {
		return statusInvalid, parsed.Diagnostics, nil
	}
	out := parsed.File.Print()
	switch {
	case bytes.Equal(out, src):
		return statusUnchanged, nil, nil
	case check:
		line, column := firstDifference(src, out)
		return statusNotCanonical, []diag.Diagnostic{{
			Code: "cli.not-canonical", Subject: rel, Position: diag.Position{Path: rel, Line: line, Column: column},
			Message: "文件不是规范格式：从这里起与 unicode fmt 的输出不同",
			Fix:     "运行 unicode fmt",
		}}, nil
	}
	if err := writeFile(full, out); err != nil {
		return "", nil, fmt.Errorf("写不进 %s：%w", rel, err)
	}
	return statusRewritten, nil, nil
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

// writeFile replaces the file's content through a rename, keeping its mode,
// so that an interrupted write leaves the old content.
func writeFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".unicode-fmt-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(tmp.Name(), info.Mode().Perm())); err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}
	return os.Rename(tmp.Name(), path)
}

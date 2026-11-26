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
		Use:                   "fmt [--check] [路径…]",
		Short:                 "把设计文件和录制文件改写成规范格式；带 --check 时只报告，不改写。不给路径时处理 design/ 和 corpus/ 下的全部文件",
		DisableFlagsInUseLine: true,
		RunE: func(_ *cobra.Command, paths []string) error {
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			return runFmt(root, g.format, check, paths, stdout, stderr)
		},
	}
	c.Flags().BoolVar(&check, "check", false, "只报告不是规范格式的文件，不改写")
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
	report := fmtReport{Files: []fileReport{}, Diagnostics: []diagnosticJSON{}}
	var diags []diag.Diagnostic
	for _, rel := range paths {
		status, ds, err := formatFile(root, rel, check)
		if err != nil {
			return &exitError{code: exitUnexpected, err: err}
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
	var out, errOut []byte
	if format == "json" {
		if out, err = json.MarshalIndent(report, "", "  "); err != nil {
			return &exitError{code: exitUnexpected, err: errors.New("写不出 JSON 报告")}
		}
		out = append(out, '\n')
	} else {
		for _, f := range report.Files {
			if f.Status == statusRewritten || f.Status == statusNotCanonical {
				out = fmt.Appendln(out, f.Path)
			}
		}
		for _, d := range diags {
			errOut = fmt.Appendf(errOut, "%s:%d:%d: %s: %s（修正：%s）\n", d.Position.Path, d.Position.Line, d.Position.Column, d.Code, d.Message, d.Fix)
		}
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
			return nil, usageError("路径 %s 不是一个可用的路径", a)
		}
		rel, err := filepath.Rel(root, abs)
		top, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		if err != nil || top != "design" && top != "corpus" {
			return nil, usageError("路径 %s 不在仓库 %s 的 design/ 或 corpus/ 之内", a, root)
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, usageError("路径 %s 不存在", a)
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
			cause, _ := fileProblem(err)
			return nil, &exitError{code: exitUnexpected, err: fmt.Errorf("读不出 %s 下的文件：%s", top, cause)}
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
			Message: "design/ 和 corpus/ 中只能有 .uc 文件，这个文件不是",
			Fix:     "把这个文件移出 design/ 和 corpus/，或给它改名",
		}}, nil
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	src, err := os.ReadFile(full)
	if err != nil {
		cause, fix := fileProblem(err)
		return statusInvalid, []diag.Diagnostic{{
			Code: "cli.unreadable-file", Subject: rel, Position: at,
			Message: "读不出这个文件：" + cause, Fix: fix,
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
	if err := renameio.WriteFile(full, out, 0o644, renameio.WithExistingPermissions()); err != nil {
		cause, _ := fileProblem(err)
		return "", nil, fmt.Errorf("写不进 %s：%s", rel, cause)
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

// fileProblem names in Chinese why a file could not be read or written, and the fix.
func fileProblem(err error) (string, string) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "文件不存在，可能在处理之前被删除或移走了", "重新运行 unicode fmt"
	case errors.Is(err, fs.ErrPermission):
		return "没有读写这个文件的权限", "给这个文件和它所在的目录加上读写权限"
	}
	return "文件系统报告了错误", "检查这个文件是否是普通文件、能否读写，或删去它"
}

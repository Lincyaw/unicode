// Command unicode is the toolchain's command line (docs/design.md §8). Standard
// output carries data only; diagnostics and errors go to standard error.
//
// Design: design/cli.uc.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	exitOK          = 0
	exitUnexpected  = 1
	exitUsage       = 2
	exitNotFound    = 3
	exitEnvironment = 7
	exitDiagnostics = 10
)

// exitCodes are listed in the usage.
var exitCodes = []struct {
	code    int
	meaning string
}{{exitOK, "success"}, {exitUnexpected, "unexpected error"}, {exitUsage, "usage error"}, {exitNotFound, "no such identity"},
	{exitEnvironment, "environment error"}, {exitDiagnostics, "diagnostics found"}}

func main() {
	os.Exit(Main(os.Args[1:]))
}

// Main runs one command and returns its exit code.
//
//uc:realizes cli.main
func Main(args []string) int {
	root := rootCommand(os.Stdout, os.Stderr)
	root.SetArgs(append([]string{}, args...))
	err := root.Execute()
	var exit *exitError
	switch {
	case err == nil:
		return exitOK
	case !errors.As(err, &exit):
		// Every error of ours is an exitError; one from cobra or pflag is about the arguments.
		exit = &exitError{code: exitUsage, err: err}
	}
	if exit.err == nil {
		return exit.code
	}
	hint := ""
	if exit.code == exitUsage {
		hint = " (run 'unicode --help' for usage)"
	}
	return write(os.Stderr, fmt.Appendf(nil, "unicode: %v%s\n", exit.err, hint), exit.code)
}

// exitError ends a command with code; err, when set, is reported on standard error.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return fmt.Sprintf("exit code %d: %v", e.code, e.err) }

func usageError(format string, args ...any) error {
	return &exitError{code: exitUsage, err: fmt.Errorf(format, args...)}
}

// write writes out and returns code, or exitUnexpected when the write fails:
// a command whose output is lost has not done its job.
func write(w io.Writer, out []byte, code int) int {
	if _, err := w.Write(out); err != nil {
		return exitUnexpected
	}
	return code
}

// globalOptions are the flags that every command takes.
type globalOptions struct {
	root   string
	format string
	rev    string
}

// readsRev marks the commands that take --rev: the queries.
const readsRev = "reads-rev"

func rootCommand(stdout, stderr io.Writer) *cobra.Command {
	g := &globalOptions{}
	root := &cobra.Command{
		Use:               "unicode",
		Short:             "unicode builds one graph of a repository's design intent, code and evidence",
		SilenceUsage:      true,
		SilenceErrors:     true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			if g.format != "text" && g.format != "json" {
				return usageError("--format must be text or json, not %q", g.format)
			}
			if _, reads := c.Annotations[readsRev]; g.rev != "" && !reads {
				return usageError("--rev applies only to the queries node, slice, impact and view, not to %s", c.CommandPath())
			}
			return nil
		},
		RunE: func(*cobra.Command, []string) error { return usageError("missing command") },
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	codes := make([]string, len(exitCodes))
	for i, e := range exitCodes {
		codes[i] = fmt.Sprintf("%d %s", e.code, e.meaning)
	}
	root.SetUsageTemplate(root.UsageTemplate() + "\nExit codes: " + strings.Join(codes, ", ") + ".\n")
	flags := root.PersistentFlags()
	flags.StringVar(&g.root, "root", "", "repository root (default: the nearest ancestor of the working directory that contains design/system.uc)")
	flags.StringVar(&g.format, "format", "text", "output format: text or json")
	flags.StringVar(&g.rev, "rev", "", "let a query read the graph of this commit instead of the working tree")
	root.AddCommand(fmtCommand(g, stdout, stderr), checkCommand(g, stdout, stderr))
	return root
}

// findRoot returns the repository root: the given directory, or else the
// nearest ancestor of the working directory that contains design/system.uc.
func findRoot(given string) (string, error) {
	if given != "" {
		abs, err := filepath.Abs(given)
		if err != nil {
			return "", usageError("--root %s: %v", given, err)
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return "", usageError("--root %s is not a directory", given)
		}
		return abs, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", usageError("cannot read the working directory: %v", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "design", "system.uc")); err == nil && !info.IsDir() {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", usageError("no ancestor of %s contains design/system.uc; give the repository root with --root", wd)
		}
	}
}

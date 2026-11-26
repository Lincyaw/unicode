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

	"github.com/spf13/cobra"
)

const (
	exitOK          = 0
	exitUnexpected  = 1
	exitUsage       = 2
	exitDiagnostics = 10
)

const usageTemplate = `用法：
  {{.UseLine}}{{if .HasAvailableSubCommands}} <命令>{{end}}{{if .HasAvailableSubCommands}}

命令：{{range .Commands}}{{if .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

参数：
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局参数：
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}

退出码：0 成功；1 未知错误；2 参数错误；10 存在诊断
`

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
		// cobra's own errors are about the arguments.
		exit = &exitError{code: exitUsage, err: err}
	}
	if exit.err == nil {
		return exit.code
	}
	hint := ""
	if exit.code == exitUsage {
		hint = "（运行 unicode --help 查看用法）"
	}
	return write(os.Stderr, fmt.Appendf(nil, "unicode：%v%s\n", exit.err, hint), exit.code)
}

// exitError ends a command with code; err, when set, is reported on standard error.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return fmt.Sprintf("退出码 %d：%v", e.code, e.err) }

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

type options struct {
	root   string
	format string
	check  bool
}

func rootCommand(stdout, stderr io.Writer) *cobra.Command {
	var o options
	root := &cobra.Command{
		Use:                   "unicode [--root DIR] [--format text|json]",
		Short:                 "把设计意图、代码和证据组织成一张图的工具链",
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		SilenceErrors:         true,
		CompletionOptions:     cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if o.format != "text" && o.format != "json" {
				return usageError("--format 只能是 text 或 json，不能是 %s", o.format)
			}
			return nil
		},
		RunE: func(*cobra.Command, []string) error { return usageError("缺少命令") },
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetUsageTemplate(usageTemplate)
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	root.PersistentFlags().BoolP("help", "h", false, "显示本说明")
	root.PersistentFlags().StringVar(&o.root, "root", "", "仓库根目录，缺省为从当前目录向上找到的第一个含 design/system.uc 的目录")
	root.PersistentFlags().StringVar(&o.format, "format", "text", "输出格式：text 或 json")

	format := &cobra.Command{
		Use:                   "fmt [--check] [path...]",
		DisableFlagsInUseLine: true,
		Short:                 "把设计文件和录制文件改写成规范格式；带 --check 时只报告，不改写。不给路径时处理 design/ 和 corpus/ 下的全部文件",
		RunE: func(_ *cobra.Command, paths []string) error {
			dir, err := findRoot(o.root)
			if err != nil {
				return err
			}
			return runFmt(dir, o, paths, stdout, stderr)
		},
	}
	format.Flags().BoolVar(&o.check, "check", false, "只报告不是规范格式的文件，不改写")
	root.AddCommand(format)
	return root
}

// findRoot returns the repository root: the given directory, or else the
// nearest ancestor of the working directory that contains design/system.uc.
func findRoot(given string) (string, error) {
	if given != "" {
		abs, err := filepath.Abs(given)
		if err != nil {
			return "", usageError("--root %s 不可用：%v", given, err)
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return "", usageError("--root %s 不是一个目录", given)
		}
		return abs, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", usageError("读不到当前目录：%v", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "design", "system.uc")); err == nil && !info.IsDir() {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", usageError("从 %s 向上找不到含 design/system.uc 的目录；用 --root 指定仓库根目录", wd)
		}
	}
}

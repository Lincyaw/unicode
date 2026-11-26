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

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	exitOK          = 0
	exitUnexpected  = 1
	exitUsage       = 2
	exitDiagnostics = 10
)

// exitCodes are listed in the usage.
var exitCodes = []struct {
	code    int
	meaning string
}{{exitOK, "成功"}, {exitUnexpected, "未知错误"}, {exitUsage, "参数错误"}, {exitDiagnostics, "存在诊断"}}

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
		// Every error of ours is an exitError; one from cobra is about the arguments.
		exit = &exitError{code: exitUsage, err: errors.New("参数有误")}
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

// globalOptions are the flags that every command takes.
type globalOptions struct {
	root   string
	format string
}

func rootCommand(stdout, stderr io.Writer) *cobra.Command {
	g := &globalOptions{}
	root := &cobra.Command{
		Use:                   "unicode [--root 目录] [--format 格式]",
		Short:                 "把设计意图、代码和证据组织成一张图的工具链",
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		CompletionOptions:     cobra.CompletionOptions{DisableDefaultCmd: true},
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError("没有 %s 这个命令", args[0])
			}
			return nil
		},
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if g.format != "text" && g.format != "json" {
				return usageError("--format 只能是 text 或 json，不能是 %s", g.format)
			}
			return nil
		},
		RunE: func(*cobra.Command, []string) error { return usageError("缺少命令") },
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetUsageFunc(usage)
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	root.SetFlagErrorFunc(flagError)
	flags := root.PersistentFlags()
	flags.BoolP("help", "h", false, "显示本说明")
	flags.StringVar(&g.root, "root", "", "仓库根`目录`，缺省为从当前目录向上找到的第一个含 design/system.uc 的目录")
	flags.StringVar(&g.format, "format", "text", "输出的`格式`：text 或 json，缺省为 text")
	root.AddCommand(fmtCommand(g, stdout, stderr))
	return root
}

// usage writes a command's usage in Chinese, with the flags drawn by flagUsages.
func usage(c *cobra.Command) error {
	var b strings.Builder
	b.WriteString("用法：\n  " + c.UseLine())
	if c.HasAvailableSubCommands() {
		b.WriteString(" <命令>\n\n命令：\n")
		for _, sub := range c.Commands() {
			if sub.IsAvailableCommand() {
				fmt.Fprintf(&b, "  %-*s %s\n", c.NamePadding(), sub.Name(), sub.Short)
			}
		}
	} else {
		b.WriteString("\n")
	}
	if c.HasAvailableLocalFlags() {
		b.WriteString("\n参数：\n" + flagUsages(c.LocalFlags()))
	}
	if c.HasAvailableInheritedFlags() {
		b.WriteString("\n全局参数：\n" + flagUsages(c.InheritedFlags()))
	}
	codes := make([]string, len(exitCodes))
	for i, e := range exitCodes {
		codes[i] = fmt.Sprintf("%d %s", e.code, e.meaning)
	}
	b.WriteString("\n退出码：" + strings.Join(codes, "；") + "\n")
	_, err := io.WriteString(c.OutOrStderr(), b.String())
	return err
}

// flagWidths aligns the flag column by display width, for placeholders in Chinese.
var flagWidths = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: true}

// flagUsages lists flags one per line: the flag with its placeholder, then its description.
func flagUsages(flags *pflag.FlagSet) string {
	var lefts, texts []string
	width := 0
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		placeholder, text := pflag.UnquoteUsage(f)
		left := "      --" + f.Name
		if f.Shorthand != "" {
			left = "  -" + f.Shorthand + ", --" + f.Name
		}
		if placeholder != "" {
			left += " " + placeholder
		}
		lefts, texts = append(lefts, left), append(texts, text)
		width = max(width, flagWidths.StringWidth(left))
	})
	var b strings.Builder
	for i, left := range lefts {
		b.WriteString(left + strings.Repeat(" ", width-flagWidths.StringWidth(left)+3) + texts[i] + "\n")
	}
	return b.String()
}

// flagError restates pflag's errors in Chinese.
func flagError(_ *cobra.Command, err error) error {
	var notExist *pflag.NotExistError
	var required *pflag.ValueRequiredError
	var invalid *pflag.InvalidValueError
	var syntax *pflag.InvalidSyntaxError
	switch {
	case errors.As(err, &notExist):
		return usageError("没有 %s 这个参数", flagName(notExist.GetSpecifiedName(), notExist.GetSpecifiedShortnames()))
	case errors.As(err, &required):
		return usageError("参数 %s 需要一个取值", flagName(required.GetSpecifiedName(), required.GetSpecifiedShortnames()))
	case errors.As(err, &invalid):
		return usageError("参数 --%s 不能取 %q", invalid.GetFlag().Name, invalid.GetValue())
	case errors.As(err, &syntax):
		return usageError("参数 %s 的写法不对", syntax.GetSpecifiedFlag())
	}
	return usageError("参数有误")
}

func flagName(name, shorthands string) string {
	if shorthands != "" {
		return "-" + name
	}
	return "--" + name
}

// findRoot returns the repository root: the given directory, or else the
// nearest ancestor of the working directory that contains design/system.uc.
func findRoot(given string) (string, error) {
	if given != "" {
		abs, err := filepath.Abs(given)
		if err != nil {
			return "", usageError("--root %s 不是一个可用的路径", given)
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return "", usageError("--root %s 不是一个目录", given)
		}
		return abs, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", usageError("读不到当前目录")
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

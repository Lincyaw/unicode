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
	"slices"
	"strings"
)

const (
	exitOK          = 0
	exitUnexpected  = 1
	exitUsage       = 2
	exitDiagnostics = 10
)

const usage = `用法：unicode [--root DIR] [--format text|json] <命令> [参数]

命令：
  fmt [--check] [path...]  把设计文件和录制文件改写成规范格式；带 --check 时只报告，不改写。
                           不给路径时处理 design/ 和 corpus/ 下的全部文件；路径可以是目录。

其余命令（check、build、gen、test、diff、node、slice、impact、view、schema）尚未实现。

全局参数（也可以写在命令之后）：
  --root DIR          仓库根目录，缺省为从当前目录向上找到的第一个含 design/system.uc 的目录
  --format text|json  输出格式，缺省为 text
  --help              显示本说明

退出码：0 成功；1 未知错误；2 参数错误；10 存在诊断
`

var commands = []string{"check", "build", "gen", "test", "diff", "node", "slice", "impact", "view", "schema"}

func main() {
	os.Exit(Main(os.Args[1:]))
}

// Main runs one command and returns its exit code.
//
//uc:realizes cli.main
func Main(args []string) int {
	o, err := parseArgs(args)
	if err != nil {
		return usageError(os.Stderr, err)
	}
	if o.help {
		return write(os.Stdout, []byte(usage), exitOK)
	}
	if o.command != "fmt" {
		return usageError(os.Stderr, fmt.Errorf("命令 %s 尚未实现", o.command))
	}
	root, err := findRoot(o.root)
	if err != nil {
		return usageError(os.Stderr, err)
	}
	return runFmt(root, o, os.Stdout, os.Stderr)
}

func usageError(stderr io.Writer, err error) int {
	return write(stderr, fmt.Appendf(nil, "unicode：%v（运行 unicode --help 查看用法）\n", err), exitUsage)
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
	root    string
	format  string
	command string
	check   bool
	help    bool
	paths   []string
}

// parseArgs reads `[global flags] <command> [flags and paths]`; the global
// flags may also follow the command.
func parseArgs(args []string) (options, error) {
	o := options{format: "text"}
	onlyPaths := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if onlyPaths || !strings.HasPrefix(a, "-") || a == "-" {
			switch o.command {
			case "":
				o.command = a
			case "fmt":
				o.paths = append(o.paths, a)
			default:
				return o, fmt.Errorf("多余的参数 %s", a)
			}
			continue
		}
		name, value, hasValue := strings.Cut(a, "=")
		takeValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("参数 %s 缺少取值", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case name == "--":
			onlyPaths = o.command != ""
			if !onlyPaths {
				return o, errors.New("-- 之前缺少命令")
			}
		case name == "--help" || name == "-h":
			o.help = true
		case name == "--root":
			o.root, err = takeValue()
		case name == "--format":
			o.format, err = takeValue()
			if err == nil && o.format != "text" && o.format != "json" {
				err = fmt.Errorf("--format 只能是 text 或 json，不能是 %s", o.format)
			}
		case name == "--check" && o.command == "fmt" && !hasValue:
			o.check = true
		default:
			err = fmt.Errorf("未知的参数 %s", a)
		}
		if err != nil {
			return o, err
		}
	}
	switch {
	case o.help:
	case o.command == "":
		return o, errors.New("缺少命令")
	case o.command != "fmt" && !slices.Contains(commands, o.command):
		return o, fmt.Errorf("未知的命令 %s", o.command)
	}
	return o, nil
}

// findRoot returns the repository root: the given directory, or else the
// nearest ancestor of the working directory that contains design/system.uc.
func findRoot(given string) (string, error) {
	if given != "" {
		abs, err := filepath.Abs(given)
		if err != nil {
			return "", fmt.Errorf("--root %s 不可用：%v", given, err)
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return "", fmt.Errorf("--root %s 不是一个目录", given)
		}
		return abs, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("读不到当前目录：%v", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "design", "system.uc")); err == nil && !info.IsDir() {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("从 %s 向上找不到含 design/system.uc 的目录；用 --root 指定仓库根目录", wd)
		}
	}
}

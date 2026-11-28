package golang

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Go's coverage tool is invoked with original file names even under -overlay.
// Forwarding its inputs through that same map makes trace and coverage measure
// the same source, without replacing compiler internals or touching the module.
// The map's path and its fingerprint come from the environment of the build
// (coverToolEnvironment), so the tool's source never changes and its binary is
// built once per toolchain and then copied from the content cache.
const coverToolSource = `package main
import("encoding/json";"fmt";"os";"os/exec";"path/filepath";"strings")
func main(){
 args:=os.Args[1:];if len(args)==0{fmt.Fprintln(os.Stderr,"missing Go tool");os.Exit(2)}
 if strings.TrimSuffix(filepath.Base(args[0]),".exe")=="cover"{
  path,fingerprint:=os.Getenv("UNICODE_COVER_OVERLAY"),os.Getenv("UNICODE_COVER_FINGERPRINT")
  if path==""||fingerprint==""{fmt.Fprintln(os.Stderr,"the coverage overlay is not set");os.Exit(2)}
  if len(args)==2&&args[1]=="-V=full" {output,err:=exec.Command(args[0],args[1:]...).Output();if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)};fmt.Printf("%s unicodeOverlay=%s\n",strings.TrimSpace(string(output)),fingerprint);return}
  data,err:=os.ReadFile(path);if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)}
  var overlay struct{Replace map[string]string};if err=json.Unmarshal(data,&overlay);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)}
  for i:=1;i<len(args);i++{if replacement,ok:=overlay.Replace[args[i]];ok{args[i]=replacement}}
 }
 cmd:=exec.Command(args[0],args[1:]...);cmd.Stdin=os.Stdin;cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;cmd.Env=os.Environ()
 if err:=cmd.Run();err!=nil{if status,ok:=err.(*exec.ExitError);ok{os.Exit(status.ExitCode())};fmt.Fprintln(os.Stderr,err);os.Exit(2)}
}`

// coverToolEnvironment is what the tool reads: the overlay's path, and a
// fingerprint of it that enters the coverage tool's version, so Go's build
// cache never reuses a package instrumented under another overlay.
func (p *measuredProgram) coverToolEnvironment() []string {
	return []string{"UNICODE_COVER_OVERLAY=" + p.overlay, fmt.Sprintf("UNICODE_COVER_FINGERPRINT=%x", sha256.Sum256([]byte(p.overlay)))}
}

// prepareTool puts the tool's binary in the run's temporary directory,
// copied from the content cache when this toolchain has built it before.
func (p *measuredProgram) prepareTool(ctx context.Context) error {
	const kind = cacheTool
	p.tool = filepath.Join(p.temp, "cover-overlay")
	if runtime.GOOS == "windows" {
		p.tool += ".exe"
	}
	cache := sharedCache()
	key, keyed := coverToolKey(p.root)
	if keyed && cache.copyEntry(kind, key, p.tool) {
		return nil
	}
	path := filepath.Join(p.temp, "cover-overlay.go")
	if err := os.WriteFile(path, []byte(coverToolSource), 0600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-o", p.tool, path)
	cmd.Dir = p.root
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build coverage overlay boundary: %w: %s", err, output)
	}
	if keyed {
		cache.putFile(kind, key, p.tool)
	}
	return nil
}

// coverToolKey is the tool's source with the settings of the go command
// that builds it in root.
func coverToolKey(root string) (string, bool) {
	env, ok := goEnvironment(root)
	if !ok {
		return "", false
	}
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "unicode-cover-tool\n1\n%s\n", coverToolSource)
	for _, name := range goEnvironmentNames {
		_, _ = fmt.Fprintf(h, "%s=%s\n", name, env[name])
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

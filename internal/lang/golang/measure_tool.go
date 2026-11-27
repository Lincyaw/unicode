package golang

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

// Go's coverage tool is invoked with original file names even under -overlay.
// Forwarding its inputs through that same map makes trace and coverage measure
// the same source, without replacing compiler internals or touching the module.
func (p *measuredProgram) prepareTool(ctx context.Context) error {
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(p.overlay)))
	source := `package main
import("encoding/json";"fmt";"os";"os/exec";"path/filepath";"strings")
func main(){
 args:=os.Args[1:];if len(args)==0{fmt.Fprintln(os.Stderr,"missing Go tool");os.Exit(2)}
 if strings.TrimSuffix(filepath.Base(args[0]),".exe")=="cover"{
  if len(args)==2&&args[1]=="-V=full" {output,err:=exec.Command(args[0],args[1:]...).Output();if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)};fmt.Printf("%s unicodeOverlay=%s\n",strings.TrimSpace(string(output)),` + strconv.Quote(fingerprint) + `);return}
  data,err:=os.ReadFile(` + strconv.Quote(p.overlay) + `);if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)}
  var overlay struct{Replace map[string]string};if err=json.Unmarshal(data,&overlay);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)}
  for i:=1;i<len(args);i++{if replacement,ok:=overlay.Replace[args[i]];ok{args[i]=replacement}}
 }
 cmd:=exec.Command(args[0],args[1:]...);cmd.Stdin=os.Stdin;cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;cmd.Env=os.Environ()
 if err:=cmd.Run();err!=nil{if status,ok:=err.(*exec.ExitError);ok{os.Exit(status.ExitCode())};fmt.Fprintln(os.Stderr,err);os.Exit(2)}
}`
	path := filepath.Join(p.temp, "cover-overlay.go")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		return err
	}
	p.tool = filepath.Join(p.temp, "cover-overlay")
	if runtime.GOOS == "windows" {
		p.tool += ".exe"
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-o", p.tool, path)
	cmd.Dir = p.root
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build coverage overlay boundary: %w: %s", err, output)
	}
	return nil
}

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"pgregory.net/rapid"
)

// served is what unicode serve did: its exit code and its two outputs.
type served struct {
	code           int
	stdout, stderr string
}

// startServe runs unicode serve on a free port in the background, its
// outputs going to files in dir; the channel gives its exit code.
func startServe(t rapid.TB, root, dir string) (chan int, *os.File, *os.File) {
	stdout, err1 := os.Create(filepath.Join(dir, "stdout"))
	stderr, err2 := os.Create(filepath.Join(dir, "stderr"))
	if err1 != nil || err2 != nil {
		t.Fatalf("setup failed: cannot create the output files: %v %v", err1, err2)
	}
	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	exit := make(chan int, 1)
	go func() {
		defer func() { os.Stdout, os.Stderr = savedOut, savedErr }()
		exit <- Main([]string{"--root", root, "--format", "json", "serve", "--addr", "127.0.0.1:0"})
	}()
	return exit, stdout, stderr
}

// address waits for serve to write its address, or to exit first.
func address(t rapid.TB, stdout *os.File, exit chan int) (string, *int) {
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		select {
		case code := <-exit:
			return "", &code
		default:
		}
		data, err := os.ReadFile(stdout.Name())
		var report serveReport
		if err == nil && json.Unmarshal(data, &report) == nil && report.URL != "" {
			return report.URL, nil
		}
	}
	t.Fatalf("unicode serve wrote no address on stdout within a minute, and did not exit")
	return "", nil
}

func finish(t rapid.TB, code int, stdout, stderr *os.File) served {
	out, err1 := readClosing(stdout)
	errOut, err2 := readClosing(stderr)
	if err1 != nil || err2 != nil {
		t.Fatalf("setup failed: cannot read the captured output: %v %v", err1, err2)
	}
	return served{code: code, stdout: string(out), stderr: string(errOut)}
}

//uc:checks cli.serve-viewer
func checkFlowServeViewer(t rapid.TB, files map[string]RepoFile, built bool) {
	root := writeRepo(t, files)
	defer removeAll(t, filepath.Dir(root))
	nodes := 0
	if built {
		direct := buildDirectly(t, root, files)
		if len(direct.diags) > 0 {
			t.Fatalf("setup failed: the drawn repository is not legal, building it gives %+v", direct.diags)
		}
		nodes = len(direct.graph.Nodes())
		if code, stdout, stderr := runMain(t, []string{"--root", root, "build"}); code != exitOK {
			t.Fatalf("setup failed: unicode build exited with %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	}
	dir, err := os.MkdirTemp("", "unicode-serve-out-")
	if err != nil {
		t.Fatalf("setup failed: cannot create a temporary directory: %v", err)
	}
	defer removeAll(t, dir)
	exit, stdout, stderr := startServe(t, root, dir)
	url, exited := address(t, stdout, exit)
	if !built {
		got := finish(t, *exited, stdout, stderr)
		if got.code != exitEnvironment || got.stdout != "" || !strings.Contains(got.stderr, "unicode build") {
			t.Fatalf("without a snapshot, unicode serve should exit with %d and say on stderr to run unicode build, yet it exited with %d\nstdout: %s\nstderr: %s",
				exitEnvironment, got.code, got.stdout, got.stderr)
		}
		return
	}
	if exited != nil {
		got := finish(t, *exited, stdout, stderr)
		t.Fatalf("unicode serve exited with %d before it served the snapshot that unicode build wrote\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
	resp, err := http.Get(url + "api/snapshot")
	if err != nil {
		t.Fatalf("unicode serve wrote the address %s, yet GET /api/snapshot fails: %v", url, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var snapshot struct{ Counts struct{ Nodes int } }
	if err != nil || resp.StatusCode != http.StatusOK || json.Unmarshal(body, &snapshot) != nil || snapshot.Counts.Nodes != nodes {
		t.Fatalf("GET /api/snapshot should answer 200 with the %d nodes of the snapshot, yet it got %d: %v\n%s", nodes, resp.StatusCode, err, body)
	}
	post, err := http.Post(url+"api/snapshot", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /api/snapshot fails: %v", err)
	}
	_ = post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("the interface is read-only, so POST /api/snapshot should get 405, yet it got %d", post.StatusCode)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("setup failed: cannot interrupt unicode serve: %v", err)
	}
	var code int
	select {
	case code = <-exit:
	case <-time.After(time.Minute):
		t.Fatalf("unicode serve did not exit within a minute of an interrupt")
	}
	got := finish(t, code, stdout, stderr)
	if got.code != exitOK || !strings.Contains(got.stderr, "GET /api/snapshot 200") || !strings.Contains(got.stderr, "POST /api/snapshot 405") {
		t.Fatalf("after an interrupt unicode serve should exit with %d, having logged each request on stderr, yet it exited with %d\nstderr: %s", exitOK, got.code, got.stderr)
	}
}

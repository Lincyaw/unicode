package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/serve"
	"github.com/lincyaw/unicode/monitor"
)

// serveCommand is `unicode serve [--snapshot FILE] [--addr HOST:PORT] [--web DIR]`.
func serveCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	snapshot, addr, web := "", "127.0.0.1:8765", ""
	c := &cobra.Command{
		Use:   "serve",
		Short: "Serve the viewer: its interface from the snapshot that build wrote, and its pages",
		Long: "serve reads the snapshot that unicode build wrote, checks it as check does, and answers the viewer's\n" +
			"read-only interface (api/openapi.yaml) under /api; when the snapshot file changes, the next request reads\n" +
			"it again. Under / it serves the viewer's pages that make build built into unicode, or, for developing the\n" +
			"frontend, those of the directory --web names. A unicode built without pages, as a plain go build is,\n" +
			"serves the interface only and says so on standard error. Once it listens, standard output gives the\n" +
			"address; each request is logged on standard error. An interrupt stops it.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			path := filepath.Join(root, ".unicode", "graph.json")
			if snapshot != "" {
				if path, err = filepath.Abs(snapshot); err != nil {
					return usageError("--snapshot %s: %v", snapshot, err)
				}
			}
			return runServe(root, path, addr, pageDir{web, c.Flags().Changed("web")}, g.format, stdout, stderr)
		},
	}
	c.Flags().StringVar(&snapshot, "snapshot", "", "the snapshot to serve (default: .unicode/graph.json under the root)")
	c.Flags().StringVar(&addr, "addr", addr, "the address to listen on, host:port")
	c.Flags().StringVar(&web, "web", "", "a directory of built pages to serve instead of those built into unicode")
	return c
}

// pageDir is --web, and whether it was given.
type pageDir struct {
	dir   string
	given bool
}

// pagesOf is the pages to serve: the directory --web names, which must
// exist, or else the pages built into unicode. When unicode carries none,
// it is nil, and the note tells why and how to get them.
func pagesOf(web pageDir) (fs.FS, string, error) {
	if web.given {
		if info, err := os.Stat(web.dir); err != nil || !info.IsDir() {
			return nil, "", usageError("--web %s is not a directory", web.dir)
		}
		return os.DirFS(web.dir), "", nil
	}
	if pages := serve.Pages(); pages != nil {
		return pages, "", nil
	}
	return nil, "unicode: no pages, since this unicode was built without them; serving the interface under /api only. " +
		"Build unicode with make build, which builds the pages into it, or give built pages with --web DIR\n", nil
}

// serveReport is the JSON of serve: the address of the pages.
type serveReport struct {
	URL string `json:"url"`
}

// runServe finds the pages, opens the snapshot, listens on addr, writes the
// address and serves until an interrupt. A snapshot that cannot be read and
// an address that cannot be listened on are environment errors.
func runServe(root, snapshot, addr string, web pageDir, format string, stdout, stderr io.Writer) error {
	pages, note, err := pagesOf(web)
	if err != nil {
		return err
	}
	// A monitored build judges the comparisons' graphs (docs/design.md §6.6).
	s, err := serve.Open(root, snapshot, monitor.Wrap[serve.Builder](CommitGraphs{}))
	if err != nil {
		return &exitError{code: exitEnvironment, err: fmt.Errorf("%w; run unicode build to write the snapshot", err)}
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return &exitError{code: exitEnvironment, err: fmt.Errorf("cannot listen on %s: %w", addr, err)}
	}
	// The interrupt is caught before the address is out, so that whoever reads the address can stop serve.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	url := "http://" + ln.Addr().String() + "/"
	if _, err := io.WriteString(stderr, note); err != nil {
		_ = ln.Close()
		return &exitError{code: exitUnexpected}
	}
	if err := writeResult(format, serveReport{URL: url}, fmt.Appendln(nil, url), stdout); err != nil {
		_ = ln.Close()
		return err
	}
	server := &http.Server{Handler: logRequests(s.Handler(pages), stderr), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return &exitError{code: exitUnexpected, err: fmt.Errorf("serving on %s failed: %w", addr, err)}
	}
	return nil
}

// logRequests writes a line for each request to log: its method, address,
// status and duration.
func logRequests(h http.Handler, log io.Writer) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(sw, r)
		mu.Lock()
		defer mu.Unlock()
		_, _ = fmt.Fprintf(log, "%s %s %d %s\n", r.Method, r.URL.RequestURI(), sw.status, time.Since(start).Round(time.Microsecond))
	})
}

// statusWriter keeps the status a handler writes.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// CommitGraphs builds the graph of a commit for serve's comparisons, as
// unicode diff builds its base.
//
//uc:realizes cli.CommitGraphs
type CommitGraphs struct{}

func (CommitGraphs) Build(root, commit string) (graph.Graph, error) {
	g, _, err := revisionGraph(root, commit)
	return g, err
}

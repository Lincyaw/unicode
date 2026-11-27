package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	"github.com/lincyaw/unicode/internal/graph"
)

// Unreadable is the error of Open for a snapshot file that does not exist,
// cannot be read, or is not a snapshot unicode build wrote.
//
//uc:realizes serve.Unreadable
type Unreadable struct {
	Path  string
	Cause error
}

func (e Unreadable) Error() string {
	return fmt.Sprintf("cannot read the snapshot %s: %v", e.Path, e.Cause)
}

func (e Unreadable) Unwrap() error { return e.Cause }

// Unbuildable is the error of building the graph of a base commit: its
// files cannot be taken out, or they build no graph.
//
//uc:realizes serve.Unbuildable
type Unbuildable struct {
	Commit string
	Cause  error
}

func (e Unbuildable) Error() string {
	return fmt.Sprintf("cannot build the graph of the commit %s: %v", e.Commit, e.Cause)
}

func (e Unbuildable) Unwrap() error { return e.Cause }

// Builder builds the graph of the repository at root as it is at a commit,
// as a query's --rev does: the command line provides it.
//
//uc:realizes serve.Builder
type Builder interface {
	Build(root, commit string) (graph.Graph, error)
}

// Request is one request of the interface: its path, which starts with
// /api, and its query parameters.
//
//uc:realizes serve.Request
type Request struct {
	Path  string
	Query map[string][]string
}

// Response is the answer to one request: its status code and JSON body.
//
//uc:realizes serve.Response
type Response struct {
	Status int
	Body   []byte
}

// Server holds the snapshot it read and the result of checking it; when the
// snapshot file changes, the next request reads it again. Each request is
// answered from one snapshot, the one current when it arrived.
//
//uc:realizes serve.Server
type Server struct {
	root, path string
	api        http.Handler
	builder    Builder
	// bases are the indexed graphs of the base commits built so far, which
	// do not change.
	bases sync.Map

	mu sync.Mutex
	// read is the snapshot last read, and stamp the file's state then;
	// broken, when set, is why the file cannot be read now.
	read   *state
	stamp  stamp
	broken error
}

// stamp tells two states of a file apart without reading it.
type stamp struct {
	modTime time.Time
	size    int64
}

// Open reads the snapshot file of the repository at root and checks it as
// unicode check does; builder builds the graphs of the commits it is
// compared with.
//
//uc:realizes serve.open
func Open(root, snapshot string, builder Builder) (*Server, error) {
	s := &Server{root: root, path: snapshot, builder: builder}
	if _, err := s.current(); err != nil {
		return nil, err
	}
	strict := NewStrictHandlerWithOptions(answers{s}, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badParameter,
		ResponseErrorHandlerFunc: writeError,
	})
	mux := http.NewServeMux()
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseURL: "/api", BaseRouter: mux, ErrorHandlerFunc: badParameter})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, refuse(http.StatusNotFound, "serve.not-found", "see api/openapi.yaml for the endpoints under /api",
			"the interface has no endpoint %s", r.URL.Path))
	})
	s.api = mux
	return s, nil
}

// current returns the snapshot to answer from, reading the file again when
// it changed since it was last read.
func (s *Server) current() (*state, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.path)
	switch {
	case err != nil:
		s.broken, s.stamp = Unreadable{Path: s.path, Cause: err}, stamp{}
	case s.read == nil || s.broken != nil || (stamp{info.ModTime(), info.Size()}) != s.stamp:
		st, err := load(s.root, s.path, info.ModTime())
		s.broken, s.stamp = err, stamp{info.ModTime(), info.Size()}
		if err == nil {
			s.read = st
		}
	}
	return s.read, s.broken
}

type stateKey struct{}

// Answer answers one request of the interface of api/openapi.yaml from the
// current snapshot. A refused request gets a {code, message, fix} body whose
// code is serve.*.
func (s *Server) Answer(request Request) Response {
	var rec recorder
	st, err := s.current()
	if err != nil {
		writeProblem(&rec, refuse(http.StatusServiceUnavailable, "serve.unreadable-snapshot",
			"run unicode build to write the snapshot again", "%v", err))
		return rec.response()
	}
	u := url.URL{Path: request.Path, RawQuery: url.Values(request.Query).Encode()}
	r, err := http.NewRequestWithContext(context.WithValue(context.Background(), stateKey{}, st), http.MethodGet, u.String(), nil)
	if err != nil {
		writeProblem(&rec, refuse(http.StatusBadRequest, "serve.bad-parameter",
			"give a path under /api and query parameters that make a URL", "%v", err))
		return rec.response()
	}
	s.api.ServeHTTP(&rec, r)
	return rec.response()
}

// Handler serves the interface under /api through Answer, and the files of
// pages, when it is not nil, everywhere else; a path that names no file gets
// index.html, which reads the rest of the address itself.
func (s *Server) Handler(pages fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeProblem(w, refuse(http.StatusMethodNotAllowed, "serve.bad-parameter", "send a GET request",
				"the interface is read-only, so it takes no %s request", r.Method))
			return
		}
		response := s.Answer(Request{Path: r.URL.Path, Query: r.URL.Query()})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Status)
		_, _ = w.Write(response.Body)
	})
	if pages != nil {
		files := http.FileServerFS(pages)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if name := path.Clean("/" + r.URL.Path)[1:]; name != "" {
				if info, err := fs.Stat(pages, name); err != nil || info.IsDir() {
					r = r.Clone(r.Context())
					r.URL.Path = "/"
				}
			}
			files.ServeHTTP(w, r)
		})
	}
	return mux
}

// recorder keeps what a handler writes, for Answer to return.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *recorder) Write(b []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return r.body.Write(b)
}

func (r *recorder) response() Response {
	return Response{Status: r.status, Body: bytes.Clone(r.body.Bytes())}
}

// problem is a refused request: its status and its body.
type problem struct {
	status int
	body   Problem
}

func (p problem) Error() string { return p.body.Message }

// refuse is a refusal with the status, the code, the fix, and the message
// that format and args give.
func refuse(status int, code, fix, format string, args ...any) problem {
	return problem{status: status, body: Problem{Code: code, Message: fmt.Sprintf(format, args...), Fix: fix}}
}

func writeProblem(w http.ResponseWriter, p problem) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(p.status)
	_ = json.NewEncoder(w).Encode(p.body)
}

// badParameter answers a request whose parameters do not bind: one missing,
// or one that is not of its type.
func badParameter(w http.ResponseWriter, _ *http.Request, err error) {
	writeProblem(w, refuse(http.StatusBadRequest, "serve.bad-parameter",
		"give every required parameter, each of the type api/openapi.yaml gives it", "%v", err))
}

// writeError answers a handler's refusal, or a failure of the server itself.
func writeError(w http.ResponseWriter, _ *http.Request, err error) {
	var p problem
	if !errors.As(err, &p) {
		p = refuse(http.StatusInternalServerError, "serve.internal-error", "report the request: the server failed to answer it", "%v", err)
	}
	writeProblem(w, p)
}

// stateOf is the snapshot that Answer put in the request's context.
func stateOf(ctx context.Context) *state { return ctx.Value(stateKey{}).(*state) }

package serve

import "fmt"

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
// snapshot file changes, the next request reads it again.
//
//uc:realizes serve.Server
type Server struct{}

// Open reads the snapshot file of the repository at root and checks it as
// unicode check does.
//
//uc:realizes serve.open
func Open(root, snapshot string) (*Server, error) {
	panic("unicode: unimplemented")
}

// Answer answers one request of the interface of api/openapi.yaml.
func (s *Server) Answer(request Request) Response {
	panic("unicode: unimplemented")
}

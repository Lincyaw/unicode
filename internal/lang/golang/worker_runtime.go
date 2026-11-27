package golang

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/trace"
	"testing"
	"time"

	"pgregory.net/rapid"
)

type ucReplayCase struct {
	Id     string      `json:"id"`
	Check  string      `json:"check"`
	Form   string      `json:"form"`
	Reason string      `json:"reason"`
	Input  ucWireInput `json:"input"`
}
type ucWorkerCase struct {
	Id, Form, Reason string
	Make             func(*ucContract) map[string]reflect.Value
}
type ucWorkerUnit struct {
	Spec   func() *ucContract
	Types  []reflect.Type
	Draw   func(*rapid.T, *ucContract) map[string]reflect.Value
	Invoke func(rapid.TB, *ucContract, map[string]reflect.Value)
	Cases  []ucWorkerCase
}
type ucWorkerRequest struct {
	Trial bool        `json:"trial"`
	Mode  string      `json:"mode"`
	Check string      `json:"check"`
	Seed  string      `json:"seed"`
	Input ucWireInput `json:"input"`
}
type ucWorkerResponse struct {
	Cases       []ucReplayCase  `json:"cases"`
	Input       ucWireInput     `json:"input"`
	Result      ucRunResult     `json:"result"`
	Rejected    bool            `json:"rejected"`
	Diagnostics []ucDiagnostic  `json:"diagnostics"`
	Durations   []time.Duration `json:"durations"`
}

var ucTraceBusinessEnabled bool

func ucBusinessTrace() func() {
	if !ucTraceBusinessEnabled {
		return func() {}
	}
	trace.Log(context.Background(), "unicode", "begin")
	return func() { trace.Log(context.Background(), "unicode", "end") }
}
func ucWorkerBits(seed string) []byte {
	out := make([]byte, 65536)
	for i := 0; i < len(out); i += sha256.Size {
		data := []byte(seed)
		var n [8]byte
		binary.LittleEndian.PutUint64(n[:], uint64(i))
		data = append(data, n[:]...)
		hash := sha256.Sum256(data)
		copy(out[i:], hash[:])
	}
	return out
}
func ucCandidateWorker(t *testing.T, units []ucWorkerUnit) (handled bool) {
	path := os.Getenv("UNICODE_CONTRACT_WORKER_REQUEST")
	if path == "" {
		return false
	}
	handled = true
	response := ucWorkerResponse{Cases: []ucReplayCase{}, Diagnostics: []ucDiagnostic{}}
	var spec *ucContract
	var request ucWorkerRequest
	defer func() {
		if problem := recover(); problem != nil {
			issue, ok := problem.(ucIssue)
			if !ok {
				issue = ucIssue{"lang.golang.input-codec-failed", fmt.Sprintf("candidate preparation failed: %v", problem), nil}
			}
			if spec == nil {
				spec = &ucContract{Path: "design/system.uc", Line: 1, Column: 1}
			}
			if request.Trial && issue.code == "lang.golang.input-outside-domain" {
				response.Rejected = true
				response.Result.Rejected = 1
				response.Result.Passed = false
			} else {
				response.Diagnostics = append(response.Diagnostics, ucDiagnosticOf(spec, issue))
			}
		}
		if spec != nil {
			response.Rejected = response.Rejected || (response.Result.Accepted == 0 && response.Result.Rejected > 0)
			response.Result.Passed = response.Result.Passed && !t.Failed() && !response.Rejected
			if response.Result.Infrastructure {
				response.Diagnostics = append(response.Diagnostics, response.Result.Diagnostics...)
			}
			response.Durations = append([]time.Duration{}, response.Result.durations...)
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Error(err)
			return
		}
		if err = os.WriteFile(os.Getenv("UNICODE_CONTRACT_WORKER_RESPONSE"), encoded, 0600); err != nil {
			t.Error(err)
		}
	}()
	data, err := os.ReadFile(path)
	if err != nil {
		ucFail("lang.golang.input-codec-failed", err.Error(), nil)
	}
	if err = json.Unmarshal(data, &request); err != nil {
		ucFail("lang.golang.input-codec-failed", err.Error(), nil)
	}
	var selected *ucWorkerUnit
	for i := range units {
		candidate := units[i].Spec()
		if candidate.ID == request.Check {
			selected = &units[i]
			spec = candidate
			break
		}
	}
	if selected == nil {
		ucFail("lang.golang.input-codec-failed", "the candidate worker does not contain the selected check", nil)
	}
	response.Result = ucRunResult{Id: spec.ID, Passed: true, Observations: append([]string{}, spec.Observe...)}
	for _, observer := range spec.Observers {
		response.Result.Observations = append(response.Result.Observations, observer.Name)
	}
	switch request.Mode {
	case "inventory":
		for _, declared := range selected.Cases {
			values := declared.Make(spec)
			input := ucFreezeInputs(spec, values)
			response.Cases = append(response.Cases, ucReplayCase{declared.Id, spec.ID, declared.Form, declared.Reason, input})
		}
	case "draw":
		complete := false
		defer func() {
			if !complete {
				response.Rejected = true
			}
		}()
		rapid.MakeFuzz(func(rt *rapid.T) { response.Input = ucFreezeInputs(spec, selected.Draw(rt, spec)); complete = true })(t, ucWorkerBits(request.Seed))
	case "prepare", "execute":
		values := ucThawInputs(request.Input, selected.Types, spec)
		response.Input = request.Input
		if request.Mode == "prepare" {
			return true
		}
		restore := ucContext(&response.Result, true)
		defer restore()
		ucTraceBusinessEnabled = true
		defer func() { ucTraceBusinessEnabled = false }()
		rapid.MakeFuzz(func(rt *rapid.T) { selected.Invoke(rt, spec, values) })(t, ucWorkerBits(ucWireHash(request.Input)))
		if response.Result.Accepted == 0 && response.Result.Rejected == 0 {
			response.Result.Infrastructure = true
			response.Result.Passed = false
			response.Result.Diagnostics = append(response.Result.Diagnostics, ucDiagnosticOf(spec, ucIssue{"lang.golang.no-effective-input", "the candidate did not execute its predicate", nil}))
		}
	default:
		ucFail("lang.golang.input-codec-failed", "the candidate worker mode is not recognized", nil)
	}
	return true
}

func ucTraceOperation(name string) func() {
	trace.Log(context.Background(), "unicode", "modelEnter:"+name)
	return func() { trace.Log(context.Background(), "unicode", "modelExit:"+name) }
}
func ucTraceError(name string) { trace.Log(context.Background(), "unicode", "modelError:"+name) }

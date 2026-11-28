package monitor

import (
	"context"
	"fmt"
	"time"
)

// The background writer writes the run records and the collected calls, so
// that no monitored call waits for a file (docs/design.md §6.6).

// job is work for the background writer: a collected call to write, or a
// request to write the period in progress, done when the work is done.
type job struct {
	collected *Collected
	flush     bool
	done      chan struct{}
}

// add keeps a judged call in the period in progress, starting the writer
// with the first one.
func (s *state) add(call JudgedCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.start()
	s.calls = append(s.calls, call)
}

// start starts the background writer once; s.mu is held.
func (s *state) start() {
	if s.started {
		return
	}
	s.started = true
	s.queue = make(chan job, 256)
	go s.write()
	go func() {
		for range time.Tick(period) {
			s.queue <- job{flush: true}
		}
	}()
}

// write is the background writer: it writes collected calls and, every
// period or on Flush, the run record of the period in progress.
func (s *state) write() {
	for j := range s.queue {
		switch {
		case j.collected != nil:
			s.writeCollected(*j.collected)
		case j.flush:
			s.writePeriod()
		}
		if j.done != nil {
			close(j.done)
		}
	}
}

// collect queues a sampled call's input for writing; a call that cannot be
// queued or frozen counts as a write failure.
func (s *state) collect(operation Operation, receiver any, args []any) {
	collected, err := collectSafely(operation, receiver, args)
	if err != nil {
		s.writeFailures.Add(context.Background(), 1)
		return
	}
	collected.Op, collected.Commit, collected.At = operation.Op, s.build.Commit, time.Now().UTC()
	s.mu.Lock()
	s.start()
	s.mu.Unlock()
	select {
	case s.queue <- job{collected: &collected}:
	default:
		s.writeFailures.Add(context.Background(), 1)
	}
}

// collectSafely collects a call, turning a panic of the collecting code into
// an error named by the panic's type.
func collectSafely(operation Operation, receiver any, args []any) (collected Collected, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("collecting the call panicked with a value of type %T", p)
		}
	}()
	return operation.Collect(receiver, args)
}

// writePeriod ends the period in progress and writes its run record when a
// property or flow judged a call in it.
func (s *state) writePeriod() {
	s.mu.Lock()
	calls, from, to := s.calls, s.from, time.Now()
	s.calls, s.from = nil, to
	s.mu.Unlock()
	if len(calls) == 0 {
		return
	}
	record := Summarize(calls, s.build, from, to)
	record.Diagnostics = append(record.Diagnostics, s.problems...)
	if err := writeRecord(s.dir, record); err != nil {
		s.writeFailures.Add(context.Background(), 1)
	}
}

// writeCollected writes one collected call, marking its operation full once
// it has maxCollected.
func (s *state) writeCollected(collected Collected) {
	full, err := writeTraffic(s.dir, collected, s.collected)
	if err != nil {
		s.writeFailures.Add(context.Background(), 1)
	}
	if full {
		s.fill(collected.Op)
	}
}

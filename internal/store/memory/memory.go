// Package memory is an in-memory store.Store for tests and zero-setup local
// runs. Its clock is injectable so a test can expire a lease without sleeping,
// which is how the resume test simulates a worker dying and another taking over.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/store"
)

type record struct {
	run        run.Run
	worker     string
	leaseUntil time.Time
}

// Store is a concurrency-safe in-memory implementation of store.Store.
type Store struct {
	mu    sync.Mutex
	recs  map[string]*record
	order []string // enqueue order, so Claim is fair (oldest first)
	now   func() time.Time
}

var _ store.Store = (*Store)(nil)

// New returns an empty store using time.Now as its clock.
func New() *Store { return NewWithClock(time.Now) }

// NewWithClock returns an empty store driven by the given clock. Tests pass a
// clock they control to expire leases deterministically.
func NewWithClock(now func() time.Time) *Store {
	return &Store{recs: map[string]*record{}, now: now}
}

func (s *Store) Enqueue(_ context.Context, task string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := store.NewID()
	s.recs[id] = &record{run: run.Run{ID: id, Task: task, Status: run.StatusPending}}
	s.order = append(s.order, id)
	return id, nil
}

func (s *Store) Claim(_ context.Context, worker string, lease time.Duration) (*run.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, id := range s.order {
		rec := s.recs[id]
		runnable := rec.run.Status == run.StatusPending ||
			(rec.run.Status == run.StatusRunning && !rec.leaseUntil.After(now))
		if !runnable {
			continue
		}
		rec.run.Status = run.StatusRunning
		rec.worker = worker
		rec.leaseUntil = now.Add(lease)
		return cloneRun(&rec.run), nil
	}
	return nil, nil
}

func (s *Store) AppendStep(_ context.Context, runID string, st run.Step) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.recs[runID]
	if !ok {
		return errNotFound(runID)
	}
	rec.run.Steps = append(rec.run.Steps, st)
	return nil
}

func (s *Store) Complete(_ context.Context, runID string, status run.Status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.recs[runID]
	if !ok {
		return errNotFound(runID)
	}
	rec.run.Status = status
	rec.run.Error = errMsg
	rec.worker = ""
	rec.leaseUntil = time.Time{}
	return nil
}

func (s *Store) Get(_ context.Context, runID string) (*run.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.recs[runID]
	if !ok {
		return nil, errNotFound(runID)
	}
	return cloneRun(&rec.run), nil
}

func (s *Store) List(_ context.Context, limit int) ([]*run.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	out := make([]*run.Run, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, cloneRun(&s.recs[s.order[i]].run))
	}
	return out, nil
}

func cloneRun(r *run.Run) *run.Run {
	c := *r
	c.Steps = append([]run.Step(nil), r.Steps...)
	return &c
}

type notFound string

func (e notFound) Error() string  { return "run not found: " + string(e) }
func errNotFound(id string) error { return notFound(id) }

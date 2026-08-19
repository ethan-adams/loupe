// Package worker drains runs from a store and executes them. It is the piece
// that makes the runtime horizontal: run many workers against one store and
// they share the queue, each claiming a different run. When a worker dies, its
// lease expires and another worker claims and resumes its run.
package worker

import (
	"context"
	"time"

	"github.com/ethan-adams/loupe/internal/agent"
	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/store"
)

// Build returns a fresh agent for a claimed run. The worker sets the agent's
// Persist hook itself, so a Build only needs to supply the model, tools, and
// (optionally) a stream.
type Build func(runID, task string) *agent.Agent

// Worker claims and runs work from a store.
type Worker struct {
	ID    string
	Store store.Store
	Build Build
	Lease time.Duration
}

// New builds a worker with a default lease.
func New(id string, s store.Store, build Build) *Worker {
	return &Worker{ID: id, Store: s, Build: build, Lease: 30 * time.Second}
}

// Once claims at most one run and executes it to a terminal state. It returns
// true if it claimed a run (whether it succeeded or failed), false if the queue
// had nothing runnable.
func (w *Worker) Once(ctx context.Context) (bool, error) {
	r, err := w.Store.Claim(ctx, w.ID, w.Lease)
	if err != nil || r == nil {
		return false, err
	}

	ag := w.Build(r.ID, r.Task)
	ag.Persist = func(c context.Context, runID string, s run.Step) error {
		return w.Store.AppendStep(c, runID, s)
	}

	var out *run.Run
	if len(r.Steps) == 0 {
		out, err = ag.Run(ctx, r.ID, r.Task)
	} else {
		out, err = ag.Continue(ctx, r)
	}

	if out != nil {
		// Record the terminal status. A context cancellation is a crash, not a
		// clean finish: leave the run running so its lease can expire and let
		// another worker resume it.
		if ctx.Err() == nil {
			_ = w.Store.Complete(ctx, r.ID, out.Status, out.Error)
		}
	}
	return true, err
}

// Run drains the queue until the context is cancelled, polling when idle.
func (w *Worker) Run(ctx context.Context, poll time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		worked, err := w.Once(ctx)
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if !worked {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(poll):
			}
		}
	}
}

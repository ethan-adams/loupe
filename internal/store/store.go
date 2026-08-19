// Package store is the durable home for runs and their steps. A Store is a
// queue and a log at once: work is enqueued, a worker claims the next available
// run under a lease, every step is appended as it commits, and the run is
// completed with a terminal status. If a worker dies mid-run, its lease expires
// and another worker can claim the same run and resume it from its stored steps.
//
// The interface has two implementations: an in-memory Store for tests and
// zero-setup local runs, and a Postgres Store for the real durable path.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/ethan-adams/loupe/internal/run"
)

// ErrClosed is returned by operations on a closed store.
var ErrClosed = errors.New("store is closed")

// Store persists runs and hands them out to workers.
type Store interface {
	// Enqueue records a new run in the queued state and returns its id.
	Enqueue(ctx context.Context, task string) (string, error)

	// Claim leases the next runnable run to worker for the given duration and
	// marks it running. A run is runnable if it is queued, or if it is running
	// but its lease has expired (the previous worker died). It returns the run
	// with its stored steps loaded, or (nil, nil) when nothing is runnable.
	Claim(ctx context.Context, worker string, lease time.Duration) (*run.Run, error)

	// AppendStep commits one step to a run, in order.
	AppendStep(ctx context.Context, runID string, s run.Step) error

	// Complete sets a run's terminal status and clears its lease.
	Complete(ctx context.Context, runID string, status run.Status, errMsg string) error

	// Get returns a run with its steps.
	Get(ctx context.Context, runID string) (*run.Run, error)

	// List returns recent runs (newest first) with their steps, up to limit.
	List(ctx context.Context, limit int) ([]*run.Run, error)
}

// NewID returns a short random hex id, used for run ids by every Store so the
// id space is the same regardless of backend.
func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

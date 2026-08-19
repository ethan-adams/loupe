// Package postgres is the durable store.Store: runs and steps live in Postgres,
// and Claim uses SELECT ... FOR UPDATE SKIP LOCKED so many workers can drain the
// same queue without stepping on each other. A worker renews its lease on every
// committed step; if it dies, the lease stops advancing and another worker
// reclaims the run once it expires. This is the piece that makes resume work
// across processes and machines, not just within one.
package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

// heartbeat is how far each committed step pushes a run's lease into the future.
// A worker that stops committing steps loses its claim after this long.
const heartbeat = 30 * time.Second

// Store is a Postgres-backed store.Store.
type Store struct {
	pool *pgxpool.Pool
}

var _ store.Store = (*Store)(nil)

// Open connects to Postgres, applies the schema, and returns a store.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

func (s *Store) Enqueue(ctx context.Context, task string) (string, error) {
	id := store.NewID()
	_, err := s.pool.Exec(ctx, `INSERT INTO runs (id, task, status) VALUES ($1, $2, 'pending')`, id, task)
	return id, err
}

func (s *Store) Claim(ctx context.Context, worker string, lease time.Duration) (*run.Run, error) {
	var id, task string
	err := s.pool.QueryRow(ctx, `
		UPDATE runs SET
			status = 'running',
			worker = $1,
			lease_until = now() + make_interval(secs => $2),
			updated_at = now()
		WHERE id = (
			SELECT id FROM runs
			WHERE status = 'pending' OR (status = 'running' AND lease_until < now())
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, task`,
		worker, lease.Seconds(),
	).Scan(&id, &task)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // nothing claimable
		}
		return nil, err
	}
	return s.load(ctx, id, task)
}

func (s *Store) AppendStep(ctx context.Context, runID string, st run.Step) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO steps
			(run_id, id, kind, thought, tool, input, output, is_error, answer, started_at, ended_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		runID, st.ID, string(st.Kind), st.Thought, st.Tool, st.Input, st.Output, st.IsError, st.Answer,
		nullTime(st.StartedAt), nullTime(st.EndedAt),
	); err != nil {
		return err
	}

	// Renew the lease: committing a step proves this worker is still alive.
	if _, err := tx.Exec(ctx, `
		UPDATE runs SET lease_until = now() + make_interval(secs => $2), updated_at = now()
		WHERE id = $1 AND status = 'running'`,
		runID, heartbeat.Seconds(),
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Complete(ctx context.Context, runID string, status run.Status, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE runs SET status = $2, error = $3, worker = '', lease_until = NULL, updated_at = now()
		WHERE id = $1`,
		runID, string(status), errMsg,
	)
	return err
}

func (s *Store) Get(ctx context.Context, runID string) (*run.Run, error) {
	var task string
	if err := s.pool.QueryRow(ctx, `SELECT task FROM runs WHERE id = $1`, runID).Scan(&task); err != nil {
		return nil, err
	}
	return s.load(ctx, runID, task)
}

// load reads a run's status and its ordered steps.
func (s *Store) load(ctx context.Context, id, task string) (*run.Run, error) {
	r := &run.Run{ID: id, Task: task}

	var status, errMsg string
	if err := s.pool.QueryRow(ctx, `SELECT status, error FROM runs WHERE id = $1`, id).Scan(&status, &errMsg); err != nil {
		return nil, err
	}
	r.Status = run.Status(status)
	r.Error = errMsg

	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, thought, tool, input, output, is_error, answer, started_at, ended_at
		FROM steps WHERE run_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var st run.Step
		var kind string
		var started, ended *time.Time
		if err := rows.Scan(&st.ID, &kind, &st.Thought, &st.Tool, &st.Input, &st.Output, &st.IsError, &st.Answer, &started, &ended); err != nil {
			return nil, err
		}
		st.Kind = run.StepKind(kind)
		if started != nil {
			st.StartedAt = *started
		}
		if ended != nil {
			st.EndedAt = *ended
		}
		r.Steps = append(r.Steps, st)
	}
	return r, rows.Err()
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

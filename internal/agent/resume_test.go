package agent

import (
	"context"
	"testing"
	"time"

	"github.com/ethan-adams/loupe/internal/model"
	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/store/memory"
	"github.com/ethan-adams/loupe/internal/tools"
)

// testClock is a manually advanced clock so a lease can expire without sleeping.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

// TestRunResumesAfterCrash is the core M2 guarantee: a worker dies the instant
// it commits the fix, its lease expires, another worker reclaims the run and
// finishes it from the stored steps, and the side effect (the file write) is
// applied exactly once.
func TestRunResumesAfterCrash(t *testing.T) {
	ctx := context.Background()
	clk := &testClock{t: time.Unix(1_700_000_000, 0)}
	st := memory.NewWithClock(clk.now)

	// A single repo shared across both workers stands in for durable world
	// state (a real repo on disk that survives the crash).
	repo := tools.NewBuggyRepo()
	reg := tools.NewRegistry(
		tools.NewListFiles(repo),
		tools.NewReadFile(repo),
		tools.NewWriteFile(repo),
		tools.NewRunTests(repo),
	)

	id, err := st.Enqueue(ctx, "fix it")
	if err != nil {
		t.Fatal(err)
	}

	// Worker A claims and crashes right after committing the write_file result.
	rA, err := st.Claim(ctx, "A", 30*time.Second)
	if err != nil || rA == nil {
		t.Fatalf("claim A: %v, run=%v", err, rA)
	}
	crashCtx, crash := context.WithCancel(ctx)
	agA := New(model.NewMock("A", fixScript()), reg, nil)
	agA.Persist = func(c context.Context, runID string, s run.Step) error {
		if e := st.AppendStep(c, runID, s); e != nil {
			return e
		}
		if s.Kind == run.StepObserve && s.Tool == "write_file" {
			crash()
		}
		return nil
	}
	_, _ = agA.Run(crashCtx, rA.ID, rA.Task)
	crash()

	// The crashed run is still running (never completed) and has not yet reached
	// a passing test.
	mid, _ := st.Get(ctx, id)
	if mid.Status != run.StatusRunning {
		t.Fatalf("after crash status = %q, want running", mid.Status)
	}
	if countPassObserves(mid) != 0 {
		t.Fatal("run should have crashed before the tests passed")
	}
	if !repo.TestsPass() {
		t.Fatal("the fix should be durably applied by the crash point")
	}

	// The lease expires; worker B reclaims and resumes.
	clk.t = clk.t.Add(31 * time.Second)
	rB, err := st.Claim(ctx, "B", 30*time.Second)
	if err != nil || rB == nil {
		t.Fatalf("worker B should reclaim the crashed run, got %v (%v)", rB, err)
	}
	if rB.ID != id {
		t.Fatalf("reclaimed the wrong run: %s", rB.ID)
	}

	agB := New(model.NewMock("B", fixScript()), reg, nil)
	agB.Persist = func(c context.Context, runID string, s run.Step) error {
		return st.AppendStep(c, runID, s)
	}
	out, err := agB.Continue(ctx, rB)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	_ = st.Complete(ctx, id, out.Status, out.Error)

	final, _ := st.Get(ctx, id)
	if final.Status != run.StatusSucceeded {
		t.Fatalf("final status = %q, want succeeded", final.Status)
	}
	if countPassObserves(final) == 0 {
		t.Error("resumed run should have a passing run_tests observation")
	}
	if n := countWriteObserves(final); n != 1 {
		t.Errorf("write_file observed %d times across the run, want exactly 1 (resume must not redo committed side effects)", n)
	}
}

func countWriteObserves(r *run.Run) int {
	n := 0
	for _, s := range r.Steps {
		if s.Kind == run.StepObserve && s.Tool == "write_file" {
			n++
		}
	}
	return n
}

func countPassObserves(r *run.Run) int {
	n := 0
	for _, s := range r.Steps {
		if s.Kind == run.StepObserve && s.Tool == "run_tests" && !s.IsError && len(s.Output) >= 4 && s.Output[:4] == "PASS" {
			n++
		}
	}
	return n
}

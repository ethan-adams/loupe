// Package demo wires the pieces into runnable scenarios. `loupe demo` runs an
// agent that fixes a failing test with every step streamed live. `loupe
// resume-demo` runs that same work, kills the worker the instant it commits the
// fix, and lets a second worker reclaim and finish it, so you can watch a run
// survive a crash.
package demo

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethan-adams/loupe/internal/agent"
	"github.com/ethan-adams/loupe/internal/model"
	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/store/memory"
	"github.com/ethan-adams/loupe/internal/tools"
	"github.com/ethan-adams/loupe/internal/trace"
)

// Task is the one-line brief the demo agent is given.
const Task = "Make the failing test in this repo pass."

// Options tunes a demo run.
type Options struct {
	UseOllama bool          // drive with a real local model instead of the script
	Pace      time.Duration // delay between emitted steps so it reads as "live"
}

// Run executes the fix-a-failing-test scenario against a fresh buggy repo and
// publishes every step to the stream.
func Run(ctx context.Context, stream *trace.Stream, opts Options) (*run.Run, error) {
	repo := tools.NewBuggyRepo()
	reg := buildRegistry(repo)

	var m model.Model
	if opts.UseOllama {
		m = model.NewOllama(&http.Client{Timeout: 120 * time.Second})
	} else {
		m = scriptedFix()
	}

	ag := agent.New(m, reg, stream)
	ag.Pace = opts.Pace
	return ag.Run(ctx, "run-demo-1", Task)
}

// ResumeDemo runs one durable run across a simulated crash. Worker A claims the
// run and works it, then loses power the instant the fix is committed. Its lease
// expires and worker B reclaims the same run, resumes it from the stored steps,
// runs the tests (which now pass, because the fix is real and durable), and
// finishes. Both workers publish to the same stream so the whole story renders
// as one timeline.
func ResumeDemo(ctx context.Context, stream *trace.Stream, pace time.Duration) (*run.Run, error) {
	clk := &clock{t: time.Unix(1_700_000_000, 0)}
	st := memory.NewWithClock(clk.now)
	repo := tools.NewBuggyRepo() // stands in for durable world-state (a real repo on disk)
	reg := buildRegistry(repo)
	lease := 30 * time.Second

	if _, err := st.Enqueue(ctx, Task); err != nil {
		return nil, err
	}

	// Worker A claims and works, then crashes right after committing the fix.
	rA, err := st.Claim(ctx, "worker-A", lease)
	if err != nil || rA == nil {
		return nil, fmt.Errorf("resume demo: worker A could not claim a run (%v)", err)
	}
	crashCtx, crash := context.WithCancel(ctx)
	defer crash()
	agA := agent.New(scriptedFix(), reg, stream)
	agA.Pace = pace
	agA.Persist = func(c context.Context, runID string, s run.Step) error {
		if err := st.AppendStep(c, runID, s); err != nil {
			return err
		}
		if s.Kind == run.StepObserve && s.Tool == "write_file" {
			crash() // the box loses power the instant the fix is committed
		}
		return nil
	}
	_, _ = agA.Run(crashCtx, rA.ID, rA.Task)
	// A crashed: we deliberately do not Complete it, so the run stays running
	// with a lease nobody is renewing.

	// Time passes; the lease expires and the run becomes reclaimable.
	clk.advance(lease + time.Second)

	// Worker B reclaims the same run and resumes it from its stored steps.
	rB, err := st.Claim(ctx, "worker-B", lease)
	if err != nil || rB == nil {
		return nil, fmt.Errorf("resume demo: worker B could not reclaim the crashed run (%v)", err)
	}
	agB := agent.New(scriptedFix(), reg, stream)
	agB.Pace = pace
	agB.Persist = func(c context.Context, runID string, s run.Step) error {
		return st.AppendStep(c, runID, s)
	}
	out, err := agB.Continue(ctx, rB)
	if out != nil {
		_ = st.Complete(ctx, rB.ID, out.Status, out.Error)
	}
	return out, err
}

// NewFixAgent builds a fresh agent for the fix-a-failing-test scenario, with a
// new repo each call. The worker command uses it to turn each claimed run into
// an agent. Note the toy repo lives in memory, so the tool's world state is not
// yet durable across processes; the run, its steps, and resume are. Durable
// tools (a real repo on disk) are a later milestone.
func NewFixAgent(stream *trace.Stream, useOllama bool) *agent.Agent {
	repo := tools.NewBuggyRepo()
	var m model.Model
	if useOllama {
		m = model.NewOllama(&http.Client{Timeout: 120 * time.Second})
	} else {
		m = scriptedFix()
	}
	return agent.New(m, buildRegistry(repo), stream)
}

func buildRegistry(repo *tools.Repo) *tools.Registry {
	return tools.NewRegistry(
		tools.NewListFiles(repo),
		tools.NewReadFile(repo),
		tools.NewWriteFile(repo),
		tools.NewRunTests(repo),
	)
}

// scriptedFix is the deterministic story the offline demo tells: run the tests,
// fumble a path (a real error the agent recovers from), read the right file,
// fix the bug, and confirm the tests pass. It is observation-count driven, so
// the same script resumes correctly after a crash.
func scriptedFix() model.Model {
	fixed := "math.py\ndef add(a, b):\n    return a + b\n"
	return model.NewMock("scripted-fix", []model.Decision{
		{Thought: "Let me run the tests to see what is failing.",
			ToolCall: &model.ToolCall{Name: "run_tests"}},
		{Thought: "A test is failing. Let me open the source file.",
			ToolCall: &model.ToolCall{Name: "read_file", Input: "mat.py"}}, // typo, on purpose
		{Thought: "Wrong path. Reading math.py instead.",
			ToolCall: &model.ToolCall{Name: "read_file", Input: "math.py"}},
		{Thought: "The bug is 'a - b'; it should be 'a + b'. Writing the fix.",
			ToolCall: &model.ToolCall{Name: "write_file", Input: fixed}},
		{Thought: "Re-running the tests to confirm the fix.",
			ToolCall: &model.ToolCall{Name: "run_tests"}},
		{Thought: "The tests pass.",
			Final: "Fixed math.py: add() now returns a + b, so add(2, 3) == 5 and the test passes."},
	})
}

// clock is a manually advanced clock for the resume demo, so the lease expires
// on cue instead of after a real 30 second wait.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

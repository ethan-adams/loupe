package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/ethan-adams/loupe/internal/model"
	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/tools"
)

// fixScript reproduces the demo story: run tests, fumble a path, read the right
// file, write the fix, re-run, finish. It exercises tool dispatch, an error the
// agent recovers from, and a clean finish.
func fixScript() []model.Decision {
	fixed := "math.py\ndef add(a, b):\n    return a + b\n"
	return []model.Decision{
		{Thought: "check", ToolCall: &model.ToolCall{Name: "run_tests"}},
		{Thought: "oops", ToolCall: &model.ToolCall{Name: "read_file", Input: "mat.py"}},
		{Thought: "read", ToolCall: &model.ToolCall{Name: "read_file", Input: "math.py"}},
		{Thought: "fix", ToolCall: &model.ToolCall{Name: "write_file", Input: fixed}},
		{Thought: "verify", ToolCall: &model.ToolCall{Name: "run_tests"}},
		{Thought: "done", Final: "fixed"},
	}
}

func newFixAgent(repo *tools.Repo, script []model.Decision) *Agent {
	reg := tools.NewRegistry(
		tools.NewListFiles(repo),
		tools.NewReadFile(repo),
		tools.NewWriteFile(repo),
		tools.NewRunTests(repo),
	)
	return New(model.NewMock("test", script), reg, nil)
}

func TestLoopFixesFailingTest(t *testing.T) {
	repo := tools.NewBuggyRepo()
	if repo.TestsPass() {
		t.Fatal("repo should start with a failing test")
	}

	ag := newFixAgent(repo, fixScript())
	r, err := ag.Run(context.Background(), "r1", "fix it")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if r.Status != run.StatusSucceeded {
		t.Fatalf("status = %q, want succeeded", r.Status)
	}
	if !repo.TestsPass() {
		t.Fatal("tests should pass after the run")
	}

	// The trace must show the agent hitting a tool error and then recovering.
	var sawError, sawFinal, sawPass bool
	for _, s := range r.Steps {
		if s.Kind == run.StepObserve && s.IsError {
			sawError = true
		}
		if s.Kind == run.StepObserve && s.Tool == "run_tests" && strings.Contains(s.Output, "PASS") {
			sawPass = true
		}
		if s.Kind == run.StepFinal {
			sawFinal = true
		}
	}
	if !sawError {
		t.Error("expected a recovered tool error in the trace")
	}
	if !sawPass {
		t.Error("expected a passing run_tests observation")
	}
	if !sawFinal {
		t.Error("expected a final step")
	}
}

func TestLoopStopsAtStepBudget(t *testing.T) {
	// A model that always calls a tool and never finishes must be stopped.
	loop := []model.Decision{{ToolCall: &model.ToolCall{Name: "run_tests"}}}
	repo := tools.NewBuggyRepo()
	reg := tools.NewRegistry(tools.NewRunTests(repo))
	ag := New(&repeatModel{d: loop[0]}, reg, nil)
	ag.MaxSteps = 4

	r, err := ag.Run(context.Background(), "r2", "spin")
	if err == nil {
		t.Fatal("expected a step-budget error")
	}
	if r.Status != run.StatusFailed {
		t.Fatalf("status = %q, want failed", r.Status)
	}
}

func TestLoopUnknownToolIsRecoverable(t *testing.T) {
	repo := tools.NewBuggyRepo()
	reg := tools.NewRegistry(tools.NewRunTests(repo))
	script := []model.Decision{
		{ToolCall: &model.ToolCall{Name: "nope"}}, // no such tool -> error observation
		{Final: "gave up gracefully"},
	}
	ag := New(model.NewMock("t", script), reg, nil)

	r, err := ag.Run(context.Background(), "r3", "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != run.StatusSucceeded {
		t.Fatalf("status = %q, want succeeded", r.Status)
	}
	found := false
	for _, s := range r.Steps {
		if s.Kind == run.StepObserve && s.IsError {
			found = true
		}
	}
	if !found {
		t.Error("unknown tool should produce an error observation, not crash the run")
	}
}

// repeatModel always returns the same decision.
type repeatModel struct{ d model.Decision }

func (m *repeatModel) Name() string { return "repeat" }
func (m *repeatModel) Next(_ context.Context, _ model.Turn) (model.Decision, error) {
	return m.d, nil
}

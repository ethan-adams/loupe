// Package demo wires the pieces into the flagship scenario: an agent is asked
// to make a failing test pass, and every step is streamed as it happens. It is
// the thing `loupe demo` runs and the shape of the eventual hero gif.
package demo

import (
	"context"
	"net/http"
	"time"

	"github.com/ethan-adams/loupe/internal/agent"
	"github.com/ethan-adams/loupe/internal/model"
	"github.com/ethan-adams/loupe/internal/run"
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
// publishes every step to the stream. The returned run carries the full trace.
func Run(ctx context.Context, stream *trace.Stream, opts Options) (*run.Run, error) {
	repo := tools.NewBuggyRepo()
	reg := tools.NewRegistry(
		tools.NewListFiles(repo),
		tools.NewReadFile(repo),
		tools.NewWriteFile(repo),
		tools.NewRunTests(repo),
	)

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

// scriptedFix is the deterministic story the offline demo tells: run the tests,
// fumble a path (a real error the agent recovers from), read the right file,
// fix the bug, and confirm the tests pass. It never touches a model server, so
// the demo runs anywhere.
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

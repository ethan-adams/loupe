// Package agent is the run engine: the plan / act / observe loop that turns a
// task into a sequence of traced steps. It asks the model what to do, runs the
// tool the model chose, feeds the result back, and repeats until the model
// says it is done or a step budget is hit.
//
// M1 keeps the loop in memory and single-run. The durability, resumability, and
// back-pressure that make this a real distributed engine are later milestones,
// but they slot in behind this same loop.
package agent

import (
	"context"
	"errors"
	"time"

	"github.com/ethan-adams/loupe/internal/model"
	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/tools"
	"github.com/ethan-adams/loupe/internal/trace"
)

// Agent wires a model to a set of tools and streams the run's steps.
type Agent struct {
	Model    model.Model
	Tools    *tools.Registry
	Stream   *trace.Stream // may be nil (tests)
	MaxSteps int           // safety budget; 0 means default
	Pace     time.Duration // optional delay between emitted steps, for a live feel

	now func() time.Time // injectable clock; defaults to time.Now
}

// New builds an agent with sensible defaults.
func New(m model.Model, reg *tools.Registry, stream *trace.Stream) *Agent {
	return &Agent{Model: m, Tools: reg, Stream: stream, MaxSteps: 20, now: time.Now}
}

// Run executes the loop for one task and returns the completed run. The returned
// error is non-nil only when the run failed (model error or step budget); the
// Run is always returned so callers can inspect the partial trace.
func (a *Agent) Run(ctx context.Context, runID, task string) (*run.Run, error) {
	if a.now == nil {
		a.now = time.Now
	}
	maxSteps := a.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 20
	}

	r := &run.Run{ID: runID, Task: task, Status: run.StatusRunning}
	a.publish(trace.Event{Type: trace.RunStarted, RunID: runID, Task: task, Status: r.Status})

	var obs []model.Observation
	stepID := 0

	for i := 0; i < maxSteps; i++ {
		if err := ctx.Err(); err != nil {
			return a.fail(r, err.Error())
		}

		turn := model.Turn{Task: task, Tools: specs(a.Tools), Observations: obs}
		dec, err := a.Model.Next(ctx, turn)
		if err != nil {
			return a.fail(r, err.Error())
		}

		if dec.Thought != "" {
			stepID++
			a.emit(r, run.Step{ID: stepID, Kind: run.StepThink, Thought: dec.Thought, StartedAt: a.now(), EndedAt: a.now()})
		}

		if dec.Final != "" {
			stepID++
			a.emit(r, run.Step{ID: stepID, Kind: run.StepFinal, Answer: dec.Final, StartedAt: a.now(), EndedAt: a.now()})
			r.Status = run.StatusSucceeded
			a.publish(trace.Event{Type: trace.RunEnded, RunID: runID, Status: r.Status})
			return r, nil
		}

		if dec.ToolCall == nil {
			return a.fail(r, "model returned neither a tool call nor a final answer")
		}

		// Emit the call before running it so a watcher sees "calling..." live.
		stepID++
		a.emit(r, run.Step{ID: stepID, Kind: run.StepToolCall, Tool: dec.ToolCall.Name, Input: dec.ToolCall.Input, StartedAt: a.now()})

		out, isErr := a.dispatch(ctx, dec.ToolCall.Name, dec.ToolCall.Input)

		stepID++
		a.emit(r, run.Step{ID: stepID, Kind: run.StepObserve, Tool: dec.ToolCall.Name, Input: dec.ToolCall.Input, Output: out, IsError: isErr, StartedAt: a.now(), EndedAt: a.now()})

		obs = append(obs, model.Observation{Tool: dec.ToolCall.Name, Input: dec.ToolCall.Input, Output: out, IsError: isErr})
	}

	return a.fail(r, "reached step budget without finishing")
}

// dispatch runs one tool. A tool error is not a run failure: it is fed back to
// the model as an observation so it can recover, which is the whole point.
func (a *Agent) dispatch(ctx context.Context, name, input string) (output string, isErr bool) {
	t, ok := a.Tools.Get(name)
	if !ok {
		return "no such tool: " + name, true
	}
	out, err := t.Run(ctx, input)
	if err != nil {
		return err.Error(), true
	}
	return out, false
}

func (a *Agent) emit(r *run.Run, s run.Step) {
	r.Steps = append(r.Steps, s)
	a.publish(trace.Event{Type: trace.StepAdded, RunID: r.ID, Step: s})
	if a.Pace > 0 {
		time.Sleep(a.Pace)
	}
}

func (a *Agent) fail(r *run.Run, msg string) (*run.Run, error) {
	r.Status = run.StatusFailed
	r.Error = msg
	a.publish(trace.Event{Type: trace.RunEnded, RunID: r.ID, Status: r.Status, Error: msg})
	return r, errors.New(msg)
}

func (a *Agent) publish(e trace.Event) {
	if a.Stream != nil {
		a.Stream.Publish(e)
	}
}

func specs(reg *tools.Registry) []model.ToolSpec {
	if reg == nil {
		return nil
	}
	src := reg.Specs()
	out := make([]model.ToolSpec, len(src))
	for i, s := range src {
		out[i] = model.ToolSpec{Name: s.Name, Description: s.Description}
	}
	return out
}

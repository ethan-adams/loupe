// Package agent is the run engine: the plan / act / observe loop that turns a
// task into a sequence of traced steps. It asks the model what to do, runs the
// tool the model chose, feeds the result back, and repeats until the model
// says it is done or a step budget is hit.
//
// The loop is resumable. Every step is handed to Persist (if set) before the
// run moves on, so a durable store holds a committed record of progress. A run
// that dies part way through is picked up with Continue, which rebuilds the
// observation history from the stored steps and carries on from there without
// re-running the work that already happened.
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

// Agent wires a model to a set of tools, streams the run's steps, and (when
// Persist is set) commits each step to a durable store.
type Agent struct {
	Model    model.Model
	Tools    *tools.Registry
	Stream   *trace.Stream // may be nil (tests)
	MaxSteps int           // safety budget; 0 means default
	Pace     time.Duration // optional delay between emitted steps, for a live feel

	// Persist is called with every new step, in order, before the run
	// continues. Returning an error stops the run: a step that cannot be
	// committed must not be built on. May be nil (no durability).
	Persist func(ctx context.Context, runID string, s run.Step) error

	now func() time.Time // injectable clock; defaults to time.Now
}

// New builds an agent with sensible defaults.
func New(m model.Model, reg *tools.Registry, stream *trace.Stream) *Agent {
	return &Agent{Model: m, Tools: reg, Stream: stream, MaxSteps: 20, now: time.Now}
}

// Run executes the loop for a fresh task from step zero.
func (a *Agent) Run(ctx context.Context, runID, task string) (*run.Run, error) {
	a.ensureClock()
	r := &run.Run{ID: runID, Task: task, Status: run.StatusRunning}
	a.publish(trace.Event{Type: trace.RunStarted, RunID: runID, Task: task, Status: r.Status})
	return a.loop(ctx, r, nil, 0)
}

// Continue resumes a run whose steps have been loaded from a store. It replays
// the stored observations as history (without re-running the tools that
// produced them) and picks up after the last committed step.
func (a *Agent) Continue(ctx context.Context, r *run.Run) (*run.Run, error) {
	a.ensureClock()

	var obs []model.Observation
	stepID := 0
	for _, s := range r.Steps {
		if s.ID > stepID {
			stepID = s.ID
		}
		if s.Kind == run.StepObserve {
			obs = append(obs, model.Observation{Tool: s.Tool, Input: s.Input, Output: s.Output, IsError: s.IsError})
		}
	}

	r.Status = run.StatusRunning
	r.Error = ""
	a.publish(trace.Event{Type: trace.RunResumed, RunID: r.ID, Task: r.Task, Status: r.Status})
	return a.loop(ctx, r, obs, stepID)
}

// loop is the shared engine for Run and Continue. obs and stepID carry any
// history a resumed run brings with it.
func (a *Agent) loop(ctx context.Context, r *run.Run, obs []model.Observation, stepID int) (*run.Run, error) {
	maxSteps := a.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 20
	}

	// Budget is per attempt: at most maxSteps model turns before we give up. A
	// resumed run gets a fresh attempt, which is the intended behaviour.
	for turnNo := 0; turnNo < maxSteps; turnNo++ {
		if err := ctx.Err(); err != nil {
			return a.fail(r, "worker stopped mid-run ("+err.Error()+")")
		}

		turn := model.Turn{Task: r.Task, Tools: specs(a.Tools), Observations: obs}
		dec, err := a.Model.Next(ctx, turn)
		if err != nil {
			return a.fail(r, err.Error())
		}

		if dec.Thought != "" {
			stepID++
			if err := a.emit(ctx, r, run.Step{ID: stepID, Kind: run.StepThink, Thought: dec.Thought, StartedAt: a.now(), EndedAt: a.now()}); err != nil {
				return a.fail(r, "persist: "+err.Error())
			}
		}

		if dec.Final != "" {
			stepID++
			if err := a.emit(ctx, r, run.Step{ID: stepID, Kind: run.StepFinal, Answer: dec.Final, StartedAt: a.now(), EndedAt: a.now()}); err != nil {
				return a.fail(r, "persist: "+err.Error())
			}
			r.Status = run.StatusSucceeded
			a.publish(trace.Event{Type: trace.RunEnded, RunID: r.ID, Status: r.Status})
			return r, nil
		}

		if dec.ToolCall == nil {
			return a.fail(r, "model returned neither a tool call nor a final answer")
		}

		stepID++
		if err := a.emit(ctx, r, run.Step{ID: stepID, Kind: run.StepToolCall, Tool: dec.ToolCall.Name, Input: dec.ToolCall.Input, StartedAt: a.now()}); err != nil {
			return a.fail(r, "persist: "+err.Error())
		}

		out, isErr := a.dispatch(ctx, dec.ToolCall.Name, dec.ToolCall.Input)

		stepID++
		if err := a.emit(ctx, r, run.Step{ID: stepID, Kind: run.StepObserve, Tool: dec.ToolCall.Name, Input: dec.ToolCall.Input, Output: out, IsError: isErr, StartedAt: a.now(), EndedAt: a.now()}); err != nil {
			return a.fail(r, "persist: "+err.Error())
		}

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

// emit records a step on the run, commits it durably (if Persist is set), then
// streams it to any live watchers. Committing before streaming keeps the stored
// record ahead of what a viewer has seen.
func (a *Agent) emit(ctx context.Context, r *run.Run, s run.Step) error {
	r.Steps = append(r.Steps, s)
	if a.Persist != nil {
		if err := a.Persist(ctx, r.ID, s); err != nil {
			return err
		}
	}
	a.publish(trace.Event{Type: trace.StepAdded, RunID: r.ID, Step: s})
	if a.Pace > 0 {
		time.Sleep(a.Pace)
	}
	return nil
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

func (a *Agent) ensureClock() {
	if a.now == nil {
		a.now = time.Now
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

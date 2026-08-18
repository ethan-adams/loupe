// Package run holds the core domain types for an agent run: the run itself and
// the ordered steps it produces. These types are deliberately free of any
// transport, model, or storage concerns so everything else can depend on them
// without creating cycles.
package run

import "time"

// Status is where a run is in its lifecycle.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// StepKind is what happened in a single step of the loop.
type StepKind string

const (
	StepThink    StepKind = "think"     // the model reasoned about what to do next
	StepToolCall StepKind = "tool_call" // the agent invoked a tool
	StepObserve  StepKind = "observe"   // a tool returned a result (or an error)
	StepFinal    StepKind = "final"     // the agent produced its final answer
)

// Step is one entry in a run's timeline. Only the fields relevant to a given
// Kind are set; the rest stay zero. Keeping every step in one flat type makes
// the trace trivial to stream and to render.
type Step struct {
	ID   int      `json:"id"`
	Kind StepKind `json:"kind"`

	// StepThink
	Thought string `json:"thought,omitempty"`

	// StepToolCall / StepObserve
	Tool    string `json:"tool,omitempty"`
	Input   string `json:"input,omitempty"`
	Output  string `json:"output,omitempty"`
	IsError bool   `json:"isError,omitempty"`

	// StepFinal
	Answer string `json:"answer,omitempty"`

	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
}

// Run is a single execution of the agent loop over a task, plus every step it
// took to get there.
type Run struct {
	ID     string `json:"id"`
	Task   string `json:"task"`
	Status Status `json:"status"`
	Steps  []Step `json:"steps"`
	Error  string `json:"error,omitempty"`
}

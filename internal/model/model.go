// Package model defines the boundary between the agent loop and whatever is
// actually deciding what to do next. A Model is handed the task, the tools it
// may use, and everything it has observed so far, and it returns one Decision.
//
// This tiny interface is the whole reason the runtime is model-agnostic: a
// deterministic scripted model (for tests and the offline demo) and a real
// local model (Ollama) are just two implementations of the same three methods.
package model

import "context"

// ToolSpec is the model-facing description of a tool it is allowed to call.
type ToolSpec struct {
	Name        string
	Description string
}

// Observation is the result of a prior tool call, fed back to the model so it
// can decide what to do next.
type Observation struct {
	Tool    string
	Input   string
	Output  string
	IsError bool
}

// Turn is everything the model sees on one iteration of the loop.
type Turn struct {
	Task         string
	Tools        []ToolSpec
	Observations []Observation // oldest first
}

// ToolCall is the model's request to invoke a tool.
type ToolCall struct {
	Name  string
	Input string
}

// Decision is what the model returns each turn. A turn either calls a tool
// (ToolCall set) or finishes (Final non-empty). Thought is optional narration
// that shows up in the trace as a "thinking" step.
type Decision struct {
	Thought  string
	ToolCall *ToolCall
	Final    string
}

// Model decides the next action given a Turn.
type Model interface {
	Next(ctx context.Context, t Turn) (Decision, error)
	Name() string
}

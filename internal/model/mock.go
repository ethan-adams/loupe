package model

import "context"

// Mock is a deterministic model that returns a fixed script of decisions, one
// per turn. It makes the loop testable and lets the demo run with no model
// server at all. Once the script is exhausted it finishes the run.
type Mock struct {
	name   string
	script []Decision
	i      int
}

// NewMock builds a scripted model. The name shows up in traces so a reader can
// tell a scripted run from a real one.
func NewMock(name string, script []Decision) *Mock {
	return &Mock{name: name, script: script}
}

func (m *Mock) Name() string { return m.name }

func (m *Mock) Next(ctx context.Context, _ Turn) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if m.i >= len(m.script) {
		return Decision{Final: "done"}, nil
	}
	d := m.script[m.i]
	m.i++
	return d, nil
}

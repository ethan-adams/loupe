package model

import "context"

// Mock is a deterministic model that returns a fixed script of decisions. It
// picks the next decision by how many observations it has seen so far, which
// makes it stateless: rebuild the observation history and the same Mock resumes
// exactly where a crashed run left off. That is what lets the resume tests use
// it. It assumes one observation per scripted decision (every non-final step
// calls a tool), which holds for the demo script.
type Mock struct {
	name   string
	script []Decision
}

// NewMock builds a scripted model. The name shows up in traces so a reader can
// tell a scripted run from a real one.
func NewMock(name string, script []Decision) *Mock {
	return &Mock{name: name, script: script}
}

func (m *Mock) Name() string { return m.name }

func (m *Mock) Next(ctx context.Context, t Turn) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	i := len(t.Observations)
	if i >= len(m.script) {
		return Decision{Final: "done"}, nil
	}
	return m.script[i], nil
}

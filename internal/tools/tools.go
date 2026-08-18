// Package tools is the agent's hands: the set of actions it can take on the
// world. A Tool is a name, a human description, and a Run function. The
// registry keeps them in a stable order so traces and prompts are reproducible.
package tools

import "context"

// Tool is one action the agent can invoke.
type Tool interface {
	Name() string
	Description() string
	Run(ctx context.Context, input string) (output string, err error)
}

// Spec is a lightweight, model-neutral description of a tool. The agent maps
// this onto whatever shape its model expects.
type Spec struct {
	Name        string
	Description string
}

// Registry is an ordered set of tools.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry builds a registry from the given tools, preserving order.
func NewRegistry(ts ...Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range ts {
		r.Add(t)
	}
	return r
}

// Add registers a tool. Re-adding a name replaces it without changing order.
func (r *Registry) Add(t Tool) {
	if _, exists := r.tools[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Specs returns the tool descriptions in registration order.
func (r *Registry) Specs() []Spec {
	out := make([]Spec, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		out = append(out, Spec{Name: t.Name(), Description: t.Description()})
	}
	return out
}

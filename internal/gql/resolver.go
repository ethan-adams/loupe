package gql

// Resolver is the dependency-injection root for the subgraph. It holds the run
// store the resolvers read through. This file is not regenerated.

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/store"
)

type Resolver struct {
	Store store.Store
}

// NewServer builds the GraphQL HTTP handler for the runs subgraph.
func NewServer(st store.Store) http.Handler {
	es := NewExecutableSchema(Config{Resolvers: &Resolver{Store: st}})
	srv := handler.New(es)
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{}) // lets tools + the router discover the schema
	return srv
}

// toRun maps a stored run to the GraphQL model.
func toRun(r *run.Run) *Run {
	if r == nil {
		return nil
	}
	out := &Run{
		ID:     r.ID,
		Task:   r.Task,
		Status: statusToGQL(r.Status),
		Error:  ptr(r.Error),
		Steps:  make([]*Step, 0, len(r.Steps)),
	}
	for _, s := range r.Steps {
		out.Steps = append(out.Steps, toStep(s))
	}
	return out
}

func toStep(s run.Step) *Step {
	return &Step{
		ID:      s.ID,
		Kind:    kindToGQL(s.Kind),
		Thought: ptr(s.Thought),
		Tool:    ptr(s.Tool),
		Input:   ptr(s.Input),
		Output:  ptr(s.Output),
		IsError: s.IsError,
		Answer:  ptr(s.Answer),
	}
}

func statusToGQL(s run.Status) RunStatus {
	switch s {
	case run.StatusPending:
		return RunStatusPending
	case run.StatusRunning:
		return RunStatusRunning
	case run.StatusSucceeded:
		return RunStatusSucceeded
	case run.StatusFailed:
		return RunStatusFailed
	default:
		return RunStatusPending
	}
}

func kindToGQL(k run.StepKind) StepKind {
	switch k {
	case run.StepThink:
		return StepKindThink
	case run.StepToolCall:
		return StepKindToolCall
	case run.StepObserve:
		return StepKindObserve
	case run.StepFinal:
		return StepKindFinal
	default:
		return StepKindThink
	}
}

// ptr returns nil for an empty string so optional GraphQL fields read as null.
func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

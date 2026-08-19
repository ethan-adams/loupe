// Package server mounts Loupe's HTTP surface: the GraphQL subgraph and its
// playground for typed reads, and a Server-Sent Events endpoint for the live
// trace. The split is deliberate and mirrors Draw: typed, low-rate reads go
// through GraphQL (and federate); the high-rate step stream does not, it goes
// over SSE, which a browser reads with a one-line EventSource.
package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ethan-adams/loupe/internal/gql"
	"github.com/ethan-adams/loupe/internal/store"
	"github.com/ethan-adams/loupe/internal/trace"
)

//go:embed ui.html
var uiHTML []byte

// New returns the HTTP handler for the whole surface.
func New(st store.Store, hub *trace.Hub) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/graphql", gql.NewServer(st))
	mux.Handle("/graphql/playground", gql.PlaygroundHandler())
	mux.HandleFunc("GET /runs/{id}/stream", streamRun(hub))
	mux.HandleFunc("POST /runs", enqueue(st))
	mux.HandleFunc("GET /{$}", ui)
	return mux
}

// enqueue submits a new run. The body may be {"task": "..."}; an empty body
// enqueues the default fix-a-failing-test task. It responds with {"id": "..."}.
func enqueue(st store.Store) http.HandlerFunc {
	const defaultTask = "Make the failing test in this repo pass."
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Task string `json:"task"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // empty body is fine
		if body.Task == "" {
			body.Task = defaultTask
		}
		id, err := st.Enqueue(r.Context(), body.Task)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	}
}

// streamRun streams one run's trace events as they happen. It replays the run's
// history first (so a late watcher catches up), then delivers live events, and
// closes when the run ends.
func streamRun(hub *trace.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		events, cancel := hub.Subscribe(r.PathValue("id"))
		defer cancel()
		flusher.Flush() // send headers so the client's connection opens

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-events:
				if !ok {
					return
				}
				b, err := json.Marshal(e)
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
				if e.Type == trace.RunEnded {
					return
				}
			}
		}
	}
}

// ui serves the control room: a self-contained page that lists runs, starts new
// ones, and renders a run's steps live as they stream in.
func ui(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(uiHTML)
}

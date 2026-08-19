// Package server mounts Loupe's HTTP surface: the GraphQL subgraph and its
// playground for typed reads, and a Server-Sent Events endpoint for the live
// trace. The split is deliberate and mirrors Draw: typed, low-rate reads go
// through GraphQL (and federate); the high-rate step stream does not, it goes
// over SSE, which a browser reads with a one-line EventSource.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ethan-adams/loupe/internal/gql"
	"github.com/ethan-adams/loupe/internal/store"
	"github.com/ethan-adams/loupe/internal/trace"
)

// New returns the HTTP handler for the whole surface.
func New(st store.Store, hub *trace.Hub) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/graphql", gql.NewServer(st))
	mux.Handle("/graphql/playground", gql.PlaygroundHandler())
	mux.HandleFunc("GET /runs/{id}/stream", streamRun(hub))
	mux.HandleFunc("GET /{$}", index)
	return mux
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

func index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexPage))
}

const indexPage = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Loupe</title>
<style>body{font:15px/1.6 ui-monospace,Menlo,monospace;max-width:640px;margin:3rem auto;padding:0 1rem;color-scheme:light dark}code{opacity:.85}</style>
</head>
<body>
  <h1>Loupe</h1>
  <p>Run an AI agent and watch every step it takes, live.</p>
  <ul>
    <li><a href="/graphql/playground">GraphQL playground</a> &mdash; typed reads over runs</li>
    <li><code>GET /runs/&lt;id&gt;/stream</code> &mdash; live trace over Server-Sent Events</li>
  </ul>
</body>
</html>`

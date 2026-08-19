// Command loupe runs agents and shows every step they take.
//
//	loupe demo         an agent fixes a failing test, streamed live
//	loupe resume-demo  the same run survives a worker crash and is resumed
//	loupe submit       enqueue a run in the Postgres store
//	loupe worker       drain runs from the Postgres store, streamed live
//
// A later milestone adds a web control room over the same stream.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/ethan-adams/loupe/internal/agent"
	"github.com/ethan-adams/loupe/internal/demo"
	"github.com/ethan-adams/loupe/internal/render"
	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/server"
	"github.com/ethan-adams/loupe/internal/store/postgres"
	"github.com/ethan-adams/loupe/internal/trace"
	"github.com/ethan-adams/loupe/internal/worker"
)

const defaultDSN = "postgres://loupe:loupe@localhost:5433/loupe?sslmode=disable"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "demo":
		os.Exit(runDemo(os.Args[2:]))
	case "resume-demo":
		os.Exit(runResumeDemo(os.Args[2:]))
	case "submit":
		os.Exit(runSubmit(os.Args[2:]))
	case "worker":
		os.Exit(runWorker(os.Args[2:]))
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "loupe: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runDemo(args []string) int {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	modelName := fs.String("model", "scripted", `which model drives the agent: "scripted" (offline, deterministic) or "ollama" (a real local model)`)
	pace := fs.Duration("pace", 300*time.Millisecond, "delay between steps, so the run reads as live; use 0 to go full speed")
	_ = fs.Parse(args)

	return stream(func(ctx context.Context, s *trace.Stream) (*run.Run, error) {
		return demo.Run(ctx, s, demo.Options{UseOllama: *modelName == "ollama", Pace: *pace})
	})
}

func runResumeDemo(args []string) int {
	fs := flag.NewFlagSet("resume-demo", flag.ExitOnError)
	pace := fs.Duration("pace", 300*time.Millisecond, "delay between steps, so the run reads as live; use 0 to go full speed")
	_ = fs.Parse(args)

	fmt.Println("Watch a run survive a crash: worker A works until it commits the fix,")
	fmt.Println("then loses power. Worker B reclaims the run and finishes it.")
	return stream(func(ctx context.Context, s *trace.Stream) (*run.Run, error) {
		return demo.ResumeDemo(ctx, s, *pace)
	})
}

func runSubmit(args []string) int {
	fs := flag.NewFlagSet("submit", flag.ExitOnError)
	task := fs.String("task", demo.Task, "the task to enqueue")
	_ = fs.Parse(args)

	ctx := context.Background()
	st, err := postgres.Open(ctx, dsn())
	if err != nil {
		fmt.Fprintf(os.Stderr, "submit: %v\n", err)
		return 1
	}
	defer st.Close()

	id, err := st.Enqueue(ctx, *task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "submit: %v\n", err)
		return 1
	}
	fmt.Println(id)
	return 0
}

func runWorker(args []string) int {
	fs := flag.NewFlagSet("worker", flag.ExitOnError)
	id := fs.String("id", "worker-1", "worker id, shown in traces")
	modelName := fs.String("model", "scripted", `which model drives the agent: "scripted" or "ollama"`)
	pace := fs.Duration("pace", 200*time.Millisecond, "delay between steps for a live feel; 0 for full speed")
	once := fs.Bool("once", false, "process at most one run, then exit")
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := postgres.Open(ctx, dsn())
	if err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		return 1
	}
	defer st.Close()

	s := trace.New()
	events, cancel := s.Subscribe()
	done := make(chan struct{})
	go func() {
		render.Terminal(os.Stdout, events)
		close(done)
	}()

	build := func(runID, task string) *agent.Agent {
		ag := demo.NewFixAgent(s, *modelName == "ollama")
		ag.Pace = *pace
		return ag
	}
	w := worker.New(*id, st, build)

	if *once {
		worked, err := w.Once(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nworker: %v\n", err)
		}
		if !worked {
			fmt.Fprintln(os.Stderr, "worker: nothing to do")
		}
	} else {
		fmt.Fprintf(os.Stderr, "worker %s draining the queue (Ctrl-C to stop)\n", *id)
		_ = w.Run(ctx, time.Second)
	}

	cancel()
	<-done
	return 0
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	workers := fs.Int("workers", 2, "how many embedded workers drain the queue")
	modelName := fs.String("model", "scripted", `which model drives runs: "scripted" or "ollama"`)
	pace := fs.Duration("pace", 200*time.Millisecond, "delay between steps for a live feel; 0 for full speed")
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := postgres.Open(ctx, dsn())
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	defer st.Close()

	// One hub shared by the embedded workers (which publish) and the SSE
	// endpoint (which subscribes), so a browser watches a run this process runs.
	hub := trace.NewHub()
	build := func(runID, task string) *agent.Agent {
		ag := demo.NewFixAgent(hub.Stream(runID), *modelName == "ollama")
		ag.Pace = *pace
		return ag
	}
	for i := 0; i < *workers; i++ {
		w := worker.New(fmt.Sprintf("serve-%d", i), st, build)
		go w.Run(ctx, time.Second)
	}

	srv := &http.Server{Addr: *addr, Handler: server.New(st, hub)}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	fmt.Fprintf(os.Stderr, "loupe serving on %s  (playground: http://localhost%s/graphql/playground)\n", *addr, *addr)
	fmt.Fprintf(os.Stderr, "%d workers draining the queue. Submit runs with `loupe submit`.\n", *workers)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}

// stream subscribes a terminal renderer, runs fn, and tears down cleanly.
func stream(fn func(context.Context, *trace.Stream) (*run.Run, error)) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	s := trace.New()
	events, cancel := s.Subscribe()
	done := make(chan struct{})
	go func() {
		render.Terminal(os.Stdout, events)
		close(done)
	}()

	r, err := fn(ctx, s)

	cancel()
	<-done

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nrun did not finish cleanly: %v\n", err)
		return 1
	}
	fmt.Printf("\n%d steps traced. This is the trace a browser (and, later, Draw) will render live.\n", len(r.Steps))
	return 0
}

func dsn() string {
	if v := os.Getenv("LOUPE_DATABASE_URL"); v != "" {
		return v
	}
	return defaultDSN
}

func usage() {
	fmt.Fprint(os.Stderr, `loupe - run agents and watch every step they take

usage:
  loupe demo [--model scripted|ollama] [--pace 300ms]
       Run an agent that fixes a failing test, streamed step by step.

  loupe resume-demo [--pace 300ms]
       Kill the worker the instant it commits the fix and watch a second
       worker reclaim and finish the run. In-memory, no setup.

  loupe submit [--task "..."]
       Enqueue a run in the Postgres store (needs `+"`make db-up`"+`). Prints its id.

  loupe worker [--id name] [--model scripted|ollama] [--once]
       Claim and run work from the Postgres store, streamed live. Run more
       than one to share the queue; a crashed worker's run is resumed.

  loupe serve [--addr :8080] [--workers 2] [--model scripted|ollama]
       Serve the GraphQL subgraph + playground and a live SSE trace stream,
       with embedded workers draining the queue. Needs `+"`make db-up`"+`.
`)
}

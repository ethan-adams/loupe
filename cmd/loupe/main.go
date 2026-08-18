// Command loupe runs agents and shows every step they take. In M1 it has one
// subcommand, `demo`, which runs the fix-a-failing-test scenario and streams
// the trace to your terminal live. Later milestones add a durable run store, a
// GraphQL trace subgraph, and a web control room.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/ethan-adams/loupe/internal/demo"
	"github.com/ethan-adams/loupe/internal/render"
	"github.com/ethan-adams/loupe/internal/trace"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "demo":
		os.Exit(runDemo(os.Args[2:]))
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

	// Ctrl-C ends the run cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stream := trace.New()
	events, cancel := stream.Subscribe()

	done := make(chan struct{})
	go func() {
		render.Terminal(os.Stdout, events)
		close(done)
	}()

	r, err := demo.Run(ctx, stream, demo.Options{
		UseOllama: *modelName == "ollama",
		Pace:      *pace,
	})

	cancel() // closes the events channel; the renderer drains and returns
	<-done

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nrun did not finish cleanly: %v\n", err)
		return 1
	}
	fmt.Printf("\n%d steps traced. This is the trace a browser (and, later, Draw) will render live.\n", len(r.Steps))
	return 0
}

func usage() {
	fmt.Fprint(os.Stderr, `loupe - run agents and watch every step they take

usage:
  loupe demo [--model scripted|ollama] [--pace 300ms]

  demo   Run an agent that fixes a failing test, streamed step by step.
         Defaults to an offline scripted model so it works with no setup.
         Use --model ollama to drive it with a real local model.
`)
}

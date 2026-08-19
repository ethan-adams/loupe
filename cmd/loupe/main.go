// Command loupe runs agents and shows every step they take.
//
//	loupe demo         an agent fixes a failing test, streamed live
//	loupe resume-demo  the same run survives a worker crash and is resumed
//
// Later milestones add a Postgres-backed worker (`loupe worker`) and a web
// control room.
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
	"github.com/ethan-adams/loupe/internal/run"
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
	case "resume-demo":
		os.Exit(runResumeDemo(os.Args[2:]))
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

func usage() {
	fmt.Fprint(os.Stderr, `loupe - run agents and watch every step they take

usage:
  loupe demo [--model scripted|ollama] [--pace 300ms]
       Run an agent that fixes a failing test, streamed step by step.
       Defaults to an offline scripted model so it works with no setup.

  loupe resume-demo [--pace 300ms]
       Run the same work, kill the worker the instant it commits the fix,
       and watch a second worker reclaim and finish the run.
`)
}

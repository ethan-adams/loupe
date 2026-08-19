// Package render turns a stream of trace events into something a human watches.
// The terminal renderer here is the M1 stand-in for the live web control room:
// same events, plainer surface.
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/ethan-adams/loupe/internal/run"
	"github.com/ethan-adams/loupe/internal/trace"
)

// Terminal reads events until the channel closes and prints each one as a line
// in the run's timeline. It blocks, so run it in its own goroutine.
func Terminal(w io.Writer, events <-chan trace.Event) {
	for e := range events {
		switch e.Type {
		case trace.RunStarted:
			fmt.Fprintf(w, "\n▶ run %s\n  task: %s\n", shortID(e.RunID), e.Task)
		case trace.RunResumed:
			fmt.Fprintf(w, "\n↻ run %s resumed by another worker\n", shortID(e.RunID))
		case trace.StepAdded:
			fmt.Fprintln(w, stepLine(e.Step))
		case trace.RunEnded:
			if e.Status == run.StatusSucceeded {
				fmt.Fprintf(w, "\n✔ run %s succeeded\n", shortID(e.RunID))
			} else {
				fmt.Fprintf(w, "\n✘ run %s failed: %s\n", shortID(e.RunID), e.Error)
			}
		}
	}
}

func stepLine(s run.Step) string {
	switch s.Kind {
	case run.StepThink:
		return fmt.Sprintf("  \U0001F9E0 %s", s.Thought)
	case run.StepToolCall:
		return fmt.Sprintf("  ⚙  call %s(%s)", s.Tool, oneLine(s.Input, 48))
	case run.StepObserve:
		if s.IsError {
			return fmt.Sprintf("     ✗ %s error: %s", s.Tool, oneLine(s.Output, 80))
		}
		return fmt.Sprintf("     ✓ %s -> %s", s.Tool, oneLine(s.Output, 80))
	case run.StepFinal:
		return fmt.Sprintf("  ★ answer: %s", oneLine(s.Answer, 200))
	default:
		return fmt.Sprintf("  ? %s", s.Kind)
	}
}

// oneLine collapses a value to its first line and caps its length so a chatty
// tool result cannot blow up the timeline.
func oneLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " ..."
	}
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

package trace

import (
	"testing"

	"github.com/ethan-adams/loupe/internal/run"
)

func TestSubscriberReceivesLiveEvents(t *testing.T) {
	s := New()
	ch, cancel := s.Subscribe()
	defer cancel()

	s.Publish(Event{Type: RunStarted, RunID: "r1", Task: "t"})
	s.Publish(Event{Type: StepAdded, RunID: "r1", Step: run.Step{ID: 1, Kind: run.StepThink, Thought: "hi"}})

	if e := <-ch; e.Type != RunStarted {
		t.Fatalf("first event = %q, want run_started", e.Type)
	}
	if e := <-ch; e.Type != StepAdded || e.Step.Thought != "hi" {
		t.Fatalf("second event = %+v", e)
	}
}

func TestLateSubscriberReplaysHistory(t *testing.T) {
	s := New()
	s.Publish(Event{Type: RunStarted, RunID: "r1"})
	s.Publish(Event{Type: RunEnded, RunID: "r1", Status: run.StatusSucceeded})

	// Subscribing after the fact still replays the whole run: the seed of
	// scrubbing back through a finished run.
	ch, cancel := s.Subscribe()
	defer cancel()

	got := []EventType{(<-ch).Type, (<-ch).Type}
	if got[0] != RunStarted || got[1] != RunEnded {
		t.Fatalf("replay = %v, want [run_started run_ended]", got)
	}
}

func TestCancelClosesChannel(t *testing.T) {
	s := New()
	ch, cancel := s.Subscribe()
	cancel()
	if _, open := <-ch; open {
		t.Fatal("cancel should close the subscriber channel")
	}
}

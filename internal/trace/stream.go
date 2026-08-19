// Package trace is the visibility layer: every meaningful thing that happens in
// a run is published here as an Event, and any number of subscribers can watch
// it live. In M1 this is an in-process broker; the same interface is what a
// later milestone backs with Postgres plus a streaming endpoint so a browser
// (and, eventually, Draw) can watch a run the same way the terminal does.
package trace

import (
	"sync"

	"github.com/ethan-adams/loupe/internal/run"
)

// EventType distinguishes the lifecycle events on the stream.
type EventType string

const (
	RunStarted EventType = "run_started"
	RunResumed EventType = "run_resumed"
	StepAdded  EventType = "step_added"
	RunEnded   EventType = "run_ended"
)

// Event is one thing that happened in a run.
type Event struct {
	Type   EventType
	RunID  string
	Task   string     // set on RunStarted
	Status run.Status // set on RunStarted and RunEnded
	Step   run.Step   // set on StepAdded
	Error  string     // set on RunEnded when the run failed
}

// Stream fans events out to every subscriber and keeps a history so a late
// subscriber can replay a run from the beginning. That replay is the seed of
// the "scrub back through a finished run" story.
type Stream struct {
	mu   sync.Mutex
	subs []chan Event
	hist []Event
}

// New returns an empty stream.
func New() *Stream { return &Stream{} }

// Subscribe returns a channel of events and a cancel func. The channel first
// replays everything published so far, then receives new events live. Call
// cancel when done; it closes the channel.
func (s *Stream) Subscribe() (<-chan Event, func()) {
	s.mu.Lock()
	// Size the buffer to hold the full replay plus headroom so Subscribe never
	// blocks while holding the lock.
	ch := make(chan Event, len(s.hist)+256)
	for _, e := range s.hist {
		ch <- e
	}
	s.subs = append(s.subs, ch)
	s.mu.Unlock()

	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, c := range s.subs {
			if c == ch {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, cancel
}

// Publish records an event and delivers it to every current subscriber. A
// subscriber whose buffer is full is skipped rather than blocking the run;
// back-pressure done properly is a later milestone, and this keeps M1 honest
// about that being a stub.
func (s *Stream) Publish(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hist = append(s.hist, e)
	for _, ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

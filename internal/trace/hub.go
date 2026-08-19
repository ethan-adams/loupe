package trace

import "sync"

// Hub routes events to a per-run Stream, so a server can host many runs at once
// and a watcher can subscribe to just the run it cares about. Each run gets its
// own Stream (history + live fan-out); the Hub is only the lookup in front.
type Hub struct {
	mu      sync.Mutex
	streams map[string]*Stream
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{streams: map[string]*Stream{}} }

// Stream returns the Stream for a run, creating it on first use. Hand this to an
// agent as its stream so its steps land on the right run's channel.
func (h *Hub) Stream(runID string) *Stream {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.streams[runID]
	if s == nil {
		s = New()
		h.streams[runID] = s
	}
	return s
}

// Subscribe watches one run: it replays that run's history, then delivers new
// events live. The cancel func closes the subscription.
func (h *Hub) Subscribe(runID string) (<-chan Event, func()) {
	return h.Stream(runID).Subscribe()
}

// Publish routes an event to its run's Stream.
func (h *Hub) Publish(e Event) { h.Stream(e.RunID).Publish(e) }

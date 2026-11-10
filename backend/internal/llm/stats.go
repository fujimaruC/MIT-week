package llm

import "sync/atomic"

// Stats counts gateway calls without recording any content. It feeds
// GET /api/v1/ops/status.
type Stats struct {
	Calls    atomic.Int64
	Failures atomic.Int64
	// LastReason is a fixed, content-free failure label.
	LastReason atomic.Value // string
}

func (s *Stats) fail(label string) {
	s.Failures.Add(1)
	s.LastReason.Store(label)
}

// Snapshot returns counters safe to serialise.
func (s *Stats) Snapshot() map[string]any {
	last, _ := s.LastReason.Load().(string)
	return map[string]any{
		"calls":       s.Calls.Load(),
		"failures":    s.Failures.Load(),
		"last_reason": last,
	}
}

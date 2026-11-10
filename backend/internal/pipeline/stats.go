package pipeline

import "sync/atomic"

// Stats counts audits without recording any content. It feeds
// GET /api/v1/ops/status.
type Stats struct {
	Audits   atomic.Int64
	Degraded atomic.Int64
	CacheHit atomic.Int64
}

// Snapshot returns counters safe to serialise.
func (s *Stats) Snapshot() map[string]any {
	return map[string]any{
		"audits":     s.Audits.Load(),
		"degraded":   s.Degraded.Load(),
		"cache_hits": s.CacheHit.Load(),
	}
}

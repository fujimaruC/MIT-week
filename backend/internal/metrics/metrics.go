// Package metrics keeps anonymous aggregate counters (TECH_SPEC §7.6) and
// exposes them in the Prometheus text format on a separate internal
// listener. Labels are fixed enums (input type, status, source): no URL,
// text, title, IP or token ever becomes a label value.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry holds every counter. The zero value is not usable; call New.
type Registry struct {
	mu        sync.Mutex
	counters  map[string]map[string]float64 // name -> labels -> value
	help      map[string]string
	durations map[string]*histogram
	started   time.Time
}

type histogram struct {
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

// New builds a registry with the counters AMBA reports.
func New() *Registry {
	r := &Registry{
		counters:  map[string]map[string]float64{},
		help:      map[string]string{},
		durations: map[string]*histogram{},
		started:   time.Now(),
	}
	r.help["amba_audits_total"] = "Completed audits by input type and verdict status."
	r.help["amba_audits_degraded_total"] = "Audits where at least one pillar failed."
	r.help["amba_rate_limited_total"] = "Requests refused by the rate limiter or the in-flight cap."
	r.help["amba_verifications_total"] = "Share-token verifications by result."
	r.help["amba_telegram_updates_total"] = "Telegram updates handled, by kind."
	r.help["amba_floors_total"] = "Verdict floors applied, by floor code."
	r.help["amba_link_reasons_total"] = "Link reasons reported, by reason code."
	r.durations["amba_audit_duration_seconds"] = &histogram{buckets: []float64{0.5, 1, 2, 3.5, 5, 8, 12}}
	return r
}

var allowedLabel = func(v string) string {
	// Labels are enums; anything unexpected collapses to "other" so a bug
	// can never leak user content into the metrics endpoint.
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return "other"
		}
	}
	if v == "" || len(v) > 24 {
		return "other"
	}
	return v
}

// Inc adds one to a counter with the given label pairs ("key", "value", ...).
func (r *Registry) Inc(name string, labels ...string) {
	if r == nil {
		return
	}
	key := labelKey(labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counters[name] == nil {
		r.counters[name] = map[string]float64{}
	}
	r.counters[name][key]++
}

// Observe records an audit duration.
func (r *Registry) Observe(name string, d time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.durations[name]
	if h == nil {
		return
	}
	if h.counts == nil {
		h.counts = make([]uint64, len(h.buckets))
	}
	s := d.Seconds()
	for i, b := range h.buckets {
		if s <= b {
			h.counts[i]++
		}
	}
	h.sum += s
	h.count++
}

func labelKey(labels []string) string {
	if len(labels)%2 != 0 {
		labels = labels[:len(labels)-1]
	}
	parts := make([]string, 0, len(labels)/2)
	for i := 0; i < len(labels); i += 2 {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, allowedLabel(labels[i]), allowedLabel(labels[i+1])))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// Render writes the Prometheus text exposition format.
func (r *Registry) Render(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.help))
	for n := range r.help {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", name, r.help[name], name)
		keys := make([]string, 0, len(r.counters[name]))
		for k := range r.counters[name] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "" {
				fmt.Fprintf(w, "%s %g\n", name, r.counters[name][k])
			} else {
				fmt.Fprintf(w, "%s{%s} %g\n", name, k, r.counters[name][k])
			}
		}
	}
	for name, h := range r.durations {
		fmt.Fprintf(w, "# HELP %s Audit wall time.\n# TYPE %s histogram\n", name, name)
		for i, b := range h.buckets {
			var c uint64
			if h.counts != nil {
				c = h.counts[i]
			}
			fmt.Fprintf(w, "%s_bucket{le=\"%g\"} %d\n", name, b, c)
		}
		fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n%s_sum %g\n%s_count %d\n", name, h.count, name, h.sum, name, h.count)
	}
	fmt.Fprintf(w, "# HELP amba_uptime_seconds Seconds since the process started.\n# TYPE amba_uptime_seconds gauge\namba_uptime_seconds %d\n", int(time.Since(r.started).Seconds()))
}

// Handler serves the registry. Mount it only on the internal listener.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/metrics" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Cache-Control", "no-store")
		r.Render(w)
	})
}

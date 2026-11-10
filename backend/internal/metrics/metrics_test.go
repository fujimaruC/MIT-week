package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCountersAndExposition(t *testing.T) {
	r := New()
	r.Inc("amba_audits_total", "input_type", "URL", "status", "TAMPERED")
	r.Inc("amba_audits_total", "input_type", "URL", "status", "TAMPERED")
	r.Observe("amba_audit_duration_seconds", 1500*time.Millisecond)

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`amba_audits_total{input_type="URL",status="TAMPERED"} 2`,
		`amba_audit_duration_seconds_bucket{le="2"} 1`,
		`amba_audit_duration_seconds_count 1`,
		"# TYPE amba_uptime_seconds gauge",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
}

// A label value that is not a short identifier can never reach the output.
func TestLabelsCannotCarryUserContent(t *testing.T) {
	r := New()
	r.Inc("amba_audits_total", "input_type", "https://evil.example/?q=secret", "status", "SAFE")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(rec.Body.String(), "evil.example") || !strings.Contains(rec.Body.String(), `input_type="other"`) {
		t.Fatalf("label leaked: %s", rec.Body.String())
	}
}

func TestNilRegistryIsSafe(t *testing.T) {
	var r *Registry
	r.Inc("x")
	r.Observe("x", time.Second)
}

package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPISecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	APISecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Cache-Control":           "no-store",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	cfg := NewCORSConfig("https://amba.example", false)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://amba.example")

	cfg.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://amba.example" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatal("credentials must never be allowed")
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	cfg := NewCORSConfig("https://amba.example", false)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://evil.example")

	cfg.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unknown origin was allowed: %q", got)
	}
}

func TestCORSDevelopmentLocalhost(t *testing.T) {
	cfg := NewCORSConfig("http://localhost:5173", true)
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173"} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Origin", origin)
		cfg.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})).ServeHTTP(rec, r)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %q not allowed (got %q)", origin, got)
		}
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	cfg := NewCORSConfig("https://amba.example", false)
	var reached bool
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", "https://amba.example")

	cfg.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	})).ServeHTTP(rec, r)

	if reached {
		t.Fatal("preflight reached the handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestParseTrustedProxiesSkipsInvalid(t *testing.T) {
	nets := ParseTrustedProxies([]string{"10.0.0.0/8", "not-a-cidr", " 192.168.0.0/16 "})
	if len(nets) != 2 {
		t.Fatalf("parsed %d networks, want 2", len(nets))
	}
}

func TestCORSDevelopmentWildcardAllowsAnyLocalPort(t *testing.T) {
	cfg := NewCORSConfig("", true)
	for _, origin := range []string{
		"http://localhost:5173",
		"http://localhost:14202",
		"http://127.0.0.1:4173",
	} {
		if !cfg.allowed(origin) {
			t.Errorf("dev origin %q should be allowed", origin)
		}
	}
}

func TestCORSDevelopmentWildcardRejectsForeignHosts(t *testing.T) {
	cfg := NewCORSConfig("", true)
	for _, origin := range []string{
		"http://localhost.evil.example:5173",
		"http://evil.example:5173",
		"http://localhost:abc",
		"https://localhost:5173",
	} {
		if cfg.allowed(origin) {
			t.Errorf("origin %q must not be allowed", origin)
		}
	}
}

func TestCORSProductionHasNoWildcard(t *testing.T) {
	cfg := NewCORSConfig("https://amba.example", false)
	if cfg.allowed("http://localhost:5173") {
		t.Error("production must not allow localhost")
	}
	if !cfg.allowed("https://amba.example") {
		t.Error("the configured app origin must be allowed")
	}
}

func TestCORSExposesRetryAfter(t *testing.T) {
	cfg := NewCORSConfig("https://amba.example", false)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://amba.example")
	cfg.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, r)
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "Retry-After" {
		t.Fatalf("expose-headers = %q, want Retry-After", got)
	}
}

func TestCORSExtraOriginsAreExact(t *testing.T) {
	cfg := NewCORSConfig("https://amba.example", false).WithOrigins("https://amba.vercel.app", "https://evil.example:*")
	if !cfg.allowed("https://amba.vercel.app") || !cfg.allowed("https://amba.example") {
		t.Fatal("configured origins must be allowed")
	}
	if cfg.allowed("https://evil.example:443") || cfg.allowed("https://preview-amba.vercel.app") {
		t.Fatal("extra origins must be exact; wildcards are refused")
	}
}

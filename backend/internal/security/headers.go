package security

import (
	"net"
	"net/http"
	"strings"
)

// APISecurityHeaders applies the JSON-only API policy from TECH_SPEC §7.4.
func APISecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// CORSConfig lists the origins allowed to call the API.
type CORSConfig struct {
	AllowedOrigins []string
}

// NewCORSConfig builds the allowlist: PUBLIC_APP_URL plus localhost in dev.
// No credentials are ever allowed, so wildcard-with-credentials mistakes are
// structurally impossible here.
func NewCORSConfig(publicAppURL string, development bool) CORSConfig {
	origins := []string{}
	if strings.TrimSpace(publicAppURL) != "" {
		origins = append(origins, strings.TrimRight(publicAppURL, "/"))
	}
	if development {
		// Any localhost port: dev, preview and Playwright all bind different
		// ports, and pinning one makes CORS failures look like invalid tokens.
		origins = append(origins, "http://localhost:*", "http://127.0.0.1:*")
	}
	return CORSConfig{AllowedOrigins: origins}
}

// WithOrigins returns a copy that also allows the given exact origins.
func (c CORSConfig) WithOrigins(extra ...string) CORSConfig {
	out := CORSConfig{AllowedOrigins: append([]string{}, c.AllowedOrigins...)}
	for _, o := range extra {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" && !strings.HasSuffix(o, ":*") {
			out.AllowedOrigins = append(out.AllowedOrigins, o)
		}
	}
	return out
}

func (c CORSConfig) allowed(origin string) bool {
	origin = strings.TrimRight(origin, "/")
	for _, o := range c.AllowedOrigins {
		if o == origin {
			return true
		}
		// Development wildcard: "http://localhost:*" matches any port on that
		// host only. Never used in production.
		if strings.HasSuffix(o, ":*") {
			prefix := strings.TrimSuffix(o, "*")
			if strings.HasPrefix(origin, prefix) {
				rest := strings.TrimPrefix(origin, prefix)
				if rest != "" && isAllDigits(rest) {
					return true
				}
			}
		}
	}
	return false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Middleware answers preflights and sets CORS headers for allowed origins only.
func (c CORSConfig) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && c.allowed(origin) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			// Without Expose-Headers a cross-origin script cannot read the
			// rate-limit countdown, and the UI falls back to a guessed value.
			h.Set("Access-Control-Expose-Headers", "Retry-After")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			// Preflight never reaches the handler, allowed or not.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ParseTrustedProxies converts CIDR strings into networks, skipping bad input
// rather than failing the whole server at request time.
func ParseTrustedProxies(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if strings.TrimSpace(c) == "*" {
			// Only for hosts that are reachable exclusively through their
			// proxy (Fly.io, Railway): every peer is that proxy.
			_, v4, _ := net.ParseCIDR("0.0.0.0/0")
			_, v6, _ := net.ParseCIDR("::/0")
			out = append(out, v4, v6)
			continue
		}
		if _, n, err := net.ParseCIDR(strings.TrimSpace(c)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

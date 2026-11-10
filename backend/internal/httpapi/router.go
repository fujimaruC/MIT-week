package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amba/amba/backend/internal/config"
	"github.com/amba/amba/backend/internal/security"
)

// NewRouter builds the API router with freshly built services.
func NewRouter(cfg *config.Config) http.Handler {
	return NewRouterWith(cfg, NewDeps(cfg), nil)
}

// NewRouterWith builds the API router around shared services. extra, when
// non-nil, mounts additional routes (the Telegram webhook) under the same
// headers, recovery and rate limit.
func NewRouterWith(cfg *config.Config, deps Deps, extra func(chi.Router)) http.Handler {
	r := chi.NewRouter()

	r.Use(security.APISecurityHeaders)
	r.Use(recoverer)

	// Per client IP. Judges in one room often share a campus NAT, so the
	// per-IP budget is generous; MaxInFlight still caps the model load.
	limiter := security.NewRateLimiter(security.RateLimitConfig{
		PerMinute:      30,
		Burst:          10,
		MaxInFlight:    32,
		IdleTTL:        10 * time.Minute,
		SweepInterval:  5 * time.Minute,
		TrustedProxies: security.ParseTrustedProxies(cfg.TrustedProxy),
		ClientIPHeader: cfg.ClientIPHeader,
	})
	cors := security.NewCORSConfig(cfg.PublicAppURL, !cfg.IsProduction()).WithOrigins(cfg.ExtraOrigins...)
	h := newHandlers(cfg, limiter, deps)

	r.Use(cors.Middleware)
	r.Use(limiter.Middleware(func(w http.ResponseWriter, _ *http.Request) {
		deps.Metrics.Inc("amba_rate_limited_total", "reason", "client")
		writeError(w, errRateLimited)
	}))

	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)

	r.Get("/healthz", health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", health)
		r.Get("/ops/status", h.getOpsStatus)
		r.Get("/demo/{id}", getDemo)
		r.Post("/audit/url", h.auditURL)
		r.Post("/audit/file", h.auditFile)
		r.Post("/verify", h.verify)
	})
	if extra != nil {
		extra(r)
	}

	return r
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

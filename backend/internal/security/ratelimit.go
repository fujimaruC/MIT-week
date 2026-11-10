package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig describes the per-client and global limits.
type RateLimitConfig struct {
	PerMinute      int           // sustained requests per minute per client
	Burst          int           // immediate burst allowance
	MaxInFlight    int           // global concurrent audits
	IdleTTL        time.Duration // evict a client after this long unused
	SweepInterval  time.Duration
	TrustedProxies []*net.IPNet // peers allowed to set the forwarded-client header
	// ClientIPHeader is the header a trusted proxy writes the client address
	// to. Empty means X-Forwarded-For.
	ClientIPHeader string
}

// DefaultRateLimitConfig returns the limits from TECH_SPEC §7.3.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		PerMinute:     15,
		Burst:         5,
		MaxInFlight:   32,
		IdleTTL:       10 * time.Minute,
		SweepInterval: 5 * time.Minute,
	}
}

type clientEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter tracks per-client token buckets keyed by a salted hash of the
// client identity. The raw identity is never stored or logged.
type RateLimiter struct {
	cfg  RateLimitConfig
	salt []byte

	mu       sync.Mutex
	clients  map[string]*clientEntry
	inFlight int
	stop     chan struct{}
	once     sync.Once
}

// NewRateLimiter starts the background sweeper.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		panic("security: cannot read entropy for rate-limit salt")
	}
	rl := &RateLimiter{
		cfg:     cfg,
		salt:    salt,
		clients: make(map[string]*clientEntry),
		stop:    make(chan struct{}),
	}
	go rl.sweep()
	return rl
}

// Close stops the sweeper.
func (rl *RateLimiter) Close() {
	rl.once.Do(func() { close(rl.stop) })
}

func (rl *RateLimiter) sweep() {
	interval := rl.cfg.SweepInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case now := <-t.C:
			rl.mu.Lock()
			for k, e := range rl.clients {
				if now.Sub(e.lastSeen) > rl.cfg.IdleTTL {
					delete(rl.clients, k)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// ClientKey derives the bucket key. The forwarded header is honoured only when
// the immediate peer is a configured trusted proxy, so a client cannot spoof
// its way into a fresh bucket.
func (rl *RateLimiter) ClientKey(r *http.Request) string {
	identity := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		identity = host
	}
	if rl.peerIsTrusted(r.RemoteAddr) {
		if forwarded := forwardedClient(r, rl.cfg.ClientIPHeader); forwarded != "" {
			identity = forwarded
		}
	}
	sum := sha256.Sum256(append(rl.salt, []byte(identity)...))
	return hex.EncodeToString(sum[:16])
}

// forwardedClient reads the client address a trusted proxy reported. For
// X-Forwarded-For the RIGHTMOST entry is used: it is the one our proxy
// appended, while everything to its left came from the client and can be
// forged. A dedicated header (Fly-Client-IP) is overwritten by the proxy and
// used as is. Anything that is not an IP address is ignored.
func forwardedClient(r *http.Request, header string) string {
	if header == "" || strings.EqualFold(header, "X-Forwarded-For") {
		values := r.Header.Values("X-Forwarded-For")
		if len(values) == 0 {
			return ""
		}
		parts := strings.Split(values[len(values)-1], ",")
		candidate := strings.TrimSpace(parts[len(parts)-1])
		if net.ParseIP(candidate) != nil {
			return candidate
		}
		return ""
	}
	candidate := strings.TrimSpace(r.Header.Get(header))
	if net.ParseIP(candidate) != nil {
		return candidate
	}
	return ""
}

func (rl *RateLimiter) peerIsTrusted(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range rl.cfg.TrustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Allow consumes one token for the client. It reports whether the request may
// proceed and, when it may not, how long to wait.
func (rl *RateLimiter) Allow(key string) (bool, time.Duration) {
	rl.mu.Lock()
	e, ok := rl.clients[key]
	if !ok {
		e = &clientEntry{limiter: rate.NewLimiter(rate.Limit(float64(rl.cfg.PerMinute)/60), rl.cfg.Burst)}
		rl.clients[key] = e
	}
	e.lastSeen = time.Now()
	rl.mu.Unlock()

	if e.limiter.Allow() {
		return true, 0
	}
	return false, e.limiter.Reserve().Delay()
}

// AcquireInFlight reserves a global concurrency slot.
func (rl *RateLimiter) AcquireInFlight() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.cfg.MaxInFlight > 0 && rl.inFlight >= rl.cfg.MaxInFlight {
		return false
	}
	rl.inFlight++
	return true
}

// ReleaseInFlight returns a slot.
func (rl *RateLimiter) ReleaseInFlight() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.inFlight > 0 {
		rl.inFlight--
	}
}

// Middleware enforces the per-client limit. When a request is refused it sets
// Retry-After and delegates to onLimited so the caller keeps a single error
// envelope rather than this package inventing a second response shape.
func (rl *RateLimiter) Middleware(onLimited http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health probes come from the platform every few seconds and must
			// never be throttled (or count against a shared proxy address).
			if strings.HasSuffix(r.URL.Path, "/healthz") {
				next.ServeHTTP(w, r)
				return
			}
			ok, delay := rl.Allow(rl.ClientKey(r))
			if !ok {
				secs := int(delay.Seconds()) + 1
				w.Header().Set("Retry-After", itoa(secs))
				onLimited(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func itoa(n int) string {
	if n <= 0 {
		return "1"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

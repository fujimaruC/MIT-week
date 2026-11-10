package linkcheck

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Domain age (TECH_SPEC §5): phishing and hoax sites are usually registered
// days or weeks before they are used, while the sites people trust are years
// old. RDAP, the registries' structured successor to WHOIS, answers when a
// domain was registered. Only the registrable domain (e.g. "contoh.my.id")
// is sent, never the path or the user's text, and answers are not kept.
const (
	// NewDomainDays and YoungDomainDays are the age limits for NEW_DOMAIN and
	// YOUNG_DOMAIN.
	NewDomainDays   = 30
	YoungDomainDays = 180

	// DefaultRDAPBootstrap is IANA's list of RDAP servers per TLD.
	DefaultRDAPBootstrap = "https://data.iana.org/rdap/dns.json"
	bootstrapTTL         = 24 * time.Hour
)

// ErrNoRDAP means the domain's registry offers no RDAP service or the
// answer carried no registration date. The caller adds nothing.
var ErrNoRDAP = errors.New("no rdap registration date")

// RDAP looks up registration dates. The zero value is disabled.
type RDAP struct {
	// HTTP must be the SSRF-safe client: the registry URL comes from IANA,
	// but the defence costs nothing.
	HTTP      *http.Client
	Bootstrap string

	mu      sync.Mutex
	servers map[string]string // TLD -> RDAP base URL ending in "/"
	fetched time.Time
}

// NewRDAP builds an enabled lookup around client.
func NewRDAP(client *http.Client) *RDAP {
	return &RDAP{HTTP: client, Bootstrap: DefaultRDAPBootstrap}
}

// Enabled reports whether lookups will be attempted.
func (r *RDAP) Enabled() bool { return r != nil && r.HTTP != nil && r.Bootstrap != "" }

// Registered returns when the registrable domain of host was registered.
func (r *RDAP) Registered(ctx context.Context, host string) (time.Time, error) {
	if !r.Enabled() {
		return time.Time{}, ErrNoRDAP
	}
	domain := Registrable(strings.TrimSuffix(host, "."))
	if domain == "" || isIPHost(domain) || !strings.Contains(domain, ".") {
		return time.Time{}, ErrNoRDAP
	}
	base, err := r.server(ctx, lastLabel(domain))
	if err != nil {
		return time.Time{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"domain/"+url.PathEscape(domain), nil)
	if err != nil {
		return time.Time{}, ErrNoRDAP
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := r.HTTP.Do(req)
	if err != nil {
		logRDAP(ctx, 0)
		return time.Time{}, ErrNoRDAP
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 404: not registered (or not served); 429: rate limited.
		logRDAP(ctx, resp.StatusCode)
		return time.Time{}, ErrNoRDAP
	}
	var body struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&body); err != nil {
		return time.Time{}, ErrNoRDAP
	}
	for _, e := range body.Events {
		if e.Action != "registration" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
			return t, nil
		}
	}
	return time.Time{}, ErrNoRDAP
}

// server finds the RDAP base URL for a TLD from IANA's bootstrap file,
// fetched at most once a day.
func (r *RDAP) server(ctx context.Context, tld string) (string, error) {
	r.mu.Lock()
	fresh := r.servers != nil && time.Since(r.fetched) < bootstrapTTL
	base, ok := r.servers[tld]
	r.mu.Unlock()
	if fresh {
		if !ok {
			return "", ErrNoRDAP
		}
		return base, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.Bootstrap, nil)
	if err != nil {
		return "", ErrNoRDAP
	}
	resp, err := r.HTTP.Do(req)
	if err != nil {
		logRDAP(ctx, 0)
		return "", ErrNoRDAP
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		logRDAP(ctx, resp.StatusCode)
		return "", ErrNoRDAP
	}
	var boot struct {
		Services [][][]string `json:"services"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&boot); err != nil {
		return "", ErrNoRDAP
	}
	servers := map[string]string{}
	for _, svc := range boot.Services {
		if len(svc) != 2 {
			continue
		}
		url := ""
		for _, u := range svc[1] {
			if strings.HasPrefix(u, "https://") {
				url = u
				break
			}
		}
		if url == "" {
			continue
		}
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		for _, t := range svc[0] {
			servers[strings.ToLower(t)] = url
		}
	}
	r.mu.Lock()
	r.servers, r.fetched = servers, time.Now()
	r.mu.Unlock()
	if base, ok := servers[tld]; ok {
		return base, nil
	}
	return "", ErrNoRDAP
}

// logRDAP says why a lookup failed: never the domain, only the reason.
func logRDAP(ctx context.Context, status int) {
	reason := "transport"
	switch {
	case ctx.Err() != nil:
		reason = "timeout"
	case status != 0:
		reason = "http_status"
	}
	slog.Warn("rdap call failed", "reason", reason, "status", status)
}

// WithDomainAge adds NEW_DOMAIN (registered within NewDomainDays, 25) or
// YOUNG_DOMAIN (within YoungDomainDays, 10). A young domain is common for
// phishing and hoax sites but not proof: shops and campaigns start somewhere,
// so it is never enough on its own for a link-only verdict.
func WithDomainAge(r Result, registered, now time.Time) Result {
	if registered.IsZero() || registered.After(now) || r.Host == "" || IsOfficialHost(r.Host) {
		return r
	}
	days := int(now.Sub(registered).Hours() / 24)
	var f Finding
	switch {
	case days <= NewDomainDays:
		f = Finding{Code: ReasonNewDomain, Weight: 25}
	case days <= YoungDomainDays:
		f = Finding{Code: ReasonYoungDomain, Weight: 10}
	default:
		return r
	}
	for _, have := range r.Findings {
		if have.Code == ReasonNewDomain || have.Code == ReasonYoungDomain {
			return r
		}
	}
	r.Findings = append(r.Findings, f)
	r.Score = capScore(r.Score + f.Weight)
	return r
}

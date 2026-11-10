package linkcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// DefaultSafeBrowsingEndpoint is Google Safe Browsing v4 Lookup.
const DefaultSafeBrowsingEndpoint = "https://safebrowsing.googleapis.com/v4/threatMatches:find"

// SafeBrowsing is an optional reputation lookup (TECH_SPEC §5 SAFE_BROWSING).
// It sends only the URLs being checked, never user text, and it is disabled
// when no key is configured.
type SafeBrowsing struct {
	Key      string
	Endpoint string
	HTTP     *http.Client
}

// NewSafeBrowsing builds a client; an empty key yields a disabled client.
func NewSafeBrowsing(key string) *SafeBrowsing {
	return &SafeBrowsing{
		Key:      key,
		Endpoint: DefaultSafeBrowsingEndpoint,
		HTTP:     &http.Client{Timeout: 2 * time.Second},
	}
}

// Enabled reports whether lookups will be attempted.
func (s *SafeBrowsing) Enabled() bool { return s != nil && s.Key != "" }

// ErrLookupFailed means the reputation service did not answer usefully. The
// caller treats it as "not checked", never as "clean".
var ErrLookupFailed = errors.New("safe browsing lookup failed")

// Check reports whether any of urls is on a Google threat list.
func (s *SafeBrowsing) Check(ctx context.Context, urls []string) (bool, error) {
	if !s.Enabled() || len(urls) == 0 {
		return false, nil
	}
	entries := make([]map[string]string, 0, len(urls))
	seen := map[string]bool{}
	for _, u := range urls {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		entries = append(entries, map[string]string{"url": u})
	}
	body, err := json.Marshal(map[string]any{
		"client": map[string]string{"clientId": "amba", "clientVersion": "1.0"},
		"threatInfo": map[string]any{
			"threatTypes":      []string{"MALWARE", "SOCIAL_ENGINEERING", "UNWANTED_SOFTWARE", "POTENTIALLY_HARMFUL_APPLICATION"},
			"platformTypes":    []string{"ANY_PLATFORM"},
			"threatEntryTypes": []string{"URL"},
			"threatEntries":    entries,
		},
	})
	if err != nil {
		return false, ErrLookupFailed
	}
	endpoint := s.Endpoint + "?key=" + url.QueryEscape(s.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, ErrLookupFailed
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		// Never log err: it quotes the URL, which holds the key.
		reason := "transport"
		if ctx.Err() != nil {
			reason = "timeout"
		}
		slog.Warn("safe browsing call failed", "reason", reason, "status", 0)
		return false, ErrLookupFailed
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 403 usually means the Safe Browsing API is not enabled for the key.
		slog.Warn("safe browsing call failed", "reason", "http_status", "status", resp.StatusCode)
		return false, ErrLookupFailed
	}
	var out struct {
		Matches []json.RawMessage `json:"matches"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&out); err != nil {
		return false, ErrLookupFailed
	}
	return len(out.Matches) > 0, nil
}

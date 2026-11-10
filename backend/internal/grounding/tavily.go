package grounding

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultTavilyEndpoint is Tavily's web search (docs.tavily.com, POST, Bearer key).
const DefaultTavilyEndpoint = "https://api.tavily.com/search"

// TavilySearcher finds fact-check articles with Tavily web search, restricted
// to the allowlisted hosts (Tavily's free plan: 1,000 basic searches a month).
// Tavily returns no ClaimReview rating, so the rating hint is the verdict tag
// Indonesian fact-checkers put at the start of a title ("[HOAKS]", "[SALAH]",
// "[KLARIFIKASI]"); like Google's rating it is a hint for the classifier only.
type TavilySearcher struct {
	Key string
	// Backup is tried in order once a key's credits are used up (HTTP 432 or
	// 433) or the key is refused (401); the searcher then stays on the key
	// that answered, so a spent key costs one extra request, not one per search.
	Backup     []string
	active     atomic.Int32
	Endpoint   string
	HTTP       *http.Client
	Grounder   *Grounder
	Domains    []string
	MaxResults int
	// Prefix is put in front of every query ("cek fakta " by default). The
	// web provenance search sets it to "" and sends its own wording.
	Prefix string
}

// NewTavilySearcher builds a searcher; an empty key disables it.
func NewTavilySearcher(key string, g *Grounder, domains []string) *TavilySearcher {
	return &TavilySearcher{
		Key:        key,
		Endpoint:   DefaultTavilyEndpoint,
		HTTP:       &http.Client{Timeout: 8 * time.Second},
		Grounder:   g,
		Domains:    domains,
		MaxResults: 5,
		Prefix:     "cek fakta ",
	}
}

// Enabled reports whether searches will be attempted.
func (s *TavilySearcher) Enabled() bool { return s != nil && len(s.keys()) > 0 }

// keys is the primary key, then the backups, without empty ones.
func (s *TavilySearcher) keys() []string {
	out := make([]string, 0, 1+len(s.Backup))
	for _, k := range append([]string{s.Key}, s.Backup...) {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// spent is a status after which the next key is worth trying: the key was
// refused, or its plan or pay-as-you-go credits are used up.
func spent(status int) bool {
	return status == http.StatusUnauthorized || status == 432 || status == 433
}

var titleTag = regexp.MustCompile(`^\s*\[([^\]]{2,40})\]`)

// Search returns allowlisted fact-check articles for query.
func (s *TavilySearcher) Search(ctx context.Context, query string) ([]Result, error) {
	if !s.Enabled() {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if r := []rune(query); len(r) > 200 {
		query = string(r[:200])
	}
	payload, err := json.Marshal(map[string]any{
		"query":           s.Prefix + query,
		"search_depth":    "basic",
		"max_results":     10,
		"include_domains": s.Domains,
		"include_answer":  false,
	})
	if err != nil {
		return nil, ErrSearchFailed
	}
	keys := s.keys()
	var resp *http.Response
	for i := int(s.active.Load()); i < len(keys); i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, ErrSearchFailed
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+keys[i])
		resp, err = s.HTTP.Do(req)
		if err != nil {
			// Never log err or the query: the claim is user content.
			slog.Warn("tavily search call failed", "reason", failureReason(ctx), "status", 0)
			return nil, ErrSearchFailed
		}
		if spent(resp.StatusCode) && i+1 < len(keys) {
			_ = resp.Body.Close()
			// Never log a key, only its place in the list.
			slog.Warn("tavily key spent, using the next one", "status", resp.StatusCode, "next_key", i+2)
			s.active.CompareAndSwap(int32(i), int32(i+1))
			continue
		}
		break
	}
	if resp == nil {
		return nil, ErrSearchFailed
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 401 a bad key; 429 too many requests; 432/433 the plan's credits are used up.
		slog.Warn("tavily search call failed", "reason", "http_status", "status", resp.StatusCode)
		return nil, ErrSearchFailed
	}
	var body struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, ErrSearchFailed
	}

	limit := s.MaxResults
	if limit <= 0 {
		limit = 5
	}
	seen := map[string]bool{}
	out := make([]Result, 0, limit)
	for _, r := range body.Results {
		if len(out) >= limit {
			break
		}
		if seen[r.URL] || s.Grounder == nil || !s.Grounder.HostAllowed(r.URL) {
			continue
		}
		seen[r.URL] = true
		title := strings.TrimSpace(r.Title)
		rating := ""
		if m := titleTag.FindStringSubmatch(title); m != nil {
			rating = "[" + strings.TrimSpace(m[1]) + "]"
		}
		out = append(out, Result{
			Title:     truncate(title, 200),
			Publisher: truncate(publisherOf(r.URL), 80),
			URL:       r.URL,
			Snippet:   truncate(strings.TrimSpace(r.Content), 300),
			Rating:    truncate(rating, 60),
		})
	}
	return out, nil
}

func publisherOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

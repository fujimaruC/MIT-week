package grounding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Result is one search hit. Only results on the allowlist ever leave the
// searcher, and their URLs are the only ones a source may carry.
type Result struct {
	Title     string
	Publisher string
	URL       string
	Snippet   string
	// Rating is the fact-checker's own verdict text ("Salah", "Hoaks"). It
	// describes the reviewed claim, which may differ from the user's claim,
	// so it is a hint for the classifier, never a verdict on its own.
	Rating string
}

// Searcher finds fact-checks for a claim (TECH_SPEC §6 search_verified_news).
type Searcher interface {
	Enabled() bool
	Search(ctx context.Context, query string) ([]Result, error)
}

// ErrSearchFailed means the search service did not answer usefully.
var ErrSearchFailed = errors.New("search failed")

// DefaultFactCheckEndpoint is the Google Fact Check Tools claims search.
const DefaultFactCheckEndpoint = "https://factchecktools.googleapis.com/v1alpha1/claims:search"

// FactCheckSearcher queries Google Fact Check Tools, which indexes ClaimReview
// articles from Indonesian fact-checkers (turnbackhoax.id, cekfakta.com,
// Kompas, Tempo, Liputan6, Tirto...). It sends the extracted claim only.
type FactCheckSearcher struct {
	Key      string
	Endpoint string
	Language string
	HTTP     *http.Client
	Grounder *Grounder
	// MaxResults bounds what reaches the model and the UI.
	MaxResults int
}

// NewFactCheckSearcher builds a searcher; an empty key disables it.
func NewFactCheckSearcher(key string, g *Grounder) *FactCheckSearcher {
	return &FactCheckSearcher{
		Key:        key,
		Endpoint:   DefaultFactCheckEndpoint,
		Language:   "id",
		HTTP:       &http.Client{Timeout: 3 * time.Second},
		Grounder:   g,
		MaxResults: 5,
	}
}

// Enabled reports whether searches will be attempted.
func (s *FactCheckSearcher) Enabled() bool { return s != nil && s.Key != "" }

// Search returns allowlisted fact-check results for query.
func (s *FactCheckSearcher) Search(ctx context.Context, query string) ([]Result, error) {
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
	q := url.Values{}
	q.Set("query", query)
	q.Set("languageCode", s.Language)
	q.Set("pageSize", "10")
	q.Set("key", s.Key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, ErrSearchFailed
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		// Never log err: it quotes the URL, which holds the key and the query.
		slog.Warn("fact check search call failed", "reason", failureReason(ctx), "status", 0)
		return nil, ErrSearchFailed
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 403 usually means the Fact Check Tools API is not enabled for the key.
		slog.Warn("fact check search call failed", "reason", "http_status", "status", resp.StatusCode)
		return nil, ErrSearchFailed
	}

	var body struct {
		Claims []struct {
			Text        string `json:"text"`
			ClaimReview []struct {
				Publisher struct {
					Name string `json:"name"`
					Site string `json:"site"`
				} `json:"publisher"`
				URL           string `json:"url"`
				Title         string `json:"title"`
				TextualRating string `json:"textualRating"`
			} `json:"claimReview"`
		} `json:"claims"`
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
	for _, c := range body.Claims {
		for _, r := range c.ClaimReview {
			if len(out) >= limit {
				return out, nil
			}
			if seen[r.URL] || s.Grounder == nil || !s.Grounder.HostAllowed(r.URL) {
				continue
			}
			seen[r.URL] = true
			publisher := strings.TrimSpace(r.Publisher.Name)
			if publisher == "" {
				publisher = strings.TrimSpace(r.Publisher.Site)
			}
			title := strings.TrimSpace(r.Title)
			if title == "" {
				title = strings.TrimSpace(c.Text)
			}
			out = append(out, Result{
				Title:     truncate(title, 200),
				Publisher: truncate(publisher, 80),
				URL:       r.URL,
				Snippet:   truncate(strings.TrimSpace(c.Text), 300),
				Rating:    truncate(strings.TrimSpace(r.TextualRating), 60),
			})
		}
	}
	return out, nil
}

// URLSet is the tool-result set Grounder.Verified checks against.
func URLSet(results []Result) map[string]bool {
	m := make(map[string]bool, len(results))
	for _, r := range results {
		m[r.URL] = true
	}
	return m
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// failureReason distinguishes a spent time budget from a transport error.
func failureReason(ctx context.Context) string {
	if ctx.Err() != nil {
		return "timeout"
	}
	return "transport"
}

// MergeResults joins result lists in order and drops repeated URLs.
func MergeResults(lists ...[]Result) []Result {
	seen := map[string]bool{}
	var out []Result
	for _, l := range lists {
		for _, r := range l {
			if seen[r.URL] {
				continue
			}
			seen[r.URL] = true
			out = append(out, r)
		}
	}
	return out
}

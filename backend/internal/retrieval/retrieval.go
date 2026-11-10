// Package retrieval finds fact-check and news coverage for a claim through a
// fixed, non-user-controlled news endpoint, then keeps only results published
// by hosts on the reviewed allowlist.
//
// The model never supplies sources. It supplies a search query at most; every
// URL shown to the user comes from this package's own HTTP response and is
// re-checked against internal/grounding. That is the difference between a
// citation and a hallucination.
package retrieval

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/models"
)

// DefaultEndpoint is Bing News' RSS search. It is fixed, not user input, so it
// needs no SSRF protection; publisher URLs are never fetched, only displayed.
const DefaultEndpoint = "https://www.bing.com/news/search"

// ErrDisabled means no search endpoint is configured.
var ErrDisabled = errors.New("retrieval disabled")

// Item is one allowlisted result.
type Item struct {
	Title     string
	Publisher string
	URL       string
	Snippet   string
	Published time.Time
	Match     models.FactStatus
}

// Searcher queries the news endpoint and filters by allowlist.
type Searcher struct {
	Endpoint string
	Client   *http.Client
	Grounder *grounding.Grounder
	MaxItems int
}

// New builds a searcher. A nil grounder uses the default allowlist.
func New(g *grounding.Grounder) *Searcher {
	if g == nil {
		g = grounding.New(grounding.DefaultAllowlist)
	}
	return &Searcher{
		Endpoint: DefaultEndpoint,
		Client: &http.Client{
			Timeout: 6 * time.Second,
			Transport: &http.Transport{
				Proxy:                  nil,
				MaxResponseHeaderBytes: 64 << 10,
				DisableKeepAlives:      true,
			},
		},
		Grounder: g,
		MaxItems: 8,
	}
}

type rss struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
			Source      string `xml:"Source"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Search runs one query. An empty or unsearchable query returns no items and
// no error: "nothing found" is not a failure.
func (s *Searcher) Search(ctx context.Context, query string) ([]Item, error) {
	if s == nil || s.Client == nil {
		return nil, ErrDisabled
	}
	q := SanitizeQuery(query)
	if q == "" {
		return []Item{}, nil
	}

	endpoint, err := url.Parse(s.Endpoint)
	if err != nil || endpoint.Scheme != "https" {
		return nil, ErrDisabled
	}
	values := endpoint.Query()
	values.Set("q", q)
	values.Set("format", "RSS")
	values.Set("setlang", "id")
	values.Set("cc", "ID")
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("retrieval request: %w", err)
	}
	req.Header.Set("User-Agent", "AMBA/1.0 (+literacy check)")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("retrieval fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("retrieval status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("retrieval read: %w", err)
	}
	var feed rss
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("retrieval parse: %w", err)
	}

	items := make([]Item, 0, len(feed.Channel.Items))
	seen := make(map[string]bool, len(feed.Channel.Items))
	for _, raw := range feed.Channel.Items {
		if len(items) >= s.MaxItems {
			break
		}
		link := publisherURL(raw.Link)
		if link == "" || seen[link] {
			continue
		}
		if !s.Grounder.HostAllowed(link) {
			continue
		}
		title := strings.TrimSpace(htmlText(raw.Title))
		if title == "" {
			continue
		}
		seen[link] = true
		items = append(items, Item{
			Title:     title,
			Publisher: publisherName(raw.Source, link),
			URL:       link,
			Snippet:   truncate(htmlText(raw.Description), 300),
			Published: parseDate(raw.PubDate),
			Match:     models.FactUnverified,
		})
	}
	return items, nil
}

// publisherURL unwraps Bing's apiclick redirect into the real publisher URL.
// Any other redirector is rejected rather than followed.
func publisherURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, "bing.com") && strings.Contains(u.Path, "apiclick") {
		target := u.Query().Get("url")
		if target == "" {
			return ""
		}
		decoded, err := url.Parse(target)
		if err != nil || decoded.Host == "" {
			return ""
		}
		return decoded.String()
	}
	return u.String()
}

func publisherName(source, link string) string {
	source = strings.TrimSpace(source)
	if source != "" {
		return source
	}
	if u, err := url.Parse(link); err == nil {
		return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	}
	return "Sumber"
}

func parseDate(raw string) time.Time {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// SanitizeQuery keeps a query short and free of search operators that the
// gateway would treat as syntax. It returns "" when nothing usable remains.
func SanitizeQuery(query string) string {
	query = htmlText(query)
	fields := strings.Fields(query)
	out := make([]string, 0, 12)
	for _, f := range fields {
		f = strings.Trim(f, `"'()[]{}<>,.;:!?/\\|`)
		lower := strings.ToLower(f)
		if lower == "" || strings.Contains(lower, ":") || strings.HasPrefix(lower, "-") || strings.HasPrefix(lower, "+") {
			continue
		}
		out = append(out, f)
		if len(out) == 12 {
			break
		}
	}
	return truncate(strings.Join(out, " "), 120)
}

// HeuristicQuery derives a search query from the user's own text when the
// model has not answered yet (or failed). URLs are removed so a link audit
// does not search for its own address; when the text is only a URL, the
// address's own words are the only honest query left.
func HeuristicQuery(text string) string {
	var b strings.Builder
	var firstLink string
	for _, field := range strings.Fields(text) {
		lower := strings.ToLower(field)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
			strings.HasPrefix(lower, "www.") {
			if firstLink == "" {
				firstLink = field
			}
			continue
		}
		b.WriteString(field)
		b.WriteByte(' ')
		if len([]rune(b.String())) > 120 {
			break
		}
	}
	if query := SanitizeQuery(b.String()); query != "" {
		return query
	}
	if firstLink == "" {
		return ""
	}
	return SanitizeQuery(slugWords(firstLink))
}

// slugWords turns a URL into its readable words: host labels, then path
// segments, with hyphens and percent-encoding removed.
func slugWords(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(u.Hostname(), ".")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	parts = append(parts, strings.Split(strings.Trim(u.Path, "/"), "/")...)
	var words []string
	for _, part := range parts {
		if decoded, err := url.PathUnescape(part); err == nil {
			part = decoded
		}
		for _, w := range strings.Split(part, "-") {
			if len([]rune(w)) >= 3 && !isDigits(w) {
				words = append(words, w)
			}
		}
	}
	return strings.Join(words, " ")
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// Annotate marks each item against the claim. Only items that share enough
// distinctive vocabulary with the claim are kept; the rest are noise.
//
// Numbers are a hard gate. A fact-check about "bantuan Rp 900.000" must never
// be cited as contradicting a claim about "bantuan Rp 500.000": the headline
// shares plenty of words, and that is exactly how a false citation happens.
func Annotate(claim string, items []Item) []Item {
	claimTokens := tokens(claim)
	claimNumbers := numbers(claim)
	if len(claimTokens) == 0 {
		return []Item{}
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		text := item.Title + " " + item.Snippet
		if !numbersCompatible(claimNumbers, numbers(text)) {
			continue
		}
		overlap := overlapCount(claimTokens, tokens(text))
		if !related(overlap, claimTokens, tokens(text)) {
			continue
		}
		item.Match = classify(text)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		rank := func(s models.FactStatus) int {
			switch s {
			case models.FactContradicts:
				return 0
			case models.FactSupports:
				return 1
			default:
				return 2
			}
		}
		return rank(out[i].Match) < rank(out[j].Match)
	})
	return out
}

// Aggregate collapses per-source matches into the pillar status. A single
// verified contradiction wins; otherwise a verified support wins.
func Aggregate(items []Item) models.FactStatus {
	status := models.FactUnverified
	for _, item := range items {
		if item.Match == models.FactContradicts {
			return models.FactContradicts
		}
		if item.Match == models.FactSupports {
			status = models.FactSupports
		}
	}
	return status
}

// classify reads explicit debunk/support markers. It is deliberately
// conservative: a title that merely mentions the topic stays UNVERIFIED.
func classify(text string) models.FactStatus {
	t := " " + strings.ToLower(htmlText(text)) + " "
	debunk := []string{
		"[hoaks]", "[salah]", " hoaks", "tidak benar", "tidak tepat", "bukan ",
		"keliru", "menyesatkan", "klarifikasi", "cek fakta", "fakta atau hoaks",
		"disinformasi", "misinformasi", "penipuan", "modus", "tipu",
	}
	support := []string{
		"[benar]", "benar bahwa", "terkonfirmasi", "dikonfirmasi",
		"resmi bahwa", "resmi mengumumkan", "sah menurut",
	}
	for _, m := range debunk {
		if strings.Contains(t, m) {
			return models.FactContradicts
		}
	}
	for _, m := range support {
		if strings.Contains(t, m) {
			return models.FactSupports
		}
	}
	return models.FactUnverified
}

var stopwords = map[string]bool{
	"yang": true, "dan": true, "di": true, "ke": true, "dari": true, "untuk": true,
	"dengan": true, "ini": true, "itu": true, "ada": true, "akan": true, "telah": true,
	"pada": true, "dalam": true, "adalah": true, "sebagai": true, "atau": true,
	"juga": true, "bisa": true, "dapat": true, "tidak": true, "bukan": true,
	"para": true, "bagi": true, "oleh": true, "saat": true, "hari": true,
	"kepada": true, "tentang": true, "agar": true, "supaya": true, "jika": true,
	"the": true, "and": true, "for": true, "with": true, "from": true,
}

func tokens(s string) map[string]bool {
	out := make(map[string]bool, 16)
	for _, field := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(field)) < 3 || stopwords[field] {
			continue
		}
		out[field] = true
	}
	return out
}

func overlapCount(a, b map[string]bool) int {
	n := 0
	for token := range a {
		if b[token] {
			n++
		}
	}
	return n
}

// related accepts an item when it shares at least two content words with the
// claim, or one long, non-generic word. Two short generic words ("bantuan",
// "gratis") are not enough to call an article coverage of the claim.
// numbers extracts digit groups with their magnitude (500.000 -> "500000").
func numbers(s string) map[string]bool {
	out := make(map[string]bool, 4)
	var b strings.Builder
	flush := func() {
		if b.Len() >= 2 {
			out[b.String()] = true
		}
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if r == '.' || r == ',' {
			continue // thousands separator inside a number
		}
		flush()
	}
	flush()
	return out
}

// numbersCompatible rejects an item when the claim names amounts and the item
// names different ones. An item with no numbers is allowed: many debunks
// describe the claim without repeating every figure.
func numbersCompatible(claim, item map[string]bool) bool {
	if len(claim) == 0 || len(item) == 0 {
		return true
	}
	for n := range claim {
		if item[n] {
			return true
		}
	}
	return false
}

func related(overlap int, claim, item map[string]bool) bool {
	if overlap >= 2 {
		return true
	}
	if overlap == 0 {
		return false
	}
	for token := range claim {
		if !item[token] || len([]rune(token)) < 8 {
			continue
		}
		switch token {
		case "bantuan", "informasi", "pemerintah", "indonesia", "sekarang", "penting", "gratis":
			continue
		}
		return true
	}
	return false
}

func htmlText(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

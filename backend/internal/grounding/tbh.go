package grounding

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// TurnBackHoax runs its own search, and it finds debunks that a general web
// search misses (10 Nov: "purbaya gubernur bank indonesia" returns the
// [SALAH] article at once, Tavily returned listing pages). This searcher
// reads that result page.

const (
	// TBHSearchURL is the site's search page.
	TBHSearchURL = "https://turnbackhoax.id/search"
	// TBHUserAgent says who is asking.
	TBHUserAgent = "AMBA/2.0 (+https://amba.madewadigital.com)"
	tbhCacheTTL  = 10 * time.Minute
	tbhCacheMax  = 200
	tbhMaxBody   = 1 << 20
)

// tbhArticle matches an article link: /articles/<id>-<slug>.
var tbhArticle = regexp.MustCompile(`^https://turnbackhoax\.id/articles/(\d+)-([a-z0-9-]+)/?$`)

// TBHSearcher searches turnbackhoax.id. It needs no key.
type TBHSearcher struct {
	Endpoint   string
	HTTP       *http.Client
	MaxResults int
	// Now is replaceable for cache tests.
	Now func() time.Time

	mu    sync.Mutex
	cache map[string]tbhEntry
}

type tbhEntry struct {
	results []Result
	expires time.Time
}

// NewTBHSearcher builds a searcher on client, which must be the SSRF-safe
// client. Each request is bounded by 12 s whatever the client allows.
func NewTBHSearcher(client *http.Client) *TBHSearcher {
	return &TBHSearcher{Endpoint: TBHSearchURL, HTTP: client, MaxResults: 5}
}

// Enabled reports whether searches will be attempted.
func (s *TBHSearcher) Enabled() bool { return s != nil && s.HTTP != nil }

func (s *TBHSearcher) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Search returns up to MaxResults fact-check articles for query.
func (s *TBHSearcher) Search(ctx context.Context, query string) ([]Result, error) {
	if !s.Enabled() {
		return nil, nil
	}
	query = strings.Join(strings.Fields(query), " ")
	if query == "" {
		return nil, nil
	}
	if r := []rune(query); len(r) > 100 {
		query = string(r[:100])
	}
	key := strings.ToLower(query)
	if res, ok := s.cached(key); ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second) // 2-5 s idle, past 8 s under load
	defer cancel()
	q := url.Values{}
	q.Set("query", query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, ErrSearchFailed
	}
	req.Header.Set("User-Agent", TBHUserAgent)
	req.Header.Set("Accept", "text/html")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		// Never log err or the query: it holds user content.
		slog.Warn("turnbackhoax search failed", "reason", failureReason(ctx), "status", 0)
		return nil, ErrSearchFailed
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("turnbackhoax search failed", "reason", "http_status", "status", resp.StatusCode)
		return nil, ErrSearchFailed
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, tbhMaxBody))
	if err != nil {
		return nil, ErrSearchFailed
	}
	limit := s.MaxResults
	if limit <= 0 {
		limit = 5
	}
	out := RelevantToQuery(query, ParseTBHSearch(string(body), 20), limit)
	s.store(key, out)
	return out, nil
}

func (s *TBHSearcher) cached(key string) ([]Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[key]
	if !ok || s.now().After(e.expires) {
		delete(s.cache, key)
		return nil, false
	}
	return append([]Result(nil), e.results...), true
}

func (s *TBHSearcher) store(key string, res []Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]tbhEntry{}
	}
	now := s.now()
	if len(s.cache) >= tbhCacheMax {
		for k, e := range s.cache {
			if now.After(e.expires) {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= tbhCacheMax {
			s.cache = map[string]tbhEntry{}
		}
	}
	s.cache[key] = tbhEntry{results: append([]Result(nil), res...), expires: now.Add(tbhCacheTTL)}
}

// tbhCard collects what one article's links show.
type tbhCard struct {
	url      string
	alt      string
	headings []string
	paras    []string
	anchor   []string
}

// ParseTBHSearch reads the result page: up to limit unique article links
// with title, rating tag and snippet. A card shows its title twice (a short
// and a long span), so the longest text wins, and the image's alt text holds
// the full title.
func ParseTBHSearch(page string, limit int) []Result {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return nil
	}
	if strings.Contains(page, "Tidak ada hasil yang ditemukan") {
		return nil
	}
	var order []string
	cards := map[string]*tbhCard{}
	var walk func(n *html.Node, inCard bool)
	walk = func(n *html.Node, inCard bool) {
		if n.Type == html.ElementNode && strings.Contains(attr(n, "class"), "news-card") {
			inCard = true
		}
		if inCard && n.Type == html.ElementNode && n.Data == "a" {
			if link := articleLink(attr(n, "href")); link != "" {
				c := cards[link]
				if c == nil {
					c = &tbhCard{url: link}
					cards[link] = c
					order = append(order, link)
				}
				collectCard(n, c)
				return
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch, inCard)
		}
	}
	walk(doc, false)

	var out []Result
	for _, link := range order {
		if len(out) >= limit {
			break
		}
		c := cards[link]
		title := c.title()
		if title == "" {
			continue
		}
		rating := ""
		if m := titleTag.FindStringSubmatch(title); m != nil {
			rating = "[" + strings.TrimSpace(m[1]) + "]"
		}
		out = append(out, Result{
			Title:     truncate(title, 200),
			Publisher: "turnbackhoax.id",
			URL:       link,
			Snippet:   truncate(c.snippet(), 300),
			Rating:    truncate(rating, 60),
		})
	}
	return out
}

// RelevantToQuery keeps the results whose title or snippet shares at least one
// keyword stem with query, at most limit. The site searches the full text of its
// articles, so a common word ("video", "instagram") returns articles that have
// nothing to do with the claim (10 Nov). A query with no keyword keeps nothing.
func RelevantToQuery(query string, res []Result, limit int) []Result {
	want := keyTerms(query)
	var out []Result
	for _, r := range res {
		if len(out) >= limit {
			break
		}
		have := keyTerms(r.Title + " " + r.Snippet)
		for w := range want {
			if have[w] {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// articleLink returns href when it is a TurnBackHoax article URL, else "".
func articleLink(href string) string {
	href = strings.TrimSpace(href)
	if !tbhArticle.MatchString(href) {
		return ""
	}
	return strings.TrimSuffix(href, "/")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func collectCard(a *html.Node, c *tbhCard) {
	if t := cleanText(nodeText(a)); t != "" {
		c.anchor = append(c.anchor, t)
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "img":
				if alt := cleanText(attr(n, "alt")); len(alt) > len(c.alt) {
					c.alt = alt
				}
			case "h1", "h2", "h3", "h4":
				c.headings = append(c.headings, spanTexts(n)...)
			case "p":
				c.paras = append(c.paras, spanTexts(n)...)
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(a)
}

// spanTexts returns the text of each span child, or the element's own text.
func spanTexts(n *html.Node) []string {
	var out []string
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && ch.Data == "span" {
			if t := cleanText(nodeText(ch)); t != "" {
				out = append(out, t)
			}
		}
	}
	if len(out) == 0 {
		if t := cleanText(nodeText(n)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return b.String()
}

func cleanText(s string) string { return strings.Join(strings.Fields(s), " ") }

func longest(list []string) string {
	best := ""
	for _, s := range list {
		if len([]rune(s)) > len([]rune(best)) {
			best = s
		}
	}
	return best
}

// title prefers the image's full title, then the longest heading, then the
// anchor text, then the slug.
func (c *tbhCard) title() string {
	for _, t := range []string{c.alt, longest(c.headings), longest(c.anchor)} {
		if t != "" {
			return t
		}
	}
	m := tbhArticle.FindStringSubmatch(c.url)
	if m == nil {
		return ""
	}
	return strings.ReplaceAll(m[2], "-", " ")
}

func (c *tbhCard) snippet() string {
	return longest(c.paras)
}

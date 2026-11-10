package pipeline

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
)

// Research (Sir, 10 Nov 2026): the conclusion about a video must rest on data
// from trusted places, shown as a trail the family can open and read. The
// trail is built after every check has run, from what the audit already
// searched: the pages the web search judged to be about this video, the claim
// check's sources, and the web search's other on-topic pages. Only when those
// leave fewer than researchMinPages does what the video shows (its fact-check
// keywords, or else its 5W1H) get a search of its own over the trusted
// newsrooms: the Tavily credits are shared with the fact check. No model call
// judges these pages, and a failed search never degrades the result.
const (
	researchBudget   = 8 * time.Second
	researchMax      = 5
	researchMinPages = 2
)

// researchTerms is the model's fact-check keywords, or with none, what the
// video shows, where and when, cut to a searchable phrase ("" when too thin).
func researchTerms(vf llm.VideoFindings) string {
	if q := strings.TrimSpace(vf.SearchQuery); q != "" {
		return q
	}
	w := vf.W5H1
	return corePhrase(strings.Join([]string{w.What, w.Where, w.When}, " "), w.Who)
}

// researchInput is what the audit already found, read once its goroutines are done.
type researchInput struct {
	terms    string
	evidence []models.WebEvidence
	facts    []models.Source
	pages    []grounding.Result
}

// buildResearch returns the trail, or nil when there is nothing to show and
// nothing to search.
func (p *Pipeline) buildResearch(ctx context.Context, in researchInput) *models.Research {
	out := models.Research{Status: models.ResearchNone, Sources: []models.ResearchSource{}}
	seen := map[string]bool{}
	add := func(title, rawURL, publisher, snippet string) {
		if len(out.Sources) >= researchMax || !p.trustedPage(rawURL) {
			return
		}
		// One page once: the same article comes back with other query strings.
		domain := domainOf(rawURL, publisher)
		key := domain + "|" + strings.ToLower(strings.TrimSpace(title))
		if seen[pageKey(rawURL)] || seen[key] {
			return
		}
		seen[pageKey(rawURL)], seen[key] = true, true
		out.Sources = append(out.Sources, models.ResearchSource{
			Title:   truncateRunes(strings.TrimSpace(title), 200),
			URL:     rawURL,
			Domain:  domain,
			Snippet: truncateRunes(strings.TrimSpace(snippet), 280),
		})
	}
	for _, e := range in.evidence {
		add(e.Title, e.URL, e.Publisher, "")
	}
	// The claim check's sources: those it matched to the claim first, the
	// others only when they share the video's subject.
	for _, s := range in.facts {
		if s.Match == models.FactContradicts || s.Match == models.FactSupports {
			add(s.Title, s.URL, s.Publisher, "")
		}
	}
	for _, s := range in.facts {
		if s.Match != models.FactContradicts && s.Match != models.FactSupports && grounding.SameTopic(in.terms, s.Title) {
			add(s.Title, s.URL, s.Publisher, "")
		}
	}
	for _, r := range in.pages {
		add(r.Title, r.URL, r.Publisher, r.Snippet)
	}
	if len(out.Sources) < researchMinPages && in.terms != "" && p.Research != nil && p.Research.Enabled() {
		out.Query = in.terms
		sctx, cancel := withBudget(ctx, researchBudget)
		results, err := p.Research.Search(sctx, in.terms)
		cancel()
		if err != nil && len(out.Sources) == 0 {
			out.Status = models.ResearchUnavailable
		}
		for _, r := range results {
			if grounding.SameTopic(in.terms, r.Title+" "+r.Snippet) {
				add(r.Title, r.URL, r.Publisher, r.Snippet)
			}
		}
	}
	if len(out.Sources) > 0 {
		out.Status = models.ResearchFound
	}
	if len(out.Sources) == 0 && out.Query == "" && in.terms == "" {
		return nil
	}
	return &out
}

// trustedPage is an https page on the fact-check or the newsroom allowlist.
func (p *Pipeline) trustedPage(rawURL string) bool {
	for _, g := range []*grounding.Grounder{p.Grounder, p.researchGrounder()} {
		if g != nil && g.HostAllowed(rawURL) {
			return true
		}
	}
	return false
}

func (p *Pipeline) researchGrounder() *grounding.Grounder {
	if p.ResearchGrounder != nil {
		return p.ResearchGrounder
	}
	return p.WebGrounder
}

// pageKey is the URL without its query string and fragment.
func pageKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return strings.ToLower(u.Host) + strings.TrimSuffix(u.Path, "/")
}

// domainOf is the page's host without "www.", or the searcher's publisher.
func domainOf(rawURL, publisher string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
		return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	}
	return publisher
}

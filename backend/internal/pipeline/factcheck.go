package pipeline

import (
	"context"
	"strings"
	"sync"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
)

// maxFactResults bounds what the classifier reads: the fact-checker's own
// search first, then the general search.
const maxFactResults = 8

// factContext is what surrounds a claim: the content's own words, and what
// kind of content it is.
type factContext struct {
	// Title is the title or caption; Summary and Transcript come from the
	// video analysis (empty when the model never ran or the input is no video).
	Title, Summary, Transcript string
	// Video is true for a downloaded or attempted social video.
	Video bool
	// Official is true when the video's uploader is the official account of
	// a newsroom (officialOutlet).
	Official bool
	// Judged is what the web provenance classification already said about a URL
	// (HOAX, REAL, ...): the same page is not sent to the model twice.
	Judged map[string]string
	// Debunk is true when the content is itself a clarification or fact-check
	// (its own title or address says so, grounding.IsFactCheck). The claim read
	// from such a page is the hoax it corrects, so a fact-checker who debunks
	// the same claim agrees with the page rather than refuting it.
	Debunk bool
}

func (c factContext) llm() llm.ClaimContext {
	return llm.ClaimContext{Title: c.Title, Summary: c.Summary, Transcript: c.Transcript}
}

// topic is the text a search result must share a subject with: the claim and
// the content around it.
func (c factContext) topic(claim string) string {
	return claim + " . " + c.Title + " . " + c.Summary
}

// officialOutlets are the exact handles (lower case, without dots, spaces,
// underscores or the @) of newsroom accounts whose own uploads may be
// confirmed by their own reporting.
var officialOutlets = map[string]bool{
	"kompascom": true, "kompastv": true, "cnnindonesia": true, "cnnindonesiaofficial": true,
	"detikcom": true, "tempodotco": true, "tempoco": true, "metrotvnews": true, "liputan6": true,
	"narasinewsroom": true, "bbcindonesia": true, "bbcnewsindonesia": true, "antaranews": true,
}

// officialOutlet reports whether the uploader name or id is exactly one of
// officialOutlets. A lookalike ("kompastv_fans") never matches.
func officialOutlet(names ...string) bool {
	for _, n := range names {
		if officialOutlets[handleKey(n)] {
			return true
		}
	}
	return false
}

func handleKey(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		case r == '.' || r == '_' || r == ' ' || r == '@' || r == '-':
			return -1
		default:
			return '?' // any other character makes the key fail every exact match
		}
	}, strings.TrimSpace(s))
}

// factQueries are the TurnBackHoax queries for a claim. The site's search
// needs every word of the query in a title, so a five-word query from a
// descriptive claim finds nothing ("purbaya didampingi sumpah gubernur bank"),
// while "purbaya gubernur" finds the debunk. The queries are therefore short:
// the claim's two strongest keywords, the title's two, then the claim's three,
// without repeats and at most three in all.
func factQueries(claim string, c factContext) []string {
	var out []string
	add := func(text string, n int) {
		q := grounding.Keywords(text, n)
		if q != "" && !containsString(out, q) && len(out) < 3 {
			out = append(out, q)
		}
	}
	add(claim, 2)
	add(c.Title, 2)
	add(claim, 3)
	add(c.Title, 3)
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// searchFacts runs the general search and the fact-checker's own search at
// once. failed is true when the general search broke; the site search is a
// bonus and never degrades a result.
func (p *Pipeline) searchFacts(ctx context.Context, claim, query string, fc factContext) (results []grounding.Result, failed bool) {
	var (
		wg        sync.WaitGroup
		general   []grounding.Result
		generalOK = true
		mu        sync.Mutex
		site      [][]grounding.Result
	)
	if p.Searcher != nil && p.Searcher.Enabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sctx, cancel := withBudget(ctx, p.SearchDeadline)
			defer cancel()
			r, err := p.Searcher.Search(sctx, query)
			mu.Lock()
			general, generalOK = r, err == nil
			mu.Unlock()
		}()
	}
	if p.SiteSearch != nil && p.SiteSearch.Enabled() {
		queries := factQueries(claim, fc)
		site = make([][]grounding.Result, len(queries))
		for i, q := range queries {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sctx, cancel := withBudget(ctx, p.SearchDeadline)
				defer cancel()
				r, err := p.SiteSearch.Search(sctx, q)
				if err != nil {
					return
				}
				site[i] = r
			}()
		}
	}
	wg.Wait()
	merged := grounding.MergeResults(append(site, general)...)
	if len(merged) > maxFactResults {
		merged = merged[:maxFactResults]
	}
	return merged, !generalOK
}

// checkClaim searches the fact-checkers (TurnBackHoax's own search and the
// general fact-check search, in parallel) and lets the model judge each result
// against the narrative of the content. Only URLs that came back from a search
// can become sources (TECH_SPEC §6 (a)), so an invented citation can never
// carry CONTRADICTS into the score. failed reports a search or classifier
// outage (the pillar then stays UNVERIFIED and the result is degraded).
func (p *Pipeline) checkClaim(ctx context.Context, claim, query string, fc factContext) (models.FactPillar, bool) {
	fact := models.FactPillar{Available: true, Claim: claim, Status: models.FactUnverified, Sources: []models.Source{}}
	general := p.Searcher != nil && p.Searcher.Enabled()
	site := p.SiteSearch != nil && p.SiteSearch.Enabled()
	if !general && !site {
		return fact, false
	}
	if query == "" {
		query = claim
	}
	results, searchFailed := p.searchFacts(ctx, claim, query, fc)

	// Only pages on the content's own subject can speak for or against it: the
	// classifier's answer was discarded for any other page anyway (SameTopic
	// below), so the others are neither judged nor shown (10 Nov: a reel was
	// listed with eight fact-checks about GMO food and diabetes).
	topic := fc.topic(claim)
	relevant := make([]grounding.Result, 0, len(results))
	for _, r := range results {
		if grounding.SameTopic(topic, r.Title+" "+r.Snippet) {
			relevant = append(relevant, r)
		}
	}
	if len(relevant) == 0 {
		return fact, searchFailed
	}

	// A page the web classification already judged keeps that judgement; only
	// the rest goes to the model, and when nothing is left there is no call.
	judged := make([]string, len(relevant))
	var evidence []llm.Evidence
	var pending []int
	for i, r := range relevant {
		if m, ok := fromWebSays(fc.Judged[r.URL]); ok {
			judged[i] = m
			continue
		}
		pending = append(pending, i)
		evidence = append(evidence, llm.Evidence{Title: r.Title, Publisher: r.Publisher, Snippet: r.Snippet, Rating: r.Rating})
	}
	var classifyErr error
	if len(pending) > 0 {
		cctx, cancel := withBudget(ctx, p.ClassifyDeadline)
		matches, err := p.LLM.Classify(cctx, claim, fc.llm(), evidence)
		cancel()
		classifyErr = err
		for j, i := range pending {
			if j < len(matches) {
				judged[i] = matches[j]
			}
		}
	}

	toolURLs := grounding.URLSet(results)
	for i, r := range relevant {
		match := claimMatch(judged[i], p.Grounder.Verified(r.URL, toolURLs), r.Title, r.URL, fc)
		fact.Sources = append(fact.Sources, models.Source{
			Title: r.Title, Publisher: r.Publisher, URL: r.URL, Match: match,
		})
	}
	fact.Status = factStatus(fact.Sources, fc)
	fact.Sources = topSources(fact.Sources, maxShownSources)
	return fact, searchFailed || classifyErr != nil
}

// isClarification reports whether a link's own title or address marks it as a
// clarification or fact-check article (for example a newsroom's
// "Klarifikasi ..." or a cekfakta page). Uploads are never one.
func isClarification(req Request, title string) bool {
	if req.InputType != models.InputURL || req.URL == "" {
		return false
	}
	return grounding.IsFactCheck(strings.TrimSpace(title), req.URL)
}

// claimMatch is one source's standing on the claim: the classifier's answer
// when the source was really returned by the search tools, UNVERIFIED
// otherwise.
//   - Only a fact-check article may contradict (9 Nov: news and unrelated
//     fact-checks "contradicted" real Kompas videos).
//   - A clarification is never contradicted by the debunk it echoes (10 Nov: a
//     Kompas clarification scored 85 "dibantah" because TurnBackHoax debunked
//     the very hoax the article corrects).
func claimMatch(judged string, verified bool, title, rawURL string, fc factContext) models.FactStatus {
	match := models.FactUnverified
	if judged != "" && verified {
		match = models.FactStatus(judged)
	}
	if match == models.FactContradicts && (fc.Debunk || !grounding.IsFactCheck(title, rawURL)) {
		match = models.FactUnverified
	}
	return match
}

// maxShownSources is how many sources the dossier lists.
const maxShownSources = 5

// topSources keeps at most n sources, those that contradict first, then those
// that support, then the rest, each group in search order.
func topSources(s []models.Source, n int) []models.Source {
	if len(s) <= n {
		return s
	}
	out := make([]models.Source, 0, n)
	for _, m := range []models.FactStatus{models.FactContradicts, models.FactSupports, models.FactUnverified} {
		for _, x := range s {
			if x.Match == m && len(out) < n {
				out = append(out, x)
			}
		}
	}
	return out
}

// fromWebSays maps a web classification onto a claim match: a page that calls
// the video a hoax contradicts it, one that reports it as real supports it.
func fromWebSays(says string) (string, bool) {
	switch says {
	case llm.WebHoax:
		return "CONTRADICTS", true
	case llm.WebReal:
		return "SUPPORTS", true
	case llm.WebAI, llm.WebUnrelated:
		return "UNVERIFIED", true
	}
	return "", false
}

// isFactCheckSource reports whether a source is a fact-check article.
func isFactCheckSource(s models.Source) bool {
	return grounding.IsFactCheck(s.Title, s.URL)
}

// statusFromSources: a verified contradiction wins over support from general
// news; only a fact-checker's own support can disagree with it. Support with
// no contradiction is SUPPORTS, anything else is UNVERIFIED.
func statusFromSources(sources []models.Source) models.FactStatus {
	contradicts, supports, checkedSupports := 0, 0, 0
	for _, s := range sources {
		switch s.Match {
		case models.FactContradicts:
			contradicts++
		case models.FactSupports:
			supports++
			if isFactCheckSource(s) {
				checkedSupports++
			}
		}
	}
	switch {
	case contradicts > 0 && checkedSupports == 0:
		return models.FactContradicts
	case supports > 0 && contradicts == 0:
		return models.FactSupports
	default:
		return models.FactUnverified
	}
}

// factStatus is statusFromSources with the rule for video: news that an event
// happened does not show that this footage belongs to it, so support that
// comes only from general news is shown but does not count, unless the
// video's uploader is the official account of a newsroom.
func factStatus(sources []models.Source, fc factContext) models.FactStatus {
	status := statusFromSources(sources)
	if status != models.FactSupports || !fc.Video || fc.Official {
		return status
	}
	for _, s := range sources {
		if s.Match == models.FactSupports && isFactCheckSource(s) {
			return status
		}
	}
	return models.FactUnverified
}

// declaredFromText is true when AI provenance rests on what the creator or the
// web says about a post, which needs no footage.
func declaredFromText(p models.Provenance) bool {
	return p.Kind == models.ProvenanceAIGenerated && (p.Source == "creator" || p.Source == "web")
}

// Package grounding decides whether a source returned by the model may be
// shown to the user. A source is only trusted when it came from the model's
// own tool results AND its host is on the reviewed allowlist.
package grounding

import (
	"net/url"
	"strings"
)

// DefaultAllowlist is the starter set from TECH_SPEC §6, pending team review.
var DefaultAllowlist = []string{
	"turnbackhoax.id",
	"cekfakta.com",
	"komdigi.go.id",
	"kominfo.go.id",
	"antaranews.com",
	"kompas.com",
	"detik.com",
	"tempo.co",
	"cnnindonesia.com",
	"liputan6.com",
	"tirto.id",
}

// / WebAllowlist is the wider set the video web-provenance search may cite:
// Indonesian and international newsrooms and fact-checkers. It is not used
// for the claim fact check, which keeps DefaultAllowlist.
var WebAllowlist = []string{
	"kompas.com", "detik.com", "cnnindonesia.com", "tempo.co", "liputan6.com", "antaranews.com",
	"tribunnews.com", "kumparan.com", "suara.com", "okezone.com", "cnbcindonesia.com",
	"turnbackhoax.id", "cekfakta.com", "komdigi.go.id", "kominfo.go.id", "tirto.id",
	"bbc.com", "bbc.co.uk", "reuters.com", "apnews.com", "factcheck.afp.com", "snopes.com",
	"politifact.com", "leadstories.com", "factcheck.org", "fullfact.org",
}

// Grounder verifies candidate sources against an allowlist.
type Grounder struct {
	allowed map[string]bool
}

// New builds a Grounder from an allowlist. A subdomain of an allowed host is
// accepted; anything else is not.
func New(allowlist []string) *Grounder {
	m := make(map[string]bool, len(allowlist))
	for _, host := range allowlist {
		m[strings.ToLower(strings.TrimSpace(host))] = true
	}
	return &Grounder{allowed: m}
}

// Verified reports whether rawURL is https, its host is allowlisted AND it
// is one of toolURLs: the URLs AMBA's own search returned (TECH_SPEC §6 (a)).
// A source the model invented cannot pass even if its host is allowlisted.
// A nil or empty tool set fails closed: no search, no verified source.
func (g *Grounder) Verified(rawURL string, toolURLs map[string]bool) bool {
	if rawURL == "" || len(toolURLs) == 0 || !toolURLs[rawURL] {
		return false
	}
	return g.HostAllowed(rawURL)
}

// HostAllowed reports whether rawURL is https on an allowlisted host. It is
// the filter applied to search results before anyone sees them.
func (g *Grounder) HostAllowed(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return false
	}
	if p := u.Port(); p != "" && p != "443" {
		return false
	}
	return g.hostAllowed(strings.ToLower(u.Hostname()))
}

func (g *Grounder) hostAllowed(host string) bool {
	if host == "" {
		return false
	}
	if g.allowed[host] {
		return true
	}
	for allowed := range g.allowed {
		if strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

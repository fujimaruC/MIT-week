// Package linkcheck scores a URL using deterministic heuristics only.
//
// Nothing here performs network I/O except Unroll and the optional Safe
// Browsing lookup. Unroll goes through the SSRF-safe client; Safe Browsing
// talks to one fixed Google host.
package linkcheck

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// Reason codes are stable identifiers; the human text is built by the caller
// from a static table so copy changes never touch this package.
const (
	ReasonLookalikeBrand  = "LOOKALIKE_BRAND"
	ReasonSuspiciousTLD   = "SUSPICIOUS_TLD"
	ReasonIPHost          = "IP_HOST"
	ReasonPunycode        = "PUNYCODE"
	ReasonManySubdomains  = "MANY_SUBDOMAINS"
	ReasonCredentials     = "CREDENTIALS_IN_URL"
	ReasonURLShortener    = "URL_SHORTENER"
	ReasonExcessiveLength = "EXCESSIVE_LENGTH"
	ReasonAtSymbol        = "AT_SYMBOL"
	ReasonNoHTTPS         = "NO_HTTPS"
	ReasonPortNonStandard = "NONSTANDARD_PORT"
	ReasonSuspiciousPath  = "SUSPICIOUS_PATH"
	ReasonScamKeywords    = "SCAM_KEYWORDS"
	ReasonShortenerHides  = "SHORTENER_HIDES"
	ReasonSafeBrowsing    = "SAFE_BROWSING"
	ReasonUnparseable     = "UNPARSEABLE"
	// The page itself (TECH_SPEC §5): a form asking for a secret on a host
	// that is not official, and the same under a brand's name.
	ReasonPageAsksSecret    = "PAGE_ASKS_SECRET"
	ReasonPageImpersonation = "PAGE_IMPERSONATION"
	// The domain's registration date from RDAP (rdap.go).
	ReasonNewDomain   = "NEW_DOMAIN"
	ReasonYoungDomain = "YOUNG_DOMAIN"
)

// HighSeverity reports codes that on their own say "this link imitates or
// hides something". They are what makes a link-only verdict defensible.
func HighSeverity(code string) bool {
	switch code {
	case ReasonLookalikeBrand, ReasonPunycode, ReasonIPHost, ReasonCredentials,
		ReasonAtSymbol, ReasonShortenerHides, ReasonSafeBrowsing, ReasonUnparseable,
		ReasonPageImpersonation:
		return true
	}
	return false
}

// brands are the names impersonated most often in Indonesian phishing.
// Ordinary words are left out on purpose ("jago", "flip", "grab", and
// "jenius", one edit away from "genius"): a brand list is only useful while
// it does not fire on everyday domains.
var brands = []string{
	"bri", "brimo", "bca", "bni", "btn", "mandiri", "bsi", "cimb", "permata",
	"danamon", "seabank", "gopay", "ovo", "dana", "shopeepay", "linkaja", "qris",
	"akulaku", "kredivo", "pegadaian",
	"tokopedia", "shopee", "lazada", "bukalapak", "blibli", "gojek",
	"whatsapp", "telegram", "instagram", "facebook", "tiktok", "youtube", "twitter",
	"kemendikbud", "kemensos", "prakerja", "kominfo", "komdigi", "pajak", "djp",
	"bpjs", "pln", "telkomsel", "indihome", "jne", "jnt",
}

// officialDomains are registrable domains that belong to a brand above or to
// a reviewed publisher. A brand token inside one of these is the real thing.
var officialDomains = map[string]bool{
	"bri.co.id": true, "bca.co.id": true, "klikbca.com": true, "bni.co.id": true,
	"bankmandiri.co.id": true, "bankbsi.co.id": true, "cimbniaga.co.id": true,
	"permatabank.com": true, "danamon.co.id": true, "gopay.co.id": true,
	"gojek.com": true, "ovo.id": true, "dana.id": true, "shopeepay.co.id": true,
	"linkaja.id": true, "qris.id": true, "tokopedia.com": true, "shopee.co.id": true,
	"shopee.com": true, "lazada.co.id": true, "bukalapak.com": true,
	"blibli.com": true, "whatsapp.com": true, "whatsapp.net": true, "wa.me": true,
	"telegram.org": true, "telegram.me": true, "t.me": true, "instagram.com": true,
	"youtube.com": true, "youtu.be": true, "twitter.com": true, "x.com": true, "fb.watch": true,
	"facebook.com": true, "fb.com": true, "tiktok.com": true, "jne.co.id": true,
	"jet.co.id": true, "btn.co.id": true, "seabank.co.id": true,
	"akulaku.com": true, "kredivo.com": true, "pegadaian.co.id": true,
	"pln.co.id": true, "telkomsel.com": true, "indihome.co.id": true,
	// The marketplaces' own sites in neighbouring countries.
	"lazada.com.my": true, "lazada.com.ph": true, "lazada.co.th": true,
	"lazada.vn": true, "lazada.sg": true, "shopee.com.my": true, "shopee.ph": true,
	"shopee.sg": true, "shopee.co.th": true, "shopee.vn": true, "shopee.tw": true,
	"shopee.com.br": true,
	// Reviewed publishers (mirrors configs/trusted_sources.yaml).
	"turnbackhoax.id": true, "cekfakta.com": true, "antaranews.com": true,
	"kompas.com": true, "detik.com": true, "tempo.co": true,
	"cnnindonesia.com": true, "liputan6.com": true, "tirto.id": true,
}

// restrictedSuffixes can only be registered by vetted institutions
// (government, universities, schools, military, villages), so a brand-like
// label under them is not a typosquat.
var restrictedSuffixes = []string{".go.id", ".ac.id", ".sch.id", ".mil.id", ".desa.id"}

// cheapSuffixes are Indonesian second-level domains anyone can register in
// minutes. In the public phishing feed AMBA was tested on (November 2026)
// they hosted a third of the Indonesian-themed phishing links and none of the
// popular legitimate sites. Reported as SUSPICIOUS_TLD.
var cheapSuffixes = []string{".my.id", ".biz.id"}

// suspiciousTLDs are cheap or frequently abused TLDs.
var suspiciousTLDs = map[string]bool{
	"zip": true, "mov": true, "top": true, "xyz": true, "click": true,
	"link": true, "shop": true, "icu": true, "buzz": true, "rest": true,
	"cam": true, "monster": true, "sbs": true, "cfd": true, "live": true,
	"cyou": true, "lol": true, "bond": true,
}

// shorteners hide the destination behind one more hop.
var shorteners = map[string]bool{
	"bit.ly": true, "tinyurl.com": true, "t.co": true, "goo.gl": true,
	"is.gd": true, "ow.ly": true, "buff.ly": true, "cutt.ly": true,
	"s.id": true, "shorturl.at": true, "rebrand.ly": true, "tiny.cc": true,
	"rb.gy": true, "v.gd": true, "lnkd.in": true,
}

// suspiciousPathWords are common in credential-harvesting pages.
var suspiciousPathWords = []string{
	"login", "verifikasi", "verify", "account", "akun", "password",
	"konfirmasi", "unlock", "secure", "update-data", "hadiah", "klaim",
}

// scamHostWords are bait words in the host name itself (TECH_SPEC §5
// SCAM_KEYWORDS). Matching is per token, so "hadiahku" does not fire on its
// own but "hadiah-bri" does.
var scamHostWords = map[string]bool{
	"bansos": true, "undian": true, "hadiah": true, "saldo": true, "klaim": true,
	"gratis": true, "verifikasi": true, "bonus": true, "giveaway": true,
	"subsidi": true, "blt": true, "cashback": true, "voucher": true,
}

// brandAffixes are the words phishers glue to a brand name without a hyphen
// ("briverifikasi", "loginbca"). A bare substring match is not enough:
// "cambridge" contains "bri" and "novotel" contains "ovo".
var brandAffixes = []string{
	"verifikasi", "verify", "login", "masuk", "promo", "hadiah", "bonus",
	"klaim", "claim", "update", "secure", "aman", "care", "help", "support",
	"official", "resmi", "online", "mobile", "center", "pusat", "undian",
	"gratis", "reward", "poin", "point", "cashback", "cs", "id", "app", "web",
}

// Finding is one heuristic hit with its contribution to the score.
type Finding struct {
	Code   string
	Weight int
	Detail string
}

// Result is the outcome of the static analysis.
type Result struct {
	Score    int
	Findings []Finding
	Host     string
	FinalURL string
}

// Analyze scores a URL from its text alone. It never performs I/O.
func Analyze(raw string) Result {
	res := Result{FinalURL: raw}

	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.Hostname() == "" {
		// A URL that cannot be parsed cannot be assessed; callers should have
		// rejected it earlier, so score it as maximally suspicious.
		res.Score = 100
		res.Findings = append(res.Findings, Finding{Code: ReasonUnparseable, Weight: 100, Detail: "unparseable"})
		return res
	}

	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	res.Host = host

	if u.User != nil {
		res.Findings = append(res.Findings, Finding{Code: ReasonCredentials, Weight: 40})
	}
	if u.User == nil && hasMisleadingAt(u) {
		res.Findings = append(res.Findings, Finding{Code: ReasonAtSymbol, Weight: 30})
	}
	if u.Scheme != "https" {
		res.Findings = append(res.Findings, Finding{Code: ReasonNoHTTPS, Weight: 15})
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		res.Findings = append(res.Findings, Finding{Code: ReasonPortNonStandard, Weight: 20})
	}
	if len(u.String()) > 200 {
		res.Findings = append(res.Findings, Finding{Code: ReasonExcessiveLength, Weight: 10})
	}
	if isIPHost(host) {
		res.Findings = append(res.Findings, Finding{Code: ReasonIPHost, Weight: 35})
		res.Score = capScore(sumWeights(res.Findings))
		return res
	}

	// A Unicode host (homoglyphs such as a Cyrillic "а") is folded to its
	// Latin look-alike so the brand rules see "bri" in "bгі" too.
	folded, unicodeHost := skeleton(host)
	if unicodeHost {
		res.Findings = append(res.Findings, Finding{Code: ReasonPunycode, Weight: 30, Detail: "unicode"})
		host = folded
	} else if strings.HasPrefix(host, "xn--") || strings.Contains(host, ".xn--") {
		res.Findings = append(res.Findings, Finding{Code: ReasonPunycode, Weight: 30})
	}
	if strings.Count(host, ".") >= 4 {
		res.Findings = append(res.Findings, Finding{Code: ReasonManySubdomains, Weight: 20})
	}

	tld := lastLabel(host)
	if suspiciousTLDs[tld] {
		res.Findings = append(res.Findings, Finding{Code: ReasonSuspiciousTLD, Weight: 25, Detail: tld})
	} else {
		for _, suffix := range cheapSuffixes {
			if strings.HasSuffix(host, suffix) {
				res.Findings = append(res.Findings, Finding{Code: ReasonSuspiciousTLD, Weight: 25, Detail: strings.TrimPrefix(suffix, ".")})
				break
			}
		}
	}
	if shorteners[registrable(host)] {
		res.Findings = append(res.Findings, Finding{Code: ReasonURLShortener, Weight: 20, Detail: registrable(host)})
	}
	if brand, ok := lookalikeBrand(host, unicodeHost); ok {
		res.Findings = append(res.Findings, Finding{Code: ReasonLookalikeBrand, Weight: 45, Detail: brand})
	}
	if word, ok := scamHostWord(host); ok {
		res.Findings = append(res.Findings, Finding{Code: ReasonScamKeywords, Weight: 20, Detail: word})
	}

	path := strings.ToLower(u.Path)
	for _, word := range suspiciousPathWords {
		if strings.Contains(path, word) {
			res.Findings = append(res.Findings, Finding{Code: ReasonSuspiciousPath, Weight: 15, Detail: word})
			break
		}
	}

	res.Score = capScore(sumWeights(res.Findings))
	return res
}

// Combine merges the analysis of a link with the analysis of where it really
// leads. A shortener that hides a risky destination is scored as that
// destination plus SHORTENER_HIDES (TECH_SPEC §5), so "bit.ly/x" pointing at
// a BRI lookalike is never rated as a harmless short link.
func Combine(first, final Result) Result {
	if final.Host == "" || registrable(final.Host) == registrable(first.Host) {
		return first
	}
	out := Result{Host: final.Host, FinalURL: final.FinalURL}
	seen := map[string]bool{}
	add := func(f Finding) {
		if seen[f.Code] {
			return
		}
		seen[f.Code] = true
		out.Findings = append(out.Findings, f)
	}
	for _, f := range final.Findings {
		add(f)
	}
	for _, f := range first.Findings {
		add(f)
	}
	score := max(first.Score, final.Score)
	if final.Score >= 30 {
		add(Finding{Code: ReasonShortenerHides, Weight: 15, Detail: final.Host})
		score += 15
	}
	out.Score = capScore(score)
	return out
}

// WithSafeBrowsingHit marks a result as a known-malicious link.
func WithSafeBrowsingHit(r Result) Result {
	for _, f := range r.Findings {
		if f.Code == ReasonSafeBrowsing {
			return r
		}
	}
	r.Findings = append([]Finding{{Code: ReasonSafeBrowsing, Weight: 100}}, r.Findings...)
	r.Score = 100
	return r
}

// WithPage adds what the page itself shows. A form that asks for a password,
// PIN, OTP or card number on a host that is not official is worth knowing
// (PAGE_ASKS_SECRET, 20); the same form on a page that names a bank, wallet
// or agency in its title is how phishing on hacked and free-hosting sites
// looks, which the address alone cannot show (PAGE_IMPERSONATION, 45, as
// strong as a lookalike domain).
func WithPage(r Result, title, siteName string, asksSecret bool) Result {
	if !asksSecret || r.Host == "" || IsOfficialHost(r.Host) {
		return r
	}
	f := Finding{Code: ReasonPageAsksSecret, Weight: 20}
	if brand, ok := titleBrand(title + " " + siteName); ok {
		f = Finding{Code: ReasonPageImpersonation, Weight: 45, Detail: brand}
	}
	for _, have := range r.Findings {
		if have.Code == f.Code {
			return r
		}
	}
	r.Findings = append(r.Findings, f)
	r.Score = capScore(r.Score + f.Weight)
	return r
}

// titleBrand finds a brand named in a page title, as a word or inside a
// product name ("KlikBCA", "myBCA", "BNIDirect"). "dana", "pajak", "qris"
// and "permata" are ordinary words in titles ("dana pensiun", "konsultan
// pajak", "toko permata"), so they do not count, and "bri" and "ovo" only
// count at the end of a word because ordinary words start with them
// ("bridge", "oval").
func titleBrand(text string) (string, bool) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		for _, b := range brands {
			switch {
			case b == "dana" || b == "pajak" || b == "qris" || b == "permata":
				continue
			case w == b:
				return b, true
			case len(w) > len(b)+6:
				continue
			case strings.HasSuffix(w, b):
				return b, true
			case strings.HasPrefix(w, b) && b != "bri" && b != "ovo":
				return b, true
			}
		}
	}
	return "", false
}

// hasMisleadingAt reports an "@" outside the authority that is not a social
// handle. "/@kompascom/video/1" is how TikTok, YouTube and Medium name an
// account and is not a trick; a pre-filled victim address
// ("?email=a@b.example") or "/secure@bank.example" in a path is.
func hasMisleadingAt(u *url.URL) bool {
	for _, segment := range strings.Split(u.Path, "/") {
		if strings.HasPrefix(segment, "@") && isHandle(segment[1:]) {
			continue
		}
		if strings.Contains(segment, "@") {
			return true
		}
	}
	return strings.Contains(u.RawQuery, "@") || strings.Contains(u.Fragment, "@")
}

func isHandle(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func sumWeights(findings []Finding) int {
	total := 0
	for _, f := range findings {
		total += f.Weight
	}
	return total
}

func capScore(n int) int {
	if n > 100 {
		return 100
	}
	if n < 0 {
		return 0
	}
	return n
}

// lookalikeBrand reports a brand that a host imitates. It works on tokens of
// the registrable name and its subdomains, never on raw substrings, so
// ordinary words that happen to contain a brand ("cambridge", "novotel",
// "danantara") are not flagged.
// A folded Unicode host is never exempt: the real "bri.co.id" is ASCII, so a
// host that only looks like it after folding is the imitation.
func lookalikeBrand(host string, folded bool) (string, bool) {
	if !folded {
		if officialDomains[registrable(host)] {
			return "", false
		}
		for _, suffix := range restrictedSuffixes {
			if strings.HasSuffix(host, suffix) {
				return "", false
			}
		}
	}
	for _, label := range nameLabels(host) {
		for _, variant := range leetVariants(label) {
			for _, token := range tokens(variant) {
				for _, brand := range brands {
					if brandMatches(token, brand) {
						return brand, true
					}
				}
			}
		}
	}
	return "", false
}

// leetVariants returns the label as written plus the readings a human sees
// when digits stand in for letters ("faceb00k"). Plain digit-free labels
// return only themselves.
func leetVariants(label string) []string {
	if !strings.ContainsAny(label, "013457") {
		return []string{label}
	}
	mk := func(one string) string {
		return strings.NewReplacer("0", "o", "1", one, "3", "e", "4", "a", "5", "s", "7", "t").Replace(label)
	}
	return []string{label, mk("i"), mk("l")}
}

func brandMatches(token, brand string) bool {
	if token == brand {
		return true
	}
	for _, affix := range brandAffixes {
		if token == brand+affix || token == affix+brand {
			return true
		}
	}
	if nearMisses[token] {
		return false
	}
	// Typosquats ("tokopedla", "shoppe", "instagrarn") only for names long
	// enough that one or two edits still leave something recognisable.
	if len(brand) >= 5 {
		limit := 1
		if len(brand) >= 9 {
			limit = 2
		}
		if abs(len(token)-len(brand)) <= limit && levenshtein(token, brand) <= limit {
			return true
		}
	}
	return false
}

// nearMisses are ordinary words one edit away from a brand ("telegraf" is
// Serbian and Ukrainian for telegraph, the name of popular news sites).
var nearMisses = map[string]bool{"telegraf": true}

func scamHostWord(host string) (string, bool) {
	for _, label := range nameLabels(host) {
		for _, token := range tokens(label) {
			if scamHostWords[token] {
				return token, true
			}
		}
	}
	return "", false
}

// nameLabels returns the labels a registrant chose: the registrable name and
// any subdomains, without the public suffix.
func nameLabels(host string) []string {
	reg := registrable(host)
	suffix := strings.TrimPrefix(reg, strings.SplitN(reg, ".", 2)[0])
	name := strings.TrimSuffix(host, suffix)
	if name == "" {
		return nil
	}
	return strings.Split(name, ".")
}

// tokens splits a label on hyphens, underscores and digits.
func tokens(label string) []string {
	return strings.FieldsFunc(label, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsDigit(r)
	})
}

// confusables maps the Cyrillic and Greek letters most used in homoglyph
// domains to the Latin letter they imitate. It is a short, reviewed table,
// not the full Unicode confusables list.
var confusables = map[rune]rune{
	'а': 'a', 'в': 'b', 'е': 'e', 'ё': 'e', 'г': 'r', 'і': 'i', 'ї': 'i', 'ј': 'j',
	'к': 'k', 'м': 'm', 'н': 'h', 'о': 'o', 'р': 'p', 'с': 'c', 'т': 't', 'у': 'y',
	'х': 'x', 'ѕ': 's', 'ԁ': 'd', 'ɡ': 'g', 'ı': 'i', 'ӏ': 'l', 'ո': 'n', 'ս': 'u',
	'α': 'a', 'β': 'b', 'ε': 'e', 'ι': 'i', 'κ': 'k', 'ν': 'v', 'ο': 'o', 'ρ': 'p',
	'τ': 't', 'υ': 'u', 'χ': 'x', 'ω': 'w',
}

// skeleton folds a Unicode host to ASCII look-alikes and reports whether the
// host contained any non-ASCII rune at all.
func skeleton(host string) (string, bool) {
	unicodeHost := false
	var b strings.Builder
	for _, r := range host {
		if r <= unicode.MaxASCII {
			b.WriteRune(r)
			continue
		}
		unicodeHost = true
		if latin, ok := confusables[unicode.ToLower(r)]; ok {
			b.WriteRune(latin)
		} else {
			b.WriteRune('x')
		}
	}
	return b.String(), unicodeHost
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func lastLabel(host string) string {
	parts := strings.Split(host, ".")
	return parts[len(parts)-1]
}

// Registrable approximates the eTLD+1 for the TLDs this project cares about.
func Registrable(host string) string { return registrable(strings.ToLower(host)) }

// IsShortener reports a known link shortener: its own age says nothing
// about where it leads.
func IsShortener(host string) bool { return shorteners[Registrable(host)] }

// IsOfficialHost reports a host on a reviewed official domain (a brand, an
// agency or a reviewed publisher) or under a suffix only vetted institutions
// can register (.go.id, .ac.id, ...).
func IsOfficialHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if officialDomains[registrable(host)] {
		return true
	}
	for _, suffix := range restrictedSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// registrable approximates the eTLD+1 for the TLDs this project cares about.
// It is deliberately simple: a full public-suffix list is out of scope, and
// every caller only needs a stable comparison key.
func registrable(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	// Handle common two-part suffixes (Indonesian SLDs and a few others).
	if len(parts) >= 3 {
		second := parts[len(parts)-2]
		if second == "co" || second == "ac" || second == "or" || second == "go" ||
			second == "net" || second == "sch" || second == "web" || second == "mil" ||
			second == "desa" || second == "my" || second == "biz" || second == "com" {
			return strings.Join(parts[len(parts)-3:], ".")
		}
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

func isIPHost(host string) bool {
	if strings.Contains(host, ":") {
		return true // bare IPv6
	}
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// Unroll follows redirects through the safe client and returns the chain.
// It stops at maxHops and never returns a body. HEAD is tried first; servers
// that refuse HEAD get a one-byte ranged GET (TECH_SPEC §5).
func Unroll(ctx context.Context, client *http.Client, raw string, maxHops int) ([]string, string, error) {
	chain := []string{raw}
	current := raw

	// Do not follow redirects inside the client: the chain is the output.
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	for i := 0; i < maxHops; i++ {
		resp, err := hop(ctx, &c, http.MethodHead, current)
		if err == nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented || resp.StatusCode == http.StatusForbidden) {
			resp, err = hop(ctx, &c, http.MethodGet, current)
		}
		if err != nil {
			return chain, current, err
		}

		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			return chain, current, nil
		}
		loc := resp.Header.Get("Location")
		if loc == "" {
			return chain, current, nil
		}
		next, err := resp.Request.URL.Parse(loc)
		if err != nil {
			return chain, current, err
		}
		if next.Scheme != "http" && next.Scheme != "https" {
			return chain, current, nil
		}
		current = next.String()
		chain = append(chain, current)
	}
	return chain, current, nil
}

func hop(ctx context.Context, c *http.Client, method, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AMBA/1.0 (+https://github.com/fujimaruC/MIT-week; literacy check)")
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-0")
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	return resp, nil
}

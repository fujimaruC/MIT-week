package grounding

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// The classifier matched loosely on 9 Nov 2026: with Tavily it let ordinary
// business news, a site's listing pages and a fact-check about an unrelated
// claim "contradict" real news, which then read "Kabar ini dibantah sumber
// terpercaya". These two checks bound what a classifier answer may mean.

var (
	factCheckTitle = regexp.MustCompile(`(?i)^\s*(\[[^\]]{2,40}\]|hoaks|hoax|cek ?fakta|disinformasi|misinformasi|klarifikasi)`)
	factCheckPath  = regexp.MustCompile(`(?i)(cekfakta|cek-fakta|hoaks|hoax|disinformasi|misinformasi|klarifikasi)`)
	listingPath    = regexp.MustCompile(`(?i)/(institusi|tag|tags|topik|category|kategori|search|cari|author|penulis)(/|$)`)
)

// IsFactCheck reports whether a search hit is a fact-check article: a
// fact-checker's own site, a verdict tag or fact-check wording at the start of
// the title, or a fact-check section in the path. Listing pages never are.
// Only a fact-check article may contradict a claim.
func IsFactCheck(title, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || listingPath.MatchString(u.Path) {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == "turnbackhoax.id" || strings.HasSuffix(host, ".turnbackhoax.id") {
		return true
	}
	return factCheckTitle.MatchString(title) || factCheckPath.MatchString(u.Path)
}

// stopwords are function words plus the vocabulary every fact-check shares,
// so that only the claim's own subject counts as overlap.
var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`yang dan atau di ke dari untuk dengan ini itu pada akan adalah sebagai juga tidak
		bukan sudah telah oleh dalam karena bahwa para saat kata hari tahun bulan baru lalu soal terkait tentang
		video foto gambar beredar viral klaim hoaks hoax cek fakta salah benar keliru klarifikasi disinformasi
		misinformasi menyesatkan narasi unggahan postingan berita resmi warganet media sosial diri jabatan
		the of and to in is are a an for on with from this that new about video photo`) {
		stopwords[w] = true
	}
}

// phraseSynonyms are multi-word forms folded to one token before splitting,
// so "wakil presiden" and "wapres" are the same word.
var phraseSynonyms = strings.NewReplacer(
	"wakil presiden", "wapres", "wakil gubernur", "wagub", "wakil wali kota", "wawali",
	"wakil walikota", "wawali", "wakil bupati", "wabup", "wakil menteri", "wamen",
	"mengundurkan diri", "undur", "pengunduran diri", "undur", "mundur dari jabatan", "undur",
	"meninggal dunia", "tewas", "kecerdasan buatan", "artificialintelligence",
)

// wordSynonyms fold whole words (before stemming) to a shared stem.
var wordSynonyms = map[string]string{
	"mundur": "undur", "undur": "undur", "pengunduran": "undur", "mengundurkan": "undur", "diundurkan": "undur",
	"meninggal": "tewas", "wafat": "tewas", "gugur": "tewas", "tewas": "tewas", "meregang": "tewas",
	"hoax": "hoaks", "hoaks": "hoaks", "wapres": "wapres", "ai": "artificialintelligence",
	"polri": "polisi", "kepolisian": "polisi", "polisi": "polisi", "pemilu": "pemilu", "pilpres": "pilpres",
	"bbm": "bbm", "unras": "demo", "demonstrasi": "demo", "unjuk": "demo",
}

var (
	prefixes = []string{"meny", "peny", "meng", "peng", "mem", "pem", "men", "pen", "ber", "ter", "me", "pe", "di", "ke", "se"}
	suffixes = []string{"nya", "kan", "lah", "kah", "an", "i"}
)

// stem is a light Indonesian stemmer: it strips one suffix and one prefix
// when at least four letters remain. It does not need to be linguistically
// right, only to give "mengundurkan", "pengunduran" and "diundurkan" one
// stem on both sides of a comparison.
func stem(w string) string {
	if s, ok := wordSynonyms[w]; ok {
		return s
	}
	for _, suf := range suffixes {
		keep := 4
		if suf == "an" || suf == "i" {
			keep = 5 // short words end in "an" or "i" naturally ("gibran", "kasi")
		}
		if strings.HasSuffix(w, suf) && len([]rune(w))-len(suf) >= keep {
			w = strings.TrimSuffix(w, suf)
			break
		}
	}
	for _, pre := range prefixes {
		if strings.HasPrefix(w, pre) && len([]rune(w))-len(pre) >= 4 {
			rest := strings.TrimPrefix(w, pre)
			if pre == "meny" || pre == "peny" {
				rest = "s" + rest
			}
			w = rest
			break
		}
	}
	if s, ok := wordSynonyms[w]; ok {
		return s
	}
	return w
}

// keyTerms are the claim's content words: lower-case, at least 4 letters
// (3 when written as an abbreviation), not stopwords, stemmed.
func keyTerms(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range words(phraseSynonyms.Replace(strings.ToLower(s))) {
		if stopwords[w] {
			continue
		}
		if _, ok := wordSynonyms[w]; !ok && len([]rune(w)) < 4 {
			continue
		}
		out[stem(w)] = true
	}
	return out
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// entities are the proper nouns in a claim: capitalised words that do not
// open a sentence, and all-caps words of three or more letters.
func entities(claim string) []string {
	var out []string
	start := true
	for _, tok := range strings.Fields(claim) {
		w := strings.ToLower(strings.TrimFunc(tok, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
		first := []rune(strings.TrimLeftFunc(tok, func(r rune) bool { return !unicode.IsLetter(r) }))
		if len(first) > 0 && len([]rune(w)) >= 3 && !stopwords[w] {
			caps := unicode.IsUpper(first[0])
			if (caps && !start) || isAllCaps(tok) {
				out = append(out, w)
			}
		}
		start = strings.HasSuffix(tok, ".") || strings.HasSuffix(tok, "?") || strings.HasSuffix(tok, "!")
	}
	return out
}

// SharesEntity reports whether text names one of subject's proper nouns, as a
// whole word. Everyday words ("berdasarkan", "kebenaran") match any debunk, so
// a page that calls a video a hoax must at least name who or what the video is
// about (10 Nov: a real Dedi Mulyadi video was "contradicted" by debunks about
// Anies and the Pilkada). False when subject names nobody.
func SharesEntity(subject, text string) bool {
	ents := namedEntities(subject)
	if len(ents) == 0 {
		return false
	}
	have := map[string]bool{}
	for _, tok := range strings.Fields(strings.ToLower(text)) {
		have[strings.TrimFunc(tok, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })] = true
	}
	for _, e := range ents {
		if have[e] {
			return true
		}
	}
	return false
}

// namedEntities are subject's proper nouns without search filler such as
// honorifics and clickbait openers ("Kang", "Viral", "Breaking").
func namedEntities(subject string) []string {
	var out []string
	for _, e := range entities(subject) {
		if !queryStop[e] {
			out = append(out, e)
		}
	}
	return out
}

func isAllCaps(tok string) bool {
	letters := 0
	for _, r := range tok {
		if unicode.IsLetter(r) {
			if !unicode.IsUpper(r) {
				return false
			}
			letters++
		}
	}
	return letters >= 3
}

// SameTopic reports whether text is about the claim's subject: at least two
// shared stems (all of them when the claim has fewer than two), or one
// shared proper noun plus one more shared stem. A source about another
// subject may neither support nor contradict the claim.
func SameTopic(claim, text string) bool {
	want := keyTerms(claim)
	if len(want) == 0 {
		return false
	}
	have := keyTerms(text)
	shared := 0
	for w := range want {
		if have[w] {
			shared++
		}
	}
	if shared >= min(2, len(want)) {
		return true
	}
	// One proper noun found in the text (even inside another word, like
	// "Prabowo's") plus one other shared stem.
	lower := strings.ToLower(text)
	for _, e := range entities(claim) {
		if !strings.Contains(lower, e) {
			continue
		}
		other := 0
		for w := range want {
			if have[w] && w != stem(e) {
				other++
			}
		}
		if other >= 1 {
			return true
		}
	}
	return false
}

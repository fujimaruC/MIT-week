package video

import (
	"regexp"
	"strings"
	"unicode"
)

// Disclosure levels. The creator said it, not the platform: a hashtag or a
// phrase in the title, description, tags or account name.
const (
	DisclosureNone   = "none"
	DisclosureWeak   = "weak"
	DisclosureStrong = "strong"
)

// Disclosure is what a creator says about AI in a post.
type Disclosure struct {
	Level  string // none, weak, strong
	Phrase string // the text that matched, "" for none
}

// generatorWords are names that identify an AI video generator on their own.
const generatorWords = `sora\s?2?|veo\s?[23]|kling(?:\s?ai)?|hailuo(?:\s?ai)?|luma\s?(?:ai|labs)|dream\s?machine|haiper(?:\s?ai)?|vidu(?:\s?ai)?|seedance|hunyuan(?:\s?video)?|pixverse|heygen|synthesia|d-id|invideo(?:\s?ai)?|runway\s?(?:ml|gen[-\s]?[34])|gen[-\s]?[34]|pika\s?labs`

// ambiguousWords are generator names that are also ordinary words or names;
// they count only with a making word or an "AI" or a version next to them.
const ambiguousWords = `sora|veo|kling|hailuo|haiper`

// lookalikeNames are generator names that double as ordinary words or common
// first names ("wan"): they count only with "AI" or a version like 2.1.
const lookalikeNames = `luma|pika|wan|vidu|runway|minimax`

const makingVerbs = `(?:made|generated|created|produced|animated|rendered|crafted|dibuat|dibikin|buatan|hasil|karya)(?:\s+(?:with|by|using|via|from|dengan|pakai|memakai|menggunakan|oleh))?`

// makingWords also admit a bare "using", "via" or "powered by" for the
// unambiguous generator names.
const makingWords = makingVerbs + `|using|via|powered by`

var (
	hashtagRe = regexp.MustCompile(`#([\p{L}\p{N}_]+)`)

	// strongTags are hashtags that declare AI generation by themselves.
	strongTags = map[string]bool{
		"aigenerated": true, "aivideo": true, "aivideos": true, "aiart": true, "aiartwork": true,
		"aigc": true, "madewithai": true, "aicreated": true, "aimade": true, "createdwithai": true,
		"generatedbyai": true, "videoai": true, "aigeneratedvideo": true, "aigeneratedcontent": true,
		"aishorts": true, "aifilm": true, "aianimation": true, "syntheticmedia": true,
		"dibuatdenganai": true, "buatanai": true, "hasilai": true,
		"sora": true, "sora2": true, "soraai": true, "openaisora": true, "veo3": true, "veo2": true,
		"googleveo": true, "klingai": true, "kling": true, "hailuoai": true, "lumaai": true,
		"dreammachine": true, "haiperai": true, "haiper": true, "seedance": true, "hunyuanvideo": true,
		"pixverse": true, "heygen": true, "synthesia": true, "runwayml": true, "runwaygen3": true,
		"runwaygen4": true, "gen3": true, "gen4": true, "pikalabs": true, "viduai": true, "wanai": true,
		"minimaxai": true, "invideoai": true,
	}
	// weakTags only say AI is somewhere in the post.
	weakTags = map[string]bool{
		"ai": true, "artificialintelligence": true, "genai": true, "generativeai": true,
		"aitools": true, "aicontent": true, "kecerdasanbuatan": true,
	}

	strongPhraseRes = []*regexp.Regexp{
		regexp.MustCompile(`\bai[-\s]?generated\b`),
		regexp.MustCompile(`\bgenerated\s+(?:by|with|using|from|via)\s+(?:an?\s+)?(?:ai|artificial intelligence)\b`),
		regexp.MustCompile(`\b(?:created|made|produced|animated|rendered|crafted)\s+(?:by|with|using|via)\s+(?:an?\s+)?(?:ai|artificial intelligence)\b`),
		regexp.MustCompile(`\b(?:dibuat|dibikin|dihasilkan|dirender)\s+(?:dengan|pakai|memakai|menggunakan|oleh|via|lewat)\s+(?:bantuan\s+)?(?:ai|kecerdasan buatan)\b`),
		regexp.MustCompile(`\bbuatan\s+(?:ai|kecerdasan buatan)\b`),
		regexp.MustCompile(`\bhasil\s+(?:ai|kecerdasan buatan)\b`),
		regexp.MustCompile(`\bsynthetic\s+media\b`),
		regexp.MustCompile(`\bai\s+(?:fan[-\s]?made|creation)\b`),
		// Loose phrases (index 8 and up) are also common in news about AI.
		regexp.MustCompile(`\b(?:ai|kecerdasan buatan)\s+(?:video|art|clip|film|animation|animasi)\b`),
		regexp.MustCompile(`\bvideo\s+(?:ai|buatan ai)\b`),
	}
	loosePhraseFrom = 8

	generatorCtxRe = regexp.MustCompile(`\b(?:` + makingWords + `)\s+(?:an?\s+|the\s+)?(?:(?:ai|openai|google)\s+)?(?:` + generatorWords + `)\b`)
	ambiguousCtxRe = regexp.MustCompile(`\b(?:` + makingVerbs + `)\s+(?:an?\s+|the\s+)?(?:(?:ai|openai|google|kuaishou)\s+)?(?:` + ambiguousWords + `)\b`)
	ambiguousAIRe  = regexp.MustCompile(`\b(?:` + ambiguousWords + `)\s*(?:ai|[0-9](?:\.[0-9])?)\b|\b(?:openai|google)\s+(?:` + ambiguousWords + `)\b|\b(?:` + lookalikeNames + `)\s*(?:ai|[0-9]\.[0-9])\b`)

	// uploaderBrands in an account name mean the account belongs to a generator
	// or posts AI clips for a living.
	uploaderBrands = []string{
		"haiper", "klingai", "kling ai", "hailuo", "lumalabs", "luma ai", "lumaai", "runwayml", "runway ml",
		"pikalabs", "pika labs", "pixverse", "heygen", "synthesia", "seedance", "hunyuan", "dream machine",
		"soraai", "sora ai", "openai sora", "veo3", "veo 3", "invideo", "viduai", "vidu ai",
		"ai video", "aivideo", "ai art", "aiart", "aigc", "ai generated", "ai film", "ai shorts", "ai clips",
	}

	// uploaderExact are generator brands that are also common words, so only
	// an account named exactly that counts (the official "Runway" channel).
	uploaderExact = map[string]bool{
		"runway": true, "pika": true, "luma": true, "kling": true, "vidu": true, "haiper": true,
		"pixverse": true, "genmo": true, "moonvalley": true, "lumaai": true, "hailuoai": true,
	}

	// newsOutlets in an account name mean reporting, not creating.
	newsOutlets = []string{
		"kompas", "detik", "cnn", "bbc", "tempo", "liputan6", "metro tv", "metrotv", "tvone", "tv one",
		"antara", "tribun", "kumparan", "cnbc", "reuters", "associated press", "al jazeera", "sindonews",
		"okezone", "suara.com", "merdeka", "republika", "jawapos", "narasi", "turnbackhoax", "cekfakta",
		"berita", "news", "fact check", "factcheck", "afp", "bloomberg", "the guardian", "nytimes",
	}

	// aboutAIRe finds headlines that talk about AI instead of using it.
	aboutAIRe = regexp.MustCompile(`\b(?:waspada|hati-hati|awas|cara\s+(?:membedakan|mengenali|mendeteksi)|deteksi|mendeteksi|kebijakan|regulasi|aturan|undang-undang|hoaks|hoax|cek\s?fakta|fact[-\s]?check|bahaya|ancaman|etika|dampak|maraknya|marak|how\s+to\s+(?:spot|detect|tell)|detect(?:ing|ion)?|policy|regulation|bans?|banned|law|warns?|warning|risks?|menkominfo|komdigi|pemerintah|dpr|ojk)\b`)
)

// DetectDisclosure reads the creator's own words. It is deterministic and
// never calls the network. News about AI is not a disclosure.
func DetectDisclosure(title, description string, tags []string, uploader string) Disclosure {
	upl := strings.ToLower(uploader)
	if containsAny(upl, newsOutlets) {
		return Disclosure{Level: DisclosureNone}
	}
	if uploaderExact[normTag(uploader)] {
		return Disclosure{Level: DisclosureStrong, Phrase: clipPhrase(uploader)}
	}
	for _, b := range uploaderBrands {
		if strings.Contains(upl, b) {
			return Disclosure{Level: DisclosureStrong, Phrase: clipPhrase(uploader)}
		}
	}

	text := strings.ToLower(title + "\n" + description)
	aboutAI := aboutAIRe.MatchString(strings.ToLower(title))

	var tagSet []string
	for _, t := range tags {
		tagSet = append(tagSet, normTag(t))
	}
	for _, m := range hashtagRe.FindAllStringSubmatch(text, -1) {
		tagSet = append(tagSet, normTag(m[1]))
	}
	for _, t := range tagSet {
		if strongTags[t] {
			return Disclosure{Level: DisclosureStrong, Phrase: "#" + t}
		}
	}
	for i, re := range strongPhraseRes {
		if i >= loosePhraseFrom && aboutAI {
			continue
		}
		if loc := re.FindStringIndex(text); loc != nil {
			return Disclosure{Level: DisclosureStrong, Phrase: clipPhrase(text[loc[0]:loc[1]])}
		}
	}
	if !aboutAI {
		for _, re := range []*regexp.Regexp{generatorCtxRe, ambiguousCtxRe, ambiguousAIRe} {
			if loc := re.FindStringIndex(text); loc != nil {
				return Disclosure{Level: DisclosureStrong, Phrase: clipPhrase(text[loc[0]:loc[1]])}
			}
		}
		for _, t := range tagSet {
			if weakTags[t] {
				return Disclosure{Level: DisclosureWeak, Phrase: "#" + t}
			}
		}
	}
	return Disclosure{Level: DisclosureNone}
}

// normTag lower-cases a tag and drops everything but letters and digits, so
// "#AI_Generated" and "ai generated" compare equal.
func normTag(t string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(t) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsAny(s string, list []string) bool {
	for _, x := range list {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func clipPhrase(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 60 {
		return string(r[:60])
	}
	return s
}

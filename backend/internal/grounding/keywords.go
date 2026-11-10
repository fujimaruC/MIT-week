package grounding

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	kwURL  = regexp.MustCompile(`https?://\S+|www\.\S+`)
	kwTags = regexp.MustCompile(`[#@][\p{L}\p{N}_]+`)
)

// queryStop are words a search query is better without, on top of the match
// stopwords: sentence openers and verbs that describe footage.
var queryStop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`menurut sebuah seorang beberapa terlihat tampak menampilkan memperlihatkan menghadiri
		mengenakan memakai menggunakan sedang sambil ketika setelah sebelum kemudian namun tetapi serta
		peringatan acara kegiatan rekaman seragam diduga disebut disebutkan mengklaim dikatakan
		saat lagi masih sudah akan bisa harus bahkan setiap semua
		pak bu bapak ibu mas mbak kak bang om tante
		detik-detik momen begini inilah heboh viral beredar terungkap fakta kabar info update
		akhirnya breaking terbaru resmi mendampingi didampingi`) {
		queryStop[w] = true
	}
}

// Keywords turns text into a short search query: at most max words (5 when
// max <= 0), proper nouns first, stopwords dropped. A proper noun is a
// capitalised word or an acronym of two or more capitals.
func Keywords(text string, max int) string {
	if max <= 0 {
		max = 5
	}
	text = kwURL.ReplaceAllString(text, " ")
	text = kwTags.ReplaceAllString(text, " ")
	var ents, rest []string
	seen := map[string]bool{}
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	}) {
		tok = strings.Trim(tok, "-")
		w := strings.ToLower(tok)
		if w == "" || seen[w] || stopwords[w] || queryStop[w] {
			continue
		}
		letters, upper := 0, 0
		for _, r := range tok {
			if unicode.IsLetter(r) {
				letters++
				if unicode.IsUpper(r) {
					upper++
				}
			}
		}
		if letters == 0 {
			continue
		}
		first := []rune(tok)[0]
		isEnt := (unicode.IsUpper(first) && letters >= 3) || (upper == letters && letters >= 2)
		if !isEnt && letters < 4 {
			continue
		}
		seen[w] = true
		if isEnt {
			ents = append(ents, w)
		} else {
			rest = append(rest, w)
		}
	}
	all := append(ents, rest...)
	if len(all) > max {
		all = all[:max]
	}
	return strings.Join(all, " ")
}

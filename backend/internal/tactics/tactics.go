// Package tactics verifies persuasion-pattern phrases against the input text.
//
// A tactic is informational only: it never enters scoring. A tactic is shown
// only when its phrase is verifiably present in the text the user submitted
// (or in OCR text derived from their image), so the UI can highlight it.
package tactics

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/amba/amba/backend/internal/models"
)

// Codes is the fixed enum from TECH_SPEC §6.1. Anything else is dropped.
var Codes = map[string]bool{
	"URGENCY":           true,
	"REWARD_BAIT":       true,
	"FALSE_AUTHORITY":   true,
	"FEAR":              true,
	"SECRECY":           true,
	"OUTRAGE_BAIT":      true,
	"IMPERSONATION":     true,
	"FAKE_SOCIAL_PROOF": true,
}

// MaxPhraseLength drops absurd phrases that would highlight a whole message.
const MaxPhraseLength = 80

// MaxTactics caps what the UI has to render.
const MaxTactics = 5

// Verify keeps only tactics whose phrase really occurs in source.
//
// Normalisation is NFKC + lower-case + collapsed whitespace, applied to both
// sides, so "Hari  Ini" matches "hari ini" without letting a near-miss through.
func Verify(candidates []models.Tactic, source string) []models.Tactic {
	normalizedSource := Normalize(source)
	if normalizedSource == "" {
		return []models.Tactic{}
	}

	out := make([]models.Tactic, 0, MaxTactics)
	seen := make(map[string]bool)

	for _, c := range candidates {
		if len(out) >= MaxTactics {
			break
		}
		if !Codes[c.Code] {
			continue
		}
		phrase := strings.TrimSpace(c.Phrase)
		if phrase == "" || len([]rune(phrase)) > MaxPhraseLength {
			continue
		}
		if c.Source != "text" && c.Source != "ocr" {
			continue
		}

		normalizedPhrase := Normalize(phrase)
		if normalizedPhrase == "" || !strings.Contains(normalizedSource, normalizedPhrase) {
			continue
		}

		key := c.Code + "\x00" + normalizedPhrase
		if seen[key] {
			continue
		}
		seen[key] = true

		// Emit the phrase as it appears in the source, so the highlighter can
		// match the visible text exactly.
		out = append(out, models.Tactic{
			Code:   c.Code,
			Phrase: originalPhrase(source, normalizedPhrase),
			Source: c.Source,
		})
	}
	return out
}

// Normalize applies NFKC, lower-casing and whitespace collapsing.
func Normalize(s string) string {
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// originalPhrase finds the source substring whose normalised form equals the
// normalised phrase, so highlights land on the user's own characters.
func originalPhrase(source, normalizedPhrase string) string {
	runes := []rune(source)
	phraseRunes := len([]rune(normalizedPhrase))

	for start := 0; start < len(runes); start++ {
		// A leading space normalises away, so starting on one would return
		// the phrase with a stray space in front of it.
		if unicode.IsSpace(runes[start]) {
			continue
		}
		for end := start + 1; end <= len(runes); end++ {
			candidate := Normalize(string(runes[start:end]))
			if candidate == normalizedPhrase {
				return string(runes[start:end])
			}
			if len([]rune(candidate)) > phraseRunes+8 {
				break
			}
		}
	}
	return normalizedPhrase
}

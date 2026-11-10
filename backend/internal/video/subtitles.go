package video

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	vttTagRe       = regexp.MustCompile(`<[^>]*>`)
	vttTimestampRe = regexp.MustCompile(`^\d{2}:\d{2}`)
)

// cleanVTT parses raw VTT bytes, removes headers, timestamps, formatting tags,
// deduplicates consecutive identical lines, and caps the result to maxRunes (4000).
func cleanVTT(raw []byte, maxRunes int) string {
	if len(raw) == 0 {
		return ""
	}

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	var lines []string
	var prevLine string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Skip WEBVTT header and metadata comments
		if strings.HasPrefix(line, "WEBVTT") || strings.HasPrefix(line, "NOTE") || strings.HasPrefix(line, "Kind:") || strings.HasPrefix(line, "Language:") {
			continue
		}
		// Skip cue identifiers (digits only)
		if isNumeric(line) {
			continue
		}
		// Skip timestamp lines: e.g. 00:00:01.000 --> 00:00:04.000
		if strings.Contains(line, "-->") || vttTimestampRe.MatchString(line) {
			continue
		}

		// Strip tags like <c> </c> <b> </b> <00:00:01.200>
		cleaned := vttTagRe.ReplaceAllString(line, "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}

		// Deduplicate repeated identical lines (frequent in scrolling subtitles)
		if cleaned == prevLine {
			continue
		}
		prevLine = cleaned
		lines = append(lines, cleaned)
	}

	result := strings.Join(lines, " ")
	return truncateRunes(result, maxRunes)
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	var count int
	for idx := range s {
		if count == maxRunes {
			return s[:idx]
		}
		count++
	}
	return s
}

package video

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	tiktokHostRe    = regexp.MustCompile(`^(www\.|m\.|vm\.|vt\.)?tiktok\.com$`)
	youtubeHostRe   = regexp.MustCompile(`^(www\.|m\.)?youtube\.com$`)
	instagramHostRe = regexp.MustCompile(`^(www\.)?instagram\.com$`)
	xHostRe         = regexp.MustCompile(`^(www\.)?(x\.com|twitter\.com)$`)
	facebookHostRe  = regexp.MustCompile(`^(www\.|m\.)?facebook\.com$`)

	xStatusRe       = regexp.MustCompile(`^/[A-Za-z0-9_]{1,50}/status/\d+/?$`)
	instagramPathRe = regexp.MustCompile(`^/(reel|reels|p|tv)/[A-Za-z0-9_-]+/?$`)
	facebookPathRe  = regexp.MustCompile(`^/(watch|reel|videos|share/v|share/r)(/.*)?$`)
)

// Platform inspects rawURL and returns the identified social platform (tiktok,
// youtube, instagram, x, facebook) if it matches the strict allowlist.
// Returns ok=false for non-https URLs, URLs with userinfo or port, IP literals,
// and unsupported paths or hosts.
func Platform(rawURL string) (platform string, ok bool) {
	if rawURL == "" {
		return "", false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}

	// Must be https only
	if u.Scheme != "https" {
		return "", false
	}

	// No userinfo (e.g. user:pass@host)
	if u.User != nil {
		return "", false
	}

	// Host check: no explicit port, no IP literal
	host := u.Host
	if strings.Contains(host, ":") {
		// Contains port or is IPv6
		return "", false
	}
	host = strings.ToLower(host)

	// Check for IPv4 literal
	if ip := net.ParseIP(host); ip != nil {
		return "", false
	}

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}

	// 1. TikTok: tiktok.com (www., m., vm., vt.)
	if tiktokHostRe.MatchString(host) {
		// Valid path check (not empty or just /)
		if path != "/" && !strings.Contains(path, "//") {
			return "tiktok", true
		}
		return "", false
	}

	// 2. YouTube: youtube.com (www., m.) only /shorts/ and /watch; youtu.be
	if youtubeHostRe.MatchString(host) {
		if strings.HasPrefix(path, "/shorts/") && len(path) > len("/shorts/") {
			return "youtube", true
		}
		if path == "/watch" && u.Query().Get("v") != "" {
			return "youtube", true
		}
		return "", false
	}
	if host == "youtu.be" {
		trimmed := strings.Trim(path, "/")
		if trimmed != "" && !strings.Contains(trimmed, "/") {
			return "youtube", true
		}
		return "", false
	}

	// 3. Instagram: instagram.com /reel/ /reels/ /p/ /tv/
	if instagramHostRe.MatchString(host) {
		if instagramPathRe.MatchString(path) {
			return "instagram", true
		}
		return "", false
	}

	// 4. X / Twitter: x.com or twitter.com /<user>/status/<id>
	if xHostRe.MatchString(host) {
		if xStatusRe.MatchString(path) {
			return "x", true
		}
		return "", false
	}

	// 5. Facebook: facebook.com /watch /reel /videos /share/v/ /share/r/; fb.watch
	if facebookHostRe.MatchString(host) {
		if facebookPathRe.MatchString(path) {
			return "facebook", true
		}
		return "", false
	}
	if host == "fb.watch" {
		trimmed := strings.Trim(path, "/")
		if trimmed != "" && !strings.Contains(trimmed, "/") {
			return "facebook", true
		}
		return "", false
	}

	return "", false
}

// ExtractorForPlatform returns the yt-dlp extractors (comma-separated, for
// --use-extractors) that may handle the platform's links. Never the generic one.
func ExtractorForPlatform(platform string) string {
	switch platform {
	case "tiktok":
		// vm.tiktok resolves the short links people share (vm./vt.tiktok.com);
		// TikTok alone rejects them as an unsupported URL.
		return "TikTok,vm.tiktok"
	case "youtube":
		return "Youtube"
	case "instagram":
		return "Instagram"
	case "x":
		return "Twitter"
	case "facebook":
		return "Facebook,facebook:reel"
	default:
		return ""
	}
}

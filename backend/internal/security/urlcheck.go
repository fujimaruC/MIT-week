package security

import (
	"net"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
)

// MaxURLLength is the longest link /audit/url accepts.
const MaxURLLength = 2048

// URLError is a syntactic refusal from ValidateInputURL. Reason is a stable
// code for tests and logs; Message is safe to show to the user (Indonesian).
type URLError struct {
	Reason  string
	Message string
}

func (e *URLError) Error() string { return e.Reason }

// Reason codes returned by ValidateInputURL.
const (
	URLEmpty      = "EMPTY"
	URLTooLong    = "TOO_LONG"
	URLControl    = "CONTROL_CHARS"
	URLScheme     = "SCHEME"
	URLUserinfo   = "USERINFO"
	URLHost       = "HOST"
	URLPort       = "PORT"
	URLDangerous  = "DANGEROUS_CHARS"
	URLUnparsable = "UNPARSABLE"
)

func urlErr(reason, msg string) *URLError { return &URLError{Reason: reason, Message: msg} }

// dangerousEncoded are percent-encodings of < > " ` and of CR, LF and NUL
// (case-insensitive). CR/LF in a link only serve header injection. An
// apostrophe is allowed: news slugs carry it ("it%27s") and nothing on the
// server builds markup, SQL or shell commands from the URL.
var dangerousEncoded = []string{"%3c", "%3e", "%22", "%60", "%0d", "%0a", "%00"}

// dangerousLinkMsg is a conclusion, not a failure: a link built to inject
// code or headers is itself the warning sign.
const dangerousLinkMsg = "Tautan ini berisi karakter berbahaya dan ditolak. Jangan dibuka atau dibagikan."

// ValidateInputURL is the single gate for a user-supplied link. It accepts
// only plain http(s) links, normalises the host to punycode and returns the
// canonical string. It never resolves or fetches anything; the SSRF guard
// (ValidateFetchURL) still runs afterwards.
func ValidateInputURL(raw string) (string, *URLError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", urlErr(URLEmpty, "Tautan kosong. Tempel tautan video atau berita.")
	}
	if len(raw) > MaxURLLength {
		return "", urlErr(URLTooLong, "Tautan terlalu panjang (maksimal 2048 karakter).")
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return "", urlErr(URLControl, "Tautan tidak boleh berisi spasi atau karakter tersembunyi. Tempel satu tautan saja.")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", urlErr(URLUnparsable, "Tautan tidak bisa dibaca. Periksa lagi penulisannya.")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", urlErr(URLScheme, "Tautan harus diawali http:// atau https://.")
	}
	if u.Opaque != "" || u.User != nil || strings.Contains(authority(raw), "@") {
		return "", urlErr(URLUserinfo, "Tautan dengan nama pengguna atau kata sandi tidak diterima.")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return "", urlErr(URLHost, "Tautan tidak punya alamat situs yang jelas.")
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return "", urlErr(URLPort, "Tautan dengan port khusus tidak diterima.")
	}
	if strings.ContainsAny(host, "<>\"'`") || strings.ContainsAny(u.Path, "<>\"`") {
		return "", urlErr(URLDangerous, dangerousLinkMsg)
	}
	lowEsc := strings.ToLower(u.EscapedPath() + "\x00" + u.Host)
	for _, enc := range dangerousEncoded {
		if strings.Contains(lowEsc, enc) {
			return "", urlErr(URLDangerous, dangerousLinkMsg)
		}
	}
	if net.ParseIP(host) == nil {
		ascii, err := idna.Lookup.ToASCII(host)
		if err != nil || ascii == "" {
			return "", urlErr(URLHost, "Alamat situs pada tautan tidak valid.")
		}
		host = ascii
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if p := u.Port(); p != "" {
		host += ":" + p
	}
	u.Scheme = scheme
	u.Host = host
	return u.String(), nil
}

// authority returns the part of raw between "//" and the next "/", "?" or
// "#", so a smuggled "@" is caught even when url.Parse forgives it.
func authority(raw string) string {
	i := strings.Index(raw, "//")
	if i < 0 {
		return ""
	}
	rest := raw[i+2:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

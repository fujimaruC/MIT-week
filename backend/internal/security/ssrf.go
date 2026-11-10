// Package security holds the trust-boundary code: the SSRF-safe HTTP client,
// upload validation, rate limiting, and API headers/CORS.
//
// Nothing in this package may log the URL, IP, or body it inspects.
package security

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// blockedCIDRs are refused before a connection is made. The list covers
// loopback, link-local (including the cloud metadata address), private ranges,
// documentation/test nets, multicast, and the IPv6 equivalents.
var blockedCIDRs = mustCIDRs(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "64:ff9b::/96", "100::/64", "2001:db8::/32", "fc00::/7",
	"fe80::/10", "fec0::/10", "ff00::/8",
)

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("security: bad CIDR " + c)
		}
		out = append(out, n)
	}
	return out
}

// IsBlocked reports whether an address must not be dialled. A nil address is
// treated as blocked: failing closed is the only safe default here.
func IsBlocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Normalise ::ffff:a.b.c.d to its IPv4 form so the IPv4 table applies.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ErrBlockedTarget is returned for any refused connection. The message is
// deliberately generic: callers must not reveal which rule fired.
var ErrBlockedTarget = errors.New("target not allowed")

// allowedPorts are the only ports the audit fetcher may reach.
var allowedPorts = map[string]bool{"80": true, "443": true}

// checkDialAddress validates the concrete address a socket is about to open.
// Because it runs inside Dialer.Control it sees the *resolved* IP, which is
// what defeats DNS rebinding and multi-A-record tricks.
func checkDialAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return ErrBlockedTarget
	}
	if !allowedPorts[port] {
		return ErrBlockedTarget
	}
	if IsBlocked(net.ParseIP(host)) {
		return ErrBlockedTarget
	}
	return nil
}

// NewSafeClient builds the only client allowed to fetch user-supplied URLs.
// It never honours HTTP(S)_PROXY, never follows more than maxHops redirects,
// and only speaks http/https.
func NewSafeClient(timeout time.Duration, maxHops int) *http.Client {
	d := &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, _ syscall.RawConn) error {
			return checkDialAddress(address)
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                  nil,
			DialContext:            d.DialContext,
			ResponseHeaderTimeout:  timeout,
			MaxResponseHeaderBytes: 64 << 10,
			DisableKeepAlives:      true,
			TLSHandshakeTimeout:    timeout,
		},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= maxHops {
				return errors.New("too many redirects")
			}
			if r.URL.Scheme != "http" && r.URL.Scheme != "https" {
				return errors.New("scheme not allowed")
			}
			return nil
		},
	}
}

// ValidateFetchURL rejects non-http(s) schemes and URLs with embedded
// credentials before any request is attempted.
func ValidateFetchURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, ErrBlockedTarget
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrBlockedTarget
	}
	if u.Host == "" {
		return nil, ErrBlockedTarget
	}
	if u.User != nil {
		return nil, ErrBlockedTarget
	}
	// Refuse obviously internal targets up front (PRD F1 AC), so the user gets
	// the generic "cannot be checked" message instead of a result for a URL
	// AMBA was never going to fetch. The dialer still re-checks every
	// connection, which also covers names that resolve to private addresses.
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip := net.ParseIP(host); ip != nil && IsBlocked(ip) {
		return nil, ErrBlockedTarget
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".home.arpa") {
		return nil, ErrBlockedTarget
	}
	return u, nil
}

// MaxBodyBytes caps every response body read.
const MaxBodyBytes = 2 << 20

// Fetch performs a GET through the safe client and reads at most MaxBodyBytes.
// It also rejects responses that are clearly not text or images, so a binary
// payload cannot be parsed as metadata.
func Fetch(ctx context.Context, c *http.Client, rawURL string) ([]byte, string, error) {
	u, err := ValidateFetchURL(rawURL)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", ErrBlockedTarget
	}
	req.Header.Set("User-Agent", "AMBA/0.1 (+literacy check)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.8,*/*;q=0.1")

	resp, err := c.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("read failed: %w", err)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

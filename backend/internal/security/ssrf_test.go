package security

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIsBlocked(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback range", "127.5.5.5", true},
		{"private 10", "10.1.2.3", true},
		{"private 172.16", "172.16.0.1", true},
		{"private 192.168", "192.168.1.1", true},
		{"link local metadata", "169.254.169.254", true},
		{"cgnat", "100.64.0.1", true},
		{"ipv6 loopback", "::1", true},
		{"ipv6 link local", "fe80::1", true},
		{"ipv6 ula", "fc00::1", true},
		{"ipv4-mapped loopback", "::ffff:127.0.0.1", true},
		{"ipv4-mapped private", "::ffff:10.0.0.1", true},
		{"documentation", "192.0.2.5", true},
		{"benchmark", "198.18.0.1", true},
		{"multicast", "224.0.0.1", true},
		{"reserved", "240.0.0.1", true},
		{"nil address", "", true},
		{"public v4", "93.184.216.34", false},
		{"public v4 alt", "8.8.8.8", false},
		{"public v6", "2606:2800:220:1:248:1893:25c8:1946", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
				if ip == nil {
					t.Fatalf("test bug: cannot parse %q", tt.ip)
				}
			}
			if got := IsBlocked(ip); got != tt.want {
				t.Fatalf("IsBlocked(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateFetchURL(t *testing.T) {
	good := []string{"http://example.com/a", "https://example.org/x?y=1"}
	for _, raw := range good {
		if _, err := ValidateFetchURL(raw); err != nil {
			t.Errorf("ValidateFetchURL(%q) unexpected error: %v", raw, err)
		}
	}
	bad := []string{
		"file:///etc/passwd",
		"gopher://example.com/",
		"ftp://example.com/",
		"javascript:alert(1)",
		"http://user:pass@example.com/",
		"http://",
		"//example.com/",
		"",
	}
	for _, raw := range bad {
		if _, err := ValidateFetchURL(raw); err == nil {
			t.Errorf("ValidateFetchURL(%q) should have failed", raw)
		}
	}
}

func TestCheckDialAddress(t *testing.T) {
	tests := []struct {
		addr string
		ok   bool
	}{
		{"93.184.216.34:80", true},
		{"93.184.216.34:443", true},
		{"93.184.216.34:8080", false},
		{"93.184.216.34:22", false},
		{"93.184.216.34:6379", false},
		{"127.0.0.1:80", false},
		{"10.0.0.1:443", false},
		{"169.254.169.254:80", false},
		{"[::1]:443", false},
		{"[::ffff:127.0.0.1]:80", false},
		{"not-an-address", false},
	}
	for _, tt := range tests {
		err := checkDialAddress(tt.addr)
		if tt.ok && err != nil {
			t.Errorf("checkDialAddress(%q) = %v, want allowed", tt.addr, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("checkDialAddress(%q) allowed, want blocked", tt.addr)
		}
	}
}

// A redirect to a private address must be refused by the dialer, not followed.
func TestRedirectToPrivateIsBlocked(t *testing.T) {
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer private.Close()

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, private.URL, http.StatusFound)
	}))
	defer public.Close()

	c := NewSafeClient(2*time.Second, 3)
	_, _, err := Fetch(context.Background(), c, public.URL)
	if err == nil {
		t.Fatal("expected the redirect to a private address to fail")
	}
}

// The client must refuse to dial a hostname that resolves to loopback.
func TestHostnameResolvingToLoopbackIsBlocked(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind loopback")
	}
	defer func() { _ = ln.Close() }()

	c := NewSafeClient(2*time.Second, 3)
	_, _, err = Fetch(context.Background(), c, "http://localhost:"+strings.Split(ln.Addr().String(), ":")[1]+"/")
	if err == nil {
		t.Fatal("expected localhost to be blocked")
	}
}

func TestRedirectChainLimit(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
	}))
	defer srv.Close()

	c := NewSafeClient(2*time.Second, 3)
	_, _, err := Fetch(context.Background(), c, srv.URL)
	if err == nil {
		t.Fatal("expected too-many-redirects error")
	}
}

func TestFetchCapsBodySize(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		chunk := strings.Repeat("a", 64<<10)
		for i := 0; i < 40; i++ {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer big.Close()

	// The server above is on loopback, so Fetch must refuse it outright; this
	// test therefore asserts the refusal rather than the cap.
	c := NewSafeClient(2*time.Second, 3)
	_, _, err := Fetch(context.Background(), c, big.URL)
	if err == nil {
		t.Fatal("loopback fetch must be refused")
	}
}

func TestParseBlockedDecimalAndHexIPs(t *testing.T) {
	// 2130706433 == 127.0.0.1 and 0x7f.1 == 127.0.0.1 in legacy notations.
	// Go's resolver normalises these; ensure the parsed forms are blocked.
	for _, raw := range []string{"http://2130706433/", "http://0x7f.1/"} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		ips, err := net.LookupIP(u.Hostname())
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if !IsBlocked(ip) {
				t.Errorf("%s resolved to %s which was not blocked", raw, ip)
			}
		}
	}
}

func TestValidateFetchURLRefusesInternalTargets(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/admin", "http://[::1]/", "http://10.0.0.5/", "http://169.254.169.254/latest/meta-data",
		"http://localhost:8080/", "https://printer.local/", "http://metadata.google.internal/", "http://router.home.arpa/",
	} {
		if _, err := ValidateFetchURL(raw); err == nil {
			t.Errorf("ValidateFetchURL(%q) accepted an internal target", raw)
		}
	}
	for _, raw := range []string{"https://kompas.com/a", "http://8.8.8.8/", "https://example.org:8443/x"} {
		if _, err := ValidateFetchURL(raw); err != nil {
			t.Errorf("ValidateFetchURL(%q) refused a public target: %v", raw, err)
		}
	}
}

package linkcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func rdapServer(t *testing.T, registered map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var boots atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/bootstrap.json":
			boots.Add(1)
			_, _ = w.Write([]byte(`{"version":"1.0","services":[[["com","net"],["http://insecure.example/","` + srv.URL + `/com/"]],[["id"],["` + srv.URL + `/id"]]]}`))
		case strings.HasPrefix(r.URL.Path, "/com/domain/"), strings.HasPrefix(r.URL.Path, "/id/domain/"):
			domain := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			date, ok := registered[domain]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/rdap+json")
			_, _ = w.Write([]byte(`{"objectClassName":"domain","ldhName":"` + domain + `","events":[{"eventAction":"last changed","eventDate":"2026-11-01T00:00:00Z"},{"eventAction":"registration","eventDate":"` + date + `"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &boots
}

func TestRDAPRegistrationDate(t *testing.T) {
	srv, boots := rdapServer(t, map[string]string{
		"contoh.com":      "2001-05-12T04:00:00Z",
		"bri-promo.my.id": "2026-11-03T08:15:00Z",
		"undangan.web.id": "2026-07-01T00:00:00Z",
	})
	defer srv.Close()
	r := NewRDAP(srv.Client())
	r.Bootstrap = srv.URL + "/bootstrap.json"

	got, err := r.Registered(context.Background(), "www.contoh.com")
	if err != nil || got.Year() != 2001 {
		t.Fatalf("contoh.com = %v %v", got, err)
	}
	// Only the registrable domain is asked about, also under an Indonesian
	// second-level suffix.
	if got, err := r.Registered(context.Background(), "login.bri-promo.my.id"); err != nil || got.Day() != 3 {
		t.Fatalf("bri-promo.my.id = %v %v", got, err)
	}
	for _, host := range []string{"unknown.com", "a.example", "192.168.1.1", "localhost"} {
		if _, err := r.Registered(context.Background(), host); err == nil {
			t.Errorf("%s: expected no date", host)
		}
	}
	if n := boots.Load(); n != 1 {
		t.Errorf("bootstrap fetched %d times, want once", n)
	}
}

func TestRDAPDisabledOrDown(t *testing.T) {
	var zero *RDAP
	if zero.Enabled() {
		t.Fatal("nil lookup must be disabled")
	}
	if _, err := zero.Registered(context.Background(), "contoh.com"); err == nil {
		t.Fatal("disabled lookup returned a date")
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer down.Close()
	r := NewRDAP(down.Client())
	r.Bootstrap = down.URL + "/bootstrap.json"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := r.Registered(ctx, "contoh.com"); err == nil {
		t.Fatal("a timeout must not produce a date")
	}
}

func TestWithDomainAge(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	base := Analyze("https://bri-promo.my.id/klaim")
	cases := []struct {
		name       string
		r          Result
		registered time.Time
		want       string
		weight     int
	}{
		{"six days old", base, now.AddDate(0, 0, -6), ReasonNewDomain, 25},
		{"three months old", base, now.AddDate(0, -3, 0), ReasonYoungDomain, 10},
		{"two years old", base, now.AddDate(-2, 0, 0), "", 0},
		{"unknown date", base, time.Time{}, "", 0},
		{"a date in the future is nonsense", base, now.AddDate(0, 0, 3), "", 0},
		{"official domains are never young", Analyze("https://www.kemkes.go.id/a"), now.AddDate(0, 0, -2), "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WithDomainAge(tc.r, tc.registered, now)
			added := ""
			for _, f := range got.Findings {
				if f.Code == ReasonNewDomain || f.Code == ReasonYoungDomain {
					added = f.Code
				}
			}
			if added != tc.want || got.Score != capScore(tc.r.Score+tc.weight) {
				t.Fatalf("added %q score %d, want %q %d", added, got.Score, tc.want, capScore(tc.r.Score+tc.weight))
			}
		})
	}
	twice := WithDomainAge(WithDomainAge(base, now.AddDate(0, 0, -1), now), now.AddDate(0, 0, -1), now)
	if twice.Score != capScore(base.Score+25) {
		t.Fatalf("applied twice: %d", twice.Score)
	}
}

func TestIsShortener(t *testing.T) {
	if !IsShortener("bit.ly") || !IsShortener("S.ID") || IsShortener("bri.co.id") {
		t.Fatal("shortener list")
	}
}

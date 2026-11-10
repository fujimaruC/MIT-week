package grounding

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const factCheckFixture = `{"claims":[
 {"text":"Bansos Rp 500 ribu cair lewat tautan","claimReview":[
   {"publisher":{"name":"Kompas.com","site":"kompas.com"},"url":"https://www.kompas.com/cekfakta/read/1","title":"[HOAKS] Bansos lewat tautan","textualRating":"Hoaks"},
   {"publisher":{"name":"Blog","site":"evil.example"},"url":"https://evil.example/x","title":"Not allowlisted","textualRating":"Benar"},
   {"publisher":{"site":"turnbackhoax.id"},"url":"https://turnbackhoax.id/2026/1","title":"","textualRating":"Salah"}
 ]},
 {"text":"Duplikat","claimReview":[
   {"publisher":{"name":"Kompas.com"},"url":"https://www.kompas.com/cekfakta/read/1","title":"dup","textualRating":"Hoaks"},
   {"publisher":{"name":"Tempo"},"url":"http://cekfakta.tempo.co/insecure","title":"http only","textualRating":"Keliru"}
 ]}
]}`

func TestFactCheckSearcherFiltersToAllowlist(t *testing.T) {
	var query, lang, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, lang, key = r.URL.Query().Get("query"), r.URL.Query().Get("languageCode"), r.URL.Query().Get("key")
		_, _ = w.Write([]byte(factCheckFixture))
	}))
	defer srv.Close()

	s := NewFactCheckSearcher("secret", New(DefaultAllowlist))
	s.Endpoint = srv.URL
	got, err := s.Search(context.Background(), "  bansos 500 ribu  ")
	if err != nil {
		t.Fatal(err)
	}
	if query != "bansos 500 ribu" || lang != "id" || key != "secret" {
		t.Fatalf("request: query=%q lang=%q key=%q", query, lang, key)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 allowlisted, deduplicated https results, got %+v", got)
	}
	if got[0].URL != "https://www.kompas.com/cekfakta/read/1" || got[0].Publisher != "Kompas.com" || got[0].Rating != "Hoaks" {
		t.Fatalf("first: %+v", got[0])
	}
	// Missing title falls back to the claim text; missing name to the site.
	if got[1].Title != "Bansos Rp 500 ribu cair lewat tautan" || got[1].Publisher != "turnbackhoax.id" {
		t.Fatalf("second: %+v", got[1])
	}
	set := URLSet(got)
	g := New(DefaultAllowlist)
	if !g.Verified(got[1].URL, set) || g.Verified("https://www.kompas.com/cekfakta/read/2", set) {
		t.Fatal("verification must accept exactly the searched URLs")
	}
}

func TestFactCheckSearcherDisabledAndFailing(t *testing.T) {
	if got, err := NewFactCheckSearcher("", New(DefaultAllowlist)).Search(context.Background(), "x"); got != nil || err != nil {
		t.Fatalf("disabled searcher must be a no-op: %v %v", got, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "quota", http.StatusForbidden)
	}))
	defer srv.Close()
	s := NewFactCheckSearcher("k", New(DefaultAllowlist))
	s.Endpoint = srv.URL
	if _, err := s.Search(context.Background(), "x"); err == nil {
		t.Fatal("a failed search must be an error")
	}
}

// A failed search tells operators the status (403: API not enabled for the
// key) without ever logging the key or the user's query.
func TestFailedSearchLogsStatusOnly(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "API not enabled", http.StatusForbidden)
	}))
	defer srv.Close()
	s := NewFactCheckSearcher("secret-key-123", New(DefaultAllowlist))
	s.Endpoint = srv.URL
	_, _ = s.Search(context.Background(), "pesan rahasia pengguna")

	out := logs.String()
	if !strings.Contains(out, `"status":403`) {
		t.Fatalf("want the status in the log, got %q", out)
	}
	for _, leak := range []string{"secret-key-123", "rahasia", srv.URL} {
		if strings.Contains(out, leak) {
			t.Fatalf("log leaks %q: %s", leak, out)
		}
	}
}

package grounding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/tbh-search.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseTBHSearchFixture(t *testing.T) {
	got := ParseTBHSearch(fixture(t), 5)
	if len(got) != 2 {
		t.Fatalf("got %d results: %+v", len(got), got)
	}
	r := got[1]
	if r.URL != "https://turnbackhoax.id/articles/36857-salah-purbaya-dilantik-jadi-gubernur-bank-indonesia" {
		t.Fatalf("url = %q", r.URL)
	}
	if r.Title != "[SALAH] Purbaya Dilantik Jadi Gubernur Bank Indonesia" {
		t.Fatalf("title = %q", r.Title)
	}
	if r.Rating != "[SALAH]" || r.Publisher != "turnbackhoax.id" {
		t.Fatalf("rating/publisher = %q/%q", r.Rating, r.Publisher)
	}
	if !strings.HasPrefix(r.Snippet, "Konteks asli dokumentasi") {
		t.Fatalf("snippet = %q", r.Snippet)
	}
	if got[0].Title != "[SALAH] Usai Dipecat, Purbaya Jadi Pejabat Bank Dunia Pertama dari Indonesia" {
		t.Fatalf("title0 = %q", got[0].Title)
	}
}

func TestParseTBHSearchLimitDedupeAndAnchorText(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<html><body><a href="https://turnbackhoax.id/articles/5-menu-sidebar">Sidebar</a><a href="/articles?page=2">next</a><div class="news-card-h-alt">`)
	b.WriteString(`<a href="https://evil.example/articles/1-x">x</a>`)
	b.WriteString(`<a href="https://turnbackhoax.id/articles/10-hoaks-a">[HOAKS] Judul A Lengkap Faktanya tidak benar</a>`)
	b.WriteString(`<a href="https://turnbackhoax.id/articles/10-hoaks-a">[HOAKS] Judul A Lengkap Faktanya tidak benar</a>`)
	for i := 11; i < 20; i++ {
		b.WriteString(`<a href="https://turnbackhoax.id/articles/` + string(rune('0'+i-10)) + `1-klarifikasi-b">[KLARIFIKASI] Judul B</a>`)
	}
	b.WriteString(`<a href="https://turnbackhoax.id/articles/77-benar-tanpa-teks"></a></div></body></html>`)
	got := ParseTBHSearch(b.String(), 5)
	if len(got) != 5 {
		t.Fatalf("limit: %d", len(got))
	}
	if got[0].Title != "[HOAKS] Judul A Lengkap Faktanya tidak benar" || got[0].Rating != "[HOAKS]" {
		t.Fatalf("anchor title: %+v", got[0])
	}
	for _, r := range got {
		if !strings.HasPrefix(r.URL, "https://turnbackhoax.id/articles/") {
			t.Fatalf("foreign link: %q", r.URL)
		}
	}
	slug := ParseTBHSearch(`<div class="news-card"><a href="https://turnbackhoax.id/articles/77-benar-tanpa-teks"></a></div>`, 5)
	if len(slug) != 1 || slug[0].Title != "benar tanpa teks" {
		t.Fatalf("slug title: %+v", slug)
	}
	if got := ParseTBHSearch("<html>", 5); len(got) != 0 {
		t.Fatal("empty page")
	}
}

func TestTBHSearcherQueryUserAgentAndCache(t *testing.T) {
	var hits int32
	var gotUA, gotQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		gotUA, gotQ = r.UserAgent(), r.URL.Query().Get("query")
		_, _ = w.Write([]byte(fixture(t)))
	}))
	defer srv.Close()
	now := time.Now()
	s := NewTBHSearcher(srv.Client())
	s.Endpoint = srv.URL
	s.Now = func() time.Time { return now }

	res, err := s.Search(context.Background(), "  purbaya   gubernur bank indonesia ")
	if err != nil || len(res) != 2 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if gotQ != "purbaya gubernur bank indonesia" || !strings.HasPrefix(gotUA, "AMBA/2.0") {
		t.Fatalf("query=%q ua=%q", gotQ, gotUA)
	}
	if _, err := s.Search(context.Background(), "Purbaya gubernur bank Indonesia"); err != nil || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("second call should hit the cache, hits=%d", hits)
	}
	now = now.Add(11 * time.Minute)
	if _, err := s.Search(context.Background(), "purbaya gubernur bank indonesia"); err != nil || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expired entry should refetch, hits=%d", hits)
	}
}

func TestTBHSearcherFailuresAndOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer srv.Close()
	s := NewTBHSearcher(srv.Client())
	s.Endpoint = srv.URL
	if _, err := s.Search(context.Background(), "purbaya"); err != ErrSearchFailed {
		t.Fatalf("err = %v", err)
	}
	var off *TBHSearcher
	if off.Enabled() || (&TBHSearcher{}).Enabled() {
		t.Fatal("must be disabled without a client")
	}
	if res, err := s.Search(context.Background(), "   "); res != nil || err != nil {
		t.Fatal("empty query")
	}
}

func TestKeywords(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Purbaya menghadiri peringatan HUT Bea Cukai dengan seragam", "purbaya hut bea cukai"},
		{"Video: Purbaya dilantik jadi Gubernur Bank Indonesia di Istana", "purbaya gubernur bank indonesia istana"},
		{"Erupsi Gunung Lewotobi terlihat dari desa #viral https://x.co/a", "erupsi gunung lewotobi desa"},
		{"", ""},
		{"yang dan di ke dari", ""},
	}
	for _, c := range cases {
		if got := Keywords(c.in, 5); got != c.want {
			t.Errorf("Keywords(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if n := len(strings.Fields(Keywords("Satu Dua Tiga Empat Lima Enam Tujuh", 0))); n != 5 {
		t.Fatalf("max words = %d", n)
	}
	// Proper nouns come before plain words, whatever the order in the text.
	if got := Keywords("kebakaran besar melanda Jakarta Selatan", 3); got != "jakarta selatan kebakaran" {
		t.Fatalf("entities first: %q", got)
	}
}

func TestMergeResultsDedupes(t *testing.T) {
	a := []Result{{URL: "https://a/1"}, {URL: "https://a/2"}}
	b := []Result{{URL: "https://a/2"}, {URL: "https://a/3"}}
	got := MergeResults(a, nil, b)
	if len(got) != 3 || got[2].URL != "https://a/3" {
		t.Fatalf("merge = %+v", got)
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseTBHNoMatchPageReturnsNothing(t *testing.T) {
	if got := ParseTBHSearch(readFixture(t, "tbh-nomatch.html"), 5); len(got) != 0 {
		t.Fatalf("a page that says nothing was found must give nothing: %+v", got)
	}
	// Even if a page that says so also carries article links elsewhere.
	page := readFixture(t, "tbh-nomatch.html") + `<a href="https://turnbackhoax.id/articles/1-salah-x">[SALAH] X</a>`
	if got := ParseTBHSearch(page, 5); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestTBHOnlyResultCardsAreRead(t *testing.T) {
	page := `<html><body><a href="https://turnbackhoax.id/articles/1-salah-sidebar">[SALAH] Sidebar</a></body></html>`
	if got := ParseTBHSearch(page, 5); len(got) != 0 {
		t.Fatalf("links outside the result cards must be ignored: %+v", got)
	}
}

func TestTBHUnrelatedResultsAreDropped(t *testing.T) {
	// "instagram reel" matches the full text of 166 articles, none about the
	// query in the title. The parser sees 4 cards; the relevance filter keeps
	// none.
	all := ParseTBHSearch(readFixture(t, "tbh-unrelated.html"), 20)
	if len(all) < 3 {
		t.Fatalf("fixture should parse, got %d", len(all))
	}
	if got := RelevantToQuery("video instagram reel", all, 5); len(got) != 0 {
		t.Fatalf("unrelated titles kept: %+v", got)
	}
	if got := RelevantToQuery("Satpol keripik kakek", all, 5); len(got) != 1 {
		t.Fatalf("related title dropped: %+v", got)
	}
	if got := RelevantToQuery("video viral", all, 5); len(got) != 0 {
		t.Fatalf("a query with no keyword keeps nothing: %+v", got)
	}
	m := ParseTBHSearch(fixture(t), 5)
	if got := RelevantToQuery("purbaya gubernur bank indonesia", m, 5); len(got) != 2 {
		t.Fatalf("match fixture: %+v", got)
	}
}

func TestTBHSearcherFiltersUnrelated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(readFixture(t, "tbh-unrelated.html")))
	}))
	defer srv.Close()
	s := NewTBHSearcher(srv.Client())
	s.Endpoint = srv.URL
	res, err := s.Search(context.Background(), "instagram reel")
	if err != nil || len(res) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

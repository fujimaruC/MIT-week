package grounding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// A second account takes over when the first one's credits run out, and the
// searcher stays on it: a spent key costs one extra request, not one per search.
func TestTavilyMovesToTheBackupKeyWhenCreditsRunOut(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Authorization")
		mu.Lock()
		asked[key]++
		mu.Unlock()
		if key != "Bearer second" {
			w.WriteHeader(432) // plan credits used up
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"[SALAH] Video banjir Jakarta","url":"https://turnbackhoax.id/a","content":"bukan Jakarta"}]}`))
	}))
	defer srv.Close()

	s := NewTavilySearcher("first", New(DefaultAllowlist), DefaultAllowlist)
	s.Backup = []string{"second"}
	s.Endpoint, s.HTTP = srv.URL, srv.Client()

	for i := 0; i < 2; i++ {
		got, err := s.Search(context.Background(), "banjir jakarta")
		if err != nil || len(got) != 1 || got[0].URL != "https://turnbackhoax.id/a" {
			t.Fatalf("search %d: want the backup key's result, got %v %v", i+1, got, err)
		}
	}
	if asked["Bearer first"] != 1 || asked["Bearer second"] != 2 {
		t.Fatalf("the spent key is asked once, then the backup only: %v", asked)
	}
}

func TestTavilyWithoutBackupStillFailsPlainlyAndAnEmptyBackupIsIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(432) }))
	defer srv.Close()
	s := NewTavilySearcher("only", New(DefaultAllowlist), DefaultAllowlist)
	s.Backup = []string{"  "}
	s.Endpoint, s.HTTP = srv.URL, srv.Client()
	if _, err := s.Search(context.Background(), "banjir jakarta"); err != ErrSearchFailed {
		t.Fatalf("want ErrSearchFailed, got %v", err)
	}
	if !s.Enabled() {
		t.Fatal("a primary key enables the searcher")
	}
	if (&TavilySearcher{Backup: []string{""}}).Enabled() {
		t.Fatal("no usable key: off")
	}
}

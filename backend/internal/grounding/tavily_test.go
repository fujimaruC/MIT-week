package grounding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTavilySearcherKeepsAllowlistedArticlesAndReadsTitleTags(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer tvly-k" {
			t.Errorf("method %s auth %q", r.Method, r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"results":[
			{"title":"[HOAKS] Video Pasukan PBB Tiba di Aceh","url":"https://turnbackhoax.id/2026/11/01/pbb/","content":"Video itu animasi."},
			{"title":"Pasukan PBB","url":"https://evil.example/pbb","content":"x"},
			{"title":"Cek Fakta: Tidak Benar","url":"http://www.liputan6.com/cek-fakta/x","content":"y"},
			{"title":"Klarifikasi soal video","url":"https://www.kompas.com/cekfakta/read/2026/11/02/x","content":"z"}]}`))
	}))
	defer srv.Close()

	s := NewTavilySearcher("tvly-k", New(DefaultAllowlist), DefaultAllowlist)
	s.Endpoint = srv.URL
	res, err := s.Search(context.Background(), "pasukan PBB tiba di Aceh")
	if err != nil || len(res) != 2 {
		t.Fatalf("results = %+v, err = %v (want the 2 https allowlisted ones)", res, err)
	}
	if res[0].Rating != "[HOAKS]" || res[0].Publisher != "turnbackhoax.id" {
		t.Fatalf("first = %+v", res[0])
	}
	if res[1].Rating != "" || res[1].Publisher != "kompas.com" {
		t.Fatalf("second = %+v", res[1])
	}
	if q, _ := got["query"].(string); !strings.Contains(q, "pasukan PBB tiba di Aceh") {
		t.Fatalf("query = %v", got["query"])
	}
	if d, _ := got["include_domains"].([]any); len(d) != len(DefaultAllowlist) || got["search_depth"] != "basic" {
		t.Fatalf("request = %v", got)
	}
}

func TestTavilySearcherFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(432) // the plan's credits are used up
	}))
	defer srv.Close()
	s := NewTavilySearcher("tvly-k", New(DefaultAllowlist), DefaultAllowlist)
	s.Endpoint = srv.URL
	if _, err := s.Search(context.Background(), "klaim"); !errors.Is(err, ErrSearchFailed) {
		t.Fatalf("err = %v, want ErrSearchFailed", err)
	}
	off := NewTavilySearcher("", New(DefaultAllowlist), DefaultAllowlist)
	if res, err := off.Search(context.Background(), "klaim"); off.Enabled() || res != nil || err != nil {
		t.Fatalf("disabled searcher returned %v, %v", res, err)
	}
}

func TestTavilyWebSearchUsesItsOwnPrefixAndWiderAllowlist(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"results":[
			{"title":"Fact check: video is AI","url":"https://www.reuters.com/fact-check/x","content":"AI"},
			{"title":"Blog","url":"https://blog.example/x","content":"x"}]}`))
	}))
	defer srv.Close()
	s := NewTavilySearcher("tvly-k", New(WebAllowlist), WebAllowlist)
	s.Endpoint, s.Prefix = srv.URL, ""
	res, err := s.Search(context.Background(), "banjir jakarta video AI")
	if err != nil || len(res) != 1 || res[0].Publisher != "reuters.com" {
		t.Fatalf("results = %+v, err = %v", res, err)
	}
	if got["query"] != "banjir jakarta video AI" {
		t.Fatalf("query = %v, want no prefix", got["query"])
	}
	// The claim fact check keeps its narrower list: Reuters is not on it.
	if New(DefaultAllowlist).HostAllowed("https://www.reuters.com/x") {
		t.Fatal("DefaultAllowlist must not widen")
	}
}

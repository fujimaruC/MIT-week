package linkcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSafeBrowsingDisabledWithoutKey(t *testing.T) {
	sb := NewSafeBrowsing("")
	hit, err := sb.Check(context.Background(), []string{"https://example.org/"})
	if hit || err != nil || sb.Enabled() {
		t.Fatalf("disabled client must be a no-op, got hit=%v err=%v", hit, err)
	}
}

func TestSafeBrowsingReportsMatches(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "k" {
			http.Error(w, "no key", http.StatusForbidden)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		raw, _ := json.Marshal(gotBody)
		if strings.Contains(string(raw), "evil.example") {
			_, _ = w.Write([]byte(`{"matches":[{"threatType":"SOCIAL_ENGINEERING"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	sb := NewSafeBrowsing("k")
	sb.Endpoint = srv.URL
	sb.HTTP = srv.Client()

	hit, err := sb.Check(context.Background(), []string{"https://bit.ly/x", "https://evil.example/login", "https://evil.example/login"})
	if err != nil || !hit {
		t.Fatalf("expected hit, got hit=%v err=%v", hit, err)
	}
	entries := gotBody["threatInfo"].(map[string]any)["threatEntries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("duplicates must be collapsed, sent %d entries", len(entries))
	}

	hit, err = sb.Check(context.Background(), []string{"https://example.org/"})
	if err != nil || hit {
		t.Fatalf("expected clean, got hit=%v err=%v", hit, err)
	}
}

func TestSafeBrowsingFailureIsNotClean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "quota", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	sb := NewSafeBrowsing("k")
	sb.Endpoint = srv.URL
	if _, err := sb.Check(context.Background(), []string{"https://example.org/"}); err == nil {
		t.Fatal("a failed lookup must surface as an error, not as clean")
	}
}

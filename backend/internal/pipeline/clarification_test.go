package pipeline

import (
	"testing"

	"github.com/amba/amba/backend/internal/models"
)

// 10 Nov: a Kompas clarification scored 85 "dibantah" because TurnBackHoax
// debunked the very hoax the article corrects. A clarification is never
// contradicted by the debunk it echoes; an ordinary page still is.
func TestClarificationIsNotRefutedByTheDebunkItEchoes(t *testing.T) {
	debunk := []string{"[SALAH] Video Banjir Bandang Jakarta Pagi Ini", "https://turnbackhoax.id/articles/37000-salah-video-banjir"}

	if got := claimMatch("CONTRADICTS", true, debunk[0], debunk[1], factContext{Debunk: true}); got != models.FactUnverified {
		t.Fatalf("clarification vs its own debunk = %s, want UNVERIFIED", got)
	}
	if got := claimMatch("CONTRADICTS", true, debunk[0], debunk[1], factContext{}); got != models.FactContradicts {
		t.Fatalf("ordinary page vs debunk = %s, want CONTRADICTS", got)
	}
	// The other rules are unchanged: an unverified source never counts, and
	// only a fact-check article may contradict.
	if got := claimMatch("CONTRADICTS", false, debunk[0], debunk[1], factContext{}); got != models.FactUnverified {
		t.Fatalf("unverified source = %s", got)
	}
	if got := claimMatch("CONTRADICTS", true, "Banjir rendam Jakarta", "https://news.detik.com/berita/d-1/banjir", factContext{}); got != models.FactUnverified {
		t.Fatalf("news page contradicting = %s", got)
	}
	if got := claimMatch("SUPPORTS", true, debunk[0], debunk[1], factContext{Debunk: true}); got != models.FactSupports {
		t.Fatalf("support is kept = %s", got)
	}
}

func TestIsClarificationReadsTheLinksOwnTitleAndAddress(t *testing.T) {
	cases := []struct {
		req   Request
		title string
		want  bool
	}{
		{Request{InputType: models.InputURL, URL: "https://www.kompas.com/tren/read/2026/11/09/101500165/klarifikasi-video-banjir-jakarta"}, "Video banjir Jakarta itu rekaman lama", true},
		{Request{InputType: models.InputURL, URL: "https://www.kompas.com/cekfakta/read/2026/11/09/1/video-banjir"}, "Video banjir Jakarta", true},
		{Request{InputType: models.InputURL, URL: "https://www.kompas.com/tren/read/2026/11/09/1/banjir-rendam-jakarta"}, "[HOAKS] Video banjir Jakarta pagi ini", true},
		{Request{InputType: models.InputURL, URL: "https://www.kompas.com/tren/read/2026/11/09/1/banjir-rendam-jakarta"}, "Banjir rendam Jakarta pagi ini", false},
		{Request{InputType: models.InputURL, URL: "https://www.tiktok.com/@akun/video/1"}, "Banjir bandang menerjang Jakarta", false},
		{Request{InputType: models.InputFile}, "Klarifikasi", false},
	}
	for _, tc := range cases {
		if got := isClarification(tc.req, tc.title); got != tc.want {
			t.Errorf("isClarification(%s, %q) = %v, want %v", tc.req.URL, tc.title, got, tc.want)
		}
	}
}

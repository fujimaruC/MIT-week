package video

import (
	"strings"
	"testing"
)

func TestExtractorsCoverShortLinksAndNeverTheGenericOne(t *testing.T) {
	// Live on 9 Nov every vt.tiktok.com link in the test set was rejected as
	// unsupported: the short links need yt-dlp's vm.tiktok extractor.
	if got := ExtractorForPlatform("tiktok"); !strings.Contains(got, "vm.tiktok") {
		t.Fatalf("tiktok extractors = %q, want vm.tiktok for short links", got)
	}
	for _, p := range []string{"tiktok", "youtube", "instagram", "x", "facebook"} {
		got := strings.ToLower(ExtractorForPlatform(p))
		if got == "" || strings.Contains(got, "generic") || strings.Contains(got, "all") {
			t.Fatalf("%s extractors = %q", p, got)
		}
	}
	if got := ExtractorForPlatform("web"); got != "" {
		t.Fatalf("unknown platform extractors = %q, want none", got)
	}
}

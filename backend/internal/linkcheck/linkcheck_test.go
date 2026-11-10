package linkcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantMin    int
		wantMax    int
		wantReason string
	}{
		{"clean https", "https://example.org/artikel", 0, 0, ""},
		{"clean indonesian news", "https://www.antaranews.com/berita/123", 0, 15, ""},
		{"plain http", "http://example.org/", 15, 15, ReasonNoHTTPS},
		{"ip host", "http://192.168.1.1/login", 50, 100, ReasonIPHost},
		{"credentials in url", "https://user:pass@example.org/", 40, 100, ReasonCredentials},
		{"at symbol trick", "https://example.org@evil.example/", 40, 100, ReasonCredentials},
		{"punycode", "https://xn--80ak6aa92e.com/", 30, 100, ReasonPunycode},
		{"suspicious tld", "https://promo-menarik.xyz/", 25, 100, ReasonSuspiciousTLD},
		{"shortener", "https://bit.ly/abc123", 20, 100, ReasonURLShortener},
		{"nonstandard port", "https://example.org:8443/", 20, 100, ReasonPortNonStandard},
		{"long url", "https://example.org/" + longPath(220), 10, 100, ReasonExcessiveLength},
		{"many subdomains", "https://a.b.c.d.example.org/", 20, 100, ReasonManySubdomains},
		{"suspicious path", "https://example.org/verifikasi-akun", 15, 100, ReasonSuspiciousPath},
		{"bri lookalike", "https://bri-verifikasi.xyz/login", 60, 100, ReasonLookalikeBrand},
		{"bca lookalike hyphen", "https://bca-login.example.org/", 45, 100, ReasonLookalikeBrand},
		{"gopay lookalike", "https://gopay-promo.example.org/", 45, 100, ReasonLookalikeBrand},
		{"real bri domain", "https://bri.co.id/informasi", 0, 20, ""},
		{"real bca domain", "https://bca.co.id/", 0, 20, ""},
		{"real tokopedia", "https://tokopedia.com/produk", 0, 20, ""},
		{"unparseable", "not a url at all", 100, 100, ""},
		{"empty host", "https:///path", 100, 100, ""},
		{"shopee lookalike", "https://shopee-hadiah.icu/klaim", 60, 100, ReasonLookalikeBrand},
		{"qris lookalike", "https://qris-update.example.org/", 45, 100, ReasonLookalikeBrand},
		{"dana lookalike", "https://dana-klaim.xyz/", 60, 100, ReasonLookalikeBrand},
		{"whatsapp lookalike", "https://whatsapp-verifikasi.example.org/", 45, 100, ReasonLookalikeBrand},
		{"pajak lookalike", "https://pajak-refund.example.org/", 45, 100, ReasonLookalikeBrand},
		{"telegram lookalike", "https://telegram-login.example.org/", 45, 100, ReasonLookalikeBrand},
		{"instagram lookalike", "https://instagram-verify.example.org/", 45, 100, ReasonLookalikeBrand},
		{"clean gov", "https://komdigi.go.id/pengumuman", 0, 20, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Analyze(tt.url)
			if got.Score < tt.wantMin || got.Score > tt.wantMax {
				t.Errorf("score = %d, want %d..%d (findings %+v)", got.Score, tt.wantMin, tt.wantMax, got.Findings)
			}
			if tt.wantReason != "" && !hasReason(got.Findings, tt.wantReason) {
				t.Errorf("missing reason %s in %+v", tt.wantReason, got.Findings)
			}
		})
	}
}

func TestScoreIsCappedAt100(t *testing.T) {
	// Every heuristic at once must still clamp to 100.
	got := Analyze("http://user:pass@192.168.1.1:8080/bca-login-verifikasi.xyz/" + longPath(300))
	if got.Score > 100 || got.Score < 0 {
		t.Fatalf("score out of range: %d", got.Score)
	}
}

func TestRegistrableHandlesIndonesianSuffixes(t *testing.T) {
	tests := map[string]string{
		"bri.co.id":         "bri.co.id",
		"www.bri.co.id":     "bri.co.id",
		"example.org":       "example.org",
		"a.b.example.co.id": "example.co.id",
		"sub.example.com":   "example.com",
	}
	for host, want := range tests {
		if got := registrable(host); got != want {
			t.Errorf("registrable(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestUnrollFollowsRedirects(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	mid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer mid.Close()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, mid.URL, http.StatusFound)
	}))
	defer first.Close()

	// These servers are on loopback, which the safe client refuses. Unroll is
	// therefore expected to return an error rather than follow them; the test
	// asserts the refusal is surfaced, not swallowed.
	client := &http.Client{Timeout: 2 * time.Second}
	chain, _, err := Unroll(context.Background(), client, first.URL, 5)
	if err == nil && len(chain) < 2 {
		t.Fatalf("expected either a refusal or a multi-hop chain, got %v", chain)
	}
}

func hasReason(findings []Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func longPath(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

// TestNoFalseBrandMatches pins the precision fix: a brand name that merely
// appears inside an ordinary word, or on the brand's own or a government
// domain, is not a lookalike. These were all flagged by the old substring
// rule, and a link-only verdict now makes such a false alarm visible.
func TestNoFalseBrandMatches(t *testing.T) {
	for _, raw := range []string{
		"https://www.cambridge.org/dictionary",
		"https://www.novotel.com/",
		"https://danantara.id/",
		"https://www.britannica.com/topic/indonesia",
		"https://fabrikasi.co.id/",
		"https://www.klikbca.com/",
		"https://www.bankbsi.co.id/",
		"https://www.bpjs-kesehatan.go.id/",
		"https://djponline.pajak.go.id/",
		"https://chat.whatsapp.com/abc",
		"https://www.telegraph.co.uk/",
		"https://www.bilibili.com/",
		"https://kedanan.com/",
		"https://www.pajakku.com/",
		"https://www.kompas.com/cekfakta",
	} {
		t.Run(raw, func(t *testing.T) {
			got := Analyze(raw)
			if hasReason(got.Findings, ReasonLookalikeBrand) {
				t.Fatalf("false LOOKALIKE_BRAND on %s: %+v", raw, got.Findings)
			}
		})
	}
}

func TestBrandLookalikesStillCaught(t *testing.T) {
	for raw, brand := range map[string]string{
		"https://briverifikasi.com/":         "bri",
		"https://loginbca.net/":              "bca",
		"https://tokopedla.com/promo":        "tokopedia",
		"https://shoppe-hadiah.com/":         "shopee",
		"https://cs-mandiri.online/":         "mandiri",
		"https://www.danabonus.xyz/":         "dana",
		"https://instagrarn.com/login":       "instagram",
		"https://bpjs-kesehatan.co.id/klaim": "bpjs",
		"https://kominfo-bansos.web.id/":     "kominfo",
		"https://xn--br-xyz.com/":            "",
	} {
		t.Run(raw, func(t *testing.T) {
			got := Analyze(raw)
			if brand == "" {
				return
			}
			found := false
			for _, f := range got.Findings {
				if f.Code == ReasonLookalikeBrand && f.Detail == brand {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected LOOKALIKE_BRAND(%s) on %s, got %+v", brand, raw, got.Findings)
			}
		})
	}
}

func TestUnicodeHomoglyphOfOfficialDomainIsFlagged(t *testing.T) {
	// "bri.co.id" spelled with a Cyrillic "і": it folds to the official name,
	// which is exactly why it must not inherit the official exemption.
	got := Analyze("https://brі.co.id/login")
	if !hasReason(got.Findings, ReasonPunycode) || !hasReason(got.Findings, ReasonLookalikeBrand) {
		t.Fatalf("homoglyph host not flagged: %+v", got.Findings)
	}
	if got.Score < 60 {
		t.Fatalf("homoglyph of a bank should be strong evidence, got %d", got.Score)
	}
}

func TestScamKeywordsInHost(t *testing.T) {
	got := Analyze("https://bansos-2026.com/")
	if !hasReason(got.Findings, ReasonScamKeywords) {
		t.Fatalf("missing SCAM_KEYWORDS: %+v", got.Findings)
	}
	clean := Analyze("https://www.hadiahku.com/")
	if hasReason(clean.Findings, ReasonScamKeywords) {
		t.Fatalf("keyword glued into a brand-like word should not fire: %+v", clean.Findings)
	}
}

func TestCombineScoresTheHiddenDestination(t *testing.T) {
	short := Analyze("https://bit.ly/abc123")
	dest := Analyze("https://bri-verifikasi.xyz/login")
	got := Combine(short, dest)
	if !hasReason(got.Findings, ReasonShortenerHides) || !hasReason(got.Findings, ReasonLookalikeBrand) {
		t.Fatalf("combined findings missing: %+v", got.Findings)
	}
	if got.Score <= dest.Score && dest.Score < 100 {
		t.Fatalf("hidden risky destination should add points: short=%d dest=%d got=%d", short.Score, dest.Score, got.Score)
	}
	if got.FinalURL != dest.FinalURL {
		t.Fatalf("final url = %q", got.FinalURL)
	}

	// Same site after redirect (http -> https, www) is not "hiding" anything.
	same := Combine(Analyze("http://kompas.com/a"), Analyze("https://www.kompas.com/a"))
	if hasReason(same.Findings, ReasonShortenerHides) {
		t.Fatalf("same-site redirect flagged: %+v", same.Findings)
	}

	// A shortener leading somewhere clean stays low.
	benign := Combine(short, Analyze("https://www.antaranews.com/berita/1"))
	if hasReason(benign.Findings, ReasonShortenerHides) || benign.Score > 20 {
		t.Fatalf("benign destination over-scored: %+v", benign)
	}
}

func TestSafeBrowsingHitIsMaximal(t *testing.T) {
	got := WithSafeBrowsingHit(Analyze("https://example.org/"))
	if got.Score != 100 || !hasReason(got.Findings, ReasonSafeBrowsing) {
		t.Fatalf("got %+v", got)
	}
	if again := WithSafeBrowsingHit(got); len(again.Findings) != len(got.Findings) {
		t.Fatalf("hit applied twice: %+v", again.Findings)
	}
}

func TestUnparseableHasItsOwnCode(t *testing.T) {
	got := Analyze("not a url at all")
	if got.Score != 100 || !hasReason(got.Findings, ReasonUnparseable) || hasReason(got.Findings, ReasonIPHost) {
		t.Fatalf("got %+v", got)
	}
}

// Real-world shapes found while testing on a public phishing feed and on
// popular legitimate sites (November 2026).
func TestRealWorldShapes(t *testing.T) {
	cases := []struct {
		url      string
		wantCode string
		absent   string
	}{
		// A social handle is not a trick.
		{"https://www.tiktok.com/@kompascom/video/7300000000000000000", "", ReasonAtSymbol},
		{"https://www.youtube.com/@KompasTV", "", ReasonAtSymbol},
		// A pre-filled victim address is.
		{"http://552924.selcdn.ru/department-via-docusign/index10.html?email=a.balao@lazada.com.ph", ReasonAtSymbol, ""},
		{"http://www.example.com/TO/dana_l_holmes@bcbsil.example", ReasonAtSymbol, ""},
		// Cheap Indonesian second-level domains carry most local phishing.
		{"http://aktif.enpres.my.id/bsi", ReasonSuspiciousTLD, ""},
		{"https://promo-dana.biz.id/", ReasonSuspiciousTLD, ""},
		// Brands added after the feed showed them impersonated.
		{"http://scandal2my.btn-app.my.id/sxy", ReasonLookalikeBrand, ""},
		{"https://brimo-tarif.example/", ReasonLookalikeBrand, ""},
		// Their real domains stay clean.
		{"https://www.btn.co.id/", "", ReasonLookalikeBrand},
		{"https://web.pln.co.id/", "", ReasonLookalikeBrand},
		{"https://www.telkomsel.com/", "", ReasonLookalikeBrand},
		{"https://kemensos.go.id/", "", ReasonLookalikeBrand},
		// Everyday words that resemble a brand are not flagged.
		{"https://genius.com/", "", ReasonLookalikeBrand},
	}
	for _, c := range cases {
		r := Analyze(c.url)
		has := map[string]bool{}
		for _, f := range r.Findings {
			has[f.Code] = true
		}
		if c.wantCode != "" && !has[c.wantCode] {
			t.Errorf("%s: want %s, got %+v", c.url, c.wantCode, r.Findings)
		}
		if c.absent != "" && has[c.absent] {
			t.Errorf("%s: %s must not fire, got %+v", c.url, c.absent, r.Findings)
		}
	}
}

// The page check adds a finding only for a secret-asking form on a host that
// is not official, and names impersonation only when the title names a brand.
func TestWithPage(t *testing.T) {
	codes := func(r Result) string {
		var out []string
		for _, f := range r.Findings {
			out = append(out, f.Code)
		}
		return strings.Join(out, ",")
	}

	hacked := Analyze("https://wisata-kita.example/wp-content/klik/")
	got := WithPage(hacked, "KlikBCA Individual", "", true)
	if !strings.Contains(codes(got), ReasonPageImpersonation) || got.Score != hacked.Score+45 || !HighSeverity(ReasonPageImpersonation) {
		t.Fatalf("impersonation: %+v", got)
	}
	if again := WithPage(got, "KlikBCA Individual", "", true); again.Score != got.Score || len(again.Findings) != len(got.Findings) {
		t.Fatalf("applied twice: %+v", again)
	}
	if got := WithPage(hacked, "Portal Siswa", "SMA Harapan", true); !strings.Contains(codes(got), ReasonPageAsksSecret) || got.Score != hacked.Score+20 {
		t.Fatalf("secret form: %+v", got)
	}
	for _, title := range []string{"Dana Pensiun Guru", "Bridge Club Login", "Toko Permata Online", "Konsultan Pajak"} {
		if got := WithPage(hacked, title, "", true); strings.Contains(codes(got), ReasonPageImpersonation) {
			t.Errorf("%q does not name a brand: %+v", title, got)
		}
	}
	for _, title := range []string{"BRImo", "Login myBCA", "BNIDirect", "Livin by Mandiri", "Masuk ke Shopee"} {
		if got := WithPage(hacked, title, "", true); !strings.Contains(codes(got), ReasonPageImpersonation) {
			t.Errorf("%q names a brand: %+v", title, got)
		}
	}
	if got := WithPage(hacked, "KlikBCA Individual", "", false); got.Score != hacked.Score {
		t.Fatalf("no form, no finding: %+v", got)
	}
	official := Analyze("https://ib.bri.co.id/login")
	if got := WithPage(official, "BRI Internet Banking", "", true); got.Score != official.Score {
		t.Fatalf("the bank's own login page: %+v", got)
	}
}

// Lookalikes of the supported platforms are not rejected: they are flagged
// as dangerous, because spotting phishing links is part of the product.
func TestPlatformLookalikesAreFlagged(t *testing.T) {
	for _, raw := range []string{
		"https://tlktok.com/@a/video/1",
		"https://tiktok.com.evil.xyz/@a/video/1",
		"https://www.tiktok.com.login-verify.net/x",
		"https://tіktok.com/@a/video/1", // Cyrillic i
		"https://youtubee.com/watch?v=abc",
		"https://youtube-login.com/watch?v=abc",
		"https://www.youtube.com.evil.xyz/watch?v=abc",
		"https://instagrarn.com/reel/abc",
		"https://faceb00k.com/watch",
		"https://twitter-support.com/x/status/1",
		"https://tiktok-hadiah.top/",
	} {
		t.Run(raw, func(t *testing.T) {
			got := Analyze(raw)
			var high bool
			for _, f := range got.Findings {
				if HighSeverity(f.Code) {
					high = true
				}
			}
			if !high {
				t.Fatalf("no high-severity finding for %s: %+v", raw, got.Findings)
			}
		})
	}
}

func TestExactPlatformHostsAreNotFlagged(t *testing.T) {
	for _, raw := range []string{
		"https://www.tiktok.com/@a/video/1", "https://vt.tiktok.com/ZS1/", "https://vm.tiktok.com/ZM1/",
		"https://www.youtube.com/watch?v=abc", "https://youtu.be/abc", "https://m.youtube.com/shorts/abc",
		"https://www.instagram.com/reel/abc/", "https://www.facebook.com/watch/?v=1", "https://fb.watch/abc/",
		"https://x.com/a/status/1", "https://twitter.com/a/status/1",
	} {
		for _, f := range Analyze(raw).Findings {
			if HighSeverity(f.Code) {
				t.Errorf("%s flagged: %+v", raw, f)
			}
		}
	}
}

package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testResolver points every oEmbed endpoint at srv. Tests use a plain client:
// the SSRF-safe client refuses loopback, which is its own tested behaviour.
func testResolver(srv *httptest.Server) *Resolver {
	r := New(&http.Client{Timeout: 2 * time.Second})
	r.OEmbed = map[string]string{
		"youtube": srv.URL + "/oembed/youtube?url=",
		"tiktok":  srv.URL + "/oembed/tiktok?url=",
		"x":       srv.URL + "/oembed/x?url=",
	}
	return r
}

func TestPlatform(t *testing.T) {
	cases := map[string]string{
		"www.youtube.com": "youtube", "youtu.be": "youtube", "m.youtube.com": "youtube",
		"www.tiktok.com": "tiktok", "vt.tiktok.com": "tiktok",
		"x.com": "x", "twitter.com": "x", "mobile.twitter.com": "x",
		"www.instagram.com": "instagram", "fb.watch": "facebook", "www.facebook.com": "facebook",
		"www.threads.net": "threads", "kompas.com": "web", "notyoutube.com": "web",
	}
	for host, want := range cases {
		if got := Platform(host); got != want {
			t.Errorf("Platform(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestOEmbedTitleAndAuthor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oembed/tiktok" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"Bansos Rp 500.000 cair hari ini!! Klik link di bio","author_name":"info.bansos","provider_name":"TikTok"}`))
	}))
	defer srv.Close()

	// The oEmbed endpoint is asked about the original URL; the platform is
	// decided from the URL's host, not the endpoint's.
	r := testResolver(srv)
	m := r.Resolve(context.Background(), "https://www.tiktok.com/@info.bansos/video/1")
	if m.Status != StatusOK || m.Via != "oembed" || m.Platform != "tiktok" {
		t.Fatalf("got %+v", m)
	}
	if m.Title != "Bansos Rp 500.000 cair hari ini!! Klik link di bio" || m.Author != "info.bansos" || m.SiteName != "TikTok" {
		t.Fatalf("fields: %+v", m)
	}
	if m.Caption() != m.Title {
		t.Fatalf("caption = %q", m.Caption())
	}
	if !strings.Contains(m.ForModel("https://www.tiktok.com/@info.bansos/video/1"), "Pengunggah: info.bansos") {
		t.Fatal("model text misses the author line")
	}
}

func TestXOEmbedUsesPostText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"author_name":"Akun Contoh","html":"<blockquote class=\"twitter-tweet\"><p lang=\"in\" dir=\"ltr\">Sebarkan! Besok listrik padam <a href=\"https://t.co/x\">pic</a> se-Jawa</p>&mdash; Akun</blockquote>"}`))
	}))
	defer srv.Close()

	m := testResolver(srv).Resolve(context.Background(), "https://x.com/akun/status/1")
	if m.Status != StatusOK || m.Description != "Sebarkan! Besok listrik padam pic se-Jawa" {
		t.Fatalf("got %+v", m)
	}
}

func TestOpenGraphFallback(t *testing.T) {
	page := `<!doctype html><html><head>
<title>Fallback title</title>
<meta property="og:title" content="Pemerintah &amp; BUMN bagi hadiah">
<meta name="description" content='Isi   formulir
 sekarang juga.'>
<meta property="og:site_name" content=Contoh>
</head><body><meta property="og:title" content="body tags are ignored"></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	m := testResolver(srv).Resolve(context.Background(), srv.URL+"/artikel")
	if m.Status != StatusOK || m.Via != "opengraph" || m.Platform != "web" {
		t.Fatalf("got %+v", m)
	}
	if m.Title != "Pemerintah & BUMN bagi hadiah" || m.Description != "Isi formulir sekarang juga." || m.SiteName != "Contoh" {
		t.Fatalf("fields: %+v", m)
	}
	if m.Caption() != "Pemerintah & BUMN bagi hadiah\nIsi formulir sekarang juga." {
		t.Fatalf("caption = %q", m.Caption())
	}
}

func TestPreviewImageReference(t *testing.T) {
	cases := map[string]string{
		`<meta property="og:image" content="/img/cover.jpg">`:                                                                                "/artikel/../img/cover.jpg",
		`<meta property="og:image" content="https://cdn.example/a.webp">`:                                                                    "https://cdn.example/a.webp",
		`<meta name="twitter:image" content="//cdn.example/t.png">`:                                                                          "http://cdn.example/t.png",
		`<meta property="og:image:secure_url" content="https://s.example/1.jpg"><meta property="og:image" content="http://s.example/1.jpg">`: "https://s.example/1.jpg",
		`<meta property="og:image" content="javascript:alert(1)">`:                                                                           "",
		`<meta property="og:image" content="data:image/png;base64,AAAA">`:                                                                    "",
		`<meta name="description" content="no image">`:                                                                                       "",
	}
	for tag, want := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>Judul</title>` + tag + `</head></html>`))
		}))
		m := testResolver(srv).Resolve(context.Background(), srv.URL+"/artikel/berita")
		srv.Close()
		if strings.HasPrefix(want, "/") {
			want = srv.URL + "/img/cover.jpg"
		}
		if m.Image != want {
			t.Errorf("%s: image = %q, want %q", tag, m.Image, want)
		}
	}
}

func TestOEmbedThumbnailIsThePreviewImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"title":"Video","author_name":"a","thumbnail_url":"https://p16.example/cover.jpeg"}`))
	}))
	defer srv.Close()
	m := testResolver(srv).Resolve(context.Background(), "https://www.tiktok.com/@a/video/1")
	if m.Image != "https://p16.example/cover.jpeg" {
		t.Fatalf("image = %q", m.Image)
	}
}

func TestPreviewImageDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xffjpeg"))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		case "/big.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, 2048))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	r := testResolver(srv)
	if data, err := r.PreviewImage(context.Background(), srv.URL+"/ok.jpg", 1024); err != nil || string(data) != "\xff\xd8\xffjpeg" {
		t.Fatalf("ok image: %q %v", data, err)
	}
	for _, path := range []string{"/page", "/big.png", "/missing.jpg"} {
		if _, err := r.PreviewImage(context.Background(), srv.URL+path, 1024); err == nil {
			t.Errorf("%s: expected an error", path)
		}
	}
}

func TestBlockedPlatforms(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"forbidden", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusForbidden) }},
		{"rate limited", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "slow down", http.StatusTooManyRequests) }},
		{"login redirect", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/accounts/login/" {
				_, _ = w.Write([]byte(`<html><head><title>Login</title></head></html>`))
				return
			}
			http.Redirect(w, r, "/accounts/login/", http.StatusFound)
		}},
		{"generic login page", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>Instagram</title></head></html>`))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			r := testResolver(srv)
			// Pretend the test server is Instagram by resolving its URL through
			// a client that rewrites the host.
			r.Client = rewriteTo(srv)
			m := r.Resolve(context.Background(), "https://www.instagram.com/p/abc/")
			if m.Status != StatusBlocked || m.Platform != "instagram" {
				t.Fatalf("got %+v", m)
			}
		})
	}
}

func TestUnavailable(t *testing.T) {
	notHTML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer notHTML.Close()
	if m := testResolver(notHTML).Resolve(context.Background(), notHTML.URL+"/a.png"); m.Status != StatusUnavailable {
		t.Fatalf("image url: %+v", m)
	}

	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer gone.Close()
	if m := testResolver(gone).Resolve(context.Background(), gone.URL+"/x"); m.Status != StatusUnavailable {
		t.Fatalf("404: %+v", m)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head></head><body>hi</body></html>`))
	}))
	defer empty.Close()
	if m := testResolver(empty).Resolve(context.Background(), empty.URL); m.Status != StatusUnavailable {
		t.Fatalf("no tags: %+v", m)
	}

	if m := testResolver(empty).Resolve(context.Background(), "::not a url"); m.Status != StatusUnavailable {
		t.Fatalf("bad url: %+v", m)
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if m := testResolver(slow).Resolve(ctx, slow.URL); m.Status != StatusUnavailable {
		t.Fatalf("got %+v", m)
	}
}

func TestCleanTruncatesOnRuneBoundary(t *testing.T) {
	long := strings.Repeat("é", maxTitle+20)
	got := clean(long, maxTitle)
	if n := len([]rune(got)); n != maxTitle {
		t.Fatalf("rune length = %d", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatal("truncation should be marked")
	}
	if clean("a\x00b​c", 10) != "ab​c" {
		t.Fatalf("control characters must go: %q", clean("a\x00b​c", 10))
	}
}

// rewriteTo returns a client that sends every request to srv, whatever host
// the URL names, so a test server can stand in for a real platform.
func rewriteTo(srv *httptest.Server) *http.Client {
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
			r2 := req.Clone(req.Context())
			r2.URL.Scheme = "http"
			r2.URL.Host = strings.TrimPrefix(srv.URL, "http://")
			r2.Host = r2.URL.Host
			return http.DefaultTransport.RoundTrip(r2)
		}),
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Inspect reports a field for a password, PIN, OTP, card number or CVV, and
// nothing for ordinary inputs.
func TestInspectFindsSecretFields(t *testing.T) {
	pages := map[string]struct {
		html  string
		asks  bool
		title string
	}{
		"/password": {`<html><head><title>Login BRImo</title></head><body><form><input type="text" name="user"><input type="password" name="pw"></form></body></html>`, true, "Login BRImo"},
		"/otp":      {`<html><head><meta property="og:title" content="Verifikasi"></head><body><input name="kode" placeholder="Masukkan kode OTP"></body></html>`, true, "Verifikasi"},
		"/ac":       {`<html><body><input autocomplete="one-time-code"></body></html>`, true, ""},
		"/card":     {`<html><body><input id="cardNumber" placeholder="Nomor kartu"><input name="cvv"></body></html>`, true, ""},
		"/pin":      {`<html><body><label>PIN</label><input type=tel name=pin maxlength=6></body></html>`, true, ""},
		"/loan":     {`<html><head><title>Simulasi pinjaman</title></head><body><input name="pinjaman" placeholder="Jumlah pinjaman"></body></html>`, false, "Simulasi pinjaman"},
		"/news":     {`<html><head><title>Berita</title></head><body><input type="email" name="newsletter" placeholder="Email Anda"><input type="search" name="q"></body></html>`, false, "Berita"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(pages[r.URL.Path].html))
	}))
	defer srv.Close()
	r := testResolver(srv)

	for path, want := range pages {
		page, err := r.Inspect(context.Background(), srv.URL+path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if page.AsksSecret != want.asks || page.Title != want.title {
			t.Errorf("%s: %+v, want asks=%v title=%q", path, page, want.asks, want.title)
		}
	}
	if _, err := r.Inspect(context.Background(), srv.URL+"/image"); err == nil {
		t.Error("an image is not a web page")
	}
}

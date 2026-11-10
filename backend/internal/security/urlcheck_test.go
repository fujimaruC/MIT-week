package security

import (
	"strings"
	"testing"
)

func TestValidateInputURLRejects(t *testing.T) {
	cases := []struct{ name, in, reason string }{
		{"empty", "", URLEmpty},
		{"spaces only", "   ", URLEmpty},
		{"javascript", "javascript:alert(1)", URLScheme},
		{"javascript mixed case", "JaVaScRiPt:alert(1)", URLScheme},
		{"javascript with tab", "java\tscript:alert(1)", URLControl},
		{"javascript with newline", "java\nscript:alert(1)", URLControl},
		{"data html", "data:text/html,<script>alert(1)</script>", URLScheme},
		{"data base64", "data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==", URLScheme},
		{"vbscript", "vbscript:msgbox(1)", URLScheme},
		{"file", "file:///etc/passwd", URLScheme},
		{"ftp", "ftp://example.com/x", URLScheme},
		{"gopher", "gopher://127.0.0.1:25/_x", URLScheme},
		{"no scheme", "tiktok.com/@a/video/1", URLScheme},
		{"scheme relative", "//evil.com/x", URLScheme},
		{"userinfo", "https://user:pass@example.com/", URLUserinfo},
		{"userinfo lookalike", "https://www.tiktok.com@evil.com/video/1", URLUserinfo},
		{"at in authority only", "https://@evil.com/", URLUserinfo},
		{"script tag in path", "https://example.com/<script>alert(1)</script>", URLDangerous},
		{"quote in path", "https://example.com/a\"onmouseover=\"x", URLDangerous},
		{"backtick in path", "https://example.com/a`b", URLDangerous},
		{"apostrophe in host", "https://exa'mple.com/", URLDangerous},
		{"encoded lt", "https://example.com/%3Cscript%3E", URLDangerous},
		{"encoded quote lower", "https://example.com/a%22b", URLDangerous},
		{"encoded backtick", "https://example.com/a%60b", URLDangerous},
		{"encoded backtick upper", "https://example.com/a%60B%60", URLDangerous},
		{"encoded CRLF header injection", "https://tiktok.com/test%0d%0aSet-Cookie:c=1", URLDangerous},
		{"encoded LF upper", "https://example.com/a%0Ab", URLDangerous},
		{"encoded NUL", "https://example.com/a%00b", URLDangerous},
		{"encoded in host", "https://exa%3Cmple.com/", URLUnparsable},
		{"port 8080", "https://example.com:8080/", URLPort},
		{"port 22", "http://example.com:22/", URLPort},
		{"space inside", "https://example.com/a b", URLControl},
		{"carriage return", "https://example.com/a\r\nHost: evil", URLControl},
		{"null byte", "https://example.com/\x00", URLControl},
		{"zero width", "https://exam​ple.com/", URLControl},
		{"bidi override", "https://example.com/‮gpj.exe", URLControl},
		{"two links pasted", "https://a.com/x https://b.com/y", URLControl},
		{"too long", "https://example.com/" + strings.Repeat("a", 2100), URLTooLong},
		{"no host", "https:///path", URLHost},
		{"bad port text", "https://example.com:abc/", URLUnparsable},
		{"ctl in host percent", "https://exa%00mple.com/", URLUnparsable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ValidateInputURL(tc.in)
			if err == nil {
				t.Fatalf("accepted %q as %q", tc.in, out)
			}
			if err.Reason != tc.reason {
				t.Fatalf("reason = %s, want %s", err.Reason, tc.reason)
			}
			if err.Message == "" {
				t.Fatal("empty user message")
			}
		})
	}
	if len(cases) < 25 {
		t.Fatal("need at least 25 attack strings")
	}
}

func TestValidateInputURLAccepts(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.tiktok.com/@a/video/1", "https://www.tiktok.com/@a/video/1"},
		{"  https://youtu.be/dQw4w9WgXcQ  ", "https://youtu.be/dQw4w9WgXcQ"},
		{"HTTPS://Example.COM/Path", "https://example.com/Path"},
		{"http://example.com:80/x", "http://example.com:80/x"},
		{"https://example.com:443/x?q=1", "https://example.com:443/x?q=1"},
		{"https://example.com./x", "https://example.com/x"},
		{"https://bücher.de/x", "https://xn--bcher-kva.de/x"},
		{"https://tіktok.com/x", "https://xn--tktok-n2e.com/x"},
		{"https://www.youtube.com/watch?v=abc&t=10s", "https://www.youtube.com/watch?v=abc&t=10s"},
		{"https://example.com/a?b='c'", "https://example.com/a?b='c'"},
		// News slugs carry apostrophes; they are harmless to the server.
		{"https://www.bbc.com/news/world-it's-official", "https://www.bbc.com/news/world-it's-official"},
		{"https://example.com/it%27s-official", "https://example.com/it%27s-official"},
	}
	for _, tc := range cases {
		got, err := ValidateInputURL(tc.in)
		if err != nil {
			t.Errorf("%q rejected: %s", tc.in, err.Reason)
			continue
		}
		if got != tc.want {
			t.Errorf("%q -> %q, want %q", tc.in, got, tc.want)
		}
	}
}

package redact

import (
	"strings"
	"testing"
)

func TestURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"no userinfo", "https://example.com/path?a=b", "https://example.com/path?a=b"},
		{"username and password", "https://user-secret:pass-secret@example.com/p", "https://redacted@example.com/p"},
		{"username only", "https://user-secret@example.com/p", "https://redacted@example.com/p"},
		{"unparsable", "https://exa mple.com/\x7f", Value},
		{"schemeless with credentials", "user-secret:pass-secret@example.com/p", Value},
		{"schemeless with username", "user-secret@example.com/p", Value},
		{"at sign in path", "https://example.com/@handle", "https://example.com/@handle"},
		{"ipv6 host", "https://user-secret:pass-secret@[::1]:8080/p", "https://redacted@[::1]:8080/p"},
		{"password only", "https://:pass-secret@example.com/", "https://redacted@example.com/"},
		{"relative path", "/local/path", "/local/path"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URL(tt.raw)
			if got != tt.want {
				t.Errorf("URL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
			if strings.Contains(got, "secret") {
				t.Errorf("URL(%q) leaked a credential: %q", tt.raw, got)
			}
		})
	}
}

func TestHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"name and value", "Authorization: Bearer secret", "Authorization: " + Value},
		{"no colon", "secret-without-colon", Value},
		{"blank name", " : secret", ": " + Value},
		{"empty value", "X-Empty:   ", "X-Empty:"},
		{"multiple colons", "Cookie: a=1; b=secret:2", "Cookie: " + Value},
		{"empty", "", Value},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Header(tt.header)
			if got != tt.want {
				t.Errorf("Header(%q) = %q, want %q", tt.header, got, tt.want)
			}
			if strings.Contains(got, "secret") {
				t.Errorf("Header(%q) leaked a value: %q", tt.header, got)
			}
		})
	}
}

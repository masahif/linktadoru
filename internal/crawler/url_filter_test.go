package crawler

import "testing"

func TestURLPolicyImplicitOriginAndExclude(t *testing.T) {
	policy, err := newURLPolicy(
		[]string{"https://", "http://"},
		nil,
		[]string{`/ika/`},
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.setImplicitOrigins([]string{"https://example.com/seed"}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/news", true},
		{"https://example.com:443/news", true},
		{"http://example.com/news", false},
		{"https://example.com.evil.test/news", false},
		{"https://example.com/ika/page", false},
		{"https://example.com/%69ka/page", true},
		{"https://example.com@evil.test/news", false},
		{"javascript:alert(1)", false},
	}
	for _, tt := range tests {
		if got := policy.allows(tt.url); got != tt.want {
			t.Errorf("allows(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestURLPolicyIncludeRequiresFullStringMatch(t *testing.T) {
	policy, err := newURLPolicy(
		[]string{"https://"},
		[]string{`^https://good\.example/$|https://extra\.example/search`},
		nil,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://good.example/", true},
		{"https://good.example/path", false},
		{"https://extra.example/search", true},
		{"https://extra.example/search/more", false},
	}
	for _, tt := range tests {
		if got := policy.allows(tt.url); got != tt.want {
			t.Errorf("allows(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestURLPolicyRejectsLegacyRelativeInclude(t *testing.T) {
	if _, err := newURLPolicy(nil, []string{`/products/`}, nil, false); err == nil {
		t.Fatal("relative include pattern was accepted")
	}
}

func TestURLPolicyFollowExternalIgnoresIncludeAndHonorsExclude(t *testing.T) {
	policy, err := newURLPolicy(
		nil,
		[]string{`^https://included\.example/only$`},
		[]string{`/private/`},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.allows("https://other.example/public/page") {
		t.Fatal("include_patterns unexpectedly narrowed follow_external_hosts")
	}
	if policy.allows("https://other.example/private/page") {
		t.Fatal("exclude did not override follow_external_hosts")
	}
}

func TestCompilePatternsRejectsInvalidRegexp(t *testing.T) {
	if _, err := compilePatterns([]string{"["}); err == nil {
		t.Fatal("invalid regexp was accepted")
	}
}

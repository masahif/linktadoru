package crawler

// Regression tests for redirect handling: a redirect must pass the same host
// filter as a freshly discovered URL (otherwise a 302 becomes an SSRF primitive),
// and credentials must not follow the crawler across an origin change.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
)

// newRedirectTestCrawler builds a crawler over the given seed so the redirect
// policy is wired exactly as production does it.
func newRedirectTestCrawler(t *testing.T, seedURL string, followExternal bool) *DefaultCrawler {
	t.Helper()

	cfg := &config.CrawlConfig{
		SeedURLs:            []string{seedURL},
		UserAgent:           "LinkTadoru-Test/1.0",
		RequestTimeout:      5 * time.Second,
		MaxResponseSize:     1 << 20,
		AllowedSchemes:      []string{"https://", "http://"},
		FollowExternalHosts: followExternal,
		IgnoreRobotsTxt:     true,
	}

	crawler, err := NewCrawler(cfg, nil)
	if err != nil {
		t.Fatalf("NewCrawler failed: %v", err)
	}
	if err := crawler.urlPolicy.setImplicitOrigins([]string{seedURL}); err != nil {
		t.Fatalf("setImplicitOrigins failed: %v", err)
	}
	return crawler
}

type credentialWiringStorage struct {
	MockStorage
	items []*URLItem
	next  int
}

func (s *credentialWiringStorage) AddSeeds(urls []string) error {
	return s.MockStorage.AddSeeds(urls)
}

func (s *credentialWiringStorage) GetNextFromQueue() (*URLItem, error) {
	if s.next >= len(s.items) {
		return nil, nil
	}
	item := s.items[s.next]
	s.next++
	return item, nil
}

func runCredentialWiringCrawl(t *testing.T, seed, queued string, includes []string) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.SeedURLs = []string{seed}
	cfg.IncludePatterns = includes
	cfg.Concurrency = 1
	cfg.RequestDelay = 0
	cfg.IgnoreRobotsTxt = true
	cfg.Auth = &config.Auth{
		Type:   config.BearerAuthType,
		Bearer: &config.BearerAuth{Token: "bearer-secret"},
	}
	cfg.Headers = []string{
		"X-Api-Key: api-secret",
		"Cookie: session=secret",
		"X-Internal-Token: custom-secret",
	}
	store := &credentialWiringStorage{items: []*URLItem{{ID: 1, URL: queued, Depth: 0}}}
	crawler, err := NewCrawler(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = crawler.Stop() }()
	if err := crawler.Start(context.Background(), cfg.SeedURLs); err != nil {
		t.Fatal(err)
	}
}

func TestStartSeparatesAccumulatedRootsFromCurrentCredentialOrigins(t *testing.T) {
	var oldHeaders, currentHeaders http.Header
	oldRoot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldHeaders = r.Header.Clone()
		_, _ = w.Write([]byte("old"))
	}))
	defer oldRoot.Close()
	currentSeed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentHeaders = r.Header.Clone()
		_, _ = w.Write([]byte("current"))
	}))
	defer currentSeed.Close()

	cfg := config.DefaultConfig()
	cfg.SeedURLs = []string{currentSeed.URL}
	cfg.Concurrency = 1
	cfg.RequestDelay = 0
	cfg.IgnoreRobotsTxt = true
	cfg.Auth = &config.Auth{
		Type:   config.BearerAuthType,
		Bearer: &config.BearerAuth{Token: "bearer-secret"},
	}
	store := &credentialWiringStorage{
		MockStorage: MockStorage{depthZero: []string{oldRoot.URL}},
		items: []*URLItem{
			{ID: 1, URL: oldRoot.URL, Depth: 0},
			{ID: 2, URL: currentSeed.URL, Depth: 0},
		},
	}
	crawler, err := NewCrawler(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = crawler.Stop() }()
	if err := crawler.Start(context.Background(), cfg.SeedURLs); err != nil {
		t.Fatal(err)
	}

	if oldHeaders == nil || currentHeaders == nil {
		t.Fatalf("accumulated roots were not both fetched: old=%v current=%v", oldHeaders != nil, currentHeaders != nil)
	}
	if got := oldHeaders.Get("Authorization"); got != "" {
		t.Fatalf("credential reached old accumulated root: %q", got)
	}
	if got := currentHeaders.Get("Authorization"); got == "" {
		t.Fatal("credential missing from current explicit seed origin")
	}
}

// A redirect that stays on the seed host is followed as before.
func TestRedirectSameHostIsFollowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>final</html>"))
	}))
	defer server.Close()

	crawler := newRedirectTestCrawler(t, server.URL, false)

	resp, err := crawler.httpClient.Get(context.Background(), server.URL+"/start")
	if err != nil {
		t.Fatalf("same-host redirect was rejected: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if want := server.URL + "/final"; resp.FinalURL != want {
		t.Errorf("FinalURL = %q, want %q", resp.FinalURL, want)
	}
}

// With follow_external_hosts=false a redirect to another host must fail, and
// the external target must never be contacted.
func TestRedirectToExternalHostIsRejected(t *testing.T) {
	var externalHits int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&externalHits, 1)
		_, _ = w.Write([]byte("secret internal content"))
	}))
	defer external.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, external.URL+"/latest/meta-data/", http.StatusFound)
	}))
	defer origin.Close()

	crawler := newRedirectTestCrawler(t, origin.URL, false)

	_, err := crawler.httpClient.Get(context.Background(), origin.URL+"/start")
	if !errors.Is(err, ErrRedirectNotAllowed) {
		t.Fatalf("err = %v, want ErrRedirectNotAllowed", err)
	}
	if hits := atomic.LoadInt32(&externalHits); hits != 0 {
		t.Errorf("external host was contacted %d time(s); redirect target must not be reached", hits)
	}
}

// With follow_external_hosts=true the redirect is allowed, but the credentials
// configured for the origin must not be forwarded to the new host.
func TestRedirectToExternalHostDropsCredentials(t *testing.T) {
	var got http.Header
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>external</html>"))
	}))
	defer external.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, external.URL+"/landing", http.StatusFound)
	}))
	defer origin.Close()

	crawler := newRedirectTestCrawler(t, origin.URL, true)
	crawler.httpClient.SetAPIKeyAuth("X-Api-Key", "super-secret")
	crawler.httpClient.SetCustomHeaders(map[string]string{"X-Internal-Token": "also-secret"})

	resp, err := crawler.httpClient.Get(context.Background(), origin.URL+"/start")
	if err != nil {
		t.Fatalf("external redirect was rejected despite follow_external_hosts=true: %v", err)
	}
	if want := external.URL + "/landing"; resp.FinalURL != want {
		t.Errorf("FinalURL = %q, want %q", resp.FinalURL, want)
	}

	for _, name := range []string{"X-Api-Key", "X-Internal-Token", "Authorization"} {
		if v := got.Get(name); v != "" {
			t.Errorf("%s leaked to the external host: %q", name, v)
		}
	}
	// Non-secret headers still travel, so the crawl keeps working.
	if got.Get("User-Agent") != "LinkTadoru-Test/1.0" {
		t.Errorf("User-Agent = %q, want it preserved across the redirect", got.Get("User-Agent"))
	}
}

// Basic and bearer auth ride in Authorization; both must be dropped off-origin.
func TestRedirectDropsBearerAuthorization(t *testing.T) {
	var got http.Header
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("ok"))
	}))
	defer external.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, external.URL+"/landing", http.StatusFound)
	}))
	defer origin.Close()

	crawler := newRedirectTestCrawler(t, origin.URL, true)
	crawler.httpClient.SetBearerAuth("token-value")

	if _, err := crawler.httpClient.Get(context.Background(), origin.URL+"/start"); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if v := got.Get("Authorization"); v != "" {
		t.Errorf("Authorization leaked to the external host: %q", v)
	}
}

func TestStartWiresCredentialPolicyForDirectIncludeOnlyRequest(t *testing.T) {
	var got http.Header
	includeOnly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("ok"))
	}))
	defer includeOnly.Close()

	include := `^` + regexp.QuoteMeta(includeOnly.URL) + `/page$`
	runCredentialWiringCrawl(t, "https://seed.example/start", includeOnly.URL+"/page", []string{include})

	for _, name := range []string{"Authorization", "X-Api-Key", "X-Internal-Token", "Cookie"} {
		if value := got.Get(name); value != "" {
			t.Errorf("%s reached include-only origin: %q", name, value)
		}
	}
}

func TestStartWiresMonotonicCredentialPolicyAcrossABARedirect(t *testing.T) {
	var aURL string
	var initialA, atB, returnedA http.Header

	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atB = r.Header.Clone()
		http.Redirect(w, r, aURL+"/return", http.StatusFound)
	}))
	defer b.Close()

	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			initialA = r.Header.Clone()
			http.Redirect(w, r, b.URL+"/bounce", http.StatusFound)
		case "/return":
			returnedA = r.Header.Clone()
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer a.Close()
	aURL = a.URL

	include := `^` + regexp.QuoteMeta(b.URL) + `/bounce$`
	runCredentialWiringCrawl(t, a.URL+"/start", a.URL+"/start", []string{include})

	secretHeaders := []string{"Authorization", "X-Api-Key", "Cookie", "X-Internal-Token"}
	for _, name := range secretHeaders {
		if initialA.Get(name) == "" {
			t.Errorf("%s missing from initial seed-origin request", name)
		}
		if value := atB.Get(name); value != "" {
			t.Errorf("%s leaked at B: %q", name, value)
		}
		if value := returnedA.Get(name); value != "" {
			t.Errorf("%s reappeared after A-B-A redirect: %q", name, value)
		}
	}
}

func TestStartKeepsCredentialsOnSameOriginRedirect(t *testing.T) {
	var initial, redirected http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			initial = r.Header.Clone()
			http.Redirect(w, r, "/return", http.StatusFound)
		case "/return":
			redirected = r.Header.Clone()
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer server.Close()

	runCredentialWiringCrawl(t, server.URL+"/start", server.URL+"/start", nil)
	for _, name := range []string{"Authorization", "X-Api-Key", "Cookie", "X-Internal-Token"} {
		if initial.Get(name) == "" || redirected.Get(name) == "" {
			t.Errorf("%s was not retained on same-origin redirect", name)
		}
	}
}

// A client with no policy installed keeps its previous behaviour: redirects are
// followed, bounded only by the hop limit.
func TestRedirectHopLimitStillApplies(t *testing.T) {
	var hops int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hops, 1)
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer server.Close()

	client := NewHTTPClient("LinkTadoru-Test/1.0", 5*time.Second)

	_, err := client.Get(context.Background(), server.URL+"/start")
	if err == nil {
		t.Fatal("endless redirect chain was followed without error")
	}
	if hops := atomic.LoadInt32(&hops); hops > maxRedirects+1 {
		t.Errorf("followed %d hops, want at most %d", hops, maxRedirects+1)
	}
}

// The policy sees each hop, not just the first one.
func TestRedirectPolicyIsConsultedPerHop(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/second", http.StatusFound)
		case "/second":
			http.Redirect(w, r, "/blocked", http.StatusFound)
		default:
			_, _ = w.Write([]byte("must not be reached"))
		}
	}))
	defer server.Close()

	client := NewHTTPClient("LinkTadoru-Test/1.0", 5*time.Second)
	client.SetRedirectPolicy(func(u *url.URL) bool {
		seen = append(seen, u.Path)
		return u.Path != "/blocked"
	})

	_, err := client.Get(context.Background(), server.URL+"/start")
	if !errors.Is(err, ErrRedirectNotAllowed) {
		t.Fatalf("err = %v, want ErrRedirectNotAllowed", err)
	}
	if len(seen) != 2 || seen[0] != "/second" || seen[1] != "/blocked" {
		t.Errorf("policy saw %v, want [/second /blocked]", seen)
	}
}

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
	return crawler
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

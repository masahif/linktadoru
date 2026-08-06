package crawler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"
)

// depthTestServer serves a link graph and records the order pages were fetched
// in, so a test can assert not just what was crawled but when.
type depthTestServer struct {
	*httptest.Server

	mu    sync.Mutex
	order []string
	fail  map[string]int // path -> remaining failures to inject
}

// newDepthTestServer serves one page per entry of graph, linking to the paths
// it maps to. A path listed in fail returns a transport-ish failure that many
// times before succeeding.
func newDepthTestServer(t *testing.T, graph map[string][]string, fail map[string]int) *depthTestServer {
	t.Helper()

	s := &depthTestServer{fail: fail}
	if s.fail == nil {
		s.fail = map[string]int{}
	}

	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		s.mu.Lock()
		s.order = append(s.order, path)
		remaining := s.fail[path]
		if remaining > 0 {
			s.fail[path] = remaining - 1
		}
		s.mu.Unlock()

		if remaining > 0 {
			// Close without a response so the client sees a transport failure,
			// which is what the retry path is scoped to.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Errorf("test server cannot hijack connections")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack failed: %v", err)
				return
			}
			_ = conn.Close()
			return
		}

		links, ok := graph[path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		var body strings.Builder
		body.WriteString("<html><body>")
		for _, target := range links {
			fmt.Fprintf(&body, `<a href="%s">link</a>`, target)
		}
		body.WriteString("</body></html>")

		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body.String()))
	}))
	t.Cleanup(s.Close)

	return s
}

func (s *depthTestServer) requestOrder() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.order...)
}

// runDepthCrawl crawls the server from /start with the given bound and returns
// the storage so the test can inspect depths.
func runDepthCrawl(t *testing.T, srv *depthTestServer, maxDepth, limit int) (*storage.SQLiteStorage, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "depth.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	seed := srv.URL + "/start"
	cfg := &config.CrawlConfig{
		SeedURLs:        []string{seed},
		MaxDepth:        maxDepth,
		Limit:           limit,
		Concurrency:     2,
		RequestDelay:    0.1,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.Start(ctx, cfg.SeedURLs); err != nil {
		t.Fatalf("crawl failed: %v", err)
	}
	return store, dbPath
}

// crawledPaths returns the paths that were actually fetched, sorted.
func crawledPaths(t *testing.T, dbPath, base string) []string {
	t.Helper()

	rows, err := storageQuery(dbPath, "SELECT url FROM pages WHERE status = 'completed' ORDER BY url")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	var paths []string
	for _, url := range rows {
		paths = append(paths, strings.TrimPrefix(url, base))
	}
	sort.Strings(paths)
	return paths
}

// depthOf returns the recorded depth of a path, and whether it is non-NULL.
func depthOf(t *testing.T, dbPath, base, path string) (int, bool) {
	t.Helper()

	depth, ok, err := storageDepth(dbPath, base+path)
	if err != nil {
		t.Fatalf("depth query failed: %v", err)
	}
	return depth, ok
}

// A chain 0 -> 1 -> 2 -> 3 is cut at exactly the configured depth, and every
// depth in between is included.
func TestMaxDepthBoundsAChain(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/one"},
		"/one":   {"/two"},
		"/two":   {"/three"},
		"/three": {},
	}

	tests := []struct {
		maxDepth int
		want     []string
	}{
		{maxDepth: 1, want: []string{"/one", "/start"}},
		{maxDepth: 2, want: []string{"/one", "/start", "/two"}},
		{maxDepth: 3, want: []string{"/one", "/start", "/three", "/two"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("max_depth=%d", tt.maxDepth), func(t *testing.T) {
			srv := newDepthTestServer(t, graph, nil)
			_, dbPath := runDepthCrawl(t, srv, tt.maxDepth, 0)

			got := crawledPaths(t, dbPath, srv.URL)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("crawled %v, want %v", got, tt.want)
			}
		})
	}
}

// max_depth 0 leaves the crawl unbounded and records no depths at all: an
// asynchronous run cannot promise a shortest path, so it stores nothing rather
// than a number that only looks authoritative.
func TestMaxDepthZeroCrawlsEverythingAndRecordsNoDepth(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/one"},
		"/one":   {"/two"},
		"/two":   {"/three"},
		"/three": {},
	}

	srv := newDepthTestServer(t, graph, nil)
	_, dbPath := runDepthCrawl(t, srv, 0, 0)

	got := crawledPaths(t, dbPath, srv.URL)
	want := []string{"/one", "/start", "/three", "/two"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("crawled %v, want the whole chain %v", got, want)
	}

	for _, path := range want {
		if _, ok := depthOf(t, dbPath, srv.URL, path); ok {
			t.Errorf("%s recorded a depth in an unbounded run; it must stay NULL", path)
		}
	}
}

// The shortcut case the layer barrier exists for: /deep is reachable in three
// hops down one branch and in one hop directly. It must be recorded — and
// bounded — by the short path, whichever branch the crawler happens to walk
// first.
func TestMaxDepthUsesShortestPath(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/a", "/deep"},
		"/a":     {"/b"},
		"/b":     {"/deep"},
		"/deep":  {"/leaf"},
		"/leaf":  {},
	}

	srv := newDepthTestServer(t, graph, nil)
	_, dbPath := runDepthCrawl(t, srv, 1, 0)

	got := crawledPaths(t, dbPath, srv.URL)
	want := []string{"/a", "/deep", "/start"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("crawled %v, want %v (/deep is one hop from the seed)", got, want)
	}

	if depth, ok := depthOf(t, dbPath, srv.URL, "/deep"); !ok || depth != 1 {
		t.Errorf("/deep depth = %d (recorded=%v), want 1", depth, ok)
	}
}

// Beyond the bound, links are still recorded for the link graph — the bound
// limits what is fetched, not what is known.
func TestMaxDepthKeepsFrontierInLinkGraph(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/one"},
		"/one":   {"/two"},
		"/two":   {},
	}

	srv := newDepthTestServer(t, graph, nil)
	store, dbPath := runDepthCrawl(t, srv, 1, 0)

	status, exists := store.GetURLStatus(srv.URL + "/two")
	if !exists {
		t.Fatal("/two should exist as a link-graph node beyond the bound")
	}
	if status != "discovered" {
		t.Errorf("/two status = %q, want discovered (recorded but not crawled)", status)
	}
	if _, ok := depthOf(t, dbPath, srv.URL, "/two"); ok {
		t.Error("/two was never queued, so its depth must stay NULL")
	}
}

// Layer order: every page at one depth is requested before any page at the
// next. This is the property that makes the recorded depth a shortest path.
func TestMaxDepthCrawlsLayerByLayer(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/a1", "/a2"},
		"/a1":    {"/b1"},
		"/a2":    {"/b2"},
		"/b1":    {},
		"/b2":    {},
	}

	srv := newDepthTestServer(t, graph, nil)
	_, _ = runDepthCrawl(t, srv, 2, 0)

	depthByPath := map[string]int{"/start": 0, "/a1": 1, "/a2": 1, "/b1": 2, "/b2": 2}
	seenAt := map[int]int{}
	for i, path := range srv.requestOrder() {
		d, ok := depthByPath[path]
		if !ok {
			continue // robots.txt and the like
		}
		seenAt[d] = i
		for deeper := d + 1; deeper <= 2; deeper++ {
			if first, ok := seenAt[deeper]; ok && first < i {
				t.Fatalf("depth %d was requested (position %d) before depth %d finished (position %d)", deeper, first, d, i)
			}
		}
	}
}

// A page that only succeeds on retry still has children, so its retries must be
// spent before the next layer opens — otherwise those children would be found
// after deeper pages had already been crawled.
func TestMaxDepthRetriesWithinLayerBeforeAdvancing(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/flaky"},
		"/flaky": {"/child"},
		"/child": {},
	}

	srv := newDepthTestServer(t, graph, map[string]int{"/flaky": 1})
	_, dbPath := runDepthCrawl(t, srv, 2, 0)

	got := crawledPaths(t, dbPath, srv.URL)
	want := []string{"/child", "/flaky", "/start"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("crawled %v, want %v (the retry must still yield its child)", got, want)
	}

	if depth, ok := depthOf(t, dbPath, srv.URL, "/child"); !ok || depth != 2 {
		t.Errorf("/child depth = %d (recorded=%v), want 2", depth, ok)
	}

	// The child must not have been requested before the retry succeeded.
	order := srv.requestOrder()
	var firstChild, lastFlaky = -1, -1
	for i, path := range order {
		if path == "/child" && firstChild < 0 {
			firstChild = i
		}
		if path == "/flaky" {
			lastFlaky = i
		}
	}
	if firstChild < 0 {
		t.Fatal("/child was never requested")
	}
	if firstChild < lastFlaky {
		t.Errorf("/child (position %d) was requested before /flaky finished retrying (position %d)", firstChild, lastFlaky)
	}
}

// Seeds are depth 0 and every one of them is queued, even when there are more
// seeds than the page limit allows to be crawled.
func TestMaxDepthQueuesEverySeedDespiteLimit(t *testing.T) {
	graph := map[string][]string{"/start": {}, "/s2": {}, "/s3": {}, "/s4": {}}
	srv := newDepthTestServer(t, graph, nil)

	store, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "seeds.db"))
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	seeds := []string{srv.URL + "/start", srv.URL + "/s2", srv.URL + "/s3", srv.URL + "/s4"}
	cfg := &config.CrawlConfig{
		SeedURLs:        seeds,
		MaxDepth:        1,
		Limit:           2, // fewer than the number of seeds
		Concurrency:     1,
		RequestDelay:    0.1,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Start(ctx, seeds); err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	// Every seed is in the database at depth 0, whether or not the limit let it
	// be crawled, and the pages the limit cut off are still queued for a resume.
	for _, seed := range seeds {
		status, exists := store.GetURLStatus(seed)
		if !exists {
			t.Errorf("seed %s was never queued; the limit must not drop seeds", seed)
			continue
		}
		if status == "discovered" {
			t.Errorf("seed %s is only a link-graph node, want it queued", seed)
		}
	}
}

// A database whose queue predates depth tracking cannot honour the bound, so a
// bounded run refuses to start rather than measuring from the wrong origin.
func TestMaxDepthRefusesQueueWithoutDepth(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	// An unbounded run's queue: pending, with no depth recorded.
	if err := store.AddToQueue([]string{"https://example.com/queued"}); err != nil {
		t.Fatalf("failed to seed queue: %v", err)
	}

	cfg := &config.CrawlConfig{
		SeedURLs:        []string{"https://example.com/queued"},
		MaxDepth:        1,
		Concurrency:     1,
		RequestDelay:    0.1,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	err = c.Start(context.Background(), cfg.SeedURLs)
	if err == nil {
		t.Fatal("expected a bounded run to refuse a queue without depth information")
	}
	if !strings.Contains(err.Error(), "--max-depth cannot be used with this database") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Link-graph nodes legitimately carry no depth, so their presence must not be
// mistaken for the legacy queue above.
func TestMaxDepthAcceptsDiscoveredRowsWithoutDepth(t *testing.T) {
	graph := map[string][]string{"/start": {"/one"}, "/one": {}}
	srv := newDepthTestServer(t, graph, nil)

	dbPath := filepath.Join(t.TempDir(), "discovered.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	// A link-graph node with no depth and no queue entry.
	if err := store.SaveLinks([]*crawler.LinkData{{
		SourceURL: srv.URL + "/start",
		TargetURL: srv.URL + "/elsewhere",
		LinkType:  "internal",
	}}); err != nil {
		t.Fatalf("failed to save link: %v", err)
	}

	cfg := &config.CrawlConfig{
		SeedURLs:        []string{srv.URL + "/start"},
		MaxDepth:        1,
		Concurrency:     1,
		RequestDelay:    0.1,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Start(ctx, cfg.SeedURLs); err != nil {
		t.Fatalf("a bounded run must accept link-graph nodes without depth: %v", err)
	}
}

// Promotion assigns the depth: a node first seen as a link, then reached by the
// crawl, ends up with the depth it was queued at rather than NULL.
func TestMaxDepthAssignsDepthOnPromotion(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/one"},
		"/one":   {},
	}

	srv := newDepthTestServer(t, graph, nil)
	_, dbPath := runDepthCrawl(t, srv, 1, 0)

	if depth, ok := depthOf(t, dbPath, srv.URL, "/one"); !ok || depth != 1 {
		t.Errorf("/one depth = %d (recorded=%v), want 1 assigned at promotion", depth, ok)
	}
	if depth, ok := depthOf(t, dbPath, srv.URL, "/start"); !ok || depth != 0 {
		t.Errorf("/start depth = %d (recorded=%v), want 0", depth, ok)
	}
}

// The page limit stops a bounded crawl part-way and leaves the rest queued, so
// the run can be resumed rather than silently reported as a complete depth-N
// crawl.
//
// Concurrency is 1 here on purpose: the limit is checked before a worker claims
// its next page, so several workers can each be mid-page when the budget runs
// out and the finished count can exceed the limit by up to concurrency-1. That
// is existing behaviour, not something the depth bound changes.
func TestMaxDepthStopsAtLimitAndKeepsQueue(t *testing.T) {
	graph := map[string][]string{
		"/start": {"/a1", "/a2", "/a3"},
		"/a1":    {},
		"/a2":    {},
		"/a3":    {},
	}

	srv := newDepthTestServer(t, graph, nil)

	store, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "limit.db"))
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	seeds := []string{srv.URL + "/start"}
	cfg := &config.CrawlConfig{
		SeedURLs:        seeds,
		MaxDepth:        1,
		Limit:           2,
		Concurrency:     1,
		RequestDelay:    0.1,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Start(ctx, seeds); err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	pending, processing, completed, _, err := store.GetQueueStatus()
	if err != nil {
		t.Fatalf("queue status failed: %v", err)
	}
	if completed > cfg.Limit {
		t.Errorf("crawled %d pages, want at most the limit of %d", completed, cfg.Limit)
	}
	if pending+processing == 0 {
		t.Error("the pages the limit cut off must stay queued for a resume")
	}
}

// Resuming must finish a shallow layer's retries before opening a deeper one.
//
// A run that is cancelled — or stopped by the page limit — can leave a depth-0
// page in 'error' with retries to spare while a depth-1 page is already queued.
// Choosing the next layer by pending work alone would pick depth 1, crawl and
// expand it, and only then come back to the depth-0 failure, which is exactly
// the ordering the barrier exists to prevent.
func TestMaxDepthResumeFinishesShallowRetriesFirst(t *testing.T) {
	graph := map[string][]string{
		"/shallow": {},
		"/deep":    {},
	}
	srv := newDepthTestServer(t, graph, nil)

	dbPath := filepath.Join(t.TempDir(), "resume.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Rebuild the state an interrupted run leaves behind: depth 0 errored with
	// retries left, depth 1 already queued.
	shallow := srv.URL + "/shallow"
	deep := srv.URL + "/deep"
	if err := store.AddToQueueWithDepth([]string{shallow}, 0); err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	item, err := store.GetNextFromQueueAtDepth(0)
	if err != nil || item == nil {
		t.Fatalf("claim failed: (%v, %v)", item, err)
	}
	if err := store.SaveFailedAttempt(item.ID, nil, "network_error", "interrupted", time.Time{}); err != nil {
		t.Fatalf("failed to record error: %v", err)
	}
	if err := store.AddToQueueWithDepth([]string{deep}, 1); err != nil {
		t.Fatalf("queue failed: %v", err)
	}

	cfg := &config.CrawlConfig{
		MaxDepth:        1,
		Concurrency:     1,
		RequestDelay:    0.1,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// No seeds: this is a resume of the existing queue.
	if err := c.Start(ctx, nil); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	var firstShallow, firstDeep = -1, -1
	for i, path := range srv.requestOrder() {
		if path == "/shallow" && firstShallow < 0 {
			firstShallow = i
		}
		if path == "/deep" && firstDeep < 0 {
			firstDeep = i
		}
	}
	if firstShallow < 0 {
		t.Fatal("the depth-0 failure was never retried on resume")
	}
	if firstDeep < 0 {
		t.Fatal("the depth-1 page was never crawled")
	}
	if firstShallow > firstDeep {
		t.Errorf("depth 1 (position %d) was crawled before the depth-0 retry (position %d)", firstDeep, firstShallow)
	}

	got := crawledPaths(t, dbPath, srv.URL)
	if strings.Join(got, ",") != "/deep,/shallow" {
		t.Errorf("crawled %v, want both pages", got)
	}
}

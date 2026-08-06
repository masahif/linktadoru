package crawler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
)

// barrierFailStorage fails one of the queries that decide whether a layer is
// finished, and records every depth the crawler tried to open. A bounded crawl
// must stop on that failure: it cannot show that the layer is done, so opening
// the next one would hand back a depth that is not a shortest path.
type barrierFailStorage struct {
	unboundedOnlyStorage

	mu             sync.Mutex
	openedDepths   []int
	failRetryCheck bool
	requeueZero    bool

	pending map[int]int // depth -> pending count
}

func newBarrierFailStorage() *barrierFailStorage {
	return &barrierFailStorage{pending: map[int]int{0: 1, 1: 1}}
}

func (s *barrierFailStorage) AddToQueueWithDepth(urls []string, depth int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[depth] += len(urls)
	return nil
}

func (s *barrierFailStorage) MinUnfinishedDepth(_ int) (*int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range []int{0, 1, 2} {
		if s.pending[d] > 0 {
			depth := d
			s.openedDepths = append(s.openedDepths, depth)
			return &depth, nil
		}
	}
	return nil, nil
}

// The layer drains immediately: no item is ever handed out, so the workers
// finish at once and the run reaches the retry check, which is what this test
// is about.
func (s *barrierFailStorage) GetNextFromQueueAtDepth(depth int) (*URLItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[depth] = 0
	return nil, nil
}

func (s *barrierFailStorage) HasQueuedItemsAtDepth(depth int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[depth] > 0, nil
}

// EarliestRetryTimeAtDepth is the query retryLayer actually consults, so both
// failure modes are injected here: a query that errors, and one that reports
// due work which the requeue below then refuses to move.
func (s *barrierFailStorage) EarliestRetryTimeAtDepth(_, _ int) (*time.Time, error) {
	if s.failRetryCheck {
		return nil, errors.New("simulated database failure")
	}
	if s.requeueZero {
		// Due now, so no Retry-After wait stands between this and the requeue.
		due := time.Now()
		return &due, nil
	}
	return nil, nil
}

func (s *barrierFailStorage) RequeueErrorPagesAtDepth(_, _ int) (int, error) {
	// Disagrees with EarliestRetryTimeAtDepth on purpose.
	return 0, nil
}

func (s *barrierFailStorage) depthsOpened() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.openedDepths...)
}

// Unused parts of the interface.
func (s *barrierFailStorage) AddToQueue([]string) error           { return nil }
func (s *barrierFailStorage) GetNextFromQueue() (*URLItem, error) { return nil, nil }
func (s *barrierFailStorage) UpdatePageStatus(int, string) error  { return nil }
func (s *barrierFailStorage) SavePageResult(int, *PageData) error { return nil }
func (s *barrierFailStorage) SaveFailedAttempt(int, *PageData, string, string, time.Time) error {
	return nil
}
func (s *barrierFailStorage) SavePageSkipped(int, string, string) error { return nil }
func (s *barrierFailStorage) SaveLink(*LinkData) error                  { return nil }
func (s *barrierFailStorage) SaveLinks([]*LinkData) error               { return nil }
func (s *barrierFailStorage) SaveError(*CrawlError) error               { return nil }
func (s *barrierFailStorage) GetQueueStatus() (int, int, int, int, error) {
	return 0, 0, 0, 0, nil
}
func (s *barrierFailStorage) CleanupStaleProcessing(time.Duration) error { return nil }
func (s *barrierFailStorage) HasQueuedItems() (bool, error)              { return false, nil }
func (s *barrierFailStorage) GetRetryablePages(int) ([]URLItem, error)   { return nil, nil }
func (s *barrierFailStorage) RequeueErrorPages(int) (int, error)         { return 0, nil }
func (s *barrierFailStorage) GetMeta(string) (string, error)             { return "", nil }
func (s *barrierFailStorage) SetMeta(string, string) error               { return nil }
func (s *barrierFailStorage) GetURLStatus(string) (string, bool)         { return "", false }
func (s *barrierFailStorage) Close() error                               { return nil }

func boundedTestConfig() *config.CrawlConfig {
	return &config.CrawlConfig{
		SeedURLs:        []string{"https://example.com/"},
		MaxDepth:        2,
		Concurrency:     1,
		RequestDelay:    0.1,
		RequestTimeout:  time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}
}

// If the crawler cannot tell whether a layer still has retryable failures, it
// must abort instead of assuming there are none — assuming would open the next
// layer over an unfinished one and silently break the depth guarantee.
func TestBoundedCrawlAbortsWhenRetryCheckFails(t *testing.T) {
	store := newBarrierFailStorage()
	store.failRetryCheck = true

	c, err := NewCrawler(boundedTestConfig(), store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	err = c.Start(context.Background(), []string{"https://example.com/"})
	if err == nil {
		t.Fatal("expected the run to fail when the retry check cannot be answered")
	}
	if !strings.Contains(err.Error(), "failed to check retryable pages") {
		t.Errorf("unexpected error: %v", err)
	}

	if opened := store.depthsOpened(); len(opened) != 1 || opened[0] != 0 {
		t.Errorf("opened depths %v, want only depth 0 - the next layer must not be opened", opened)
	}
}

// The two retry queries disagreeing is an invariant violation, not something to
// loop or shrug off: one says work remains, the other moves nothing.
func TestBoundedCrawlAbortsWhenRequeueMovesNothing(t *testing.T) {
	store := newBarrierFailStorage()
	store.requeueZero = true

	c, err := NewCrawler(boundedTestConfig(), store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	err = c.Start(context.Background(), []string{"https://example.com/"})
	if err == nil {
		t.Fatal("expected the run to fail when a retryable layer requeues nothing")
	}
	if !strings.Contains(err.Error(), "refusing to continue") {
		t.Errorf("unexpected error: %v", err)
	}

	if opened := store.depthsOpened(); len(opened) != 1 || opened[0] != 0 {
		t.Errorf("opened depths %v, want only depth 0", opened)
	}
}

// cleanupFailStorage fails the stale-row reset that runs before anything else,
// and records whether the crawl went on to touch the queue.
type cleanupFailStorage struct {
	barrierFailStorage

	queued      int
	depthsAsked int
}

func (s *cleanupFailStorage) CleanupStaleProcessing(time.Duration) error {
	return errors.New("simulated database failure")
}

func (s *cleanupFailStorage) AddToQueueWithDepth(urls []string, _ int) error {
	s.queued += len(urls)
	return nil
}

func (s *cleanupFailStorage) MinUnfinishedDepth(_ int) (*int, error) {
	s.depthsAsked++
	return nil, nil
}

// A bounded crawl cannot establish the queue state its barrier reasons about if
// the stale-row reset fails, so it must stop before queueing or opening
// anything rather than proceed on an unknown state.
func TestBoundedCrawlAbortsWhenStaleCleanupFails(t *testing.T) {
	store := &cleanupFailStorage{barrierFailStorage: *newBarrierFailStorage()}

	c, err := NewCrawler(boundedTestConfig(), store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	err = c.Start(context.Background(), []string{"https://example.com/"})
	if err == nil {
		t.Fatal("expected a bounded run to stop when the stale-row reset fails")
	}
	if !strings.Contains(err.Error(), "failed to reset stale processing rows") {
		t.Errorf("unexpected error: %v", err)
	}
	if store.queued != 0 {
		t.Errorf("queued %d seeds, want none before the run was abandoned", store.queued)
	}
	if store.depthsAsked != 0 {
		t.Errorf("asked for a depth layer %d times, want none", store.depthsAsked)
	}
}

// The unbounded path keeps its existing tolerance: it logs and carries on,
// because it has no barrier resting on the queue state.
func TestUnboundedCrawlToleratesStaleCleanupFailure(t *testing.T) {
	store := &cleanupFailStorage{barrierFailStorage: *newBarrierFailStorage()}

	cfg := boundedTestConfig()
	cfg.MaxDepth = 0

	c, err := NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("failed to create crawler: %v", err)
	}

	if err := c.Start(context.Background(), []string{"https://example.com/"}); err != nil {
		t.Errorf("an unbounded run must survive a failed stale-row reset, got: %v", err)
	}
}

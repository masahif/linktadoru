package crawler

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
)

// MockStorage implements Storage interface for testing
type MockStorage struct {
	depthZero []string
}

type seedRecordingStorage struct {
	MockStorage
	queued []string
}

type startupFailureStorage struct {
	MockStorage
	cleanupErr error
	depthErr   error
	seedsAdded bool
	claimsMade bool
}

func (s *startupFailureStorage) CleanupStaleProcessing(time.Duration) error {
	return s.cleanupErr
}

func (s *startupFailureStorage) ValidateDepthTracking([]string) error {
	return s.depthErr
}

func (s *startupFailureStorage) AddSeeds([]string) error {
	s.seedsAdded = true
	return nil
}

func (s *startupFailureStorage) GetNextFromQueue() (*URLItem, error) {
	s.claimsMade = true
	return nil, nil
}

func (s *seedRecordingStorage) AddSeeds(urls []string) error {
	_ = s.MockStorage.AddSeeds(urls)
	s.queued = append(s.queued, urls...)
	return nil
}

func (m *MockStorage) SaveLink(link *LinkData) error {
	return nil
}

func (m *MockStorage) SaveLinks(links []*LinkData) error {
	return nil
}

func (m *MockStorage) SaveError(crawlError *CrawlError) error {
	return nil
}

func (m *MockStorage) Close() error {
	return nil
}

func (m *MockStorage) AddSeeds(urls []string) error {
	m.depthZero = append(m.depthZero, urls...)
	return nil
}

func (m *MockStorage) AddToQueue(urls []string, depth int) error {
	return nil
}

func (m *MockStorage) GetNextFromQueue() (*URLItem, error) {
	return nil, nil
}

func (m *MockStorage) UpdatePageStatus(id int, status string) error {
	return nil
}

func (m *MockStorage) SavePageResult(id int, page *PageData) error {
	return nil
}

func (m *MockStorage) SavePageResponseError(id int, page *PageData, errorType, errorMessage string) error {
	return nil
}

func (m *MockStorage) SavePageError(id int, errorType, errorMessage string) error {
	return nil
}

func (m *MockStorage) GetQueueStatus() (pending int, processing int, completed int, errors int, err error) {
	return 0, 0, 0, 0, nil
}

func (m *MockStorage) CleanupStaleProcessing(timeout time.Duration) error {
	return nil
}

func (m *MockStorage) GetMeta(key string) (string, error) {
	return "", nil
}

func (m *MockStorage) SetMeta(key, value string) error {
	return nil
}

func (m *MockStorage) HasQueuedItems() (bool, error) {
	return false, nil
}

func (m *MockStorage) HasResumableWork() (bool, error) {
	return false, nil
}

func (m *MockStorage) ValidateDepthTracking(seedURLs []string) error {
	return nil
}

func (m *MockStorage) GetDepthZeroURLs() ([]string, error) {
	return append([]string(nil), m.depthZero...), nil
}

func (m *MockStorage) SavePageSkipped(id int, reason, message string) error {
	return nil
}

func (m *MockStorage) RequeueErrorPages(maxRetries int) (int, error) {
	return 0, nil
}

func TestLimit(t *testing.T) {
	// Test that limit configuration is properly set
	config := &config.CrawlConfig{
		SeedURLs:        []string{"http://example.com"},
		Limit:           5,
		Concurrency:     1,
		RequestDelay:    0.01, // 10ms in seconds
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
	}

	// Create test storage using mock
	store := &MockStorage{}

	// Create crawler
	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Verify config is set correctly
	if crawler.config.Limit != 5 {
		t.Errorf("Expected limit to be 5, got %d", crawler.config.Limit)
	}

	t.Logf("Limit configuration test passed: limit=%d", crawler.config.Limit)
}

func TestLimitDoesNotDiscardExplicitSeeds(t *testing.T) {
	seeds := []string{
		"https://a.example",
		"https://b.example",
		"https://c.example",
	}
	cfg := config.DefaultConfig()
	cfg.SeedURLs = seeds
	cfg.Limit = 1
	cfg.Concurrency = 1
	cfg.IgnoreRobotsTxt = true

	store := &seedRecordingStorage{}
	c, err := NewCrawler(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop() })

	if err := c.Start(context.Background(), seeds); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.queued, seeds) {
		t.Fatalf("queued seeds = %q, want all explicit seeds %q", store.queued, seeds)
	}
}

func TestStartupValidationFailsBeforeSeedMutationOrWorkers(t *testing.T) {
	tests := []struct {
		name       string
		cleanupErr error
		depthErr   error
	}{
		{"cleanup", errors.New("cleanup failed"), nil},
		{"legacy depth", nil, errors.New("unknown depth")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &startupFailureStorage{cleanupErr: tt.cleanupErr, depthErr: tt.depthErr}
			cfg := config.DefaultConfig()
			cfg.SeedURLs = []string{"https://example.com/"}
			cfg.IgnoreRobotsTxt = true
			crawler, err := NewCrawler(cfg, store)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = crawler.Stop() }()
			if err := crawler.Start(context.Background(), cfg.SeedURLs); err == nil {
				t.Fatal("startup failure was ignored")
			}
			if store.seedsAdded || store.claimsMade {
				t.Fatalf("startup continued: seedsAdded=%v claimsMade=%v", store.seedsAdded, store.claimsMade)
			}
		})
	}
}

func TestLimitLogic(t *testing.T) {
	// Test the limit logic in worker function
	tests := []struct {
		name         string
		limit        int
		pagesCrawled int
		shouldStop   bool
	}{
		{"limit_disabled", 0, 10, false},
		{"under_limit", 10, 5, false},
		{"at_limit", 10, 10, true},
		{"over_limit", 10, 15, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic that would be used in worker
			shouldStop := tt.limit > 0 && tt.pagesCrawled >= tt.limit

			if shouldStop != tt.shouldStop {
				t.Errorf("Expected shouldStop=%v, got %v for limit=%d, pagesCrawled=%d",
					tt.shouldStop, shouldStop, tt.limit, tt.pagesCrawled)
			}

			t.Logf("Test %s passed: limit=%d, pagesCrawled=%d, shouldStop=%v",
				tt.name, tt.limit, tt.pagesCrawled, shouldStop)
		})
	}
}

func TestGetStats(t *testing.T) {
	// Test the GetStats method
	config := &config.CrawlConfig{
		SeedURLs:        []string{"http://example.com"},
		Limit:           0,
		Concurrency:     1,
		RequestDelay:    0.01, // 10ms in seconds
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
	}

	store := &MockStorage{}
	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Test initial stats
	stats := crawler.GetStats()
	if stats.PagesCrawled != 0 {
		t.Errorf("Expected PagesCrawled to be 0, got %d", stats.PagesCrawled)
	}
	if stats.ErrorCount != 0 {
		t.Errorf("Expected Errors to be 0, got %d", stats.ErrorCount)
	}

	// Test incrementing counters
	crawler.incrementCrawledCount()
	crawler.incrementErrorCount()

	stats = crawler.GetStats()
	if stats.PagesCrawled != 1 {
		t.Errorf("Expected PagesCrawled to be 1, got %d", stats.PagesCrawled)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("Expected Errors to be 1, got %d", stats.ErrorCount)
	}

	// Test multiple increments
	crawler.incrementCrawledCount()
	crawler.incrementCrawledCount()
	crawler.incrementErrorCount()

	stats = crawler.GetStats()
	if stats.PagesCrawled != 3 {
		t.Errorf("Expected PagesCrawled to be 3, got %d", stats.PagesCrawled)
	}
	if stats.ErrorCount != 2 {
		t.Errorf("Expected Errors to be 2, got %d", stats.ErrorCount)
	}
}

func TestIncrementCounters(t *testing.T) {
	// Test individual counter increment functions
	config := &config.CrawlConfig{
		SeedURLs:        []string{"http://example.com"},
		Limit:           0,
		Concurrency:     1,
		RequestDelay:    0.01, // 10ms in seconds
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
	}

	store := &MockStorage{}
	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Test incrementCrawledCount
	initialCount := crawler.GetStats().PagesCrawled
	crawler.incrementCrawledCount()
	newCount := crawler.GetStats().PagesCrawled
	if newCount != initialCount+1 {
		t.Errorf("Expected PagesCrawled to increase by 1, got %d", newCount-initialCount)
	}

	// Test incrementErrorCount
	initialErrors := crawler.GetStats().ErrorCount
	crawler.incrementErrorCount()
	newErrors := crawler.GetStats().ErrorCount
	if newErrors != initialErrors+1 {
		t.Errorf("Expected Errors to increase by 1, got %d", newErrors-initialErrors)
	}
}

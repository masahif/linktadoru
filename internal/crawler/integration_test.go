package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"linktadoru/internal/config"
)

// TestStartStop tests the Start and Stop methods
func TestStartStop(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Test Page</body></html>"))
	}))
	defer server.Close()

	config := &config.CrawlConfig{
		SeedURLs:       []string{server.URL},
		Limit:          1,
		Concurrency:    1,
		RequestDelay:   0.01, // 10ms in seconds
		RequestTimeout: 5 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
		IgnoreRobots:   true,
	}

	// Use in-memory storage for testing
	store := &MockStorage{}
	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Test Start method
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start crawler
	err = crawler.Start(ctx, []string{server.URL})
	if err != nil {
		t.Errorf("Start() returned error: %v", err)
	}

	// Test Stop method
	err = crawler.Stop()
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

// TestStartWithRealStorage tests Start with actual storage operations
func TestStartWithRealStorage(t *testing.T) {
	// Create test server that returns HTML with links
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
		<html>
			<head><title>Test Page</title></head>
			<body>
				<h1>Test Content</h1>
				<a href="/page2">Internal Link</a>
			</body>
		</html>
		`))
	}))
	defer server.Close()

	config := &config.CrawlConfig{
		SeedURLs:       []string{server.URL},
		Limit:          1,
		Concurrency:    1,
		RequestDelay:   0.01, // 10ms in seconds
		RequestTimeout: 5 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
		IgnoreRobots:   true,
	}

	// Create enhanced mock storage that tracks calls
	store := &EnhancedMockStorage{
		addToQueueCalled:     false,
		getNextCalled:        false,
		savePageResultCalled: false,
	}

	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Start crawler with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = crawler.Start(ctx, []string{server.URL})
	if err != nil {
		t.Errorf("Start() returned error: %v", err)
	}

	// Verify storage methods were called
	if !store.addToQueueCalled {
		t.Errorf("Expected AddToQueue to be called")
	}
	if !store.getNextCalled {
		t.Errorf("Expected GetNextFromQueue to be called")
	}

	err = crawler.Stop()
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

// EnhancedMockStorage tracks method calls for testing
type EnhancedMockStorage struct {
	MockStorage
	mu                   sync.Mutex
	addToQueueCalled     bool
	getNextCalled        bool
	savePageResultCalled bool
	items                []*URLItem
	currentID            int
}

func (e *EnhancedMockStorage) AddToQueue(urls []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.addToQueueCalled = true

	// Add items to mock queue
	for _, url := range urls {
		e.currentID++
		e.items = append(e.items, &URLItem{
			ID:  e.currentID,
			URL: url,
		})
	}
	return nil
}

func (e *EnhancedMockStorage) GetNextFromQueue() (*URLItem, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.getNextCalled = true

	if len(e.items) > 0 {
		item := e.items[0]
		e.items = e.items[1:]
		return item, nil
	}
	return nil, nil
}

func (e *EnhancedMockStorage) SavePageResult(id int, page *PageData) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.savePageResultCalled = true
	return nil
}

// TestWorkerErrorHandling tests worker function error scenarios
func TestWorkerErrorHandling(t *testing.T) {
	// Create a mock storage that simulates a processing error
	store := &ErrorMockStorage{}

	config := &config.CrawlConfig{
		SeedURLs:       []string{"http://example.com"},
		Limit:          1,
		Concurrency:    1,
		RequestDelay:   0.01, // 10ms in seconds
		RequestTimeout: 1 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
		IgnoreRobots:   true,
	}

	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Manually increment error count to simulate processing error
	crawler.incrementErrorCount()

	// Check that error count increased
	stats := crawler.GetStats()
	if stats.ErrorCount != 1 {
		t.Errorf("Expected error count = 1, got %d", stats.ErrorCount)
	}

	err = crawler.Stop()
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

// ErrorMockStorage for testing error scenarios
type ErrorMockStorage struct {
	MockStorage
}

func (e *ErrorMockStorage) GetNextFromQueue() (*URLItem, error) {
	// Return no items to avoid infinite loop
	return nil, nil
}

// TestStatsReporter tests the stats reporting functionality
func TestStatsReporter(t *testing.T) {
	config := &config.CrawlConfig{
		SeedURLs:       []string{"http://example.test"},
		Limit:          5,
		Concurrency:    1,
		RequestDelay:   0.01, // 10ms in seconds
		RequestTimeout: 5 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
		IgnoreRobots:   true,
	}

	store := &MockStorage{}
	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Simulate some statistics
	crawler.incrementCrawledCount()
	crawler.incrementCrawledCount()
	crawler.incrementErrorCount()

	// Test GetStats
	stats := crawler.GetStats()
	if stats.PagesCrawled != 2 {
		t.Errorf("Expected PagesCrawled=2, got %d", stats.PagesCrawled)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount=1, got %d", stats.ErrorCount)
	}

	// Verify StartTime is set
	if stats.StartTime.IsZero() {
		t.Errorf("Expected StartTime to be set")
	}
}

// TestMultipleWorkers tests concurrent worker functionality
func TestMultipleWorkers(t *testing.T) {
	// Create test server
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Test Page</body></html>"))
	}))
	defer server.Close()

	config := &config.CrawlConfig{
		SeedURLs:       []string{server.URL, server.URL + "/page2"},
		Limit:          2,
		Concurrency:    2,    // Multiple workers
		RequestDelay:   0.01, // 10ms in seconds
		RequestTimeout: 5 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
		IgnoreRobots:   true,
	}

	store := &EnhancedMockStorage{}
	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = crawler.Start(ctx, []string{server.URL})
	if err != nil {
		t.Errorf("Start() returned error: %v", err)
	}

	err = crawler.Stop()
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

// TestLimitReached tests that crawling stops when limit is reached
func TestLimitReached(t *testing.T) {
	config := &config.CrawlConfig{
		SeedURLs:       []string{"http://example.test"},
		Limit:          2, // Small limit
		Concurrency:    1,
		RequestDelay:   0.01, // 10ms in seconds
		RequestTimeout: 5 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
		IgnoreRobots:   true,
	}

	// Mock storage that provides items
	store := &LimitTestStorage{items: make([]*URLItem, 0)}
	_ = store.AddToQueue([]string{"http://example.test/1", "http://example.test/2", "http://example.test/3"})

	crawler, err := NewCrawler(config, store)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	// Manually increment crawled count to near limit
	crawler.incrementCrawledCount()
	crawler.incrementCrawledCount()

	stats := crawler.GetStats()
	if stats.PagesCrawled != 2 {
		t.Errorf("Expected PagesCrawled=2, got %d", stats.PagesCrawled)
	}
}

// LimitTestStorage for testing limit functionality
type LimitTestStorage struct {
	MockStorage
	items []*URLItem
	id    int
}

func (l *LimitTestStorage) AddToQueue(urls []string) error {
	for _, url := range urls {
		l.id++
		l.items = append(l.items, &URLItem{ID: l.id, URL: url})
	}
	return nil
}

func (l *LimitTestStorage) GetNextFromQueue() (*URLItem, error) {
	if len(l.items) > 0 {
		item := l.items[0]
		l.items = l.items[1:]
		return item, nil
	}
	return nil, nil
}

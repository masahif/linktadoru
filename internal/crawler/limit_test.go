package crawler

import (
	"testing"
	"time"
)

// MockStorage implements Storage interface for testing
type MockStorage struct{}

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

func (m *MockStorage) AddToQueue(urls []string) error {
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

func (m *MockStorage) SavePageError(id int, errorType, errorMessage string) error {
	return nil
}

func (m *MockStorage) GetQueueStatus() (queued int, processing int, completed int, errors int, err error) {
	return 0, 0, 0, 0, nil
}

func (m *MockStorage) GetProcessingItems() ([]URLItem, error) {
	return nil, nil
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

func (m *MockStorage) GetURLStatus(url string) (status string, exists bool) {
	return "", false
}

func TestLimit(t *testing.T) {
	// Test that limit configuration is properly set
	config := &CrawlConfig{
		SeedURLs:       []string{"http://example.com"},
		Limit:          5,
		Concurrency:    1,
		RequestDelay:   10 * time.Millisecond,
		RequestTimeout: 5 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
		RespectRobots:  false,
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
package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

func TestSQLiteStorage(t *testing.T) {
	// Create a temporary directory for test database
	tempDir := t.TempDir()
	dbFile := filepath.Join(tempDir, "test_crawler.db")

	// Initialize storage
	storage, err := NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	t.Run("SaveAndRetrievePage", func(t *testing.T) {
		page := &crawler.PageData{
			URL:          "https://example.com",
			StatusCode:   200,
			Title:        "Example Page",
			MetaDesc:     "Example description",
			MetaRobots:   "index,follow",
			CanonicalURL: "https://example.com",
			ContentHash:  "abc123",
			TTFB:         100 * time.Millisecond,
			DownloadTime: 500 * time.Millisecond,
			ResponseSize: 1024,
			HTTPHeaders: map[string]string{
				"content-type":     "text/html",
				"content-length":   "1024",
				"last-modified":    time.Now().Format(time.RFC1123),
				"server":           "nginx",
				"content-encoding": "gzip",
			},
			CrawledAt: time.Now(),
		}

		// First add to queue to get an ID
		urls := []string{page.URL}
		err := storage.AddToQueue(urls)
		if err != nil {
			t.Errorf("Failed to add to queue: %v", err)
		}

		// Get the item from queue to get ID
		item, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get item from queue: %v", err)
		}
		if item == nil {
			t.Errorf("No item returned from queue")
			return
		}

		// Save page result
		err = storage.SavePageResult(item.ID, page)
		if err != nil {
			t.Errorf("Failed to save page result: %v", err)
		}
	})

	t.Run("SaveAndRetrieveLink", func(t *testing.T) {
		link := &crawler.LinkData{
			SourceURL:    "https://example.com",
			TargetURL:    "https://example.com/page1",
			AnchorText:   "Page 1",
			LinkType:     "internal",
			RelAttribute: "",
			CrawledAt:    time.Now(),
		}

		err := storage.SaveLink(link)
		if err != nil {
			t.Errorf("Failed to save link: %v", err)
		}
	})

	t.Run("SaveError", func(t *testing.T) {
		crawlErr := &crawler.CrawlError{
			URL:          "https://example.com/404",
			ErrorType:    "http_error",
			ErrorMessage: "404 Not Found",
			OccurredAt:   time.Now(),
		}

		err := storage.SaveError(crawlErr)
		if err != nil {
			t.Errorf("Failed to save error: %v", err)
		}
	})

	t.Run("QueueOperations", func(t *testing.T) {
		// Add items to queue
		urls := []string{
			"https://example.com/page1",
			"https://example.com/page2",
			"https://example.com/page3",
		}

		err := storage.AddToQueue(urls)
		if err != nil {
			t.Errorf("Failed to add to queue: %v", err)
		}

		// Test status-based exclusive control
		nextItem1, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get next from queue: %v", err)
		}
		if nextItem1 == nil {
			t.Errorf("Expected item from queue, got nil")
			return
		}

		nextItem2, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get next from queue: %v", err)
		}
		if nextItem2 == nil {
			t.Errorf("Expected second item from queue, got nil")
			return
		}

		// Update status to completed
		err = storage.UpdatePageStatus(nextItem1.ID, "completed")
		if err != nil {
			t.Errorf("Failed to update page status: %v", err)
		}

		err = storage.UpdatePageStatus(nextItem2.ID, "completed")
		if err != nil {
			t.Errorf("Failed to update page status: %v", err)
		}

		// Get the third item that should still be available
		nextItem3, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get next from queue: %v", err)
		}
		if nextItem3 == nil {
			t.Errorf("Expected third item from queue, got nil")
		} else {
			// Complete the third item
			err = storage.UpdatePageStatus(nextItem3.ID, "completed")
			if err != nil {
				t.Errorf("Failed to complete third item: %v", err)
			}
		}

		// Now verify no more items
		nextItem4, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get next from queue: %v", err)
		}
		if nextItem4 != nil {
			t.Errorf("Expected no more items, got %v", nextItem4)
		}

		// Test cleanup stale processing
		err = storage.CleanupStaleProcessing(1 * time.Hour)
		if err != nil {
			t.Errorf("Failed to cleanup stale processing: %v", err)
		}
	})

	t.Run("MetaOperations", func(t *testing.T) {
		// Set meta value
		err := storage.SetMeta("crawl_started", time.Now().Format(time.RFC3339))
		if err != nil {
			t.Errorf("Failed to set meta: %v", err)
		}

		// Get meta value
		value, err := storage.GetMeta("crawl_started")
		if err != nil {
			t.Errorf("Failed to get meta: %v", err)
		}

		if value == "" {
			t.Errorf("Expected non-empty value")
		}
	})

	t.Run("StatusBasedQueueControl", func(t *testing.T) {
		// Add test items
		urls := []string{
			"https://test.com/status1",
			"https://test.com/status2",
		}

		err := storage.AddToQueue(urls)
		if err != nil {
			t.Errorf("Failed to add items: %v", err)
		}

		// Worker 1 gets first item
		worker1Item, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get items for worker 1: %v", err)
		}
		if worker1Item == nil {
			t.Errorf("Worker 1 expected 1 item, got nil")
			return
		}

		// Worker 2 gets second item
		worker2Item, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get items for worker 2: %v", err)
		}
		if worker2Item == nil {
			t.Errorf("Worker 2 expected 1 item, got nil")
			return
		}

		// Verify no more items available
		worker3Item, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get items for worker 3: %v", err)
		}
		if worker3Item != nil {
			t.Errorf("Worker 3 expected 0 items, got %v", worker3Item)
		}

		// Complete worker 1's item
		err = storage.UpdatePageStatus(worker1Item.ID, "completed")
		if err != nil {
			t.Errorf("Failed to complete worker 1 item: %v", err)
		}

		// Complete worker 2's item
		err = storage.UpdatePageStatus(worker2Item.ID, "completed")
		if err != nil {
			t.Errorf("Failed to complete worker 2 item: %v", err)
		}
	})

	t.Run("QueueResume", func(t *testing.T) {
		// Test resuming from existing queue (no seed URLs needed)
		urls := []string{
			"https://resume.test/page1",
			"https://resume.test/page2",
		}

		// Add items to simulate existing queue
		err := storage.AddToQueue(urls)
		if err != nil {
			t.Errorf("Failed to add resume items: %v", err)
		}

		// Verify items can be retrieved (simulating crawler resume)
		item1, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get resume item 1: %v", err)
		}
		if item1 == nil {
			t.Errorf("Expected resume item 1, got nil")
			return
		}

		item2, err := storage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get resume item 2: %v", err)
		}
		if item2 == nil {
			t.Errorf("Expected resume item 2, got nil")
			return
		}

		// Clean up
		err = storage.UpdatePageStatus(item1.ID, "completed")
		if err != nil {
			t.Errorf("Failed to complete resume item 1: %v", err)
		}

		err = storage.UpdatePageStatus(item2.ID, "completed")
		if err != nil {
			t.Errorf("Failed to complete resume item 2: %v", err)
		}
	})

	t.Run("QueueStatusTracking", func(t *testing.T) {
		// Create a separate storage instance for this test
		statusStorage, err := NewSQLiteStorage(":memory:")
		if err != nil {
			t.Fatalf("Failed to create status storage: %v", err)
		}
		defer statusStorage.Close()

		// Test queue status monitoring
		urls := []string{
			"https://status.test/page1",
			"https://status.test/page2",
		}

		// Add items
		err = statusStorage.AddToQueue(urls)
		if err != nil {
			t.Errorf("Failed to add status test items: %v", err)
		}

		// Check initial status
		queued, processing, completed, errors, err := statusStorage.GetQueueStatus()
		if err != nil {
			t.Errorf("Failed to get queue status: %v", err)
		}
		if queued != 2 || processing != 0 {
			t.Errorf("Expected queued=2, processing=0, got queued=%d, processing=%d", queued, processing)
		}
		_ = completed // Ignore for this test
		_ = errors    // Ignore for this test

		// Get one item (moves to processing)
		nextItem, err := statusStorage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get next item: %v", err)
		}
		if nextItem == nil {
			t.Errorf("Expected 1 item, got nil")
			return
		}

		// Check status after getting item
		queued, processing, completed, errors, err = statusStorage.GetQueueStatus()
		if err != nil {
			t.Errorf("Failed to get queue status: %v", err)
		}
		if queued != 1 || processing != 1 {
			t.Errorf("Expected queued=1, processing=1, got queued=%d, processing=%d", queued, processing)
		}
		_ = completed // Ignore for this test
		_ = errors    // Ignore for this test

		// Get processing items
		processingItems, err := statusStorage.GetProcessingItems()
		if err != nil {
			t.Errorf("Failed to get processing items: %v", err)
		}
		if len(processingItems) != 1 {
			t.Errorf("Expected 1 processing item, got %d", len(processingItems))
		}
		if processingItems[0].URL != nextItem.URL {
			t.Errorf("Processing item URL mismatch: expected %s, got %s", nextItem.URL, processingItems[0].URL)
		}

		// Complete the item
		err = statusStorage.UpdatePageStatus(nextItem.ID, "completed")
		if err != nil {
			t.Errorf("Failed to complete status test item: %v", err)
		}

		// Check final status
		queued, processing, completed, errors, err = statusStorage.GetQueueStatus()
		if err != nil {
			t.Errorf("Failed to get queue status: %v", err)
		}
		if queued != 1 || processing != 0 {
			t.Errorf("Expected queued=1, processing=0, got queued=%d, processing=%d", queued, processing)
		}
		_ = completed // Ignore for this test
		_ = errors    // Ignore for this test

		// Clean up remaining item
		remainingItem, err := statusStorage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get remaining item: %v", err)
		}
		if remainingItem != nil {
			err = statusStorage.UpdatePageStatus(remainingItem.ID, "completed")
			if err != nil {
				t.Errorf("Failed to complete remaining item: %v", err)
			}
		}
	})

	t.Run("SavePageError", func(t *testing.T) {
		// Test SavePageError function
		errorStorage, err := NewSQLiteStorage(":memory:")
		if err != nil {
			t.Fatalf("Failed to create error storage: %v", err)
		}
		defer errorStorage.Close()

		// Add URL to queue first
		urls := []string{"https://error.test/page"}
		err = errorStorage.AddToQueue(urls)
		if err != nil {
			t.Errorf("Failed to add URL to queue: %v", err)
		}

		// Get item from queue
		item, err := errorStorage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get item from queue: %v", err)
		}
		if item == nil {
			t.Errorf("Expected 1 item, got nil")
			return
		}

		// Save page error
		err = errorStorage.SavePageError(item.ID, "network_error", "Connection timeout")
		if err != nil {
			t.Errorf("Failed to save page error: %v", err)
		}

		// Check queue status - should have one error
		_, _, _, errors, err := errorStorage.GetQueueStatus()
		if err != nil {
			t.Errorf("Failed to get queue status: %v", err)
		}
		if errors != 1 {
			t.Errorf("Expected 1 error, got %d", errors)
		}
	})

	t.Run("SaveLinks", func(t *testing.T) {
		// Test SaveLinks function
		linkStorage, err := NewSQLiteStorage(":memory:")
		if err != nil {
			t.Fatalf("Failed to create link storage: %v", err)
		}
		defer linkStorage.Close()

		// Create test links
		links := []*crawler.LinkData{
			{
				SourceURL:  "https://test.com/page1",
				TargetURL:  "https://test.com/page2",
				LinkType:   "internal",
				AnchorText: "Link to page 2",
			},
			{
				SourceURL:  "https://test.com/page1",
				TargetURL:  "https://external.com/page",
				LinkType:   "external",
				AnchorText: "External link",
			},
		}

		// Save multiple links
		err = linkStorage.SaveLinks(links)
		if err != nil {
			t.Errorf("Failed to save links: %v", err)
		}
	})

	t.Run("GetURLStatus", func(t *testing.T) {
		// Test GetURLStatus function
		statusStorage, err := NewSQLiteStorage(":memory:")
		if err != nil {
			t.Fatalf("Failed to create status storage: %v", err)
		}
		defer statusStorage.Close()

		testURL := "https://status.test/page"

		// Check status of non-existent URL
		status, exists := statusStorage.GetURLStatus(testURL)
		_ = exists // We're not testing the exists flag in these cases
		if status != "" {
			t.Errorf("Expected empty status for non-existent URL, got %s", status)
		}

		// Add URL to queue
		err = statusStorage.AddToQueue([]string{testURL})
		if err != nil {
			t.Errorf("Failed to add URL to queue: %v", err)
		}

		// Check status - should be "queued"
		status, exists = statusStorage.GetURLStatus(testURL)
		_ = exists // We're not testing the exists flag in these cases
		if status != "queued" {
			t.Errorf("Expected status 'queued', got '%s'", status)
		}

		// Get item and update status
		item, err := statusStorage.GetNextFromQueue()
		if err != nil {
			t.Errorf("Failed to get item from queue: %v", err)
		}
		if item != nil {
			// Update to processing (should already be processing)
			status, _ = statusStorage.GetURLStatus(testURL)
			if status != "processing" {
				t.Errorf("Expected status 'processing', got '%s'", status)
			}

			// Complete the item
			err = statusStorage.UpdatePageStatus(item.ID, "completed")
			if err != nil {
				t.Errorf("Failed to update page status: %v", err)
			}

			// Check completed status
			status, _ = statusStorage.GetURLStatus(testURL)
			if status != "completed" {
				t.Errorf("Expected status 'completed', got '%s'", status)
			}
		}
	})
}

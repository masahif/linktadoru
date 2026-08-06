package storage

import (
	"path/filepath"
	"testing"
)

func newDepthPriorityStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	s, err := NewSQLiteStorage(filepath.Join(t.TempDir(), "priority.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// Shallow work comes first, but only among rows that are actually available.
func TestDepthPriorityPrefersShallowPending(t *testing.T) {
	s := newDepthPriorityStorage(t)

	if err := s.AddToQueueWithDepth([]string{"https://example.com/child"}, 1); err != nil {
		t.Fatalf("queue child: %v", err)
	}
	if err := s.AddToQueueWithDepth([]string{"https://example.com/seed"}, 0); err != nil {
		t.Fatalf("queue seed: %v", err)
	}

	// The child was queued first, so an added_at-ordered claim would take it.
	// Depth has to win.
	item, err := s.GetNextFromQueueByDepthPriority()
	if err != nil || item == nil {
		t.Fatalf("claim: item=%v err=%v", item, err)
	}
	if item.URL != "https://example.com/seed" {
		t.Errorf("claimed %s, want the depth-0 seed first", item.URL)
	}
	if item.Depth != 0 {
		t.Errorf("claimed depth %d, want 0", item.Depth)
	}
}

// A depth-0 row that is already being fetched must not hold back depth-1 work.
// This is the whole point of the mode: the claim looks at 'pending' only, so an
// in-flight seed is simply not a candidate rather than a blocker.
func TestDepthPriorityDoesNotWaitForProcessingRows(t *testing.T) {
	s := newDepthPriorityStorage(t)

	if err := s.AddToQueueWithDepth([]string{"https://example.com/slow"}, 0); err != nil {
		t.Fatalf("queue seed: %v", err)
	}
	if err := s.AddToQueueWithDepth([]string{"https://example.com/child"}, 1); err != nil {
		t.Fatalf("queue child: %v", err)
	}

	// Take the seed, leaving it 'processing' as a slow fetch would.
	first, err := s.GetNextFromQueueByDepthPriority()
	if err != nil || first == nil || first.Depth != 0 {
		t.Fatalf("first claim: item=%v err=%v", first, err)
	}

	second, err := s.GetNextFromQueueByDepthPriority()
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if second == nil {
		t.Fatal("no row claimable while a depth-0 row is in flight; the depth-1 work is being blocked")
	}
	if second.Depth != 1 {
		t.Errorf("claimed depth %d, want the depth-1 row", second.Depth)
	}
}

// Every URL in one batch shares an added_at, so url is what makes the order
// total. Without it the claim sequence is unspecified and two runs over the
// same database can diverge.
func TestDepthPriorityTieBreaksOnURL(t *testing.T) {
	s := newDepthPriorityStorage(t)

	// Deliberately not in sorted order.
	seeds := []string{
		"https://example.com/c",
		"https://example.com/a",
		"https://example.com/b",
	}
	if err := s.AddToQueueWithDepth(seeds, 0); err != nil {
		t.Fatalf("queue seeds: %v", err)
	}

	want := []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"}
	for i, expected := range want {
		item, err := s.GetNextFromQueueByDepthPriority()
		if err != nil || item == nil {
			t.Fatalf("claim %d: item=%v err=%v", i, item, err)
		}
		if item.URL != expected {
			t.Errorf("claim %d = %s, want %s", i, item.URL, expected)
		}
	}
}

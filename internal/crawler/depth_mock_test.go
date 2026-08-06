package crawler

import "time"

// unboundedOnlyStorage supplies the depth-aware half of the Storage interface,
// along with the retry-pacing and run-reporting methods, for test doubles that
// do not exercise them. Embedding it keeps those doubles focused on what they
// actually assert; a bounded test that reached one of these would fail loudly
// rather than quietly do nothing.
type unboundedOnlyStorage struct{}

func (unboundedOnlyStorage) AddToQueueWithDepth(_ []string, _ int) error { return nil }

func (unboundedOnlyStorage) GetNextFromQueueAtDepth(_ int) (*URLItem, error) { return nil, nil }

func (unboundedOnlyStorage) GetNextFromQueueByDepthPriority() (*URLItem, error) { return nil, nil }

func (unboundedOnlyStorage) MinUnfinishedDepth(_ int) (*int, error) { return nil, nil }

func (unboundedOnlyStorage) HasQueuedItemsAtDepth(_ int) (bool, error) { return false, nil }

func (unboundedOnlyStorage) HasRetryablePagesAtDepth(_, _ int) (bool, error) { return false, nil }

func (unboundedOnlyStorage) RequeueErrorPagesAtDepth(_, _ int) (int, error) { return 0, nil }

func (unboundedOnlyStorage) EarliestRetryTimeAtDepth(_, _ int) (*time.Time, error) {
	return nil, nil
}

func (unboundedOnlyStorage) HasDepthlessWork(_ int) (bool, error) { return false, nil }

// nil means "nothing retryable", which ends the retry loop immediately — the
// right default for a double that is not testing retries.
func (unboundedOnlyStorage) EarliestRetryTime(_ int) (*time.Time, error) { return nil, nil }

func (unboundedOnlyStorage) GetRunSummary() (RunSummary, error) { return RunSummary{}, nil }

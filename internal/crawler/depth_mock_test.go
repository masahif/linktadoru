package crawler

// unboundedOnlyStorage supplies the depth-aware half of the Storage interface
// for test doubles that only exercise unbounded crawling. Embedding it keeps
// those doubles focused on what they actually assert; a bounded test that
// reached one of these would fail loudly rather than quietly do nothing.
type unboundedOnlyStorage struct{}

func (unboundedOnlyStorage) AddToQueueWithDepth(_ []string, _ int) error { return nil }

func (unboundedOnlyStorage) GetNextFromQueueAtDepth(_ int) (*URLItem, error) { return nil, nil }

func (unboundedOnlyStorage) MinUnfinishedDepth(_ int) (*int, error) { return nil, nil }

func (unboundedOnlyStorage) HasQueuedItemsAtDepth(_ int) (bool, error) { return false, nil }

func (unboundedOnlyStorage) HasRetryablePagesAtDepth(_, _ int) (bool, error) { return false, nil }

func (unboundedOnlyStorage) RequeueErrorPagesAtDepth(_, _ int) (int, error) { return 0, nil }

func (unboundedOnlyStorage) HasDepthlessWork(_ int) (bool, error) { return false, nil }

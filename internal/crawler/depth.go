package crawler

import (
	"fmt"
	"log/slog"
)

// MaxRetries bounds retry attempts per URL. A bounded crawl spends this budget
// inside each depth layer and an unbounded one spends it after the queue
// drains, but the accounting is the same and stays per URL
// (pages.retry_count).
//
// It is exported because the CLI has to agree with it: the check that decides
// whether a database still has work to resume asks the same question the
// crawler will, and a different number there would either skip work the
// crawler would have done or start a run with nothing to do.
const MaxRetries = 3

// bounded reports whether this run applies a depth bound. Everything
// depth-related is gated on it: an unbounded crawl keeps the asynchronous
// queue it has always had, and leaves pages.depth NULL, because a value
// recorded there would be the depth of first discovery rather than the
// shortest path.
func (c *DefaultCrawler) bounded() bool {
	return c.config.MaxDepth > 0
}

// strictLayers reports whether this run needs the depth-layer barrier, as
// distinct from merely tracking depth.
//
// The barrier exists because a page reached later by a shorter path can change
// whether its children fall inside the bound. At max_depth 1 there are no such
// children — depth-1 pages are never expanded — so the fetched set and the
// recorded depths are the same with or without it, and waiting only lets one
// slow seed idle every other worker. From max_depth 2 the argument stops
// holding.
func (c *DefaultCrawler) strictLayers() bool {
	return c.config.MaxDepth >= 2
}

// requireDepthTracking refuses to start a bounded crawl against a queue whose
// depths were never recorded.
//
// Treating those rows as depth 0 would silently move the origin of the bound:
// --max-depth 2 would mean "two hops from wherever the previous run stopped"
// rather than two hops from the seeds, and would crawl further than the
// operator asked for. Refusing is the honest answer, and the direction of the
// error matters — a bound that quietly under-reaches is a nuisance, one that
// quietly over-reaches is the thing the flag exists to prevent.
func (c *DefaultCrawler) requireDepthTracking() error {
	depthless, err := c.storage.HasDepthlessWork(MaxRetries)
	if err != nil {
		return fmt.Errorf("failed to check the queue for depth information: %w", err)
	}
	if !depthless {
		return nil
	}

	return fmt.Errorf("--max-depth cannot be used with this database: it still holds unfinished pages — queued, or failed with retries left — whose distance from the seeds was never recorded, " +
		"either from a run without --max-depth or from a release older than depth tracking. " +
		"Their depth is unknown, so the bound cannot be applied to them. Start with a new database file, or finish this work without --max-depth first")
}

// runWorkers runs one round of workers to completion. Callers must not have
// workers running: c.wg is reused across rounds, as the retry phase already
// does.
func (c *DefaultCrawler) runWorkers() {
	for i := 0; i < c.config.Concurrency; i++ {
		c.wg.Add(1)
		go c.worker(i)
	}
	c.wg.Wait()
}

// runBoundedCrawl walks the queue one depth at a time.
//
// A layer is opened only once every shallower layer is finished — including its
// retries — which is what makes the recorded depth a true shortest path rather
// than an artifact of crawl order. Without the barrier, a URL reached first
// down a long branch would be labelled with that branch's depth, its children
// would inherit the inflated value, and a later short path to the same URL
// could not undo it: the page is already expanded.
//
// Every decision that guards the barrier reports its failure instead of
// logging and carrying on. If the crawler cannot confirm that a layer is
// finished, it cannot open the next one without risking exactly the wrong
// answer the bound exists to prevent, so the run stops and says so.
func (c *DefaultCrawler) runBoundedCrawl() error {
	for {
		if c.ctx.Err() != nil {
			slog.Info("Crawling cancelled")
			return nil
		}
		if c.limitReached() {
			c.logLimitStop()
			return nil
		}

		// Includes layers whose only remaining work is a retryable failure, so
		// a resume picks up a shallow error left by a cancelled or
		// limit-stopped run before opening anything deeper.
		depth, err := c.storage.MinUnfinishedDepth(MaxRetries)
		if err != nil {
			return fmt.Errorf("failed to find the next depth layer: %w", err)
		}
		if depth == nil {
			slog.Info("Crawling completed - no unfinished pages remain")
			return nil
		}
		if *depth > c.config.MaxDepth {
			// Links past the bound are never queued, so this only happens when
			// resuming a queue that an earlier run built with a larger bound.
			slog.Info("Remaining queued pages are deeper than the limit - stopping",
				"next_depth", *depth, "max_depth", c.config.MaxDepth)
			return nil
		}

		c.layerDepth = *depth
		slog.Info("Crawling depth layer", "depth", *depth, "max_depth", c.config.MaxDepth)
		c.runWorkers()

		if err := c.retryLayer(*depth); err != nil {
			return err
		}
		if err := c.confirmLayerDrained(*depth); err != nil {
			return err
		}
	}
}

// retryLayer spends this layer's retry budget before the next layer opens.
//
// A page that only succeeds on its second attempt still has children, and those
// children belong to the very next layer. Retrying after the whole crawl (as an
// unbounded run does) would surface them once deeper layers had already been
// crawled and expanded, which breaks the shortest-path guarantee the bound is
// supposed to give.
func (c *DefaultCrawler) retryLayer(depth int) error {
	for {
		if c.ctx.Err() != nil || c.limitReached() {
			return nil
		}

		// One query drives the loop. Asking "is anything retryable?" and then
		// requeueing would let the two answers disagree whenever a row is
		// retryable but still waiting out its Retry-After — a correct state
		// that the guard below would otherwise read as a fault.
		due, err := c.storage.EarliestRetryTimeAtDepth(MaxRetries, depth)
		if err != nil {
			return fmt.Errorf("failed to check retryable pages at depth %d: %w", depth, err)
		}
		if due == nil {
			return nil
		}
		if !c.waitUntil(*due) {
			return nil // cancelled while waiting
		}

		requeued, err := c.storage.RequeueErrorPagesAtDepth(MaxRetries, depth)
		if err != nil {
			return fmt.Errorf("failed to requeue failed pages at depth %d: %w", depth, err)
		}
		if requeued == 0 {
			// The rows were due, so the two queries genuinely disagree: one says
			// there is a retryable failure here, the other moved nothing.
			// Looping would spin and continuing would open the next layer over
			// an unfinished one, so neither is safe.
			return fmt.Errorf("depth %d reports retryable pages but none could be requeued; refusing to continue", depth)
		}

		slog.Info("Retrying failed pages before opening the next layer", "depth", depth, "count", requeued)
		c.runWorkers()
	}
}

// confirmLayerDrained verifies a layer really is finished before the next one
// opens. A round that ends with work still at its own depth — without the run
// being cancelled or capped — means the barrier did not hold, so the crawl
// stops rather than proceeding on a false guarantee (and rather than picking
// the same depth again and spinning).
func (c *DefaultCrawler) confirmLayerDrained(depth int) error {
	if c.ctx.Err() != nil || c.limitReached() {
		return nil
	}

	hasItems, err := c.storage.HasQueuedItemsAtDepth(depth)
	if err != nil {
		return fmt.Errorf("failed to confirm depth %d is finished: %w", depth, err)
	}
	if hasItems {
		return fmt.Errorf("depth %d still holds queued pages after its workers finished; refusing to open the next layer", depth)
	}
	return nil
}

// limitReached reports whether the page budget is used up.
func (c *DefaultCrawler) limitReached() bool {
	if c.config.Limit <= 0 {
		return false
	}

	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()
	return c.stats.PagesCrawled >= c.config.Limit
}

// logLimitStop records that the depth bound was cut short by the page budget.
// Without this the run looks like a completed depth-N crawl when it is really a
// partial one, and the queued remainder would go unmentioned.
func (c *DefaultCrawler) logLimitStop() {
	pending, processing, _, _, err := c.storage.GetQueueStatus()
	if err != nil {
		slog.Info("Page limit reached before the depth limit was covered",
			"limit", c.config.Limit, "max_depth", c.config.MaxDepth, "stopped_at_depth", c.layerDepth)
		return
	}

	slog.Info("Page limit reached before the depth limit was covered - queued pages are kept for a later resume",
		"limit", c.config.Limit,
		"max_depth", c.config.MaxDepth,
		"stopped_at_depth", c.layerDepth,
		"pending", pending+processing)
}

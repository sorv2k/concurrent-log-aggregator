package ingestion

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/souravg/concurrent-log-aggregator/aggregator"
	"github.com/souravg/concurrent-log-aggregator/ratelimiter"
)

// CollectorMetrics holds metrics for the collector
type CollectorMetrics struct {
	EventsReceived int64            `json:"events_received"`
	EventsDropped  int64            `json:"events_dropped"`
	SourcesActive  int              `json:"sources_active"`
	PerSource      map[string]int64 `json:"per_source"`
}

// Collector manages multiple goroutines that generate log events
// from mock sources and send them through the rate limiter to the buffer
type Collector struct {
	rateLimiter     *ratelimiter.TokenBucketLimiter
	buffer          *aggregator.Buffer
	numSources      int
	eventsPerSecond int

	// Metrics
	eventsReceived atomic.Int64
	eventsDropped  atomic.Int64
	perSourceCount map[string]*atomic.Int64
	mu             sync.RWMutex

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCollector creates a new collector instance
func NewCollector(rateLimiter *ratelimiter.TokenBucketLimiter, buffer *aggregator.Buffer, numSources, eventsPerSecond int) *Collector {
	if numSources <= 0 {
		panic(fmt.Sprintf("number of sources must be positive, got %d", numSources))
	}
	if eventsPerSecond <= 0 {
		panic(fmt.Sprintf("events per second must be positive, got %d", eventsPerSecond))
	}

	return &Collector{
		rateLimiter:     rateLimiter,
		buffer:          buffer,
		numSources:      numSources,
		eventsPerSecond: eventsPerSecond,
		perSourceCount:  make(map[string]*atomic.Int64),
	}
}

// Start spawns goroutines for each mock source
func (c *Collector) Start(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)

	for i := 0; i < c.numSources; i++ {
		sourceID := fmt.Sprintf("source-%d", i+1)
		
		// Initialize per-source counter
		c.mu.Lock()
		c.perSourceCount[sourceID] = &atomic.Int64{}
		c.mu.Unlock()

		c.wg.Add(1)
		go c.runSource(c.ctx, sourceID)
	}
}

// Stop gracefully stops all source goroutines
func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

// GetMetrics returns current collector metrics
func (c *Collector) GetMetrics() CollectorMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	perSource := make(map[string]int64)
	for source, count := range c.perSourceCount {
		perSource[source] = count.Load()
	}

	return CollectorMetrics{
		EventsReceived: c.eventsReceived.Load(),
		EventsDropped:  c.eventsDropped.Load(),
		SourcesActive:  c.numSources,
		PerSource:      perSource,
	}
}

// runSource generates events for a single mock source
func (c *Collector) runSource(ctx context.Context, sourceID string) {
	defer c.wg.Done()

	// Calculate interval between events
	interval := time.Second / time.Duration(c.eventsPerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Generate mock event
			event := GenerateEvent(sourceID)

			// Apply rate limiting with backpressure
			if err := c.rateLimiter.Allow(ctx); err != nil {
				// Context cancelled or timed out
				if ctx.Err() != nil {
					return
				}
				c.eventsDropped.Add(1)
				continue
			}

			// Add to buffer
			if err := c.buffer.Add(event); err != nil {
				c.eventsDropped.Add(1)
				continue
			}

			// Update metrics
			c.eventsReceived.Add(1)
			c.mu.RLock()
			if counter, exists := c.perSourceCount[sourceID]; exists {
				counter.Add(1)
			}
			c.mu.RUnlock()
		}
	}
}

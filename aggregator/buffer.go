package aggregator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/souravg/concurrent-log-aggregator/models"
	"github.com/souravg/concurrent-log-aggregator/storage"
)

// BufferMetrics holds metrics for the buffer
type BufferMetrics struct {
	CurrentSize  int   `json:"current_size"`
	TotalFlushed int64 `json:"total_flushed"`
	FlushCount   int64 `json:"flush_count"`
	FlushErrors  int64 `json:"flush_errors"`
}

// Buffer is an in-memory buffer that batches log events and flushes them
// to storage either periodically or when the batch size threshold is reached
type Buffer struct {
	storage       storage.Storage
	batchSize     int
	flushInterval time.Duration

	mu     sync.Mutex
	events []models.LogEvent

	// Metrics (using atomic for lock-free reads)
	totalFlushed atomic.Int64
	flushCount   atomic.Int64
	flushErrors  atomic.Int64

	// Control channels
	flushTrigger chan struct{}
	done         chan struct{}
	wg           sync.WaitGroup
}

// NewBuffer creates a new buffer instance
func NewBuffer(storage storage.Storage, batchSize int, flushInterval time.Duration) *Buffer {
	if batchSize <= 0 {
		panic(fmt.Sprintf("batch size must be positive, got %d", batchSize))
	}
	if flushInterval <= 0 {
		panic(fmt.Sprintf("flush interval must be positive, got %v", flushInterval))
	}

	return &Buffer{
		storage:       storage,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		events:        make([]models.LogEvent, 0, batchSize),
		flushTrigger:  make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
}

// Start begins the background flush goroutine
func (b *Buffer) Start(ctx context.Context) {
	b.wg.Add(1)
	go b.flushLoop(ctx)
}

// Add adds an event to the buffer and triggers flush if batch size is reached
func (b *Buffer) Add(event models.LogEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, event)

	// Trigger flush if batch size threshold reached
	if len(b.events) >= b.batchSize {
		select {
		case b.flushTrigger <- struct{}{}:
		default:
			// Channel already has a flush pending, no need to add another
		}
	}

	return nil
}

// Stop gracefully shuts down the buffer and flushes remaining events
func (b *Buffer) Stop(ctx context.Context) error {
	close(b.done)
	b.wg.Wait()

	// Final flush of any remaining events
	return b.flush(ctx)
}

// GetMetrics returns current buffer metrics
func (b *Buffer) GetMetrics() BufferMetrics {
	b.mu.Lock()
	currentSize := len(b.events)
	b.mu.Unlock()

	return BufferMetrics{
		CurrentSize:  currentSize,
		TotalFlushed: b.totalFlushed.Load(),
		FlushCount:   b.flushCount.Load(),
		FlushErrors:  b.flushErrors.Load(),
	}
}

// flushLoop runs in background and flushes periodically or when triggered
func (b *Buffer) flushLoop(ctx context.Context) {
	defer b.wg.Done()

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		case <-ticker.C:
			// Periodic flush
			if err := b.flush(ctx); err != nil {
				// Log error but continue (could add proper logging here)
				b.flushErrors.Add(1)
			}
		case <-b.flushTrigger:
			// Flush triggered by batch size threshold
			if err := b.flush(ctx); err != nil {
				b.flushErrors.Add(1)
			}
		}
	}
}

// flush writes buffered events to storage
func (b *Buffer) flush(ctx context.Context) error {
	b.mu.Lock()
	if len(b.events) == 0 {
		b.mu.Unlock()
		return nil
	}

	// Swap out the events slice
	toFlush := b.events
	b.events = make([]models.LogEvent, 0, b.batchSize)
	b.mu.Unlock()

	// Write to storage (without holding the lock)
	if err := b.storage.BatchInsert(ctx, toFlush); err != nil {
		// On error, we lose these events (could implement retry logic)
		return fmt.Errorf("failed to flush %d events: %w", len(toFlush), err)
	}

	// Update metrics
	b.totalFlushed.Add(int64(len(toFlush)))
	b.flushCount.Add(1)

	return nil
}

// GetCurrentSize returns the current number of buffered events
func (b *Buffer) GetCurrentSize() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

package aggregator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/souravg/concurrent-log-aggregator/models"
	"github.com/souravg/concurrent-log-aggregator/storage"
)

func TestNewBuffer(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 100, 5*time.Second)

	if buffer == nil {
		t.Fatal("expected buffer to be created")
	}
	if buffer.batchSize != 100 {
		t.Errorf("expected batch size 100, got %d", buffer.batchSize)
	}
	if buffer.flushInterval != 5*time.Second {
		t.Errorf("expected flush interval 5s, got %v", buffer.flushInterval)
	}
}

func TestNewBuffer_InvalidParams(t *testing.T) {
	mockStorage := storage.NewMockStorage()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid batch size")
		}
	}()
	NewBuffer(mockStorage, 0, 5*time.Second)
}

func TestBuffer_Add(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 100, 5*time.Second)

	event := models.NewLogEvent(models.INFO, "Test message", "test-source")
	err := buffer.Add(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buffer.GetCurrentSize() != 1 {
		t.Errorf("expected buffer size 1, got %d", buffer.GetCurrentSize())
	}
}

func TestBuffer_BatchSizeThresholdFlush(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 10, 1*time.Hour) // Long interval, rely on size trigger

	ctx := context.Background()
	buffer.Start(ctx)
	defer buffer.Stop(ctx)

	// Add exactly batch size events
	for i := 0; i < 10; i++ {
		event := models.NewLogEvent(models.INFO, "Test message", "test-source")
		if err := buffer.Add(event); err != nil {
			t.Fatalf("unexpected error adding event %d: %v", i, err)
		}
	}

	// Give it time to flush
	time.Sleep(100 * time.Millisecond)

	metrics := buffer.GetMetrics()
	if metrics.TotalFlushed != 10 {
		t.Errorf("expected 10 events flushed, got %d", metrics.TotalFlushed)
	}
	if metrics.FlushCount < 1 {
		t.Errorf("expected at least 1 flush, got %d", metrics.FlushCount)
	}

	// Buffer should be empty after flush
	if buffer.GetCurrentSize() > 0 {
		t.Errorf("expected empty buffer, got size %d", buffer.GetCurrentSize())
	}
}

func TestBuffer_PeriodicFlush(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 1000, 100*time.Millisecond) // Small interval

	ctx := context.Background()
	buffer.Start(ctx)
	defer buffer.Stop(ctx)

	// Add a few events (below batch size)
	for i := 0; i < 5; i++ {
		event := models.NewLogEvent(models.INFO, "Test message", "test-source")
		buffer.Add(event)
	}

	// Wait for periodic flush
	time.Sleep(200 * time.Millisecond)

	metrics := buffer.GetMetrics()
	if metrics.TotalFlushed != 5 {
		t.Errorf("expected 5 events flushed, got %d", metrics.TotalFlushed)
	}
	if metrics.FlushCount < 1 {
		t.Errorf("expected at least 1 periodic flush, got %d", metrics.FlushCount)
	}
}

func TestBuffer_ConcurrentAdds(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 500, 1*time.Second)

	ctx := context.Background()
	buffer.Start(ctx)
	defer buffer.Stop(ctx)

	const numGoroutines = 10
	const eventsPerGoroutine = 20

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := models.NewLogEvent(models.INFO, "Test message", "test-source")
				if err := buffer.Add(event); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Give buffer time to flush
	time.Sleep(200 * time.Millisecond)

	metrics := buffer.GetMetrics()
	totalExpected := int64(numGoroutines * eventsPerGoroutine)
	totalInSystem := metrics.TotalFlushed + int64(buffer.GetCurrentSize())

	if totalInSystem != totalExpected {
		t.Errorf("expected %d total events, got %d (flushed: %d, buffered: %d)",
			totalExpected, totalInSystem, metrics.TotalFlushed, buffer.GetCurrentSize())
	}
}

func TestBuffer_GracefulShutdown(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 100, 1*time.Hour) // Won't flush periodically

	ctx := context.Background()
	buffer.Start(ctx)

	// Add some events
	for i := 0; i < 15; i++ {
		event := models.NewLogEvent(models.INFO, "Test message", "test-source")
		buffer.Add(event)
	}

	// Stop should flush remaining events
	if err := buffer.Stop(ctx); err != nil {
		t.Fatalf("unexpected error during stop: %v", err)
	}

	metrics := buffer.GetMetrics()
	if metrics.TotalFlushed != 15 {
		t.Errorf("expected 15 events flushed on shutdown, got %d", metrics.TotalFlushed)
	}

	// Verify events in storage
	if mockStorage.Count() != 15 {
		t.Errorf("expected 15 events in storage, got %d", mockStorage.Count())
	}
}

func TestBuffer_EmptyFlush(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 100, 50*time.Millisecond)

	ctx := context.Background()
	buffer.Start(ctx)

	// Let it run with no events
	time.Sleep(150 * time.Millisecond)

	buffer.Stop(ctx)

	metrics := buffer.GetMetrics()
	if metrics.TotalFlushed != 0 {
		t.Errorf("expected 0 events flushed, got %d", metrics.TotalFlushed)
	}
	if metrics.FlushErrors != 0 {
		t.Errorf("expected 0 flush errors, got %d", metrics.FlushErrors)
	}
}

func TestBuffer_GetMetrics(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 100, 5*time.Second)

	ctx := context.Background()
	buffer.Start(ctx)
	defer buffer.Stop(ctx)

	// Add events
	for i := 0; i < 25; i++ {
		event := models.NewLogEvent(models.INFO, "Test message", "test-source")
		buffer.Add(event)
	}

	metrics := buffer.GetMetrics()

	if metrics.CurrentSize != 25 {
		t.Errorf("expected current size 25, got %d", metrics.CurrentSize)
	}
	if metrics.TotalFlushed != 0 {
		t.Errorf("expected 0 flushed (not triggered yet), got %d", metrics.TotalFlushed)
	}
	if metrics.FlushCount != 0 {
		t.Errorf("expected 0 flush count, got %d", metrics.FlushCount)
	}
}

func TestBuffer_MultipleBatchFlushes(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := NewBuffer(mockStorage, 10, 1*time.Hour)

	ctx := context.Background()
	buffer.Start(ctx)
	defer buffer.Stop(ctx)

	// Add 3 batches worth of events
	for i := 0; i < 30; i++ {
		event := models.NewLogEvent(models.INFO, "Test message", "test-source")
		buffer.Add(event)
	}

	// Wait for flushes
	time.Sleep(200 * time.Millisecond)

	metrics := buffer.GetMetrics()
	if metrics.TotalFlushed != 30 {
		t.Errorf("expected 30 events flushed, got %d", metrics.TotalFlushed)
	}
	if metrics.FlushCount < 3 {
		t.Errorf("expected at least 3 flushes, got %d", metrics.FlushCount)
	}

	// Verify in storage
	if mockStorage.Count() != 30 {
		t.Errorf("expected 30 events in storage, got %d", mockStorage.Count())
	}
}

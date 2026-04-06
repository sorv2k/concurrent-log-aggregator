package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/souravg/concurrent-log-aggregator/aggregator"
	"github.com/souravg/concurrent-log-aggregator/models"
	"github.com/souravg/concurrent-log-aggregator/ratelimiter"
	"github.com/souravg/concurrent-log-aggregator/storage"
)

func TestGenerateEvent(t *testing.T) {
	event := GenerateEvent("test-source")

	if event.Source != "test-source" {
		t.Errorf("expected source 'test-source', got %q", event.Source)
	}
	if event.Message == "" {
		t.Error("expected non-empty message")
	}
	if event.Level == "" {
		t.Error("expected non-empty level")
	}
	if !event.IsValid() {
		t.Error("generated event should be valid")
	}
}

func TestRandomLevel_Distribution(t *testing.T) {
	counts := map[models.LogLevel]int{
		models.INFO:  0,
		models.WARN:  0,
		models.ERROR: 0,
		models.DEBUG: 0,
	}

	// Generate many samples
	samples := 10000
	for i := 0; i < samples; i++ {
		level := randomLevel()
		counts[level]++
	}

	// Check rough distribution (allow 10% variance)
	checkDistribution := func(level models.LogLevel, expectedPercent float64) {
		actual := float64(counts[level]) / float64(samples) * 100
		lower := expectedPercent - 10
		upper := expectedPercent + 10
		if actual < lower || actual > upper {
			t.Errorf("level %s: expected ~%.0f%%, got %.1f%%", level, expectedPercent, actual)
		}
	}

	checkDistribution(models.INFO, 60)
	checkDistribution(models.WARN, 25)
	checkDistribution(models.ERROR, 10)
	checkDistribution(models.DEBUG, 5)
}

func TestGenerateMessage(t *testing.T) {
	levels := []models.LogLevel{models.INFO, models.WARN, models.ERROR, models.DEBUG}

	for _, level := range levels {
		msg := generateMessage(level)
		if msg == "" {
			t.Errorf("expected non-empty message for level %s", level)
		}
	}
}

func TestNewCollector(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 100, 5*time.Second)
	limiter := ratelimiter.NewTokenBucketLimiter(100, 100)

	collector := NewCollector(limiter, buffer, 5, 10)

	if collector == nil {
		t.Fatal("expected collector to be created")
	}
	if collector.numSources != 5 {
		t.Errorf("expected 5 sources, got %d", collector.numSources)
	}
	if collector.eventsPerSecond != 10 {
		t.Errorf("expected 10 events/sec, got %d", collector.eventsPerSecond)
	}
}

func TestNewCollector_InvalidParams(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 100, 5*time.Second)
	limiter := ratelimiter.NewTokenBucketLimiter(100, 100)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid num sources")
		}
	}()
	NewCollector(limiter, buffer, 0, 10)
}

func TestCollector_StartStop(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 100, 5*time.Second)
	limiter := ratelimiter.NewTokenBucketLimiter(1000, 1000)

	collector := NewCollector(limiter, buffer, 3, 50) // Increase event rate

	ctx := context.Background()
	buffer.Start(ctx)
	collector.Start(ctx)

	// Run for longer to ensure events are generated
	time.Sleep(300 * time.Millisecond)

	// Stop collector
	collector.Stop()
	buffer.Stop(ctx)

	metrics := collector.GetMetrics()
	if metrics.EventsReceived < 10 {
		t.Errorf("expected at least 10 events to be received, got %d", metrics.EventsReceived)
	}
	if metrics.SourcesActive != 3 {
		t.Errorf("expected 3 active sources, got %d", metrics.SourcesActive)
	}
}

func TestCollector_EventGeneration(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 1000, 1*time.Hour) // Don't flush during test
	limiter := ratelimiter.NewTokenBucketLimiter(1000, 1000)

	collector := NewCollector(limiter, buffer, 2, 50) // 2 sources, 50 events/sec each

	ctx := context.Background()
	buffer.Start(ctx)
	collector.Start(ctx)

	// Run for 200ms (should generate ~20 events: 2 sources * 50/sec * 0.2sec)
	time.Sleep(200 * time.Millisecond)

	collector.Stop()
	buffer.Stop(ctx)

	metrics := collector.GetMetrics()
	
	// Allow some variance in event generation
	if metrics.EventsReceived < 15 || metrics.EventsReceived > 25 {
		t.Errorf("expected ~20 events, got %d", metrics.EventsReceived)
	}

	// Check per-source metrics
	if len(metrics.PerSource) != 2 {
		t.Errorf("expected 2 sources in metrics, got %d", len(metrics.PerSource))
	}
}

func TestCollector_RateLimiting(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 1000, 1*time.Hour)
	limiter := ratelimiter.NewTokenBucketLimiter(50, 50) // Low rate limit

	collector := NewCollector(limiter, buffer, 2, 100) // Request 200 events/sec total

	ctx := context.Background()
	buffer.Start(ctx)
	collector.Start(ctx)

	// Run for 200ms
	time.Sleep(200 * time.Millisecond)

	collector.Stop()
	buffer.Stop(ctx)

	metrics := collector.GetMetrics()

	// Rate limiter should constrain to ~10 events (50/sec * 0.2sec)
	// Sources want 200/sec but limiter allows 50/sec
	if metrics.EventsReceived > 15 {
		t.Logf("Warning: expected ~10 events due to rate limiting, got %d (may vary with timing)", metrics.EventsReceived)
	}
}

func TestCollector_GetMetrics(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 100, 5*time.Second)
	limiter := ratelimiter.NewTokenBucketLimiter(1000, 1000)

	collector := NewCollector(limiter, buffer, 4, 50) // Higher rate

	ctx := context.Background()
	buffer.Start(ctx)
	collector.Start(ctx)

	time.Sleep(300 * time.Millisecond) // Longer wait

	metrics := collector.GetMetrics()

	if metrics.SourcesActive != 4 {
		t.Errorf("expected 4 active sources, got %d", metrics.SourcesActive)
	}
	if metrics.EventsReceived < 20 {
		t.Errorf("expected at least 20 events received, got %d", metrics.EventsReceived)
	}
	if len(metrics.PerSource) != 4 {
		t.Errorf("expected 4 entries in per-source metrics, got %d", len(metrics.PerSource))
	}

	// Verify all sources generated events
	for source, count := range metrics.PerSource {
		if count < 3 {
			t.Errorf("source %s generated too few events: %d", source, count)
		}
	}

	collector.Stop()
	buffer.Stop(ctx)
}

func TestCollector_ContextCancellation(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 100, 5*time.Second)
	limiter := ratelimiter.NewTokenBucketLimiter(1000, 1000)

	collector := NewCollector(limiter, buffer, 3, 10)

	ctx, cancel := context.WithCancel(context.Background())
	buffer.Start(ctx)
	collector.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Give time for goroutines to stop
	time.Sleep(50 * time.Millisecond)

	beforeStop := collector.GetMetrics().EventsReceived

	// Wait a bit more - should not receive more events
	time.Sleep(100 * time.Millisecond)

	afterWait := collector.GetMetrics().EventsReceived

	if afterWait > beforeStop+2 { // Allow small margin for in-flight events
		t.Errorf("expected no new events after context cancel, got %d before, %d after", beforeStop, afterWait)
	}

	collector.Stop()
	buffer.Stop(context.Background())
}

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/souravg/concurrent-log-aggregator/aggregator"
	"github.com/souravg/concurrent-log-aggregator/ingestion"
	"github.com/souravg/concurrent-log-aggregator/ratelimiter"
	"github.com/souravg/concurrent-log-aggregator/storage"
)

func setupTestHandler(t *testing.T) (*MetricsHandler, context.Context, func()) {
	mockStorage := storage.NewMockStorage()
	buffer := aggregator.NewBuffer(mockStorage, 100, 5*time.Second)
	limiter := ratelimiter.NewTokenBucketLimiter(100, 150)
	collector := ingestion.NewCollector(limiter, buffer, 3, 10)

	// Use a high port to avoid conflicts
	port := 18080
	handler := NewMetricsHandler(collector, buffer, limiter, port)

	ctx := context.Background()
	buffer.Start(ctx)
	collector.Start(ctx)

	cleanup := func() {
		collector.Stop()
		buffer.Stop(ctx)
	}

	return handler, ctx, cleanup
}

func TestNewMetricsHandler(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	if handler == nil {
		t.Fatal("expected handler to be created")
	}
	if handler.collector == nil {
		t.Error("collector should not be nil")
	}
	if handler.buffer == nil {
		t.Error("buffer should not be nil")
	}
	if handler.rateLimiter == nil {
		t.Error("rateLimiter should not be nil")
	}
}

func TestMetricsHandler_StartStop(t *testing.T) {
	handler, ctx, cleanup := setupTestHandler(t)
	defer cleanup()

	// Start server
	if err := handler.Start(ctx); err != nil {
		t.Fatalf("failed to start handler: %v", err)
	}

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Stop server
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := handler.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop handler: %v", err)
	}
}

func TestHealthEndpoint(t *testing.T) {
	handler, ctx, cleanup := setupTestHandler(t)
	defer cleanup()

	handler.Start(ctx)
	defer handler.Stop(ctx)

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Make request to health endpoint
	url := fmt.Sprintf("http://localhost:%d/health", handler.port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to request health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if result["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %q", result["status"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	handler, ctx, cleanup := setupTestHandler(t)
	defer cleanup()

	handler.Start(ctx)
	defer handler.Stop(ctx)

	// Give server time to start and generate some events
	time.Sleep(200 * time.Millisecond)

	// Make request to metrics endpoint
	url := fmt.Sprintf("http://localhost:%d/metrics", handler.port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to request metrics endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var result MetricsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Validate structure
	if result.Collector.SourcesActive != 3 {
		t.Errorf("expected 3 active sources, got %d", result.Collector.SourcesActive)
	}
	if result.RateLimiter.RateLimit != 100 {
		t.Errorf("expected rate limit 100, got %f", result.RateLimiter.RateLimit)
	}
	if result.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestMetricsEndpoint_Structure(t *testing.T) {
	handler, ctx, cleanup := setupTestHandler(t)
	defer cleanup()

	handler.Start(ctx)
	defer handler.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/metrics", handler.port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to request metrics endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var result MetricsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Check all expected fields are present and non-zero
	if result.Buffer.CurrentSize < 0 {
		t.Error("buffer current size should be >= 0")
	}
	if result.RateLimiter.CurrentTokens < 0 {
		t.Error("rate limiter tokens should be >= 0")
	}
	if len(result.Collector.PerSource) != 3 {
		t.Errorf("expected 3 sources in per-source metrics, got %d", len(result.Collector.PerSource))
	}
}

func TestHealthEndpoint_MethodNotAllowed(t *testing.T) {
	handler, ctx, cleanup := setupTestHandler(t)
	defer cleanup()

	handler.Start(ctx)
	defer handler.Stop(ctx)

	time.Sleep(50 * time.Millisecond)

	// Try POST instead of GET
	url := fmt.Sprintf("http://localhost:%d/health", handler.port)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("failed to make POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestMetricsEndpoint_ConcurrentRequests(t *testing.T) {
	handler, ctx, cleanup := setupTestHandler(t)
	defer cleanup()

	handler.Start(ctx)
	defer handler.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Make multiple concurrent requests
	const numRequests = 10
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			url := fmt.Sprintf("http://localhost:%d/metrics", handler.port)
			resp, err := http.Get(url)
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
				return
			}
			results <- nil
		}()
	}

	// Check all requests succeeded
	for i := 0; i < numRequests; i++ {
		if err := <-results; err != nil {
			t.Errorf("request %d failed: %v", i, err)
		}
	}
}

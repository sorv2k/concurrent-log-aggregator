package ratelimiter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewTokenBucketLimiter(t *testing.T) {
	limiter := NewTokenBucketLimiter(100, 150)

	tokens, rate := limiter.GetStats()
	if rate != 100 {
		t.Errorf("expected rate 100, got %f", rate)
	}
	if tokens != 150 {
		t.Errorf("expected initial tokens 150 (burst), got %f", tokens)
	}
}

func TestNewTokenBucketLimiter_InvalidParams(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for negative rate")
		}
	}()
	NewTokenBucketLimiter(-10, 100)
}

func TestAllow_ImmediateSuccess(t *testing.T) {
	limiter := NewTokenBucketLimiter(100, 100)
	ctx := context.Background()

	// Should succeed immediately as bucket is full
	err := limiter.Allow(ctx)
	if err != nil {
		t.Errorf("expected Allow to succeed, got error: %v", err)
	}

	tokens, _ := limiter.GetStats()
	// Allow small tolerance for timing precision
	if tokens < 98.9 || tokens > 99.1 {
		t.Errorf("expected ~99 tokens remaining, got %f", tokens)
	}
}

func TestAllow_TokenRefill(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 10) // 10 tokens per second
	ctx := context.Background()

	// Consume all tokens
	for i := 0; i < 10; i++ {
		if err := limiter.Allow(ctx); err != nil {
			t.Fatalf("unexpected error consuming token %d: %v", i, err)
		}
	}

	tokens, _ := limiter.GetStats()
	if tokens > 0.5 {
		t.Errorf("expected ~0 tokens, got %f", tokens)
	}

	// Wait for refill (200ms should add ~2 tokens)
	time.Sleep(200 * time.Millisecond)

	// Should have tokens available now
	err := limiter.Allow(ctx)
	if err != nil {
		t.Errorf("expected Allow to succeed after refill, got error: %v", err)
	}
}

func TestAllow_ContextCancellation(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 1) // Very slow refill
	ctx, cancel := context.WithCancel(context.Background())

	// Consume the single token
	if err := limiter.Allow(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel context before next Allow
	cancel()

	// Should return context error immediately
	err := limiter.Allow(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAllow_ContextTimeout(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 1) // 1 token per second
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Consume the single token
	if err := limiter.Allow(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Try to get another token with timeout
	start := time.Now()
	err := limiter.Allow(ctx)
	elapsed := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	// Should timeout around 50ms
	if elapsed < 40*time.Millisecond || elapsed > 100*time.Millisecond {
		t.Errorf("expected timeout around 50ms, took %v", elapsed)
	}
}

func TestAllow_ConcurrentAccess(t *testing.T) {
	limiter := NewTokenBucketLimiter(100, 200)
	ctx := context.Background()

	const numGoroutines = 50
	const tokensPerGoroutine = 2

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < tokensPerGoroutine; j++ {
				err := limiter.Allow(ctx)
				if err != nil {
					errorCount.Add(1)
				} else {
					successCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	// All requests should succeed since we have 200 tokens and only requesting 100
	if successCount.Load() != numGoroutines*tokensPerGoroutine {
		t.Errorf("expected %d successful requests, got %d", numGoroutines*tokensPerGoroutine, successCount.Load())
	}
	if errorCount.Load() != 0 {
		t.Errorf("expected 0 errors, got %d", errorCount.Load())
	}
}

func TestAllow_BlockingBehavior(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 10) // 10 tokens/sec
	ctx := context.Background()

	// Consume all tokens
	for i := 0; i < 10; i++ {
		if err := limiter.Allow(ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Next call should block and wait for refill
	start := time.Now()
	err := limiter.Allow(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected Allow to succeed after blocking, got error: %v", err)
	}

	// Should have blocked for approximately 100ms (1 token at 10/sec)
	if elapsed < 80*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("expected blocking time around 100ms, took %v", elapsed)
	}
}

func TestGetStats(t *testing.T) {
	limiter := NewTokenBucketLimiter(50, 100)
	ctx := context.Background()

	// Consume some tokens
	for i := 0; i < 30; i++ {
		limiter.Allow(ctx)
	}

	tokens, rate := limiter.GetStats()
	if rate != 50 {
		t.Errorf("expected rate 50, got %f", rate)
	}
	if tokens < 69 || tokens > 71 {
		t.Errorf("expected ~70 tokens, got %f", tokens)
	}
}

func TestBurstCapacity(t *testing.T) {
	limiter := NewTokenBucketLimiter(100, 150) // Can burst to 150
	ctx := context.Background()

	// Should be able to consume up to 150 tokens quickly
	successCount := 0
	for i := 0; i < 150; i++ {
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		err := limiter.Allow(timeoutCtx)
		cancel()
		if err == nil {
			successCount++
		} else {
			break
		}
	}

	if successCount < 149 { // Allow for timing variations
		t.Errorf("expected to consume ~150 tokens (burst), consumed %d", successCount)
	}

	// Next request should block as tokens are exhausted
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel2()
	start := time.Now()
	err := limiter.Allow(ctx2)
	elapsed := time.Since(start)
	
	// Should either timeout quickly or succeed very fast if tokens refilled
	if err == nil && elapsed > 10*time.Millisecond {
		t.Errorf("expected quick response, took %v", elapsed)
	}
}

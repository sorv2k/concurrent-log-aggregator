package ratelimiter

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TokenBucketLimiter implements a token bucket rate limiting algorithm
// with blocking backpressure when capacity is exhausted
type TokenBucketLimiter struct {
	rate          float64       // tokens per second
	burst         float64       // maximum token capacity
	tokens        float64       // current available tokens
	lastRefill    time.Time     // last time tokens were refilled
	mu            sync.Mutex    // protects tokens and lastRefill
	cond          *sync.Cond    // for blocking when no tokens available
}

// NewTokenBucketLimiter creates a new token bucket rate limiter
// rate: tokens added per second
// burst: maximum token capacity (bucket size)
func NewTokenBucketLimiter(rate, burst float64) *TokenBucketLimiter {
	if rate <= 0 {
		panic(fmt.Sprintf("rate must be positive, got %f", rate))
	}
	if burst < rate {
		panic(fmt.Sprintf("burst (%f) must be >= rate (%f)", burst, rate))
	}

	limiter := &TokenBucketLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     burst, // start with full bucket
		lastRefill: time.Now(),
	}
	limiter.cond = sync.NewCond(&limiter.mu)

	return limiter
}

// Allow blocks until a token is available or context is cancelled
// Returns nil when token is acquired, or context error if cancelled
func (l *TokenBucketLimiter) Allow(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check for context cancellation before waiting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	for {
		// Refill tokens based on elapsed time
		l.refill()

		// If we have tokens available, consume one and return
		if l.tokens >= 1.0 {
			l.tokens -= 1.0
			return nil
		}

		// No tokens available, need to wait
		// Calculate how long until next token is available
		tokensNeeded := 1.0 - l.tokens
		waitDuration := time.Duration(float64(time.Second) * tokensNeeded / l.rate)

		// Create a timer for the wait duration
		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		// Unlock mutex while waiting on context or timer
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				l.cond.Broadcast()
				close(done)
			case <-timer.C:
				l.cond.Broadcast()
				close(done)
			}
		}()

		// Wait for signal
		l.cond.Wait()

		// Check if context was cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Loop back to try acquiring token again
	}
}

// refill adds tokens to the bucket based on elapsed time
// Must be called with mutex locked
func (l *TokenBucketLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()

	// Calculate tokens to add based on elapsed time
	tokensToAdd := elapsed * l.rate

	// Add tokens, but don't exceed burst capacity
	l.tokens = min(l.tokens+tokensToAdd, l.burst)
	l.lastRefill = now
}

// GetStats returns current rate limiter statistics
func (l *TokenBucketLimiter) GetStats() (currentTokens, rateLimit float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill() // Update tokens before returning stats
	return l.tokens, l.rate
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

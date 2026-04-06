package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/souravg/concurrent-log-aggregator/aggregator"
	"github.com/souravg/concurrent-log-aggregator/ingestion"
	"github.com/souravg/concurrent-log-aggregator/ratelimiter"
)

// MetricsResponse is the JSON response structure for the /metrics endpoint
type MetricsResponse struct {
	Collector   ingestion.CollectorMetrics `json:"collector"`
	Buffer      aggregator.BufferMetrics   `json:"buffer"`
	RateLimiter RateLimiterMetrics         `json:"rate_limiter"`
	Timestamp   time.Time                  `json:"timestamp"`
}

// RateLimiterMetrics holds rate limiter statistics
type RateLimiterMetrics struct {
	CurrentTokens float64 `json:"current_tokens"`
	RateLimit     float64 `json:"rate_limit"`
}

// MetricsHandler serves HTTP endpoints for health and metrics
type MetricsHandler struct {
	collector   *ingestion.Collector
	buffer      *aggregator.Buffer
	rateLimiter *ratelimiter.TokenBucketLimiter
	server      *http.Server
	port        int
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(collector *ingestion.Collector, buffer *aggregator.Buffer, rateLimiter *ratelimiter.TokenBucketLimiter, port int) *MetricsHandler {
	return &MetricsHandler{
		collector:   collector,
		buffer:      buffer,
		rateLimiter: rateLimiter,
		port:        port,
	}
}

// Start starts the HTTP server
func (h *MetricsHandler) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/metrics", h.handleMetrics)

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in background
	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error (in production would use proper logger)
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the HTTP server
func (h *MetricsHandler) Stop(ctx context.Context) error {
	if h.server == nil {
		return nil
	}
	return h.server.Shutdown(ctx)
}

// handleHealth returns 200 OK if the system is healthy
func (h *MetricsHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"service": "log-aggregator",
	})
}

// handleMetrics returns detailed system metrics as JSON
func (h *MetricsHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Gather metrics from all components
	collectorMetrics := h.collector.GetMetrics()
	bufferMetrics := h.buffer.GetMetrics()
	tokens, rateLimit := h.rateLimiter.GetStats()

	response := MetricsResponse{
		Collector: collectorMetrics,
		Buffer:    bufferMetrics,
		RateLimiter: RateLimiterMetrics{
			CurrentTokens: tokens,
			RateLimit:     rateLimit,
		},
		Timestamp: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
		return
	}
}

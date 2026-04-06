package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/souravg/concurrent-log-aggregator/aggregator"
	"github.com/souravg/concurrent-log-aggregator/config"
	"github.com/souravg/concurrent-log-aggregator/ingestion"
	"github.com/souravg/concurrent-log-aggregator/metrics"
	"github.com/souravg/concurrent-log-aggregator/ratelimiter"
	"github.com/souravg/concurrent-log-aggregator/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("Starting Concurrent Log Aggregator...")

	// Load configuration from environment
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("Configuration loaded:\n")
	fmt.Printf("  - Buffer Size: %d\n", cfg.BufferSize)
	fmt.Printf("  - Batch Threshold: %d\n", cfg.BatchThreshold)
	fmt.Printf("  - Flush Interval: %v\n", cfg.FlushInterval)
	fmt.Printf("  - Rate Limit: %.0f events/sec\n", cfg.RateLimit)
	fmt.Printf("  - Burst Capacity: %.0f\n", cfg.BurstCapacity)
	fmt.Printf("  - Mock Sources: %d\n", cfg.NumSources)
	fmt.Printf("  - Events Per Source: %d/sec\n", cfg.EventsPerSource)
	fmt.Printf("  - Metrics Port: %d\n", cfg.MetricsPort)

	// Create main context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize PostgreSQL storage
	fmt.Println("\nInitializing PostgreSQL storage...")
	store, err := storage.NewPostgresStorage(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer store.Close()

	// Initialize database schema
	if err := store.InitSchema(ctx); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	fmt.Println("✓ PostgreSQL storage initialized")

	// Create rate limiter
	fmt.Println("\nCreating rate limiter...")
	rateLimiter := ratelimiter.NewTokenBucketLimiter(cfg.RateLimit, cfg.BurstCapacity)
	fmt.Printf("✓ Rate limiter created (rate: %.0f/sec, burst: %.0f)\n", cfg.RateLimit, cfg.BurstCapacity)

	// Create buffer
	fmt.Println("\nCreating buffer...")
	buffer := aggregator.NewBuffer(store, cfg.BatchThreshold, cfg.FlushInterval)
	buffer.Start(ctx)
	fmt.Printf("✓ Buffer started (batch size: %d, flush interval: %v)\n", cfg.BatchThreshold, cfg.FlushInterval)

	// Create collector
	fmt.Println("\nCreating collector...")
	collector := ingestion.NewCollector(rateLimiter, buffer, cfg.NumSources, cfg.EventsPerSource)
	collector.Start(ctx)
	fmt.Printf("✓ Collector started (%d sources, %d events/sec per source)\n", cfg.NumSources, cfg.EventsPerSource)

	// Create metrics handler
	fmt.Println("\nStarting metrics server...")
	metricsHandler := metrics.NewMetricsHandler(collector, buffer, rateLimiter, cfg.MetricsPort)
	if err := metricsHandler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}
	fmt.Printf("✓ Metrics server started on port %d\n", cfg.MetricsPort)
	fmt.Printf("  - Health: http://localhost:%d/health\n", cfg.MetricsPort)
	fmt.Printf("  - Metrics: http://localhost:%d/metrics\n", cfg.MetricsPort)

	fmt.Println("\n===========================================")
	fmt.Println("Log Aggregator is running!")
	fmt.Println("Press Ctrl+C to shut down gracefully...")
	fmt.Println("===========================================")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n\nShutdown signal received, gracefully stopping...")

	// Graceful shutdown sequence
	fmt.Println("1. Stopping collector (no new events)...")
	collector.Stop()
	fmt.Println("   ✓ Collector stopped")

	fmt.Println("2. Stopping buffer (flushing remaining events)...")
	if err := buffer.Stop(ctx); err != nil {
		fmt.Printf("   ⚠ Error stopping buffer: %v\n", err)
	} else {
		fmt.Println("   ✓ Buffer stopped and flushed")
	}

	fmt.Println("3. Stopping metrics server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := metricsHandler.Stop(shutdownCtx); err != nil {
		fmt.Printf("   ⚠ Error stopping metrics server: %v\n", err)
	} else {
		fmt.Println("   ✓ Metrics server stopped")
	}

	fmt.Println("4. Closing storage connection...")
	if err := store.Close(); err != nil {
		fmt.Printf("   ⚠ Error closing storage: %v\n", err)
	} else {
		fmt.Println("   ✓ Storage closed")
	}

	// Print final statistics
	fmt.Println("\n===========================================")
	fmt.Println("Final Statistics:")
	fmt.Println("===========================================")
	collectorMetrics := collector.GetMetrics()
	bufferMetrics := buffer.GetMetrics()
	
	fmt.Printf("Collector:\n")
	fmt.Printf("  - Events Received: %d\n", collectorMetrics.EventsReceived)
	fmt.Printf("  - Events Dropped: %d\n", collectorMetrics.EventsDropped)
	fmt.Printf("  - Sources Active: %d\n", collectorMetrics.SourcesActive)
	
	fmt.Printf("\nBuffer:\n")
	fmt.Printf("  - Total Flushed: %d\n", bufferMetrics.TotalFlushed)
	fmt.Printf("  - Flush Count: %d\n", bufferMetrics.FlushCount)
	fmt.Printf("  - Flush Errors: %d\n", bufferMetrics.FlushErrors)
	fmt.Printf("  - Current Size: %d\n", bufferMetrics.CurrentSize)

	fmt.Println("\nShutdown complete. Goodbye!")
	return nil
}

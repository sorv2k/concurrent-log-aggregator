package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration loaded from environment variables
type Config struct {
	// Buffer configuration
	BufferSize      int           `json:"buffer_size"`
	FlushInterval   time.Duration `json:"flush_interval"`
	BatchThreshold  int           `json:"batch_threshold"`

	// Rate limiter configuration
	RateLimit     float64 `json:"rate_limit"`
	BurstCapacity float64 `json:"burst_capacity"`

	// Database configuration
	DatabaseURL string `json:"-"` // Hidden from JSON for security

	// Metrics server configuration
	MetricsPort int `json:"metrics_port"`

	// Mock source configuration
	NumSources       int `json:"num_sources"`
	EventsPerSource  int `json:"events_per_source"`
}

// LoadConfig loads configuration from environment variables with sensible defaults
func LoadConfig() (*Config, error) {
	cfg := &Config{
		BufferSize:      getEnvInt("BUFFER_SIZE", 1000),
		FlushInterval:   time.Duration(getEnvInt("FLUSH_INTERVAL_SEC", 5)) * time.Second,
		BatchThreshold:  getEnvInt("BATCH_THRESHOLD", 100),
		RateLimit:       getEnvFloat("RATE_LIMIT", 1000.0),
		BurstCapacity:   getEnvFloat("BURST_CAPACITY", 1500.0),
		DatabaseURL:     getEnvString("DATABASE_URL", "postgres://loguser:logpass@localhost:5432/logdb?sslmode=disable"),
		MetricsPort:     getEnvInt("METRICS_PORT", 8080),
		NumSources:      getEnvInt("NUM_SOURCES", 5),
		EventsPerSource: getEnvInt("EVENTS_PER_SOURCE", 10),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.BufferSize <= 0 {
		return fmt.Errorf("buffer size must be positive, got %d", c.BufferSize)
	}
	if c.FlushInterval <= 0 {
		return fmt.Errorf("flush interval must be positive, got %v", c.FlushInterval)
	}
	if c.BatchThreshold <= 0 {
		return fmt.Errorf("batch threshold must be positive, got %d", c.BatchThreshold)
	}
	if c.RateLimit <= 0 {
		return fmt.Errorf("rate limit must be positive, got %f", c.RateLimit)
	}
	if c.BurstCapacity < c.RateLimit {
		return fmt.Errorf("burst capacity (%f) must be >= rate limit (%f)", c.BurstCapacity, c.RateLimit)
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("database URL cannot be empty")
	}
	if c.MetricsPort <= 0 || c.MetricsPort > 65535 {
		return fmt.Errorf("metrics port must be between 1 and 65535, got %d", c.MetricsPort)
	}
	if c.NumSources <= 0 {
		return fmt.Errorf("number of sources must be positive, got %d", c.NumSources)
	}
	if c.EventsPerSource <= 0 {
		return fmt.Errorf("events per source must be positive, got %d", c.EventsPerSource)
	}
	return nil
}

// Helper functions to read environment variables with defaults

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

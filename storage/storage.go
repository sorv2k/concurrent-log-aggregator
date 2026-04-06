package storage

import (
	"context"

	"github.com/souravg/concurrent-log-aggregator/models"
)

// Storage defines the interface for persisting log events
type Storage interface {
	// BatchInsert inserts multiple log events in a single operation
	BatchInsert(ctx context.Context, events []models.LogEvent) error
	
	// Close cleans up resources and closes connections
	Close() error
}

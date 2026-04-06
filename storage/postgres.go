package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/souravg/concurrent-log-aggregator/models"
)

// PostgresStorage implements the Storage interface using PostgreSQL
type PostgresStorage struct {
	pool *pgxpool.Pool
}

// NewPostgresStorage creates a new PostgreSQL storage instance
func NewPostgresStorage(ctx context.Context, connString string) (*PostgresStorage, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Configure connection pool
	config.MaxConns = 10
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresStorage{pool: pool}, nil
}

// InitSchema creates the logs table and indices if they don't exist
func (s *PostgresStorage) InitSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS logs (
		id UUID PRIMARY KEY,
		timestamp TIMESTAMPTZ NOT NULL,
		level VARCHAR(10) NOT NULL,
		message TEXT NOT NULL,
		source VARCHAR(255) NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	
	CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_logs_source ON logs(source);
	CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);
	`

	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	return nil
}

// BatchInsert inserts multiple log events using COPY for optimal performance
func (s *PostgresStorage) BatchInsert(ctx context.Context, events []models.LogEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Use COPY for bulk insert - much faster than individual INSERTs
	rows := make([][]interface{}, len(events))
	for i, event := range events {
		rows[i] = []interface{}{
			event.ID,
			event.Timestamp,
			string(event.Level),
			event.Message,
			event.Source,
		}
	}

	copyCount, err := s.pool.CopyFrom(
		ctx,
		pgx.Identifier{"logs"},
		[]string{"id", "timestamp", "level", "message", "source"},
		pgx.CopyFromRows(rows),
	)

	if err != nil {
		return fmt.Errorf("failed to batch insert %d events: %w", len(events), err)
	}

	if copyCount != int64(len(events)) {
		return fmt.Errorf("expected to insert %d events, but inserted %d", len(events), copyCount)
	}

	return nil
}

// Close closes the connection pool
func (s *PostgresStorage) Close() error {
	s.pool.Close()
	return nil
}

// GetStats returns connection pool statistics
func (s *PostgresStorage) GetStats() map[string]interface{} {
	stat := s.pool.Stat()
	return map[string]interface{}{
		"total_conns":     stat.TotalConns(),
		"idle_conns":      stat.IdleConns(),
		"acquired_conns":  stat.AcquiredConns(),
		"max_conns":       stat.MaxConns(),
	}
}

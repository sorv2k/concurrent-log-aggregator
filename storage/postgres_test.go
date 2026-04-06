package storage

import (
	"context"
	"testing"
	"time"

	"github.com/souravg/concurrent-log-aggregator/models"
)

func TestMockStorage_BatchInsert(t *testing.T) {
	mock := NewMockStorage()
	ctx := context.Background()

	events := []models.LogEvent{
		models.NewLogEvent(models.INFO, "Test message 1", "test-source"),
		models.NewLogEvent(models.ERROR, "Test message 2", "test-source"),
		models.NewLogEvent(models.WARN, "Test message 3", "test-source"),
	}

	err := mock.BatchInsert(ctx, events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.Count() != 3 {
		t.Errorf("expected 3 events, got %d", mock.Count())
	}

	storedEvents := mock.GetEvents()
	for i, event := range events {
		if storedEvents[i].Message != event.Message {
			t.Errorf("event %d: expected message %q, got %q", i, event.Message, storedEvents[i].Message)
		}
	}
}

func TestMockStorage_EmptyBatch(t *testing.T) {
	mock := NewMockStorage()
	ctx := context.Background()

	err := mock.BatchInsert(ctx, []models.LogEvent{})
	if err != nil {
		t.Errorf("unexpected error for empty batch: %v", err)
	}

	if mock.Count() != 0 {
		t.Errorf("expected 0 events, got %d", mock.Count())
	}
}

func TestMockStorage_MultipleBatches(t *testing.T) {
	mock := NewMockStorage()
	ctx := context.Background()

	batch1 := []models.LogEvent{
		models.NewLogEvent(models.INFO, "Batch 1 message 1", "source-1"),
		models.NewLogEvent(models.INFO, "Batch 1 message 2", "source-1"),
	}

	batch2 := []models.LogEvent{
		models.NewLogEvent(models.WARN, "Batch 2 message 1", "source-2"),
		models.NewLogEvent(models.ERROR, "Batch 2 message 2", "source-2"),
		models.NewLogEvent(models.DEBUG, "Batch 2 message 3", "source-2"),
	}

	if err := mock.BatchInsert(ctx, batch1); err != nil {
		t.Fatalf("batch1 error: %v", err)
	}

	if err := mock.BatchInsert(ctx, batch2); err != nil {
		t.Fatalf("batch2 error: %v", err)
	}

	if mock.Count() != 5 {
		t.Errorf("expected 5 total events, got %d", mock.Count())
	}
}

func TestMockStorage_Close(t *testing.T) {
	mock := NewMockStorage()
	ctx := context.Background()

	events := []models.LogEvent{
		models.NewLogEvent(models.INFO, "Test", "source"),
	}

	// Should work before close
	err := mock.BatchInsert(ctx, events)
	if err != nil {
		t.Fatalf("unexpected error before close: %v", err)
	}

	// Close storage
	if err := mock.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	// Should fail after close
	err = mock.BatchInsert(ctx, events)
	if err != ErrStorageClosed {
		t.Errorf("expected ErrStorageClosed after close, got %v", err)
	}
}

func TestLogEventCreation(t *testing.T) {
	event := models.NewLogEvent(models.INFO, "Test message", "test-source")

	if event.Level != models.INFO {
		t.Errorf("expected level INFO, got %s", event.Level)
	}
	if event.Message != "Test message" {
		t.Errorf("expected message 'Test message', got %q", event.Message)
	}
	if event.Source != "test-source" {
		t.Errorf("expected source 'test-source', got %q", event.Source)
	}
	if event.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("expected non-nil UUID")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	// Timestamp should be recent
	if time.Since(event.Timestamp) > time.Second {
		t.Error("timestamp is too old")
	}
}

func TestLogEventValidation(t *testing.T) {
	tests := []struct {
		name  string
		event models.LogEvent
		valid bool
	}{
		{
			name:  "valid event",
			event: models.NewLogEvent(models.INFO, "message", "source"),
			valid: true,
		},
		{
			name: "empty message",
			event: models.LogEvent{
				ID:        models.NewLogEvent(models.INFO, "x", "x").ID,
				Timestamp: time.Now(),
				Level:     models.INFO,
				Message:   "",
				Source:    "source",
			},
			valid: false,
		},
		{
			name: "empty source",
			event: models.LogEvent{
				ID:        models.NewLogEvent(models.INFO, "x", "x").ID,
				Timestamp: time.Now(),
				Level:     models.INFO,
				Message:   "message",
				Source:    "",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.event.IsValid()
			if valid != tt.valid {
				t.Errorf("expected IsValid() = %v, got %v", tt.valid, valid)
			}
		})
	}
}

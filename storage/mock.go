package storage

import (
	"context"
	"fmt"

	"github.com/souravg/concurrent-log-aggregator/models"
)

// MockStorage is a simple in-memory storage for testing
type MockStorage struct {
	events []models.LogEvent
	closed bool
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		events: make([]models.LogEvent, 0),
	}
}

func (m *MockStorage) BatchInsert(ctx context.Context, events []models.LogEvent) error {
	if m.closed {
		return ErrStorageClosed
	}
	m.events = append(m.events, events...)
	return nil
}

func (m *MockStorage) Close() error {
	m.closed = true
	return nil
}

func (m *MockStorage) GetEvents() []models.LogEvent {
	return m.events
}

func (m *MockStorage) Count() int {
	return len(m.events)
}

var ErrStorageClosed = fmt.Errorf("storage is closed")

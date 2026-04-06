package models

import (
	"time"

	"github.com/google/uuid"
)

// LogLevel represents the severity level of a log event
type LogLevel string

const (
	DEBUG LogLevel = "DEBUG"
	INFO  LogLevel = "INFO"
	WARN  LogLevel = "WARN"
	ERROR LogLevel = "ERROR"
)

// LogEvent represents a single log entry in the system
type LogEvent struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
}

// NewLogEvent creates a new log event with generated UUID and current timestamp
func NewLogEvent(level LogLevel, message, source string) LogEvent {
	return LogEvent{
		ID:        uuid.New(),
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   message,
		Source:    source,
	}
}

// IsValid checks if the log event has all required fields
func (e LogEvent) IsValid() bool {
	return e.ID != uuid.Nil &&
		!e.Timestamp.IsZero() &&
		e.Level != "" &&
		e.Message != "" &&
		e.Source != ""
}

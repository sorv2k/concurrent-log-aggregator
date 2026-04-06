package ingestion

import (
	"fmt"
	"math/rand"

	"github.com/souravg/concurrent-log-aggregator/models"
)

// Mock message templates for realistic log generation
var (
	infoMessages = []string{
		"Request processed successfully",
		"User authentication successful",
		"Database query completed",
		"Cache hit for key",
		"API response sent",
		"Service health check passed",
		"Configuration loaded",
		"Connection established",
		"Transaction committed",
		"File uploaded successfully",
	}

	warnMessages = []string{
		"High memory usage detected",
		"Slow query execution time",
		"Retry attempt failed",
		"Cache miss - fetching from database",
		"Rate limit approaching threshold",
		"Connection pool nearing capacity",
		"Deprecated API endpoint called",
		"Invalid request parameter ignored",
		"Session about to expire",
		"Queue depth increasing",
	}

	errorMessages = []string{
		"Database connection failed",
		"Failed to parse request body",
		"Authentication token expired",
		"File not found",
		"Null pointer exception",
		"Timeout waiting for response",
		"Invalid configuration parameter",
		"Permission denied",
		"Out of memory error",
		"Network connection lost",
	}

	debugMessages = []string{
		"Entering function processRequest",
		"Variable state: counter=42",
		"Middleware chain executed",
		"Cache lookup for key: user_123",
		"Serializing response object",
		"Validating input parameters",
		"Acquired database connection from pool",
		"Released lock on resource",
		"Parsing JSON payload",
		"Routing request to handler",
	}
)

// GenerateEvent creates a realistic mock log event
func GenerateEvent(sourceID string) models.LogEvent {
	level := randomLevel()
	message := generateMessage(level)
	return models.NewLogEvent(level, message, sourceID)
}

// randomLevel returns a weighted random log level
// Distribution: INFO 60%, WARN 25%, ERROR 10%, DEBUG 5%
func randomLevel() models.LogLevel {
	r := rand.Intn(100)
	switch {
	case r < 60:
		return models.INFO
	case r < 85:
		return models.WARN
	case r < 95:
		return models.ERROR
	default:
		return models.DEBUG
	}
}

// generateMessage returns a random message for the given level
func generateMessage(level models.LogLevel) string {
	var messages []string
	switch level {
	case models.INFO:
		messages = infoMessages
	case models.WARN:
		messages = warnMessages
	case models.ERROR:
		messages = errorMessages
	case models.DEBUG:
		messages = debugMessages
	default:
		messages = infoMessages
	}

	idx := rand.Intn(len(messages))
	return fmt.Sprintf("%s [id=%d]", messages[idx], rand.Intn(10000))
}

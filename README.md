# Concurrent Log Aggregator

A concurrent log aggregation system that collects events from multiple sources, applies token bucket rate limiting with backpressure, buffers them in memory, and flushes batches to PostgreSQL. Built to demonstrate high-throughput event processing patterns with graceful degradation under load.

## Tech Stack

Go 1.24, PostgreSQL 15, pgx/v5 driver, Docker

## Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- PostgreSQL 15 (provided via Docker Compose)

## Running Locally

### With Docker Compose

```bash
docker-compose up -d
docker-compose logs -f log-aggregator
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

### Without Docker

Start PostgreSQL:

```bash
docker run -d \
  --name postgres \
  -e POSTGRES_USER=loguser \
  -e POSTGRES_PASSWORD=logpass \
  -e POSTGRES_DB=logdb \
  -p 5432:5432 \
  postgres:15-alpine
```

Run the application:

```bash
go mod download
export DATABASE_URL="postgres://loguser:logpass@localhost:5432/logdb?sslmode=disable"
go run .
```

The application starts mock log sources and begins aggregating events. Access metrics at `http://localhost:8080/metrics` and health checks at `http://localhost:8080/health`.

.PHONY: help build test test-race test-cover clean docker-build docker-up docker-down run

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-15s %s\n", $$1, $$2 } /^##@/ { printf "\n%s\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

build: ## Build the Go binary
	@echo "Building..."
	go build -o log-aggregator .
	@echo "✓ Build complete: ./log-aggregator"

run: ## Run the application locally
	@echo "Starting log aggregator..."
	go run .

test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	go test -race -v ./...

test-cover: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report generated: coverage.html"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -f log-aggregator coverage.out coverage.html
	@echo "✓ Cleaned"

##@ Docker

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t log-aggregator:latest .
	@echo "✓ Docker image built: log-aggregator:latest"

docker-up: ## Start services with Docker Compose
	@echo "Starting services..."
	docker-compose up -d
	@echo "✓ Services started"
	@echo ""
	@echo "Health: http://localhost:8080/health"
	@echo "Metrics: http://localhost:8080/metrics"
	@echo ""
	@echo "View logs: docker-compose logs -f log-aggregator"

docker-down: ## Stop services with Docker Compose
	@echo "Stopping services..."
	docker-compose down
	@echo "✓ Services stopped"

docker-restart: docker-down docker-up ## Restart Docker services

docker-logs: ## View Docker logs
	docker-compose logs -f log-aggregator

##@ Database

db-connect: ## Connect to PostgreSQL
	docker exec -it log-aggregator-postgres psql -U loguser -d logdb

db-logs: ## View database logs
	docker-compose logs postgres

##@ Utilities

fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...
	@echo "✓ Code formatted"

lint: ## Run linter (requires golangci-lint)
	@echo "Running linter..."
	golangci-lint run
	@echo "✓ Linting complete"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	@echo "✓ Dependencies downloaded"

tidy: ## Tidy go.mod
	@echo "Tidying go.mod..."
	go mod tidy
	@echo "✓ go.mod tidied"

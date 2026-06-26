.PHONY: all build test lint proto ci clean dev-up dev-down

all: lint test build

# Build all packages
build:
	go build ./...

# Run all tests with race detection
test:
	go test ./internal/... ./pkg/... -v -race -count=1

# Run tests without race (faster)
test-fast:
	go test ./internal/... ./pkg/... -count=1

# Lint all packages
lint:
	golangci-lint run ./internal/... ./pkg/...

# Generate protobuf code
proto:
	buf generate --path proto/spatialserver/v1

# Validate protobuf definitions
proto-lint:
	buf lint proto/

# Check for breaking proto changes
proto-breaking:
	buf breaking proto/ --against .git#branch=main

# Full CI pipeline
ci: lint test build

# Clean generated files
clean:
	rm -rf proto/gen/spatialserver/v1/*.pb.go

# Start development environment
dev-up:
	docker compose -f deploy/docker-compose/docker-compose.yml up -d

# Stop development environment
dev-down:
	docker compose -f deploy/docker-compose/docker-compose.yml down

# Tidy Go module dependencies
tidy:
	go mod tidy

# Format Go source code
fmt:
	go fmt ./internal/... ./pkg/...

# Update all dependencies
deps:
	go get -u ./...
	go mod tidy

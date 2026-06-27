.PHONY: all build test lint proto ci clean dev-up dev-down

all: lint test build

# Build all packages
build:
	go build ./...

# Run all tests with race detection
test:
	go test ./internal/... ./pkg/... ./apps/... -v -race -count=1

# Run tests without race (faster)
test-fast:
	go test ./internal/... ./pkg/... ./apps/... -count=1

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

# Build Docker images for all services
docker-build:
	docker build -f build/docker/gateway.Dockerfile -t spatial-gateway .
	docker build -f build/docker/room-service.Dockerfile -t spatial-room-service .
	docker build -f build/docker/game-server.Dockerfile -t spatial-game-server .

# Start demo environment with a test client
demo:
	@trap 'docker compose -f deploy/docker-compose/docker-compose.yml down' EXIT; \
	docker compose -f deploy/docker-compose/docker-compose.yml up -d --build --force-recreate; \
	echo "Waiting for gateway..."; \
	for i in 1 2 3 4 5 6 7 8 9 10; do \
		if curl -sf http://localhost:8080/health > /dev/null 2>&1; then \
			echo "Gateway ready!"; \
			break; \
		fi; \
		echo "  attempt $$i/10..."; \
		sleep 2; \
	done; \
	for i in 1 2 3 4 5; do \
		echo "Starting client (attempt $$i)"; \
		if go run ./tools/client/ -addr localhost:8080 -player p1 -runtime r1 -zone z1; then \
			break; \
		fi; \
		echo "  client failed, retrying in 3s..."; \
		sleep 3; \
	done

.PHONY: docker-build demo

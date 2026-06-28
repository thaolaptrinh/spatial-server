.PHONY: all build test lint proto ci clean dev-up dev-up-full dev-down scale-up scale-down obs-up obs-down

# Docker Compose files live under deploy/docker-compose/ (layered, Compose v2):
#   compose.yaml          base infra (Postgres + Redis)
#   compose.app.yaml      app services (room-service + gateway + game-server)
#   compose.scaling.yaml  multi-node (2 named game-server nodes)
#   compose.obs.yaml      observability overlay (Prometheus + Grafana + Loki)
COMPOSE := deploy/docker-compose

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

# Start base infra only (Postgres + Redis) for local `go run` development
dev-up:
	docker compose -f $(COMPOSE)/compose.yaml up -d

# Start base infra + app services (room-service + gateway + game-server)
dev-up-full:
	docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.app.yaml up -d

# Stop development environment
dev-down:
	docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.app.yaml down

# Multi-node topology: 2 named game-server nodes (ownership-transfer testing)
scale-up:
	docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.scaling.yaml up -d --build

scale-down:
	docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.scaling.yaml down

# Observability overlay (run on top of the app stack)
obs-up:
	docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.app.yaml -f $(COMPOSE)/compose.obs.yaml up -d

obs-down:
	docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.app.yaml -f $(COMPOSE)/compose.obs.yaml down

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

# Build Docker images via compose (reads build config from compose.app.yaml)
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
docker-build:
	docker compose -f $(COMPOSE)/compose.app.yaml build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME)

# Start demo environment (infra + app) with a test client
demo:
	@trap 'docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.app.yaml down' EXIT; \
	docker compose -f $(COMPOSE)/compose.yaml -f $(COMPOSE)/compose.app.yaml up -d --build --force-recreate; \
	echo "Waiting for gateway (HEALTHCHECK ensures room-service is ready)..."; \
	sleep 3; \
	for i in 1 2 3 4 5; do \
		echo "Starting client (attempt $$i)"; \
		if go run ./tools/client/ \
			-addr localhost:8080 \
			-player p1 -runtime r1 \
			-provision -room-service localhost:9001 \
			-pg-dsn "postgres://spatial:spatial@localhost:5432/spatial?sslmode=disable"; then \
			break; \
		fi; \
		echo "  client failed, retrying in 3s..."; \
		sleep 3; \
	done

.PHONY: test-integration
test-integration:
	go test -tags=integration -count=1 -timeout=120s ./tests/integration/...

.PHONY: docker-build demo

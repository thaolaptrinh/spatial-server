FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /game-server ./apps/game-server/
RUN CGO_ENABLED=0 go install github.com/grpc-ecosystem/grpc-health-probe@latest && \
    cp /go/bin/grpc-health-probe /grpc_health_probe

FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="spatial-game-server"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
LABEL org.opencontainers.image.source="https://github.com/anomalyco/spatial-server"
LABEL org.opencontainers.image.description="Game Server - Entity simulation, AOI"

WORKDIR /app
COPY --from=builder /game-server .
COPY --from=builder /grpc_health_probe /usr/local/bin/
COPY configs/ configs/
COPY internal/storage/migrations/ internal/storage/migrations/
EXPOSE 9000
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=3 \
  CMD ["/usr/local/bin/grpc_health_probe", "-addr=:9000"]
CMD ["./game-server"]

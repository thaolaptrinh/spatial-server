FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gateway ./apps/gateway/

FROM alpine:3.21
ARG VERSION=dev
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="spatial-gateway"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
LABEL org.opencontainers.image.source="https://github.com/anomalyco/spatial-server"
LABEL org.opencontainers.image.description="Gateway - WebSocket termination, auth, routing"

RUN apk add --no-cache ca-certificates wget && \
    adduser -D -u 1001 nonroot
WORKDIR /app
COPY --from=builder /gateway .
COPY configs/ configs/
EXPOSE 8080 9000
USER nonroot
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1
CMD ["./gateway"]

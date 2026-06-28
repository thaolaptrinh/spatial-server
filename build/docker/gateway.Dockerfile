FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gateway ./apps/gateway/
RUN CGO_ENABLED=0 go build -o /healthcheck ./tools/healthcheck/

FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="spatial-gateway"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
LABEL org.opencontainers.image.source="https://github.com/anomalyco/spatial-server"
LABEL org.opencontainers.image.description="Gateway - WebSocket termination, auth, routing"

WORKDIR /app
COPY --from=builder /gateway .
COPY --from=builder /healthcheck /usr/local/bin/
COPY configs/ configs/
EXPOSE 8080 9000
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/healthcheck"]
CMD ["./gateway"]

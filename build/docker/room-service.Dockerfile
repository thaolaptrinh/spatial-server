FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /room-service ./apps/room-service/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /room-service .
COPY configs/ configs/
COPY internal/storage/migrations/ internal/storage/migrations/
EXPOSE 9000
CMD ["./room-service"]

# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Install swag CLI (match go.mod version)
RUN go install github.com/swaggo/swag/cmd/swag@v1.8.12

# Copy dependency files first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate swagger docs
RUN swag init -g cmd/api/main.go

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o api ./cmd/api

# Runtime stage
# Use postgres:15-alpine so pg_dump / pg_restore are bundled at exactly the
# same major version as the admission-db container — the /admin/db/backup
# and /admin/db/restore endpoints shell out to those binaries.
FROM postgres:15-alpine

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy binary and required files from builder
COPY --from=builder /app/api .
COPY --from=builder /app/migration ./migration

# Copy and set up entrypoint
COPY entrypoint.sh /app/entrypoint.sh
RUN sed -i 's/\r$//' /app/entrypoint.sh && chmod +x /app/entrypoint.sh

# Expose application port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]

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
# Use postgres:18-alpine so pg_dump / pg_restore are at the newest stable
# major. The db server stays on pg 15 — newer clients are officially
# supported against older servers, AND pg_restore 18 can read dumps produced
# by any pg 15..18 client (custom-format archive v1.14..v1.16). This lets
# operators upload dumps exported from a local Mac (where libpq is often
# the newest version brew ships) without hitting "unsupported version" errors.
FROM postgres:18-alpine

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

#!/bin/sh
# pre-commit hook: ensure code compiles and passes basic checks

set -e

echo "Running pre-commit checks..."

# 1. Build
echo "[1/2] go build ./cmd/api"
go build ./cmd/api
echo "    OK"

# 2. Vet
echo "[2/2] go vet ./..."
go vet ./...
echo "    OK"

echo "All pre-commit checks passed."

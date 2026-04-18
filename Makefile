.PHONY: build run dev db up migrate-up migrate-down test test-cover lint tidy swagger

build:
	$(MAKE) swagger
	go build -o api ./cmd/api

run:
	go run ./cmd/api

dev:
	docker-compose up -d

db:
	docker-compose up -d db redis

up: dev
	@echo "Waiting for services to be ready..."
	@sleep 3
	$(MAKE) migrate-up
	$(MAKE) run

migrate-up:
	go run ./cmd/api -migrate up

migrate-down:
	go run ./cmd/api -migrate down

test:
	go test ./...

test-cover:
	go test -cover ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

swagger:
	swag init -g cmd/api/main.go

.PHONY: dev run down logs build db

db:
	@if [ ! -f .env ]; then \
		echo "Creating .env from .env.example..."; \
		cp .env.example .env; \
	fi
	@echo "Ensuring component variables in .env..."
	@POSTGRES_PORT=$$(grep '^POSTGRES_PORT=' .env 2>/dev/null | cut -d= -f2); \
	POSTGRES_PORT=$${POSTGRES_PORT:-5432}; \
	REDIS_PORT=$$(grep '^REDIS_PORT=' .env 2>/dev/null | cut -d= -f2); \
	REDIS_PORT=$${REDIS_PORT:-6379}; \
	if grep -q "^POSTGRES_PORT=" .env; then \
		sed -i.bak "s|^POSTGRES_PORT=.*|POSTGRES_PORT=$${POSTGRES_PORT}|" .env && rm -f .env.bak; \
	else \
		echo "POSTGRES_PORT=$${POSTGRES_PORT}" >> .env; \
	fi; \
	if grep -q "^REDIS_PORT=" .env; then \
		sed -i.bak "s|^REDIS_PORT=.*|REDIS_PORT=$${REDIS_PORT}|" .env && rm -f .env.bak; \
	else \
		echo "REDIS_PORT=$${REDIS_PORT}" >> .env; \
	fi
	@echo "Starting infrastructure containers..."
	@docker-compose up -d db redis
	@echo "Waiting for database to be ready..."
	@until docker-compose exec -T db pg_isready -U app -d admission > /dev/null 2>&1; do \
		sleep 1; \
	done
	@echo "Waiting for redis to be ready..."
	@until docker-compose exec -T redis redis-cli ping | grep -q PONG; do \
		sleep 1; \
	done
	@echo "Running database migrations..."
	@go run ./cmd/api -migrate up
	@echo "Database initialized successfully!"

dev:
	@if [ ! -f .env ]; then \
		echo "Creating .env from .env.example..."; \
		cp .env.example .env; \
	fi
	@if [ ! -f docs/docs.go ]; then \
		echo "Generating swagger docs..."; \
		go run github.com/swaggo/swag/cmd/swag@v1.8.12 init -g cmd/api/main.go; \
	fi
	docker-compose up -d
	@echo "Waiting for db..."
	@sleep 3
	go run ./cmd/api -migrate up
	go run ./cmd/api

run:
	docker-compose -f docker-compose.prod.yml up --build -d

down:
	docker-compose down
	docker-compose -f docker-compose.prod.yml down

logs:
	docker-compose -f docker-compose.prod.yml logs -f app

build:
	docker build -t admission-api .

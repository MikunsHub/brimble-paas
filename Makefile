.PHONY: generate-module test-prod test-prod-setup test-prod-run test-prod-cleanup check-health fetch-staging-db
-include .env
export

DB_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)"

dev:
	@set -a && . ./.env && set +a && air

dev-up:
	docker compose -f docker-compose.yml up -d

dev-down:
	docker compose -f docker-compose.yml down

dev-logs:
	docker compose -f docker-compose.yml logs -f

MIGRATIONS_DIR = api/migrations

migration-up:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_URL)" up

migration-down:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_URL)" down

migration-create:
	@[ "$(name)" ] || { echo "Usage: make migration-create name=<migration_name>"; exit 1; }
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

build:
	cd api && go build -o bin/server ./cmd/app

run:
	cd api && go run ./cmd/app

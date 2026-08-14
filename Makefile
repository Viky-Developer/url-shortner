SHELL := /bin/bash

MIGRATIONS_DIR := internal/db/migrations
DB_DSN = postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

-include .env
export

.DEFAULT_GOAL := help

## ---------------------------------------------------------------------------
## Help
## ---------------------------------------------------------------------------

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}'

## ---------------------------------------------------------------------------
## Docker
## ---------------------------------------------------------------------------

.PHONY: docker-up
docker-up: ## Start postgres container (and infra) in background
	docker compose up -d

.PHONY: docker-down
docker-down: ## Stop and remove containers, keep volumes
	docker compose down

.PHONY: docker-down-volumes
docker-down-volumes: ## Stop and remove containers AND volumes (destroys data)
	docker compose down -v

.PHONY: db-wait
db-wait: ## Wait until postgres is healthy
	@echo "Waiting for postgres..."
	@until [ "$$(docker inspect --format '{{.State.Health.Status}}' url-shortner-db 2>/dev/null)" = "healthy" ]; do \
		sleep 2; \
	done
	@echo "Postgres is healthy"

.PHONY: db-up
db-up: docker-up db-wait ## Start db and apply migrations
	$(MAKE) migration-up

## ---------------------------------------------------------------------------
## Hooks (lefthook)
## ---------------------------------------------------------------------------

.PHONY: lefthook-install
lefthook-install: ## Install git hooks via lefthook
	lefthook install

.PHONY: lefthook-run
lefthook-run: ## Manually run all pre-commit checks
	lefthook run pre-commit

## ---------------------------------------------------------------------------
## Branches
## ---------------------------------------------------------------------------

.PHONY: branch
branch: ## Create a prefixed branch: make branch type=feat name=add-login
	@test -n "$(type)" || (echo "Usage: make branch type=<feat|refactor|bug|fix|chore> name=<branch-name>"; exit 1)
	@test -n "$(name)" || (echo "Usage: make branch type=<feat|refactor|bug|fix|chore> name=<branch-name>"; exit 1)
	@case "$(type)" in feat|refactor|bug|fix|chore|hotfix) ;; *) echo "invalid type '$(type)': allowed types are feat|refactor|bug|fix|chore|hotfix"; exit 1;; esac
	git checkout -b "$(type)/$(name)"

## ---------------------------------------------------------------------------
## Code quality
## ---------------------------------------------------------------------------

.PHONY: format
format: ## Auto-format Go files with gofmt + goimports
	gofmt -w $$(git ls-files '*.go')
	goimports -w $$(git ls-files '*.go')

.PHONY: format-check
format-check: ## Check formatting (fails if unformatted)
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))" || { echo "unformatted files:"; gofmt -l $$(git ls-files '*.go'); exit 1; }

CHECK_PACKAGES := $(shell go list ./... | grep -v '/internal/db/gen')

.PHONY: field-alignment
field-alignment: ## Auto-align Go struct fields (fieldalignment -fix)
	@for p in $(CHECK_PACKAGES); do fieldalignment -fix $$p || exit 1; done

.PHONY: field-alignment-check
field-alignment-check: ## Check struct field alignment (fails if unaligned)
	@for p in $(CHECK_PACKAGES); do fieldalignment $$p || exit 1; done

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./... --timeout 5m

.PHONY: vulncheck
vulncheck: ## Scan dependencies for known vulnerabilities
	govulncheck ./...

## ---------------------------------------------------------------------------
## Migrations (goose)
## ---------------------------------------------------------------------------

.PHONY: migration-create
migration-create: ## Create a new migration: make migration-create name=add_indexes
	@test -n "$(name)" || (echo "Usage: make migration-create name=your_migration_name"; exit 1)
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

.PHONY: migration-up
migration-up: ## Apply all pending migrations
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" up

.PHONY: migration-down
migration-down: ## Roll back the last migration
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" down

.PHONY: migration-status
migration-status: ## Show applied vs pending migrations
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" status

.PHONY: migration-reset
migration-reset: ## Roll back ALL migrations
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" reset

## ---------------------------------------------------------------------------
## Build & run
## ---------------------------------------------------------------------------

.PHONY: sqlc-generate
sqlc-generate: ## Regenerate type-safe db code from SQL queries
	sqlc generate

.PHONY: build
build: ## Compile the server binary
	go build -o bin/url-shortner ./cmd/server

.PHONY: run
run: ## Build and run the server
	go run ./cmd/server

.PHONY: test
test: ## Run all tests
	go test ./... -count=1

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin

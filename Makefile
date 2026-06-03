.DEFAULT_GOAL := help
SHELL := /bin/bash

# Image name for `make docker`.
IMAGE ?= stonewrit/server

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the server, stonewrit CLI, and verify binaries into ./bin
	go build -o bin/server ./server/cmd/server
	go build -o bin/stonewrit ./server/cmd/stonewrit
	go build -o bin/verify ./cmd/verify

.PHONY: test
test: ## Run unit tests with the race detector and coverage
	go test -race -coverprofile=coverage.out ./...

.PHONY: test-integration
test-integration: ## Run integration tests against a disposable Postgres in Docker
	@echo "Starting a throwaway Postgres for integration tests..."
	@docker run --rm -d --name stonewrit-it -e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_DB=stonewrit -p 55432:5432 postgres:16-alpine >/dev/null
	@echo "Waiting for Postgres..."
	@until docker exec stonewrit-it pg_isready -U postgres >/dev/null 2>&1; do sleep 0.5; done
	@DATABASE_URL="postgres://postgres:postgres@127.0.0.1:55432/stonewrit?sslmode=disable" \
		go test -tags=integration -p 1 ./... ; status=$$? ; \
		docker rm -f stonewrit-it >/dev/null ; exit $$status

.PHONY: fuzz
fuzz: ## Run the canonicalization fuzz target for 30s
	go test -run '^$$' -fuzz=FuzzCanonicalJSON -fuzztime=30s ./core/

.PHONY: lint
lint: ## Run golangci-lint (install from https://golangci-lint.run)
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format all Go code
	gofmt -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: sqlc
sqlc: ## Regenerate type-safe query code from the schema
	sqlc generate

.PHONY: vuln
vuln: ## Scan for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: golden
golden: ## Rebuild the frozen conformance vectors (only when intentionally rotating the version)
	go test ./core/ -run TestGoldenVectors -update

.PHONY: migrate
migrate: ## Apply database migrations (reads DATABASE_URL)
	go run ./server/cmd/stonewrit migrate up

.PHONY: run
run: ## Run the server locally
	go run ./server/cmd/server

.PHONY: verify
verify: ## Verify an evidence bundle, for example: make verify BUNDLE=bundle.json
	go run ./cmd/verify $(BUNDLE)

.PHONY: docker
docker: ## Build the server Docker image
	docker build -t $(IMAGE) .

.PHONY: up
up: ## Start Postgres, run migrations, and run the server via docker compose
	docker compose up --build

.PHONY: clean
clean: ## Remove build and coverage artifacts
	rm -rf bin dist coverage.out coverage.html

IMAGE := mcp-monday-projects:local
COMPOSE := docker compose -f compose.yaml

.PHONY: help build up run down logs test race fmt-check vet validate clean

help: ## Show available Docker-first targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

build: ## Build the production image
	docker build -t $(IMAGE) .

up: build ## Start the Compose MCP service
	$(COMPOSE) up -d mcp

run: build ## Run the stdio MCP server interactively
	$(COMPOSE) run --rm mcp

down: ## Stop Compose services
	$(COMPOSE) down

logs: ## Follow Compose logs
	$(COMPOSE) logs -f mcp

test: ## Run all Go tests inside the builder container
	$(COMPOSE) --profile test run --rm test

race: ## Run race-enabled tests inside the Docker race builder
	docker build --target race-builder -t $(IMAGE)-race-builder .
	docker run --rm $(IMAGE)-race-builder sh -c 'CGO_ENABLED=1 go test -race ./...'

fmt-check: ## Fail if any Go file needs gofmt
	docker build --target builder -t $(IMAGE)-builder .
	docker run --rm $(IMAGE)-builder sh -c 'test -z "$$(gofmt -l .)"'

vet: ## Run go vet inside Docker
	docker build --target builder -t $(IMAGE)-builder .
	docker run --rm $(IMAGE)-builder go vet ./...

validate: build test race fmt-check vet ## Run the complete local validation suite
	@echo "Validation passed"

clean: ## Remove local images and Compose resources
	$(COMPOSE) down --remove-orphans
	docker image rm $(IMAGE) $(IMAGE)-builder $(IMAGE)-race-builder 2>/dev/null || true

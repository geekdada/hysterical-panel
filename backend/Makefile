# Hysterical Panel backend — PocketBase-based Go app
# See README.md for env vars and API overview.

.PHONY: help build run serve dev test vet fmt tidy clean migrate migrate-create openapi

GO ?= go
BIN_NAME ?= hysterical-panel
BIN_DIR ?= dist
MAIN ?= .
VERSION ?= $(shell tr -d '[:space:]' < ../VERSION 2>/dev/null || echo dev)
LDFLAGS ?= -ldflags "-s -w -X hysterical-panel/internal/version.Version=$(VERSION)"

# Fallback when not using direnv: load .env into make's environment
ifneq (,$(wildcard .env))
include .env
export
endif

.DEFAULT_GOAL := help

help: ## Show targets
	@grep -E '^[a-zA-Z0-9_.-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build binary to dist/
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(BIN_NAME) $(MAIN)

run: serve ## Alias for serve

serve: ## Run dev server (requires PANEL_MASTER_KEY in env)
	@test -n "$$PANEL_MASTER_KEY" || (echo "error: cp .env.example .env, run direnv allow, or export PANEL_MASTER_KEY" >&2; exit 1)
	$(GO) run $(MAIN) serve

dev: serve ## Alias for serve

$(BIN_DIR)/$(BIN_NAME): build

serve-bin: $(BIN_DIR)/$(BIN_NAME) ## Run built binary
	@test -n "$$PANEL_MASTER_KEY" || (echo "error: cp .env.example .env, run direnv allow, or export PANEL_MASTER_KEY" >&2; exit 1)
	./$(BIN_DIR)/$(BIN_NAME) serve

test: ## Run tests
	$(GO) test ./...

vet: ## Run go vet
	$(GO) vet ./...

fmt: ## Format packages
	$(GO) fmt ./...

tidy: ## Sync go.mod / go.sum
	$(GO) mod tidy

clean: ## Remove build artifacts and test outputs
	rm -rf $(BIN_DIR) coverage.* *.test *.out

migrate: ## Apply database migrations
	$(GO) run $(MAIN) migrate

migrate-create: ## Create migration (usage: make migrate-create name=add_foo)
	@test -n "$(name)" || (echo "usage: make migrate-create name=<migration_name>" >&2; exit 1)
	$(GO) run $(MAIN) migrate create "$(name)"

openapi: ## Generate openapi.json
	PANEL_MASTER_KEY=skip $(GO) run $(MAIN) openapi-schema -o openapi.json

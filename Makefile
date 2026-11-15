BINARY := cohort-builder
GO ?= go
NPM ?= npm

.DEFAULT_GOAL := help

.PHONY: help
help: ## show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: dev
dev: ## run API (:8080) + vite dev server (:5173) together
	@$(GO) run ./cmd/server & \
	cd web && $(NPM) run dev

.PHONY: sync-kueue
sync-kueue: ## re-sync typed API snapshots + CRDs from kueue-operator branches
	./hack/sync-kueue.sh

.PHONY: web-build
web-build: ## build the UI into internal/server/static
	cd web && $(NPM) run build
	rm -rf internal/server/static
	cp -r web/dist internal/server/static

.PHONY: build
build: web-build ## single binary with embedded UI
	$(GO) build -o bin/$(BINARY) ./cmd/server

.PHONY: test
test: ## go tests
	$(GO) test ./...

.PHONY: test-web
test-web: ## frontend build gate (tsc + vite)
	cd web && $(NPM) run build

.PHONY: run
run: ## run the built binary
	./bin/$(BINARY)

.PHONY: clean
clean:
	rm -rf bin web/dist internal/server/static

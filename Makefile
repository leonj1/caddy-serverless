.PHONY: help test integration-test lint build clean docker-test-images run-example

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run unit tests
	go test -v -race ./...

integration-test: docker-test-images ## Run integration tests (requires Docker)
	go test -v -tags=integration -timeout=10m ./...

lint: ## Run golangci-lint
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m; \
	else \
		echo "golangci-lint not installed. Install it from https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

build: ## Build Caddy with the serverless plugin using xcaddy
	@if command -v xcaddy >/dev/null 2>&1; then \
		xcaddy build --with github.com/jose/caddy-serverless=.; \
	else \
		echo "xcaddy not installed. Install it with: go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest"; \
		exit 1; \
	fi

clean: ## Clean build artifacts and test cache
	rm -f caddy
	go clean -cache
	go clean -testcache
	docker rmi caddy-serverless-go-echoserver-test:latest 2>/dev/null || true
	docker rmi caddy-serverless-py-echoserver-test:latest 2>/dev/null || true

docker-test-images: ## Build Docker test images for integration tests
	@echo "Building Go echo server test image..."
	docker build -t caddy-serverless-go-echoserver-test:latest ./testdata/echoserver/
	@echo "Building Python echo server test image..."
	docker build -t caddy-serverless-py-echoserver-test:latest ./testdata/pyechoserver/

run-example: build ## Run Caddy with the example configuration
	./caddy run --config example.Caddyfile --adapter caddyfile

fmt: ## Format Go code
	go fmt ./...
	gofmt -s -w .

vet: ## Run go vet
	go vet ./...

mod-tidy: ## Tidy go.mod and go.sum
	go mod tidy

mod-verify: ## Verify module dependencies
	go mod verify

check: fmt vet lint test ## Run all checks (format, vet, lint, test)

release-dry-run: ## Dry run of goreleaser
	@if command -v goreleaser >/dev/null 2>&1; then \
		goreleaser release --snapshot --skip-publish --clean; \
	else \
		echo "goreleaser not installed. Install it from https://goreleaser.com/install/"; \
		exit 1; \
	fi

# Development helpers
.PHONY: dev-deps
dev-deps: ## Install development dependencies
	go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Docker helpers
.PHONY: docker-cleanup
docker-cleanup: ## Clean up Docker containers created by serverless functions
	@echo "Stopping and removing serverless containers..."
	docker ps -a --filter "label=caddy.serverless" -q | xargs -r docker stop
	docker ps -a --filter "label=caddy.serverless" -q | xargs -r docker rm

# CI simulation
.PHONY: ci
ci: clean check integration-test build ## Run full CI pipeline locally
	@echo "CI pipeline completed successfully!"
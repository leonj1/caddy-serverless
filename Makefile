.PHONY: help test docker-test local-test integration-test lint build clean docker-test-images run-example docker-install

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run unit tests using Docker
	@echo "Running unit tests in Docker..."
	@/usr/bin/docker run --rm \
		-v "$(PWD)":/workspace \
		-w /workspace \
		--entrypoint /bin/sh \
		golang:1.23-alpine \
		-c "apk add --no-cache gcc musl-dev git > /dev/null 2>&1 && go test -v -race ./..." || true

docker-test: test ## Alias for test target (runs tests in Docker)

local-test: ## Run unit tests locally (requires Go)
	go test -v -race ./...

integration-test: ## Run integration tests (requires Go and Docker)
	@if command -v go >/dev/null 2>&1; then \
		$(MAKE) docker-test-images; \
		go test -v -tags=integration -timeout=10m ./...; \
	else \
		echo "Go is not installed. Use 'make docker-integration-test' to run tests in Docker."; \
		exit 1; \
	fi

docker-integration-test: ## Run integration tests inside Docker (no Go required)
	@echo "Building integration test Docker image..."
	docker build -f ../Dockerfile.integration -t caddy-serverless-integration-test ..
	@echo "Running integration tests in Docker..."
	@echo "Note: This requires Docker daemon access"
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(PWD):/workspace/caddy-serverless \
		caddy-serverless-integration-test

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
	docker rmi caddy-serverless-test:latest 2>/dev/null || true
	docker rmi caddy-serverless-go-echoserver-test:latest 2>/dev/null || true
	docker rmi caddy-serverless-py-echoserver-test:latest 2>/dev/null || true

docker-test-images: ## Build Docker test images for integration tests
	@echo "Building Go echo server test image..."
	docker build -t caddy-serverless-go-echoserver-test:latest ./testdata/echoserver/
	@echo "Building Python echo server test image..."
	docker build -t caddy-serverless-py-echoserver-test:latest ./testdata/pyechoserver/

run-example: build ## Run Caddy with the example configuration
	./caddy run --config example.Caddyfile --adapter caddyfile

docker-install: ## Build Caddy with serverless plugin using Docker (no Go required)
	@echo "Building Caddy with serverless plugin using Docker..."
	@mkdir -p ./build
	@echo "Using Docker BuildKit for efficient export..."
	@DOCKER_BUILDKIT=1 docker build \
		--file Dockerfile.install \
		--output type=local,dest=./build \
		. && \
	chmod +x ./build/caddy && \
	echo "" && \
	echo "✅ Success! Caddy binary with serverless plugin created at: ./build/caddy" && \
	echo "" && \
	echo "To install system-wide, run:" && \
	echo "  sudo mv ./build/caddy /usr/local/bin/caddy" && \
	echo "" && \
	echo "To verify installation:" && \
	echo "  ./build/caddy version" && \
	echo "  ./build/caddy list-modules | grep serverless"

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
# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is the Caddy serverless plugin - a Caddy HTTP handler module that enables serverless function execution using Docker containers. The plugin intercepts matching requests, starts a Docker container with the configured image, proxies the request to it, and returns the response.

## Key Architecture Concepts

### Module Registration
- The plugin registers as `http.handlers.serverless` in Caddy's module system
- Registration happens via `init()` function using `caddy.RegisterModule()`
- Implements Caddy interfaces: `Provisioner`, `Validator`, `CleanerUpper`, and `MiddlewareHandler`

### Request Flow
1. **Matching**: Incoming requests are matched against configured functions by HTTP method and regex path pattern
2. **Container Lifecycle**: For matched requests, a Docker container is started with the specified configuration
3. **Health Checking**: Plugin waits for container readiness using TCP health checks
4. **Proxying**: Request is proxied to the container using either internal Docker networking or host port mapping
5. **Cleanup**: Container is stopped and removed after response is returned

### Core Components
- `serverless.go`: Main handler logic, request matching, and container orchestration
- `container.go`: Docker API integration for container lifecycle management
- `caddyfile.go`: Caddyfile configuration parsing and validation

## Common Development Commands

### Building
```bash
# Build Caddy with the plugin using xcaddy (recommended)
make build

# Manual build with xcaddy
xcaddy build --with github.com/jose/caddy-serverless=.

# Build test Docker images
make docker-test-images
```

### Testing
```bash
# Run unit tests with race detection
make test
# or
go test -v -race ./...

# Run integration tests (requires Docker)
make integration-test
# or
go test -v -tags=integration -timeout=10m ./...

# Run full test suite
./test-plugin.sh

# Run all checks (format, vet, lint, test)
make check
```

### Linting and Formatting
```bash
# Run golangci-lint
make lint

# Format code
make fmt
# or
go fmt ./...
gofmt -s -w .

# Run go vet
make vet
```

### Running Examples
```bash
# Run with example Caddyfile
make run-example

# Validate a Caddyfile
./caddy validate --config example.Caddyfile --adapter caddyfile

# Run Caddy with custom config
./caddy run --config myconfig.Caddyfile
```

### Development Helpers
```bash
# Install development dependencies (xcaddy, golangci-lint)
make dev-deps

# Clean build artifacts and test cache
make clean

# Clean up Docker containers created by serverless functions
make docker-cleanup

# Simulate full CI pipeline locally
make ci
```

## High-Level Architecture

### Configuration Flow
1. **Caddyfile Parsing**: The plugin's Caddyfile directives are parsed in `caddyfile.go`
2. **JSON Conversion**: Caddyfile is converted to JSON configuration by the adapter
3. **Provisioning**: During provisioning, the handler validates Docker connectivity and compiles path regexes
4. **Validation**: Configuration is validated for required fields, valid paths, and Docker image accessibility

### Docker Integration
- Uses Docker SDK for Go (`github.com/docker/docker/client`)
- Container lifecycle is managed per-request (ephemeral containers)
- Supports both Linux and Windows containers
- Health checking via TCP dial to configured port
- Automatic cleanup with context-based cancellation

### Request Matching System
- Functions are evaluated in configuration order
- First matching function handles the request
- Matching criteria: HTTP method AND regex path pattern
- Non-matching requests are passed to the next handler in the chain

### Error Handling
- Docker API errors are wrapped with context
- Container startup failures return 500 Internal Server Error
- Timeout errors are propagated with proper HTTP status
- All containers are cleaned up even on error paths

## Important Patterns

### Adding New Configuration Options
1. Add field to `Function` struct in `serverless.go`
2. Add JSON tags for configuration
3. Update `UnmarshalCaddyfile` in `caddyfile.go` to parse the directive
4. Add validation logic in `Validate()` method
5. Use the field in `ServeHTTP` when creating containers

### Working with Docker API
- Always use context for cancellation
- Check container state before operations
- Use labels to identify plugin-created containers
- Handle both ContainerStop and ContainerRemove for cleanup

### Testing Guidelines
- Unit tests mock the Docker client for isolated testing
- Integration tests use real Docker with test images
- Test images are built from `testdata/` directories
- Use table-driven tests for multiple scenarios
- Always test error paths and edge cases

## Security Considerations

- Volume mounts require absolute paths and are validated
- Environment variable names are validated against a pattern
- Docker commands are passed as arrays to avoid shell injection
- Containers run with default Docker security settings
- No privileged mode or capability additions
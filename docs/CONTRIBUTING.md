# Contributing to Caddy Serverless Plugin

Thank you for your interest in contributing to the Caddy Serverless Plugin! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to abide by the [Caddy Code of Conduct](https://github.com/caddyserver/caddy/blob/master/.github/CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Issues

Before creating an issue, please:

1. Check the [existing issues](https://github.com/jose/caddy-serverless/issues) to avoid duplicates
2. Include the version of Caddy and the plugin you're using
3. Provide clear steps to reproduce the issue
4. Include relevant logs and error messages

### Suggesting Features

1. Open an issue with the "enhancement" label
2. Clearly describe the feature and its use case
3. Provide examples of how the feature would work
4. Be open to discussion and feedback

### Submitting Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Write clear commit messages** that explain the changes
3. **Follow the code style** of the project (use `gofmt` and `golangci-lint`)
4. **Add tests** for new functionality
5. **Update documentation** if needed
6. **Ensure all tests pass** before submitting

#### Pull Request Process

1. Fork the repo and create your branch:
   ```bash
   git checkout -b feature/my-feature
   ```

2. Make your changes and commit:
   ```bash
   git add .
   git commit -m "feat: add new feature"
   ```

3. Push to your fork:
   ```bash
   git push origin feature/my-feature
   ```

4. Open a Pull Request with a clear title and description

## Development Setup

### Prerequisites

- Go 1.21 or later
- Docker (for running serverless functions)
- xcaddy (for building Caddy with the plugin)
- golangci-lint (for code linting)

### Building

```bash
# Clone your fork
git clone https://github.com/your-username/caddy-serverless.git
cd caddy-serverless

# Install dependencies
go mod download

# Build Caddy with the plugin
make build

# Run tests
make test

# Run integration tests
make integration-test

# Run linter
make lint
```

### Testing

Always write tests for new functionality:

1. **Unit tests** - Test individual functions and methods
2. **Integration tests** - Test the plugin with real Docker containers

Example test structure:

```go
func TestFunctionName(t *testing.T) {
    // Arrange
    handler := &ServerlessHandler{
        // setup
    }
    
    // Act
    result := handler.SomeMethod()
    
    // Assert
    if result != expected {
        t.Errorf("expected %v, got %v", expected, result)
    }
}
```

### Code Style

- Use `gofmt` to format your code
- Follow Go best practices and idioms
- Keep functions small and focused
- Write clear, self-documenting code
- Add comments for complex logic

### Commit Messages

Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation changes
- `style:` - Code style changes (formatting, etc.)
- `refactor:` - Code refactoring
- `test:` - Adding or updating tests
- `chore:` - Maintenance tasks

Examples:
```
feat: add support for custom networks
fix: handle container timeout correctly
docs: update configuration examples
test: add integration tests for volumes
```

## Release Process

Releases are managed by maintainers:

1. Update version numbers
2. Update CHANGELOG.md
3. Create a new tag
4. GitHub Actions will build and publish

## Getting Help

- Open an issue for bugs or feature requests
- Join the [Caddy Community](https://caddy.community) forum
- Check the [documentation](README.md)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
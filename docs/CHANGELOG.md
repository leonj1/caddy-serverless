# Changelog

All notable changes to the Caddy Serverless Plugin will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2024-01-16

### Added
- Initial release of the Caddy Serverless Plugin
- Support for executing serverless functions in Docker containers
- HTTP method matching (GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS)
- Regex-based URL path matching
- Docker container lifecycle management
- Environment variable configuration
- Volume mount support
- Configurable timeouts and ports
- Automatic container cleanup after execution
- Caddyfile directive support
- JSON configuration support
- Integration tests with Go and Python examples
- Comprehensive documentation and examples

### Changed
- Extracted from Caddy core into standalone plugin
- Updated module path to `github.com/jose/caddy-serverless`
- Improved error handling and logging

### Technical Details
- Compatible with Caddy 2.8.4+
- Requires Go 1.21+
- Requires Docker for serverless function execution

[Unreleased]: https://github.com/jose/caddy-serverless/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/jose/caddy-serverless/releases/tag/v0.1.0
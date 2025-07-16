# Release Notes - v0.1.0

## 🎉 Initial Release

We're excited to announce the first release of the Caddy Serverless Plugin! This plugin enables Caddy to execute serverless functions using Docker containers, bringing FaaS capabilities to your Caddy server.

## ✨ Features

- **Docker-based Function Execution**: Run any containerized application as a serverless function
- **Flexible Routing**: Use regex patterns and HTTP method matching to route requests
- **Container Management**: Automatic container lifecycle management with cleanup
- **Configuration Options**: 
  - Environment variables
  - Volume mounts
  - Custom commands
  - Timeout settings
  - Port configuration
- **Multiple Language Support**: Works with any language that can run in a container

## 📦 Installation

Build Caddy with the serverless plugin using xcaddy:

```bash
xcaddy build --with github.com/jose/caddy-serverless
```

## 🚀 Quick Start

```caddyfile
{
    order serverless before file_server
}

localhost:8080 {
    serverless {
        function {
            methods GET POST
            path /hello
            image alpine:latest
            command /bin/sh -c "echo 'Hello from serverless!' | nc -l -p 8080"
            port 8080
        }
    }
}
```

## 📖 Migration from Built-in Module

If you were using the serverless module when it was part of Caddy core:

1. Remove the old Caddy binary
2. Build a new Caddy binary with: `xcaddy build --with github.com/jose/caddy-serverless`
3. Your existing Caddyfile configurations should work without changes

## 📋 Requirements

- Caddy 2.8.4 or later
- Go 1.21 or later (for building)
- Docker (for running serverless functions)

## 🐛 Known Issues

- Container startup time adds latency to first request
- Each request creates a new container (no warm containers yet)

## 🙏 Acknowledgments

This plugin was originally developed as part of the Caddy Web Server project. Special thanks to the Caddy community for their support.

## 📚 Documentation

For detailed documentation and examples, see the [README](https://github.com/jose/caddy-serverless/blob/main/README.md).

## 💬 Feedback

Please report issues and feature requests on our [GitHub repository](https://github.com/jose/caddy-serverless/issues).
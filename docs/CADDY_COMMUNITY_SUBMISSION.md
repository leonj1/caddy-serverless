# Caddy Community Plugin Submission

## Plugin Information

**Name**: Caddy Serverless Plugin  
**Repository**: https://github.com/leonj1/caddy-serverless  
**Documentation**: https://github.com/leonj1/caddy-serverless/blob/main/README.md  
**Category**: Handler  
**Module ID**: `http.handlers.serverless`  

## Description

The Caddy Serverless Plugin enables Caddy to execute serverless functions using Docker containers. It provides a simple way to run containerized applications as HTTP endpoints without managing long-running services.

## Key Features

- Execute any Docker container as a serverless function
- Automatic container lifecycle management
- Support for environment variables and volume mounts
- Regex-based path matching and HTTP method filtering
- Configurable timeouts and ports
- Zero-downtime deployments

## Example Usage

```caddyfile
example.com {
    serverless {
        function {
            methods GET POST
            path /api/.*
            image my-api:latest
            env DATABASE_URL=postgres://localhost/db
            timeout 30s
        }
    }
}
```

## Installation

```bash
xcaddy build --with github.com/leonj1/caddy-serverless
```

## Requirements

- Caddy 2.8.4+
- Docker
- Go 1.21+ (for building)

## Use Cases

- Microservices with independent scaling
- Development environments
- Webhook handlers
- Batch processing endpoints
- Multi-language applications

## Maintainer

- GitHub: [@leonj1](https://github.com/leonj1)
- Email: [leonj1@gmail.com]

## License

Apache License 2.0

## Notes

This plugin was originally part of Caddy core and has been extracted as a standalone plugin for better modularity and maintenance.

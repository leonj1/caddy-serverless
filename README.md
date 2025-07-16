# Caddy Serverless Functions Plugin

A Caddy plugin that enables serverless function execution using Docker containers. When a request matches a configured route, the plugin starts a Docker container, proxies the request to it, and returns the response.

## Installation

### Using xcaddy (recommended)

The easiest way to use this plugin is to build a custom Caddy binary with [xcaddy](https://github.com/caddyserver/xcaddy):

```bash
xcaddy build --with github.com/jose/caddy-serverless
```

### Building from source

```bash
git clone https://github.com/jose/caddy-serverless.git
cd caddy-serverless
make build
```

### Docker

You can also use a pre-built Docker image or build your own:

```dockerfile
FROM caddy:builder AS builder

RUN xcaddy build \
    --with github.com/jose/caddy-serverless

FROM caddy:latest

COPY --from=builder /usr/bin/caddy /usr/bin/caddy
```

## Features

- **HTTP Method Matching**: Support for GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS
- **Path Pattern Matching**: Regex-based URL path matching
- **Docker Integration**: Automatic container lifecycle management
- **Environment Variables**: Pass custom environment variables to containers
- **Volume Mounts**: Mount host directories into containers
- **Configurable Timeouts**: Set execution timeouts for functions
- **Port Configuration**: Specify container listening ports
- **Automatic Cleanup**: Containers are automatically stopped after execution

## Quick Start

1. Build Caddy with the serverless plugin:
   ```bash
   xcaddy build --with github.com/jose/caddy-serverless
   ```

2. Create a `Caddyfile`:
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

3. Run Caddy:
   ```bash
   ./caddy run
   ```

4. Test the endpoint:
   ```bash
   curl http://localhost:8080/hello
   ```

## Configuration

### JSON Configuration

```json
{
  "handler": "serverless",
  "functions": [
    {
      "methods": ["GET", "POST"],
      "path": "/api/users/.*",
      "image": "my-function:latest",
      "command": ["/app/handler"],
      "environment": {
        "DATABASE_URL": "postgres://localhost/mydb",
        "API_KEY": "secret"
      },
      "volumes": [
        {
          "source": "/host/data",
          "target": "/app/data",
          "readonly": true
        }
      ],
      "timeout": "30s",
      "port": 8080
    }
  ]
}
```

### Caddyfile Configuration

```caddyfile
example.com {
    serverless {
        function {
            methods GET POST
            path /api/.*
            image nginx:latest
            command /bin/sh -c "echo hello"
            env KEY=value
            env ANOTHER=test
            volume /host:/container
            volume /host2:/container2:ro
            timeout 30s
            port 8080
        }
    }
}
```

## Configuration Options

### Function Configuration

- **methods** (required): Array of HTTP methods this function handles
- **path** (required): Regex pattern for URL path matching
- **image** (required): Docker image to run
- **command** (optional): Command to execute in the container
- **environment** (optional): Environment variables to pass to the container. In the Caddyfile, use multiple `env` lines for multiple variables.
- **volumes** (optional): Volume mounts for the container
- **timeout** (optional): Maximum execution time (default: 30s)
- **port** (optional): Port the container listens on (default: 8080)

### Volume Mount Configuration

- **source** (required): Absolute path on the host
- **target** (required): Absolute path in the container
- **readonly** (optional): Whether the mount is read-only (default: false)

## How It Works

1. **Request Matching**: When a request arrives, the plugin checks if it matches any configured function based on HTTP method and URL path
2. **Container Startup**: If a match is found, a new Docker container is started with the specified configuration
3. **Health Check**: The plugin waits for the container to be ready to accept connections
4. **Request Proxying**: The original HTTP request is proxied to the container
5. **Response Handling**: The container's response is returned to the client
6. **Cleanup**: The container is automatically stopped and removed

## Example Use Cases

### Simple API Function

```caddyfile
api.example.com {
    serverless {
        function {
            methods GET POST
            path /hello
            image nginx:alpine
            command /bin/sh -c "echo 'HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nHello World' | nc -l -p 8080"
            port 8080
        }
    }
}
```

### Python Flask Application

```caddyfile
app.example.com {
    serverless {
        function {
            methods GET POST PUT DELETE
            path /.*
            image python:3.9-slim
            command python -c "
                from flask import Flask
                app = Flask(__name__)
                @app.route('/', defaults={'path': ''})
                @app.route('/<path:path>')
                def catch_all(path):
                    return f'Hello from {path}'
                app.run(host='0.0.0.0', port=8080)
            "
            env FLASK_ENV=production
            timeout 60s
            port 8080
        }
    }
}
```

### Node.js Function with Volume Mount

```caddyfile
node.example.com {
    serverless {
        function {
            methods GET
            path /files/.*
            image node:16-alpine
            command node -e "
                const http = require('http');
                const fs = require('fs');
                const server = http.createServer((req, res) => {
                    const files = fs.readdirSync('/data');
                    res.writeHead(200, {'Content-Type': 'application/json'});
                    res.end(JSON.stringify(files));
                });
                server.listen(8080, '0.0.0.0');
            "
            volume /host/files:/data:ro
            port 8080
        }
    }
}
```

### Advanced Rate Limiting Example

This example demonstrates how to implement different rate limits for GET and POST/PUT/PATCH/DELETE requests to an API, in conjunction with the serverless plugin. It also shows how to return a custom error message for rate-limited requests.

```caddyfile
example.com {
    # Different rate limits for different endpoints
    @get_requests {
        path /api/*
        method GET
    }
    rate_limit @get_requests 100r/m

    @post_requests {
        path /api/*
        method POST PUT PATCH DELETE
    }
    rate_limit @post_requests 20r/m

    # Serverless configuration
    serverless {
        function {
            methods GET POST PUT DELETE
            path /api/users/.*
            image my-function:latest
            timeout 30s
        }
    }

    # Return 429 Too Many Requests with a custom error message
    handle_errors {
        @rate_limited {
            expression {http.error.status_code} == 429
        }
        respond @rate_limited 429 {
            body "Rate limit exceeded. Please try again later."
        }
    }
}
```

## Requirements

- Docker must be installed and accessible via the `docker` command
- The Caddy process must have permission to execute Docker commands
- Docker images must be available locally or pullable from a registry

## Security Considerations

- Containers run with default Docker security settings
- Volume mounts should use absolute paths and appropriate permissions
- Consider using read-only mounts when possible
- Environment variables may contain sensitive data - handle with care
- Network isolation depends on Docker configuration

## Troubleshooting

### Container Fails to Start
- Check if the Docker image exists and is accessible
- Verify Docker daemon is running
- Check Caddy logs for detailed error messages

### Container Not Ready
- Ensure the container listens on the configured port
- Check if the application starts quickly enough within the timeout
- Verify the container doesn't exit immediately

### Request Timeout
- Increase the timeout value if functions need more time
- Check container logs for application errors
- Verify the container is responding on the correct port

## Performance Notes

- Each request starts a new container, which has overhead
- Consider container startup time when setting timeouts
- Use lightweight base images for faster startup
- Pre-built images start faster than those requiring compilation

## Development

### Prerequisites

- Go 1.21 or later
- Docker
- xcaddy (install with `go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest`)

### Building

```bash
# Clone the repository
git clone https://github.com/jose/caddy-serverless.git
cd caddy-serverless

# Run tests
make test

# Run integration tests (requires Docker)
make integration-test

# Build Caddy with the plugin
make build

# Run with example configuration
make run-example
```

### Testing

The plugin includes both unit tests and integration tests:

```bash
# Unit tests
go test -v ./...

# Integration tests with Docker
go test -v -tags=integration ./...

# Lint the code
make lint
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Originally developed as part of the [Caddy Web Server](https://github.com/caddyserver/caddy) project
- Thanks to the Caddy community for their support and feedback

# Foreman

A single-binary, cross-platform local services monitor and manager. Start, stop, restart, build, and monitor all your development services from one web dashboard.

## Quick Start

```bash
# Build
cd tools/foreman
GO111MODULE=on go build -o bin/foreman ./cmd/foreman

# Create a config file
cp foreman.example.yaml foreman.yaml
# Edit foreman.yaml to define your services

# Run
./bin/foreman -c foreman.yaml
# Open http://127.0.0.1:9090 in your browser
```

## Features

- **Single binary** — Go binary with embedded web UI, no runtime dependencies
- **Real-time logs** — WebSocket-based live log streaming per service
- **Interactive stdin** — Send input to running processes from the web UI
- **Docker Compose** — Auto-discovers services from compose files
- **Build integration** — Per-service build commands with output in log viewer
- **Config reload** — Hot-reload `foreman.yaml` without stopping running services
- **Authenticated** — Password-protected web UI and API token support
- **Environment inspector** — View resolved environment variables per service

## Configuration

See [foreman.example.yaml](foreman.example.yaml) for a complete example.

```yaml
project_root: .
password: "${FOREMAN_PASSWORD:-admin}"
port: 9090

services:
  my-api:
    command: ./bin/api-server
    args: ["--port", "8080"]
    working_dir: services/api
    auto_start: true
    build:
      command: go
      args: ["build", "-o", "bin/api-server", "."]
    health_check:
      url: http://localhost:8080/health

  docker-stack:
    type: docker-compose
    compose_file: docker-compose.yml
    auto_start: true
```

## CLI Usage

```bash
foreman -c foreman.yaml          # Start with config file
foreman -config path/to/config   # Long flag form
```

## API

All API endpoints require authentication via cookie (after login) or Bearer token.

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/login` | Login with password |
| GET | `/api/services` | List all services |
| POST | `/api/service/{id}/start` | Start a service |
| POST | `/api/service/{id}/stop` | Stop a service |
| POST | `/api/service/{id}/restart` | Restart a service |
| POST | `/api/service/{id}/build` | Build a service |
| GET | `/api/service/{id}/logs` | Get recent logs |
| GET | `/api/service/{id}/env` | Get environment variables |
| POST | `/api/config/reload` | Reload configuration |
| GET | `/api/health` | Health check |

## Documentation

- [Architecture](docs/architecture.md) — System design and component overview
- [Development](docs/development.md) — How to build and develop Foreman

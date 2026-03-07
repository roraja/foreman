# Foreman

A single-binary, cross-platform local services monitor and manager. Start, stop, restart, build, and monitor all your development services from one web dashboard. Define reusable commands, compose configs from multiple YAML files, and run tasks from the CLI or web UI.

## Install

**One-liner** (downloads to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/roraja/foreman/master/install.sh | sh
```

Or specify a version or install directory:

```bash
FOREMAN_VERSION=v0.0.3 INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/roraja/foreman/master/install.sh | sh
```

**From source:**

```bash
go build -o bin/foreman ./cmd/foreman
```

## Install Binary and Boot Service

Download the latest foreman, install it to your PATH, and set it up as a systemd service that starts on boot:

```bash
# Download and install the binary
curl -fsSL https://raw.githubusercontent.com/roraja/foreman/main/install.sh | INSTALL_DIR=/usr/local/bin sh

# Install foreman as a user systemd service (starts on boot)
foreman install
```

This copies the binary to `~/.foreman/foreman`, creates a default config at `~/.foreman/foreman.yaml`, and enables a systemd user service. Once running, register any project to start on boot:

```bash
cd /path/to/my-project
foreman runOnBoot
```

This adds the project as a service in the global `~/.foreman/foreman.yaml` so the boot-level foreman starts it automatically.

## Quick Start

```bash
# Create a config file
cp foreman.example.yaml foreman.yaml
# Edit foreman.yaml to define your services and commands

# Start the dashboard
foreman -c foreman.yaml
# Open http://127.0.0.1:9090 in your browser

# List all commands
foreman commands -c foreman.yaml

# Run a command
foreman run install -c foreman.yaml
```

## Features

- **Single binary** — Go binary with embedded web UI, no runtime dependencies
- **Commands** — Reusable, composable command definitions with dependencies, parallel execution, and timeouts
- **Composable config** — Import and merge multiple YAML files via `imports`
- **Real-time logs** — WebSocket-based live log streaming per service and command
- **Interactive stdin** — Send input to running processes from the web UI
- **Docker Compose** — Auto-discovers services from compose files
- **Build integration** — Per-service build commands with output in log viewer
- **Cross-platform** — Platform-specific command overrides for Linux, macOS, and Windows
- **Config reload** — Hot-reload `foreman.yaml` without stopping running services
- **Authenticated** — Password-protected web UI and API token support
- **Environment inspector** — View resolved environment variables per service

## Configuration

See [foreman.example.yaml](foreman.example.yaml) for a complete example, or explore the [example-repo/](example-repo/) for a full project setup with mock binaries.

### Services

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

  docker-stack:
    type: docker-compose
    compose_file: docker-compose.yml
    auto_start: true
```

### Commands

Commands are reusable, one-shot task definitions. They support dependencies, parallel execution, timeouts, and cross-platform overrides.

```yaml
commands:
  install:
    label: "Install Dependencies"
    description: "Install all project dependencies"
    group: setup
    tags: [deps, install]
    run: "npm install"
    timeout: "2m"

  build-api:
    label: "Build API"
    group: build
    command: go
    args: ["build", "-o", "./bin/api", "./cmd/api"]
    working_dir: ./backend

  build-frontend:
    label: "Build Frontend"
    group: build
    run: "npm run build"
    working_dir: ./frontend
    depends_on: [install]

  build-all:
    label: "Build Everything"
    group: build
    run: "echo 'All builds complete'"
    parallel: [build-api, build-frontend]

  lint:
    label: "Lint"
    group: quality
    run: "npm run lint"
    ignore_errors: true

  db-reset:
    label: "Reset Database"
    group: database
    run: "npx prisma migrate reset --force"
    confirm: true
```

#### Command fields

| Field | Type | Description |
|-------|------|-------------|
| `label` | string | Human-readable name (defaults to command ID) |
| `description` | string | What the command does (shown in UI/search) |
| `group` | string | Category for grouping |
| `tags` | list | Tags for search/filtering |
| `run` | string | Shell command (implies `shell: true`) |
| `command` | string | Executable path (mutually exclusive with `run`) |
| `args` | list | Arguments to pass |
| `shell` | bool | Wrap in platform shell (default: false) |
| `env` | map | Inline environment variables |
| `env_file` | string | Path to `.env` file |
| `working_dir` | string | Working directory (relative to `project_root`) |
| `platform` | map | OS-specific overrides (`linux`, `darwin`, `windows`) |
| `depends_on` | list | Commands to run sequentially before this one |
| `parallel` | list | Commands to run in parallel before this one |
| `timeout` | string | Kill if exceeded (e.g. `"30s"`, `"5m"`) |
| `ignore_errors` | bool | Continue even if exit code ≠ 0 |
| `confirm` | bool | Prompt before running |
| `interactive` | bool | Needs stdin |

#### Service → Command reference

Services can reference commands instead of inlining `command`/`args`:

```yaml
commands:
  run-api:
    command: go
    args: ["run", "./cmd/api"]
    env:
      GIN_MODE: debug

services:
  api:
    uses: run-api            # reuse the command definition
    auto_start: true
    env:
      PORT: "8080"           # merged into run-api's env
    build:
      uses: build-api        # build step also references a command
```

### Composable Config (Imports)

Split your configuration across multiple YAML files using `imports`. Imported commands and services are merged into the main config, with the base file taking precedence for duplicate IDs.

```yaml
# foreman.yaml
imports:
  - db-commands.yaml         # relative path
  - quality-commands.yaml
  - shared/services.yaml     # nested paths work too

commands:
  install:
    run: "npm install"
```

```yaml
# db-commands.yaml
commands:
  db-migrate:
    label: "Run Migrations"
    run: "npx prisma migrate deploy"
    depends_on: [install]     # can reference commands from the parent
```

Imports support:
- **Relative paths** resolved from the importing file's directory
- **Nested imports** (imported files can import other files)
- **Circular import detection** with automatic error reporting
- **Depth limiting** (max 10 levels) to prevent runaway imports
- **Commands and services** from imported files are merged

### Cross-Platform Commands

```yaml
commands:
  format:
    label: "Format Code"
    platform:
      linux:
        run: "gofmt -w . && npx prettier --write ."
      darwin:
        run: "gofmt -w . && npx prettier --write ."
      windows:
        run: "gofmt -w . && npx prettier --write ."
```

At runtime, Foreman checks `runtime.GOOS` and merges the matching platform block over the base definition.

### Environment Variables

Foreman provides several ways to inject environment variables into services:

**1. Root-level `.env` file** — shared across all services:

```yaml
env_file: .env
services:
  my-api:
    command: ./bin/api-server
```

**2. Per-service `.env` file** — overrides values from the root env file:

```yaml
services:
  my-api:
    command: ./bin/api-server
    env_file: services/api/.env
```

**3. Inline `env` map** — highest priority, overrides both env files:

```yaml
services:
  my-api:
    command: ./bin/api-server
    env_file: .env
    env:
      PORT: "8080"
      LOG_LEVEL: debug
      DATABASE_URL: "postgres://localhost:5432/mydb"
```

**4. YAML-level variable interpolation** — reference host environment variables anywhere in the config using `${VAR}` or `${VAR:-default}`:

```yaml
password: "${FOREMAN_PASSWORD:-admin}"
services:
  my-api:
    command: ./bin/api-server
    env:
      API_KEY: "${API_KEY}"
      NODE_ENV: "${NODE_ENV:-development}"
```

**Priority order** (highest wins): inline `env` > per-service `env_file` > root `env_file`.

When a service uses `uses` to reference a command, environment variables are merged:
```
command env → platform env → service env
```

The `.env` file format supports `KEY=value` pairs, blank lines, `#` comments, and optional quoting:

```env
# Database
DATABASE_URL="postgres://localhost:5432/mydb"
REDIS_URL=redis://localhost:6379

# App
LOG_LEVEL=debug
SECRET_KEY='s3cret'
```

## CLI Usage

```bash
# Start the web dashboard (default mode)
foreman -c foreman.yaml
foreman -config path/to/config

# List all commands
foreman commands -c foreman.yaml
foreman commands -c foreman.yaml -group build     # filter by group
foreman commands -c foreman.yaml -tag ci           # filter by tag
foreman commands -c foreman.yaml -q "database"     # search

# Run a command
foreman run install -c foreman.yaml
foreman run build-api -c foreman.yaml --dry-run    # show what would run
foreman run install db-migrate db-seed -c foreman.yaml  # sequential
foreman run lint test --parallel -c foreman.yaml        # parallel
foreman run build -c foreman.yaml --env NODE_ENV=production
foreman run build -c foreman.yaml -- --extra-flag       # extra args
```

## API

All API endpoints require authentication via cookie (after login) or Bearer token.

### Service Endpoints

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

### Command Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/commands` | List all commands |
| GET | `/api/commands?q=search` | Search commands |
| GET | `/api/commands?group=build` | Filter by group |
| GET | `/api/commands?tag=ci` | Filter by tag |
| POST | `/api/command/{id}/run` | Run a command (body: `{"env": {}, "args": []}`) |
| GET | `/api/command/{id}/status` | Get command status |
| GET | `/api/command/{id}/logs?lines=100` | Get command output |
| POST | `/api/command/{id}/cancel` | Cancel a running command |

### WebSocket Endpoints

| Endpoint | Description |
|----------|-------------|
| `WS /ws/logs/{service-id}` | Stream service logs in real time |
| `WS /ws/stdin/{service-id}` | Send stdin to a service |
| `WS /ws/command/{command-id}` | Stream command output in real time |

## Documentation

- [Architecture](docs/architecture.md) — System design and component overview
- [Development](docs/development.md) — How to build and develop Foreman
- [Commands Spec](docs/next/commands.md) — Full commands feature specification

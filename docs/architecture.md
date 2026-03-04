# Foreman Architecture

## Overview

Foreman is a single-binary Go application that manages local development services. It embeds a web UI (inline HTML/JS) and exposes a REST API + WebSocket endpoints for real-time service management.

## Architecture Diagram

```mermaid
graph TB
    subgraph "Single Go Binary"
        M[main.go<br/>CLI entry point]
        
        subgraph "Config Layer"
            CL[config/loader.go<br/>YAML parsing + env interpolation]
            CE[config/env.go<br/>.env file parser]
        end
        
        subgraph "Service Management"
            O[orchestrator/orchestrator.go<br/>Lifecycle coordinator]
            PM[process/manager.go<br/>Native process wrapper]
            DM[docker/manager.go<br/>Docker Compose integration]
        end
        
        subgraph "HTTP Layer"
            S[server/api.go<br/>REST API handlers]
            WS[server/websocket.go<br/>Log streaming + stdin]
            FE[server/frontend.go<br/>Inline web UI]
        end
        
        subgraph "Shared Types"
            T[types/service.go<br/>ServiceInfo, LogEntry, LogBuffer]
        end
    end
    
    subgraph "External"
        B[Browser<br/>Web Dashboard]
        DC[Docker Compose<br/>CLI]
        P[Native Processes<br/>stdout/stderr/stdin]
    end
    
    M --> CL
    CL --> CE
    M --> O
    O --> PM
    O --> DM
    M --> S
    S --> O
    WS --> O
    
    B -->|HTTP/WS| S
    B -->|WebSocket| WS
    DM -->|exec| DC
    PM -->|exec| P
    
    PM --> T
    DM --> T
    O --> T
    S --> T
```

## Data Flow

### Log Streaming

```mermaid
sequenceDiagram
    participant P as Process (stdout/stderr)
    participant RB as Ring Buffer
    participant BC as Broadcast Channel
    participant WS1 as WebSocket Client 1
    participant WS2 as WebSocket Client 2
    
    P->>RB: Log line
    P->>BC: Log line
    BC->>WS1: JSON message
    BC->>WS2: JSON message
```

### Service Lifecycle

```mermaid
stateDiagram-v2
    [*] --> stopped
    stopped --> building: build()
    building --> stopped: build success
    building --> crashed: build failure
    stopped --> starting: start()
    starting --> running: process started
    starting --> crashed: start failure
    running --> stopping: stop()
    stopping --> stopped: graceful shutdown
    running --> crashed: process exit
    crashed --> starting: start()/restart()
    crashed --> building: build()
```

### Config Reload

```mermaid
sequenceDiagram
    participant UI as Web UI
    participant API as REST API
    participant O as Orchestrator
    participant C as Config Loader
    
    UI->>API: POST /api/config/reload
    API->>O: ReloadConfig()
    O->>C: Load(configPath)
    C-->>O: New config
    O->>O: Diff services (added/removed)
    O->>O: Add new services
    O->>O: Update existing configs
    O-->>API: {added, removed}
    API-->>UI: JSON response
```

## Folder Structure

```
tools/foreman/
├── cmd/
│   └── foreman/
│       └── main.go              # Entry point, CLI flags, signal handling
├── internal/
│   ├── config/
│   │   ├── loader.go            # YAML config parser, env var interpolation
│   │   └── env.go               # .env file parser
│   ├── orchestrator/
│   │   └── orchestrator.go      # Service lifecycle coordinator
│   ├── process/
│   │   └── manager.go           # Native process management (start/stop/stdin/logs)
│   ├── docker/
│   │   └── manager.go           # Docker Compose integration + auto-discovery
│   ├── server/
│   │   ├── api.go               # REST API routes and handlers
│   │   ├── websocket.go         # WebSocket handlers (logs + stdin)
│   │   └── frontend.go          # Inline HTML/JS web dashboard
│   └── types/
│       └── service.go           # Shared types (ServiceInfo, LogEntry, LogBuffer)
├── frontend/
│   └── dist/
│       └── index.html           # Placeholder (for go:embed when building with React)
├── docs/
│   ├── architecture.md          # This file
│   └── development.md           # Development guide
├── .vscode/
│   └── commands.sh              # Build and dev commands
├── bin/                         # Build output (gitignored)
├── foreman.example.yaml         # Example configuration
├── go.mod                       # Go module definition
├── go.sum                       # Go dependency checksums
└── README.md                    # Project README
```

## Component Details

### Config Layer (`internal/config/`)

- **loader.go**: Parses `foreman.yaml`, interpolates `${ENV_VAR:-default}` patterns, resolves relative paths, merges environment variables (root env → service env_file → inline env), resolves build config inheritance
- **env.go**: Parses `.env` files (KEY=value format, supports comments and quoted values)

### Process Manager (`internal/process/`)

- **manager.go**: Wraps `os/exec.Cmd` with:
  - stdout/stderr capture via goroutines → ring buffer + broadcast
  - stdin pipe for interactive input from web UI
  - Process group management (`Setpgid: true`) for clean signal delivery
  - SIGTERM → wait 10s → SIGKILL shutdown sequence
  - Build command execution with log output

### Docker Manager (`internal/docker/`)

- **manager.go**: Wraps `docker compose` CLI:
  - Auto-discovers services via `docker compose config --services`
  - Maps Docker container states to Foreman status enum
  - Streams logs per sub-service via `docker compose logs -f`
  - Supports individual service start/stop/restart

### Orchestrator (`internal/orchestrator/`)

- **orchestrator.go**: Top-level coordinator:
  - Routes actions to the correct manager (process vs docker)
  - Handles "parent/child" service ID notation for docker sub-services
  - Config reload: diffs services, adds new ones, updates existing configs
  - Auto-start: starts services with `auto_start: true` on launch

### HTTP Server (`internal/server/`)

- **api.go**: REST API with cookie + Bearer token auth
- **websocket.go**: WebSocket handlers using `golang.org/x/net/websocket`
- **frontend.go**: Serves inline HTML/JS dashboard (no external dependencies needed)

### Types (`internal/types/`)

- **service.go**: Thread-safe `LogBuffer` (ring buffer), `ServiceInfo`, `LogEntry`, status enums

## Technology Choices

| Choice | Rationale |
|--------|-----------|
| `net/http` (stdlib) | No external router dependency needed for this scale |
| `golang.org/x/net/websocket` | Lightweight WebSocket support without gorilla dependency |
| Inline HTML/JS | Zero frontend build step required; single binary with no external assets |
| `os/exec` for Docker | No Docker SDK dependency; works with any `docker compose` version |
| Ring buffer for logs | In-memory, bounded, fast; services handle their own persistent logging |

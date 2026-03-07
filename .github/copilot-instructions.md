# Copilot Instructions for Foreman

## Build, Test, and Lint

```bash
# Build
go build -o bin/foreman ./cmd/foreman

# Release build (static, stripped)
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/foreman ./cmd/foreman

# Run all tests
go test ./... -v

# Run tests for a single package
go test ./internal/config -v

# Run a single test by name
go test ./internal/config -v -run TestLoadBasicConfig

# Lint
go vet ./...

# Format
gofmt -w .
```

If the project is inside `$GOPATH`, prefix commands with `GO111MODULE=on`.

## Architecture

Foreman is a single-binary Go service manager with an embedded web dashboard (no separate frontend build). The entry point is `cmd/foreman/main.go` which provides subcommands: server (default), `commands`, `run`, and `install`.

### Package dependency flow

```
cmd/foreman (CLI + flag parsing)
  → internal/config     (YAML loading, env interpolation, import merging, validation)
  → internal/orchestrator (coordinates everything below)
      → internal/process   (native process lifecycle, stdin/stdout piping, process groups)
      → internal/docker    (shells out to `docker compose` CLI, no Docker SDK)
      → internal/command   (one-shot command runner: dependency resolution, parallel exec, timeouts)
  → internal/server     (REST API + WebSocket handlers + embedded web UI)
  → internal/types      (shared data structures: ServiceInfo, CommandInfo, LogEntry, status enums)
  → internal/logging    (file-based log writer with run numbering)
  → internal/binary     (GitHub release downloader for binary_source config)
```

### Web UI

The web UI is a single HTML/JS file embedded inline in `internal/server/frontend.go` (~900 lines). There is no npm/webpack build step — edit the Go file directly. The server tries `embed.FS` first and falls back to the inline HTML.

### External dependencies

Only two: `golang.org/x/net` (WebSocket) and `gopkg.in/yaml.v3`. The HTTP server uses stdlib `net/http.ServeMux` with no external router.

## Key Conventions

### Error handling

Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain. In async goroutines, log errors with `log.Printf` since there's no caller to return to.

### Concurrency

- Use `sync.RWMutex` to protect shared state (status, logs, subscribers). Name the field `mu`; use `subMu` for subscriber-specific locks.
- Broadcast log entries to subscribers via channels (`map[chan types.LogEntry]struct{}`). Use non-blocking sends with `select`/`default` to drop entries for slow consumers.
- Pass `context.Context` through command execution for cancellation and timeouts.

### Process management

- Unix: processes are started in their own process group (`Setpgid`), and the entire group is killed on stop. Platform-specific code lives in `proc_unix.go` / `proc_windows.go`.
- Shutdown sequence: SIGTERM → wait 10s → SIGKILL.

### Config system

- YAML config supports `${VAR}` and `${VAR:-default}` interpolation resolved from host environment.
- Imports are recursive (max 10 levels) with circular import detection. Base file wins on duplicate IDs.
- `run` (shell string) and `command` (executable path) are mutually exclusive in command definitions.
- Services can reference commands via `uses` field.
- Env priority: inline `env` > per-service `env_file` > root `env_file`.

### Naming

- Constructors: `NewProcess`, `NewRunner`, `NewLogBuffer`
- No `Get` prefix on getters: `Status()`, `Info()`, `Last(n)`
- Mutex fields: `mu`, `subMu`
- Short receiver names: `p *Process`, `r *Runner`, `o *Orchestrator`, `s *Server`

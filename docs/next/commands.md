# Commands: Config Language Extension

## Summary

Extend `foreman.yaml` with a top-level `commands` block — reusable, composable, cross-platform command definitions that can be run standalone or referenced by services.

---

## Motivation

Today, `foreman.yaml` only models **services** (long-running processes). Many projects also need:

- One-shot tasks (migrations, seed scripts, linting, code-gen)
- Build/setup steps that aren't tied to a single service
- Cross-platform command wrappers (different shells on Linux/macOS vs Windows)
- Reusable command definitions shared across services (e.g. the same build step used by multiple services)
- A searchable, browsable catalog of project commands in the web UI

Commands solve all of these while staying composable with the existing service model.

---

## YAML Schema

### Top-level `commands` block

```yaml
commands:
  <command-id>:
    label: "Human-readable name"          # optional, defaults to command-id
    description: "What this command does"  # optional, shown in UI/search
    group: <group-name>                    # optional, for categorization
    tags: [<tag>, ...]                     # optional, for search/filtering

    # --- Execution ---
    run: <shell-string>                    # simple form: single shell command
    # OR structured form:
    command: <executable>
    args: [<arg>, ...]
    shell: true | false                    # wrap in platform shell (default: false)

    # --- Environment ---
    env:                                   # inline env vars
      KEY: value
    env_file: <path>                       # load from .env file

    # --- Working directory ---
    working_dir: <path>                    # relative to project_root (default: project_root)

    # --- Cross-platform ---
    platform:                              # platform-specific overrides
      windows:
        run: "..."                         # or command/args
        shell: true
        env: { ... }
      darwin:
        run: "..."
      linux:
        run: "..."

    # --- Composition ---
    depends_on: [<command-id>, ...]        # run these commands first (sequentially)
    parallel: [<command-id>, ...]          # run these commands in parallel before this one

    # --- Behavior ---
    timeout: <duration>                    # e.g. "30s", "5m" — kill if exceeded
    ignore_errors: true | false            # continue even if exit code != 0 (default: false)
    confirm: true | false                  # prompt user before running (default: false, web UI shows dialog)
    interactive: true | false              # needs stdin (default: false)
```

### Service → Command reference

Services can reference a command instead of inlining `command`/`args`:

```yaml
services:
  my-api:
    label: "API Server"
    uses: <command-id>           # run this command as a long-running service
    auto_start: true
    url: "http://localhost:8080"

    # Service-level overrides (merged on top of the command's definition)
    env:
      PORT: "8080"
    args: ["--watch"]
```

Services can also reference commands for their build step:

```yaml
services:
  my-api:
    label: "API Server"
    command: ./bin/api
    build:
      uses: build-api            # reference a command for the build step
```

---

## Resolution & Merge Order

When a service uses `uses: <command-id>`, properties are resolved in this order (last wins):

1. Command definition (from `commands` block)
2. Platform-specific overrides from the command (matched to `runtime.GOOS`)
3. Service-level overrides (`env`, `args`, `working_dir` in the service block)

Environment variables merge (not replace) at each layer:

```
root env_file → command env_file → command inline env → platform env → service env_file → service inline env
```

---

## Cross-Platform Execution

| `shell` value | Linux / macOS | Windows |
|---|---|---|
| `false` (default) | Direct exec via `os/exec` | Direct exec via `os/exec` |
| `true` | Wraps in `sh -c "..."` | Wraps in `cmd /C "..."` |

When `run:` (string form) is used, `shell: true` is implied automatically.

The `platform:` block allows completely different commands per OS. At runtime, Foreman checks `runtime.GOOS` and merges the matching platform block over the base command definition.

---

## Composability

### Sequential dependencies

```yaml
commands:
  install-deps:
    run: "npm install"

  generate-types:
    run: "npm run codegen"
    depends_on: [install-deps]

  build-frontend:
    run: "npm run build"
    working_dir: ./frontend
    depends_on: [install-deps, generate-types]
```

Running `build-frontend` will first execute `install-deps`, then `generate-types`, then the build itself.

### Parallel pre-steps

```yaml
commands:
  lint:
    run: "npm run lint"

  typecheck:
    run: "npm run typecheck"

  verify:
    label: "Verify All"
    run: "echo 'All checks passed'"
    parallel: [lint, typecheck]       # these run concurrently before "verify"
```

### Service referencing a command

```yaml
commands:
  run-api:
    label: "Run API"
    command: go
    args: ["run", "./cmd/api"]
    env:
      GIN_MODE: debug
    working_dir: ./backend

services:
  api:
    label: "API Server"
    uses: run-api                      # reuse the command definition
    auto_start: true
    url: "http://localhost:8080"
    env:
      PORT: "8080"                     # merged into run-api's env
    build:
      uses: build-api                  # build step also references a command
```

### Build step referencing a command

```yaml
commands:
  build-api:
    label: "Build API"
    run: "go build -o ./bin/api ./cmd/api"
    working_dir: ./backend

services:
  api:
    label: "API Server"
    command: ./bin/api
    build:
      uses: build-api
```

---

## Full Example

```yaml
project_root: .
password: admin
port: 9090
host: 127.0.0.1
log_retention_lines: 5000

commands:
  install:
    label: "Install Dependencies"
    description: "Install all project dependencies"
    group: setup
    tags: [deps, install]
    run: "npm install"
    timeout: "2m"

  db-migrate:
    label: "Run Migrations"
    description: "Apply database migrations"
    group: database
    tags: [db, migrate]
    command: npx
    args: ["prisma", "migrate", "deploy"]
    env:
      DATABASE_URL: "postgresql://localhost:5432/mydb"
    depends_on: [install]

  db-seed:
    label: "Seed Database"
    description: "Populate database with test data"
    group: database
    tags: [db, seed]
    command: npx
    args: ["prisma", "db", "seed"]
    env:
      DATABASE_URL: "postgresql://localhost:5432/mydb"
    depends_on: [db-migrate]
    confirm: true

  db-reset:
    label: "Reset Database"
    description: "Drop, recreate, migrate, and seed the database"
    group: database
    tags: [db, reset, destructive]
    run: "npx prisma migrate reset --force"
    confirm: true

  build-api:
    label: "Build API"
    group: build
    tags: [build, go, api]
    run: "go build -o ./bin/api ./cmd/api"
    working_dir: ./backend
    platform:
      windows:
        run: "go build -o .\\bin\\api.exe .\\cmd\\api"

  build-frontend:
    label: "Build Frontend"
    group: build
    tags: [build, frontend, react]
    run: "npm run build"
    working_dir: ./frontend
    depends_on: [install]

  build-all:
    label: "Build Everything"
    group: build
    tags: [build, all]
    run: "echo 'All builds complete'"
    parallel: [build-api, build-frontend]

  run-api:
    label: "Run API (dev)"
    group: dev
    command: go
    args: ["run", "./cmd/api"]
    working_dir: ./backend
    env:
      GIN_MODE: debug

  run-frontend:
    label: "Run Frontend (dev)"
    group: dev
    run: "npm run dev"
    working_dir: ./frontend

  lint:
    label: "Lint"
    group: quality
    tags: [lint, ci]
    run: "npm run lint"
    ignore_errors: true

  test:
    label: "Run Tests"
    group: quality
    tags: [test, ci]
    run: "npm test"
    timeout: "5m"

  format:
    label: "Format Code"
    group: quality
    tags: [format]
    platform:
      linux:
        run: "gofmt -w . && npx prettier --write ."
      darwin:
        run: "gofmt -w . && npx prettier --write ."
      windows:
        run: "gofmt -w . && npx prettier --write ."

services:
  api:
    label: "API Server"
    group: backend
    uses: run-api
    auto_start: true
    url: "http://localhost:8080"
    env:
      PORT: "8080"
    build:
      uses: build-api

  frontend:
    label: "Frontend Dev Server"
    group: frontend
    uses: run-frontend
    auto_start: true
    url: "http://localhost:3000"
    build:
      uses: build-frontend
```

---

## CLI Interface

```bash
# List all commands
foreman commands

# Run a command by ID
foreman run <command-id>

# Run with env overrides
foreman run <command-id> --env KEY=value --env KEY2=value2

# Run with working directory override
foreman run <command-id> --cwd /path/to/dir

# Run with extra args appended
foreman run <command-id> -- --extra-flag

# Dry-run: show what would execute (resolved command, env, platform)
foreman run <command-id> --dry-run

# Run multiple commands sequentially
foreman run install db-migrate db-seed

# Run multiple commands in parallel
foreman run --parallel lint test
```

---

## Web UI

### Commands Panel

Add a new **Commands** tab/panel alongside the existing Services view:

- **List view**: Shows all commands, grouped by `group`, with label, description, tags
- **Search**: Full-text search across command ID, label, description, and tags
- **Filter**: Filter by group and/or tags
- **Run button**: Triggers command execution, opens a log output panel
- **Status indicators**: Shows running / succeeded / failed state for each command
- **Output streaming**: WebSocket-based real-time output (reuses existing log infrastructure)
- **Confirm dialog**: For commands with `confirm: true`, shows a confirmation modal before running

### API Endpoints

```
GET    /api/commands                     # List all commands
GET    /api/commands?q=<search>          # Search commands
GET    /api/commands?group=<group>       # Filter by group
GET    /api/commands?tag=<tag>           # Filter by tag
POST   /api/command/<id>/run             # Run a command
POST   /api/command/<id>/run             # Body: { "env": {...}, "args": [...] } for overrides
GET    /api/command/<id>/status          # Get execution status
GET    /api/command/<id>/logs?lines=100  # Get command output
POST   /api/command/<id>/cancel          # Cancel a running command
WS     /ws/command/<id>                  # Stream command output in real time
```

### UI Mockup (conceptual)

```
┌──────────────────────────────────────────────────────────┐
│  Foreman                          [Services] [Commands]  │
├──────────────────────────────────────────────────────────┤
│  🔍 Search commands...              Filter: [All Groups] │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ── setup ──                                             │
│  ▶ Install Dependencies         npm install       [Run]  │
│                                                          │
│  ── database ──                                          │
│  ▶ Run Migrations               prisma migrate    [Run]  │
│  ▶ Seed Database                prisma db seed    [Run]  │
│  ▶ Reset Database         ⚠️    prisma reset      [Run]  │
│                                                          │
│  ── build ──                                             │
│  ▶ Build API                    go build          [Run]  │
│  ▶ Build Frontend               npm run build     [Run]  │
│  ▶ Build Everything             parallel          [Run]  │
│                                                          │
│  ── quality ──                                           │
│  ✓ Lint                         completed (0.8s)         │
│  ✗ Run Tests                    failed (exit 1)   [Run]  │
│                                                          │
├──────────────────────────────────────────────────────────┤
│  Output: Run Tests                                       │
│  ─────────────────                                       │
│  > npm test                                              │
│  FAIL src/utils.test.ts                                  │
│  ✗ should validate email (3ms)                           │
│  Expected: true, Received: false                         │
│                                                          │
│  Tests: 1 failed, 42 passed, 43 total                    │
│  Exit code: 1                                            │
└──────────────────────────────────────────────────────────┘
```

---

## Implementation Plan

### Phase 1: Config Parsing

1. **Add `CommandConfig` struct** to `internal/config/loader.go`
   - Fields: `Label`, `Description`, `Group`, `Tags`, `Run`, `Command`, `Args`, `Shell`, `Env`, `EnvFile`, `WorkingDir`, `Platform`, `DependsOn`, `Parallel`, `Timeout`, `IgnoreErrors`, `Confirm`, `Interactive`
   - `PlatformOverride` sub-struct with the same exec fields

2. **Add `commands` map to `Config` struct**
   ```go
   Commands map[string]*CommandConfig `yaml:"commands"`
   ```

3. **Add `Uses` field to `ServiceConfig` and `BuildConfig`**
   ```go
   Uses string `yaml:"uses"`
   ```

4. **Resolve `uses` references** during `Load()`:
   - For services with `uses`, merge the referenced command into the service config
   - For build blocks with `uses`, merge the referenced command into the build config
   - Error on circular or missing references

5. **Resolve platform overrides** during `Load()`:
   - Check `runtime.GOOS`, merge matching `platform.<os>` block into base command

6. **Resolve `run:` shorthand**:
   - If `run` is set, convert to `command: sh`, `args: ["-c", run]` (or `cmd /C` on Windows)
   - Set `shell: true` implicitly

### Phase 2: Command Execution

7. **Add `CommandRunner`** (`internal/command/runner.go`)
   - Executes a resolved `CommandConfig`
   - Handles `depends_on` (sequential pre-execution)
   - Handles `parallel` (concurrent pre-execution with `sync.WaitGroup`)
   - Enforces `timeout` via `context.WithTimeout`
   - Captures stdout/stderr into `LogBuffer`
   - Reports exit code and duration
   - Surfaces `confirm` flag to callers

8. **Add `CommandInfo` type** to `internal/types/service.go`
   ```go
   type CommandInfo struct {
       ID          string        `json:"id"`
       Label       string        `json:"label"`
       Description string        `json:"description"`
       Group       string        `json:"group,omitempty"`
       Tags        []string      `json:"tags,omitempty"`
       Status      CommandStatus `json:"status"`
       ExitCode    *int          `json:"exit_code,omitempty"`
       Duration    string        `json:"duration,omitempty"`
       Confirm     bool          `json:"confirm"`
   }
   ```

9. **Integrate into Orchestrator**
   - Add `commands map[string]*command.Runner` to `Orchestrator`
   - Add methods: `ListCommands()`, `RunCommand(id)`, `CancelCommand(id)`, `GetCommandLogs(id, n)`, `GetCommandStatus(id)`
   - Handle config reload for commands

### Phase 3: CLI

10. **Add `foreman commands` subcommand**: list all commands with group/label/description
11. **Add `foreman run <id>` subcommand**: execute a command by ID
    - Flags: `--env`, `--cwd`, `--dry-run`, `--parallel`
    - Extra args after `--`

### Phase 4: Web UI & API

12. **Add API routes** in `internal/server/api.go`:
    - `GET /api/commands` — list/search/filter
    - `POST /api/command/<id>/run` — trigger execution
    - `GET /api/command/<id>/status` — poll status
    - `GET /api/command/<id>/logs` — fetch output
    - `POST /api/command/<id>/cancel` — cancel running command
13. **Add WebSocket route** `WS /ws/command/<id>` for real-time output streaming
14. **Extend frontend** in `internal/server/frontend.go`:
    - Commands tab with list, search, group filter
    - Run button + confirm dialog
    - Output panel with streaming logs

---

## Go Types (Draft)

```go
// CommandConfig defines a reusable command in the configuration.
type CommandConfig struct {
    Label        string                       `yaml:"label"`
    Description  string                       `yaml:"description"`
    Group        string                       `yaml:"group"`
    Tags         []string                     `yaml:"tags"`
    Run          string                       `yaml:"run"`
    Command      string                       `yaml:"command"`
    Args         []string                     `yaml:"args"`
    Shell        bool                         `yaml:"shell"`
    Env          map[string]string            `yaml:"env"`
    EnvFile      string                       `yaml:"env_file"`
    WorkingDir   string                       `yaml:"working_dir"`
    Platform     map[string]*PlatformOverride `yaml:"platform"`
    DependsOn    []string                     `yaml:"depends_on"`
    Parallel     []string                     `yaml:"parallel"`
    Timeout      string                       `yaml:"timeout"`
    IgnoreErrors bool                         `yaml:"ignore_errors"`
    Confirm      bool                         `yaml:"confirm"`
    Interactive  bool                         `yaml:"interactive"`
}

// PlatformOverride allows OS-specific command definitions.
type PlatformOverride struct {
    Run     string            `yaml:"run"`
    Command string            `yaml:"command"`
    Args    []string          `yaml:"args"`
    Shell   bool              `yaml:"shell"`
    Env     map[string]string `yaml:"env"`
}

// In ServiceConfig, add:
//   Uses string `yaml:"uses"`
//
// In BuildConfig, add:
//   Uses string `yaml:"uses"`
```

---

## Edge Cases & Validation

| Case | Behavior |
|---|---|
| `uses` references a non-existent command | Error at config load time |
| Circular `depends_on` / `parallel` | Detected at load time, returns error |
| Both `run` and `command` specified | Error: mutually exclusive |
| `uses` + inline `command` on a service | Error: mutually exclusive |
| Platform block missing for current OS | Falls back to base command definition |
| Command timeout exceeded | Process killed (SIGTERM → SIGKILL), exit code set, status → `failed` |
| `depends_on` command fails | Abort chain, report which dep failed (unless `ignore_errors`) |
| `parallel` command fails | Wait for all to finish, report failures, abort main command |

---

## Backwards Compatibility

- The `commands` block is entirely optional. Existing `foreman.yaml` files with only `services` continue to work unchanged.
- Services can keep using inline `command`/`args` without `uses`.
- No existing fields are removed or renamed.

# Foreman Example Repo

A complete working example that demonstrates every Foreman feature: commands, services, composable YAML imports, cross-platform overrides, dependency chains, and parallel execution.

All tools here are **mock binaries** — simple bash scripts in `mock-bin/` that simulate `npm`, `node`, `npx`, `go`, and `gofmt` so you can exercise Foreman without installing any real runtimes.

---

## Prerequisites

Build the Foreman binary from the repository root:

```bash
cd /path/to/foreman
go build -o bin/foreman ./cmd/foreman
```

Verify the mock binaries are executable:

```bash
ls -l example-repo/mock-bin/
# All files should show -rwxrwxr-x permissions
# If not: chmod +x example-repo/mock-bin/*
```

---

## Step-by-Step Walkthrough

### Step 1 — Understand the project layout

```
example-repo/
├── foreman.yaml             # Main config (imports other YAML files)
├── db-commands.yaml          # Database commands (imported)
├── quality-commands.yaml     # Quality/CI commands (imported)
├── .env                      # Shared environment variables
├── mock-bin/                 # Mock binaries that simulate real tools
│   ├── npm                   #   npm install, npm run build, etc.
│   ├── node                  #   Node.js runtime
│   ├── npx                   #   npx prisma, npx prettier, etc.
│   ├── go                    #   go build, go run, go test
│   └── gofmt                 #   Go formatter
├── frontend/                 # Frontend project directory (empty)
└── backend/cmd/api/          # Backend project directory (empty)
```

The main `foreman.yaml` imports `db-commands.yaml` and `quality-commands.yaml`, demonstrating composable configuration. Commands defined in the imported files can reference commands in the parent (e.g. `depends_on: [install]`).

### Step 2 — List all commands

See everything Foreman discovered, including commands pulled in via imports:

```bash
./bin/foreman commands -c example-repo/foreman.yaml
```

You should see 14 commands across 6 groups: **build**, **database**, **dev**, **quality**, **setup**.

### Step 3 — Filter and search commands

Filter by group:

```bash
./bin/foreman commands -c example-repo/foreman.yaml -group build
```

Filter by tag:

```bash
./bin/foreman commands -c example-repo/foreman.yaml -tag ci
```

Search by keyword:

```bash
./bin/foreman commands -c example-repo/foreman.yaml -q database
```

### Step 4 — Run a single command

Run the `install` command (simulates `npm install`):

```bash
./bin/foreman run install -c example-repo/foreman.yaml
```

Exit code 0 means success. The mock npm prints simulated output into the Foreman log buffer.

### Step 5 — Dry-run to inspect resolution

See exactly what Foreman would execute — the resolved executable, args, working directory, and shell mode — without running anything:

```bash
./bin/foreman run install -c example-repo/foreman.yaml --dry-run
```

Expected output:

```
Command: install
  Executable: sh
  Args: [-c ./mock-bin/npm install]
  Working Dir: /path/to/example-repo
  Shell: true
```

Try it on a command with parallel pre-steps:

```bash
./bin/foreman run build-all -c example-repo/foreman.yaml --dry-run
```

```
Command: build-all
  Executable: sh
  Args: [-c echo 'All builds complete']
  Working Dir: /path/to/example-repo
  Shell: true
  Parallel: [build-api build-frontend]
```

### Step 6 — Run a command with dependencies (depends_on)

`build-frontend` depends on `install`. Foreman automatically runs `install` first:

```bash
./bin/foreman run build-frontend -c example-repo/foreman.yaml
```

This executes: `install` → `build-frontend` (sequentially).

### Step 7 — Run a command with parallel pre-steps

`build-all` runs `build-api` and `build-frontend` in parallel, then runs itself:

```bash
./bin/foreman run build-all -c example-repo/foreman.yaml
```

This executes: `build-api` ‖ `build-frontend` (concurrently) → `build-all`.

`verify` runs `lint` and `typecheck` in parallel:

```bash
./bin/foreman run verify -c example-repo/foreman.yaml
```

This executes: `lint` ‖ `typecheck` (concurrently) → `verify`.

### Step 8 — Run multiple commands sequentially

Pass multiple command IDs to run them one after another:

```bash
./bin/foreman run install lint test -c example-repo/foreman.yaml
```

### Step 9 — Run multiple commands in parallel

Use `--parallel` to run multiple commands concurrently:

```bash
./bin/foreman run lint test --parallel -c example-repo/foreman.yaml
```

### Step 10 — Run with environment overrides

Override environment variables at runtime:

```bash
./bin/foreman run install -c example-repo/foreman.yaml --env NODE_ENV=production
```

### Step 11 — Run a cross-platform command

The `format` command uses `platform:` overrides — Foreman picks the block matching the current OS:

```bash
./bin/foreman run format -c example-repo/foreman.yaml
```

### Step 12 — Run an imported command

Commands from `db-commands.yaml` are available just like local ones:

```bash
./bin/foreman run db-reset -c example-repo/foreman.yaml
```

### Step 13 — Run the full test suite

Go back to the repo root and run all unit tests to confirm everything is solid:

```bash
cd /path/to/foreman
go test ./... -count=1
```

Expected:

```
ok   github.com/anthropic/foreman/internal/binary    0.003s
ok   github.com/anthropic/foreman/internal/command    0.8s
ok   github.com/anthropic/foreman/internal/config     0.01s
```

### Step 14 — Start the web dashboard

Launch the Foreman dashboard. The 3 services (`api`, `frontend`, `echo-test`) are configured with `auto_start: false`, so they won't start on their own:

```bash
./bin/foreman -c example-repo/foreman.yaml
```

Open [http://127.0.0.1:7044](http://127.0.0.1:7044) in your browser and log in with password **admin**.

From the dashboard you can:
- Start/stop/restart services
- View real-time service logs via WebSocket
- Trigger builds (the `api` and `frontend` services have `build.uses` referencing commands)

### Step 15 — Use the commands API

While the dashboard is running, test the commands REST API in a separate terminal:

```bash
# List all commands
curl -s http://127.0.0.1:7044/api/commands \
  -H 'Cookie: foreman_auth=admin' | python3 -m json.tool | head -30

# Filter by group
curl -s 'http://127.0.0.1:7044/api/commands?group=database' \
  -H 'Cookie: foreman_auth=admin' | python3 -m json.tool

# Search commands
curl -s 'http://127.0.0.1:7044/api/commands?q=build' \
  -H 'Cookie: foreman_auth=admin' | python3 -m json.tool

# Run a command
curl -s -X POST http://127.0.0.1:7044/api/command/install/run \
  -H 'Cookie: foreman_auth=admin' | python3 -m json.tool

# Check command status
curl -s http://127.0.0.1:7044/api/command/install/status \
  -H 'Cookie: foreman_auth=admin' | python3 -m json.tool

# Get command logs
curl -s 'http://127.0.0.1:7044/api/command/install/logs?lines=20' \
  -H 'Cookie: foreman_auth=admin' | python3 -m json.tool
```

---

## What each feature demonstrates

| Feature | Where to see it |
|---------|----------------|
| **Commands** | All `commands:` blocks in `foreman.yaml` |
| **YAML imports** | `imports: [db-commands.yaml, quality-commands.yaml]` in `foreman.yaml` |
| **Cross-file depends_on** | `db-migrate` in `db-commands.yaml` depends on `install` from `foreman.yaml` |
| **Sequential deps** | `build-frontend` → depends on `install` |
| **Parallel pre-steps** | `build-all` → parallel `[build-api, build-frontend]` |
| **Service uses command** | `services.api.uses: run-api` |
| **Build uses command** | `services.api.build.uses: build-api` |
| **Cross-platform** | `format` command has `platform.linux/darwin/windows` overrides |
| **Env merge** | `run-api` has `GIN_MODE: debug`, service `api` adds `PORT: 8080` |
| **Timeout** | `install` has `timeout: "2m"` |
| **ignore_errors** | `lint` has `ignore_errors: true` |
| **confirm** | `db-seed` and `db-reset` have `confirm: true` |
| **Env file** | Root-level `env_file: .env` loads `DATABASE_URL`, `NODE_ENV`, `LOG_LEVEL` |
| **Tags** | Commands tagged with `ci`, `build`, `db`, etc. for filtering |
| **Groups** | Commands grouped into `setup`, `build`, `database`, `quality`, `dev` |
| **Mock binaries** | `mock-bin/` has bash scripts simulating npm, node, npx, go, gofmt |

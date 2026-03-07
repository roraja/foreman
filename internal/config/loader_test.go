package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBasicConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
port: 8080
host: "0.0.0.0"
services:
  web:
    label: "Web Server"
    command: echo
    args: ["hello"]
    auto_start: true
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	svc := cfg.Services["web"]
	if svc.Label != "Web Server" {
		t.Errorf("expected label 'Web Server', got %q", svc.Label)
	}
	if svc.Command != "echo" {
		t.Errorf("expected command 'echo', got %q", svc.Command)
	}
	if !svc.AutoStart {
		t.Error("expected auto_start true")
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
services: {}
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected default port 9090, got %d", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.LogRetentionLines != 10000 {
		t.Errorf("expected default log_retention_lines 10000, got %d", cfg.LogRetentionLines)
	}
}

func TestLoadCommands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
commands:
  install:
    label: "Install"
    description: "Install dependencies"
    group: setup
    tags: [deps, install]
    run: "npm install"
    timeout: "2m"
  build:
    label: "Build"
    command: go
    args: ["build", "./..."]
    working_dir: ./backend
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cfg.Commands))
	}

	install := cfg.Commands["install"]
	if install.Label != "Install" {
		t.Errorf("expected label 'Install', got %q", install.Label)
	}
	if install.Description != "Install dependencies" {
		t.Errorf("expected description, got %q", install.Description)
	}
	if install.Group != "setup" {
		t.Errorf("expected group 'setup', got %q", install.Group)
	}
	if len(install.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(install.Tags))
	}
	if install.Timeout != "2m" {
		t.Errorf("expected timeout '2m', got %q", install.Timeout)
	}
	// run: implies shell: true
	if !install.IsShell() {
		t.Error("expected shell to be true when run: is used")
	}
	cmd, args := install.ResolvedCommand()
	if cmd != "sh" || args[0] != "-c" || args[1] != "npm install" {
		t.Errorf("expected sh -c 'npm install', got %s %v", cmd, args)
	}

	build := cfg.Commands["build"]
	cmd, args = build.ResolvedCommand()
	if cmd != "go" {
		t.Errorf("expected command 'go', got %q", cmd)
	}
	if len(args) != 2 || args[0] != "build" || args[1] != "./..." {
		t.Errorf("expected args [build ./...], got %v", args)
	}
}

func TestLoadCommandsMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
commands:
  bad:
    run: "echo hello"
    command: echo
    args: ["hello"]
`)
	_, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err == nil {
		t.Fatal("expected error for mutually exclusive run/command, got nil")
	}
}

func TestLoadCommandsDependsOnUnknown(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
commands:
  build:
    run: "go build"
    depends_on: [nonexistent]
`)
	_, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err == nil {
		t.Fatal("expected error for unknown depends_on ref, got nil")
	}
}

func TestLoadCommandsCircularDeps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
commands:
  a:
    run: "echo a"
    depends_on: [b]
  b:
    run: "echo b"
    depends_on: [c]
  c:
    run: "echo c"
    depends_on: [a]
`)
	_, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err == nil {
		t.Fatal("expected error for circular deps, got nil")
	}
}

func TestLoadServiceUsesCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
commands:
  run-api:
    label: "Run API"
    command: go
    args: ["run", "./cmd/api"]
    working_dir: ./backend
    env:
      GIN_MODE: debug
services:
  api:
    label: "API Server"
    uses: run-api
    auto_start: true
    env:
      PORT: "8080"
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	svc := cfg.Services["api"]
	if svc.Command != "go" {
		t.Errorf("expected command 'go' from uses, got %q", svc.Command)
	}
	if len(svc.Args) != 2 || svc.Args[0] != "run" {
		t.Errorf("expected args from uses, got %v", svc.Args)
	}
	// Env should be merged: command env + service env
	if svc.Env["GIN_MODE"] != "debug" {
		t.Errorf("expected GIN_MODE=debug from command, got %q", svc.Env["GIN_MODE"])
	}
	if svc.Env["PORT"] != "8080" {
		t.Errorf("expected PORT=8080 from service, got %q", svc.Env["PORT"])
	}
}

func TestLoadServiceUsesUnknown(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
services:
  api:
    uses: nonexistent
`)
	_, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err == nil {
		t.Fatal("expected error for unknown uses reference, got nil")
	}
}

func TestLoadServiceUsesAndCommandMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
commands:
  run-api:
    command: go
    args: ["run", "."]
services:
  api:
    uses: run-api
    command: node
`)
	_, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err == nil {
		t.Fatal("expected error for uses + command, got nil")
	}
}

func TestLoadBuildUsesCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
commands:
  build-api:
    label: "Build API"
    run: "go build -o ./bin/api ./cmd/api"
    working_dir: ./backend
services:
  api:
    label: "API"
    command: ./bin/api
    build:
      uses: build-api
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	svc := cfg.Services["api"]
	if svc.Build == nil {
		t.Fatal("expected build config")
	}
	if svc.Build.Command != "sh" {
		t.Errorf("expected build command 'sh' from uses (run: shorthand), got %q", svc.Build.Command)
	}
}

func TestLoadImports(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "db.yaml", `
commands:
  db-migrate:
    label: "Migrate DB"
    run: "migrate up"
    group: database
  db-seed:
    label: "Seed DB"
    run: "seed data"
    group: database
`)
	writeFile(t, dir, "foreman.yaml", `
project_root: .
imports:
  - db.yaml
commands:
  install:
    label: "Install"
    run: "npm install"
    group: setup
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should have 3 commands: 1 local + 2 imported
	if len(cfg.Commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cfg.Commands))
	}
	if _, ok := cfg.Commands["install"]; !ok {
		t.Error("expected 'install' command")
	}
	if _, ok := cfg.Commands["db-migrate"]; !ok {
		t.Error("expected 'db-migrate' command from import")
	}
	if _, ok := cfg.Commands["db-seed"]; !ok {
		t.Error("expected 'db-seed' command from import")
	}
}

func TestLoadImportsBaseOverridesImported(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "shared.yaml", `
commands:
  install:
    label: "Shared Install"
    run: "yarn install"
`)
	writeFile(t, dir, "foreman.yaml", `
project_root: .
imports:
  - shared.yaml
commands:
  install:
    label: "Local Install"
    run: "npm install"
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Base config should win
	install := cfg.Commands["install"]
	if install.Label != "Local Install" {
		t.Errorf("expected base config to override import, got label %q", install.Label)
	}
}

func TestLoadImportsCircular(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", `
imports:
  - b.yaml
commands:
  a-cmd:
    run: "echo a"
`)
	writeFile(t, dir, "b.yaml", `
imports:
  - a.yaml
commands:
  b-cmd:
    run: "echo b"
`)
	_, err := Load(filepath.Join(dir, "a.yaml"))
	if err == nil {
		t.Fatal("expected error for circular imports, got nil")
	}
}

func TestLoadImportsNested(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, subdir, "deep.yaml", `
commands:
  deep-cmd:
    label: "Deep Command"
    run: "echo deep"
`)
	writeFile(t, dir, "mid.yaml", `
imports:
  - configs/deep.yaml
commands:
  mid-cmd:
    label: "Mid Command"
    run: "echo mid"
`)
	writeFile(t, dir, "foreman.yaml", `
project_root: .
imports:
  - mid.yaml
commands:
  top-cmd:
    label: "Top Command"
    run: "echo top"
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Commands) != 3 {
		t.Fatalf("expected 3 commands from nested imports, got %d", len(cfg.Commands))
	}
	if _, ok := cfg.Commands["deep-cmd"]; !ok {
		t.Error("expected 'deep-cmd' from nested import")
	}
}

func TestLoadImportServices(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "services.yaml", `
services:
  imported-svc:
    label: "Imported Service"
    command: echo
    args: ["imported"]
    auto_start: false
`)
	writeFile(t, dir, "foreman.yaml", `
project_root: .
imports:
  - services.yaml
services:
  local-svc:
    label: "Local Service"
    command: echo
    args: ["local"]
    auto_start: false
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}
	if _, ok := cfg.Services["imported-svc"]; !ok {
		t.Error("expected 'imported-svc' from import")
	}
}

func TestLoadCommandWithEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
commands:
  test:
    run: "echo test"
    env:
      FOO: bar
      BAZ: qux
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	cmd := cfg.Commands["test"]
	if cmd.Env["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got %q", cmd.Env["FOO"])
	}
	if cmd.Env["BAZ"] != "qux" {
		t.Errorf("expected BAZ=qux, got %q", cmd.Env["BAZ"])
	}
}

func TestLoadCommandWithDependsOnAndParallel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
commands:
  install:
    run: "npm install"
  lint:
    run: "npm run lint"
  test:
    run: "npm test"
  verify:
    run: "echo done"
    depends_on: [install]
    parallel: [lint, test]
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	verify := cfg.Commands["verify"]
	if len(verify.DependsOn) != 1 || verify.DependsOn[0] != "install" {
		t.Errorf("expected depends_on [install], got %v", verify.DependsOn)
	}
	if len(verify.Parallel) != 2 {
		t.Errorf("expected parallel [lint, test], got %v", verify.Parallel)
	}
}

func TestLoadCommandConfirmAndIgnoreErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
commands:
  dangerous:
    run: "rm -rf /"
    confirm: true
    ignore_errors: true
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	cmd := cfg.Commands["dangerous"]
	if !cmd.Confirm {
		t.Error("expected confirm: true")
	}
	if !cmd.IgnoreErrors {
		t.Error("expected ignore_errors: true")
	}
}

func TestLoadEnvInterpolation(t *testing.T) {
	os.Setenv("TEST_PORT", "3000")
	defer os.Unsetenv("TEST_PORT")

	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
port: 8080
commands:
  test:
    run: "echo test"
    env:
      PORT: "${TEST_PORT}"
      MISSING: "${NOT_SET:-fallback}"
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	cmd := cfg.Commands["test"]
	if cmd.Env["PORT"] != "3000" {
		t.Errorf("expected PORT=3000 from env interpolation, got %q", cmd.Env["PORT"])
	}
	if cmd.Env["MISSING"] != "fallback" {
		t.Errorf("expected MISSING=fallback from default, got %q", cmd.Env["MISSING"])
	}
}

func TestLoadServiceWithArgsAppended(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
commands:
  run-api:
    command: go
    args: ["run", "./cmd/api"]
services:
  api:
    uses: run-api
    args: ["--watch"]
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	svc := cfg.Services["api"]
	// Service args should be appended after command args
	expected := []string{"run", "./cmd/api", "--watch"}
	if len(svc.Args) != len(expected) {
		t.Fatalf("expected args %v, got %v", expected, svc.Args)
	}
	for i, arg := range expected {
		if svc.Args[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, svc.Args[i])
		}
	}
}

func TestLoadImportDepthLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a chain of imports that exceeds the depth limit
	for i := 0; i < maxImportDepth+2; i++ {
		name := "level" + string(rune('0'+i)) + ".yaml"
		next := "level" + string(rune('0'+i+1)) + ".yaml"
		if i < maxImportDepth+1 {
			writeFile(t, dir, name, "imports:\n  - "+next+"\ncommands:\n  cmd"+string(rune('0'+i))+":\n    run: \"echo\"\n")
		} else {
			writeFile(t, dir, name, "commands:\n  cmdlast:\n    run: \"echo\"\n")
		}
	}

	_, err := Load(filepath.Join(dir, "level0.yaml"))
	if err == nil {
		t.Fatal("expected error for import depth exceeded, got nil")
	}
}

func TestResolvedCommandRunForm(t *testing.T) {
	cmd := &CommandConfig{Run: "npm install"}
	name, args := cmd.ResolvedCommand()
	if name != "sh" {
		t.Errorf("expected 'sh', got %q", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "npm install" {
		t.Errorf("expected [-c, npm install], got %v", args)
	}
}

func TestResolvedCommandDirectForm(t *testing.T) {
	cmd := &CommandConfig{Command: "go", Args: []string{"build", "."}}
	name, args := cmd.ResolvedCommand()
	if name != "go" {
		t.Errorf("expected 'go', got %q", name)
	}
	if len(args) != 2 || args[0] != "build" {
		t.Errorf("expected [build .], got %v", args)
	}
}

func TestIsShellDefault(t *testing.T) {
	cmd := &CommandConfig{}
	if cmd.IsShell() {
		t.Error("expected IsShell() false by default")
	}
}

func TestIsShellExplicit(t *testing.T) {
	trueVal := true
	cmd := &CommandConfig{Shell: &trueVal}
	if !cmd.IsShell() {
		t.Error("expected IsShell() true when explicitly set")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

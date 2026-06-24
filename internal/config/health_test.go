package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadHealthCheckAndSupervision(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
port: 8080
services:
  tunnel:
    label: "SSH Tunnel"
    command: ssh
    args: ["-N", "-L", "6043:localhost:4310", "host"]
    auto_start: true
    auto_restart: true
    restart_delay: "5m"
    max_retries: 20
    health_check:
      ports: [6043]
      interval: "30s"
      grace_period: "10s"
      kill_port_holder: true
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	svc := cfg.Services["tunnel"]
	if svc == nil {
		t.Fatal("tunnel service missing")
	}
	if !svc.AutoRestart {
		t.Error("expected auto_restart true")
	}
	if svc.MaxRetries != 20 {
		t.Errorf("expected max_retries 20, got %d", svc.MaxRetries)
	}
	if got := svc.RestartDelayDuration(); got != 5*time.Minute {
		t.Errorf("expected restart delay 5m, got %s", got)
	}
	if svc.HealthCheck == nil {
		t.Fatal("expected health_check")
	}
	if len(svc.HealthCheck.Ports) != 1 || svc.HealthCheck.Ports[0] != 6043 {
		t.Errorf("expected ports [6043], got %v", svc.HealthCheck.Ports)
	}
	if !svc.HealthCheck.KillPortHolder {
		t.Error("expected kill_port_holder true")
	}
	if got := svc.HealthCheck.IntervalDuration(); got != 30*time.Second {
		t.Errorf("expected interval 30s, got %s", got)
	}
	if got := svc.HealthCheck.GracePeriodDuration(); got != 10*time.Second {
		t.Errorf("expected grace 10s, got %s", got)
	}
}

func TestSupervisionDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
port: 8080
services:
  web:
    command: echo
    args: ["hi"]
`)
	cfg, err := Load(filepath.Join(dir, "foreman.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	svc := cfg.Services["web"]
	if got := svc.RestartDelayDuration(); got != 5*time.Second {
		t.Errorf("expected default restart delay 5s, got %s", got)
	}
	var hc *HealthCheck
	if got := hc.IntervalDuration(); got != 30*time.Second {
		t.Errorf("expected default interval 30s, got %s", got)
	}
	if got := hc.GracePeriodDuration(); got != 15*time.Second {
		t.Errorf("expected default grace 15s, got %s", got)
	}
}

func TestInvalidHealthCheckPort(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foreman.yaml", `
project_root: .
port: 8080
services:
  bad:
    command: echo
    health_check:
      ports: [70000]
`)
	if _, err := Load(filepath.Join(dir, "foreman.yaml")); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}

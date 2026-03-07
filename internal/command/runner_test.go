package command

import (
	"context"
	"testing"
	"time"

	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/types"
)

func TestRunSimpleCommand(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:   "Test Echo",
		Command: "echo",
		Args:    []string{"hello", "world"},
	}
	runner := NewRunner("test-echo", cfg, 100)
	commands := map[string]*Runner{"test-echo": runner}

	err := runner.Run(context.Background(), commands, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if runner.Status() != types.CommandSuccess {
		t.Errorf("expected status success, got %v", runner.Status())
	}

	info := runner.Info()
	if info.ExitCode == nil || *info.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %v", info.ExitCode)
	}
	if info.Duration == "" {
		t.Error("expected duration to be set")
	}
}

func TestRunShellCommand(t *testing.T) {
	cfg := &config.CommandConfig{
		Label: "Shell Echo",
		Run:   "echo 'hello from shell'",
	}
	trueVal := true
	cfg.Shell = &trueVal

	runner := NewRunner("shell-echo", cfg, 100)
	commands := map[string]*Runner{"shell-echo": runner}

	err := runner.Run(context.Background(), commands, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	logs := runner.Logs(100)
	found := false
	for _, entry := range logs {
		if entry.Line == "hello from shell" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'hello from shell' in logs")
		for _, entry := range logs {
			t.Logf("  log: [%s] %s", entry.Stream, entry.Line)
		}
	}
}

func TestRunWithExtraEnv(t *testing.T) {
	cfg := &config.CommandConfig{
		Label: "Env Test",
		Run:   "echo $TEST_VAR",
	}
	trueVal := true
	cfg.Shell = &trueVal

	runner := NewRunner("env-test", cfg, 100)
	commands := map[string]*Runner{"env-test": runner}

	err := runner.Run(context.Background(), commands, map[string]string{"TEST_VAR": "custom_value"}, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	logs := runner.Logs(100)
	found := false
	for _, entry := range logs {
		if entry.Line == "custom_value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'custom_value' in logs")
	}
}

func TestRunWithExtraArgs(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:   "Args Test",
		Command: "echo",
		Args:    []string{"base"},
	}
	runner := NewRunner("args-test", cfg, 100)
	commands := map[string]*Runner{"args-test": runner}

	err := runner.Run(context.Background(), commands, nil, []string{"extra1", "extra2"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	logs := runner.Logs(100)
	found := false
	for _, entry := range logs {
		if entry.Line == "base extra1 extra2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'base extra1 extra2' in logs")
		for _, entry := range logs {
			t.Logf("  log: [%s] %s", entry.Stream, entry.Line)
		}
	}
}

func TestRunFailingCommand(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:   "Fail",
		Command: "false",
	}
	runner := NewRunner("fail", cfg, 100)
	commands := map[string]*Runner{"fail": runner}

	err := runner.Run(context.Background(), commands, nil, nil)
	if err == nil {
		t.Fatal("expected error from failing command")
	}
	if runner.Status() != types.CommandFailed {
		t.Errorf("expected status failed, got %v", runner.Status())
	}
}

func TestRunIgnoreErrors(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:        "Ignore Fail",
		Command:      "false",
		IgnoreErrors: true,
	}
	runner := NewRunner("ignore-fail", cfg, 100)
	commands := map[string]*Runner{"ignore-fail": runner}

	err := runner.Run(context.Background(), commands, nil, nil)
	if err != nil {
		t.Fatalf("expected no error with ignore_errors, got %v", err)
	}
	if runner.Status() != types.CommandSuccess {
		t.Errorf("expected status success with ignore_errors, got %v", runner.Status())
	}
}

func TestRunTimeout(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:   "Timeout",
		Command: "sleep",
		Args:    []string{"10"},
		Timeout: "200ms",
	}

	runner := NewRunner("timeout", cfg, 100)
	commands := map[string]*Runner{"timeout": runner}

	err := runner.Run(context.Background(), commands, nil, nil)
	if err == nil {
		t.Fatal("expected error from timeout")
	}
	if runner.Status() != types.CommandFailed {
		t.Errorf("expected status failed, got %v", runner.Status())
	}
}

func TestRunDependsOn(t *testing.T) {
	depCfg := &config.CommandConfig{
		Label:   "Dependency",
		Command: "echo",
		Args:    []string{"dep-done"},
	}
	mainCfg := &config.CommandConfig{
		Label:     "Main",
		Command:   "echo",
		Args:      []string{"main-done"},
		DependsOn: []string{"dep"},
	}

	dep := NewRunner("dep", depCfg, 100)
	main := NewRunner("main", mainCfg, 100)
	commands := map[string]*Runner{
		"dep":  dep,
		"main": main,
	}

	err := main.Run(context.Background(), commands, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Dependency should have run successfully
	if dep.Status() != types.CommandSuccess {
		t.Errorf("expected dep status success, got %v", dep.Status())
	}
	if main.Status() != types.CommandSuccess {
		t.Errorf("expected main status success, got %v", main.Status())
	}
}

func TestRunDependsOnFailure(t *testing.T) {
	depCfg := &config.CommandConfig{
		Label:   "Bad Dep",
		Command: "false",
	}
	mainCfg := &config.CommandConfig{
		Label:     "Main",
		Command:   "echo",
		Args:      []string{"should-not-run"},
		DependsOn: []string{"bad-dep"},
	}

	dep := NewRunner("bad-dep", depCfg, 100)
	main := NewRunner("main", mainCfg, 100)
	commands := map[string]*Runner{
		"bad-dep": dep,
		"main":    main,
	}

	err := main.Run(context.Background(), commands, nil, nil)
	if err == nil {
		t.Fatal("expected error from failed dependency")
	}
}

func TestRunParallel(t *testing.T) {
	par1Cfg := &config.CommandConfig{
		Label:   "Parallel 1",
		Command: "echo",
		Args:    []string{"p1"},
	}
	par2Cfg := &config.CommandConfig{
		Label:   "Parallel 2",
		Command: "echo",
		Args:    []string{"p2"},
	}
	mainCfg := &config.CommandConfig{
		Label:    "Main",
		Command:  "echo",
		Args:     []string{"main"},
		Parallel: []string{"p1", "p2"},
	}

	p1 := NewRunner("p1", par1Cfg, 100)
	p2 := NewRunner("p2", par2Cfg, 100)
	main := NewRunner("main", mainCfg, 100)
	commands := map[string]*Runner{
		"p1":   p1,
		"p2":   p2,
		"main": main,
	}

	err := main.Run(context.Background(), commands, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p1.Status() != types.CommandSuccess {
		t.Errorf("expected p1 status success, got %v", p1.Status())
	}
	if p2.Status() != types.CommandSuccess {
		t.Errorf("expected p2 status success, got %v", p2.Status())
	}
	if main.Status() != types.CommandSuccess {
		t.Errorf("expected main status success, got %v", main.Status())
	}
}

func TestRunParallelFailure(t *testing.T) {
	par1Cfg := &config.CommandConfig{
		Label:   "Good Parallel",
		Command: "echo",
		Args:    []string{"ok"},
	}
	par2Cfg := &config.CommandConfig{
		Label:   "Bad Parallel",
		Command: "false",
	}
	mainCfg := &config.CommandConfig{
		Label:    "Main",
		Command:  "echo",
		Args:     []string{"main"},
		Parallel: []string{"good-par", "bad-par"},
	}

	p1 := NewRunner("good-par", par1Cfg, 100)
	p2 := NewRunner("bad-par", par2Cfg, 100)
	main := NewRunner("main", mainCfg, 100)
	commands := map[string]*Runner{
		"good-par": p1,
		"bad-par":  p2,
		"main":     main,
	}

	err := main.Run(context.Background(), commands, nil, nil)
	if err == nil {
		t.Fatal("expected error from failed parallel step")
	}
}

func TestCancel(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:   "Long Running",
		Command: "sleep",
		Args:    []string{"60"},
	}

	runner := NewRunner("long", cfg, 100)
	commands := map[string]*Runner{"long": runner}

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(context.Background(), commands, nil, nil)
	}()

	// Wait for command to start and set the cancel function
	time.Sleep(200 * time.Millisecond)

	err := runner.Cancel()
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	select {
	case <-done:
		// Command should have stopped
	case <-time.After(5 * time.Second):
		t.Fatal("command did not stop after cancel")
	}

	if runner.Status() != types.CommandCanceled {
		t.Errorf("expected status canceled, got %v", runner.Status())
	}
}

func TestRunnerInfo(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:       "Test Command",
		Description: "A test command",
		Group:       "test",
		Tags:        []string{"test", "ci"},
		Confirm:     true,
		DependsOn:   []string{"dep1"},
		Parallel:    []string{"par1"},
		Command:     "echo",
		Args:        []string{"hello"},
	}
	runner := NewRunner("test-info", cfg, 100)

	info := runner.Info()
	if info.ID != "test-info" {
		t.Errorf("expected id 'test-info', got %q", info.ID)
	}
	if info.Label != "Test Command" {
		t.Errorf("expected label 'Test Command', got %q", info.Label)
	}
	if info.Description != "A test command" {
		t.Errorf("expected description, got %q", info.Description)
	}
	if info.Group != "test" {
		t.Errorf("expected group 'test', got %q", info.Group)
	}
	if len(info.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(info.Tags))
	}
	if !info.Confirm {
		t.Error("expected confirm true")
	}
	if info.Status != types.CommandIdle {
		t.Errorf("expected status idle, got %v", info.Status)
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	cfg := &config.CommandConfig{
		Label:   "Sub Test",
		Command: "echo",
		Args:    []string{"subscribe-test"},
	}
	runner := NewRunner("sub-test", cfg, 100)

	ch := runner.Subscribe()

	// Run in background
	commands := map[string]*Runner{"sub-test": runner}
	go func() {
		_ = runner.Run(context.Background(), commands, nil, nil)
	}()

	// Read at least one log entry
	select {
	case entry := <-ch:
		if entry.Line == "" {
			t.Error("expected non-empty log entry")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for log entry")
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	runner.Unsubscribe(ch)

	// Drain any buffered entries, then check channel is closed
	for {
		_, ok := <-ch
		if !ok {
			break
		}
	}
}

func TestRunNoCommand(t *testing.T) {
	// A command with only depends_on and no run/command should succeed
	depCfg := &config.CommandConfig{
		Label:   "Dep",
		Command: "echo",
		Args:    []string{"dep-ok"},
	}
	mainCfg := &config.CommandConfig{
		Label:     "Orchestrator",
		DependsOn: []string{"dep-only"},
	}

	dep := NewRunner("dep-only", depCfg, 100)
	main := NewRunner("orchestrator", mainCfg, 100)
	commands := map[string]*Runner{
		"dep-only":     dep,
		"orchestrator": main,
	}

	err := main.Run(context.Background(), commands, nil, nil)
	if err != nil {
		t.Fatalf("expected no error for command with no executable, got %v", err)
	}
	if main.Status() != types.CommandSuccess {
		t.Errorf("expected status success, got %v", main.Status())
	}
}

func TestRunAlreadyRunning(t *testing.T) {
	cfg := &config.CommandConfig{
		Label: "Blocking",
		Run:   "sleep 60",
	}
	trueVal := true
	cfg.Shell = &trueVal

	runner := NewRunner("blocking", cfg, 100)
	commands := map[string]*Runner{"blocking": runner}

	go func() {
		_ = runner.Run(context.Background(), commands, nil, nil)
	}()

	time.Sleep(100 * time.Millisecond)

	err := runner.Run(context.Background(), commands, nil, nil)
	if err == nil {
		t.Fatal("expected error when trying to run already-running command")
	}

	_ = runner.Cancel()
	time.Sleep(100 * time.Millisecond)
}

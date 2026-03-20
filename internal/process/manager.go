package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/anthropic/foreman/internal/binary"
	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/logging"
	"github.com/anthropic/foreman/internal/types"
)

// Process wraps an OS process with log capture and stdin forwarding.
type Process struct {
	ID     string
	Config *config.ServiceConfig

	mu        sync.RWMutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	status    types.ServiceStatus
	pid       int
	exitCode  *int
	startedAt time.Time
	restarts  int
	generation uint64 // incremented on each Start() to detect stale waitForExit goroutines
	logs      *types.LogBuffer
	cancel    context.CancelFunc
	fileLog   *logging.FileLogger

	// subscribers receive new log entries in real-time
	subMu       sync.RWMutex
	subscribers map[chan types.LogEntry]struct{}
}

// NewProcess creates a new process wrapper.
func NewProcess(id string, cfg *config.ServiceConfig, bufferSize int) *Process {
	return &Process{
		ID:          id,
		Config:      cfg,
		status:      types.StatusStopped,
		logs:        types.NewLogBuffer(bufferSize),
		subscribers: make(map[chan types.LogEntry]struct{}),
	}
}

// SetFileLogger attaches a file logger for persisting logs to disk.
func (p *Process) SetFileLogger(fl *logging.FileLogger) {
	p.fileLog = fl
}

// Start launches the process.
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If binary_source is configured, ensure the binary is downloaded
	command := p.Config.Command
	if p.Config.BinarySource != "" {
		binPath, err := binary.EnsureBinary(p.Config.BinarySource, p.Config.BinaryName)
		if err != nil {
			log.Printf("[%s] failed to ensure binary from %s: %v", p.ID, p.Config.BinarySource, err)
			p.emitLog("stderr", fmt.Sprintf("Failed to download binary: %v", err))
			return fmt.Errorf("ensuring binary for %s: %w", p.ID, err)
		}
		command = binPath
		log.Printf("[%s] using downloaded binary: %s", p.ID, binPath)
	}

	log.Printf("[%s] starting process: %s %v (dir: %s)", p.ID, command, p.Config.Args, p.Config.WorkingDir)

	if p.status == types.StatusRunning || p.status == types.StatusStarting {
		log.Printf("[%s] already running, skipping start", p.ID)
		return fmt.Errorf("service %s is already running", p.ID)
	}

	// Start new file log run
	if p.fileLog != nil {
		if err := p.fileLog.StartRun(); err != nil {
			log.Printf("[%s] warning: could not start file log: %v", p.ID, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	cmd := exec.CommandContext(ctx, command, p.Config.Args...)
	cmd.Dir = p.Config.WorkingDir
	setSysProcAttr(cmd)

	// Set environment
	env := os.Environ()
	for k, v := range p.Config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Capture stdout/stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("creating stderr pipe: %w", err)
	}

	// Create stdin pipe
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("creating stdin pipe: %w", err)
	}
	p.stdin = stdinPipe

	p.status = types.StatusStarting
	p.cmd = cmd

	if err := cmd.Start(); err != nil {
		p.status = types.StatusCrashed
		cancel()
		log.Printf("[%s] failed to start: %v", p.ID, err)
		p.emitLog("stderr", fmt.Sprintf("Failed to start: %v", err))
		return fmt.Errorf("starting process: %w", err)
	}

	p.pid = cmd.Process.Pid
	p.startedAt = time.Now()
	p.status = types.StatusRunning
	p.generation++
	gen := p.generation
	log.Printf("[%s] started successfully (PID: %d, gen: %d)", p.ID, p.pid, gen)

	// Stream logs
	go p.streamOutput(stdout, "stdout")
	go p.streamOutput(stderr, "stderr")

	// Wait for exit — pass generation so stale goroutines don't overwrite state
	go p.waitForExit(cmd, gen)

	return nil
}

// Stop gracefully stops the process.
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status != types.StatusRunning && p.status != types.StatusStarting {
		log.Printf("[%s] not running (status: %s), nothing to stop", p.ID, p.status)
		return nil
	}

	log.Printf("[%s] stopping process (PID: %d)", p.ID, p.pid)
	p.status = types.StatusStopping

	if p.cancel != nil {
		p.cancel()
	}

	// Gracefully terminate the process (platform-specific)
	if p.cmd != nil && p.cmd.Process != nil {
		gracefulStop(p.cmd, p.ID)
	}

	p.status = types.StatusStopped
	log.Printf("[%s] stopped", p.ID)
	return nil
}

// Restart stops then starts the process.
func (p *Process) Restart() error {
	log.Printf("[%s] restarting process", p.ID)
	if err := p.Stop(); err != nil {
		log.Printf("[%s] error during restart stop phase: %v", p.ID, err)
		return err
	}
	p.mu.Lock()
	p.restarts++
	count := p.restarts
	p.mu.Unlock()
	log.Printf("[%s] restart count: %d", p.ID, count)
	return p.Start()
}

// Build runs the build command for this service.
func (p *Process) Build() error {
	if p.Config.Build == nil {
		log.Printf("[%s] no build config defined", p.ID)
		return fmt.Errorf("service %s has no build config", p.ID)
	}

	log.Printf("[%s] starting build: %s %v (dir: %s)", p.ID, p.Config.Build.Command, p.Config.Build.Args, p.Config.Build.WorkingDir)

	p.mu.Lock()
	p.status = types.StatusBuilding
	p.mu.Unlock()

	build := p.Config.Build
	cmd := exec.Command(build.Command, build.Args...)
	cmd.Dir = build.WorkingDir

	env := os.Environ()
	for k, v := range build.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Capture build output to log buffer
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		p.mu.Lock()
		p.status = types.StatusStopped
		p.mu.Unlock()
		log.Printf("[%s] build failed to start: %v", p.ID, err)
		p.emitLog("stderr", fmt.Sprintf("Build failed to start: %v", err))
		return fmt.Errorf("build failed to start: %w", err)
	}

	go p.streamOutput(stdout, "stdout")
	go p.streamOutput(stderr, "stderr")

	err := cmd.Wait()
	p.mu.Lock()
	if err != nil {
		p.status = types.StatusCrashed
	} else {
		p.status = types.StatusStopped
	}
	p.mu.Unlock()

	if err != nil {
		log.Printf("[%s] build failed: %v", p.ID, err)
		p.emitLog("stderr", fmt.Sprintf("Build failed: %v", err))
		return fmt.Errorf("build failed: %w", err)
	}
	log.Printf("[%s] build completed successfully", p.ID)
	return nil
}

// WriteStdin sends data to the process's standard input.
func (p *Process) WriteStdin(data []byte) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.stdin == nil {
		return fmt.Errorf("process stdin not available")
	}
	_, err := p.stdin.Write(data)
	return err
}

// Info returns the current ServiceInfo for this process.
func (p *Process) Info() types.ServiceInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	info := types.ServiceInfo{
		ID:        p.ID,
		Label:     p.Config.Label,
		Group:     p.Config.Group,
		Type:      types.TypeProcess,
		Status:    p.status,
		PID:       p.pid,
		Restarts:  p.restarts,
		AutoStart: p.Config.AutoStart,
		HasBuild:  p.Config.Build != nil,
		URL:       p.Config.URL,
	}

	if p.status == types.StatusRunning {
		uptime := time.Since(p.startedAt)
		info.Uptime = formatDuration(uptime)
	}
	if p.exitCode != nil {
		info.ExitCode = p.exitCode
	}

	return info
}

// Logs returns recent log entries.
func (p *Process) Logs(n int) []types.LogEntry {
	return p.logs.Last(n)
}

// Subscribe returns a channel that receives new log entries.
func (p *Process) Subscribe() chan types.LogEntry {
	ch := make(chan types.LogEntry, 100)
	p.subMu.Lock()
	p.subscribers[ch] = struct{}{}
	p.subMu.Unlock()
	return ch
}

// Unsubscribe removes a log subscription.
func (p *Process) Unsubscribe(ch chan types.LogEntry) {
	p.subMu.Lock()
	delete(p.subscribers, ch)
	p.subMu.Unlock()
	close(ch)
}

// GetEnv returns the resolved environment variables for this process.
func (p *Process) GetEnv() map[string]string {
	return p.Config.Env
}

// emitLog adds a log entry to the buffer and broadcasts it to subscribers.
func (p *Process) emitLog(stream, line string) {
	entry := types.LogEntry{
		Timestamp: time.Now(),
		Stream:    stream,
		Line:      line,
	}
	p.logs.Add(entry)
	p.broadcast(entry)
	if p.fileLog != nil {
		p.fileLog.WriteLog(stream, line)
	}
}

func (p *Process) streamOutput(r io.Reader, stream string) {
	scanner := bufio.NewScanner(r)
	// Allow larger lines (1MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		entry := types.LogEntry{
			Timestamp: time.Now(),
			Stream:    stream,
			Line:      line,
		}
		p.logs.Add(entry)
		p.broadcast(entry)
		if p.fileLog != nil {
			p.fileLog.WriteLog(stream, line)
		}
	}
}

func (p *Process) broadcast(entry types.LogEntry) {
	p.subMu.RLock()
	defer p.subMu.RUnlock()
	for ch := range p.subscribers {
		select {
		case ch <- entry:
		default:
			// Drop if subscriber is slow
		}
	}
}

func (p *Process) waitForExit(cmd *exec.Cmd, gen uint64) {
	err := cmd.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()

	// If a newer process has been started since this goroutine was spawned,
	// this is a stale exit notification — log it but don't touch the status.
	if p.generation != gen {
		log.Printf("[%s] stale waitForExit (gen %d, current %d) — ignoring exit from old process", p.ID, gen, p.generation)
		return
	}

	// If Stop() already set the status to stopped/stopping, don't overwrite it.
	// This handles the case where gracefulStop() already called cmd.Wait() and
	// this goroutine gets a "waitid: no child processes" error from the double-wait.
	if p.status == types.StatusStopped || p.status == types.StatusStopping {
		if err != nil {
			log.Printf("[%s] waitForExit: process already stopped (status: %s), ignoring wait error: %v", p.ID, p.status, err)
		} else {
			log.Printf("[%s] process exited cleanly (code 0)", p.ID)
		}
		p.status = types.StatusStopped
		return
	}

	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			code := exitErr.ExitCode()
			p.exitCode = &code
			log.Printf("[%s] process exited with code %d", p.ID, code)
			p.emitLog("stderr", fmt.Sprintf("Process exited with code %d", code))
		} else {
			log.Printf("[%s] process exited with error: %v", p.ID, err)
			p.emitLog("stderr", fmt.Sprintf("Process exited with error: %v", err))
		}
		p.status = types.StatusCrashed
		log.Printf("[%s] process crashed (was not being stopped)", p.ID)
		p.emitLog("stderr", "Process crashed unexpectedly")
	} else {
		code := 0
		p.exitCode = &code
		p.status = types.StatusStopped
		log.Printf("[%s] process exited cleanly (code 0)", p.ID)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

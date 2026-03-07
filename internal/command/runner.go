package command

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

	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/logging"
	"github.com/anthropic/foreman/internal/types"
)

// Runner manages execution of a single command definition.
type Runner struct {
	ID     string
	Config *config.CommandConfig

	mu        sync.RWMutex
	status    types.CommandStatus
	exitCode  *int
	startedAt time.Time
	duration  time.Duration
	cancel    context.CancelFunc
	logs      *types.LogBuffer
	fileLog   *logging.FileLogger

	subMu       sync.RWMutex
	subscribers map[chan types.LogEntry]struct{}
}

// NewRunner creates a new command runner.
func NewRunner(id string, cfg *config.CommandConfig, bufferSize int) *Runner {
	return &Runner{
		ID:          id,
		Config:      cfg,
		status:      types.CommandIdle,
		logs:        types.NewLogBuffer(bufferSize),
		subscribers: make(map[chan types.LogEntry]struct{}),
	}
}

// SetFileLogger attaches a file logger for persisting logs to disk.
func (r *Runner) SetFileLogger(fl *logging.FileLogger) {
	r.fileLog = fl
}

// Info returns the current CommandInfo for this runner.
func (r *Runner) Info() types.CommandInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info := types.CommandInfo{
		ID:          r.ID,
		Label:       r.Config.Label,
		Description: r.Config.Description,
		Group:       r.Config.Group,
		Tags:        r.Config.Tags,
		Status:      r.status,
		Confirm:     r.Config.Confirm,
		DependsOn:   r.Config.DependsOn,
		Parallel:    r.Config.Parallel,
	}
	if r.exitCode != nil {
		info.ExitCode = r.exitCode
	}
	if r.duration > 0 {
		info.Duration = r.duration.String()
	}
	return info
}

// Status returns the current status.
func (r *Runner) Status() types.CommandStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

// Logs returns recent log entries.
func (r *Runner) Logs(n int) []types.LogEntry {
	return r.logs.Last(n)
}

// Subscribe returns a channel that receives new log entries.
func (r *Runner) Subscribe() chan types.LogEntry {
	ch := make(chan types.LogEntry, 100)
	r.subMu.Lock()
	r.subscribers[ch] = struct{}{}
	r.subMu.Unlock()
	return ch
}

// Unsubscribe removes a log subscription.
func (r *Runner) Unsubscribe(ch chan types.LogEntry) {
	r.subMu.Lock()
	delete(r.subscribers, ch)
	r.subMu.Unlock()
	close(ch)
}

// Cancel cancels a running command.
func (r *Runner) Cancel() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != types.CommandRunning {
		return fmt.Errorf("command %s is not running", r.ID)
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.status = types.CommandCanceled
	r.emitLog("stderr", "Command canceled")
	return nil
}

// Run executes this command. The commands map is needed to resolve depends_on/parallel.
func (r *Runner) Run(ctx context.Context, commands map[string]*Runner, extraEnv map[string]string, extraArgs []string) error {
	r.mu.Lock()
	if r.status == types.CommandRunning {
		r.mu.Unlock()
		return fmt.Errorf("command %s is already running", r.ID)
	}
	r.status = types.CommandRunning
	r.exitCode = nil
	r.startedAt = time.Now()
	r.logs.Clear()
	r.mu.Unlock()

	// Start new file log run
	if r.fileLog != nil {
		if err := r.fileLog.StartRun(); err != nil {
			log.Printf("[cmd:%s] warning: could not start file log: %v", r.ID, err)
		}
	}

	r.emitLog("stdout", fmt.Sprintf("=== Running command: %s ===", r.ID))

	// Execute sequential dependencies
	for _, depID := range r.Config.DependsOn {
		dep, ok := commands[depID]
		if !ok {
			r.setFailed(fmt.Errorf("dependency %q not found", depID))
			return fmt.Errorf("dependency %q not found", depID)
		}
		r.emitLog("stdout", fmt.Sprintf("Running dependency: %s", depID))
		if err := dep.Run(ctx, commands, nil, nil); err != nil {
			if !dep.Config.IgnoreErrors {
				r.setFailed(fmt.Errorf("dependency %s failed: %w", depID, err))
				return fmt.Errorf("dependency %s failed: %w", depID, err)
			}
			r.emitLog("stderr", fmt.Sprintf("Dependency %s failed (ignored): %v", depID, err))
		}
	}

	// Execute parallel pre-steps
	if len(r.Config.Parallel) > 0 {
		r.emitLog("stdout", fmt.Sprintf("Running %d parallel pre-steps", len(r.Config.Parallel)))
		var wg sync.WaitGroup
		errCh := make(chan error, len(r.Config.Parallel))

		for _, parID := range r.Config.Parallel {
			par, ok := commands[parID]
			if !ok {
				r.setFailed(fmt.Errorf("parallel command %q not found", parID))
				return fmt.Errorf("parallel command %q not found", parID)
			}
			wg.Add(1)
			go func(id string, runner *Runner) {
				defer wg.Done()
				if err := runner.Run(ctx, commands, nil, nil); err != nil {
					errCh <- fmt.Errorf("parallel command %s failed: %w", id, err)
				}
			}(parID, par)
		}

		wg.Wait()
		close(errCh)

		var errs []error
		for err := range errCh {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			combinedErr := fmt.Errorf("parallel pre-steps failed: %v", errs)
			r.setFailed(combinedErr)
			return combinedErr
		}
	}

	// Execute the command itself
	err := r.execute(ctx, extraEnv, extraArgs)

	r.mu.Lock()
	r.duration = time.Since(r.startedAt)
	if err != nil {
		if r.status != types.CommandCanceled {
			r.status = types.CommandFailed
		}
		r.mu.Unlock()
		r.emitLog("stderr", fmt.Sprintf("Command failed: %v (duration: %s)", err, r.duration))
		return err
	}
	r.status = types.CommandSuccess
	r.mu.Unlock()
	r.emitLog("stdout", fmt.Sprintf("=== Command %s completed (duration: %s) ===", r.ID, r.duration))
	return nil
}

func (r *Runner) execute(ctx context.Context, extraEnv map[string]string, extraArgs []string) error {
	cmdName, cmdArgs := r.Config.ResolvedCommand()
	if cmdName == "" {
		// No command to run (e.g., just depends_on/parallel)
		return nil
	}

	if len(extraArgs) > 0 {
		cmdArgs = append(cmdArgs, extraArgs...)
	}

	// Apply timeout first, before creating the exec context
	if r.Config.Timeout != "" {
		dur, err := time.ParseDuration(r.Config.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", r.Config.Timeout, err)
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}

	// Create cancellable context derived from (possibly timeout-wrapped) ctx
	execCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
	defer cancel()

	cmd := exec.CommandContext(execCtx, cmdName, cmdArgs...)
	cmd.Dir = r.Config.WorkingDir

	// Build environment
	env := os.Environ()
	for k, v := range r.Config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range extraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	r.emitLog("stdout", fmt.Sprintf("> %s %v", cmdName, cmdArgs))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	// Stream output
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); r.streamOutput(stdout, "stdout") }()
	go func() { defer wg.Done(); r.streamOutput(stderr, "stderr") }()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			r.mu.Lock()
			r.exitCode = &code
			r.mu.Unlock()
			if r.Config.IgnoreErrors {
				log.Printf("[cmd:%s] command exited with code %d (ignored)", r.ID, code)
				return nil
			}
			return fmt.Errorf("exit code %d", code)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("command timed out or was canceled")
		}
		return err
	}

	code := 0
	r.mu.Lock()
	r.exitCode = &code
	r.mu.Unlock()
	return nil
}

func (r *Runner) setFailed(err error) {
	r.mu.Lock()
	r.status = types.CommandFailed
	r.duration = time.Since(r.startedAt)
	r.mu.Unlock()
	r.emitLog("stderr", err.Error())
}

func (r *Runner) emitLog(stream, line string) {
	entry := types.LogEntry{
		Timestamp: time.Now(),
		Stream:    stream,
		Line:      line,
	}
	r.logs.Add(entry)
	r.broadcast(entry)
	if r.fileLog != nil {
		r.fileLog.WriteLog(stream, line)
	}
}

func (r *Runner) streamOutput(rd io.Reader, stream string) {
	scanner := bufio.NewScanner(rd)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		entry := types.LogEntry{
			Timestamp: time.Now(),
			Stream:    stream,
			Line:      line,
		}
		r.logs.Add(entry)
		r.broadcast(entry)
		if r.fileLog != nil {
			r.fileLog.WriteLog(stream, line)
		}
	}
}

func (r *Runner) broadcast(entry types.LogEntry) {
	r.subMu.RLock()
	defer r.subMu.RUnlock()
	for ch := range r.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
}

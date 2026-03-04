package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/types"
)

const (
	// Timeout for docker compose up/down/restart commands
	composeCommandTimeout = 120 * time.Second
	// Timeout for docker compose ps/config commands
	composeQueryTimeout = 15 * time.Second
)

// ComposeService represents a single discovered docker-compose service.
type ComposeService struct {
	Name   string
	Status types.ServiceStatus
}

// ComposeManager manages docker-compose services for a single compose file.
type ComposeManager struct {
	ID          string
	Config      *config.ServiceConfig
	ComposeFile string

	mu        sync.RWMutex
	services  []ComposeService
	logs      map[string]*types.LogBuffer
	subMu     sync.RWMutex
	subscribers map[string]map[chan types.LogEntry]struct{}
	cancelLogs map[string]context.CancelFunc

	// opsLogs captures output from docker compose commands (up, stop, restart, etc.)
	// These are viewable when expanding the parent compose group in the UI.
	opsLogs       *types.LogBuffer
	opsSubMu      sync.RWMutex
	opsSubscribers map[chan types.LogEntry]struct{}

	// cancelAllLogs cancels the combined "docker compose logs -f" stream
	cancelAllLogs context.CancelFunc
}

// NewComposeManager creates a new docker-compose manager.
func NewComposeManager(id string, cfg *config.ServiceConfig, bufferSize int) *ComposeManager {
	return &ComposeManager{
		ID:             id,
		Config:         cfg,
		ComposeFile:    cfg.ComposeFile,
		logs:           make(map[string]*types.LogBuffer),
		subscribers:    make(map[string]map[chan types.LogEntry]struct{}),
		cancelLogs:     make(map[string]context.CancelFunc),
		opsLogs:        types.NewLogBuffer(bufferSize),
		opsSubscribers: make(map[chan types.LogEntry]struct{}),
	}
}

// composeCmd creates a docker compose command with proper working dir, env, and timeout context.
func (cm *ComposeManager) composeCmd(timeout time.Duration, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	fullArgs := append([]string{"compose", "-f", cm.ComposeFile}, args...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)

	// Set working directory to the compose file's parent directory
	// so docker compose can find .env and relative paths
	cmd.Dir = filepath.Dir(cm.ComposeFile)

	// Inherit all current environment + service-level env vars
	cmd.Env = os.Environ()
	for k, v := range cm.Config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return cmd, cancel
}

// DiscoverServices runs `docker compose config --services` to find all services.
func (cm *ComposeManager) DiscoverServices() error {
	log.Printf("[docker:%s] discovering services from %s (compose_file: %s)", cm.ID, cm.ComposeFile, cm.ComposeFile)
	cmd, cancel := cm.composeCmd(composeQueryTimeout, "config", "--services")
	defer cancel()
	log.Printf("[docker:%s] running: docker compose config --services (cwd: %s)", cm.ID, cmd.Dir)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("[docker:%s] failed to discover services: %v", cm.ID, err)
		return fmt.Errorf("discovering compose services: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.services = nil
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			cm.services = append(cm.services, ComposeService{
				Name:   name,
				Status: types.StatusStopped,
			})
			if _, ok := cm.logs[name]; !ok {
				cm.logs[name] = types.NewLogBuffer(10000)
			}
			log.Printf("[docker:%s] discovered service: %s", cm.ID, name)
		}
	}

	log.Printf("[docker:%s] discovered %d services total", cm.ID, len(cm.services))
	return nil
}

// RefreshStatus updates the status of all discovered services.
func (cm *ComposeManager) RefreshStatus() error {
	cmd, cancel := cm.composeCmd(composeQueryTimeout, "ps", "--format", "json")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		log.Printf("[docker:%s] refresh status: docker compose ps failed (marking all stopped): %v", cm.ID, err)
		// If compose is not running, all services are stopped
		cm.mu.Lock()
		for i := range cm.services {
			cm.services[i].Status = types.StatusStopped
		}
		cm.mu.Unlock()
		return nil
	}

	// Parse JSON output (docker compose ps --format json outputs one JSON object per line)
	type containerInfo struct {
		Name    string `json:"Name"`
		Service string `json:"Service"`
		State   string `json:"State"`
		Health  string `json:"Health"`
	}

	containers := make(map[string]containerInfo)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Try parsing as JSON array first, then as single object
		if strings.HasPrefix(line, "[") {
			var arr []containerInfo
			if err := json.Unmarshal([]byte(line), &arr); err == nil {
				for _, c := range arr {
					containers[c.Service] = c
				}
				continue
			}
		}
		var c containerInfo
		if err := json.Unmarshal([]byte(line), &c); err == nil {
			containers[c.Service] = c
		}
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, svc := range cm.services {
		if c, ok := containers[svc.Name]; ok {
			cm.services[i].Status = mapDockerStatus(c.State, c.Health)
		} else {
			cm.services[i].Status = types.StatusStopped
		}
	}

	return nil
}

// StartService starts a single docker-compose service.
func (cm *ComposeManager) StartService(name string) error {
	log.Printf("[docker:%s] starting service: %s", cm.ID, name)
	cm.opsLog("info", "Starting service: %s", name)
	start := time.Now()

	if err := cm.runComposeStreamed("start "+name, composeCommandTimeout, "up", "-d", name); err != nil {
		cm.opsLog("error", "Failed to start %s after %s: %v", name, time.Since(start).Round(time.Millisecond), err)
		return fmt.Errorf("starting service %s: %w", name, err)
	}

	cm.opsLog("info", "Started service: %s (took %s)", name, time.Since(start).Round(time.Millisecond))
	go cm.streamServiceLogs(name)
	return nil
}

// StopService stops a single docker-compose service and removes its container.
func (cm *ComposeManager) StopService(name string) error {
	log.Printf("[docker:%s] stopping service: %s", cm.ID, name)
	cm.opsLog("info", "Stopping service: %s", name)
	start := time.Now()

	cm.cancelServiceLogs(name)

	if err := cm.runComposeStreamed("stop "+name, composeCommandTimeout, "stop", "-t", "10", name); err != nil {
		cm.opsLog("error", "Failed to stop %s after %s: %v", name, time.Since(start).Round(time.Millisecond), err)
		return fmt.Errorf("stopping service %s: %w", name, err)
	}

	// Remove the stopped container to fully clean up
	log.Printf("[docker:%s] removing container for service: %s", cm.ID, name)
	if err := cm.runComposeStreamed("rm "+name, composeCommandTimeout, "rm", "-f", name); err != nil {
		log.Printf("[docker:%s] warning: failed to remove container for %s: %v (non-fatal)", cm.ID, name, err)
	}

	cm.opsLog("info", "Stopped service: %s (took %s)", name, time.Since(start).Round(time.Millisecond))
	return nil
}

// RestartService restarts a single docker-compose service.
func (cm *ComposeManager) RestartService(name string) error {
	log.Printf("[docker:%s] restarting service: %s", cm.ID, name)
	cm.opsLog("info", "Restarting service: %s", name)
	start := time.Now()

	cm.cancelServiceLogs(name)

	if err := cm.runComposeStreamed("restart "+name, composeCommandTimeout, "restart", "-t", "10", name); err != nil {
		cm.opsLog("error", "Failed to restart %s after %s: %v", name, time.Since(start).Round(time.Millisecond), err)
		return fmt.Errorf("restarting service %s: %w", name, err)
	}

	cm.opsLog("info", "Restarted service: %s (took %s)", name, time.Since(start).Round(time.Millisecond))
	go cm.streamServiceLogs(name)
	return nil
}

// StartAll starts all services in the compose stack.
func (cm *ComposeManager) StartAll() error {
	log.Printf("[docker:%s] starting all services", cm.ID)
	cm.opsLog("info", "=== Starting all docker compose services ===")
	start := time.Now()

	if err := cm.runComposeStreamed("up -d", composeCommandTimeout, "up", "-d"); err != nil {
		cm.opsLog("error", "Failed to start all services after %s: %v", time.Since(start).Round(time.Millisecond), err)
		return fmt.Errorf("starting compose stack: %w", err)
	}

	cm.opsLog("info", "=== All services started (took %s) ===", time.Since(start).Round(time.Millisecond))
	log.Printf("[docker:%s] all services started (took %s), starting log streams", cm.ID, time.Since(start).Round(time.Millisecond))
	cm.mu.RLock()
	for _, svc := range cm.services {
		go cm.streamServiceLogs(svc.Name)
	}
	cm.mu.RUnlock()

	// Stream combined logs from all services to the ops log
	go cm.streamAllLogs()
	return nil
}

// StopAll stops all services in the compose stack.
// Uses 'docker compose down' to stop and remove containers and networks.
func (cm *ComposeManager) StopAll() error {
	log.Printf("[docker:%s] stopping all services (using docker compose down)", cm.ID)
	cm.opsLog("info", "=== Stopping all docker compose services (down) ===")
	start := time.Now()

	// Cancel the combined log stream
	cm.cancelCombinedLogs()

	// Copy service names under lock, then cancel each without holding the lock
	// (cancelServiceLogs acquires cm.mu.Lock, so we must not hold RLock)
	names := cm.ServiceNames()
	for _, name := range names {
		cm.cancelServiceLogs(name)
	}

	if err := cm.runComposeStreamed("down", composeCommandTimeout, "down", "-t", "10"); err != nil {
		cm.opsLog("error", "Failed to stop all services (down) after %s: %v", time.Since(start).Round(time.Millisecond), err)
		return fmt.Errorf("stopping compose stack (down): %w", err)
	}

	cm.opsLog("info", "=== All services stopped and removed (took %s) ===", time.Since(start).Round(time.Millisecond))
	log.Printf("[docker:%s] all services stopped via docker compose down (took %s)", cm.ID, time.Since(start).Round(time.Millisecond))
	return nil
}

// RestartAll restarts all services using docker compose restart (no down/up cycle).
func (cm *ComposeManager) RestartAll() error {
	log.Printf("[docker:%s] restarting all services", cm.ID)
	cm.opsLog("info", "=== Restarting all docker compose services ===")
	start := time.Now()

	// Cancel combined log stream before restart
	cm.cancelCombinedLogs()

	// Copy service names under lock, then cancel each without holding the lock
	names := cm.ServiceNames()
	for _, name := range names {
		cm.cancelServiceLogs(name)
	}

	if err := cm.runComposeStreamed("restart", composeCommandTimeout, "restart", "-t", "10"); err != nil {
		cm.opsLog("error", "Failed to restart all services after %s: %v", time.Since(start).Round(time.Millisecond), err)
		return fmt.Errorf("restarting compose stack: %w", err)
	}

	cm.opsLog("info", "=== All services restarted (took %s) ===", time.Since(start).Round(time.Millisecond))
	log.Printf("[docker:%s] all services restarted (took %s), restarting log streams", cm.ID, time.Since(start).Round(time.Millisecond))
	cm.mu.RLock()
	for _, svc := range cm.services {
		go cm.streamServiceLogs(svc.Name)
	}
	cm.mu.RUnlock()

	// Restart combined log stream
	go cm.streamAllLogs()
	return nil
}

// Info returns ServiceInfo for the compose group, with children for each sub-service.
func (cm *ComposeManager) Info() types.ServiceInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	info := types.ServiceInfo{
		ID:        cm.ID,
		Label:     cm.Config.Label,
		Group:     cm.Config.Group,
		Type:      types.TypeDockerCompose,
		Status:    types.StatusStopped,
		AutoStart: cm.Config.AutoStart,
		URL:       cm.Config.URL,
	}

	runningCount := 0
	for _, svc := range cm.services {
		child := types.ServiceInfo{
			ID:     cm.ID + "/" + svc.Name,
			Label:  svc.Name,
			Type:   types.TypeDockerCompose,
			Status: svc.Status,
		}
		info.Children = append(info.Children, child)
		if svc.Status == types.StatusRunning {
			runningCount++
		}
	}

	// Overall status: running if any child is running
	if runningCount == len(cm.services) && runningCount > 0 {
		info.Status = types.StatusRunning
	} else if runningCount > 0 {
		info.Status = types.StatusRunning // partial
	}

	return info
}

// Logs returns recent log entries for a sub-service.
// If serviceName is empty, returns the parent-level operations log.
func (cm *ComposeManager) Logs(serviceName string, n int) []types.LogEntry {
	if serviceName == "" {
		return cm.opsLogs.Last(n)
	}
	cm.mu.RLock()
	buf, ok := cm.logs[serviceName]
	cm.mu.RUnlock()
	if !ok {
		return nil
	}
	return buf.Last(n)
}

// Subscribe returns a channel that receives log entries for a sub-service.
// If serviceName is empty, subscribes to the parent-level operations log.
func (cm *ComposeManager) Subscribe(serviceName string) chan types.LogEntry {
	ch := make(chan types.LogEntry, 100)
	if serviceName == "" {
		// Subscribe to parent ops log
		cm.opsSubMu.Lock()
		cm.opsSubscribers[ch] = struct{}{}
		cm.opsSubMu.Unlock()
		return ch
	}
	cm.subMu.Lock()
	if cm.subscribers[serviceName] == nil {
		cm.subscribers[serviceName] = make(map[chan types.LogEntry]struct{})
	}
	cm.subscribers[serviceName][ch] = struct{}{}
	cm.subMu.Unlock()
	return ch
}

// Unsubscribe removes a log subscription for a sub-service.
// If serviceName is empty, unsubscribes from the parent-level operations log.
func (cm *ComposeManager) Unsubscribe(serviceName string, ch chan types.LogEntry) {
	if serviceName == "" {
		cm.opsSubMu.Lock()
		delete(cm.opsSubscribers, ch)
		cm.opsSubMu.Unlock()
		close(ch)
		return
	}
	cm.subMu.Lock()
	if subs, ok := cm.subscribers[serviceName]; ok {
		delete(subs, ch)
	}
	cm.subMu.Unlock()
	close(ch)
}

// ServiceNames returns the list of discovered service names.
func (cm *ComposeManager) ServiceNames() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	names := make([]string, len(cm.services))
	for i, s := range cm.services {
		names[i] = s.Name
	}
	return names
}

// streamAllLogs runs `docker compose logs -f --tail=50` for ALL services combined
// and pipes output into the ops log buffer (visible when expanding the parent compose group).
func (cm *ComposeManager) streamAllLogs() {
	// Cancel any existing combined log stream
	cm.cancelCombinedLogs()

	ctx, cancel := context.WithCancel(context.Background())
	cm.mu.Lock()
	cm.cancelAllLogs = cancel
	cm.mu.Unlock()

	args := []string{"compose", "-f", cm.ComposeFile, "logs", "-f", "--tail=50"}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = filepath.Dir(cm.ComposeFile)
	cmd.Env = os.Environ()
	for k, v := range cm.Config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	log.Printf("[docker:%s] starting combined log stream: docker compose logs -f --tail=50 (cwd: %s)", cm.ID, cmd.Dir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[docker:%s] failed to create stdout pipe for combined logs: %v", cm.ID, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[docker:%s] failed to create stderr pipe for combined logs: %v", cm.ID, err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[docker:%s] failed to start combined log streaming: %v", cm.ID, err)
		return
	}

	log.Printf("[docker:%s] combined log stream started (PID: %d)", cm.ID, cmd.Process.Pid)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			cm.emitOpsLog("stdout", scanner.Text())
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			cm.emitOpsLog("stderr", scanner.Text())
		}
	}()

	wg.Wait()
	_ = cmd.Wait()
	log.Printf("[docker:%s] combined log stream ended", cm.ID)
}

// cancelCombinedLogs stops the combined "docker compose logs -f" stream.
func (cm *ComposeManager) cancelCombinedLogs() {
	cm.mu.Lock()
	if cm.cancelAllLogs != nil {
		log.Printf("[docker:%s] cancelling combined log stream", cm.ID)
		cm.cancelAllLogs()
		cm.cancelAllLogs = nil
	}
	cm.mu.Unlock()
}

func (cm *ComposeManager) streamServiceLogs(name string) {
	ctx, cancel := context.WithCancel(context.Background())
	cm.mu.Lock()
	cm.cancelLogs[name] = cancel
	cm.mu.Unlock()

	args := []string{"compose", "-f", cm.ComposeFile, "logs", "-f", "--tail=100", name}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = filepath.Dir(cm.ComposeFile)
	cmd.Env = os.Environ()
	for k, v := range cm.Config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	log.Printf("[docker:%s] starting log stream for service %s (cwd: %s)", cm.ID, name, cmd.Dir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[docker:%s] failed to create stdout pipe for %s logs: %v", cm.ID, name, err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[docker:%s] failed to start log streaming for %s: %v", cm.ID, name, err)
		return
	}

	log.Printf("[docker:%s] log stream for %s started (PID: %d)", cm.ID, name, cmd.Process.Pid)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		entry := types.LogEntry{
			Timestamp: time.Now(),
			Stream:    "stdout",
			Line:      scanner.Text(),
		}
		cm.mu.RLock()
		if buf, ok := cm.logs[name]; ok {
			buf.Add(entry)
		}
		cm.mu.RUnlock()

		cm.subMu.RLock()
		if subs, ok := cm.subscribers[name]; ok {
			for ch := range subs {
				select {
				case ch <- entry:
				default:
				}
			}
		}
		cm.subMu.RUnlock()
	}

	_ = cmd.Wait()
	log.Printf("[docker:%s] log stream for %s ended", cm.ID, name)
}

func (cm *ComposeManager) cancelServiceLogs(name string) {
	cm.mu.Lock()
	if cancel, ok := cm.cancelLogs[name]; ok {
		log.Printf("[docker:%s] cancelling log stream for service %s", cm.ID, name)
		cancel()
		delete(cm.cancelLogs, name)
	}
	cm.mu.Unlock()
}

// runComposeStreamed runs a docker compose command and streams its stdout/stderr
// line-by-line to the ops log buffer and subscribers in real-time.
func (cm *ComposeManager) runComposeStreamed(label string, timeout time.Duration, args ...string) error {
	cmd, cancel := cm.composeCmd(timeout, args...)
	defer cancel()

	log.Printf("[docker:%s] running: docker compose %s (timeout: %s, cwd: %s)", cm.ID, strings.Join(args, " "), timeout, cmd.Dir)

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[docker:%s] failed to create stdout pipe for %s: %v", cm.ID, label, err)
		return fmt.Errorf("creating stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[docker:%s] failed to create stderr pipe for %s: %v", cm.ID, label, err)
		return fmt.Errorf("creating stderr pipe: %w", err)
	}

	if startErr := cmd.Start(); startErr != nil {
		log.Printf("[docker:%s] failed to start command for %s: %v", cm.ID, label, startErr)
		return fmt.Errorf("starting command: %w", startErr)
	}

	log.Printf("[docker:%s] command started (PID: %d)", cm.ID, cmd.Process.Pid)

	// Stream stdout and stderr concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[docker:%s] [%s] %s", cm.ID, label, line)
			cm.emitOpsLog("stdout", line)
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[docker:%s] [%s] (stderr) %s", cm.ID, label, line)
			cm.emitOpsLog("stderr", line)
		}
	}()

	wg.Wait()

	if waitErr := cmd.Wait(); waitErr != nil {
		log.Printf("[docker:%s] command '%s' exited with error: %v", cm.ID, label, waitErr)
		return fmt.Errorf("command '%s' failed: %w", label, waitErr)
	}

	log.Printf("[docker:%s] command '%s' completed successfully", cm.ID, label)
	return nil
}

// opsLog writes a formatted message to the ops log buffer (and foreman log).
func (cm *ComposeManager) opsLog(stream string, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[docker:%s] %s", cm.ID, msg)
	cm.emitOpsLog(stream, msg)
}

// emitOpsLog writes a raw line to the ops log buffer and broadcasts to subscribers.
func (cm *ComposeManager) emitOpsLog(stream string, line string) {
	entry := types.LogEntry{
		Timestamp: time.Now(),
		Stream:    stream,
		Line:      line,
	}
	cm.opsLogs.Add(entry)

	cm.opsSubMu.RLock()
	for ch := range cm.opsSubscribers {
		select {
		case ch <- entry:
		default:
		}
	}
	cm.opsSubMu.RUnlock()
}

func mapDockerStatus(state, health string) types.ServiceStatus {
	switch strings.ToLower(state) {
	case "running":
		if strings.ToLower(health) == "unhealthy" {
			return types.StatusUnhealthy
		}
		return types.StatusRunning
	case "created", "restarting":
		return types.StatusStarting
	default:
		return types.StatusStopped
	}
}

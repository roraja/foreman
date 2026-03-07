package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/anthropic/foreman/internal/command"
	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/docker"
	"github.com/anthropic/foreman/internal/logging"
	"github.com/anthropic/foreman/internal/process"
	"github.com/anthropic/foreman/internal/types"
)

// Orchestrator coordinates all services (native processes + docker-compose).
type Orchestrator struct {
	mu              sync.RWMutex
	cfg             *config.Config
	configPath      string
	processes       map[string]*process.Process
	composeManagers map[string]*docker.ComposeManager
	commands        map[string]*command.Runner
}

// New creates an orchestrator from the given config.
func New(cfg *config.Config, configPath string) *Orchestrator {
	o := &Orchestrator{
		cfg:             cfg,
		configPath:      configPath,
		processes:       make(map[string]*process.Process),
		composeManagers: make(map[string]*docker.ComposeManager),
		commands:        make(map[string]*command.Runner),
	}
	o.initServices()
	o.initCommands()
	return o
}

func (o *Orchestrator) initServices() {
	log.Printf("initializing services from config (%d services configured)", len(o.cfg.Services))
	logsDir := o.cfg.ResolvedLogsDir()
	for id, svc := range o.cfg.Services {
		if svc.Type == "docker-compose" {
			log.Printf("  creating compose manager: %s (compose_file: %s)", id, svc.ComposeFile)
			cm := docker.NewComposeManager(id, svc, o.cfg.LogRetentionLines)
			if err := cm.DiscoverServices(); err != nil {
				log.Printf("warning: could not discover services for %s: %v", id, err)
			}
			o.composeManagers[id] = cm
		} else {
			log.Printf("  creating process: %s (command: %s)", id, svc.Command)
			p := process.NewProcess(id, svc, o.cfg.LogRetentionLines)
			if o.cfg.ServiceLogsEnabled() {
				p.SetFileLogger(logging.NewFileLogger(logsDir, id))
			}
			o.processes[id] = p
		}
	}
	log.Printf("initialized %d processes and %d compose managers", len(o.processes), len(o.composeManagers))
}

// StartService starts a service by ID. For docker-compose, use "parent/child" for sub-services.
func (o *Orchestrator) StartService(id string) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	log.Printf("start requested for service: %s", id)

	// Check for docker sub-service (e.g., "docker-services/web")
	parentID, childName := splitServiceID(id)
	if childName != "" {
		if cm, ok := o.composeManagers[parentID]; ok {
			return cm.StartService(childName)
		}
		log.Printf("compose group %s not found", parentID)
		return fmt.Errorf("compose group %s not found", parentID)
	}

	if p, ok := o.processes[id]; ok {
		return p.Start()
	}
	if cm, ok := o.composeManagers[id]; ok {
		return cm.StartAll()
	}
	log.Printf("service %s not found", id)
	return fmt.Errorf("service %s not found", id)
}

// StopService stops a service by ID.
func (o *Orchestrator) StopService(id string) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	log.Printf("stop requested for service: %s", id)

	parentID, childName := splitServiceID(id)
	if childName != "" {
		if cm, ok := o.composeManagers[parentID]; ok {
			return cm.StopService(childName)
		}
		log.Printf("compose group %s not found", parentID)
		return fmt.Errorf("compose group %s not found", parentID)
	}

	if p, ok := o.processes[id]; ok {
		return p.Stop()
	}
	if cm, ok := o.composeManagers[id]; ok {
		return cm.StopAll()
	}
	log.Printf("service %s not found", id)
	return fmt.Errorf("service %s not found", id)
}

// RestartService restarts a service by ID.
func (o *Orchestrator) RestartService(id string) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	log.Printf("restart requested for service: %s", id)

	parentID, childName := splitServiceID(id)
	if childName != "" {
		if cm, ok := o.composeManagers[parentID]; ok {
			return cm.RestartService(childName)
		}
		return fmt.Errorf("compose group %s not found", parentID)
	}

	if p, ok := o.processes[id]; ok {
		return p.Restart()
	}
	if cm, ok := o.composeManagers[id]; ok {
		return cm.RestartAll()
	}
	return fmt.Errorf("service %s not found", id)
}

// BuildService runs the build command for a service.
func (o *Orchestrator) BuildService(id string) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	log.Printf("build requested for service: %s", id)

	if p, ok := o.processes[id]; ok {
		return p.Build()
	}
	log.Printf("service %s not found or is docker-compose (cannot build)", id)
	return fmt.Errorf("service %s not found or is a docker-compose service (use docker compose build)", id)
}

// ListServices returns info for all services.
func (o *Orchestrator) ListServices() []types.ServiceInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var result []types.ServiceInfo

	// Refresh docker statuses
	for _, cm := range o.composeManagers {
		_ = cm.RefreshStatus()
	}

	for _, p := range o.processes {
		result = append(result, p.Info())
	}
	for _, cm := range o.composeManagers {
		result = append(result, cm.Info())
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		return result[i].ID < result[j].ID
	})

	return result
}

// GetServiceInfo returns info for a single service.
func (o *Orchestrator) GetServiceInfo(id string) (types.ServiceInfo, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if p, ok := o.processes[id]; ok {
		return p.Info(), nil
	}
	if cm, ok := o.composeManagers[id]; ok {
		_ = cm.RefreshStatus()
		return cm.Info(), nil
	}
	return types.ServiceInfo{}, fmt.Errorf("service %s not found", id)
}

// GetLogs returns recent logs for a service.
func (o *Orchestrator) GetLogs(id string, n int) ([]types.LogEntry, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	parentID, childName := splitServiceID(id)
	if childName != "" {
		if cm, ok := o.composeManagers[parentID]; ok {
			return cm.Logs(childName, n), nil
		}
		return nil, fmt.Errorf("compose group %s not found", parentID)
	}

	if p, ok := o.processes[id]; ok {
		return p.Logs(n), nil
	}
	if cm, ok := o.composeManagers[id]; ok {
		// Return the parent-level operations log (docker compose command output)
		return cm.Logs("", n), nil
	}
	return nil, fmt.Errorf("service %s not found", id)
}

// GetEnv returns environment variables for a service.
func (o *Orchestrator) GetEnv(id string) (map[string]string, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if p, ok := o.processes[id]; ok {
		return p.GetEnv(), nil
	}
	return nil, fmt.Errorf("service %s not found or is a docker-compose service", id)
}

// SubscribeLogs subscribes to real-time log entries for a service.
func (o *Orchestrator) SubscribeLogs(id string) (chan types.LogEntry, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	parentID, childName := splitServiceID(id)
	if childName != "" {
		if cm, ok := o.composeManagers[parentID]; ok {
			return cm.Subscribe(childName), nil
		}
		return nil, fmt.Errorf("compose group %s not found", parentID)
	}

	if p, ok := o.processes[id]; ok {
		return p.Subscribe(), nil
	}
	// Subscribe to the parent compose group ops log
	if cm, ok := o.composeManagers[id]; ok {
		return cm.Subscribe(""), nil
	}
	return nil, fmt.Errorf("service %s not found", id)
}

// UnsubscribeLogs unsubscribes from real-time log entries.
func (o *Orchestrator) UnsubscribeLogs(id string, ch chan types.LogEntry) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	parentID, childName := splitServiceID(id)
	if childName != "" {
		if cm, ok := o.composeManagers[parentID]; ok {
			cm.Unsubscribe(childName, ch)
			return
		}
		return
	}

	if p, ok := o.processes[id]; ok {
		p.Unsubscribe(ch)
		return
	}
	// Unsubscribe from parent compose group ops log
	if cm, ok := o.composeManagers[id]; ok {
		cm.Unsubscribe("", ch)
		return
	}
}

// WriteStdin sends data to a service's stdin.
func (o *Orchestrator) WriteStdin(id string, data []byte) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if p, ok := o.processes[id]; ok {
		return p.WriteStdin(data)
	}
	return fmt.Errorf("stdin not supported for service %s", id)
}

// StartAutoStart starts all services with auto_start=true.
func (o *Orchestrator) StartAutoStart() {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for id, p := range o.processes {
		if p.Config.AutoStart {
			log.Printf("auto-starting service: %s", id)
			if err := p.Start(); err != nil {
				log.Printf("failed to auto-start %s: %v", id, err)
			}
		}
	}
	for id, cm := range o.composeManagers {
		if cm.Config.AutoStart {
			log.Printf("auto-starting compose stack: %s", id)
			if err := cm.StartAll(); err != nil {
				log.Printf("failed to auto-start %s: %v", id, err)
			}
		}
	}
}

// StopAll stops all running services.
func (o *Orchestrator) StopAll() {
	o.mu.RLock()
	defer o.mu.RUnlock()

	log.Printf("stopping all services")

	for id, p := range o.processes {
		if err := p.Stop(); err != nil {
			log.Printf("error stopping %s: %v", id, err)
		}
	}
	for id, cm := range o.composeManagers {
		if err := cm.StopAll(); err != nil {
			log.Printf("error stopping compose %s: %v", id, err)
		}
	}
}

// ReloadConfig re-reads the configuration file and updates services.
func (o *Orchestrator) ReloadConfig() (added []string, removed []string, err error) {
	log.Printf("reloading configuration from %s", o.configPath)
	newCfg, err := config.Load(o.configPath)
	if err != nil {
		log.Printf("config reload failed: %v", err)
		return nil, nil, fmt.Errorf("reloading config: %w", err)
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	// Find new services
	for id := range newCfg.Services {
		if _, existsProcess := o.processes[id]; existsProcess {
			continue
		}
		if _, existsCompose := o.composeManagers[id]; existsCompose {
			continue
		}
		added = append(added, id)
	}

	// Find removed services
	for id := range o.processes {
		if _, exists := newCfg.Services[id]; !exists {
			removed = append(removed, id)
		}
	}
	for id := range o.composeManagers {
		if _, exists := newCfg.Services[id]; !exists {
			removed = append(removed, id)
		}
	}

	// Add new services
	for _, id := range added {
		svc := newCfg.Services[id]
		if svc.Type == "docker-compose" {
			cm := docker.NewComposeManager(id, svc, newCfg.LogRetentionLines)
			_ = cm.DiscoverServices()
			o.composeManagers[id] = cm
		} else {
			o.processes[id] = process.NewProcess(id, svc, newCfg.LogRetentionLines)
		}
	}

	// Update configs for existing services (takes effect on next restart)
	for id, svc := range newCfg.Services {
		if p, ok := o.processes[id]; ok {
			p.Config = svc
		}
		if cm, ok := o.composeManagers[id]; ok {
			cm.Config = svc
			_ = cm.DiscoverServices()
		}
	}

	o.cfg = newCfg

	// Reinitialize commands
	o.commands = make(map[string]*command.Runner)
	if newCfg.Commands != nil {
		logsDir := newCfg.ResolvedLogsDir()
		for id, cmd := range newCfg.Commands {
			runner := command.NewRunner(id, cmd, newCfg.LogRetentionLines)
			if newCfg.CommandLogsEnabled() {
				runner.SetFileLogger(logging.NewFileLogger(logsDir, id))
			}
			o.commands[id] = runner
		}
	}

	log.Printf("config reloaded: %d added, %d removed", len(added), len(removed))
	if len(added) > 0 {
		log.Printf("  added services: %v", added)
	}
	if len(removed) > 0 {
		log.Printf("  removed services: %v", removed)
	}

	return added, removed, nil
}

// Config returns the current configuration.
func (o *Orchestrator) Config() *config.Config {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.cfg
}

func (o *Orchestrator) initCommands() {
	if o.cfg.Commands == nil {
		return
	}
	logsDir := o.cfg.ResolvedLogsDir()
	log.Printf("initializing commands from config (%d commands configured)", len(o.cfg.Commands))
	for id, cmd := range o.cfg.Commands {
		log.Printf("  creating command runner: %s (label: %s)", id, cmd.Label)
		runner := command.NewRunner(id, cmd, o.cfg.LogRetentionLines)
		if o.cfg.CommandLogsEnabled() {
			runner.SetFileLogger(logging.NewFileLogger(logsDir, id))
		}
		o.commands[id] = runner
	}
}

// ListCommands returns info for all commands.
func (o *Orchestrator) ListCommands(query, group, tag string) []types.CommandInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var result []types.CommandInfo
	for _, runner := range o.commands {
		info := runner.Info()

		// Filter by group
		if group != "" && info.Group != group {
			continue
		}

		// Filter by tag
		if tag != "" {
			found := false
			for _, t := range info.Tags {
				if t == tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Filter by search query (matches id, label, description, tags)
		if query != "" {
			q := strings.ToLower(query)
			matched := strings.Contains(strings.ToLower(info.ID), q) ||
				strings.Contains(strings.ToLower(info.Label), q) ||
				strings.Contains(strings.ToLower(info.Description), q)
			if !matched {
				for _, t := range info.Tags {
					if strings.Contains(strings.ToLower(t), q) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// RunCommand runs a command by ID.
func (o *Orchestrator) RunCommand(id string, env map[string]string, args []string) error {
	o.mu.RLock()
	runner, ok := o.commands[id]
	commands := o.commands
	o.mu.RUnlock()

	if !ok {
		return fmt.Errorf("command %s not found", id)
	}

	return runner.Run(context.Background(), commands, env, args)
}

// CancelCommand cancels a running command.
func (o *Orchestrator) CancelCommand(id string) error {
	o.mu.RLock()
	runner, ok := o.commands[id]
	o.mu.RUnlock()

	if !ok {
		return fmt.Errorf("command %s not found", id)
	}
	return runner.Cancel()
}

// GetCommandStatus returns the status of a command.
func (o *Orchestrator) GetCommandStatus(id string) (types.CommandInfo, error) {
	o.mu.RLock()
	runner, ok := o.commands[id]
	o.mu.RUnlock()

	if !ok {
		return types.CommandInfo{}, fmt.Errorf("command %s not found", id)
	}
	return runner.Info(), nil
}

// GetCommandLogs returns recent logs for a command.
func (o *Orchestrator) GetCommandLogs(id string, n int) ([]types.LogEntry, error) {
	o.mu.RLock()
	runner, ok := o.commands[id]
	o.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("command %s not found", id)
	}
	return runner.Logs(n), nil
}

// SubscribeCommandLogs subscribes to real-time log entries for a command.
func (o *Orchestrator) SubscribeCommandLogs(id string) (chan types.LogEntry, error) {
	o.mu.RLock()
	runner, ok := o.commands[id]
	o.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("command %s not found", id)
	}
	return runner.Subscribe(), nil
}

// UnsubscribeCommandLogs unsubscribes from command log entries.
func (o *Orchestrator) UnsubscribeCommandLogs(id string, ch chan types.LogEntry) {
	o.mu.RLock()
	runner, ok := o.commands[id]
	o.mu.RUnlock()

	if !ok {
		return
	}
	runner.Unsubscribe(ch)
}

// ListLogRuns returns metadata about persisted log files for a service or command.
func (o *Orchestrator) ListLogRuns(name string) ([]logging.LogRunInfo, error) {
	return logging.ListRuns(o.cfg.ResolvedLogsDir(), name)
}

// ReadLogFile reads a persisted log file.
func (o *Orchestrator) ReadLogFile(path string, n int) ([]string, error) {
	return logging.ReadLogFile(path, n)
}

func splitServiceID(id string) (parent, child string) {
	for i, c := range id {
		if c == '/' {
			return id[:i], id[i+1:]
		}
	}
	return id, ""
}

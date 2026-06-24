package process

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/anthropic/foreman/internal/types"
)

// Pid returns the current process PID (0 if not running).
func (p *Process) Pid() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pid
}

// Status returns the current service status.
func (p *Process) Status() types.ServiceStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// StartedAt returns the time the current run started.
func (p *Process) StartedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.startedAt
}

// HasHealthCheck reports whether this service declares a port health check.
func (p *Process) HasHealthCheck() bool {
	return p.Config.HealthCheck != nil && len(p.Config.HealthCheck.Ports) > 0
}

// AutoRestart reports whether the supervisor should restart this service.
func (p *Process) AutoRestart() bool {
	return p.Config.AutoRestart
}

// CheckHealth probes the declared ports and returns whether the service is
// healthy along with a human-readable note. A service is healthy when every
// declared port is in LISTEN state and owned by the service's process tree.
//
// It returns healthy=true (with reason) when a check is not applicable yet:
// no health check configured, process not running, or still within the
// post-start grace period.
func (p *Process) CheckHealth() (bool, string) {
	if !p.HasHealthCheck() {
		return true, "no health check configured"
	}

	p.mu.RLock()
	status := p.status
	pid := p.pid
	started := p.startedAt
	p.mu.RUnlock()

	if status != types.StatusRunning {
		return true, "not running"
	}
	if pid <= 0 {
		return true, "no pid yet"
	}
	if grace := p.Config.HealthCheck.GracePeriodDuration(); time.Since(started) < grace {
		return true, "within grace period"
	}

	tree := processTreePIDs(pid)
	var bad []int
	for _, port := range p.Config.HealthCheck.Ports {
		owned := false
		for _, lp := range listenersOnPort(port) {
			if tree[lp] {
				owned = true
				break
			}
		}
		if !owned {
			bad = append(bad, port)
		}
	}

	if len(bad) == 0 {
		return true, fmt.Sprintf("ports %v healthy", p.Config.HealthCheck.Ports)
	}
	return false, fmt.Sprintf("ports %v not held by process (pid %d)", bad, pid)
}

// RecordHealth stores the latest health-check result for display via Info().
func (p *Process) RecordHealth(healthy bool, note string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthChecked = true
	p.healthy = healthy
	p.healthNote = note
	if healthy {
		if p.status == types.StatusUnhealthy {
			p.status = types.StatusRunning
		}
	} else if p.status == types.StatusRunning {
		p.status = types.StatusUnhealthy
	}
}

// KillPortHolders force-kills any foreign processes (not in this service's
// process tree) that are squatting on the declared health-check ports, so a
// subsequent restart can bind cleanly. Returns the PIDs that were killed.
func (p *Process) KillPortHolders() []int {
	if !p.HasHealthCheck() {
		return nil
	}
	p.mu.RLock()
	pid := p.pid
	p.mu.RUnlock()

	tree := processTreePIDs(pid)
	self := map[int]bool{}
	killedSet := map[int]bool{}
	var killed []int
	for _, port := range p.Config.HealthCheck.Ports {
		for _, lp := range listenersOnPort(port) {
			if lp <= 0 || tree[lp] || self[lp] {
				continue
			}
			if killedSet[lp] {
				continue
			}
			log.Printf("[%s] killing foreign process %d squatting on port %d", p.ID, lp, port)
			if err := killPID(lp); err != nil {
				log.Printf("[%s] failed to kill pid %d: %v", p.ID, lp, err)
				continue
			}
			killedSet[lp] = true
			killed = append(killed, lp)
		}
	}
	sort.Ints(killed)
	return killed
}

package types

import (
	"sync"
	"time"
)

// ServiceStatus represents the current state of a service.
type ServiceStatus string

const (
	StatusStopped   ServiceStatus = "stopped"
	StatusStarting  ServiceStatus = "starting"
	StatusRunning   ServiceStatus = "running"
	StatusStopping  ServiceStatus = "stopping"
	StatusCrashed   ServiceStatus = "crashed"
	StatusUnhealthy ServiceStatus = "unhealthy"
	StatusBuilding  ServiceStatus = "building"
)

// ServiceType distinguishes native processes from docker-compose services.
type ServiceType string

const (
	TypeProcess        ServiceType = "process"
	TypeDockerCompose  ServiceType = "docker-compose"
)

// LogEntry represents a single log line from a service.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"` // "stdout" or "stderr"
	Line      string    `json:"line"`
}

// ServiceInfo is the API-facing representation of a service.
type ServiceInfo struct {
	ID         string        `json:"id"`
	Label      string        `json:"label"`
	Group      string        `json:"group,omitempty"`
	Type       ServiceType   `json:"type"`
	Status     ServiceStatus `json:"status"`
	PID        int           `json:"pid,omitempty"`
	Uptime     string        `json:"uptime,omitempty"`
	Restarts   int           `json:"restarts"`
	ExitCode   *int          `json:"exit_code,omitempty"`
	AutoStart  bool          `json:"auto_start"`
	HasBuild   bool          `json:"has_build"`
	URL        string        `json:"url,omitempty"`
	Children   []ServiceInfo `json:"children,omitempty"`
}

// LogBuffer is a thread-safe ring buffer for log entries.
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

// NewLogBuffer creates a new ring buffer with the given capacity.
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add appends a log entry, evicting the oldest if at capacity.
func (lb *LogBuffer) Add(entry LogEntry) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if len(lb.entries) >= lb.maxSize {
		lb.entries = lb.entries[1:]
	}
	lb.entries = append(lb.entries, entry)
}

// Last returns the most recent n entries.
func (lb *LogBuffer) Last(n int) []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	if n > len(lb.entries) {
		n = len(lb.entries)
	}
	result := make([]LogEntry, n)
	copy(result, lb.entries[len(lb.entries)-n:])
	return result
}

// Clear removes all entries.
func (lb *LogBuffer) Clear() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.entries = lb.entries[:0]
}

package types

// CommandStatus represents the current state of a command execution.
type CommandStatus string

const (
	CommandIdle     CommandStatus = "idle"
	CommandRunning  CommandStatus = "running"
	CommandSuccess  CommandStatus = "success"
	CommandFailed   CommandStatus = "failed"
	CommandCanceled CommandStatus = "canceled"
)

// CommandInfo is the API-facing representation of a command.
type CommandInfo struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	Group       string        `json:"group,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	Status      CommandStatus `json:"status"`
	ExitCode    *int          `json:"exit_code,omitempty"`
	Duration    string        `json:"duration,omitempty"`
	Confirm     bool          `json:"confirm"`
	DependsOn   []string      `json:"depends_on,omitempty"`
	Parallel    []string      `json:"parallel,omitempty"`
}

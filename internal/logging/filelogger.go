package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileLogger writes stdout/stderr log lines to files on disk.
// Files are stored as: <logsDir>/<name>/<nn>-<datetime>.stdout.log
// where nn is a zero-padded run counter.
type FileLogger struct {
	mu       sync.Mutex
	logsDir  string
	name     string
	runCount int
	stdout   *os.File
	stderr   *os.File
}

// NewFileLogger creates a file logger for a service or command.
func NewFileLogger(logsDir, name string) *FileLogger {
	return &FileLogger{
		logsDir: logsDir,
		name:    sanitizeName(name),
	}
}

// StartRun opens new log files for a new run of this service/command.
// Each call increments the run counter and creates timestamped files.
func (fl *FileLogger) StartRun() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	fl.closeFiles()

	dir := filepath.Join(fl.logsDir, fl.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating log directory %s: %w", dir, err)
	}

	// Determine next run number by scanning existing files
	fl.runCount = fl.nextRunNumber(dir)

	ts := time.Now().Format("20060102-150405")
	prefix := fmt.Sprintf("%03d-%s", fl.runCount, ts)

	stdoutPath := filepath.Join(dir, prefix+".stdout.log")
	stderrPath := filepath.Join(dir, prefix+".stderr.log")

	var err error
	fl.stdout, err = os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening stdout log: %w", err)
	}

	fl.stderr, err = os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fl.stdout.Close()
		fl.stdout = nil
		return fmt.Errorf("opening stderr log: %w", err)
	}

	return nil
}

// WriteLog writes a timestamped line to the appropriate log file.
func (fl *FileLogger) WriteLog(stream, line string) {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	ts := time.Now().Format("2006-01-02T15:04:05.000")
	formatted := fmt.Sprintf("[%s] %s\n", ts, line)

	if stream == "stderr" {
		if fl.stderr != nil {
			fl.stderr.WriteString(formatted)
		}
	} else {
		if fl.stdout != nil {
			fl.stdout.WriteString(formatted)
		}
	}
}

// Close closes open log files.
func (fl *FileLogger) Close() {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fl.closeFiles()
}

func (fl *FileLogger) closeFiles() {
	if fl.stdout != nil {
		fl.stdout.Close()
		fl.stdout = nil
	}
	if fl.stderr != nil {
		fl.stderr.Close()
		fl.stderr = nil
	}
}

func (fl *FileLogger) nextRunNumber(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}
	maxNum := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Parse "NNN-..." prefix
		if len(name) >= 3 {
			n := 0
			for i := 0; i < 3 && i < len(name); i++ {
				if name[i] >= '0' && name[i] <= '9' {
					n = n*10 + int(name[i]-'0')
				} else {
					break
				}
			}
			if n > maxNum {
				maxNum = n
			}
		}
	}
	return maxNum + 1
}

// LogRunInfo holds metadata about a single logged run.
type LogRunInfo struct {
	RunNumber  int    `json:"run_number"`
	Timestamp  string `json:"timestamp"`
	StdoutFile string `json:"stdout_file"`
	StderrFile string `json:"stderr_file"`
	StdoutSize int64  `json:"stdout_size"`
	StderrSize int64  `json:"stderr_size"`
}

// ListRuns returns metadata about all logged runs for a given service/command name.
func ListRuns(logsDir, name string) ([]LogRunInfo, error) {
	dir := filepath.Join(logsDir, sanitizeName(name))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Group by run number
	runs := make(map[int]*LogRunInfo)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fname := e.Name()
		if !strings.HasSuffix(fname, ".log") {
			continue
		}

		// Parse "NNN-YYYYMMDD-HHMMSS.stdout.log"
		parts := strings.SplitN(fname, "-", 2)
		if len(parts) < 2 {
			continue
		}
		n := 0
		for _, c := range parts[0] {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		if n == 0 {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		run, ok := runs[n]
		if !ok {
			// Extract timestamp from filename: NNN-YYYYMMDD-HHMMSS.stdout.log
			ts := ""
			rest := parts[1] // "YYYYMMDD-HHMMSS.stdout.log"
			if idx := strings.Index(rest, ".stdout.log"); idx > 0 {
				ts = rest[:idx]
			} else if idx := strings.Index(rest, ".stderr.log"); idx > 0 {
				ts = rest[:idx]
			}
			run = &LogRunInfo{RunNumber: n, Timestamp: ts}
			runs[n] = run
		}

		fullPath := filepath.Join(dir, fname)
		if strings.Contains(fname, ".stdout.log") {
			run.StdoutFile = fullPath
			run.StdoutSize = info.Size()
		} else if strings.Contains(fname, ".stderr.log") {
			run.StderrFile = fullPath
			run.StderrSize = info.Size()
		}
	}

	// Sort by run number descending (newest first)
	result := make([]LogRunInfo, 0, len(runs))
	for _, r := range runs {
		result = append(result, *r)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].RunNumber > result[j].RunNumber
	})

	return result, nil
}

// ReadLogFile reads a log file and returns the last n lines.
func ReadLogFile(path string, n int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func sanitizeName(name string) string {
	// Replace path separators and problematic characters
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return r.Replace(name)
}

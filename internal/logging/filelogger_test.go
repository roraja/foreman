package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileLoggerCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir, "test-service")

	if err := fl.StartRun(); err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	defer fl.Close()

	fl.WriteLog("stdout", "hello stdout")
	fl.WriteLog("stderr", "hello stderr")
	fl.Close()

	// Check directory was created
	entries, err := os.ReadDir(filepath.Join(dir, "test-service"))
	if err != nil {
		t.Fatalf("reading log dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files (stdout + stderr), got %d", len(entries))
	}

	// Check filenames
	var hasStdout, hasStderr bool
	for _, e := range entries {
		if strings.Contains(e.Name(), ".stdout.log") {
			hasStdout = true
		}
		if strings.Contains(e.Name(), ".stderr.log") {
			hasStderr = true
		}
	}
	if !hasStdout {
		t.Error("expected .stdout.log file")
	}
	if !hasStderr {
		t.Error("expected .stderr.log file")
	}
}

func TestFileLoggerRunNumbering(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir, "counter-test")

	// First run
	if err := fl.StartRun(); err != nil {
		t.Fatalf("StartRun 1 failed: %v", err)
	}
	fl.WriteLog("stdout", "run 1")
	fl.Close()

	// Second run
	if err := fl.StartRun(); err != nil {
		t.Fatalf("StartRun 2 failed: %v", err)
	}
	fl.WriteLog("stdout", "run 2")
	fl.Close()

	entries, err := os.ReadDir(filepath.Join(dir, "counter-test"))
	if err != nil {
		t.Fatalf("reading log dir: %v", err)
	}

	// Should have 4 files: 2 runs × 2 streams
	if len(entries) != 4 {
		t.Fatalf("expected 4 files, got %d", len(entries))
	}

	// Check run numbers
	has001, has002 := false, false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "001-") {
			has001 = true
		}
		if strings.HasPrefix(e.Name(), "002-") {
			has002 = true
		}
	}
	if !has001 {
		t.Error("expected file starting with 001-")
	}
	if !has002 {
		t.Error("expected file starting with 002-")
	}
}

func TestFileLoggerContent(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir, "content-test")

	if err := fl.StartRun(); err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	fl.WriteLog("stdout", "line one")
	fl.WriteLog("stdout", "line two")
	fl.WriteLog("stderr", "error line")
	fl.Close()

	entries, _ := os.ReadDir(filepath.Join(dir, "content-test"))
	for _, e := range entries {
		path := filepath.Join(dir, "content-test", e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		content := string(data)
		if strings.Contains(e.Name(), ".stdout.log") {
			if !strings.Contains(content, "line one") {
				t.Errorf("stdout log missing 'line one': %s", content)
			}
			if !strings.Contains(content, "line two") {
				t.Errorf("stdout log missing 'line two': %s", content)
			}
		}
		if strings.Contains(e.Name(), ".stderr.log") {
			if !strings.Contains(content, "error line") {
				t.Errorf("stderr log missing 'error line': %s", content)
			}
		}
	}
}

func TestFileLoggerTimestampFormat(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir, "ts-test")

	if err := fl.StartRun(); err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	fl.WriteLog("stdout", "timestamped")
	fl.Close()

	entries, _ := os.ReadDir(filepath.Join(dir, "ts-test"))
	for _, e := range entries {
		if !strings.Contains(e.Name(), ".stdout.log") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(dir, "ts-test", e.Name()))
		content := string(data)
		// Should have [YYYY-MM-DDTHH:MM:SS.mmm] prefix
		if !strings.Contains(content, "[20") {
			t.Errorf("expected timestamp prefix, got: %s", content)
		}
	}
}

func TestListRuns(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir, "list-test")

	// Create 3 runs
	for i := 0; i < 3; i++ {
		if err := fl.StartRun(); err != nil {
			t.Fatalf("StartRun %d failed: %v", i, err)
		}
		fl.WriteLog("stdout", "run output")
		fl.Close()
	}

	runs, err := ListRuns(dir, "list-test")
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}

	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}

	// Should be sorted newest first
	if runs[0].RunNumber != 3 {
		t.Errorf("expected newest run (3) first, got %d", runs[0].RunNumber)
	}
	if runs[2].RunNumber != 1 {
		t.Errorf("expected oldest run (1) last, got %d", runs[2].RunNumber)
	}

	// Each run should have both files
	for _, run := range runs {
		if run.StdoutFile == "" {
			t.Errorf("run %d missing stdout file", run.RunNumber)
		}
		if run.StderrFile == "" {
			t.Errorf("run %d missing stderr file", run.RunNumber)
		}
	}
}

func TestListRunsEmpty(t *testing.T) {
	dir := t.TempDir()
	runs, err := ListRuns(dir, "nonexistent")
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if runs != nil {
		t.Errorf("expected nil for nonexistent, got %v", runs)
	}
}

func TestReadLogFile(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir, "read-test")

	if err := fl.StartRun(); err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	for i := 0; i < 10; i++ {
		fl.WriteLog("stdout", "line")
	}
	fl.Close()

	runs, _ := ListRuns(dir, "read-test")
	if len(runs) == 0 {
		t.Fatal("no runs found")
	}

	// Read all lines
	lines, err := ReadLogFile(runs[0].StdoutFile, 0)
	if err != nil {
		t.Fatalf("ReadLogFile failed: %v", err)
	}
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}

	// Read last 3 lines
	lines, err = ReadLogFile(runs[0].StdoutFile, 3)
	if err != nil {
		t.Fatalf("ReadLogFile(3) failed: %v", err)
	}
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"simple", "simple"},
		{"with/slash", "with_slash"},
		{"with spaces", "with_spaces"},
		{"a:b:c", "a_b_c"},
	}
	for _, tc := range cases {
		got := sanitizeName(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFileLoggerMultipleWritesBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir, "multi-write")

	// Start first run
	if err := fl.StartRun(); err != nil {
		t.Fatal(err)
	}
	fl.WriteLog("stdout", "first run line 1")
	fl.WriteLog("stdout", "first run line 2")

	// Start second run (should close first)
	if err := fl.StartRun(); err != nil {
		t.Fatal(err)
	}
	fl.WriteLog("stdout", "second run line 1")
	fl.Close()

	runs, _ := ListRuns(dir, "multi-write")
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	// Verify first run has 2 lines, second has 1
	lines1, _ := ReadLogFile(runs[1].StdoutFile, 0) // oldest (run 1)
	lines2, _ := ReadLogFile(runs[0].StdoutFile, 0) // newest (run 2)
	if len(lines1) != 2 {
		t.Errorf("run 1: expected 2 lines, got %d", len(lines1))
	}
	if len(lines2) != 1 {
		t.Errorf("run 2: expected 1 line, got %d", len(lines2))
	}
}

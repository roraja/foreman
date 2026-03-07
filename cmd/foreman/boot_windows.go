//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// platformInstall sets up a Windows startup shortcut and scheduled task for foreman.
func platformInstall(foremanBin, foremanConfig, foremanDir string, port int, host string, noStart bool) error {
	// Write foreman.yaml
	fmt.Printf("Writing config to %s\n", foremanConfig)
	configContent := "# Foreman global service configuration\r\n" +
		"# Add services and commands here to manage them via the foreman dashboard.\r\n" +
		"#\r\n" +
		"# Documentation: foreman help\r\n" +
		"\r\n" +
		"project_root: " + foremanDir + "\r\n" +
		"port: " + strconv.Itoa(port) + "\r\n" +
		"host: " + host + "\r\n" +
		"log_retention_lines: 10000\r\n" +
		"logs_dir: " + filepath.Join(foremanDir, "logs") + "\r\n" +
		"\r\n" +
		"# Import additional config files:\r\n" +
		"# imports:\r\n" +
		"#   - services.yaml\r\n" +
		"\r\n" +
		"services: {}\r\n" +
		"commands: {}\r\n"

	if err := os.WriteFile(foremanConfig, []byte(configContent), 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Create a scheduled task that runs at user logon
	taskName := "ForemanService"
	fmt.Println("Creating scheduled task for foreman...")

	// Delete existing task if present (ignore errors)
	_ = exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run()

	args := []string{
		"/Create",
		"/TN", taskName,
		"/TR", fmt.Sprintf(`"%s" -c "%s"`, foremanBin, foremanConfig),
		"/SC", "ONLOGON",
		"/RL", "LIMITED",
		"/F",
	}
	if out, err := exec.Command("schtasks", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("creating scheduled task: %w\n%s", err, out)
	}

	if !noStart {
		fmt.Println("Starting foreman...")
		if out, err := exec.Command("schtasks", "/Run", "/TN", taskName).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not start task: %v\n%s\n", err, out)
		}
	}

	fmt.Println()
	fmt.Println("✓ Foreman installed successfully!")
	fmt.Printf("  Binary:  %s\n", foremanBin)
	fmt.Printf("  Config:  %s\n", foremanConfig)
	fmt.Printf("  Task:    %s (runs at logon)\n", taskName)
	fmt.Printf("  Dashboard: http://%s:%d\n", host, port)
	fmt.Println()
	fmt.Println("Useful commands:")
	fmt.Println(`  schtasks /Query /TN ForemanService       Check task status`)
	fmt.Println(`  schtasks /Run /TN ForemanService         Start the task`)
	fmt.Println(`  schtasks /End /TN ForemanService         Stop the task`)
	fmt.Println(`  schtasks /Delete /TN ForemanService /F   Remove the task`)

	return nil
}

// isPlatformBootInstalled checks if the Windows scheduled task exists.
func isPlatformBootInstalled() bool {
	err := exec.Command("schtasks", "/Query", "/TN", "ForemanService").Run()
	return err == nil
}

// platformRestartHint returns the command to restart the global foreman on Windows.
func platformRestartHint() string {
	return `  schtasks /End /TN ForemanService && schtasks /Run /TN ForemanService`
}

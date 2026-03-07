//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// platformInstall sets up a launchd user agent for foreman.
func platformInstall(foremanBin, foremanConfig, foremanDir string, port int, host string, noStart bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	plistFile := filepath.Join(launchAgentsDir, "com.foreman.agent.plist")

	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", launchAgentsDir, err)
	}

	// Write foreman.yaml
	fmt.Printf("Writing config to %s\n", foremanConfig)
	configContent := "# Foreman global service configuration\n" +
		"# Add services and commands here to manage them via the foreman dashboard.\n" +
		"#\n" +
		"# Documentation: foreman help\n" +
		"\n" +
		"project_root: " + foremanDir + "\n" +
		"port: " + strconv.Itoa(port) + "\n" +
		"host: " + host + "\n" +
		"log_retention_lines: 10000\n" +
		"logs_dir: " + filepath.Join(foremanDir, "logs") + "\n" +
		"\n" +
		"# Import additional config files:\n" +
		"# imports:\n" +
		"#   - services.yaml\n" +
		"\n" +
		"services: {}\n" +
		"commands: {}\n"

	if err := os.WriteFile(foremanConfig, []byte(configContent), 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Write launchd plist
	fmt.Printf("Writing launchd agent to %s\n", plistFile)
	logPath := filepath.Join(foremanDir, "logs", "daemon.log")
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.foreman.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + foremanBin + `</string>
        <string>-c</string>
        <string>` + foremanConfig + `</string>
    </array>
    <key>WorkingDirectory</key>
    <string>` + foremanDir + `</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>StandardOutPath</key>
    <string>` + logPath + `</string>
    <key>StandardErrorPath</key>
    <string>` + logPath + `</string>
</dict>
</plist>
`

	if err := os.WriteFile(plistFile, []byte(plistContent), 0o644); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	if !noStart {
		fmt.Println("Loading foreman agent...")
		// Use launchctl bootstrap for modern macOS, fall back to load
		if out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), plistFile).CombinedOutput(); err != nil {
			// Fall back to legacy load for older macOS
			if out2, err2 := exec.Command("launchctl", "load", "-w", plistFile).CombinedOutput(); err2 != nil {
				fmt.Fprintf(os.Stderr, "Warning: launchctl load failed: %v\n%s\n%s\n", err2, out, out2)
			}
		}
	}

	fmt.Println()
	fmt.Println("✓ Foreman installed successfully!")
	fmt.Printf("  Binary:  %s\n", foremanBin)
	fmt.Printf("  Config:  %s\n", foremanConfig)
	fmt.Printf("  Agent:   %s\n", plistFile)
	fmt.Printf("  Dashboard: http://%s:%d\n", host, port)
	fmt.Println()
	fmt.Println("Useful commands:")
	fmt.Println("  launchctl list | grep foreman       Check if running")
	fmt.Println("  launchctl kickstart -k gui/$(id -u)/com.foreman.agent   Restart")
	fmt.Println("  launchctl bootout gui/$(id -u)/com.foreman.agent        Stop")

	return nil
}

// isPlatformBootInstalled checks if the launchd plist exists.
func isPlatformBootInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	plistFile := filepath.Join(home, "Library", "LaunchAgents", "com.foreman.agent.plist")
	_, err = os.Stat(plistFile)
	return err == nil
}

// platformRestartHint returns the command to restart the global foreman on macOS.
func platformRestartHint() string {
	return "  launchctl kickstart -k gui/$(id -u)/com.foreman.agent"
}

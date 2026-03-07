//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// platformInstall sets up a systemd user service for foreman.
func platformInstall(foremanBin, foremanConfig, foremanDir string, port int, host string, noStart bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	systemdDir := filepath.Join(home, ".config", "systemd", "user")
	unitFile := filepath.Join(systemdDir, "foreman.service")

	if err := os.MkdirAll(systemdDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", systemdDir, err)
	}

	// Write foreman.yaml if it doesn't exist
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

	// Write systemd unit file
	fmt.Printf("Writing systemd unit to %s\n", unitFile)
	unitContent := "[Unit]\n" +
		"Description=Foreman — local services monitor and command runner\n" +
		"After=network.target\n" +
		"\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + foremanBin + " -c " + foremanConfig + "\n" +
		"Restart=on-failure\n" +
		"RestartSec=5\n" +
		"WorkingDirectory=" + foremanDir + "\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"

	if err := os.WriteFile(unitFile, []byte(unitContent), 0o644); err != nil {
		return fmt.Errorf("writing systemd unit: %w", err)
	}

	// Reload systemd and enable the service
	fmt.Println("Enabling foreman service...")
	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: systemctl daemon-reload failed: %v\n%s\n", err, out)
	}
	if out, err := exec.Command("systemctl", "--user", "enable", "foreman.service").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: systemctl enable failed: %v\n%s\n", err, out)
	}

	if !noStart {
		fmt.Println("Starting foreman service...")
		if out, err := exec.Command("systemctl", "--user", "start", "foreman.service").CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: systemctl start failed: %v\n%s\n", err, out)
		}
	}

	fmt.Println()
	fmt.Println("✓ Foreman installed successfully!")
	fmt.Printf("  Binary:  %s\n", foremanBin)
	fmt.Printf("  Config:  %s\n", foremanConfig)
	fmt.Printf("  Service: %s\n", unitFile)
	fmt.Printf("  Dashboard: http://%s:%d\n", host, port)
	fmt.Println()
	fmt.Println("Useful commands:")
	fmt.Println("  systemctl --user status foreman    Check service status")
	fmt.Println("  systemctl --user restart foreman   Restart the service")
	fmt.Println("  systemctl --user stop foreman      Stop the service")
	fmt.Println("  journalctl --user -u foreman -f    Follow logs")

	return nil
}

// isPlatformBootInstalled checks if the systemd unit file exists.
func isPlatformBootInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	unitFile := filepath.Join(home, ".config", "systemd", "user", "foreman.service")
	_, err = os.Stat(unitFile)
	return err == nil
}

// platformRestartHint returns the command to restart the global foreman on this OS.
func platformRestartHint() string {
	return "  systemctl --user restart foreman"
}

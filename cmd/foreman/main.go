package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/anthropic/foreman/internal/command"
	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/logging"
	"github.com/anthropic/foreman/internal/orchestrator"
	"github.com/anthropic/foreman/internal/server"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "commands":
			cmdCommands(os.Args[2:])
			return
		case "run":
			cmdRun(os.Args[2:])
			return
		case "install":
			cmdInstall(os.Args[2:])
			return
		case "runOnBoot":
			cmdRunOnBoot(os.Args[2:])
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	// Default: start the server
	cmdServe(os.Args[1:])
}

func printUsage() {
	fmt.Print(`Foreman — local services monitor and command runner

Usage:
  foreman [flags]                           Start the web dashboard
  foreman commands [flags]                  List configured commands
  foreman run <command-id> [flags] [-- extra-args]  Run a command
  foreman install [flags]                   Install foreman as a user systemd service
  foreman runOnBoot [flags]                 Register this project to start on boot
  foreman help                              Show this help

Server flags:
  -c, --config <path>     Path to foreman.yaml (default: foreman.yaml)

Commands flags:
  -c, --config <path>     Path to foreman.yaml (default: foreman.yaml)
  -group <name>           Filter commands by group
  -tag <name>             Filter commands by tag
  -q <query>              Search commands by name, label, or description

Run flags:
  -c, --config <path>     Path to foreman.yaml (default: foreman.yaml)
  --dry-run               Show resolved command without executing
  --parallel              Run multiple commands in parallel
  --cwd <path>            Override working directory
  --env KEY=value         Set environment variable (repeatable)
  -- <args>               Extra arguments appended to the command

Install flags:
  --port <port>           Port for the foreman service (default: 9090)
  --host <host>           Host for the foreman service (default: 127.0.0.1)
  --no-start              Don't start the service after installing

RunOnBoot flags:
  -c, --config <path>     Path to foreman.yaml (default: foreman.yaml)
  --id <name>             Service ID in global config (default: derived from folder name)

Examples:
  foreman -c foreman.yaml                         Start dashboard
  foreman commands -c foreman.yaml                List all commands
  foreman commands -c foreman.yaml -group build   Filter by group
  foreman run install -c foreman.yaml             Run a command
  foreman run install -c foreman.yaml --dry-run   Preview execution
  foreman run lint test --parallel                 Run in parallel
  foreman run build --env NODE_ENV=production      With env override
  foreman run build -- --verbose                   With extra args
  foreman install                                  Install global foreman service
  foreman install --port 8080                      Install on custom port
  foreman runOnBoot                                Register current project to start on boot
  foreman runOnBoot -c myconfig.yaml --id my-proj  Register with custom ID
`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("foreman", flag.ExitOnError)
	configPath := fs.String("c", "foreman.yaml", "Path to foreman.yaml config file")
	fs.StringVar(configPath, "config", "foreman.yaml", "Path to foreman.yaml config file")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Set up file logging if configured
	if cfg.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating log directory: %v\n", err)
			os.Exit(1)
		}
		logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening log file %s: %v\n", cfg.LogFile, err)
			os.Exit(1)
		}
		defer logFile.Close()
		multiWriter := io.MultiWriter(os.Stderr, logFile)
		log.SetOutput(multiWriter)
		log.Printf("Logging to file: %s", cfg.LogFile)
	}

	log.Printf("Foreman starting — project root: %s", cfg.ProjectRoot)

	// Set up daemon file logging
	if cfg.DaemonLogEnabled() {
		daemonLogPath := filepath.Join(cfg.ResolvedLogsDir(), "daemon.log")
		if err := os.MkdirAll(filepath.Dir(daemonLogPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create daemon log dir: %v\n", err)
		} else {
			dlf, err := os.OpenFile(daemonLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not open daemon log %s: %v\n", daemonLogPath, err)
			} else {
				defer dlf.Close()
				log.SetOutput(io.MultiWriter(os.Stderr, dlf))
				log.Printf("Daemon log: %s", daemonLogPath)
			}
		}
	}

	orch := orchestrator.New(cfg, *configPath)
	srv := server.New(orch, cfg.Password, cfg.APIToken)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Web UI: http://%s", addr)

	orch.StartAutoStart()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down...")
		orch.StopAll()
		os.Exit(0)
	}()

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func cmdCommands(args []string) {
	fs := flag.NewFlagSet("commands", flag.ExitOnError)
	configPath := fs.String("c", "foreman.yaml", "Path to foreman.yaml config file")
	fs.StringVar(configPath, "config", "foreman.yaml", "Path to foreman.yaml config file")
	group := fs.String("group", "", "Filter by group")
	tag := fs.String("tag", "", "Filter by tag")
	query := fs.String("q", "", "Search query")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	orch := orchestrator.New(cfg, *configPath)
	commands := orch.ListCommands(*query, *group, *tag)

	if len(commands) == 0 {
		fmt.Println("No commands found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tLABEL\tGROUP\tDESCRIPTION\tTAGS")
	fmt.Fprintln(w, "──\t─────\t─────\t───────────\t────")
	for _, cmd := range commands {
		tags := ""
		if len(cmd.Tags) > 0 {
			tags = strings.Join(cmd.Tags, ", ")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", cmd.ID, cmd.Label, cmd.Group, cmd.Description, tags)
	}
	w.Flush()
}

func cmdRun(args []string) {
	// Manual argument parsing to support mixed flag/positional ordering and --env
	var configPath = "foreman.yaml"
	var dryRun, parallel bool
	var cwd string
	var envOverrides []string
	var commandIDs []string
	var extraArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			extraArgs = args[i+1:]
			break
		}
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) {
				i++
				configPath = args[i]
			}
		case strings.HasPrefix(arg, "-c="):
			configPath = arg[3:]
		case strings.HasPrefix(arg, "--config="):
			configPath = arg[9:]
		case arg == "--dry-run":
			dryRun = true
		case arg == "--parallel":
			parallel = true
		case arg == "--cwd":
			if i+1 < len(args) {
				i++
				cwd = args[i]
			}
		case strings.HasPrefix(arg, "--cwd="):
			cwd = arg[6:]
		case arg == "--env":
			if i+1 < len(args) {
				i++
				envOverrides = append(envOverrides, args[i])
			}
		case strings.HasPrefix(arg, "--env="):
			envOverrides = append(envOverrides, arg[6:])
		case !strings.HasPrefix(arg, "-"):
			commandIDs = append(commandIDs, arg)
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
			os.Exit(1)
		}
	}

	if len(commandIDs) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: foreman run <command-id> [<command-id>...] [--env KEY=value] [-- extra-args...]")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Build env map from overrides
	envMap := make(map[string]string)
	for _, e := range envOverrides {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Apply cwd override
	if cwd != "" {
		for _, cmd := range cfg.Commands {
			cmd.WorkingDir = cwd
		}
	}

	// Build command runners map with file loggers
	commands := make(map[string]*command.Runner)
	for id, cmd := range cfg.Commands {
		runner := command.NewRunner(id, cmd, cfg.LogRetentionLines)
		if cfg.CommandLogsEnabled() {
			runner.SetFileLogger(logging.NewFileLogger(cfg.ResolvedLogsDir(), id))
		}
		commands[id] = runner
	}

	if dryRun {
		for _, id := range commandIDs {
			cmd, ok := cfg.Commands[id]
			if !ok {
				fmt.Fprintf(os.Stderr, "Command %q not found\n", id)
				os.Exit(1)
			}
			resolved, resolvedArgs := cmd.ResolvedCommand()
			fmt.Printf("Command: %s\n", id)
			fmt.Printf("  Executable: %s\n", resolved)
			fmt.Printf("  Args: %v\n", resolvedArgs)
			fmt.Printf("  Working Dir: %s\n", cmd.WorkingDir)
			fmt.Printf("  Shell: %v\n", cmd.IsShell())
			if len(cmd.Env) > 0 {
				fmt.Printf("  Env:\n")
				for k, v := range cmd.Env {
					fmt.Printf("    %s=%s\n", k, v)
				}
			}
			if len(cmd.DependsOn) > 0 {
				fmt.Printf("  Depends On: %v\n", cmd.DependsOn)
			}
			if len(cmd.Parallel) > 0 {
				fmt.Printf("  Parallel: %v\n", cmd.Parallel)
			}
			fmt.Println()
		}
		return
	}

	ctx := context.Background()

	if parallel && len(commandIDs) > 1 {
		// Run all commands in parallel
		errCh := make(chan error, len(commandIDs))
		for _, id := range commandIDs {
			runner, ok := commands[id]
			if !ok {
				fmt.Fprintf(os.Stderr, "Command %q not found\n", id)
				os.Exit(1)
			}
			go func(r *command.Runner) {
				errCh <- r.Run(ctx, commands, envMap, extraArgs)
			}(runner)
		}
		var failed bool
		for range commandIDs {
			if err := <-errCh; err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				failed = true
			}
		}
		if failed {
			os.Exit(1)
		}
	} else {
		// Run sequentially
		for _, id := range commandIDs {
			runner, ok := commands[id]
			if !ok {
				fmt.Fprintf(os.Stderr, "Command %q not found\n", id)
				os.Exit(1)
			}
			if err := runner.Run(ctx, commands, envMap, extraArgs); err != nil {
				fmt.Fprintf(os.Stderr, "Command %s failed: %v\n", id, err)
				os.Exit(1)
			}
		}
	}
}

func cmdInstall(args []string) {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "Error: foreman install is only supported on Linux (requires systemd)")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("install", flag.ExitOnError)
	port := fs.Int("port", 9090, "Port for the foreman service")
	host := fs.String("host", "127.0.0.1", "Host for the foreman service")
	noStart := fs.Bool("no-start", false, "Don't start the service after installing")
	_ = fs.Parse(args)

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	foremanDir := filepath.Join(home, ".foreman")
	foremanBin := filepath.Join(foremanDir, "foreman")
	foremanConfig := filepath.Join(foremanDir, "foreman.yaml")
	systemdDir := filepath.Join(home, ".config", "systemd", "user")
	unitFile := filepath.Join(systemdDir, "foreman.service")

	// Create directories
	for _, dir := range []string{foremanDir, systemdDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// Copy current binary to ~/.foreman/foreman
	selfPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine current executable path: %v\n", err)
		os.Exit(1)
	}
	selfPath, err = filepath.EvalSymlinks(selfPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot resolve executable path: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Copying foreman binary to %s\n", foremanBin)
	if err := copyFile(selfPath, foremanBin, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying binary: %v\n", err)
		os.Exit(1)
	}

	// Write foreman.yaml
	fmt.Printf("Writing config to %s\n", foremanConfig)
	configContent := "# Foreman global service configuration\n" +
		"# Add services and commands here to manage them via the foreman dashboard.\n" +
		"#\n" +
		"# Documentation: foreman help\n" +
		"\n" +
		"project_root: " + foremanDir + "\n" +
		"port: " + strconv.Itoa(*port) + "\n" +
		"host: " + *host + "\n" +
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
		fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "Error writing systemd unit: %v\n", err)
		os.Exit(1)
	}

	// Reload systemd and enable the service
	fmt.Println("Enabling foreman service...")
	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: systemctl daemon-reload failed: %v\n%s\n", err, out)
	}
	if out, err := exec.Command("systemctl", "--user", "enable", "foreman.service").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: systemctl enable failed: %v\n%s\n", err, out)
	}

	if !*noStart {
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
	fmt.Printf("  Dashboard: http://%s:%d\n", *host, *port)
	fmt.Println()
	fmt.Println("Useful commands:")
	fmt.Println("  systemctl --user status foreman    Check service status")
	fmt.Println("  systemctl --user restart foreman   Restart the service")
	fmt.Println("  systemctl --user stop foreman      Stop the service")
	fmt.Println("  journalctl --user -u foreman -f    Follow logs")
}

func cmdRunOnBoot(args []string) {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "Error: foreman runOnBoot is only supported on Linux (requires systemd)")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("runOnBoot", flag.ExitOnError)
	configPath := fs.String("c", "foreman.yaml", "Path to foreman.yaml config file")
	fs.StringVar(configPath, "config", "foreman.yaml", "Path to foreman.yaml config file")
	serviceID := fs.String("id", "", "Service ID in global config (default: derived from folder name)")
	_ = fs.Parse(args)

	// Resolve the local config to an absolute path
	absConfig, err := filepath.Abs(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving config path: %v\n", err)
		os.Exit(1)
	}

	// Validate local config exists and is loadable
	if _, err := os.Stat(absConfig); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: config file not found: %s\n", absConfig)
		os.Exit(1)
	}
	if _, err := config.Load(absConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config %s: %v\n", absConfig, err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	foremanDir := filepath.Join(home, ".foreman")
	foremanBin := filepath.Join(foremanDir, "foreman")
	globalConfig := filepath.Join(foremanDir, "foreman.yaml")

	// Ensure foreman is installed as a systemd service
	if !isBootloaderInstalled(foremanBin, globalConfig) {
		fmt.Println("Foreman bootloader not installed. Running install...")
		cmdInstall([]string{})
		fmt.Println()
	}

	// Derive service ID from folder name if not provided
	projectDir := filepath.Dir(absConfig)
	if *serviceID == "" {
		*serviceID = sanitizeServiceID(filepath.Base(projectDir))
	}

	// Read and update global config
	globalData, err := os.ReadFile(globalConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading global config: %v\n", err)
		os.Exit(1)
	}

	var globalCfg yaml.Node
	if err := yaml.Unmarshal(globalData, &globalCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing global config: %v\n", err)
		os.Exit(1)
	}

	if err := upsertService(&globalCfg, *serviceID, foremanBin, absConfig, projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating global config: %v\n", err)
		os.Exit(1)
	}

	out, err := yaml.Marshal(&globalCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling global config: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(globalConfig, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing global config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Registered service %q in %s\n", *serviceID, globalConfig)
	fmt.Printf("  Config:      %s\n", absConfig)
	fmt.Printf("  Working Dir: %s\n", projectDir)
	fmt.Println()
	fmt.Println("The global foreman will start this project's foreman on boot.")
	fmt.Println("To apply immediately, restart the global foreman:")
	fmt.Println("  systemctl --user restart foreman")
}

// isBootloaderInstalled checks if the foreman binary and global config exist.
func isBootloaderInstalled(foremanBin, globalConfig string) bool {
	if _, err := os.Stat(foremanBin); os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(globalConfig); os.IsNotExist(err) {
		return false
	}
	// Check systemd unit exists
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	unitFile := filepath.Join(home, ".config", "systemd", "user", "foreman.service")
	if _, err := os.Stat(unitFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// sanitizeServiceID converts a folder name into a valid YAML key / service ID.
func sanitizeServiceID(name string) string {
	// Replace non-alphanumeric chars (except hyphens) with hyphens
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "project"
	}
	return id
}

// upsertService adds or updates a service entry in the global config YAML node tree.
func upsertService(doc *yaml.Node, id, foremanBin, configPath, workingDir string) error {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("invalid YAML document")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping at root")
	}

	// Find or create the "services" key
	var servicesValue *yaml.Node
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "services" {
			servicesValue = root.Content[i+1]
			break
		}
	}

	if servicesValue == nil {
		// Add a "services" key
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "services"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
		servicesValue = root.Content[len(root.Content)-1]
	}

	// If services was an empty scalar (e.g., `services: {}`), convert to mapping
	if servicesValue.Kind == yaml.ScalarNode || (servicesValue.Kind == yaml.MappingNode && servicesValue.Tag == "!!map" && len(servicesValue.Content) == 0) {
		servicesValue.Kind = yaml.MappingNode
		servicesValue.Tag = "!!map"
		servicesValue.Content = nil
	}

	// Check if this service ID already exists and update it
	for i := 0; i < len(servicesValue.Content)-1; i += 2 {
		if servicesValue.Content[i].Value == id {
			// Update the existing entry
			servicesValue.Content[i+1] = buildServiceNode(foremanBin, configPath, workingDir)
			return nil
		}
	}

	// Add new service entry
	servicesValue.Content = append(servicesValue.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: id},
		buildServiceNode(foremanBin, configPath, workingDir),
	)
	return nil
}

// buildServiceNode creates a YAML mapping node for the foreman service entry.
func buildServiceNode(foremanBin, configPath, workingDir string) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "command"},
			{Kind: yaml.ScalarNode, Value: foremanBin},
			{Kind: yaml.ScalarNode, Value: "args"},
			{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "-c"},
				{Kind: yaml.ScalarNode, Value: configPath},
			}},
			{Kind: yaml.ScalarNode, Value: "working_dir"},
			{Kind: yaml.ScalarNode, Value: workingDir},
			{Kind: yaml.ScalarNode, Value: "auto_start"},
			{Kind: yaml.ScalarNode, Value: "true", Tag: "!!bool"},
		},
	}
}

func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	return os.WriteFile(dst, data, perm)
}

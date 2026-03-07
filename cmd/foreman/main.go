package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/anthropic/foreman/internal/command"
	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/orchestrator"
	"github.com/anthropic/foreman/internal/server"
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

Examples:
  foreman -c foreman.yaml                         Start dashboard
  foreman commands -c foreman.yaml                List all commands
  foreman commands -c foreman.yaml -group build   Filter by group
  foreman run install -c foreman.yaml             Run a command
  foreman run install -c foreman.yaml --dry-run   Preview execution
  foreman run lint test --parallel                 Run in parallel
  foreman run build --env NODE_ENV=production      With env override
  foreman run build -- --verbose                   With extra args
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

	// Build command runners map
	commands := make(map[string]*command.Runner)
	for id, cmd := range cfg.Commands {
		commands[id] = command.NewRunner(id, cmd, cfg.LogRetentionLines)
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

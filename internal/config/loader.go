package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for Foreman.
type Config struct {
	ProjectRoot       string                     `yaml:"project_root"`
	EnvFile           string                     `yaml:"env_file"`
	Password          string                     `yaml:"password"`
	Port              int                        `yaml:"port"`
	Host              string                     `yaml:"host"`
	AutoStartOnLogin  bool                       `yaml:"auto_start_on_login"`
	LogRetentionLines int                        `yaml:"log_retention_lines"`
	LogFile           string                     `yaml:"log_file"`
	LogsDir           string                     `yaml:"logs_dir"`
	EnableServiceLogs *bool                      `yaml:"enable_service_logs"`
	EnableCommandLogs *bool                      `yaml:"enable_command_logs"`
	EnableDaemonLog   *bool                      `yaml:"enable_daemon_log"`
	APIToken          string                     `yaml:"api_token"`
	Imports           []string                   `yaml:"imports"`
	Commands          map[string]*CommandConfig  `yaml:"commands"`
	Services          map[string]*ServiceConfig  `yaml:"services"`
}

// ServiceLogsEnabled returns whether service file logging is enabled (default: true).
func (c *Config) ServiceLogsEnabled() bool {
	if c.EnableServiceLogs != nil {
		return *c.EnableServiceLogs
	}
	return true
}

// CommandLogsEnabled returns whether command file logging is enabled (default: true).
func (c *Config) CommandLogsEnabled() bool {
	if c.EnableCommandLogs != nil {
		return *c.EnableCommandLogs
	}
	return true
}

// DaemonLogEnabled returns whether daemon file logging is enabled (default: true).
func (c *Config) DaemonLogEnabled() bool {
	if c.EnableDaemonLog != nil {
		return *c.EnableDaemonLog
	}
	return true
}

// ResolvedLogsDir returns the absolute logs directory path.
func (c *Config) ResolvedLogsDir() string {
	if c.LogsDir != "" {
		return c.LogsDir
	}
	return filepath.Join(c.ProjectRoot, "logs")
}

// ServiceConfig defines a single service in the configuration.
type ServiceConfig struct {
	Label        string            `yaml:"label"`
	Group        string            `yaml:"group"`
	Type         string            `yaml:"type"`
	Uses         string            `yaml:"uses"`
	Command      string            `yaml:"command"`
	Args         []string          `yaml:"args"`
	WorkingDir   string            `yaml:"working_dir"`
	URL          string            `yaml:"url"`
	Shell        bool              `yaml:"shell"`
	ComposeFile  string            `yaml:"compose_file"`
	EnvFile      string            `yaml:"env_file"`
	Env          map[string]string `yaml:"env"`
	AutoStart    bool              `yaml:"auto_start"`
	DependsOn    []string          `yaml:"depends_on"`
	Build        *BuildConfig      `yaml:"build"`
	BinarySource string            `yaml:"binary_source"`
	BinaryName   string            `yaml:"binary_name"`
}

// BuildConfig defines how to build a service before starting it.
type BuildConfig struct {
	Uses       string            `yaml:"uses"`
	Command    string            `yaml:"command"`
	Args       []string          `yaml:"args"`
	WorkingDir string            `yaml:"working_dir"`
	EnvFile    string            `yaml:"env_file"`
	Env        map[string]string `yaml:"env"`
}

// CommandConfig defines a reusable command in the configuration.
type CommandConfig struct {
	Label        string                       `yaml:"label"`
	Description  string                       `yaml:"description"`
	Group        string                       `yaml:"group"`
	Tags         []string                     `yaml:"tags"`
	Run          string                       `yaml:"run"`
	Command      string                       `yaml:"command"`
	Args         []string                     `yaml:"args"`
	Shell        *bool                        `yaml:"shell"`
	Env          map[string]string            `yaml:"env"`
	EnvFile      string                       `yaml:"env_file"`
	WorkingDir   string                       `yaml:"working_dir"`
	Platform     map[string]*PlatformOverride `yaml:"platform"`
	DependsOn    []string                     `yaml:"depends_on"`
	Parallel     []string                     `yaml:"parallel"`
	Timeout      string                       `yaml:"timeout"`
	IgnoreErrors bool                         `yaml:"ignore_errors"`
	Confirm      bool                         `yaml:"confirm"`
	Interactive  bool                         `yaml:"interactive"`
}

// PlatformOverride allows OS-specific command definitions.
type PlatformOverride struct {
	Run     string            `yaml:"run"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Shell   *bool             `yaml:"shell"`
	Env     map[string]string `yaml:"env"`
}

// IsShell returns the effective shell setting (defaults to false).
func (c *CommandConfig) IsShell() bool {
	if c.Shell != nil {
		return *c.Shell
	}
	return false
}

// ResolvedCommand returns the command and args after resolving run: shorthand.
func (c *CommandConfig) ResolvedCommand() (string, []string) {
	if c.Run != "" {
		return "sh", []string{"-c", c.Run}
	}
	return c.Command, c.Args
}

// Load reads and parses a foreman.yaml config file.
func Load(path string) (*Config, error) {
	cfg, err := loadRaw(path, 0, nil)
	if err != nil {
		return nil, err
	}

	// Now resolve everything on the fully-merged config
	if err := resolveCommands(cfg); err != nil {
		return nil, fmt.Errorf("resolving commands: %w", err)
	}

	// Resolve service references and paths
	rootEnv := make(map[string]string)
	if cfg.EnvFile != "" {
		envPath := cfg.EnvFile
		if !filepath.IsAbs(envPath) {
			envPath = filepath.Join(cfg.ProjectRoot, envPath)
		}
		var envErr error
		rootEnv, envErr = LoadEnvFile(envPath)
		if envErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load root env file %s: %v\n", envPath, envErr)
			rootEnv = make(map[string]string)
		}
	}

	for id, svc := range cfg.Services {
		if svc.Uses != "" {
			if svc.Command != "" {
				return nil, fmt.Errorf("service %s: 'uses' and 'command' are mutually exclusive", id)
			}
			cmdCfg, ok := cfg.Commands[svc.Uses]
			if !ok {
				return nil, fmt.Errorf("service %s: uses references unknown command %q", id, svc.Uses)
			}
			resolveServiceFromCommand(svc, cmdCfg)
		}

		if svc.Build != nil && svc.Build.Uses != "" {
			if svc.Build.Command != "" {
				return nil, fmt.Errorf("service %s build: 'uses' and 'command' are mutually exclusive", id)
			}
			cmdCfg, ok := cfg.Commands[svc.Build.Uses]
			if !ok {
				return nil, fmt.Errorf("service %s build: uses references unknown command %q", id, svc.Build.Uses)
			}
			resolveBuildFromCommand(svc.Build, cmdCfg)
		}

		if svc.Label == "" {
			svc.Label = id
		}
		if svc.Type == "" {
			svc.Type = "process"
		}

		if svc.WorkingDir != "" && !filepath.IsAbs(svc.WorkingDir) {
			svc.WorkingDir = filepath.Join(cfg.ProjectRoot, svc.WorkingDir)
		} else if svc.WorkingDir == "" {
			svc.WorkingDir = cfg.ProjectRoot
		}

		mergedEnv := make(map[string]string)
		for k, v := range rootEnv {
			mergedEnv[k] = v
		}
		if svc.EnvFile != "" {
			envPath := svc.EnvFile
			if !filepath.IsAbs(envPath) {
				envPath = filepath.Join(cfg.ProjectRoot, envPath)
			}
			svcEnv, err := LoadEnvFile(envPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not load env file for service %s: %v\n", id, err)
			} else {
				for k, v := range svcEnv {
					mergedEnv[k] = v
				}
			}
		}
		for k, v := range svc.Env {
			mergedEnv[k] = v
		}
		svc.Env = mergedEnv

		if svc.Build != nil {
			if svc.Build.WorkingDir == "" {
				svc.Build.WorkingDir = svc.WorkingDir
			} else if !filepath.IsAbs(svc.Build.WorkingDir) {
				svc.Build.WorkingDir = filepath.Join(cfg.ProjectRoot, svc.Build.WorkingDir)
			}
			buildEnv := make(map[string]string)
			for k, v := range svc.Env {
				buildEnv[k] = v
			}
			for k, v := range svc.Build.Env {
				buildEnv[k] = v
			}
			svc.Build.Env = buildEnv
		}

		if svc.ComposeFile != "" && !filepath.IsAbs(svc.ComposeFile) {
			svc.ComposeFile = filepath.Join(cfg.ProjectRoot, svc.ComposeFile)
		}
	}

	return cfg, nil
}

const maxImportDepth = 10

// loadRaw loads and merges configs without resolving command references or validating dependencies.
func loadRaw(path string, depth int, seen map[string]bool) (*Config, error) {
	if depth > maxImportDepth {
		return nil, fmt.Errorf("import depth exceeded (max %d): possible circular import", maxImportDepth)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving config path: %w", err)
	}

	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[absPath] {
		return nil, fmt.Errorf("circular import detected: %s", absPath)
	}
	seen[absPath] = true

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	content := interpolateEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	configDir := filepath.Dir(absPath)

	// Process imports: load and merge each imported file
	for _, imp := range cfg.Imports {
		impPath := imp
		if !filepath.IsAbs(impPath) {
			impPath = filepath.Join(configDir, impPath)
		}
		imported, err := loadRaw(impPath, depth+1, seen)
		if err != nil {
			return nil, fmt.Errorf("importing %s: %w", imp, err)
		}
		mergeConfig(&cfg, imported)
	}

	// Set defaults
	if cfg.Port == 0 {
		cfg.Port = 9090
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.LogRetentionLines == 0 {
		cfg.LogRetentionLines = 10000
	}
	if cfg.ProjectRoot == "" {
		cfg.ProjectRoot = configDir
	} else if !filepath.IsAbs(cfg.ProjectRoot) {
		cfg.ProjectRoot = filepath.Join(configDir, cfg.ProjectRoot)
	}

	cfg.ProjectRoot, err = filepath.Abs(cfg.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving project root: %w", err)
	}

	if cfg.LogFile != "" && !filepath.IsAbs(cfg.LogFile) {
		cfg.LogFile = filepath.Join(cfg.ProjectRoot, cfg.LogFile)
	}

	// Resolve logs directory
	if cfg.LogsDir != "" && !filepath.IsAbs(cfg.LogsDir) {
		cfg.LogsDir = filepath.Join(cfg.ProjectRoot, cfg.LogsDir)
	}

	return &cfg, nil
}

// mergeConfig merges imported config into the base config.
// Imported values only fill in gaps — the base config takes precedence for scalar fields.
// Maps (commands, services) are merged: imported entries are added if not already present.
func mergeConfig(base, imported *Config) {
	if imported.Commands != nil {
		if base.Commands == nil {
			base.Commands = make(map[string]*CommandConfig)
		}
		for id, cmd := range imported.Commands {
			if _, exists := base.Commands[id]; !exists {
				base.Commands[id] = cmd
			}
		}
	}
	if imported.Services != nil {
		if base.Services == nil {
			base.Services = make(map[string]*ServiceConfig)
		}
		for id, svc := range imported.Services {
			if _, exists := base.Services[id]; !exists {
				base.Services[id] = svc
			}
		}
	}
}

// resolveCommands validates and resolves all command definitions.
func resolveCommands(cfg *Config) error {
	if cfg.Commands == nil {
		return nil
	}

	for id, cmd := range cfg.Commands {
		// Validate: run and command are mutually exclusive
		if cmd.Run != "" && cmd.Command != "" {
			return fmt.Errorf("command %s: 'run' and 'command' are mutually exclusive", id)
		}

		// Set label default
		if cmd.Label == "" {
			cmd.Label = id
		}

		// Apply platform overrides
		if cmd.Platform != nil {
			goos := runtime.GOOS
			if override, ok := cmd.Platform[goos]; ok {
				applyPlatformOverride(cmd, override)
			}
		}

		// Resolve run: shorthand — if run is set, it implies shell: true
		if cmd.Run != "" && cmd.Shell == nil {
			t := true
			cmd.Shell = &t
		}

		// Resolve working directory relative to project root
		if cmd.WorkingDir != "" && !filepath.IsAbs(cmd.WorkingDir) {
			cmd.WorkingDir = filepath.Join(cfg.ProjectRoot, cmd.WorkingDir)
		} else if cmd.WorkingDir == "" {
			cmd.WorkingDir = cfg.ProjectRoot
		}

		// Resolve env file
		if cmd.EnvFile != "" {
			envPath := cmd.EnvFile
			if !filepath.IsAbs(envPath) {
				envPath = filepath.Join(cfg.ProjectRoot, envPath)
			}
			envVars, err := LoadEnvFile(envPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not load env file for command %s: %v\n", id, err)
			} else {
				if cmd.Env == nil {
					cmd.Env = make(map[string]string)
				}
				// env_file vars are lower priority than inline env
				merged := make(map[string]string)
				for k, v := range envVars {
					merged[k] = v
				}
				for k, v := range cmd.Env {
					merged[k] = v
				}
				cmd.Env = merged
			}
		}

		// Validate depends_on references
		for _, dep := range cmd.DependsOn {
			if _, ok := cfg.Commands[dep]; !ok {
				return fmt.Errorf("command %s: depends_on references unknown command %q", id, dep)
			}
		}
		for _, dep := range cmd.Parallel {
			if _, ok := cfg.Commands[dep]; !ok {
				return fmt.Errorf("command %s: parallel references unknown command %q", id, dep)
			}
		}
	}

	// Detect circular dependencies
	if err := detectCircularDeps(cfg.Commands); err != nil {
		return err
	}

	return nil
}

// detectCircularDeps checks for cycles in depends_on and parallel references.
func detectCircularDeps(commands map[string]*CommandConfig) error {
	// State: 0=unvisited, 1=visiting, 2=visited
	state := make(map[string]int)

	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		if state[id] == 2 {
			return nil
		}
		if state[id] == 1 {
			return fmt.Errorf("circular dependency detected: %s -> %s", strings.Join(path, " -> "), id)
		}
		state[id] = 1
		cmd := commands[id]
		if cmd != nil {
			deps := append(cmd.DependsOn, cmd.Parallel...)
			for _, dep := range deps {
				if err := visit(dep, append(path, id)); err != nil {
					return err
				}
			}
		}
		state[id] = 2
		return nil
	}

	for id := range commands {
		if err := visit(id, nil); err != nil {
			return err
		}
	}
	return nil
}

// applyPlatformOverride merges a platform override into a command config.
func applyPlatformOverride(cmd *CommandConfig, override *PlatformOverride) {
	if override.Run != "" {
		cmd.Run = override.Run
		cmd.Command = ""
		cmd.Args = nil
	}
	if override.Command != "" {
		cmd.Command = override.Command
		cmd.Run = ""
	}
	if override.Args != nil {
		cmd.Args = override.Args
	}
	if override.Shell != nil {
		cmd.Shell = override.Shell
	}
	if override.Env != nil {
		if cmd.Env == nil {
			cmd.Env = make(map[string]string)
		}
		for k, v := range override.Env {
			cmd.Env[k] = v
		}
	}
}

// resolveServiceFromCommand applies command defaults to a service using 'uses'.
func resolveServiceFromCommand(svc *ServiceConfig, cmd *CommandConfig) {
	resolved, resolvedArgs := cmd.ResolvedCommand()
	if svc.Command == "" {
		svc.Command = resolved
	}
	// Append service args after command args
	if len(svc.Args) > 0 {
		svc.Args = append(resolvedArgs, svc.Args...)
	} else {
		svc.Args = resolvedArgs
	}
	if svc.WorkingDir == "" {
		svc.WorkingDir = cmd.WorkingDir
	}
	if cmd.IsShell() {
		svc.Shell = true
	}
	// Merge env: command env is base, service env overrides
	if cmd.Env != nil {
		if svc.Env == nil {
			svc.Env = make(map[string]string)
		}
		merged := make(map[string]string)
		for k, v := range cmd.Env {
			merged[k] = v
		}
		for k, v := range svc.Env {
			merged[k] = v
		}
		svc.Env = merged
	}
}

// resolveBuildFromCommand applies command defaults to a build config using 'uses'.
func resolveBuildFromCommand(build *BuildConfig, cmd *CommandConfig) {
	resolved, resolvedArgs := cmd.ResolvedCommand()
	build.Command = resolved
	build.Args = resolvedArgs
	if build.WorkingDir == "" {
		build.WorkingDir = cmd.WorkingDir
	}
	if cmd.Env != nil {
		if build.Env == nil {
			build.Env = make(map[string]string)
		}
		merged := make(map[string]string)
		for k, v := range cmd.Env {
			merged[k] = v
		}
		for k, v := range build.Env {
			merged[k] = v
		}
		build.Env = merged
	}
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name and optional default
		inner := match[2 : len(match)-1] // strip ${ and }
		parts := strings.SplitN(inner, ":-", 2)
		name := parts[0]
		defaultVal := ""
		if len(parts) == 2 {
			defaultVal = parts[1]
		}
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		return defaultVal
	})
}

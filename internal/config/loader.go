package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for Foreman.
type Config struct {
	ProjectRoot       string                    `yaml:"project_root"`
	EnvFile           string                    `yaml:"env_file"`
	Password          string                    `yaml:"password"`
	Port              int                       `yaml:"port"`
	Host              string                    `yaml:"host"`
	AutoStartOnLogin  bool                      `yaml:"auto_start_on_login"`
	LogRetentionLines int                       `yaml:"log_retention_lines"`
	LogFile           string                    `yaml:"log_file"`
	APIToken          string                    `yaml:"api_token"`
	Services          map[string]*ServiceConfig `yaml:"services"`
}

// ServiceConfig defines a single service in the configuration.
type ServiceConfig struct {
	Label        string            `yaml:"label"`
	Group        string            `yaml:"group"`
	Type         string            `yaml:"type"`
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
	Command    string            `yaml:"command"`
	Args       []string          `yaml:"args"`
	WorkingDir string            `yaml:"working_dir"`
	EnvFile    string            `yaml:"env_file"`
	Env        map[string]string `yaml:"env"`
}

// Load reads and parses a foreman.yaml config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Interpolate environment variables
	content := interpolateEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
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
		cfg.ProjectRoot = filepath.Dir(path)
	} else if !filepath.IsAbs(cfg.ProjectRoot) {
		cfg.ProjectRoot = filepath.Join(filepath.Dir(path), cfg.ProjectRoot)
	}

	// Resolve project root to absolute
	cfg.ProjectRoot, err = filepath.Abs(cfg.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving project root: %w", err)
	}

	// Resolve log file path
	if cfg.LogFile != "" && !filepath.IsAbs(cfg.LogFile) {
		cfg.LogFile = filepath.Join(cfg.ProjectRoot, cfg.LogFile)
	}

	// Load root env file
	rootEnv := make(map[string]string)
	if cfg.EnvFile != "" {
		envPath := cfg.EnvFile
		if !filepath.IsAbs(envPath) {
			envPath = filepath.Join(cfg.ProjectRoot, envPath)
		}
		rootEnv, err = LoadEnvFile(envPath)
		if err != nil {
			// Non-fatal: warn but continue
			fmt.Fprintf(os.Stderr, "warning: could not load root env file %s: %v\n", envPath, err)
		}
	}

	// Process each service
	for id, svc := range cfg.Services {
		if svc.Label == "" {
			svc.Label = id
		}
		if svc.Type == "" {
			svc.Type = "process"
		}

		// Resolve working directory
		if svc.WorkingDir != "" && !filepath.IsAbs(svc.WorkingDir) {
			svc.WorkingDir = filepath.Join(cfg.ProjectRoot, svc.WorkingDir)
		} else if svc.WorkingDir == "" {
			svc.WorkingDir = cfg.ProjectRoot
		}

		// Merge environment: root env → service env_file → service inline env
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

		// Resolve build config working dir (inherits from service if not set)
		if svc.Build != nil {
			if svc.Build.WorkingDir == "" {
				svc.Build.WorkingDir = svc.WorkingDir
			} else if !filepath.IsAbs(svc.Build.WorkingDir) {
				svc.Build.WorkingDir = filepath.Join(cfg.ProjectRoot, svc.Build.WorkingDir)
			}
			// Build env inherits from service env, then overrides
			buildEnv := make(map[string]string)
			for k, v := range svc.Env {
				buildEnv[k] = v
			}
			for k, v := range svc.Build.Env {
				buildEnv[k] = v
			}
			svc.Build.Env = buildEnv
		}

		// Resolve compose file path
		if svc.ComposeFile != "" && !filepath.IsAbs(svc.ComposeFile) {
			svc.ComposeFile = filepath.Join(cfg.ProjectRoot, svc.ComposeFile)
		}
	}

	return &cfg, nil
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

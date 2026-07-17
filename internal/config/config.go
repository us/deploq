package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ValidProjectName is the regex for valid project names. Exported for use by server package.
var ValidProjectName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var validBranchName = regexp.MustCompile(`^[a-zA-Z0-9/_.-]+$`)

const (
	DefaultComposeFile   = "docker-compose.yml"
	DefaultDeployTimeout = 15 * time.Minute
	MinSecretLength      = 16
)

var validTriggerTypes = map[string]bool{
	"push":    true,
	"release": true,
}

type Config struct {
	Listen   string                    `yaml:"listen"`
	Projects map[string]*ProjectConfig `yaml:"projects"`
}

type ProjectConfig struct {
	Path          string        `yaml:"path"`
	Branch        string        `yaml:"branch"`
	Secret        string        `yaml:"secret"`
	ComposeFile   string        `yaml:"compose_file"`
	DeployTimeout time.Duration `yaml:"deploy_timeout"`
	Trigger       []string      `yaml:"trigger"`
	// OnFailure is an optional shell command run via "sh -c" when a deploy fails.
	// It receives DEPLOQ_PROJECT, DEPLOQ_SHA, DEPLOQ_STEP, and DEPLOQ_ERROR env vars.
	OnFailure string `yaml:"on_failure"`
	// DeployCommand, when non-empty, replaces the built-in "docker compose build" +
	// "docker compose up" steps with a custom shell command run via "sh -c".
	// The command runs with the project path as the working directory and inherits
	// the deploq process environment. A non-zero exit code fails the deploy.
	// When empty (the default), the built-in compose steps are used unchanged.
	DeployCommand       string        `yaml:"deploy_command"`
	RequireStatusChecks bool          `yaml:"require_status_checks"`
	StatusCheckMaxWait  time.Duration `yaml:"status_check_max_wait"`
	// ChecksDiscoveryTimeout is how long to wait for all RequiredCheckNames to
	// first appear before failing closed (a commit that spawned no gated
	// workflow). Nested inside StatusCheckMaxWait. Defaults to 2m.
	ChecksDiscoveryTimeout time.Duration `yaml:"checks_discovery_timeout"`
	// RequiredCheckNames is the allowlist of check-run names that gate the
	// deploy. Only these are evaluated; every other check-run is ignored, so an
	// unrelated job's flake cannot block a deploy. Required (non-empty) when
	// RequireStatusChecks is true.
	RequiredCheckNames []string `yaml:"required_check_names"`
}

// Load reads and parses a deploq config file with env var interpolation.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Interpolate environment variables before YAML parsing
	expanded, err := interpolateEnv(string(data))
	if err != nil {
		return nil, fmt.Errorf("env interpolation: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	// Apply defaults
	for _, p := range cfg.Projects {
		if p.ComposeFile == "" {
			p.ComposeFile = DefaultComposeFile
		}
		if p.DeployTimeout == 0 {
			p.DeployTimeout = DefaultDeployTimeout
		}
		if len(p.Trigger) == 0 {
			p.Trigger = []string{"push"}
		}
		if p.StatusCheckMaxWait == 0 {
			p.StatusCheckMaxWait = 5 * time.Minute
		}
		if p.ChecksDiscoveryTimeout == 0 {
			p.ChecksDiscoveryTimeout = 2 * time.Minute
		}
	}

	return &cfg, nil
}

// Validate checks the config for correctness.
func (c *Config) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf("listen address is required")
	}

	if len(c.Projects) == 0 {
		return fmt.Errorf("at least one project is required")
	}

	for name, p := range c.Projects {
		if !ValidProjectName.MatchString(name) {
			return fmt.Errorf("project %q: name must match %s", name, ValidProjectName.String())
		}
		if p.Path == "" {
			return fmt.Errorf("project %q: path is required", name)
		}
		if p.Branch == "" {
			return fmt.Errorf("project %q: branch is required", name)
		}
		if !validBranchName.MatchString(p.Branch) || strings.Contains(p.Branch, "..") {
			return fmt.Errorf("project %q: branch name contains invalid characters", name)
		}
		cleanedCompose := filepath.Clean(p.ComposeFile)
		if filepath.IsAbs(cleanedCompose) || strings.HasPrefix(cleanedCompose, "..") {
			return fmt.Errorf("project %q: compose_file must be a relative path within the project directory", name)
		}
		if len(p.Secret) < MinSecretLength {
			return fmt.Errorf("project %q: secret must be at least %d characters (got %d)", name, MinSecretLength, len(p.Secret))
		}
		if p.DeployTimeout <= 0 {
			return fmt.Errorf("project %q: deploy_timeout must be positive", name)
		}
		for _, t := range p.Trigger {
			if !validTriggerTypes[t] {
				return fmt.Errorf("project %q: invalid trigger type %q (allowed: push, release)", name, t)
			}
		}
		if p.RequireStatusChecks {
			if len(p.RequiredCheckNames) == 0 {
				return fmt.Errorf("project %q: required_check_names must be non-empty when require_status_checks is true", name)
			}
			for _, cn := range p.RequiredCheckNames {
				if strings.TrimSpace(cn) == "" {
					return fmt.Errorf("project %q: required_check_names must not contain empty entries", name)
				}
			}
			if p.StatusCheckMaxWait <= 0 {
				return fmt.Errorf("project %q: status_check_max_wait must be positive when require_status_checks is true", name)
			}
			if p.StatusCheckMaxWait >= p.DeployTimeout {
				return fmt.Errorf("project %q: status_check_max_wait (%v) must be less than deploy_timeout (%v)", name, p.StatusCheckMaxWait, p.DeployTimeout)
			}
			if p.ChecksDiscoveryTimeout <= 0 {
				return fmt.Errorf("project %q: checks_discovery_timeout must be positive when require_status_checks is true", name)
			}
			if p.ChecksDiscoveryTimeout >= p.StatusCheckMaxWait {
				return fmt.Errorf("project %q: checks_discovery_timeout (%v) must be less than status_check_max_wait (%v)", name, p.ChecksDiscoveryTimeout, p.StatusCheckMaxWait)
			}
			if slices.Contains(p.Trigger, "release") {
				slog.Warn("require_status_checks with release trigger: CI check will be skipped for release events (no SHA available)",
					"project", name,
				)
			}
		}
	}

	return nil
}

// interpolateEnv replaces ${VAR} patterns with environment variable values.
// Returns an error if any referenced variable is not set.
func interpolateEnv(s string) (string, error) {
	var missing []string
	result := os.Expand(s, func(key string) string {
		val, ok := os.LookupEnv(key)
		if !ok {
			missing = append(missing, key)
			return ""
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("required environment variables not set: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

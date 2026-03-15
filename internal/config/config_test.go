package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	t.Setenv("TEST_SECRET", "this-is-a-test-secret-value-long-enough")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "deploq.yaml")
	content := `
listen: ":9090"
projects:
  my-app:
    path: /tmp/my-app
    branch: main
    secret: "${TEST_SECRET}"
    compose_file: docker-compose.prod.yml
    deploy_timeout: 10m
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %q, want %q", cfg.Listen, ":9090")
	}

	p, ok := cfg.Projects["my-app"]
	if !ok {
		t.Fatal("project my-app not found")
	}
	if p.Path != "/tmp/my-app" {
		t.Errorf("Path = %q, want %q", p.Path, "/tmp/my-app")
	}
	if p.Branch != "main" {
		t.Errorf("Branch = %q, want %q", p.Branch, "main")
	}
	if p.Secret != "this-is-a-test-secret-value-long-enough" {
		t.Errorf("Secret not interpolated correctly")
	}
	if p.ComposeFile != "docker-compose.prod.yml" {
		t.Errorf("ComposeFile = %q, want %q", p.ComposeFile, "docker-compose.prod.yml")
	}
}

func TestLoad_DefaultComposeFile(t *testing.T) {
	t.Setenv("TEST_SECRET", "this-is-a-test-secret-value-long-enough")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "deploq.yaml")
	content := `
listen: ":9090"
projects:
  app:
    path: /tmp/app
    branch: main
    secret: "${TEST_SECRET}"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	p := cfg.Projects["app"]
	if p.ComposeFile != DefaultComposeFile {
		t.Errorf("ComposeFile = %q, want default %q", p.ComposeFile, DefaultComposeFile)
	}
	if p.DeployTimeout != DefaultDeployTimeout {
		t.Errorf("DeployTimeout = %v, want default %v", p.DeployTimeout, DefaultDeployTimeout)
	}
}

func TestLoad_DefaultTrigger(t *testing.T) {
	t.Setenv("TEST_SECRET", "this-is-a-test-secret-value-long-enough")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "deploq.yaml")
	content := `
listen: ":9090"
projects:
  app:
    path: /tmp/app
    branch: main
    secret: "${TEST_SECRET}"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	p := cfg.Projects["app"]
	if len(p.Trigger) != 1 || p.Trigger[0] != "push" {
		t.Errorf("Trigger = %v, want [push]", p.Trigger)
	}
}

func TestLoad_CustomTrigger(t *testing.T) {
	t.Setenv("TEST_SECRET", "this-is-a-test-secret-value-long-enough")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "deploq.yaml")
	content := `
listen: ":9090"
projects:
  app:
    path: /tmp/app
    branch: main
    secret: "${TEST_SECRET}"
    trigger: [push, release]
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	p := cfg.Projects["app"]
	if len(p.Trigger) != 2 {
		t.Fatalf("Trigger length = %d, want 2", len(p.Trigger))
	}
	if p.Trigger[0] != "push" || p.Trigger[1] != "release" {
		t.Errorf("Trigger = %v, want [push, release]", p.Trigger)
	}
}

func TestLoad_MissingEnvVar(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR_12345")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "deploq.yaml")
	content := `
listen: ":9090"
projects:
  app:
    path: /tmp/app
    branch: main
    secret: "${NONEXISTENT_VAR_12345}"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestValidate_EmptyListen(t *testing.T) {
	cfg := &Config{Listen: "", Projects: map[string]*ProjectConfig{}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty listen")
	}
}

func TestValidate_NoProjects(t *testing.T) {
	cfg := &Config{Listen: ":9090", Projects: map[string]*ProjectConfig{}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for no projects")
	}
}

func TestValidate_InvalidProjectName(t *testing.T) {
	cfg := &Config{
		Listen: ":9090",
		Projects: map[string]*ProjectConfig{
			"../bad": {Path: "/tmp", Branch: "main", Secret: "long-enough-secret-value"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid project name")
	}
}

func TestValidate_ShortSecret(t *testing.T) {
	cfg := &Config{
		Listen: ":9090",
		Projects: map[string]*ProjectConfig{
			"app": {Path: "/tmp", Branch: "main", Secret: "short"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for short secret")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		Listen: ":9090",
		Projects: map[string]*ProjectConfig{
			"my-app": {
				Path:               "/tmp/app",
				Branch:             "main",
				Secret:             "this-is-long-enough-secret",
				ComposeFile:        "docker-compose.yml",
				DeployTimeout:      DefaultDeployTimeout,
				Trigger:            []string{"push"},
				StatusCheckMaxWait: 5 * time.Minute,
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidTrigger(t *testing.T) {
	cfg := &Config{
		Listen: ":9090",
		Projects: map[string]*ProjectConfig{
			"app": {
				Path:               "/tmp/app",
				Branch:             "main",
				Secret:             "this-is-long-enough-secret",
				ComposeFile:        "docker-compose.yml",
				DeployTimeout:      DefaultDeployTimeout,
				Trigger:            []string{"push", "issue"},
				StatusCheckMaxWait: 5 * time.Minute,
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid trigger type")
	}
}

func TestValidate_RequireStatusChecks(t *testing.T) {
	cfg := &Config{
		Listen: ":9090",
		Projects: map[string]*ProjectConfig{
			"app": {
				Path:                "/tmp/app",
				Branch:              "main",
				Secret:              "this-is-long-enough-secret",
				ComposeFile:         "docker-compose.yml",
				DeployTimeout:       DefaultDeployTimeout,
				Trigger:             []string{"push"},
				RequireStatusChecks: true,
				StatusCheckMaxWait:  10 * time.Minute,
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_StatusCheckMaxWait_ExceedsDeployTimeout(t *testing.T) {
	cfg := &Config{
		Listen: ":9090",
		Projects: map[string]*ProjectConfig{
			"app": {
				Path:                "/tmp/app",
				Branch:              "main",
				Secret:              "this-is-long-enough-secret",
				ComposeFile:         "docker-compose.yml",
				DeployTimeout:       5 * time.Minute,
				Trigger:             []string{"push"},
				RequireStatusChecks: true,
				StatusCheckMaxWait:  10 * time.Minute,
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when status_check_max_wait >= deploy_timeout")
	}
}

func TestValidate_BranchDoubleDot(t *testing.T) {
	cfg := &Config{
		Listen: ":9090",
		Projects: map[string]*ProjectConfig{
			"app": {
				Path:               "/tmp/app",
				Branch:             "main..feature",
				Secret:             "this-is-long-enough-secret",
				ComposeFile:        "docker-compose.yml",
				DeployTimeout:      DefaultDeployTimeout,
				Trigger:            []string{"push"},
				StatusCheckMaxWait: 5 * time.Minute,
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for branch with '..'")
	}
}

func TestValidate_ComposeFilePathTraversal(t *testing.T) {
	tests := []struct {
		name        string
		composeFile string
	}{
		{"double dot", "../docker-compose.yml"},
		{"absolute path", "/etc/docker-compose.yml"},
		{"sneaky traversal", "subdir/../../etc/passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Listen: ":9090",
				Projects: map[string]*ProjectConfig{
					"app": {
						Path:               "/tmp/app",
						Branch:             "main",
						Secret:             "this-is-long-enough-secret",
						ComposeFile:        tt.composeFile,
						DeployTimeout:      DefaultDeployTimeout,
						Trigger:            []string{"push"},
						StatusCheckMaxWait: 5 * time.Minute,
					},
				},
			}
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected error for compose_file %q", tt.composeFile)
			}
		})
	}
}

func TestInterpolateEnv(t *testing.T) {
	t.Setenv("FOO", "bar")
	t.Setenv("BAZ", "qux")

	result, err := interpolateEnv("hello ${FOO} world ${BAZ}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello bar world qux" {
		t.Errorf("got %q, want %q", result, "hello bar world qux")
	}
}

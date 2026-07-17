package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/us/deploq/internal/config"
)

func TestSetStatus_PopulatesTimestampAndError(t *testing.T) {
	cfg := &config.Config{
		Listen:   ":9090",
		Projects: map[string]*config.ProjectConfig{},
	}
	d := New(cfg)

	before := time.Now()
	d.setStatus("test", Result{
		SHA:  "abc123",
		Step: "git_fetch",
		Err:  fmt.Errorf("connection refused"),
	})
	after := time.Now()

	result := d.Status("test")
	if result == nil {
		t.Fatal("expected status to be set")
	}
	if result.Timestamp.Before(before) || result.Timestamp.After(after) {
		t.Errorf("Timestamp = %v, expected between %v and %v", result.Timestamp, before, after)
	}
	if result.Error != "connection refused" {
		t.Errorf("Error = %q, want %q", result.Error, "connection refused")
	}
}

func TestSetStatus_NoErrorString_WhenSuccess(t *testing.T) {
	cfg := &config.Config{
		Listen:   ":9090",
		Projects: map[string]*config.ProjectConfig{},
	}
	d := New(cfg)

	d.setStatus("test", Result{
		SHA:  "abc123",
		Step: "done",
	})

	result := d.Status("test")
	if result.Error != "" {
		t.Errorf("Error = %q, want empty", result.Error)
	}
}

func TestStatus_NilForUnknownProject(t *testing.T) {
	cfg := &config.Config{
		Listen:   ":9090",
		Projects: map[string]*config.ProjectConfig{},
	}
	d := New(cfg)

	if result := d.Status("nonexistent"); result != nil {
		t.Errorf("expected nil for unknown project, got %+v", result)
	}
}

func TestRunFailureHook_EmptyOnFailure(t *testing.T) {
	cfg := &config.Config{
		Listen:   ":9090",
		Projects: map[string]*config.ProjectConfig{},
	}
	d := New(cfg)

	project := &config.ProjectConfig{OnFailure: ""}
	// Should return without error — no hook to run
	d.runFailureHook("test", project, Result{Step: "git_fetch", Err: fmt.Errorf("fail")})
}

func TestRunFailureHook_ExecutesAndPassesEnvVars(t *testing.T) {
	cfg := &config.Config{
		Listen:   ":9090",
		Projects: map[string]*config.ProjectConfig{},
	}
	d := New(cfg)

	dir := t.TempDir()
	outFile := filepath.Join(dir, "hook_output.txt")

	project := &config.ProjectConfig{
		OnFailure: fmt.Sprintf("echo \"$DEPLOQ_PROJECT|$DEPLOQ_SHA|$DEPLOQ_STEP|$DEPLOQ_ERROR\" > %s", outFile),
	}

	result := Result{
		SHA:  "abc123",
		Step: "git_fetch",
		Err:  fmt.Errorf("connection refused"),
	}
	d.runFailureHook("myproject", project, result)

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read hook output: %v", err)
	}

	output := strings.TrimSpace(string(data))
	expected := "myproject|abc123|git_fetch|connection refused"
	if output != expected {
		t.Errorf("hook output = %q, want %q", output, expected)
	}
}

func TestRunDeployCommand_Success(t *testing.T) {
	cfg := &config.Config{
		Listen:   ":9090",
		Projects: map[string]*config.ProjectConfig{},
	}
	d := New(cfg)

	dir := t.TempDir()
	outFile := filepath.Join(dir, "ran.txt")
	project := &config.ProjectConfig{
		Path:          dir,
		DeployCommand: fmt.Sprintf("echo ok > %s", outFile),
	}

	result := d.runDeployCommand(t.Context(), "test", project, "abc123")
	if result.Err != nil {
		t.Fatalf("expected success, got error: %v (step=%s)", result.Err, result.Step)
	}
	if result.Step != "done" {
		t.Errorf("Step = %q, want %q", result.Step, "done")
	}
	if result.SHA != "abc123" {
		t.Errorf("SHA = %q, want %q", result.SHA, "abc123")
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("deploy command did not create output file: %v", err)
	}
	if !strings.Contains(string(data), "ok") {
		t.Errorf("unexpected output file contents: %q", string(data))
	}
}

func TestRunDeployCommand_Failure(t *testing.T) {
	cfg := &config.Config{
		Listen:   ":9090",
		Projects: map[string]*config.ProjectConfig{},
	}
	d := New(cfg)

	dir := t.TempDir()
	project := &config.ProjectConfig{
		Path:          dir,
		DeployCommand: "exit 1",
	}

	result := d.runDeployCommand(t.Context(), "test", project, "abc123")
	if result.Err == nil {
		t.Fatal("expected error from failing deploy command, got nil")
	}
	if result.Step != "deploy_command" {
		t.Errorf("Step = %q, want %q", result.Step, "deploy_command")
	}
	if result.SHA != "abc123" {
		t.Errorf("SHA = %q, want %q", result.SHA, "abc123")
	}
}

func TestSanitizeEnvValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal", "connection refused", "connection refused"},
		{"newlines", "line1\nline2\rline3", "line1 line2 line3"},
		{"long", strings.Repeat("a", 600), strings.Repeat("a", 512)},
		{"null byte", "hello\x00world", "helloworld"},
		{"utf8 safe truncation", strings.Repeat("a", 511) + "ü", strings.Repeat("a", 511)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeEnvValue(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeEnvValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsGated(t *testing.T) {
	tok := true
	noTok := false
	cases := []struct {
		name     string
		require  bool
		sha      string
		hasCheck bool
		want     bool
	}{
		{"gated: opted-in + sha + token", true, "abc123", tok, true},
		{"not gated: opted out", false, "abc123", tok, false},
		{"not gated: empty sha (release/CLI)", true, "", tok, false},
		{"not gated: no token", true, "abc123", noTok, false},
		{"not gated: none", false, "", noTok, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &config.ProjectConfig{RequireStatusChecks: c.require}
			if got := isGated(p, c.sha, c.hasCheck); got != c.want {
				t.Errorf("isGated(require=%v, sha=%q, hasCheck=%v) = %v, want %v",
					c.require, c.sha, c.hasCheck, got, c.want)
			}
		})
	}
}

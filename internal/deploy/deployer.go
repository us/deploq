package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/us/deploq/internal/config"
	"github.com/us/deploq/internal/github"
)

// Result contains the outcome of a deploy operation.
type Result struct {
	SHA       string    `json:"sha"`
	Step      string    `json:"step"`
	Err       error     `json:"-"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Deployer orchestrates the deploy pipeline for projects.
type Deployer struct {
	cfg           *config.Config
	locker        *ProjectLocker
	wg            sync.WaitGroup
	statusChecker *github.StatusChecker

	// status tracks last deploy result per project
	mu     sync.RWMutex
	status map[string]*Result

	// repoInfo caches remote URL parsing per project path
	repoMu   sync.Mutex
	repoInfo map[string]*repoInfo
}

type repoInfo struct {
	owner string
	repo  string
}

// New creates a new Deployer.
func New(cfg *config.Config) *Deployer {
	d := &Deployer{
		cfg:      cfg,
		locker:   NewLocker(),
		status:   make(map[string]*Result),
		repoInfo: make(map[string]*repoInfo),
	}

	if token := os.Getenv("DEPLOQ_GITHUB_TOKEN"); token != "" {
		d.statusChecker = github.NewStatusChecker(token)
		slog.Info("github status checker enabled")
	}

	return d
}

// Deploy starts an async deploy for the given project. Returns immediately.
// The deploy runs with its own background context (independent of the caller)
// so it is not cancelled when the HTTP connection closes.
func (d *Deployer) Deploy(projectName string, project *config.ProjectConfig, sha string) (isDuplicate bool, isLocked bool) {
	unlock, duplicate, acquired := d.locker.TryLock(projectName, sha)
	if duplicate {
		slog.Info("skipping duplicate sha", "project", projectName, "sha", sha)
		return true, false
	}
	if !acquired {
		slog.Warn("deploy already in progress", "project", projectName)
		return false, true
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("deploy goroutine panicked",
					"project", projectName,
					"panic", r,
				)
				unlock(false)
				d.setStatus(projectName, Result{
					SHA:  sha,
					Step: "panic",
					Err:  fmt.Errorf("internal panic: %v", r),
				})
			}
		}()

		deployCtx, cancel := context.WithTimeout(context.Background(), project.DeployTimeout)
		defer cancel()

		result := d.run(deployCtx, projectName, project, sha)
		unlock(result.Err == nil)
		d.setStatus(projectName, result)

		if result.Err != nil {
			slog.Error("deploy failed",
				"project", projectName,
				"step", result.Step,
				"error", result.Err,
			)
			d.runFailureHook(projectName, project, result)
		} else {
			slog.Info("deploy completed",
				"project", projectName,
				"sha", result.SHA,
			)
		}
	}()

	return false, false
}

// DeploySync executes a synchronous deploy with lock protection.
// Used by the CLI deploy command.
func (d *Deployer) DeploySync(ctx context.Context, projectName string, project *config.ProjectConfig) Result {
	unlock, duplicate, acquired := d.locker.TryLock(projectName, "")
	if duplicate {
		return Result{Step: "lock", Err: fmt.Errorf("duplicate SHA")}
	}
	if !acquired {
		return Result{Step: "lock", Err: fmt.Errorf("deploy already in progress")}
	}
	result := d.run(ctx, projectName, project, "")
	unlock(result.Err == nil)
	d.setStatus(projectName, result)
	return result
}

// run executes the deploy pipeline synchronously without locking.
func (d *Deployer) run(ctx context.Context, projectName string, project *config.ProjectConfig, webhookSHA string) Result {
	slog.Info("starting deploy", "project", projectName, "path", project.Path, "branch", project.Branch)

	// Step 0: CI status check (if enabled and SHA available)
	if project.RequireStatusChecks && d.statusChecker == nil {
		return Result{Step: "status_check", Err: fmt.Errorf("require_status_checks is enabled but DEPLOQ_GITHUB_TOKEN is not set")}
	}
	if project.RequireStatusChecks && webhookSHA == "" {
		slog.Warn("require_status_checks is enabled but no SHA available; skipping CI check",
			"project", projectName,
		)
	}
	if project.RequireStatusChecks && webhookSHA != "" && d.statusChecker != nil {
		slog.Info("waiting for CI status checks", "project", projectName, "sha", webhookSHA)
		ri, err := d.getRepoInfo(ctx, project.Path)
		if err != nil {
			return Result{Step: "status_check", Err: fmt.Errorf("getting repo info: %w", err)}
		}
		if err := d.statusChecker.WaitForSuccess(ctx, ri.owner, ri.repo, webhookSHA, project.StatusCheckMaxWait); err != nil {
			return Result{SHA: webhookSHA, Step: "status_check", Err: err}
		}
		slog.Info("CI status checks passed", "project", projectName)
	}

	// Step 1: git fetch
	slog.Info("git fetch", "project", projectName)
	output, err := GitFetch(ctx, project.Path, project.Branch)
	if err != nil {
		slog.Error("git fetch output", "project", projectName, "output", output)
		return Result{Step: "git_fetch", Err: err}
	}

	// Step 2: git reset --hard
	slog.Info("git reset", "project", projectName)
	output, err = GitReset(ctx, project.Path, project.Branch)
	if err != nil {
		slog.Error("git reset output", "project", projectName, "output", output)
		return Result{Step: "git_reset", Err: err}
	}

	// Step 3: get current SHA
	sha, err := GitCurrentSHA(ctx, project.Path)
	if err != nil {
		return Result{Step: "git_sha", Err: err}
	}

	// Step 4: docker compose build
	slog.Info("docker compose build", "project", projectName)
	output, err = ComposeBuild(ctx, project.Path, project.ComposeFile)
	if err != nil {
		slog.Error("docker compose build failed — working directory updated but containers unchanged",
			"project", projectName,
			"sha", sha,
			"output", output,
		)
		return Result{SHA: sha, Step: "compose_build", Err: err}
	}

	// Step 5: docker compose up
	slog.Info("docker compose up", "project", projectName)
	output, err = ComposeUp(ctx, project.Path, project.ComposeFile)
	if err != nil {
		slog.Error("docker compose up failed",
			"project", projectName,
			"sha", sha,
			"output", output,
		)
		return Result{SHA: sha, Step: "compose_up", Err: err}
	}

	return Result{SHA: sha, Step: "done"}
}

// Wait blocks until all active deploys complete or the context is cancelled.
func (d *Deployer) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		slog.Warn("deploy wait timed out, some deploys may still be running")
		return ctx.Err()
	}
}

// Status returns the last deploy result for a project.
func (d *Deployer) Status(projectName string) *Result {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status[projectName]
}

func (d *Deployer) setStatus(projectName string, result Result) {
	result.Timestamp = time.Now()
	if result.Err != nil {
		result.Error = result.Err.Error()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status[projectName] = &result
}

func (d *Deployer) runFailureHook(projectName string, project *config.ProjectConfig, result Result) {
	if project.OnFailure == "" {
		return
	}

	slog.Info("running on_failure hook", "project", projectName)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", project.OnFailure)
	errMsg := ""
	if result.Err != nil {
		errMsg = result.Err.Error()
	}
	cmd.Env = append(os.Environ(),
		"DEPLOQ_PROJECT="+projectName,
		"DEPLOQ_SHA="+result.SHA,
		"DEPLOQ_STEP="+result.Step,
		"DEPLOQ_ERROR="+sanitizeEnvValue(errMsg),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("on_failure hook failed",
			"project", projectName,
			"error", err,
			"output", string(out),
		)
	} else {
		slog.Info("on_failure hook completed", "project", projectName)
	}
}

const maxEnvValueLen = 512

func sanitizeEnvValue(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\x00", "")
	if len(s) > maxEnvValueLen {
		s = s[:maxEnvValueLen]
		// Avoid cutting a multi-byte UTF-8 character in half
		for !utf8.ValidString(s) && len(s) > 0 {
			s = s[:len(s)-1]
		}
	}
	return s
}

func (d *Deployer) getRepoInfo(ctx context.Context, projectPath string) (*repoInfo, error) {
	d.repoMu.Lock()
	defer d.repoMu.Unlock()

	if ri, ok := d.repoInfo[projectPath]; ok {
		return ri, nil
	}

	// Hold lock during git call to prevent duplicate work from concurrent deploys.
	// git remote get-url is fast (<50ms) so lock contention is negligible.
	remoteURL, err := github.GetRemoteURL(ctx, projectPath)
	if err != nil {
		return nil, err
	}
	owner, repo, err := github.ParseRemoteURL(remoteURL)
	if err != nil {
		return nil, err
	}

	ri := &repoInfo{owner: owner, repo: repo}
	d.repoInfo[projectPath] = ri
	return ri, nil
}

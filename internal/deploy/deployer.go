package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/us/deploq/internal/config"
)

// Result contains the outcome of a deploy operation.
type Result struct {
	SHA  string
	Step string
	Err  error
}

// Deployer orchestrates the deploy pipeline for projects.
type Deployer struct {
	cfg    *config.Config
	locker *ProjectLocker
	wg     sync.WaitGroup

	// status tracks last deploy result per project
	mu     sync.RWMutex
	status map[string]*Result
}

// New creates a new Deployer.
func New(cfg *config.Config) *Deployer {
	return &Deployer{
		cfg:    cfg,
		locker: NewLocker(),
		status: make(map[string]*Result),
	}
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
					Step: "panic",
					Err:  fmt.Errorf("internal panic: %v", r),
				})
			}
		}()

		deployCtx, cancel := context.WithTimeout(context.Background(), project.DeployTimeout)
		defer cancel()

		result := d.run(deployCtx, projectName, project)
		unlock(result.Err == nil)
		d.setStatus(projectName, result)

		if result.Err != nil {
			slog.Error("deploy failed",
				"project", projectName,
				"step", result.Step,
				"error", result.Err,
			)
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
	result := d.run(ctx, projectName, project)
	unlock(result.Err == nil)
	d.setStatus(projectName, result)
	return result
}

// run executes the deploy pipeline synchronously without locking.
func (d *Deployer) run(ctx context.Context, projectName string, project *config.ProjectConfig) Result {
	slog.Info("starting deploy", "project", projectName, "path", project.Path, "branch", project.Branch)

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

// Wait blocks until all active deploys complete.
func (d *Deployer) Wait() {
	d.wg.Wait()
}

// Status returns the last deploy result for a project.
func (d *Deployer) Status(projectName string) *Result {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status[projectName]
}

func (d *Deployer) setStatus(projectName string, result Result) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status[projectName] = &result
}

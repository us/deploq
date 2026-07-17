package deploy

import (
	"context"
	"errors"
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
//
// isCoalesced is true when a deploy was already running: instead of dropping
// this webhook, the running deploy is flagged to rerun once it finishes, which
// git-resets to origin/main HEAD (the latest commit). Piled-up webhooks thus
// collapse to a single final deploy of the newest commit.
func (d *Deployer) Deploy(projectName string, project *config.ProjectConfig, sha string) (isDuplicate bool, isCoalesced bool) {
	acquired, duplicate := d.locker.TryAcquire(projectName, sha)
	if duplicate {
		slog.Info("skipping duplicate sha", "project", projectName, "sha", sha)
		return true, false
	}
	if !acquired {
		// A deploy is in progress — coalesce this request onto it rather than
		// dropping it. MarkPending can only fail if the slot freed in the race
		// window since TryAcquire; retry once to start a fresh deploy then.
		if d.locker.MarkPending(projectName, sha) {
			slog.Info("deploy in progress; coalesced (will redeploy latest)", "project", projectName, "sha", sha)
			return false, true
		}
		acquired, duplicate = d.locker.TryAcquire(projectName, sha)
		if duplicate {
			return true, false
		}
		if !acquired {
			// Lost the retry race to another goroutine that just acquired; it
			// will pick up our pending flag. Treat as coalesced.
			d.locker.MarkPending(projectName, sha)
			return false, true
		}
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		// nextSHA carries a SHA into the run for the optional CI status-check
		// gate + duplicate recording. On the GATED path run() resets to that
		// EXACT SHA (see run() Step 2) so the deploy lands on the commit whose
		// checks were verified; on every other path it resets to origin/<branch>
		// HEAD. The first iteration passes the webhook SHA; reruns pass the
		// pending (newer) SHA, so each coalesced commit is gated on its own.
		nextSHA := sha
		for {
			result := d.runGuarded(projectName, project, nextSHA)
			next, rerun := d.locker.FinishOrRerun(projectName, result.SHA, result.Err == nil)
			if !rerun {
				return
			}
			slog.Info("coalesced redeploy: newer commit pending", "project", projectName, "sha", next)
			nextSHA = next
		}
	}()

	return false, false
}

// runGuarded runs one deploy iteration under its own timeout context and records
// its status. A panic anywhere in run/setStatus/failure-hook is recovered into a
// failed Result so the caller's loop always reaches FinishOrRerun and releases
// the single-flight slot (a panic can never wedge a project's deploys).
func (d *Deployer) runGuarded(projectName string, project *config.ProjectConfig, sha string) (result Result) {
	deployCtx, cancel := context.WithTimeout(context.Background(), project.DeployTimeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("deploy goroutine panicked", "project", projectName, "panic", r)
			result = Result{SHA: sha, Step: "panic", Err: fmt.Errorf("internal panic: %v", r)}
			d.setStatus(projectName, result)
		}
	}()

	result = d.run(deployCtx, projectName, project, sha)
	d.setStatus(projectName, result)
	if result.Err != nil {
		slog.Error("deploy failed", "project", projectName, "step", result.Step, "error", result.Err)
		d.runFailureHook(projectName, project, result)
	} else {
		slog.Info("deploy completed", "project", projectName, "sha", result.SHA)
	}
	return result
}

// DeploySync executes a synchronous deploy with lock protection.
// Used by the CLI deploy command.
func (d *Deployer) DeploySync(ctx context.Context, projectName string, project *config.ProjectConfig) Result {
	acquired, duplicate := d.locker.TryAcquire(projectName, "")
	if duplicate {
		return Result{Step: "lock", Err: fmt.Errorf("duplicate SHA")}
	}
	if !acquired {
		return Result{Step: "lock", Err: fmt.Errorf("deploy already in progress")}
	}
	result := d.run(ctx, projectName, project, "")
	// CLI is one-shot: no webhooks can set pending, so the rerun signal is moot.
	d.locker.FinishOrRerun(projectName, result.SHA, result.Err == nil)
	d.setStatus(projectName, result)
	return result
}

// isGated reports whether this deploy must reset to the EXACT verified SHA (the
// CI-gated path) rather than to the branch tip. Gated requires all three: the
// project opted in, a concrete SHA to gate on (release/CLI pass ""), and a
// status checker (token) to gate with. Only the gated path uses GitResetToSHA;
// every other path keeps the branch reset, so release + CLI deploys are unaffected.
func isGated(project *config.ProjectConfig, webhookSHA string, hasChecker bool) bool {
	return project.RequireStatusChecks && webhookSHA != "" && hasChecker
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
	gated := isGated(project, webhookSHA, d.statusChecker != nil)
	if gated {
		slog.Info("waiting for CI status checks", "project", projectName, "sha", webhookSHA, "required", project.RequiredCheckNames)
		ri, err := d.getRepoInfo(ctx, project.Path)
		if err != nil {
			return Result{Step: "status_check", Err: fmt.Errorf("getting repo info: %w", err)}
		}
		if err := d.statusChecker.WaitForSuccess(ctx, ri.owner, ri.repo, webhookSHA,
			project.RequiredCheckNames, project.ChecksDiscoveryTimeout, project.StatusCheckMaxWait); err != nil {
			return Result{SHA: webhookSHA, Step: "status_check", Err: err}
		}
		slog.Info("CI status checks passed", "project", projectName, "sha", webhookSHA)
	}

	// Step 1: git fetch
	slog.Info("git fetch", "project", projectName)
	output, err := GitFetch(ctx, project.Path, project.Branch)
	if err != nil {
		slog.Error("git fetch output", "project", projectName, "output", output)
		return Result{Step: "git_fetch", Err: err}
	}

	// Step 2: git reset --hard.
	//
	// On the CI-gated path, reset to the EXACT gated SHA — not origin/<branch> —
	// so the deploy lands on the commit whose checks we just verified. Resetting
	// to the branch tip would let a commit pushed during the (minutes-long) CI
	// wait ship without ever being gated (TOCTOU). Every other path (release
	// events + CLI, both empty-SHA) keeps the branch reset unchanged.
	slog.Info("git reset", "project", projectName)
	if gated {
		output, err = GitResetToSHA(ctx, project.Path, webhookSHA)
	} else {
		output, err = GitReset(ctx, project.Path, project.Branch)
	}
	if err != nil {
		slog.Error("git reset output", "project", projectName, "output", output)
		return Result{Step: "git_reset", Err: err}
	}

	// Step 3: get current SHA
	sha, err := GitCurrentSHA(ctx, project.Path)
	if err != nil {
		return Result{Step: "git_sha", Err: err}
	}

	// Steps 4+5: custom deploy command OR built-in docker compose build+up.
	if project.DeployCommand != "" {
		return d.runDeployCommand(ctx, projectName, project, sha)
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

// runDeployCommand executes project.DeployCommand in place of the built-in compose steps.
// It runs the command via "sh -c" with project.Path as the working directory and the
// deploq process environment. stdout and stderr are captured and logged on failure.
// A non-zero exit code is a failed deploy (Step: "deploy_command").
func (d *Deployer) runDeployCommand(ctx context.Context, projectName string, project *config.ProjectConfig, sha string) Result {
	slog.Info("deploy command", "project", projectName, "cmd", project.DeployCommand)

	cmd := exec.CommandContext(ctx, "sh", "-c", project.DeployCommand)
	cmd.Dir = project.Path
	// Expose the resolved project + SHA so the deploy script can tag images,
	// create deployments, or notify without re-running `git rev-parse`. Mirrors
	// the env the OnFailure hook injects.
	cmd.Env = append(os.Environ(),
		"DEPLOQ_PROJECT="+projectName,
		"DEPLOQ_SHA="+sha,
	)

	out := newLimitedWriter()
	errOut := newLimitedWriter()
	cmd.Stdout = out
	cmd.Stderr = errOut
	configureProcessGroup(cmd)

	// exec.ErrWaitDelay means the command exited 0 (no timeout/Cancel) but a
	// child it backgrounded still held the stdout/stderr pipe past WaitDelay.
	// That is a successful deploy whose script spawned a lingering process — not
	// a failure — so don't fail the deploy on it. A real timeout instead fires
	// Cancel and Wait returns the kill error, which is NOT ErrWaitDelay.
	if err := cmd.Run(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		combined := out.String() + errOut.String()
		slog.Error("deploy command failed",
			"project", projectName,
			"sha", sha,
			"output", combined,
		)
		return Result{SHA: sha, Step: "deploy_command", Err: err}
	} else if err != nil {
		slog.Warn("deploy command exited 0 but backgrounded a process holding stdout; "+
			"redirect its fds (e.g. `>/dev/null 2>&1 &`) to avoid a WaitDelay stall",
			"project", projectName, "sha", sha)
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
	configureProcessGroup(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
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

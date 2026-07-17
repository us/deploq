package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const maxOutputSize = 1 << 20 // 1 MB

// limitedWriter wraps a bytes.Buffer and stops writing after a limit.
type limitedWriter struct {
	buf   bytes.Buffer
	limit int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	remaining := lw.limit - lw.buf.Len()
	if remaining <= 0 {
		return len(p), nil // discard silently
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return lw.buf.Write(p)
}

func (lw *limitedWriter) String() string {
	return lw.buf.String()
}

func newLimitedWriter() *limitedWriter {
	return &limitedWriter{limit: maxOutputSize}
}

// runCommand executes a command with limited output capture.
func runCommand(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error) {
	out := newLimitedWriter()
	errOut := newLimitedWriter()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = io.Writer(out)
	cmd.Stderr = io.Writer(errOut)
	configureProcessGroup(cmd)

	// A clean exit whose backgrounded child held the pipe past WaitDelay
	// (exec.ErrWaitDelay) is a success, not a command failure.
	if err := cmd.Run(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return out.String(), errOut.String(), err
	}
	return out.String(), errOut.String(), nil
}

// GitFetch runs git fetch origin <branch> in the given directory.
func GitFetch(ctx context.Context, dir, branch string) (string, error) {
	_, stderr, err := runCommand(ctx, dir, "git", "fetch", "origin", branch)
	if err != nil {
		return stderr, fmt.Errorf("git fetch: %w", err)
	}
	return "", nil
}

// GitReset runs git reset --hard origin/<branch> in the given directory.
func GitReset(ctx context.Context, dir, branch string) (string, error) {
	_, stderr, err := runCommand(ctx, dir, "git", "reset", "--hard", "origin/"+branch)
	if err != nil {
		return stderr, fmt.Errorf("git reset: %w", err)
	}
	return "", nil
}

// GitResetToSHA runs git reset --hard <sha> in the given directory. Used on the
// CI-gated path so the deploy lands on the EXACT commit whose checks were
// verified, not whatever origin/<branch> advanced to during the CI wait (the
// TOCTOU that would otherwise let an ungated commit ship). Callers must only use
// this with a non-empty, validated SHA; empty-SHA paths (release, CLI) keep
// GitReset(branch).
func GitResetToSHA(ctx context.Context, dir, sha string) (string, error) {
	_, stderr, err := runCommand(ctx, dir, "git", "reset", "--hard", sha)
	if err != nil {
		return stderr, fmt.Errorf("git reset to %s: %w", sha, err)
	}
	return "", nil
}

// GitCurrentSHA returns the current HEAD SHA in the given directory.
func GitCurrentSHA(ctx context.Context, dir string) (string, error) {
	stdout, _, err := runCommand(ctx, dir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(stdout), nil
}

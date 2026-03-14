package deploy

import (
	"bytes"
	"context"
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

	if err := cmd.Run(); err != nil {
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

// GitCurrentSHA returns the current HEAD SHA in the given directory.
func GitCurrentSHA(ctx context.Context, dir string) (string, error) {
	stdout, _, err := runCommand(ctx, dir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(stdout), nil
}

package github

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var (
	sshRemoteRe    = regexp.MustCompile(`^git@([^:]+):([^/]+)/([^/.]+?)(?:\.git)?$`)
	httpsRemoteRe  = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/.]+?)(?:\.git)?$`)
	validOwnerRepo = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
)

// ParseRemoteURL extracts owner and repo from a git remote URL.
// Supports SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git) formats.
func ParseRemoteURL(url string) (owner, repo string, err error) {
	url = strings.TrimSpace(url)

	if m := sshRemoteRe.FindStringSubmatch(url); m != nil {
		return validateOwnerRepo(m[2], m[3])
	}
	if m := httpsRemoteRe.FindStringSubmatch(url); m != nil {
		return validateOwnerRepo(m[2], m[3])
	}

	return "", "", fmt.Errorf("unable to parse git remote URL: %q", url)
}

func validateOwnerRepo(owner, repo string) (string, string, error) {
	if !validOwnerRepo.MatchString(owner) {
		return "", "", fmt.Errorf("invalid owner format: %q", owner)
	}
	if !validOwnerRepo.MatchString(repo) {
		return "", "", fmt.Errorf("invalid repo format: %q", repo)
	}
	return owner, repo, nil
}

// GetRemoteURL runs git to get the origin remote URL for a repo directory.
func GetRemoteURL(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

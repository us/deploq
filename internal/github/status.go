package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"
)

const maxTransientErrors = 3

var defaultBackoff = []time.Duration{0, 10 * time.Second, 20 * time.Second, 30 * time.Second}

// StatusChecker polls the GitHub commit status API.
type StatusChecker struct {
	client  *http.Client
	token   string
	baseURL string
	backoff []time.Duration // overridable for testing
}

// NewStatusChecker creates a StatusChecker with the given GitHub token.
func NewStatusChecker(token string) *StatusChecker {
	return &StatusChecker{
		client:  &http.Client{Timeout: 10 * time.Second},
		token:   token,
		baseURL: "https://api.github.com",
		backoff: slices.Clone(defaultBackoff),
	}
}

type combinedStatus struct {
	State string `json:"state"` // "success", "failure", "error", "pending"
}

// WaitForSuccess polls the GitHub combined status API until the commit status is success,
// returns an error on failure/error state, or times out after maxWait.
func (sc *StatusChecker) WaitForSuccess(ctx context.Context, owner, repo, sha string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	transientErrors := 0

	for i := 0; ; i++ {
		if time.Now().After(deadline) {
			return fmt.Errorf("status check timed out after %s", maxWait)
		}

		// Determine wait duration
		var wait time.Duration
		if i < len(sc.backoff) {
			wait = sc.backoff[i]
		} else {
			wait = 30 * time.Second
		}

		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		state, err := sc.fetchStatus(ctx, owner, repo, sha)
		if err != nil {
			transientErrors++
			if transientErrors >= maxTransientErrors {
				return fmt.Errorf("fetching commit status (after %d retries): %w", transientErrors, err)
			}
			slog.Warn("transient error fetching CI status, will retry",
				"sha", sha, "error", err, "attempt", transientErrors,
			)
			continue
		}
		transientErrors = 0

		switch state {
		case "success":
			return nil
		case "failure", "error":
			return fmt.Errorf("CI status is %q for commit %s", state, sha)
		case "pending":
			continue
		default:
			return fmt.Errorf("unexpected CI status %q for commit %s", state, sha)
		}
	}
}

func (sc *StatusChecker) fetchStatus(ctx context.Context, owner, repo, sha string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s/status", sc.baseURL, owner, repo, sha)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if sc.token != "" {
		req.Header.Set("Authorization", "Bearer "+sc.token)
	}

	resp, err := sc.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody := string(body)
		if len(errBody) > 200 {
			errBody = errBody[:200] + "...(truncated)"
		}
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, errBody)
	}

	var status combinedStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return "", fmt.Errorf("parsing status response: %w", err)
	}

	return status.State, nil
}

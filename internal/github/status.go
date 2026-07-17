package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

const maxTransientErrors = 3

// perPage caps check-runs pagination page size (GitHub max is 100).
const perPage = 100

var defaultBackoff = []time.Duration{0, 10 * time.Second, 20 * time.Second, 30 * time.Second}

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// StatusChecker polls the GitHub check-runs API to gate a deploy on CI.
//
// It queries the check-runs API (what GitHub Actions writes to), NOT the legacy
// combined commit-status API (which Actions never populates — that endpoint
// returns an eternal "pending"/total_count:0 for an Actions-only repo).
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

type checkRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`     // queued | in_progress | completed
	Conclusion string `json:"conclusion"` // set only when status==completed
	ID         int64  `json:"id"`
	StartedAt  string `json:"started_at"`
}

type checkRunsResponse struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []checkRun `json:"check_runs"`
}

// WaitForSuccess polls the check-runs for `sha` until every gated check
// (required) has completed successfully, or a required check fails, or it times
// out. `required` is the allowlist of check-run names that gate the deploy; only
// those are evaluated — any other check (e.g. an SEO-ping job) is ignored, so an
// unrelated flake can never block a deploy.
//
// Two timeouts, both consumed from the same wall clock:
//   - discoveryTimeout: how long to wait for ALL required checks to first APPEAR.
//     A commit that never spawns the gated workflow would otherwise wait forever;
//     instead it fails closed once discovery elapses (naming the missing checks).
//   - maxWait: the overall budget (discoveryTimeout is nested inside it).
//
// Success requires, for EACH required name: present, status==completed, and
// conclusion=="success" — strictly success. skipped/neutral/cancelled/stale/
// timed_out/action_required/startup_failure and any unknown value fail closed:
// a gated check must really pass, never be waved through by a skip.
func (sc *StatusChecker) WaitForSuccess(ctx context.Context, owner, repo, sha string, required []string, discoveryTimeout, maxWait time.Duration) error {
	// Defense in depth: an empty allowlist would make allRequiredGreen vacuously
	// true and wave every deploy through. config.Validate() already rejects this,
	// but the gate must never silently pass on a misconfiguration.
	if len(required) == 0 {
		return fmt.Errorf("no required check names configured for %s", sha)
	}
	start := time.Now()
	deadline := start.Add(maxWait)
	discoveryDeadline := start.Add(discoveryTimeout)
	transientErrors := 0

	for i := 0; ; i++ {
		if time.Now().After(deadline) {
			return fmt.Errorf("CI wait timed out after %s for %s (required: %s)", maxWait, sha, strings.Join(required, ", "))
		}

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

		runs, err := sc.fetchCheckRuns(ctx, owner, repo, sha)
		if err != nil {
			transientErrors++
			if transientErrors >= maxTransientErrors {
				return fmt.Errorf("fetching check-runs (after %d retries): %w", transientErrors, err)
			}
			slog.Warn("transient error fetching check-runs, will retry",
				"sha", sha, "error", err, "attempt", transientErrors,
			)
			continue
		}
		transientErrors = 0

		latest := latestByName(runs)

		// Any required check that has FAILED is terminal — fail closed now.
		for _, name := range required {
			r, ok := latest[name]
			if ok && r.Status == "completed" && r.Conclusion != "success" {
				return fmt.Errorf("CI red: required check %q concluded %q for %s", name, r.Conclusion, sha)
			}
		}

		// All required present AND completed AND success → pass.
		if allRequiredGreen(required, latest) {
			return nil
		}

		// Not all green yet. If any required check is still MISSING and discovery
		// has elapsed, fail closed (the workflow likely never triggered for this
		// SHA — a path filter, a disabled workflow, or a webhook problem).
		if missing := missingRequired(required, latest); len(missing) > 0 && time.Now().After(discoveryDeadline) {
			return fmt.Errorf("CI discovery timeout: required check(s) %s never appeared within %s for %s",
				strings.Join(missing, ", "), discoveryTimeout, sha)
		}
		// else: keep polling (checks present but still running, or discovery window open).
		slog.Info("waiting on CI", "sha", sha, "seen", checkSummary(latest), "required", required)
	}
}

// latestByName picks, per check-run name, the newest run (a re-run creates a new
// row with the same name; the newest by started_at, tie-broken by id, wins).
func latestByName(runs []checkRun) map[string]checkRun {
	out := make(map[string]checkRun, len(runs))
	for _, r := range runs {
		cur, ok := out[r.Name]
		if !ok || newer(r, cur) {
			out[r.Name] = r
		}
	}
	return out
}

func newer(a, b checkRun) bool {
	if a.StartedAt != b.StartedAt {
		return a.StartedAt > b.StartedAt // ISO-8601 sorts lexically
	}
	return a.ID > b.ID
}

func allRequiredGreen(required []string, latest map[string]checkRun) bool {
	for _, name := range required {
		r, ok := latest[name]
		if !ok || r.Status != "completed" || r.Conclusion != "success" {
			return false
		}
	}
	return true
}

func missingRequired(required []string, latest map[string]checkRun) []string {
	var missing []string
	for _, name := range required {
		if _, ok := latest[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func checkSummary(latest map[string]checkRun) string {
	parts := make([]string, 0, len(latest))
	for name, r := range latest {
		state := r.Status
		if r.Status == "completed" {
			state = r.Conclusion
		}
		parts = append(parts, name+"="+state)
	}
	slices.Sort(parts)
	return strings.Join(parts, " ")
}

// fetchCheckRuns lists every check-run for a ref, following pagination so a repo
// with many jobs is not silently truncated to the first page.
func (sc *StatusChecker) fetchCheckRuns(ctx context.Context, owner, repo, sha string) ([]checkRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s/check-runs?per_page=%d", sc.baseURL, owner, repo, sha, perPage)
	var all []checkRun

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		if sc.token != "" {
			req.Header.Set("Authorization", "Bearer "+sc.token)
		}

		resp, err := sc.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			errBody := string(body)
			if len(errBody) > 200 {
				errBody = errBody[:200] + "...(truncated)"
			}
			return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, errBody)
		}

		var page checkRunsResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing check-runs response: %w", err)
		}
		all = append(all, page.CheckRuns...)

		url = nextPageURL(resp.Header.Get("Link"))
	}
	return all, nil
}

// nextPageURL extracts the rel="next" URL from a GitHub Link header, or "".
func nextPageURL(link string) string {
	if link == "" {
		return ""
	}
	if m := linkNextRe.FindStringSubmatch(link); m != nil {
		return m[1]
	}
	return ""
}

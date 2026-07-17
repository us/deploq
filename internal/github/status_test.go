package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testChecker(token string, url string) *StatusChecker {
	sc := NewStatusChecker(token)
	sc.baseURL = url
	sc.backoff = make([]time.Duration, 100) // no delay in tests, enough slots to avoid 30s fallback
	return sc
}

// crJSON builds a check-runs API body from (name, status, conclusion) triples.
func crJSON(runs ...checkRun) string {
	body := checkRunsResponse{TotalCount: len(runs), CheckRuns: runs}
	b, _ := json.Marshal(body)
	return string(b)
}

func run(name, status, conclusion string) checkRun {
	return checkRun{Name: name, Status: status, Conclusion: conclusion, ID: 1, StartedAt: "2026-07-17T00:00:00Z"}
}

var allowlist = []string{"verify", "team-integration", "build"}

func waitFast(sc *StatusChecker, ctx context.Context, required []string, maxWait time.Duration) error {
	// discovery nested well inside maxWait
	disc := maxWait / 2
	if disc <= 0 {
		disc = maxWait
	}
	return sc.WaitForSuccess(ctx, "owner", "repo", "abc123", required, disc, maxWait)
}

func TestWait_AllGreen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "success"),
			run("build", "completed", "success"),
			run("indexnow-notify", "completed", "failure"), // non-allowlist failure — ignored
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWait_NonAllowlistFailureIgnored(t *testing.T) {
	// Proves F16: a red indexnow-notify must NOT block when the allowlist is green.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "success"),
			run("build", "completed", "success"),
			run("indexnow-notify", "completed", "failure"),
			run("scorecard", "completed", "cancelled"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err != nil {
		t.Fatalf("non-allowlist failures must be ignored, got: %v", err)
	}
}

func TestWait_RequiredFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "failure"),
			run("build", "completed", "success"),
		))
	}))
	defer srv.Close()
	err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for a red required check")
	}
}

func TestWait_SkippedRequiredFailsClosed(t *testing.T) {
	// A gated job reporting completed/skipped must NOT satisfy the gate.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "skipped"),
			run("build", "completed", "success"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err == nil {
		t.Fatal("expected fail-closed on a skipped required check")
	}
}

func TestWait_StaleRequiredFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "stale"),
			run("build", "completed", "success"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err == nil {
		t.Fatal("expected fail-closed on a stale required check")
	}
}

func TestWait_RunningThenGreen(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			fmt.Fprint(w, crJSON(
				run("verify", "in_progress", ""),
				run("team-integration", "queued", ""),
				run("build", "completed", "success"),
			))
			return
		}
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "success"),
			run("build", "completed", "success"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 2*time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 polls, got %d", calls.Load())
	}
}

func TestWait_DiscoveryTimeout_MissingCheck(t *testing.T) {
	// build never appears → discovery timeout fails closed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "success"),
		))
	}))
	defer srv.Close()
	// small maxWait so discovery (=maxWait/2) elapses fast
	err := sc_wait(testChecker("t", srv.URL), 60*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected discovery timeout for a permanently-missing required check")
	}
}

func TestWait_EmptyChecksDiscoveryTimeout(t *testing.T) {
	// total_count:0 forever (the exact combined-status trap) → discovery timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON())
	}))
	defer srv.Close()
	if err := sc_wait(testChecker("t", srv.URL), 60*time.Millisecond, 100*time.Millisecond); err == nil {
		t.Fatal("expected discovery timeout on empty check-runs")
	}
}

// sc_wait runs WaitForSuccess with explicit discovery+max timeouts.
func sc_wait(sc *StatusChecker, disc, max time.Duration) error {
	return sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", allowlist, disc, max)
}

func TestWait_DuplicateNameNewestWins(t *testing.T) {
	// An old failing run and a newer successful re-run of the same name: newest wins.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			checkRun{Name: "verify", Status: "completed", Conclusion: "failure", ID: 1, StartedAt: "2026-07-17T00:00:00Z"},
			checkRun{Name: "verify", Status: "completed", Conclusion: "success", ID: 2, StartedAt: "2026-07-17T01:00:00Z"},
			run("team-integration", "completed", "success"),
			run("build", "completed", "success"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err != nil {
		t.Fatalf("newest re-run should win, got: %v", err)
	}
}

func TestWait_Pagination(t *testing.T) {
	// verify+team-integration on page 1, build on page 2 via Link header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, crJSON(run("build", "completed", "success")))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/x?page=2>; rel="next"`, "http://"+r.Host))
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "success"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err != nil {
		t.Fatalf("paginated build check should be found, got: %v", err)
	}
}

func TestWait_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(
			run("verify", "in_progress", ""),
			run("team-integration", "in_progress", ""),
			run("build", "in_progress", ""),
		))
	}))
	defer srv.Close()
	// present-but-never-completing: not a discovery miss, must hit the overall maxWait.
	if err := sc_wait(testChecker("t", srv.URL), 1*time.Millisecond, 3*time.Millisecond); err == nil {
		t.Fatal("expected overall timeout for never-completing checks")
	}
}

func TestWait_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, crJSON(run("verify", "in_progress", "")))
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitFast(testChecker("t", srv.URL), ctx, allowlist, 1*time.Minute); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestWait_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer my-gh-token" {
			t.Errorf("Authorization = %q, want Bearer my-gh-token", auth)
		}
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "success"),
			run("build", "completed", "success"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("my-gh-token", srv.URL), context.Background(), allowlist, 30*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWait_TransientErrorThenGreen(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"boom"}`)
			return
		}
		fmt.Fprint(w, crJSON(
			run("verify", "completed", "success"),
			run("team-integration", "completed", "success"),
			run("build", "completed", "success"),
		))
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWait_TooManyTransientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"message":"bad gateway"}`)
	}))
	defer srv.Close()
	if err := waitFast(testChecker("t", srv.URL), context.Background(), allowlist, 30*time.Second); err == nil {
		t.Fatal("expected error after too many transient errors")
	}
}

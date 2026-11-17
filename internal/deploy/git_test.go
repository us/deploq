package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeGit puts a git on PATH that fails `fails` times with stderr, then succeeds,
// and returns a function reporting how many times it ran.
func fakeGit(t *testing.T, fails int, stderr string) func() int {
	t.Helper()
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	script := `#!/bin/sh
n=$(cat "` + count + `" 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > "` + count + `"
if [ "$n" -le ` + strconv.Itoa(fails) + ` ]; then
  printf '%s\n' "` + stderr + `" >&2
  exit 128
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	old := fetchRetryDelays
	fetchRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { fetchRetryDelays = old })
	return func() int {
		b, _ := os.ReadFile(count)
		n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		return n
	}
}

const notFound = "ERROR: Repository not found.\nfatal: Could not read from remote repository."

func TestGitFetch_RetriesTransientErrorThenSucceeds(t *testing.T) {
	calls := fakeGit(t, 2, notFound)
	if out, err := GitFetch(context.Background(), t.TempDir(), "main"); err != nil {
		t.Fatalf("want success after retries, got %v (%s)", err, out)
	}
	if got := calls(); got != 3 {
		t.Fatalf("want 3 git runs, got %d", got)
	}
}

func TestGitFetch_GivesUpAfterBoundedRetries(t *testing.T) {
	calls := fakeGit(t, 100, notFound)
	out, err := GitFetch(context.Background(), t.TempDir(), "main")
	if err == nil {
		t.Fatal("want an error when every fetch fails")
	}
	if !strings.Contains(out, "Repository not found") {
		t.Fatalf("want the last stderr returned, got %q", out)
	}
	if got := calls(); got != 3 {
		t.Fatalf("want 3 git runs, got %d", got)
	}
}

func TestGitFetch_DoesNotRetryOtherErrors(t *testing.T) {
	calls := fakeGit(t, 100, "fatal: couldn't find remote ref nope")
	if _, err := GitFetch(context.Background(), t.TempDir(), "nope"); err == nil {
		t.Fatal("want an error")
	}
	if got := calls(); got != 1 {
		t.Fatalf("want 1 git run, got %d", got)
	}
}

func TestGitFetch_StopsWhenContextEnds(t *testing.T) {
	calls := fakeGit(t, 100, notFound)
	fetchRetryDelays = []time.Duration{time.Hour, time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := GitFetch(ctx, t.TempDir(), "main"); err == nil {
		t.Fatal("want an error")
	}
	if time.Since(start) > 10*time.Second || calls() != 1 {
		t.Fatalf("want one run and a prompt return, got %d runs in %s", calls(), time.Since(start))
	}
}

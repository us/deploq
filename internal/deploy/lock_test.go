package deploy

import (
	"sync"
	"testing"
)

// release finishes a deploy and asserts it did not request a rerun (the common
// case in these tests where no pending SHA was set).
func release(t *testing.T, l *ProjectLocker, project, sha string, success bool) {
	t.Helper()
	if _, rerun := l.FinishOrRerun(project, sha, success); rerun {
		t.Fatalf("unexpected rerun for %s@%s", project, sha)
	}
}

func TestTryAcquire(t *testing.T) {
	l := NewLocker()
	acquired, dup := l.TryAcquire("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	if dup {
		t.Fatal("expected not duplicate")
	}
	release(t, l, "project-a", "sha1", true)
}

func TestTryAcquire_AlreadyRunning(t *testing.T) {
	l := NewLocker()
	acquired, _ := l.TryAcquire("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	defer release(t, l, "project-a", "sha1", true)

	acquired2, dup2 := l.TryAcquire("project-a", "sha2")
	if acquired2 {
		t.Fatal("expected slot to be unavailable while running")
	}
	if dup2 {
		t.Fatal("expected not duplicate (different sha)")
	}
}

func TestTryAcquire_DifferentProjects(t *testing.T) {
	l := NewLocker()
	a1, _ := l.TryAcquire("project-a", "sha1")
	if !a1 {
		t.Fatal("expected to acquire project-a")
	}
	defer release(t, l, "project-a", "sha1", true)

	a2, _ := l.TryAcquire("project-b", "sha1")
	if !a2 {
		t.Fatal("expected to acquire project-b (independent)")
	}
	defer release(t, l, "project-b", "sha1", true)
}

func TestTryAcquire_DuplicateSHA(t *testing.T) {
	l := NewLocker()
	acquired, _ := l.TryAcquire("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	release(t, l, "project-a", "sha1", true) // records sha1

	acquired2, dup := l.TryAcquire("project-a", "sha1")
	if acquired2 {
		t.Fatal("expected duplicate sha to prevent acquisition")
	}
	if !dup {
		t.Fatal("expected duplicate=true")
	}
}

func TestTryAcquire_FailedDeploy_AllowsRetry(t *testing.T) {
	l := NewLocker()
	acquired, _ := l.TryAcquire("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	release(t, l, "project-a", "sha1", false) // failed — sha not recorded

	acquired2, dup := l.TryAcquire("project-a", "sha1")
	if !acquired2 {
		t.Fatal("expected to re-acquire after a failed deploy")
	}
	if dup {
		t.Fatal("expected not duplicate after failed deploy")
	}
	release(t, l, "project-a", "sha1", true)
}

func TestTryAcquire_DifferentSHA_AfterRelease(t *testing.T) {
	l := NewLocker()
	acquired, _ := l.TryAcquire("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	release(t, l, "project-a", "sha1", true)

	acquired2, dup := l.TryAcquire("project-a", "sha2")
	if !acquired2 {
		t.Fatal("expected to acquire with a different sha")
	}
	if dup {
		t.Fatal("expected not duplicate with a different sha")
	}
	release(t, l, "project-a", "sha2", true)
}

// A newer commit arriving mid-deploy reruns exactly once, deploying that SHA.
func TestFinishOrRerun_CoalesceNewer(t *testing.T) {
	l := NewLocker()
	acquired, _ := l.TryAcquire("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire")
	}

	// Two newer webhooks arrive while running; newest (sha3) wins.
	if !l.MarkPending("project-a", "sha2") {
		t.Fatal("expected MarkPending to succeed while running")
	}
	if !l.MarkPending("project-a", "sha3") {
		t.Fatal("expected second MarkPending to succeed while running")
	}

	// Finishing sha1 sees pending sha3 (advanced) -> rerun that one.
	next, rerun := l.FinishOrRerun("project-a", "sha1", true)
	if !rerun {
		t.Fatal("expected rerun after a newer pending sha")
	}
	if next != "sha3" {
		t.Fatalf("expected rerun sha3 (newest), got %q", next)
	}
	// Finishing the rerun with no new pending -> release.
	if _, rerun := l.FinishOrRerun("project-a", "sha3", true); rerun {
		t.Fatal("expected no rerun once pending is cleared")
	}

	acquired2, _ := l.TryAcquire("project-a", "sha4")
	if !acquired2 {
		t.Fatal("expected slot free after coalesced rerun completed")
	}
	release(t, l, "project-a", "sha4", true)
}

// A duplicate delivery of the SAME commit mid-deploy must NOT trigger a
// redundant rebuild.
func TestFinishOrRerun_DuplicateNoRerun(t *testing.T) {
	l := NewLocker()
	acquired, _ := l.TryAcquire("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	if !l.MarkPending("project-a", "sha1") { // same commit, duplicate webhook
		t.Fatal("expected MarkPending to succeed while running")
	}
	next, rerun := l.FinishOrRerun("project-a", "sha1", true)
	if rerun {
		t.Fatalf("expected NO rerun for a duplicate of the deployed sha, got rerun sha %q", next)
	}
	// Slot released, sha1 recorded as last deployed.
	if l.LastSHA("project-a") != "sha1" {
		t.Fatalf("expected lastSHA sha1, got %q", l.LastSHA("project-a"))
	}
}

// A redelivery of an ALREADY-deployed old commit that arrives while a different
// commit is deploying must be recognized as a duplicate (not coalesced into a
// redundant rerun).
func TestTryAcquire_DuplicateOfOldSHA_WhileRunning(t *testing.T) {
	l := NewLocker()
	// Deploy A, record it.
	acqA, _ := l.TryAcquire("project-a", "A")
	if !acqA {
		t.Fatal("expected to acquire A")
	}
	release(t, l, "project-a", "A", true)

	// Start deploying B (running).
	acqB, _ := l.TryAcquire("project-a", "B")
	if !acqB {
		t.Fatal("expected to acquire B")
	}
	// Redelivery of old A arrives mid-deploy → duplicate, NOT a pending rerun.
	acqA2, dupA := l.TryAcquire("project-a", "A")
	if acqA2 {
		t.Fatal("expected not acquired (B running)")
	}
	if !dupA {
		t.Fatal("expected duplicate=true for a redelivery of already-deployed A")
	}
	// B finishes with no pending → no rerun.
	if _, rerun := l.FinishOrRerun("project-a", "B", true); rerun {
		t.Fatal("expected no rerun; the A redelivery was a duplicate, not pending")
	}
}

// MarkPending on an idle project returns false so the caller starts fresh.
func TestMarkPending_NotRunning(t *testing.T) {
	l := NewLocker()
	if l.MarkPending("project-a", "sha1") {
		t.Fatal("expected MarkPending=false when no deploy is running")
	}
}

func TestTryAcquire_Concurrent_SingleFlight(t *testing.T) {
	l := NewLocker()
	const n = 100
	var acquiredCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			acquired, _ := l.TryAcquire("project-a", "unique-sha")
			if acquired {
				mu.Lock()
				acquiredCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Exactly one goroutine may hold the single-flight slot at once; none
	// released, so exactly one acquired.
	if acquiredCount != 1 {
		t.Fatalf("expected exactly one acquisition, got %d", acquiredCount)
	}
}

func TestLastSHA(t *testing.T) {
	l := NewLocker()
	if sha := l.LastSHA("nonexistent"); sha != "" {
		t.Errorf("expected empty SHA for nonexistent project, got %q", sha)
	}

	acquired, _ := l.TryAcquire("project-a", "sha123")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	release(t, l, "project-a", "sha123", true)

	if sha := l.LastSHA("project-a"); sha != "sha123" {
		t.Errorf("LastSHA = %q, want %q", sha, "sha123")
	}
}

func TestLastSHA_NotRecordedOnFailure(t *testing.T) {
	l := NewLocker()
	acquired, _ := l.TryAcquire("project-a", "sha123")
	if !acquired {
		t.Fatal("expected to acquire")
	}
	release(t, l, "project-a", "sha123", false)

	if sha := l.LastSHA("project-a"); sha != "" {
		t.Errorf("LastSHA should be empty after failed deploy, got %q", sha)
	}
}

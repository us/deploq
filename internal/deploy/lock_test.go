package deploy

import (
	"sync"
	"testing"
)

func TestTryLock_Acquire(t *testing.T) {
	l := NewLocker()
	unlock, dup, acquired := l.TryLock("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire lock")
	}
	if dup {
		t.Fatal("expected not duplicate")
	}
	unlock(true)
}

func TestTryLock_AlreadyLocked(t *testing.T) {
	l := NewLocker()
	unlock, _, acquired := l.TryLock("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire lock")
	}
	defer unlock(true)

	_, _, acquired2 := l.TryLock("project-a", "sha2")
	if acquired2 {
		t.Fatal("expected lock to be unavailable")
	}
}

func TestTryLock_DifferentProjects(t *testing.T) {
	l := NewLocker()
	unlock1, _, acquired1 := l.TryLock("project-a", "sha1")
	if !acquired1 {
		t.Fatal("expected to acquire lock for project-a")
	}
	defer unlock1(true)

	unlock2, _, acquired2 := l.TryLock("project-b", "sha1")
	if !acquired2 {
		t.Fatal("expected to acquire lock for project-b (different project)")
	}
	defer unlock2(true)
}

func TestTryLock_DuplicateSHA(t *testing.T) {
	l := NewLocker()
	unlock, _, acquired := l.TryLock("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire lock")
	}
	unlock(true) // releases lock and records SHA

	_, dup, acquired2 := l.TryLock("project-a", "sha1")
	if acquired2 {
		t.Fatal("expected duplicate SHA to prevent acquisition")
	}
	if !dup {
		t.Fatal("expected isDuplicate=true")
	}
}

func TestTryLock_FailedDeploy_AllowsRetry(t *testing.T) {
	l := NewLocker()
	unlock, _, acquired := l.TryLock("project-a", "sha1")
	if !acquired {
		t.Fatal("expected to acquire lock")
	}
	unlock(false) // failed deploy — SHA should NOT be recorded

	unlock2, dup, acquired2 := l.TryLock("project-a", "sha1")
	if !acquired2 {
		t.Fatal("expected to acquire lock — failed deploy should not record SHA")
	}
	if dup {
		t.Fatal("expected not duplicate after failed deploy")
	}
	unlock2(true)
}

func TestTryLock_DifferentSHA_AfterRelease(t *testing.T) {
	l := NewLocker()
	unlock, _, _ := l.TryLock("project-a", "sha1")
	unlock(true)

	unlock2, dup, acquired := l.TryLock("project-a", "sha2")
	if !acquired {
		t.Fatal("expected to acquire lock with different SHA")
	}
	if dup {
		t.Fatal("expected not duplicate with different SHA")
	}
	unlock2(true)
}

func TestTryLock_Concurrent(t *testing.T) {
	l := NewLocker()
	const n = 100
	var acquiredCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, _, acquired := l.TryLock("project-a", "unique-sha")
			if acquired {
				mu.Lock()
				acquiredCount++
				mu.Unlock()
				unlock(true)
			}
		}()
	}
	wg.Wait()

	if acquiredCount == 0 {
		t.Fatal("expected at least one goroutine to acquire the lock")
	}
}

func TestLastSHA(t *testing.T) {
	l := NewLocker()
	if sha := l.LastSHA("nonexistent"); sha != "" {
		t.Errorf("expected empty SHA for nonexistent project, got %q", sha)
	}

	unlock, _, _ := l.TryLock("project-a", "sha123")
	unlock(true)

	if sha := l.LastSHA("project-a"); sha != "sha123" {
		t.Errorf("LastSHA = %q, want %q", sha, "sha123")
	}
}

func TestLastSHA_NotRecordedOnFailure(t *testing.T) {
	l := NewLocker()
	unlock, _, _ := l.TryLock("project-a", "sha123")
	unlock(false)

	if sha := l.LastSHA("project-a"); sha != "" {
		t.Errorf("LastSHA should be empty after failed deploy, got %q", sha)
	}
}

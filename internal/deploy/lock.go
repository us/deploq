package deploy

import "sync"

// ProjectLocker manages per-project deploy locks.
// Uses non-blocking TryLock to prevent goroutine queuing.
type ProjectLocker struct {
	mu    sync.Mutex
	locks map[string]*projectLock
}

type projectLock struct {
	mu      sync.Mutex
	lastSHA string
}

// NewLocker creates a new ProjectLocker.
func NewLocker() *ProjectLocker {
	return &ProjectLocker{
		locks: make(map[string]*projectLock),
	}
}

// TryLock attempts to acquire the deploy lock for a project.
// Returns (unlock func, isDuplicate bool, acquired bool).
// The unlock function takes a success bool — SHA is only recorded on success,
// so failed deploys can be retried with the same SHA.
func (l *ProjectLocker) TryLock(project, sha string) (unlock func(success bool), isDuplicate bool, acquired bool) {
	l.mu.Lock()
	pl, ok := l.locks[project]
	if !ok {
		pl = &projectLock{}
		l.locks[project] = pl
	}
	l.mu.Unlock()

	if !pl.mu.TryLock() {
		return nil, false, false
	}

	// Check duplicate SHA inside the lock (prevents race condition)
	if sha != "" && pl.lastSHA == sha {
		pl.mu.Unlock()
		return nil, true, false
	}

	unlock = func(success bool) {
		if sha != "" && success {
			pl.lastSHA = sha
		}
		pl.mu.Unlock()
	}

	return unlock, false, true
}

// LastSHA returns the last successfully deployed SHA for a project.
func (l *ProjectLocker) LastSHA(project string) string {
	l.mu.Lock()
	pl, ok := l.locks[project]
	l.mu.Unlock()
	if !ok {
		return ""
	}
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.lastSHA
}

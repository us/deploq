package deploy

import "sync"

// ProjectLocker enforces single-flight deploys per project and coalesces
// webhooks that arrive while a deploy is running down to a single "deploy the
// latest" rerun. All state is guarded by one mutex so acquire / mark-pending /
// finish are atomic with respect to each other (no lost-wakeup race).
//
// Model: at most one deploy runs per project (`running`). A webhook that can't
// acquire records its SHA in `pendingSHA` instead of being dropped; when the
// running deploy finishes it reruns ONLY if that pending SHA differs from the
// one just deployed (i.e. origin/main actually advanced). N piled-up webhooks
// therefore collapse to one final deploy of the newest commit — only the last
// one matters — while pure duplicate deliveries of the same commit do NOT
// trigger a redundant rebuild.
type ProjectLocker struct {
	mu    sync.Mutex
	locks map[string]*projectLock
}

type projectLock struct {
	running    bool   // a deploy currently owns the single-flight slot
	pendingSHA string // newest SHA requested while running ("" = none)
	lastSHA    string // last SHA deployed successfully (duplicate suppression)
}

// NewLocker creates a new ProjectLocker.
func NewLocker() *ProjectLocker {
	return &ProjectLocker{locks: make(map[string]*projectLock)}
}

// get returns (creating if needed) the lock for a project. Caller holds l.mu.
func (l *ProjectLocker) get(project string) *projectLock {
	pl, ok := l.locks[project]
	if !ok {
		pl = &projectLock{}
		l.locks[project] = pl
	}
	return pl
}

// TryAcquire attempts to start a deploy for a project.
//   - acquired:  the caller now owns the single-flight slot and MUST eventually
//     call FinishOrRerun until it returns false.
//   - duplicate: this SHA was already deployed successfully; nothing to do.
//
// If neither is true, a deploy is already running — the caller should call
// MarkPending to coalesce this request onto the in-flight deploy.
func (l *ProjectLocker) TryAcquire(project, sha string) (acquired, duplicate bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pl := l.get(project)
	// Duplicate check FIRST: a redelivery of an already-deployed SHA must be
	// recognized as a duplicate even while a *different* commit is deploying,
	// so it isn't recorded as pending and doesn't trigger a redundant rerun.
	// (The in-flight commit's own redelivery isn't in lastSHA yet, so it still
	// falls through to the running check and is coalesced + suppressed later.)
	if sha != "" && pl.lastSHA == sha {
		return false, true
	}
	if pl.running {
		return false, false
	}
	pl.running = true
	return true, false
}

// MarkPending records the SHA of a newer deploy request that arrived while one
// is running (newest wins). Returns false if no deploy is actually running (a
// slot freed in the race window between a failed TryAcquire and this call) so
// the caller can retry.
func (l *ProjectLocker) MarkPending(project, sha string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	pl := l.get(project)
	if !pl.running {
		return false
	}
	pl.pendingSHA = sha
	return true
}

// FinishOrRerun records a deploy's outcome and reports whether to immediately
// redeploy the pending commit. It reruns ONLY when a pending SHA was recorded
// that differs from the one just deployed — i.e. origin/main actually advanced;
// a duplicate delivery of the same commit clears pending without a redundant
// rebuild. On rerun it keeps the slot owned by the caller and returns the SHA
// to deploy next; otherwise it releases the slot. lastSHA is recorded only on
// success, so a failed deploy can be retried with the same SHA.
func (l *ProjectLocker) FinishOrRerun(project, deployedSHA string, success bool) (nextSHA string, rerun bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pl := l.get(project)
	if success && deployedSHA != "" {
		pl.lastSHA = deployedSHA
	}
	pending := pl.pendingSHA
	pl.pendingSHA = ""
	if pending != "" && pending != deployedSHA {
		return pending, true // keep running == true; caller loops on `pending`
	}
	pl.running = false
	return "", false
}

// LastSHA returns the last successfully deployed SHA for a project.
func (l *ProjectLocker) LastSHA(project string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	pl, ok := l.locks[project]
	if !ok {
		return ""
	}
	return pl.lastSHA
}

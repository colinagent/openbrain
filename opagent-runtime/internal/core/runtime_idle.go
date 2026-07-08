package core

import (
	"sync"
	"time"
)

var runtimeWorkTracker = struct {
	mu             sync.RWMutex
	lastActivityAt time.Time
}{
	lastActivityAt: time.Now().UTC(),
}

func noteRuntimeWorkActivity(now time.Time) {
	runtimeWorkTracker.mu.Lock()
	defer runtimeWorkTracker.mu.Unlock()
	runtimeWorkTracker.lastActivityAt = now.UTC()
}

func IsRuntimeIdle(gracePeriod time.Duration) bool {
	runtimeLoops.mu.RLock()
	hasActiveLoops := len(runtimeLoops.loopsByThread) > 0
	runtimeLoops.mu.RUnlock()
	if hasActiveLoops {
		return false
	}

	if gracePeriod <= 0 {
		return true
	}

	runtimeWorkTracker.mu.RLock()
	lastActivityAt := runtimeWorkTracker.lastActivityAt
	runtimeWorkTracker.mu.RUnlock()
	if lastActivityAt.IsZero() {
		return true
	}
	return time.Since(lastActivityAt) >= gracePeriod
}

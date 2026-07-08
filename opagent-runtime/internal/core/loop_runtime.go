package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	queueKindSteering = string(op.ThreadQueueKindSteering)
	queueKindFollowUp = string(op.ThreadQueueKindFollowUp)
)

var errRuntimeLoopNotRunning = errors.New("runtime loop is not running")

type runtimeLoopRegistry struct {
	mu            sync.RWMutex
	loopsByThread map[string]*runtimeLoop
}

type runtimeLoop struct {
	threadID string
	chatPath string
	cancel   context.CancelFunc
}

type runtimeLoopNotRunningError struct {
	threadID string
}

func (e runtimeLoopNotRunningError) Error() string {
	return fmt.Sprintf("no running loop for thread: %s", e.threadID)
}

func (e runtimeLoopNotRunningError) Is(target error) bool {
	return target == errRuntimeLoopNotRunning
}

var runtimeLoops = &runtimeLoopRegistry{
	loopsByThread: make(map[string]*runtimeLoop),
}

func newRuntimeLoop(threadID, chatPath string, cancel context.CancelFunc) *runtimeLoop {
	return &runtimeLoop{
		threadID: strings.TrimSpace(threadID),
		chatPath: strings.TrimSpace(chatPath),
		cancel:   cancel,
	}
}

func registerRuntimeLoop(loop *runtimeLoop) error {
	if loop == nil || loop.threadID == "" {
		return fmt.Errorf("runtime loop with valid threadID is required")
	}
	runtimeLoops.mu.Lock()
	defer runtimeLoops.mu.Unlock()
	if runtimeLoops.loopsByThread[loop.threadID] != nil {
		return fmt.Errorf("runtime loop already exists for thread: %s", loop.threadID)
	}
	runtimeLoops.loopsByThread[loop.threadID] = loop
	noteRuntimeWorkActivity(time.Now().UTC())
	return nil
}

func unregisterRuntimeLoop(threadID string, loop *runtimeLoop) {
	trimmedThreadID := strings.TrimSpace(threadID)
	if trimmedThreadID == "" {
		return
	}
	runtimeLoops.mu.Lock()
	defer runtimeLoops.mu.Unlock()
	current := runtimeLoops.loopsByThread[trimmedThreadID]
	if current == nil {
		return
	}
	if loop != nil && current != loop {
		return
	}
	delete(runtimeLoops.loopsByThread, trimmedThreadID)
	noteRuntimeWorkActivity(time.Now().UTC())
}

func getRuntimeLoopByThreadID(threadID string) (*runtimeLoop, error) {
	trimmedThreadID := strings.TrimSpace(threadID)
	if trimmedThreadID == "" {
		return nil, fmt.Errorf("threadID is required")
	}
	runtimeLoops.mu.RLock()
	defer runtimeLoops.mu.RUnlock()
	loop := runtimeLoops.loopsByThread[trimmedThreadID]
	if loop == nil {
		return nil, runtimeLoopNotRunningError{threadID: trimmedThreadID}
	}
	return loop, nil
}

func isRuntimeThreadActive(threadID string) bool {
	trimmedThreadID := strings.TrimSpace(threadID)
	if trimmedThreadID == "" {
		return false
	}
	runtimeLoops.mu.RLock()
	defer runtimeLoops.mu.RUnlock()
	return runtimeLoops.loopsByThread[trimmedThreadID] != nil
}

func listRuntimeThreads() []op.ThreadRuntimeInfo {
	runtimeLoops.mu.RLock()
	defer runtimeLoops.mu.RUnlock()

	threads := make([]op.ThreadRuntimeInfo, 0, len(runtimeLoops.loopsByThread))
	for threadID, loop := range runtimeLoops.loopsByThread {
		if loop == nil {
			continue
		}
		threads = append(threads, op.ThreadRuntimeInfo{
			ThreadID: strings.TrimSpace(threadID),
			ChatPath: strings.TrimSpace(loop.chatPath),
		})
	}
	sort.Slice(threads, func(i, j int) bool {
		if threads[i].ThreadID == threads[j].ThreadID {
			return threads[i].ChatPath < threads[j].ChatPath
		}
		return threads[i].ThreadID < threads[j].ThreadID
	})
	return threads
}

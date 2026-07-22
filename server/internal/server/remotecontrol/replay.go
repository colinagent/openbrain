package remotecontrol

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

type replayKey struct {
	uid           string
	environmentID string
	clientID      string
	requestID     string
}

type replayResult struct {
	payload     json.RawMessage
	remoteError *protocol.RemoteError
}

type replayEntry struct {
	fingerprint [sha256.Size]byte
	done        chan struct{}
	completedAt time.Time
	result      replayResult
}

type replayCache struct {
	mu             sync.Mutex
	entries        map[replayKey]*replayEntry
	window         time.Duration
	maxEntries     int
	maxResultBytes int
	now            func() time.Time
}

func newReplayCache(window time.Duration, maxEntries, maxResultBytes int) *replayCache {
	return &replayCache{
		entries:        make(map[replayKey]*replayEntry),
		window:         window,
		maxEntries:     maxEntries,
		maxResultBytes: maxResultBytes,
		now:            time.Now,
	}
}

func (c *replayCache) Do(
	ctx context.Context,
	principal Principal,
	request protocol.Envelope,
	execute func() replayResult,
) replayResult {
	key := replayKey{
		uid:           principal.UID,
		environmentID: principal.EnvironmentID,
		clientID:      principal.ClientID,
		requestID:     request.RequestID,
	}
	fingerprint := requestFingerprint(request.Operation, request.Payload)

	c.mu.Lock()
	c.deleteExpiredLocked(c.now())
	if existing, ok := c.entries[key]; ok {
		if existing.fingerprint != fingerprint {
			c.mu.Unlock()
			return replayFailure(protocol.ErrorRequestConflict, "requestID was already used for different content", false)
		}
		done := existing.done
		c.mu.Unlock()
		select {
		case <-done:
			return cloneReplayResult(existing.result)
		case <-ctx.Done():
			return replayFailure(protocol.ErrorRequestConflict, "matching request is still in progress", true)
		}
	}
	if len(c.entries) >= c.maxEntries {
		c.mu.Unlock()
		return replayFailure(protocol.ErrorRateLimited, "request replay window is full", true)
	}
	entry := &replayEntry{fingerprint: fingerprint, done: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()

	result := cloneReplayResult(execute())
	if replayResultSize(result) > c.maxResultBytes {
		result = replayFailure(protocol.ErrorInternal, "mutating response exceeded the replay size limit", false)
	}
	c.mu.Lock()
	entry.result = result
	entry.completedAt = c.now()
	close(entry.done)
	c.mu.Unlock()
	return cloneReplayResult(result)
}

func (c *replayCache) deleteExpiredLocked(now time.Time) {
	for key, entry := range c.entries {
		if !entry.completedAt.IsZero() && now.Sub(entry.completedAt) >= c.window {
			delete(c.entries, key)
		}
	}
}

func requestFingerprint(operation protocol.Operation, payload json.RawMessage) [sha256.Size]byte {
	canonicalPayload := payload
	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if decoder.Decode(&value) == nil {
		if encoded, err := json.Marshal(value); err == nil {
			canonicalPayload = encoded
		}
	}
	material := make([]byte, 0, len(operation)+1+len(canonicalPayload))
	material = append(material, operation...)
	material = append(material, 0)
	material = append(material, canonicalPayload...)
	return sha256.Sum256(material)
}

func cloneReplayResult(result replayResult) replayResult {
	cloned := replayResult{payload: append(json.RawMessage(nil), result.payload...)}
	if result.remoteError != nil {
		remoteErr := *result.remoteError
		remoteErr.Details = append(json.RawMessage(nil), result.remoteError.Details...)
		cloned.remoteError = &remoteErr
	}
	return cloned
}

func replayFailure(code protocol.ErrorCode, message string, retryable bool) replayResult {
	return replayResult{remoteError: &protocol.RemoteError{
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}}
}

func replayResultSize(result replayResult) int {
	size := len(result.payload)
	if result.remoteError != nil {
		size += len(result.remoteError.Code)
		size += len(result.remoteError.Message)
		size += len(result.remoteError.Details)
	}
	return size
}

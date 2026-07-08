package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx/compaction"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

var (
	autoRetryMaxRetries = 3
	autoRetryBaseDelay  = 2 * time.Second
	autoRetryMaxDelay   = 60 * time.Second
)

type autoRetryStartPayload struct {
	Attempt      int    `json:"attempt"`
	MaxAttempts  int    `json:"maxAttempts"`
	DelayMs      int64  `json:"delayMs"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type autoRetryEndPayload struct {
	Success    bool   `json:"success"`
	Attempt    int    `json:"attempt"`
	FinalError string `json:"finalError,omitempty"`
}

func (l *AgentLoop) streamAssistantTurnResultWithRetry() (assistantTurnResult, error) {
	retryAttempts := 0
	overflowCompacted := false
	modelRefreshed := false

	for {
		result, err := l.streamAssistantTurnResult()
		if err == nil {
			if retryAttempts > 0 {
				emitAutoRetryEnd(l.Meta, autoRetryEndPayload{
					Success: true,
					Attempt: retryAttempts,
				})
			}
			return result, nil
		}

		if compaction.IsOverflowError(err) {
			if overflowCompacted {
				if retryAttempts > 0 {
					emitAutoRetryEnd(l.Meta, autoRetryEndPayload{
						Success:    false,
						Attempt:    retryAttempts,
						FinalError: err.Error(),
					})
				}
				return assistantTurnResult{}, err
			}
			if compactErr := l.forceCompact(); compactErr != nil {
				if retryAttempts > 0 {
					emitAutoRetryEnd(l.Meta, autoRetryEndPayload{
						Success:    false,
						Attempt:    retryAttempts,
						FinalError: compactErr.Error(),
					})
				}
				return assistantTurnResult{}, compactErr
			}
			overflowCompacted = true
			continue
		}

		if isRetryModelUnavailableError(err) && !modelRefreshed {
			if refreshErr := l.refreshModelClientForRetry(); refreshErr == nil {
				modelRefreshed = true
				continue
			}
		}

		retryErr := ai.NormalizeRetryError(err)
		if retryErr == nil || !retryErr.Retryable {
			if retryAttempts > 0 {
				emitAutoRetryEnd(l.Meta, autoRetryEndPayload{
					Success:    false,
					Attempt:    retryAttempts,
					FinalError: err.Error(),
				})
			}
			if retryErr != nil {
				return assistantTurnResult{}, retryErr
			}
			return assistantTurnResult{}, err
		}
		if retryAttempts >= autoRetryMaxRetries {
			emitAutoRetryEnd(l.Meta, autoRetryEndPayload{
				Success:    false,
				Attempt:    retryAttempts,
				FinalError: retryErr.Error(),
			})
			return assistantTurnResult{}, retryErr
		}

		delay := resolveAutoRetryDelay(retryAttempts+1, retryErr)
		retryAttempts++
		emitAutoRetryStart(l.Meta, autoRetryStartPayload{
			Attempt:      retryAttempts,
			MaxAttempts:  autoRetryMaxRetries,
			DelayMs:      delay.Milliseconds(),
			ErrorMessage: retryErr.Error(),
		})
		if err := sleepWithContext(l.Ctx, delay); err != nil {
			cancelledErr := context.Canceled
			emitAutoRetryEnd(l.Meta, autoRetryEndPayload{
				Success:    false,
				Attempt:    retryAttempts,
				FinalError: "Retry cancelled",
			})
			return assistantTurnResult{}, cancelledErr
		}
	}
}

func (l *AgentLoop) refreshModelClientForRetry() error {
	if l == nil || l.rebuildModel == nil {
		return fmt.Errorf("model rebuild is unavailable")
	}
	model, err := l.rebuildModel(l.Ctx, l.Meta.Clone())
	if err != nil {
		return err
	}
	l.Model = model
	return nil
}

func isRetryModelUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "has no available providers")
}

func resolveAutoRetryDelay(nextAttempt int, retryErr *ai.RetryError) time.Duration {
	if nextAttempt <= 0 {
		nextAttempt = 1
	}
	delay := autoRetryBaseDelay
	for i := 1; i < nextAttempt; i++ {
		delay *= 2
		if autoRetryMaxDelay > 0 && delay >= autoRetryMaxDelay {
			delay = autoRetryMaxDelay
			break
		}
	}
	if retryErr != nil && retryErr.RetryAfterMs > 0 {
		retryAfter := time.Duration(retryErr.RetryAfterMs) * time.Millisecond
		if retryAfter < 0 {
			retryAfter = 0
		}
		if autoRetryMaxDelay > 0 && retryAfter > autoRetryMaxDelay {
			retryAfter = autoRetryMaxDelay
		}
		return retryAfter
	}
	if delay < 0 {
		return 0
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func emitAutoRetryStart(meta op.Meta, payload autoRetryStartPayload) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type": "auto_retry_start",
	}), &op.JsonContent{Raw: raw})
}

func emitAutoRetryEnd(meta op.Meta, payload autoRetryEndPayload) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type": "auto_retry_end",
	}), &op.JsonContent{Raw: raw})
}

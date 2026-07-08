package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx/compaction"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func compactSessionContext(ctx context.Context, threadMeta op.ThreadMeta, meta op.Meta) error {
	_ = threadMeta
	threadID := strings.TrimSpace(metaString(meta, "threadID"))
	agentID := strings.TrimSpace(metaString(meta, "agentID"))
	if threadID == "" {
		threadID = strings.TrimSpace(threadMeta.ThreadID)
	}
	if threadID == "" {
		return fmt.Errorf("threadID is required")
	}
	if agentID == "" {
		agentID = strings.TrimSpace(threadMeta.AgentID)
	}
	if agentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	sessionCtx, err := loadThreadContext(threadID, agentID)
	if err != nil {
		return fmt.Errorf("load thread context: %w", err)
	}
	if len(sessionCtx.canonicalMessages) == 0 {
		return fmt.Errorf("nothing to compact")
	}
	if len(sessionCtx.canonicalMessages) == 1 && isCanonicalContextCheckpoint(sessionCtx.canonicalMessages[0]) {
		return fmt.Errorf("already compacted")
	}

	cfg := config.GetConfig()
	var compactionCfg op.CompactionConfig
	if cfg != nil {
		compactionCfg = cfg.Compaction
	}
	_, _, keepRecentTokens := compaction.ResolveSettings(compactionCfg)
	tokensBefore := estimateCanonicalContextTokens(sessionCtx.canonicalMessages, "")

	summarize := func(summaryCtx context.Context, conversation string) (string, error) {
		model, err := newCompactionModelClient(summaryCtx, compactionCfg, sessionCtx.meta, meta)
		if err != nil {
			return "", err
		}
		return generateCompactionSummaryWithModel(summaryCtx, conversation, model, meta)
	}

	compacted, err := compactCanonicalMessages(ctx, sessionCtx.canonicalMessages, keepRecentTokens, summarize)
	if err != nil {
		return err
	}
	if len(compacted) == len(sessionCtx.canonicalMessages) {
		return fmt.Errorf("nothing to compact")
	}
	return replaceThreadCanonicalMessagesWithCompaction(sessionCtx.meta, compacted, tokensBefore)
}

func isCanonicalContextCheckpoint(msg ai.ConversationMessage) bool {
	if msg.Role != ai.RoleCanonicalSystem {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(canonicalSummaryText(msg)), "Context checkpoint summary:")
}

func newCompactionModelClient(ctx context.Context, compactionCfg op.CompactionConfig, threadMeta op.ThreadMeta, meta op.Meta) (*ModelClient, error) {
	modelID := strings.TrimSpace(compactionCfg.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(metaString(meta, "modelKey"))
	}
	if modelID == "" {
		return nil, fmt.Errorf("compaction model is not configured")
	}
	modelMeta := op.Meta{}
	if meta != nil {
		modelMeta = meta.Clone()
	}
	modelMeta["modelKey"] = modelID
	delete(modelMeta, "model")
	model, err := NewModelClient(ctx, modelID, modelMeta)
	if err != nil {
		return nil, err
	}
	return model, nil
}

func generateCompactionSummaryWithModel(ctx context.Context, conversation string, model *ModelClient, meta op.Meta) (string, error) {
	if model == nil || model.config == nil {
		return "", fmt.Errorf("compaction model is not initialized")
	}
	if strings.TrimSpace(conversation) == "" {
		return "", nil
	}
	modelName := strings.TrimSpace(model.config.Name)
	if modelName == "" {
		return "", fmt.Errorf("compaction model name is empty")
	}
	canonical := model.canonicalProvider()
	if canonical == nil {
		return "", fmt.Errorf("compaction model provider is not initialized")
	}
	resp, err := canonical.CompleteCanonical(ctx, &ai.ProviderRequest{
		Context: ai.ConversationContext{
			SystemPrompt: compaction.SummarizationSystemPrompt,
			Messages: []ai.ConversationMessage{{
				Role: ai.RoleCanonicalUser,
				Content: []ai.ContentBlock{{
					Type: ai.BlockText,
					Text: conversation,
				}},
			}},
		},
		Config: ai.GenerationConfig{
			Model:       modelName,
			ServiceTier: serviceTierForModelMeta(model.config, meta),
		},
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("empty compaction summary response")
	}
	msg, err := ai.OpMessageFromCanonical(resp.Message)
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(msg.Content)
	if summary == "" {
		slog.Warn("compaction model returned empty summary")
	}
	return summary, nil
}

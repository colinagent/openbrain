package opagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	protocol "github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
)

func TestProductRuntimeOptionsInstallGBrainScope(t *testing.T) {
	options, err := ProductRuntimeOptions(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta := protocol.Meta{"gbrainQueryScope": map[string]any{"kind": "source", "sourceID": "source-1", "label": "Research"}}
	prompt, err := options.PromptAugmenter(context.Background(), "Base", gbrainAgentID, meta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `source_id "source-1"`) {
		t.Fatalf("prompt = %q", prompt)
	}
	params, err := options.ToolScope(context.Background(), meta, gbrainCloudNodeID, "query", nil, map[string]any{"query": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := params.(map[string]any)["source_id"]; got != "source-1" {
		t.Fatalf("source_id = %#v", got)
	}
	if _, err := options.ToolScope(context.Background(), meta, gbrainCloudNodeID, "query", nil, map[string]any{"source_id": "source-2"}); err == nil {
		t.Fatal("out-of-scope source was accepted")
	}
}

func TestOpenBrainCronPolicyProtectsCloudSyncIdentity(t *testing.T) {
	valid := productCronTask{ID: cloudSyncTaskID}
	valid.Target.Kind = "agent"
	valid.Target.AgentID = cloudSyncTargetAgentID
	valid.Payload = validCloudSyncPayload()
	content := cronPolicyContent(t, map[string]any{"task": valid})
	if err := openBrainCronPolicy(context.Background(), protocol.OpCode("cron/upsert"), nil, content); err != nil {
		t.Fatalf("managed task rejected: %v", err)
	}

	for name, mutate := range map[string]func(*productCronTask){
		"forged id":     func(task *productCronTask) { task.ID = "task-forged" },
		"forged target": func(task *productCronTask) { task.Target.AgentID = "agent-forged" },
		"missing skill": func(task *productCronTask) { delete(task.Payload.Data, "selectedSkillIDs") },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Payload.Data = clonePolicyMap(valid.Payload.Data)
			mutate(&candidate)
			if err := openBrainCronPolicy(context.Background(), protocol.OpCode("cron/upsert"), nil, cronPolicyContent(t, map[string]any{"task": candidate})); err == nil {
				t.Fatal("forged managed task was accepted")
			}
		})
	}

	normal := productCronTask{ID: "task-user"}
	normal.Target.Kind = "agent"
	normal.Target.AgentID = "agent-coder"
	normal.Payload = productCronPayload{Kind: "agentTurn", Text: "User task", Data: map[string]any{"modelKey": "chat"}}
	if err := openBrainCronPolicy(context.Background(), protocol.OpCode("cron/upsert"), nil, cronPolicyContent(t, map[string]any{"task": normal})); err != nil {
		t.Fatalf("ordinary cron task rejected: %v", err)
	}
}

func TestOpenBrainCronPolicyValidatesManualCloudSyncOverride(t *testing.T) {
	valid := validCloudSyncPayload()
	if err := openBrainCronPolicy(context.Background(), protocol.OpCode("cron/run"), nil, cronPolicyContent(t, map[string]any{"id": cloudSyncTaskID, "payload": valid})); err != nil {
		t.Fatalf("manual payload rejected: %v", err)
	}
	invalid := valid
	invalid.Data = clonePolicyMap(valid.Data)
	invalid.Data["managedKind"] = "forged"
	if err := openBrainCronPolicy(context.Background(), protocol.OpCode("cron/run"), nil, cronPolicyContent(t, map[string]any{"id": cloudSyncTaskID, "payload": invalid})); err == nil {
		t.Fatal("forged manual payload was accepted")
	}
}

func validCloudSyncPayload() productCronPayload {
	return productCronPayload{
		Kind: cloudSyncPayloadKind,
		Text: cloudSyncPromptPreamble + "\n\nUse the managed skill.",
		Data: map[string]any{
			"managedKind":          cloudSyncManagedKind,
			"selectedSkillIDs":     []any{cloudSyncSkillID},
			"selectedSkillContext": map[string]any{"managedKind": cloudSyncManagedKind},
		},
	}
}

func cronPolicyContent(t *testing.T, value any) *protocol.JsonContent {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.NewJsonContentRaw(raw)
}

func clonePolicyMap(values map[string]any) map[string]any {
	next := make(map[string]any, len(values))
	for key, value := range values {
		next[key] = value
	}
	return next
}

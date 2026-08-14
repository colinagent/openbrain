package opagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	protocol "github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
)

const (
	gbrainAgentID           = "agent-gbrain"
	cloudSyncTaskID         = "task-openbrain-cloud-sync"
	cloudSyncManagedKind    = "openbrain-cloud-sync"
	cloudSyncSkillID        = "skill-openbrain-cloud-sync"
	cloudSyncTargetAgentID  = "agent-coder"
	cloudSyncPayloadKind    = "agentTurn"
	cloudSyncPromptPreamble = "Run OpenBrain Cloud Sync."
)

func gbrainPromptAugmenter(_ context.Context, prompt, agentID string, meta protocol.Meta) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if strings.TrimSpace(agentID) != gbrainAgentID {
		return prompt, nil
	}
	scope := metaObject(meta, "gbrainQueryScope")
	if scope == nil || strings.TrimSpace(metaString(scope, "kind")) != "source" {
		return prompt, nil
	}
	sourceID := strings.TrimSpace(metaString(scope, "sourceID"))
	if sourceID == "" {
		return prompt, nil
	}
	label := strings.TrimSpace(metaString(scope, "label"))
	if label == "" {
		label = sourceID
	}
	scopePrompt := strings.Join([]string{
		"## OpenBrain GBrain Query Scope",
		"",
		fmt.Sprintf("This turn was started from OpenBrain graph scope %q.", label),
		fmt.Sprintf("Only use GBrain Cloud results from source_id %q.", sourceID),
		fmt.Sprintf("When calling the gbrain-cloud query tool, include source_id %q.", sourceID),
		"Do not use search or an unscoped query unless the user explicitly asks to broaden the scope.",
	}, "\n")
	if prompt == "" {
		return scopePrompt, nil
	}
	return prompt + "\n\n" + scopePrompt, nil
}

func gbrainToolScope(_ context.Context, meta protocol.Meta, serverID, toolName string, inputSchema, params any) (any, error) {
	if !isGBrainSourceScopedTool(serverID, toolName, inputSchema, params) {
		return params, nil
	}
	scope := metaObject(meta, "gbrainQueryScope")
	if scope == nil || strings.TrimSpace(metaString(scope, "kind")) != "source" {
		return params, nil
	}
	sourceID := strings.TrimSpace(metaString(scope, "sourceID"))
	if sourceID == "" {
		return params, errors.New("GBrain query scope has no allowed source ID; refresh the OpenBrain graph")
	}
	arguments, ok := params.(map[string]any)
	if !ok {
		return params, errors.New("GBrain query scope requires object tool arguments")
	}
	requested := strings.TrimSpace(stringValue(arguments["source_id"]))
	if requested != "" && requested != sourceID {
		return params, fmt.Errorf("source_id %q is outside the OpenBrain graph scope", requested)
	}
	if requested == sourceID {
		return params, nil
	}
	next := make(map[string]any, len(arguments)+1)
	for key, value := range arguments {
		next[key] = value
	}
	next["source_id"] = sourceID
	return next, nil
}

func isGBrainSourceScopedTool(serverID, toolName string, inputSchema, params any) bool {
	if !strings.Contains(strings.ToLower(strings.TrimSpace(serverID)), "gbrain-cloud") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "query", "search":
		return true
	}
	if schema, ok := inputSchema.(map[string]any); ok {
		if properties, ok := schema["properties"].(map[string]any); ok {
			if _, ok := properties["source_id"]; ok {
				return true
			}
		}
	}
	if arguments, ok := params.(map[string]any); ok {
		_, ok = arguments["source_id"]
		return ok
	}
	return false
}

type productCronTask struct {
	ID     string `json:"id"`
	Target struct {
		Kind    string `json:"kind"`
		AgentID string `json:"agentID"`
	} `json:"target"`
	Payload productCronPayload `json:"payload"`
}

type productCronPayload struct {
	Kind string         `json:"kind"`
	Text string         `json:"text"`
	Data map[string]any `json:"data"`
}

func openBrainCronPolicy(_ context.Context, opCode protocol.OpCode, _ protocol.Meta, content protocol.Content) error {
	raw, err := cronContentJSON(content)
	if err != nil {
		return err
	}
	switch string(opCode) {
	case "cron/add", "cron/upsert", "cron/update":
		var request struct {
			Task productCronTask `json:"task"`
		}
		if len(raw) != 0 {
			if err := json.Unmarshal(raw, &request); err != nil {
				return fmt.Errorf("decode OpenBrain cron write policy: %w", err)
			}
		}
		return authorizeCloudSyncTask(request.Task)
	case "cron/run":
		var request struct {
			ID      string              `json:"id"`
			Payload *productCronPayload `json:"payload"`
		}
		if len(raw) != 0 {
			if err := json.Unmarshal(raw, &request); err != nil {
				return fmt.Errorf("decode OpenBrain cron run policy: %w", err)
			}
		}
		if strings.TrimSpace(request.ID) != cloudSyncTaskID {
			if request.Payload != nil && isCloudSyncPayload(*request.Payload) {
				return errors.New("OpenBrain Cloud Sync payload is reserved for its product-managed task")
			}
			return nil
		}
		if request.Payload == nil {
			return nil
		}
		return validateCloudSyncPayload(*request.Payload)
	default:
		return nil
	}
}

func cronContentJSON(content protocol.Content) ([]byte, error) {
	if content == nil {
		return nil, nil
	}
	jsonContent, ok := content.(*protocol.JsonContent)
	if !ok || jsonContent == nil {
		return nil, errors.New("OpenBrain cron policy requires JSON content")
	}
	return jsonContent.Raw, nil
}

func authorizeCloudSyncTask(task productCronTask) error {
	protected := strings.TrimSpace(task.ID) == cloudSyncTaskID || isCloudSyncPayload(task.Payload)
	if !protected {
		return nil
	}
	if strings.TrimSpace(task.ID) != cloudSyncTaskID {
		return errors.New("OpenBrain Cloud Sync identity is reserved for its product-managed task")
	}
	if strings.TrimSpace(task.Target.Kind) != "agent" || strings.TrimSpace(task.Target.AgentID) != cloudSyncTargetAgentID {
		return errors.New("OpenBrain Cloud Sync must target the managed Coder agent")
	}
	return validateCloudSyncPayload(task.Payload)
}

func validateCloudSyncPayload(payload productCronPayload) error {
	if strings.TrimSpace(payload.Kind) != cloudSyncPayloadKind || !strings.HasPrefix(strings.TrimSpace(payload.Text), cloudSyncPromptPreamble) {
		return errors.New("OpenBrain Cloud Sync payload shape does not match product policy")
	}
	if strings.TrimSpace(stringValue(payload.Data["managedKind"])) != cloudSyncManagedKind {
		return errors.New("OpenBrain Cloud Sync managed kind is missing")
	}
	if !stringListContains(payload.Data["selectedSkillIDs"], cloudSyncSkillID) {
		return errors.New("OpenBrain Cloud Sync must select its product skill")
	}
	context := metaObject(payload.Data, "selectedSkillContext")
	if context == nil || strings.TrimSpace(metaString(context, "managedKind")) != cloudSyncManagedKind {
		return errors.New("OpenBrain Cloud Sync selected skill context is invalid")
	}
	return nil
}

func isCloudSyncPayload(payload productCronPayload) bool {
	if strings.TrimSpace(stringValue(payload.Data["managedKind"])) == cloudSyncManagedKind {
		return true
	}
	if stringListContains(payload.Data["selectedSkillIDs"], cloudSyncSkillID) {
		return true
	}
	context := metaObject(payload.Data, "selectedSkillContext")
	return context != nil && strings.TrimSpace(metaString(context, "managedKind")) == cloudSyncManagedKind
}

func metaObject(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	value, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case protocol.Meta:
		return map[string]any(typed)
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func metaString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	return stringValue(values[key])
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func stringListContains(value any, expected string) bool {
	switch values := value.(type) {
	case []string:
		for _, value := range values {
			if strings.TrimSpace(value) == expected {
				return true
			}
		}
	case []any:
		for _, value := range values {
			if strings.TrimSpace(stringValue(value)) == expected {
				return true
			}
		}
	}
	return false
}

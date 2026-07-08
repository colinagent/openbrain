package ws

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/colinagent/openbrain/server/internal/server/storage"
)

const defaultOpenBrainSyncIntervalSec = 300
const openBrainCloudSyncTaskID = "task-openbrain-cloud-sync"
const openBrainCloudSyncTaskName = "OpenBrain Cloud Sync"
const openBrainCloudSyncManagedKind = "openbrain-cloud-sync"
const openBrainCloudSyncSkillID = "skill-openbrain-cloud-sync"

func (h *Handler) handleStorageStatus(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.StorageStatusParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	result, err := h.storage.Status(p)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: err.Error()}
	}
	modelSelection := openBrainCloudSyncModelFromParams(p.ModelKey, p.ThinkingLevel, p.ContextWindow, p.ServiceTier)
	if _, rpcErr := h.ensureOpenBrainCloudSyncCron(false, nil, modelSelection); rpcErr != nil {
		result.Status = "error"
		result.Error = rpcErr.Message
		result.Message = "Failed to prepare sync cron: " + rpcErr.Message
	}
	return result, nil
}

func (h *Handler) handleStorageSyncNow(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.StorageSyncNowParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	result, err := h.storage.Status(protocol.StorageStatusParams{WorkspaceID: p.WorkspaceID, Path: p.Path})
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: err.Error()}
	}
	modelSelection := openBrainCloudSyncModelFromParams(p.ModelKey, p.ThinkingLevel, p.ContextWindow, p.ServiceTier)
	queued, rpcErr := h.ensureOpenBrainCloudSyncCron(true, &openBrainCloudSyncRunRequest{
		WorkspaceID: result.WorkspaceID,
		Path:        result.Path,
	}, modelSelection)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if queued {
		result.Status = "syncing"
		result.Message = "Workspace sync queued."
	}
	return result, nil
}

func (h *Handler) handleStorageUpdatePolicy(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.StorageUpdatePolicyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	result, err := h.storage.UpdatePolicy(p)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: err.Error()}
	}
	modelSelection := openBrainCloudSyncModelFromParams(p.ModelKey, p.ThinkingLevel, p.ContextWindow, p.ServiceTier)
	if _, rpcErr := h.ensureOpenBrainCloudSyncCron(false, nil, modelSelection); rpcErr != nil {
		result.Status = "error"
		result.Error = rpcErr.Message
		result.Message = "Failed to prepare sync cron: " + rpcErr.Message
	}
	return result, nil
}

type openBrainCloudSyncRunRequest struct {
	WorkspaceID string
	Path        string
}

type openBrainCloudSyncModelSelection struct {
	ModelKey      string
	ThinkingLevel string
	ContextWindow int64
	ServiceTier   string
}

func (h *Handler) ensureOpenBrainCloudSyncCron(runNow bool, request *openBrainCloudSyncRunRequest, modelSelection openBrainCloudSyncModelSelection) (bool, *protocol.RPCError) {
	bindings, err := h.storage.CronBindings()
	if err != nil {
		return false, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: err.Error()}
	}
	accountUID := h.storage.CurrentAccountUID()
	task := openBrainCloudSyncTask(accountUID, bindings, modelSelection)
	listRaw, rpcErr := h.callHostNode(op.OpCode(protocol.MethodCronList), protocol.CronListParams{})
	if rpcErr != nil {
		return false, rpcErr
	}
	list, ok := decodeCronListResult(listRaw)
	if !ok {
		return false, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: "Failed to decode cron task list"}
	}
	for _, record := range list.Tasks {
		if !isLegacyOpenBrainSyncTask(record) {
			continue
		}
		_, rpcErr = h.callHostNode(op.OpCode(protocol.MethodCronRemove), protocol.CronIDParams{ID: record.Task.ID})
		if rpcErr != nil {
			return false, rpcErr
		}
	}
	if existing, ok := findCronTaskRecord(list.Tasks, task.ID); ok {
		task = mergeOpenBrainCloudSyncTask(existing, task)
	}
	if strings.TrimSpace(stringFromMap(task.Payload.Data, "modelKey")) == "" {
		return false, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "OpenBrain Cloud Sync requires an explicit modelKey from the creation request. Open Models > Policies and set Default Chat Model to an enabled chat model, then retry."}
	}
	if existing, ok := findCronTaskRecord(list.Tasks, task.ID); ok {
		if !sameCronTaskDefinition(existing.Task, task) {
			_, rpcErr = h.callHostNode(op.OpCode(protocol.MethodCronUpsert), protocol.CronTaskUpsertParams{Task: task})
		}
	} else {
		_, rpcErr = h.callHostNode(op.OpCode(protocol.MethodCronUpsert), protocol.CronTaskUpsertParams{Task: task})
	}
	if rpcErr != nil {
		return false, rpcErr
	}
	if runNow {
		params := protocol.CronIDParams{ID: task.ID}
		if request != nil && runRequestPresent(*request) {
			override := openBrainCloudSyncManualPayload(accountUID, bindings, *request, openBrainCloudSyncModelFromPayload(task.Payload.Data))
			params.Payload = &override
		}
		_, rpcErr = h.callHostNode(op.OpCode(protocol.MethodCronRun), params)
		if rpcErr != nil {
			return false, rpcErr
		}
		return true, nil
	}
	return false, nil
}

func isLegacyOpenBrainSyncTask(record protocol.CronTaskRecord) bool {
	return record.Task.Target.AgentID == "agent-openbrain-sync" ||
		strings.HasPrefix(strings.TrimSpace(record.Task.ID), "task-openbrain-sync-") ||
		stringFromMap(record.Task.Payload.Data, "managedKind") == "openbrain-sync"
}

func openBrainCloudSyncTask(accountUID string, bindings []storage.WorkspaceCronBinding, modelSelection openBrainCloudSyncModelSelection) protocol.CronTask {
	accountUID = strings.TrimSpace(accountUID)
	intervalSec := defaultOpenBrainSyncIntervalSec
	enabled := false
	workspaceSnapshot := make([]interface{}, 0, len(bindings))
	cwd := defaultOpenBrainCloudSyncCWD()
	for _, binding := range bindings {
		workspacePath := strings.TrimSpace(binding.WorkspacePath)
		if workspacePath == "" {
			continue
		}
		if cwd == defaultOpenBrainCloudSyncCWD() {
			cwd = workspacePath
		}
		if binding.Enabled && accountUID != "" {
			enabled = true
			nextInterval := defaultSyncIntervalSec(binding.IntervalSec)
			if intervalSec <= 0 || nextInterval < intervalSec {
				intervalSec = nextInterval
			}
		}
		if binding.Enabled {
			workspaceSnapshot = append(workspaceSnapshot, openBrainCloudSyncWorkspacePayload(binding))
		}
	}
	digest := openBrainCloudSyncBindingDigest(accountUID, bindings)
	payload := openBrainCloudSyncPayload(nil, nil, false, accountUID, digest, workspaceSnapshot, modelSelection)
	return protocol.CronTask{
		ID:          openBrainCloudSyncTaskID,
		Name:        openBrainCloudSyncTaskName,
		Description: "Sync OpenBrain Cloud workspaces through agent-coder and the OpenBrain Cloud Sync skill.",
		Enabled:     enabled,
		Schedule: protocol.CronTaskSchedule{
			Every: formatSyncInterval(intervalSec),
		},
		Target: protocol.CronTaskTarget{
			Kind:    "agent",
			AgentID: "agent-coder",
			CWD:     cwd,
		},
		Payload: payload,
	}
}

func openBrainCloudSyncManualPayload(accountUID string, bindings []storage.WorkspaceCronBinding, request openBrainCloudSyncRunRequest, modelSelection openBrainCloudSyncModelSelection) protocol.CronTaskPayload {
	workspaces := make([]interface{}, 0, 1)
	for _, binding := range bindings {
		if !requestedWorkspace(binding, request) {
			continue
		}
		if strings.TrimSpace(binding.WorkspacePath) == "" {
			continue
		}
		workspaces = append(workspaces, openBrainCloudSyncWorkspacePayload(binding))
	}
	requestData := map[string]interface{}{}
	if strings.TrimSpace(request.WorkspaceID) != "" {
		requestData["workspaceID"] = strings.TrimSpace(request.WorkspaceID)
	}
	if strings.TrimSpace(request.Path) != "" {
		requestData["workspacePath"] = strings.TrimSpace(request.Path)
	}
	return openBrainCloudSyncPayload(workspaces, requestData, true, strings.TrimSpace(accountUID), openBrainCloudSyncBindingDigest(accountUID, bindings), workspaces, modelSelection)
}

func openBrainCloudSyncWorkspacePayload(binding storage.WorkspaceCronBinding) map[string]interface{} {
	return map[string]interface{}{
		"workspaceID":   strings.TrimSpace(binding.WorkspaceID),
		"orgID":         strings.TrimSpace(binding.OrgID),
		"workspacePath": strings.TrimSpace(binding.WorkspacePath),
		"workspaceName": openBrainSyncWorkspaceName(binding),
		"repoURL":       strings.TrimSpace(binding.RepoURL),
		"branch":        strings.TrimSpace(binding.Branch),
		"locationKind":  strings.TrimSpace(binding.LocationKind),
		"enabled":       binding.Enabled,
		"intervalSec":   defaultSyncIntervalSec(binding.IntervalSec),
	}
}

func openBrainCloudSyncPayload(workspaces []interface{}, request map[string]interface{}, manual bool, accountUID string, bindingDigest string, workspaceSnapshot []interface{}, modelSelection openBrainCloudSyncModelSelection) protocol.CronTaskPayload {
	if request == nil {
		request = map[string]interface{}{}
	}
	if workspaces == nil {
		workspaces = []interface{}{}
	}
	if workspaceSnapshot == nil {
		workspaceSnapshot = []interface{}{}
	}
	data := map[string]interface{}{
		"managedKind":   openBrainCloudSyncManagedKind,
		"nameMode":      "auto",
		"defaultName":   openBrainCloudSyncTaskName,
		"accountUID":    strings.TrimSpace(accountUID),
		"bindingDigest": strings.TrimSpace(bindingDigest),
		"selectedSkillIDs": []interface{}{
			openBrainCloudSyncSkillID,
		},
		"selectedSkillContext": map[string]interface{}{
			"managedKind":        openBrainCloudSyncManagedKind,
			"accountUID":         strings.TrimSpace(accountUID),
			"bindingDigest":      strings.TrimSpace(bindingDigest),
			"workspaces":         workspaces,
			"workspaceSnapshot":  workspaceSnapshot,
			"requestedWorkspace": request,
			"manual":             manual,
		},
		"workspaces":                workspaces,
		"workspaceSnapshot":         workspaceSnapshot,
		"requestedWorkspace":        request,
		"manualRunIncludesDisabled": manual,
	}
	setOpenBrainCloudSyncModel(data, modelSelection)
	return protocol.CronTaskPayload{
		Kind: "agentTurn",
		Text: openBrainCloudSyncTaskText(workspaces, request, manual),
		Data: data,
	}
}

func openBrainCloudSyncBindingDigest(accountUID string, bindings []storage.WorkspaceCronBinding) string {
	type digestBinding struct {
		WorkspaceID   string `json:"workspaceID"`
		OrgID         string `json:"orgID"`
		WorkspacePath string `json:"workspacePath"`
		RepoURL       string `json:"repoURL"`
		Branch        string `json:"branch"`
		LocationKind  string `json:"locationKind"`
		Enabled       bool   `json:"enabled"`
		IntervalSec   int    `json:"intervalSec"`
	}
	payload := struct {
		AccountUID string          `json:"accountUID"`
		Bindings   []digestBinding `json:"bindings"`
	}{
		AccountUID: strings.TrimSpace(accountUID),
		Bindings:   make([]digestBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		payload.Bindings = append(payload.Bindings, digestBinding{
			WorkspaceID:   strings.TrimSpace(binding.WorkspaceID),
			OrgID:         strings.TrimSpace(binding.OrgID),
			WorkspacePath: strings.TrimSpace(binding.WorkspacePath),
			RepoURL:       strings.TrimSpace(binding.RepoURL),
			Branch:        strings.TrimSpace(binding.Branch),
			LocationKind:  strings.TrimSpace(binding.LocationKind),
			Enabled:       binding.Enabled,
			IntervalSec:   defaultSyncIntervalSec(binding.IntervalSec),
		})
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum)
}

func openBrainCloudSyncModelFromParams(modelKey string, thinkingLevel string, contextWindow int64, serviceTier string) openBrainCloudSyncModelSelection {
	if contextWindow < 0 {
		contextWindow = 0
	}
	return openBrainCloudSyncModelSelection{
		ModelKey:      strings.TrimSpace(modelKey),
		ThinkingLevel: strings.TrimSpace(thinkingLevel),
		ContextWindow: contextWindow,
		ServiceTier:   strings.TrimSpace(serviceTier),
	}
}

func openBrainCloudSyncModelFromPayload(data map[string]interface{}) openBrainCloudSyncModelSelection {
	if data == nil {
		return openBrainCloudSyncModelSelection{}
	}
	return openBrainCloudSyncModelFromParams(
		stringFromMap(data, "modelKey"),
		stringFromMap(data, "thinkingLevel"),
		int64FromMap(data, "contextWindow"),
		stringFromMap(data, "serviceTier"),
	)
}

func setOpenBrainCloudSyncModel(data map[string]interface{}, modelSelection openBrainCloudSyncModelSelection) {
	if data == nil {
		return
	}
	if strings.TrimSpace(modelSelection.ModelKey) != "" {
		data["modelKey"] = strings.TrimSpace(modelSelection.ModelKey)
	}
	if strings.TrimSpace(modelSelection.ThinkingLevel) != "" {
		data["thinkingLevel"] = strings.TrimSpace(modelSelection.ThinkingLevel)
	}
	if modelSelection.ContextWindow > 0 {
		data["contextWindow"] = modelSelection.ContextWindow
	}
	if strings.TrimSpace(modelSelection.ServiceTier) != "" {
		data["serviceTier"] = strings.TrimSpace(modelSelection.ServiceTier)
	}
}

func requestedWorkspace(binding storage.WorkspaceCronBinding, request openBrainCloudSyncRunRequest) bool {
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	if workspaceID != "" && strings.TrimSpace(binding.WorkspaceID) == workspaceID {
		return true
	}
	requestPath := strings.TrimSpace(request.Path)
	return requestPath != "" && filepath.Clean(strings.TrimSpace(binding.WorkspacePath)) == filepath.Clean(requestPath)
}

func runRequestPresent(request openBrainCloudSyncRunRequest) bool {
	return strings.TrimSpace(request.WorkspaceID) != "" || strings.TrimSpace(request.Path) != ""
}

func defaultOpenBrainCloudSyncCWD() string {
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return strings.TrimSpace(home)
	}
	return string(filepath.Separator)
}

func openBrainCloudSyncTaskText(workspaces []interface{}, request map[string]interface{}, manual bool) string {
	if !manual {
		return strings.TrimSpace(`Run OpenBrain Cloud Sync.

This is a scheduled run. Use the selected OpenBrain Cloud Sync skill for this turn. Do not trust any persisted cron workspace snapshot as the source of truth. First run the helper preflight; it reads the current OpenBrain auth uid and the current account partition from the local workspace index. Process only the workspaces returned by that helper preflight. Skip clean workspaces. For workspaces with changes or sync issues, use the helper for standard sync when it is safe. If the helper reports conflicts, nested git/gitlink problems, rebase failures, destructive choices, cloud permission errors, account binding misses, or any uncertainty, inspect with normal git commands when possible and publish a Messenger message to the user before making irreversible decisions.`)
	}
	requestRaw, _ := json.MarshalIndent(request, "", "  ")
	raw, _ := json.MarshalIndent(workspaces, "", "  ")
	return strings.TrimSpace(`Run OpenBrain Cloud Sync.

This is a manual run. Use the selected OpenBrain Cloud Sync skill for this turn. The requested workspace below was resolved from the current OpenBrain account partition. First run the helper preflight with --include-disabled for this manual request, then sync only the requested workspace. If the helper returns workspace_not_bound_for_account, cloud_permission_denied, conflicts, nested git/gitlink problems, rebase failures, destructive choices, or any uncertainty, inspect with normal git commands when possible and publish a Messenger message to the user before making irreversible decisions.

Requested workspace JSON:
` + string(requestRaw) + `

Workspaces JSON:
` + string(raw))
}

func openBrainSyncWorkspaceName(binding storage.WorkspaceCronBinding) string {
	name := strings.TrimSpace(binding.LocalName)
	if name == "" {
		name = filepath.Base(strings.TrimSpace(binding.WorkspacePath))
	}
	if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
		return "workspace"
	}
	return name
}

func mergeOpenBrainCloudSyncTask(existing interface{}, next protocol.CronTask) protocol.CronTask {
	current, ok := decodeCronTaskRecord(existing)
	if !ok {
		return next
	}
	merged := next
	merged.CreatedAtMs = current.CreatedAtMs
	existingData := current.Payload.Data
	if existingData == nil {
		existingData = map[string]interface{}{}
	}
	if merged.Payload.Data == nil {
		merged.Payload.Data = map[string]interface{}{}
	}
	nameMode := strings.TrimSpace(stringFromMap(existingData, "nameMode"))
	if nameMode == "" {
		existingDefaultName := stringFromMap(existingData, "defaultName")
		nextDefaultName := stringFromMap(next.Payload.Data, "defaultName")
		currentName := strings.TrimSpace(current.Name)
		if currentName != "" && currentName != existingDefaultName && currentName != nextDefaultName {
			nameMode = "custom"
		} else {
			nameMode = "auto"
		}
	}
	if nameMode == "custom" && strings.TrimSpace(current.Name) != "" {
		merged.Name = strings.TrimSpace(current.Name)
	} else {
		nameMode = "auto"
	}
	merged.Payload.Data["nameMode"] = nameMode
	if existingModel := openBrainCloudSyncModelFromPayload(existingData); existingModel.ModelKey != "" {
		delete(merged.Payload.Data, "thinkingLevel")
		delete(merged.Payload.Data, "contextWindow")
		delete(merged.Payload.Data, "serviceTier")
		setOpenBrainCloudSyncModel(merged.Payload.Data, existingModel)
	}
	return merged
}

func decodeCronTaskRecord(value interface{}) (protocol.CronTask, bool) {
	raw, err := json.Marshal(value)
	if err != nil {
		return protocol.CronTask{}, false
	}
	var record protocol.CronTaskRecord
	if err := json.Unmarshal(raw, &record); err == nil && strings.TrimSpace(record.Task.ID) != "" {
		return record.Task, true
	}
	var task protocol.CronTask
	if err := json.Unmarshal(raw, &task); err == nil && strings.TrimSpace(task.ID) != "" {
		return task, true
	}
	return protocol.CronTask{}, false
}

func decodeCronListResult(value interface{}) (protocol.CronListResult, bool) {
	if value == nil {
		return protocol.CronListResult{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return protocol.CronListResult{}, false
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return protocol.CronListResult{}, false
	}
	var result protocol.CronListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return protocol.CronListResult{}, false
	}
	return result, true
}

func findCronTaskRecord(records []protocol.CronTaskRecord, id string) (protocol.CronTaskRecord, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return protocol.CronTaskRecord{}, false
	}
	for _, record := range records {
		if strings.TrimSpace(record.Task.ID) == id {
			return record, true
		}
	}
	return protocol.CronTaskRecord{}, false
}

func sameCronTaskDefinition(left protocol.CronTask, right protocol.CronTask) bool {
	type comparableCronTask struct {
		ID          string                    `json:"id"`
		Name        string                    `json:"name"`
		Description string                    `json:"description,omitempty"`
		Enabled     bool                      `json:"enabled"`
		Schedule    protocol.CronTaskSchedule `json:"schedule"`
		Target      protocol.CronTaskTarget   `json:"target"`
		Payload     protocol.CronTaskPayload  `json:"payload"`
		CreatedAtMs int64                     `json:"createdAtMs,omitempty"`
	}
	leftRaw, leftErr := json.Marshal(comparableCronTask{
		ID:          left.ID,
		Name:        left.Name,
		Description: left.Description,
		Enabled:     left.Enabled,
		Schedule:    left.Schedule,
		Target:      left.Target,
		Payload:     left.Payload,
		CreatedAtMs: left.CreatedAtMs,
	})
	rightRaw, rightErr := json.Marshal(comparableCronTask{
		ID:          right.ID,
		Name:        right.Name,
		Description: right.Description,
		Enabled:     right.Enabled,
		Schedule:    right.Schedule,
		Target:      right.Target,
		Payload:     right.Payload,
		CreatedAtMs: right.CreatedAtMs,
	})
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

func stringFromMap(data map[string]interface{}, key string) string {
	value, ok := data[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func int64FromMap(data map[string]interface{}, key string) int64 {
	value, ok := data[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		if typed > 0 {
			return typed
		}
	case int:
		if typed > 0 {
			return int64(typed)
		}
	case float64:
		const maxInt64AsFloat = float64(1<<63 - 1)
		if typed > 0 && typed <= maxInt64AsFloat && typed == float64(int64(typed)) {
			return int64(typed)
		}
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil && parsed > 0 {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func formatSyncInterval(intervalSec int) string {
	intervalSec = defaultSyncIntervalSec(intervalSec)
	if intervalSec%60 == 0 {
		return strconv.Itoa(intervalSec/60) + "m"
	}
	return strconv.Itoa(intervalSec) + "s"
}

func defaultSyncIntervalSec(intervalSec int) int {
	if intervalSec <= 0 {
		return defaultOpenBrainSyncIntervalSec
	}
	return intervalSec
}

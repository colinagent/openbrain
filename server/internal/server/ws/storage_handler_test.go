package ws

import (
	"strings"
	"testing"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/colinagent/openbrain/server/internal/server/storage"
)

func TestOpenBrainCloudSyncTaskUsesOneStableID(t *testing.T) {
	task := openBrainCloudSyncTask("user-current", []storage.WorkspaceCronBinding{
		{
			WorkspaceID:   "ws-alpha",
			OrgID:         "org",
			WorkspacePath: "/tmp/alpha",
			LocalName:     "Alpha",
			RepoURL:       "https://github.com/example/alpha.git",
			Branch:        "main",
			Enabled:       true,
			IntervalSec:   900,
		},
		{
			WorkspaceID:   "ws-beta",
			OrgID:         "org",
			WorkspacePath: "/tmp/beta",
			LocalName:     "Beta",
			RepoURL:       "https://github.com/example/beta.git",
			Branch:        "trunk",
			Enabled:       true,
			IntervalSec:   300,
		},
		{
			WorkspaceID:   "ws-off",
			OrgID:         "org",
			WorkspacePath: "/tmp/off",
			LocalName:     "Off",
			RepoURL:       "https://github.com/example/off.git",
			Branch:        "main",
			Enabled:       false,
			IntervalSec:   60,
		},
	}, openBrainCloudSyncModelSelection{ModelKey: "test:model", ThinkingLevel: "high"})

	if task.ID != openBrainCloudSyncTaskID {
		t.Fatalf("ID = %q, want %q", task.ID, openBrainCloudSyncTaskID)
	}
	if task.Name != openBrainCloudSyncTaskName {
		t.Fatalf("Name = %q, want %q", task.Name, openBrainCloudSyncTaskName)
	}
	if task.Target.AgentID != "agent-coder" {
		t.Fatalf("AgentID = %q, want agent-coder", task.Target.AgentID)
	}
	if task.Schedule.Every != "5m" {
		t.Fatalf("Every = %q, want shortest enabled interval 5m", task.Schedule.Every)
	}
	if task.Payload.Data["managedKind"] != openBrainCloudSyncManagedKind {
		t.Fatalf("managedKind = %v", task.Payload.Data["managedKind"])
	}
	if task.Payload.Data["modelKey"] != "test:model" {
		t.Fatalf("modelKey = %v, want test:model", task.Payload.Data["modelKey"])
	}
	if task.Payload.Data["thinkingLevel"] != "high" {
		t.Fatalf("thinkingLevel = %v, want high", task.Payload.Data["thinkingLevel"])
	}
	if task.Payload.Data["accountUID"] != "user-current" {
		t.Fatalf("accountUID = %v, want user-current", task.Payload.Data["accountUID"])
	}
	if got := task.Payload.Data["bindingDigest"]; !strings.HasPrefix(got.(string), "sha256:") {
		t.Fatalf("bindingDigest = %v, want sha256 digest", got)
	}
	selected, ok := task.Payload.Data["selectedSkillIDs"].([]interface{})
	if !ok || len(selected) != 1 || selected[0] != openBrainCloudSyncSkillID {
		t.Fatalf("selectedSkillIDs = %#v", task.Payload.Data["selectedSkillIDs"])
	}
	context, ok := task.Payload.Data["selectedSkillContext"].(map[string]interface{})
	if !ok {
		t.Fatalf("selectedSkillContext = %#v", task.Payload.Data["selectedSkillContext"])
	}
	workspaces, ok := context["workspaces"].([]interface{})
	if !ok || len(workspaces) != 0 {
		t.Fatalf("workspaces = %#v", context["workspaces"])
	}
	snapshot, ok := context["workspaceSnapshot"].([]interface{})
	if !ok || len(snapshot) != 2 {
		t.Fatalf("workspaceSnapshot = %#v", context["workspaceSnapshot"])
	}
	if strings.Contains(task.Payload.Text, "ws-alpha") || strings.Contains(task.Payload.Text, "ws-off") {
		t.Fatalf("scheduled prompt should not include workspace snapshot as an authoritative list: %q", task.Payload.Text)
	}
	if !strings.Contains(task.Payload.Text, "helper preflight") {
		t.Fatalf("payload text missing helper preflight guidance: %q", task.Payload.Text)
	}
}

func TestOpenBrainCloudSyncManualPayloadIncludesRequestedDisabledWorkspace(t *testing.T) {
	payload := openBrainCloudSyncManualPayload("user-current", []storage.WorkspaceCronBinding{
		{
			WorkspaceID:   "ws-off",
			OrgID:         "org",
			WorkspacePath: "/tmp/off",
			LocalName:     "Off",
			RepoURL:       "https://github.com/example/off.git",
			Branch:        "main",
			Enabled:       false,
			IntervalSec:   60,
		},
	}, openBrainCloudSyncRunRequest{WorkspaceID: "ws-off"}, openBrainCloudSyncModelSelection{ModelKey: "test:model", ThinkingLevel: "high"})

	if payload.Data["manualRunIncludesDisabled"] != true {
		t.Fatalf("manualRunIncludesDisabled = %#v", payload.Data["manualRunIncludesDisabled"])
	}
	if payload.Data["modelKey"] != "test:model" {
		t.Fatalf("modelKey = %v, want test:model", payload.Data["modelKey"])
	}
	if payload.Data["accountUID"] != "user-current" {
		t.Fatalf("accountUID = %v, want user-current", payload.Data["accountUID"])
	}
	context, ok := payload.Data["selectedSkillContext"].(map[string]interface{})
	if !ok {
		t.Fatalf("selectedSkillContext = %#v", payload.Data["selectedSkillContext"])
	}
	workspaces, ok := context["workspaces"].([]interface{})
	if !ok || len(workspaces) != 1 {
		t.Fatalf("workspaces = %#v", context["workspaces"])
	}
	if !strings.Contains(payload.Text, "manual run") || !strings.Contains(payload.Text, "--include-disabled") {
		t.Fatalf("manual payload text missing manual guidance: %q", payload.Text)
	}
}

func TestOpenBrainCloudSyncTaskDisabledWhenNoWorkspaceAutoSync(t *testing.T) {
	task := openBrainCloudSyncTask("user-current", []storage.WorkspaceCronBinding{
		{
			WorkspaceID:   "ws-alpha",
			WorkspacePath: "/tmp/alpha",
			LocalName:     "Alpha",
			Enabled:       false,
			IntervalSec:   60,
		},
	}, openBrainCloudSyncModelSelection{ModelKey: "test:model"})
	if task.Enabled {
		t.Fatal("Enabled = true, want false")
	}
	if task.Schedule.Every != "5m" {
		t.Fatalf("Every = %q, want default 5m when no enabled workspace", task.Schedule.Every)
	}
}

func TestOpenBrainCloudSyncTaskDisabledWhenNoAccountUID(t *testing.T) {
	task := openBrainCloudSyncTask("", []storage.WorkspaceCronBinding{
		{
			WorkspaceID:   "ws-alpha",
			WorkspacePath: "/tmp/alpha",
			LocalName:     "Alpha",
			Enabled:       true,
			IntervalSec:   60,
		},
	}, openBrainCloudSyncModelSelection{ModelKey: "test:model"})
	if task.Enabled {
		t.Fatal("Enabled = true, want false without current account uid")
	}
	if got := task.Payload.Data["accountUID"]; got != "" {
		t.Fatalf("accountUID = %v, want empty", got)
	}
}

func TestMergeOpenBrainCloudSyncTaskPreservesCustomName(t *testing.T) {
	next := openBrainCloudSyncTask("user-current", []storage.WorkspaceCronBinding{
		{
			WorkspaceID:   "ws-alpha",
			WorkspacePath: "/tmp/alpha",
			LocalName:     "Alpha",
			Enabled:       true,
			IntervalSec:   300,
		},
	}, openBrainCloudSyncModelSelection{ModelKey: "next:model", ThinkingLevel: "medium"})
	current := next
	current.Name = "My Cloud Sync"
	current.Payload.Data["nameMode"] = "custom"
	current.Payload.Data["modelKey"] = "current:model"
	current.Payload.Data["thinkingLevel"] = "low"

	merged := mergeOpenBrainCloudSyncTask(protocol.CronTaskRecord{Task: current}, next)
	if merged.Name != "My Cloud Sync" {
		t.Fatalf("Name = %q, want custom name", merged.Name)
	}
	if got := merged.Payload.Data["nameMode"]; got != "custom" {
		t.Fatalf("nameMode = %v, want custom", got)
	}
	if got := merged.Payload.Data["modelKey"]; got != "current:model" {
		t.Fatalf("modelKey = %v, want current:model", got)
	}
	if got := merged.Payload.Data["thinkingLevel"]; got != "low" {
		t.Fatalf("thinkingLevel = %v, want low", got)
	}
}

func TestMergeOpenBrainCloudSyncTaskBackfillsMissingModel(t *testing.T) {
	current := openBrainCloudSyncTask("", nil, openBrainCloudSyncModelSelection{})
	next := openBrainCloudSyncTask("", nil, openBrainCloudSyncModelSelection{ModelKey: "next:model", ThinkingLevel: "high"})

	merged := mergeOpenBrainCloudSyncTask(protocol.CronTaskRecord{Task: current}, next)
	if got := merged.Payload.Data["modelKey"]; got != "next:model" {
		t.Fatalf("modelKey = %v, want next:model", got)
	}
	if got := merged.Payload.Data["thinkingLevel"]; got != "high" {
		t.Fatalf("thinkingLevel = %v, want high", got)
	}
}

func TestSameCronTaskDefinitionIgnoresUpdatedAt(t *testing.T) {
	left := openBrainCloudSyncTask("user-current", []storage.WorkspaceCronBinding{
		{
			WorkspaceID:   "ws-alpha",
			WorkspacePath: "/tmp/alpha",
			LocalName:     "Alpha",
			Enabled:       true,
			IntervalSec:   300,
		},
	}, openBrainCloudSyncModelSelection{ModelKey: "test:model"})
	right := left
	left.CreatedAtMs = 100
	left.UpdatedAtMs = 200
	right.CreatedAtMs = 100
	right.UpdatedAtMs = 900

	if !sameCronTaskDefinition(left, right) {
		t.Fatal("sameCronTaskDefinition() = false, want true when only UpdatedAtMs changes")
	}
	right.Schedule.Every = "15m"
	if sameCronTaskDefinition(left, right) {
		t.Fatal("sameCronTaskDefinition() = true, want false when schedule changes")
	}
}

func TestIsLegacyOpenBrainSyncTask(t *testing.T) {
	legacyByAgent := protocol.CronTaskRecord{Task: protocol.CronTask{
		ID:     "custom-sync",
		Target: protocol.CronTaskTarget{AgentID: "agent-openbrain-sync"},
	}}
	legacyByID := protocol.CronTaskRecord{Task: protocol.CronTask{
		ID: "task-openbrain-sync-ws-alpha",
	}}
	legacyByManagedKind := protocol.CronTaskRecord{Task: protocol.CronTask{
		ID: "custom",
		Payload: protocol.CronTaskPayload{Data: map[string]interface{}{
			"managedKind": "openbrain-sync",
		}},
	}}
	current := protocol.CronTaskRecord{Task: openBrainCloudSyncTask("", nil, openBrainCloudSyncModelSelection{ModelKey: "test:model"})}

	if !isLegacyOpenBrainSyncTask(legacyByAgent) {
		t.Fatal("legacy agent task was not detected")
	}
	if !isLegacyOpenBrainSyncTask(legacyByID) {
		t.Fatal("legacy id task was not detected")
	}
	if !isLegacyOpenBrainSyncTask(legacyByManagedKind) {
		t.Fatal("legacy managedKind task was not detected")
	}
	if isLegacyOpenBrainSyncTask(current) {
		t.Fatal("current OpenBrain Cloud Sync task detected as legacy")
	}
}

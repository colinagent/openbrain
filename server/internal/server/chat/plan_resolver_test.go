package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func writeJSONLFixture(t *testing.T, filePath string, lines ...any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	defer f.Close()
	for _, line := range lines {
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
}

func canonicalEntry(id string, msg ai.ConversationMessage) op.ThreadCanonicalMessageEntry {
	return op.ThreadCanonicalMessageEntry{
		ThreadEntryBase: op.ThreadEntryBase{Type: op.ThreadEntryTypeCanonicalMessage, ID: id},
		Message:         msg,
	}
}

func writePlanFile(t *testing.T, filePath string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func newThreadMeta(threadFilePath, cwd string) *op.ThreadMeta {
	return &op.ThreadMeta{
		ThreadID:       "thread-test",
		ThreadFilePath: threadFilePath,
		CWD:            cwd,
		ChatPath:       filepath.Join(cwd, ".agent", "chat", "thread.md"),
		PlanPath:       "/stale/path.md",
	}
}

func TestResolveLatestPlanPath_ReturnsLatestValidPlan(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace", "demo")
	threadPath := filepath.Join(t.TempDir(), "threads", "thread-test.jsonl")
	firstPlan := filepath.Join(cwd, ".agent", "context", "first.md")
	secondPlan := filepath.Join(cwd, ".agent", "context", "second.md")
	writePlanFile(t, firstPlan, "# First\n\n## Tasks\n- [ ] One\n")
	writePlanFile(t, secondPlan, "# Second\n\n## Tasks\n- [ ] Two\n")

	writeJSONLFixture(t, threadPath,
		op.ThreadHeader{Type: "thread", ID: "thread-test", CWD: cwd, ChatPath: filepath.Join(cwd, ".agent", "chat", "thread.md")},
		canonicalEntry("1", ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolCall,
				ToolCall: &ai.CanonicalToolCall{
					ID:        "call-1",
					Name:      "write",
					Arguments: map[string]any{"path": ".agent/context/first.md", "content": "# First"},
				},
			}},
		}),
		canonicalEntry("2", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-1",
					ToolName:   "write",
					OutputText: "ok",
				},
			}},
		}),
		canonicalEntry("3", ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolCall,
				ToolCall: &ai.CanonicalToolCall{
					ID:        "call-2",
					Name:      "edit",
					Arguments: map[string]any{"path": ".agent/context/second.md", "oldText": "x", "newText": "y"},
				},
			}},
		}),
		canonicalEntry("4", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-2",
					ToolName:   "edit",
					OutputText: "updated",
				},
			}},
		}),
	)

	got, err := resolveLatestPlanPath(newThreadMeta(threadPath, cwd))
	if err != nil {
		t.Fatalf("resolveLatestPlanPath: %v", err)
	}
	if got != secondPlan {
		t.Fatalf("resolveLatestPlanPath = %q, want %q", got, secondPlan)
	}
}

func TestResolveLatestPlanPath_FallsBackWhenLatestPlanInvalid(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace", "demo")
	threadPath := filepath.Join(t.TempDir(), "threads", "thread-test.jsonl")
	firstPlan := filepath.Join(cwd, ".agent", "context", "first.md")
	secondPlan := filepath.Join(cwd, ".agent", "context", "second.md")
	writePlanFile(t, firstPlan, "# First\n\n## Tasks\n- [ ] One\n")
	writePlanFile(t, secondPlan, "# Second\n\nOnly prose\n")

	writeJSONLFixture(t, threadPath,
		op.ThreadHeader{Type: "thread", ID: "thread-test", CWD: cwd, ChatPath: filepath.Join(cwd, ".agent", "chat", "thread.md")},
		canonicalEntry("1", ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolCall,
				ToolCall: &ai.CanonicalToolCall{
					ID:        "call-1",
					Name:      "write",
					Arguments: map[string]any{"path": ".agent/context/first.md", "content": "# First"},
				},
			}},
		}),
		canonicalEntry("2", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-1",
					ToolName:   "write",
					OutputText: "ok",
				},
			}},
		}),
		canonicalEntry("3", ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolCall,
				ToolCall: &ai.CanonicalToolCall{
					ID:        "call-2",
					Name:      "write",
					Arguments: map[string]any{"path": ".agent/context/second.md", "content": "# Second"},
				},
			}},
		}),
		canonicalEntry("4", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-2",
					ToolName:   "write",
					OutputText: "ok",
				},
			}},
		}),
	)

	got, err := resolveLatestPlanPath(newThreadMeta(threadPath, cwd))
	if err != nil {
		t.Fatalf("resolveLatestPlanPath: %v", err)
	}
	if got != firstPlan {
		t.Fatalf("resolveLatestPlanPath = %q, want %q", got, firstPlan)
	}
}

func TestResolveLatestPlanPath_IgnoresOutsideContextDirAndToolErrors(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace", "demo")
	threadPath := filepath.Join(t.TempDir(), "threads", "thread-test.jsonl")
	outsidePlan := filepath.Join(cwd, "notes.md")
	legacyPlan := filepath.Join(cwd, ".agent", "plan", "legacy.md")
	writePlanFile(t, outsidePlan, "# Notes\n")
	writePlanFile(t, legacyPlan, "# Legacy\n\n## Tasks\n- [ ] Old path\n")

	writeJSONLFixture(t, threadPath,
		op.ThreadHeader{Type: "thread", ID: "thread-test", CWD: cwd, ChatPath: filepath.Join(cwd, ".agent", "chat", "thread.md")},
		canonicalEntry("1", ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{
				{
					Type: ai.BlockToolCall,
					ToolCall: &ai.CanonicalToolCall{
						ID:        "call-1",
						Name:      "write",
						Arguments: map[string]any{"path": "notes.md", "content": "# Notes"},
					},
				},
				{
					Type: ai.BlockToolCall,
					ToolCall: &ai.CanonicalToolCall{
						ID:        "call-2",
						Name:      "write",
						Arguments: map[string]any{"path": ".agent/context/missing.md", "content": "# Missing"},
					},
				},
				{
					Type: ai.BlockToolCall,
					ToolCall: &ai.CanonicalToolCall{
						ID:        "call-3",
						Name:      "write",
						Arguments: map[string]any{"path": ".agent/plan/legacy.md", "content": "# Legacy"},
					},
				},
			},
		}),
		canonicalEntry("2", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-1",
					ToolName:   "write",
					OutputText: "ok",
				},
			}},
		}),
		canonicalEntry("3", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-2",
					ToolName:   "write",
					OutputText: "tool execution failed",
				},
			}},
		}),
		canonicalEntry("4", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-3",
					ToolName:   "write",
					OutputText: "ok",
				},
			}},
		}),
	)

	got, err := resolveLatestPlanPath(newThreadMeta(threadPath, cwd))
	if err != nil {
		t.Fatalf("resolveLatestPlanPath: %v", err)
	}
	if got != "" {
		t.Fatalf("resolveLatestPlanPath = %q, want empty", got)
	}
}

func TestResolveLatestPlanPath_UsesAbsolutePlanPath(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace", "demo")
	threadPath := filepath.Join(t.TempDir(), "threads", "thread-test.jsonl")
	planPath := filepath.Join(cwd, ".agent", "context", "absolute.md")
	writePlanFile(t, planPath, "# Absolute\n\n## 任务\n- [ ] One\n")

	writeJSONLFixture(t, threadPath,
		op.ThreadHeader{Type: "thread", ID: "thread-test", CWD: cwd, ChatPath: filepath.Join(cwd, ".agent", "chat", "thread.md")},
		canonicalEntry("1", ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolCall,
				ToolCall: &ai.CanonicalToolCall{
					ID:        "call-1",
					Name:      "edit",
					Arguments: map[string]any{"path": planPath, "oldText": "x", "newText": "y"},
				},
			}},
		}),
		canonicalEntry("2", ai.ConversationMessage{
			Role: ai.RoleCanonicalTool,
			Content: []ai.ContentBlock{{
				Type: ai.BlockToolResult,
				ToolResult: &ai.CanonicalToolResult{
					ToolCallID: "call-1",
					ToolName:   "edit",
					OutputText: "ok",
				},
			}},
		}),
	)

	got, err := resolveLatestPlanPath(newThreadMeta(threadPath, cwd))
	if err != nil {
		t.Fatalf("resolveLatestPlanPath: %v", err)
	}
	if got != planPath {
		t.Fatalf("resolveLatestPlanPath = %q, want %q", got, planPath)
	}
}

func TestWithDynamicPlanPath_ClearsStalePlanPathWhenNoPlanFound(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace", "demo")
	threadPath := filepath.Join(t.TempDir(), "threads", "thread-test.jsonl")
	writeJSONLFixture(t, threadPath,
		op.ThreadHeader{Type: "thread", ID: "thread-test", CWD: cwd, ChatPath: filepath.Join(cwd, ".agent", "chat", "thread.md")},
	)

	service := &Service{}
	meta := service.withDynamicPlanPath(newThreadMeta(threadPath, cwd))
	if meta == nil {
		t.Fatal("withDynamicPlanPath returned nil")
	}
	if meta.PlanPath != "" {
		t.Fatalf("meta.PlanPath = %q, want empty", meta.PlanPath)
	}
}

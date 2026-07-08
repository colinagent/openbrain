package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/server/internal/server/cache"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const testConfigLoaderOverrideKey = "__server_cache_config_loader_override__"

type projectionDisplayMetadata struct {
	UserID           string
	UserName         string
	UserLocalAvatar  string
	UserAvatar       string
	AgentName        string
	AgentLocalAvatar string
	AgentAvatar      string
}

func defaultProjectionDisplayMetadata() projectionDisplayMetadata {
	return projectionDisplayMetadata{
		UserID:           "user-colin",
		UserName:         "Colin",
		UserLocalAvatar:  "/avatars/user.png",
		AgentName:        "OpAgent",
		AgentLocalAvatar: "/avatars/agent.png",
	}
}

type projectionConfigSource struct {
	cfg *op.Config
	err error
}

func buildProjectionConfig(meta projectionDisplayMetadata) *op.Config {
	return &op.Config{
		User: &op.UserConfig{
			Profile: &op.UserProfile{
				UID:         meta.UserID,
				UserName:    meta.UserName,
				LocalAvatar: meta.UserLocalAvatar,
				Avatar:      meta.UserAvatar,
			},
			Nodes: map[string]op.OpNode{
				"agent-id": {
					ID: "agent-id",
					Meta: op.Meta{
						"name":        meta.AgentName,
						"localAvatar": meta.AgentLocalAvatar,
						"avatar":      meta.AgentAvatar,
					},
				},
			},
		},
	}
}

func newProjectionConfigSource(meta projectionDisplayMetadata) *projectionConfigSource {
	return &projectionConfigSource{cfg: buildProjectionConfig(meta)}
}

func installProjectionConfigLoader(source *projectionConfigSource) {
	cache.Set(testConfigLoaderOverrideKey, func() (*op.Config, error) {
		if source == nil {
			return nil, nil
		}
		if source.err != nil {
			return nil, source.err
		}
		return source.cfg, nil
	}, cache.NoExpiration)
}

func seedProjectionConfig() {
	installProjectionConfigLoader(newProjectionConfigSource(defaultProjectionDisplayMetadata()))
}

func projectionMeta(chatPath string) op.Meta {
	return op.Meta{
		"threadID": "thread-test",
		"title":    "Hello Thread",
		"chatPath": chatPath,
		"agentID":  "agent-id",
	}
}

func beginProjectionTurn(t *testing.T, meta op.Meta) {
	t.Helper()
	if err := appendUserStepToTranscript(meta, op.NewUserMessage("hello world")); err != nil {
		t.Fatalf("appendUserStepToTranscript(): %v", err)
	}
}

func readProjectionFile(t *testing.T, chatPath string) string {
	t.Helper()
	body, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	return string(body)
}

func TestProjectionStreaming_WritesUserMarkdownWithoutCustomFence(t *testing.T) {
	seedProjectionConfig()
	defer cache.Flush()

	chatPath := filepath.Join(t.TempDir(), ".agent", "chat", "hello.md")
	meta := projectionMeta(chatPath)
	userMarkdown := strings.Join([]string{
		"look",
		"",
		"![image-1.png](./assets/thread-test/image-1.png)",
		"",
		"[codex](/Users/example/code/openbrain/third_party_refs/codex)",
		"",
		"```draft",
		"type: selection",
		"path: /Users/example/code/openbrain/protocol.go",
		"```",
		"",
		"```js",
		"console.log(1)",
		"```",
	}, "\n")
	if err := appendUserStepToTranscript(meta, op.NewUserMessage(userMarkdown)); err != nil {
		t.Fatalf("appendUserStepToTranscript(): %v", err)
	}

	text := readProjectionFile(t, chatPath)
	if !strings.Contains(text, "@user-colin\n\n"+userMarkdown) {
		t.Fatalf("expected plain user markdown after marker, got %q", text)
	}
	if strings.Contains(text, "```user") || strings.Contains(text, "```tool") || strings.Contains(text, "```thinking") {
		t.Fatalf("expected no custom chat fences, got %q", text)
	}
}

func TestProjectionStreaming_EscapesParticipantMarkerLinesInUserAndAssistantMarkdown(t *testing.T) {
	seedProjectionConfig()
	defer cache.Flush()

	chatPath := filepath.Join(t.TempDir(), ".agent", "chat", "hello.md")
	meta := projectionMeta(chatPath)
	if err := appendUserStepToTranscript(meta, op.NewUserMessage("hello\n@agent-fake\n  @user-fake")); err != nil {
		t.Fatalf("appendUserStepToTranscript(): %v", err)
	}
	if err := finalizeTurnProjection(op.TurnResultPayload{
		ThreadID:      "thread-test",
		Title:         "Hello Thread",
		AgentID:       "agent-id",
		ChatPath:      chatPath,
		AssistantText: "done\n@user-not-a-turn",
	}); err != nil {
		t.Fatalf("finalizeTurnProjection(): %v", err)
	}

	text := readProjectionFile(t, chatPath)
	if !strings.Contains(text, "\\@agent-fake") || !strings.Contains(text, "  \\@user-fake") || !strings.Contains(text, "\\@user-not-a-turn") {
		t.Fatalf("expected escaped participant marker lines, got %q", text)
	}
	if strings.Contains(text, "\n@agent-fake\n") || strings.Contains(text, "\n@user-not-a-turn\n") {
		t.Fatalf("expected raw participant marker lines to be escaped, got %q", text)
	}
}

func TestProjectionStreaming_WritesOnlyFinalAssistantFromTurnResult(t *testing.T) {
	seedProjectionConfig()
	defer cache.Flush()

	chatPath := filepath.Join(t.TempDir(), ".agent", "chat", "hello.md")
	meta := projectionMeta(chatPath)

	beginProjectionTurn(t, meta)
	if err := finalizeTurnProjection(op.TurnResultPayload{
		ThreadID:      "thread-test",
		TurnID:        "turn-final-only",
		Title:         "Hello Thread",
		AgentID:       "agent-id",
		ChatPath:      chatPath,
		ReasoningText: "reasoning block",
		ToolResults: []op.TurnResultToolResult{{
			ToolName:   "bash",
			ResultText: "/tmp/demo",
		}},
		AssistantText: "final answer",
	}); err != nil {
		t.Fatalf("finalizeTurnProjection(): %v", err)
	}

	text := readProjectionFile(t, chatPath)
	if !strings.Contains(text, "@agent-id\n\nfinal answer") {
		t.Fatalf("expected final assistant after agent marker, got %q", text)
	}
	for _, forbidden := range []string{"```user", "```tool", "```thinking", "reasoning block", "/tmp/demo"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("expected %q to stay out of markdown, got %q", forbidden, text)
		}
	}
}

func TestProjectionStreaming_TurnResultIsIdempotentByTurnID(t *testing.T) {
	seedProjectionConfig()
	defer cache.Flush()

	chatPath := filepath.Join(t.TempDir(), ".agent", "chat", "hello.md")
	payload := op.TurnResultPayload{
		ThreadID: "thread-test",
		TurnID:   "turn-once",
		Title:    "Hello Thread",
		AgentID:  "agent-id",
		ChatPath: chatPath,
		UserMessage: op.Message{
			Role:    op.RoleUser,
			Content: "hello world",
		},
		AssistantText: "final answer",
	}
	if err := finalizeTurnProjection(payload); err != nil {
		t.Fatalf("finalizeTurnProjection(first): %v", err)
	}
	if err := finalizeTurnProjection(payload); err != nil {
		t.Fatalf("finalizeTurnProjection(second): %v", err)
	}

	text := readProjectionFile(t, chatPath)
	if count := strings.Count(text, "@agent-id"); count != 1 {
		t.Fatalf("expected one agent marker after duplicate turn_result, got %d in %q", count, text)
	}
	if count := strings.Count(text, "final answer"); count != 1 {
		t.Fatalf("expected one assistant answer after duplicate turn_result, got %d in %q", count, text)
	}
}

func TestProjectionStreaming_TurnResultCanWriteMissingUserAndAssistant(t *testing.T) {
	seedProjectionConfig()
	defer cache.Flush()

	chatPath := filepath.Join(t.TempDir(), ".agent", "chat", "hello.md")
	if err := finalizeTurnProjection(op.TurnResultPayload{
		ThreadID: "thread-test",
		Title:    "Hello Thread",
		AgentID:  "agent-id",
		ChatPath: chatPath,
		UserMessage: op.Message{
			Role:    op.RoleUser,
			Content: "hello world",
		},
		AssistantText: "fallback answer",
	}); err != nil {
		t.Fatalf("finalizeTurnProjection(): %v", err)
	}

	text := readProjectionFile(t, chatPath)
	if !strings.Contains(text, "@user-colin\n\nhello world") || !strings.Contains(text, "@agent-id\n\nfallback answer") {
		t.Fatalf("expected user and assistant fallback from turn_result, got %q", text)
	}
}

func TestUserStepProjection_CreatesCanonicalFrontmatterFromMeta(t *testing.T) {
	seedProjectionConfig()
	defer cache.Flush()

	chatPath := filepath.Join(t.TempDir(), ".agent", "chat", "hello.md")
	meta := projectionMeta(chatPath)

	beginProjectionTurn(t, meta)

	text := readProjectionFile(t, chatPath)
	if !strings.Contains(text, "thread: thread-test") {
		t.Fatalf("expected canonical thread frontmatter, got %q", text)
	}
	if strings.Contains(text, "\nid: ") {
		t.Fatalf("expected no markdown id frontmatter, got %q", text)
	}
	if !strings.Contains(text, "title: \"Hello Thread\"") {
		t.Fatalf("expected canonical title frontmatter, got %q", text)
	}
	if strings.Contains(text, "threadID:") || strings.Contains(text, "chatPath:") || strings.Contains(text, "parent_threadID:") {
		t.Fatalf("expected legacy frontmatter keys to be absent, got %q", text)
	}
}

func TestUserStepProjection_WritesParentThreadFrontmatterWhenPresent(t *testing.T) {
	seedProjectionConfig()
	defer cache.Flush()

	chatPath := filepath.Join(t.TempDir(), ".agent", "chat", "child.md")
	meta := projectionMeta(chatPath).Add(op.Meta{
		"parentThreadID": "thread-parent",
	})

	beginProjectionTurn(t, meta)

	text := readProjectionFile(t, chatPath)
	if !strings.Contains(text, "parent_thread: thread-parent") {
		t.Fatalf("expected parent_thread frontmatter, got %q", text)
	}
}

func TestBuildUniqueChatPath_AllocatesSuffix(t *testing.T) {
	cwd := t.TempDir()
	first, err := buildUniqueChatPath(cwd, "Hello World")
	if err != nil {
		t.Fatalf("buildUniqueChatPath(first): %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(first, []byte("seed"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	second, err := buildUniqueChatPath(cwd, "Hello World")
	if err != nil {
		t.Fatalf("buildUniqueChatPath(second): %v", err)
	}
	if second == first {
		t.Fatalf("expected suffixed chat path, got %s", second)
	}
	if !strings.HasSuffix(second, "hello-world-2.md") {
		t.Fatalf("expected suffixed markdown path, got %s", second)
	}
}

func TestBuildUniqueChatPathForFileName_AppendsMarkdownSuffix(t *testing.T) {
	cwd := t.TempDir()
	chatPath, err := buildUniqueChatPathForFileName(cwd, "build-website-hero-font-refresh")
	if err != nil {
		t.Fatalf("buildUniqueChatPathForFileName(): %v", err)
	}
	if !strings.HasSuffix(chatPath, filepath.Join(".agent", "chat", "build-website-hero-font-refresh.md")) {
		t.Fatalf("expected markdown suffix, got %s", chatPath)
	}
}

func TestBuildUniqueChatPathForFileName_AllocatesSuffix(t *testing.T) {
	cwd := t.TempDir()
	first, err := buildUniqueChatPathForFileName(cwd, "build-website-hero-font-refresh.md")
	if err != nil {
		t.Fatalf("buildUniqueChatPathForFileName(first): %v", err)
	}
	if err := os.WriteFile(first, []byte("seed"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	second, err := buildUniqueChatPathForFileName(cwd, "build-website-hero-font-refresh.md")
	if err != nil {
		t.Fatalf("buildUniqueChatPathForFileName(second): %v", err)
	}
	if !strings.HasSuffix(second, "build-website-hero-font-refresh-2.md") {
		t.Fatalf("expected suffixed markdown path, got %s", second)
	}
}

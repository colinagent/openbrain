package archive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/klauspost/compress/zip"
)

type stubCoreClient struct {
	baseDir                 string
	missingDefaultWorkspace bool
	active                  []op.ThreadRuntimeInfo
	updates                 []op.ThreadMetaUpdateParams
}

func (s *stubCoreClient) GetSystemConfig(context.Context) (*op.SystemConfigResult, error) {
	defaultWorkspace := filepath.Join(s.baseDir, "workspace")
	if s.missingDefaultWorkspace {
		defaultWorkspace = ""
	}
	return &op.SystemConfigResult{
		SystemConfig:     op.SystemConfig{BaseDir: s.baseDir},
		DefaultWorkspace: defaultWorkspace,
	}, nil
}

func (s *stubCoreClient) ListActiveThreads(context.Context) ([]op.ThreadRuntimeInfo, error) {
	return append([]op.ThreadRuntimeInfo(nil), s.active...), nil
}

func (s *stubCoreClient) UpdateThreadMeta(_ context.Context, params op.ThreadMetaUpdateParams) (*op.ThreadMeta, error) {
	s.updates = append(s.updates, params)
	return &op.ThreadMeta{
		ThreadID:          params.ThreadID,
		ChatPath:          params.ChatPath,
		PlanPath:          params.PlanPath,
		ExecutionPlanPath: params.ExecutionPlanPath,
		Title:             params.Title,
	}, nil
}

func writeChatFile(t *testing.T, path, threadID, title, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := strings.Join([]string{
		"---",
		"thread: " + threadID,
		"title: \"" + title + "\"",
		"---",
		"",
		body,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRunRequiresRuntimeDefaultWorkspace(t *testing.T) {
	service := NewService(&stubCoreClient{
		baseDir:                 t.TempDir(),
		missingDefaultWorkspace: true,
	})
	_, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{})
	if err == nil || !strings.Contains(err.Error(), "defaultWorkspace") {
		t.Fatalf("Run() error = %v, want missing defaultWorkspace", err)
	}
}

func writeThreadFile(t *testing.T, path string, header op.ThreadHeader) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	raw, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal thread header: %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeReviewFile(t *testing.T, sessionPath string) {
	t.Helper()
	reviewPath := strings.TrimSuffix(sessionPath, ".jsonl") + ".review/turns/turn-1/manifest.json"
	if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
		t.Fatalf("mkdir review: %v", err)
	}
	if err := os.WriteFile(reviewPath, []byte(`{"turnID":"turn-1"}`), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
}

func writeZipFile(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir zip dir: %v", err)
	}
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("os.Create(%s): %v", archivePath, err)
	}
	writer := zip.NewWriter(file)
	for name, body := range entries {
		entryWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("writer.Create(%s): %v", name, err)
		}
		if _, err := entryWriter.Write([]byte(body)); err != nil {
			t.Fatalf("entryWriter.Write(%s): %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close(): %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file.Close(): %v", err)
	}
}

func mustSetModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestRunMovesOldChatsAndUpdatesLiveSessions(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.Local)

	for i := 0; i < 12; i++ {
		threadID := "thread-chat-" + strconvItoa(i)
		chatPath := filepath.Join(chatRoot, "chat-"+strconvItoa(i)+".md")
		writeChatFile(t, chatPath, threadID, "Chat "+strconvItoa(i), "body")
		writeThreadFile(t, filepath.Join(threadRoot, threadID+".jsonl"), op.ThreadHeader{
			Type:      "thread",
			Version:   1,
			ID:        threadID,
			Timestamp: now.UTC().Format(time.RFC3339Nano),
			ChatPath:  chatPath,
			Title:     "Chat " + strconvItoa(i),
		})
		mustSetModTime(t, chatPath, now.Add(-time.Duration(i+20)*time.Minute))
	}

	assetPath := filepath.Join(chatRoot, "assets", "thread-chat-10", "image.png")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if result.MovedChats != 2 {
		t.Fatalf("MovedChats = %d, want 2", result.MovedChats)
	}
	historyDir := filepath.Join(chatRoot, "history", "2026-04-04")
	if _, err := os.Stat(filepath.Join(historyDir, "chat-10.md")); err != nil {
		t.Fatalf("expected archived chat-10.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(historyDir, "assets", "thread-chat-10", "image.png")); err != nil {
		t.Fatalf("expected archived assets: %v", err)
	}
	if len(core.updates) != 2 {
		t.Fatalf("live thread updates = %d, want 2", len(core.updates))
	}
}

func TestRunMigratesLegacyChatZipAndRewritesSessionPath(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local)

	oldHistoryPath := filepath.Join(chatRoot, "history", "2026-03-29", "legacy.md")
	writeZipFile(t, filepath.Join(chatRoot, "history", "2026-03.zip"), map[string]string{
		"2026-03-29/legacy.md": strings.Join([]string{
			"---",
			"thread: thread-legacy",
			"title: \"Legacy\"",
			"---",
			"",
			"legacy body",
			"",
		}, "\n"),
	})
	writeThreadFile(t, filepath.Join(threadRoot, "thread-legacy.jsonl"), op.ThreadHeader{
		Type:      "thread",
		Version:   1,
		ID:        "thread-legacy",
		Timestamp: now.UTC().Format(time.RFC3339Nano),
		ChatPath:  oldHistoryPath,
		Title:     "Legacy",
	})

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	newPath := filepath.Join(chatRoot, "history", "2026-03", "2026-03-29", "legacy.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected migrated legacy chat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chatRoot, "history", "2026-03.zip")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy zip removed, got err=%v", err)
	}
	if len(core.updates) != 1 || core.updates[0].ChatPath != newPath {
		t.Fatalf("thread updates = %+v, want migrated chat path %s", core.updates, newPath)
	}
	if result.RewrittenThreads != 1 {
		t.Fatalf("RewrittenThreads = %d, want 1", result.RewrittenThreads)
	}
}

func TestRunMovesPlanAndRewritesChatAndSessions(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	planRoot := filepath.Join(workspaceRoot, ".agent", "context")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.Local)

	var movedPlanPath string
	for i := 0; i < 11; i++ {
		planPath := filepath.Join(planRoot, "plan-"+strconvItoa(i)+".md")
		if i == 10 {
			movedPlanPath = planPath
		}
		if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
			t.Fatalf("mkdir plan dir: %v", err)
		}
		if err := os.WriteFile(planPath, []byte("# plan\n"), 0o644); err != nil {
			t.Fatalf("write plan: %v", err)
		}
		mustSetModTime(t, planPath, now.Add(-time.Duration(i+20)*time.Minute))
	}

	chatPath := filepath.Join(chatRoot, "ref.md")
	writeChatFile(t, chatPath, "thread-live", "Ref", "planFilePath: "+movedPlanPath)

	writeThreadFile(t, filepath.Join(threadRoot, "thread-live.jsonl"), op.ThreadHeader{
		Type:      "thread",
		Version:   1,
		ID:        "thread-live",
		Timestamp: now.UTC().Format(time.RFC3339Nano),
		ChatPath:  chatPath,
		PlanPath:  movedPlanPath,
		Title:     "Ref",
	})

	flatSessionPath := filepath.Join(threadRoot, "thread-history.jsonl")
	writeThreadFile(t, flatSessionPath, op.ThreadHeader{
		Type:      "thread",
		Version:   1,
		ID:        "thread-history",
		Timestamp: now.UTC().Format(time.RFC3339Nano),
		ChatPath:  filepath.Join(chatRoot, "old.md"),
		PlanPath:  movedPlanPath,
		Title:     "Old",
	})

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if result.MovedPlans != 1 {
		t.Fatalf("MovedPlans = %d, want 1", result.MovedPlans)
	}
	if len(core.updates) != 2 {
		t.Fatalf("thread updates = %d, want 2", len(core.updates))
	}
	newPlanPath := filepath.Join(planRoot, "history", "2026-04-04", "plan-10.md")
	body, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatalf("read chat: %v", err)
	}
	if !strings.Contains(string(body), newPlanPath) {
		t.Fatalf("chat markdown missing rewritten plan path: %q", string(body))
	}
	updatedPlans := map[string]bool{}
	for _, update := range core.updates {
		updatedPlans[update.PlanPath] = true
	}
	if !updatedPlans[newPlanPath] {
		t.Fatalf("thread updates = %+v, want plan path %s", core.updates, newPlanPath)
	}
}

func TestRunLeavesFlatThreadAndReviewDirInPlace(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.Local)

	for i := 0; i < 11; i++ {
		threadID := "thread-thread-" + strconvItoa(i)
		sessionPath := filepath.Join(threadRoot, threadID+".jsonl")
		writeThreadFile(t, sessionPath, op.ThreadHeader{
			Type:      "thread",
			Version:   1,
			ID:        threadID,
			Timestamp: now.UTC().Format(time.RFC3339Nano),
			ChatPath:  filepath.Join(workspaceRoot, ".agent", "chat", threadID+".md"),
			Title:     threadID,
		})
		mustSetModTime(t, sessionPath, now.Add(-time.Duration(i+20)*time.Minute))
		if i == 10 {
			writeReviewFile(t, sessionPath)
		}
	}

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if result.MovedThreads != 0 {
		t.Fatalf("MovedThreads = %d, want 0", result.MovedThreads)
	}
	flatPath := filepath.Join(threadRoot, "thread-thread-10.jsonl")
	if _, err := os.Stat(flatPath); err != nil {
		t.Fatalf("expected flat thread file retained: %v", err)
	}
	if _, err := os.Stat(strings.TrimSuffix(flatPath, ".jsonl") + ".review/turns/turn-1/manifest.json"); err != nil {
		t.Fatalf("expected flat review dir retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(threadRoot, "history")); !os.IsNotExist(err) {
		t.Fatalf("expected no thread history dir, got err=%v", err)
	}
}

func TestRunCopiesFlatAssetsAlongsideArchivedChat(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.Local)

	for i := 0; i < 11; i++ {
		threadID := "thread-flat-" + strconvItoa(i)
		chatPath := filepath.Join(chatRoot, "flat-"+strconvItoa(i)+".md")
		body := "body"
		if i == 10 {
			body = "![image](./assets/image-flat.png)"
		}
		writeChatFile(t, chatPath, threadID, "Flat "+strconvItoa(i), body)
		writeThreadFile(t, filepath.Join(threadRoot, threadID+".jsonl"), op.ThreadHeader{
			Type:      "thread",
			Version:   1,
			ID:        threadID,
			Timestamp: now.UTC().Format(time.RFC3339Nano),
			ChatPath:  chatPath,
			Title:     "Flat " + strconvItoa(i),
		})
		mustSetModTime(t, chatPath, now.Add(-time.Duration(i+20)*time.Minute))
	}
	flatAssetPath := filepath.Join(chatRoot, "assets", "image-flat.png")
	if err := os.MkdirAll(filepath.Dir(flatAssetPath), 0o755); err != nil {
		t.Fatalf("mkdir flat assets: %v", err)
	}
	if err := os.WriteFile(flatAssetPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write flat asset: %v", err)
	}

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	if _, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	}); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	archivedAsset := filepath.Join(chatRoot, "history", "2026-04-04", "assets", "image-flat.png")
	if _, err := os.Stat(archivedAsset); err != nil {
		t.Fatalf("expected copied flat asset: %v", err)
	}
	if _, err := os.Stat(flatAssetPath); err != nil {
		t.Fatalf("expected original flat asset kept: %v", err)
	}
}

func TestRunRollsDailyHistoryIntoMonthlyDirAndKeepsOldMonths(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local)

	dayChatPath := filepath.Join(chatRoot, "history", "2026-03-20", "archived.md")
	writeChatFile(t, dayChatPath, "thread-archived", "Archived", "body")
	mustSetModTime(t, dayChatPath, time.Date(2026, 3, 20, 8, 0, 0, 0, time.Local))
	writeThreadFile(t, filepath.Join(threadRoot, "thread-live.jsonl"), op.ThreadHeader{
		Type:      "thread",
		Version:   1,
		ID:        "thread-live",
		Timestamp: now.UTC().Format(time.RFC3339Nano),
		ChatPath:  dayChatPath,
		Title:     "Archived",
	})

	for _, month := range []string{"2025-11", "2025-12", "2026-01", "2026-02"} {
		dir := filepath.Join(chatRoot, "history", month, month+"-01")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir month dir: %v", err)
		}
	}

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	monthPath := filepath.Join(chatRoot, "history", "2026-03", "2026-03-20", "archived.md")
	if _, err := os.Stat(monthPath); err != nil {
		t.Fatalf("expected monthly archived chat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chatRoot, "history", "2025-11")); err != nil {
		t.Fatalf("expected old month retained, got err=%v", err)
	}
	if result.RolledIntoMonthlyDirs != 1 {
		t.Fatalf("RolledIntoMonthlyDirs = %d, want 1", result.RolledIntoMonthlyDirs)
	}
	if result.PrunedArchives != 0 {
		t.Fatalf("PrunedArchives = %d, want 0", result.PrunedArchives)
	}
}

func TestRunCompressesChatMonthsOlderThanOneYearIntoYearZip(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local)

	oldChatPath := filepath.Join(chatRoot, "history", "2025-03", "2025-03-20", "old.md")
	writeChatFile(t, oldChatPath, "thread-old", "Old", "old body")
	olderChatPath := filepath.Join(chatRoot, "history", "2025-02", "2025-02-14", "older.md")
	writeChatFile(t, olderChatPath, "thread-older", "Older", "older body")
	keptChatPath := filepath.Join(chatRoot, "history", "2025-04", "2025-04-01", "kept.md")
	writeChatFile(t, keptChatPath, "thread-kept", "Kept", "kept body")

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if result.CompressedArchives != 2 {
		t.Fatalf("CompressedArchives = %d, want 2", result.CompressedArchives)
	}
	if _, err := os.Stat(filepath.Join(chatRoot, "history", "2025-03")); !os.IsNotExist(err) {
		t.Fatalf("expected 2025-03 month dir removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(chatRoot, "history", "2025-02")); !os.IsNotExist(err) {
		t.Fatalf("expected 2025-02 month dir removed, got err=%v", err)
	}
	if _, err := os.Stat(keptChatPath); err != nil {
		t.Fatalf("expected 2025-04 month dir retained: %v", err)
	}

	archivePath := filepath.Join(chatRoot, "history", "2025.zip")
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("open year zip: %v", err)
	}
	defer reader.Close()

	found := map[string]bool{
		"2025-03/2025-03-20/old.md":   false,
		"2025-02/2025-02-14/older.md": false,
	}
	for _, entry := range reader.File {
		if _, ok := found[entry.Name]; ok {
			found[entry.Name] = true
		}
	}
	for name, ok := range found {
		if !ok {
			t.Fatalf("year zip missing archived chat entry %s", name)
		}
	}
}

func TestRunSkipsChatMonthCompressionWhenCurrentSessionReferencesIt(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local)

	oldChatPath := filepath.Join(chatRoot, "history", "2025-03", "2025-03-20", "live.md")
	writeChatFile(t, oldChatPath, "thread-live-old", "Live Old", "old body")
	writeThreadFile(t, filepath.Join(threadRoot, "thread-live-old.jsonl"), op.ThreadHeader{
		Type:      "thread",
		Version:   1,
		ID:        "thread-live-old",
		Timestamp: now.UTC().Format(time.RFC3339Nano),
		ChatPath:  oldChatPath,
		Title:     "Live Old",
	})

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if result.CompressedArchives != 0 {
		t.Fatalf("CompressedArchives = %d, want 0", result.CompressedArchives)
	}
	if result.SkippedActiveThreads != 1 {
		t.Fatalf("SkippedActiveThreads = %d, want 1", result.SkippedActiveThreads)
	}
	if _, err := os.Stat(filepath.Join(chatRoot, "history", "2025-03")); err != nil {
		t.Fatalf("expected month dir retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chatRoot, "history", "2025.zip")); !os.IsNotExist(err) {
		t.Fatalf("expected no year zip, stat err=%v", err)
	}
}

func TestRunKeepsChatMonthDirWhenYearZipAlreadyHasEntry(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(workspaceRoot, ".agent", "chat")
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local)

	oldChatPath := filepath.Join(chatRoot, "history", "2025-03", "2025-03-20", "old.md")
	writeChatFile(t, oldChatPath, "thread-old", "Old", "old body")
	writeZipFile(t, filepath.Join(chatRoot, "history", "2025.zip"), map[string]string{
		"2025-03/2025-03-20/old.md": "existing body",
	})

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if result.CompressedArchives != 0 {
		t.Fatalf("CompressedArchives = %d, want 0", result.CompressedArchives)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected duplicate archive entry error")
	}
	if !strings.Contains(result.Errors[0], "archive entry already exists") {
		t.Fatalf("first error = %q, want duplicate archive entry error", result.Errors[0])
	}
	if _, err := os.Stat(filepath.Join(chatRoot, "history", "2025-03")); err != nil {
		t.Fatalf("expected month dir retained: %v", err)
	}
}

func TestRunLeavesThreadHistoryMonthsUntouched(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot := filepath.Join(baseDir, "workspace", "demo")
	threadRoot := filepath.Join(baseDir, "threads")
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local)

	if err := os.MkdirAll(threadRoot, 0o755); err != nil {
		t.Fatalf("mkdir thread root: %v", err)
	}
	for _, month := range []string{"2025-12", "2026-01", "2026-02", "2026-03"} {
		dir := filepath.Join(threadRoot, "history", month, month+"-01")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir month dir: %v", err)
		}
	}

	core := &stubCoreClient{baseDir: baseDir}
	service := NewService(core)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(threadRoot, "history", "2025-12")); err != nil {
		t.Fatalf("expected old thread month retained, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(threadRoot, "history", "2026-01")); err != nil {
		t.Fatalf("expected retained thread month: %v", err)
	}
	if result.PrunedArchives != 0 {
		t.Fatalf("PrunedArchives = %d, want 0", result.PrunedArchives)
	}
}

func strconvItoa(value int) string {
	return strconv.FormatInt(int64(value), 10)
}

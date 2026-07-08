package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func setupReviewTestSession(t *testing.T) (string, string, *op.ThreadMeta) {
	t.Helper()
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "workspace/proj")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "test.md"),
		Title:    "test",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	return cwd, result.ThreadID, meta
}

func commitReviewTestFile(t *testing.T, meta *op.ThreadMeta, cwd, relPath, baseline, final string) (string, string) {
	t.Helper()
	turnID := op.GenerateTurnID()
	return commitReviewTestFileForTurn(t, meta, turnID, cwd, relPath, baseline, final), turnID
}

func commitReviewTestFileForTurn(t *testing.T, meta *op.ThreadMeta, turnID, cwd, relPath, baseline, final string) string {
	t.Helper()
	filePath := filepath.Join(cwd, relPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir file dir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(baseline), 0o644); err != nil {
		t.Fatalf("write baseline file: %v", err)
	}
	prepared, err := prepareReviewMutation(*meta, turnID, cwd, relPath)
	if err != nil {
		t.Fatalf("prepareReviewMutation: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(final), 0o644); err != nil {
		t.Fatalf("write final file: %v", err)
	}
	if _, err := commitReviewMutation(*meta, prepared, final); err != nil {
		t.Fatalf("commitReviewMutation: %v", err)
	}
	return filePath
}

func TestReviewApproveAndRollbackFile(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "workspace/proj")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "test.md"),
		Title:    "test",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}

	filePath := filepath.Join(cwd, "main.go")
	if err := os.WriteFile(filePath, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write baseline file: %v", err)
	}

	turnID := op.GenerateTurnID()
	prepared, err := prepareReviewMutation(*meta, turnID, cwd, "main.go")
	if err != nil {
		t.Fatalf("prepareReviewMutation: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("write final file: %v", err)
	}
	review, err := commitReviewMutation(*meta, prepared, "after\n")
	if err != nil {
		t.Fatalf("commitReviewMutation: %v", err)
	}
	if review.Status != op.ThreadReviewTurnPending {
		t.Fatalf("review.Status = %q, want pending", review.Status)
	}
	if got := review.Files[0].FirstChangedLine; got != 1 {
		t.Fatalf("review.Files[0].FirstChangedLine = %d, want 1", got)
	}
	if got := len(review.Files[0].ChangedRanges); got != 1 {
		t.Fatalf("len(review.Files[0].ChangedRanges) = %d, want 1", got)
	}
	if got := review.Files[0].ChangedRanges[0]; got.StartLine != 1 || got.EndLine != 1 {
		t.Fatalf("review.Files[0].ChangedRanges[0] = %+v, want {1 1}", got)
	}
	if got := len(review.Files[0].Hunks); got != 1 {
		t.Fatalf("len(review.Files[0].Hunks) = %d, want 1", got)
	}
	if got := review.Files[0].Hunks[0]; got.OldStartLine != 1 || got.OldLineCount != 1 || got.NewStartLine != 1 || got.NewLineCount != 1 {
		t.Fatalf("review.Files[0].Hunks[0] = %+v, want one-line replacement", got)
	}

	review, err = resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: result.ThreadID,
		TurnID:   turnID,
		Path:     filePath,
		Decision: op.ThreadReviewDecisionApprove,
	})
	if err != nil {
		t.Fatalf("resolveThreadReview approve: %v", err)
	}
	if review.Status != op.ThreadReviewTurnResolved {
		t.Fatalf("review.Status = %q, want resolved", review.Status)
	}
	afterApprove, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after approve: %v", err)
	}
	if string(afterApprove) != "after\n" {
		t.Fatalf("file after approve = %q, want %q", string(afterApprove), "after\n")
	}

	review, err = rollbackThreadReview(op.ThreadReviewRollbackParams{
		ThreadID: result.ThreadID,
		TurnID:   turnID,
		Path:     filePath,
		Scope:    op.ThreadReviewRollbackFile,
	})
	if err != nil {
		t.Fatalf("rollbackThreadReview file: %v", err)
	}
	if review.Files[0].Status != op.ThreadReviewFileRolledBack {
		t.Fatalf("review.Files[0].Status = %q, want rolledBack", review.Files[0].Status)
	}
	afterRollback, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after rollback: %v", err)
	}
	if string(afterRollback) != "before\n" {
		t.Fatalf("file after rollback = %q, want %q", string(afterRollback), "before\n")
	}
}

func TestReviewRejectAllRemovesCreatedFile(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "workspace/proj")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "test.md"),
		Title:    "test",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}

	turnID := op.GenerateTurnID()
	prepared, err := prepareReviewMutation(*meta, turnID, cwd, "new.txt")
	if err != nil {
		t.Fatalf("prepareReviewMutation: %v", err)
	}
	newPath := filepath.Join(cwd, "new.txt")
	if err := os.WriteFile(newPath, []byte("created\n"), 0o644); err != nil {
		t.Fatalf("write new file: %v", err)
	}
	if _, err := commitReviewMutation(*meta, prepared, "created\n"); err != nil {
		t.Fatalf("commitReviewMutation: %v", err)
	}
	if got := prepared.ResolvedPath; got != newPath {
		t.Fatalf("prepared.ResolvedPath = %q, want %q", got, newPath)
	}

	review, err := resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: result.ThreadID,
		TurnID:   turnID,
		Decision: op.ThreadReviewDecisionRejectAll,
	})
	if err != nil {
		t.Fatalf("resolveThreadReview rejectAll: %v", err)
	}
	if review.Status != op.ThreadReviewTurnResolved {
		t.Fatalf("review.Status = %q, want resolved", review.Status)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("new file still exists after rejectAll: %v", err)
	}
}

func TestReviewRejectPreservesUserEditsOutsideAgentHunk(t *testing.T) {
	cwd, threadID, meta := setupReviewTestSession(t)
	filePath, turnID := commitReviewTestFile(t, meta, cwd, "main.txt",
		"A\nB\nC\nD\n",
		"A\nB\nX\nD\n",
	)
	if err := os.WriteFile(filePath, []byte("A\nY\nX\nD\n"), 0o644); err != nil {
		t.Fatalf("write user edit: %v", err)
	}

	review, err := resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: threadID,
		TurnID:   turnID,
		Path:     filePath,
		Decision: op.ThreadReviewDecisionReject,
	})
	if err != nil {
		t.Fatalf("resolveThreadReview reject: %v", err)
	}
	if review.Files[0].Status != op.ThreadReviewFileRejected {
		t.Fatalf("review.Files[0].Status = %q, want rejected", review.Files[0].Status)
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after reject: %v", err)
	}
	if string(raw) != "A\nY\nC\nD\n" {
		t.Fatalf("file after reject = %q, want user edit preserved", string(raw))
	}
}

func TestReviewRejectConflictsWhenUserEditsAgentHunk(t *testing.T) {
	cwd, threadID, meta := setupReviewTestSession(t)
	filePath, turnID := commitReviewTestFile(t, meta, cwd, "main.txt",
		"A\nB\nC\nD\n",
		"A\nB\nX\nD\n",
	)
	if err := os.WriteFile(filePath, []byte("A\nB\nZ\nD\n"), 0o644); err != nil {
		t.Fatalf("write user edit: %v", err)
	}

	review, err := resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: threadID,
		TurnID:   turnID,
		Path:     filePath,
		Decision: op.ThreadReviewDecisionReject,
	})
	if err != nil {
		t.Fatalf("resolveThreadReview reject: %v", err)
	}
	if review.Files[0].Status != op.ThreadReviewFilePending {
		t.Fatalf("review.Files[0].Status = %q, want pending", review.Files[0].Status)
	}
	if review.Files[0].MergeState != op.ThreadReviewMergeConflicted {
		t.Fatalf("review.Files[0].MergeState = %q, want conflicted", review.Files[0].MergeState)
	}
	if review.ConflictCount != 1 {
		t.Fatalf("review.ConflictCount = %d, want 1", review.ConflictCount)
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after reject: %v", err)
	}
	if string(raw) != "A\nB\nZ\nD\n" {
		t.Fatalf("file after conflicted reject = %q, want unchanged", string(raw))
	}
}

func TestReviewRejectMarksUserUndoneWithoutWrite(t *testing.T) {
	cwd, threadID, meta := setupReviewTestSession(t)
	filePath, turnID := commitReviewTestFile(t, meta, cwd, "main.txt", "before\n", "after\n")
	if err := os.WriteFile(filePath, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write user undo: %v", err)
	}

	review, err := resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: threadID,
		TurnID:   turnID,
		Path:     filePath,
		Decision: op.ThreadReviewDecisionReject,
	})
	if err != nil {
		t.Fatalf("resolveThreadReview reject: %v", err)
	}
	if review.Files[0].Status != op.ThreadReviewFileRejected {
		t.Fatalf("review.Files[0].Status = %q, want rejected", review.Files[0].Status)
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after reject: %v", err)
	}
	if string(raw) != "before\n" {
		t.Fatalf("file after reject = %q, want unchanged baseline", string(raw))
	}
}

func TestReviewRejectAllPartiallySucceedsOnConflict(t *testing.T) {
	cwd, threadID, meta := setupReviewTestSession(t)
	turnID := op.GenerateTurnID()
	okPath := commitReviewTestFileForTurn(t, meta, turnID, cwd, "ok.txt",
		"A\nB\nC\nD\n",
		"A\nB\nX\nD\n",
	)
	conflictPath := commitReviewTestFileForTurn(t, meta, turnID, cwd, "conflict.txt",
		"A\nB\nC\nD\n",
		"A\nB\nX\nD\n",
	)
	if err := os.WriteFile(okPath, []byte("A\nY\nX\nD\n"), 0o644); err != nil {
		t.Fatalf("write ok user edit: %v", err)
	}
	if err := os.WriteFile(conflictPath, []byte("A\nB\nZ\nD\n"), 0o644); err != nil {
		t.Fatalf("write conflict user edit: %v", err)
	}

	review, err := resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: threadID,
		TurnID:   turnID,
		Decision: op.ThreadReviewDecisionRejectAll,
	})
	if err != nil {
		t.Fatalf("resolveThreadReview rejectAll: %v", err)
	}
	if review.RejectedCount != 1 || review.Unresolved != 1 || review.ConflictCount != 1 {
		t.Fatalf("counts = rejected %d unresolved %d conflict %d, want 1/1/1", review.RejectedCount, review.Unresolved, review.ConflictCount)
	}
	okRaw, err := os.ReadFile(okPath)
	if err != nil {
		t.Fatalf("read ok file: %v", err)
	}
	if string(okRaw) != "A\nY\nC\nD\n" {
		t.Fatalf("ok file = %q, want reverse-applied user edit", string(okRaw))
	}
	conflictRaw, err := os.ReadFile(conflictPath)
	if err != nil {
		t.Fatalf("read conflict file: %v", err)
	}
	if string(conflictRaw) != "A\nB\nZ\nD\n" {
		t.Fatalf("conflict file = %q, want unchanged", string(conflictRaw))
	}
}

func TestReviewRollbackPreservesUserEditsOutsideAgentHunk(t *testing.T) {
	cwd, threadID, meta := setupReviewTestSession(t)
	filePath, turnID := commitReviewTestFile(t, meta, cwd, "main.txt",
		"A\nB\nC\nD\n",
		"A\nB\nX\nD\n",
	)
	if _, err := resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: threadID,
		TurnID:   turnID,
		Path:     filePath,
		Decision: op.ThreadReviewDecisionApprove,
	}); err != nil {
		t.Fatalf("resolveThreadReview approve: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("A\nY\nX\nD\n"), 0o644); err != nil {
		t.Fatalf("write post-approve user edit: %v", err)
	}

	review, err := rollbackThreadReview(op.ThreadReviewRollbackParams{
		ThreadID: threadID,
		TurnID:   turnID,
		Path:     filePath,
		Scope:    op.ThreadReviewRollbackFile,
	})
	if err != nil {
		t.Fatalf("rollbackThreadReview: %v", err)
	}
	if review.Files[0].Status != op.ThreadReviewFileRolledBack {
		t.Fatalf("review.Files[0].Status = %q, want rolledBack", review.Files[0].Status)
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after rollback: %v", err)
	}
	if string(raw) != "A\nY\nC\nD\n" {
		t.Fatalf("file after rollback = %q, want user edit preserved", string(raw))
	}
}

func TestGenerateReviewChangedRangesMultipleBlocks(t *testing.T) {
	ranges := generateReviewChangedRanges("a\nb\nc\nd\n", "a\nB\nc\nD\n")
	if len(ranges) != 2 {
		t.Fatalf("len(ranges) = %d, want 2", len(ranges))
	}
	if ranges[0].StartLine != 2 || ranges[0].EndLine != 2 {
		t.Fatalf("ranges[0] = %+v, want {2 2}", ranges[0])
	}
	if ranges[1].StartLine != 4 || ranges[1].EndLine != 4 {
		t.Fatalf("ranges[1] = %+v, want {4 4}", ranges[1])
	}
}

func TestGenerateReviewHunksMarksMultipleBlocks(t *testing.T) {
	hunks := generateReviewHunks("a\nb\nc\nd\n", "a\nB\nc\nD\n")
	if len(hunks) != 2 {
		t.Fatalf("len(hunks) = %d, want 2", len(hunks))
	}
	if hunks[0].OldStartLine != 2 || hunks[0].OldLineCount != 1 || hunks[0].NewStartLine != 2 || hunks[0].NewLineCount != 1 {
		t.Fatalf("hunks[0] = %+v, want line 2 replacement", hunks[0])
	}
	if got := hunks[0].RemovedLines; len(got) != 1 || got[0] != "b" {
		t.Fatalf("hunks[0].RemovedLines = %#v, want [b]", got)
	}
	if got := hunks[0].AddedLines; len(got) != 1 || got[0] != "B" {
		t.Fatalf("hunks[0].AddedLines = %#v, want [B]", got)
	}
	if hunks[1].OldStartLine != 4 || hunks[1].OldLineCount != 1 || hunks[1].NewStartLine != 4 || hunks[1].NewLineCount != 1 {
		t.Fatalf("hunks[1] = %+v, want line 4 replacement", hunks[1])
	}
}

func TestGenerateReviewHunksMarksDeleteOnlyBlock(t *testing.T) {
	hunks := generateReviewHunks("a\nb\nc\n", "a\nc\n")
	if len(hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(hunks))
	}
	if got := hunks[0]; got.OldStartLine != 2 || got.OldLineCount != 1 || got.NewStartLine != 2 || got.NewLineCount != 0 {
		t.Fatalf("hunk = %+v, want delete-only hunk at line 2", got)
	}
	if got := hunks[0].RemovedLines; len(got) != 1 || got[0] != "b" {
		t.Fatalf("hunk.RemovedLines = %#v, want [b]", got)
	}
	if got := reviewChangedRangesFromHunks(hunks); len(got) != 0 {
		t.Fatalf("changed ranges for delete-only hunk = %#v, want none", got)
	}
}

func TestGenerateReviewHunksMarksAddOnlyBlock(t *testing.T) {
	hunks := generateReviewHunks("a\nc\n", "a\nb\nc\n")
	if len(hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(hunks))
	}
	if got := hunks[0]; got.OldStartLine != 2 || got.OldLineCount != 0 || got.NewStartLine != 2 || got.NewLineCount != 1 {
		t.Fatalf("hunk = %+v, want add-only hunk at line 2", got)
	}
	if got := hunks[0].AddedLines; len(got) != 1 || got[0] != "b" {
		t.Fatalf("hunk.AddedLines = %#v, want [b]", got)
	}
	if got := reviewChangedRangesFromHunks(hunks); len(got) != 1 || got[0].StartLine != 2 || got[0].EndLine != 2 {
		t.Fatalf("changed ranges = %#v, want line 2", got)
	}
}

func TestListThreadReviewStatesReturnsAllTurnsNewestFirst(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "workspace/proj")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "test.md"),
		Title:    "test",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}

	firstPath := filepath.Join(cwd, "first.txt")
	if err := os.WriteFile(firstPath, []byte("before first\n"), 0o644); err != nil {
		t.Fatalf("write first baseline: %v", err)
	}
	firstTurnID := op.GenerateTurnID()
	firstPrepared, err := prepareReviewMutation(*meta, firstTurnID, cwd, "first.txt")
	if err != nil {
		t.Fatalf("prepareReviewMutation first: %v", err)
	}
	if err := os.WriteFile(firstPath, []byte("after first\n"), 0o644); err != nil {
		t.Fatalf("write first final: %v", err)
	}
	if _, err := commitReviewMutation(*meta, firstPrepared, "after first\n"); err != nil {
		t.Fatalf("commitReviewMutation first: %v", err)
	}

	time.Sleep(time.Millisecond)

	secondPath := filepath.Join(cwd, "second.txt")
	if err := os.WriteFile(secondPath, []byte("before second\n"), 0o644); err != nil {
		t.Fatalf("write second baseline: %v", err)
	}
	secondTurnID := op.GenerateTurnID()
	secondPrepared, err := prepareReviewMutation(*meta, secondTurnID, cwd, "second.txt")
	if err != nil {
		t.Fatalf("prepareReviewMutation second: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("after second\n"), 0o644); err != nil {
		t.Fatalf("write second final: %v", err)
	}
	if _, err := commitReviewMutation(*meta, secondPrepared, "after second\n"); err != nil {
		t.Fatalf("commitReviewMutation second: %v", err)
	}

	reviews, err := listThreadReviewStates(op.ThreadReviewListParams{
		ThreadID: result.ThreadID,
		ChatPath: result.ChatPath,
	})
	if err != nil {
		t.Fatalf("listThreadReviewStates: %v", err)
	}
	if got := len(reviews); got != 2 {
		t.Fatalf("len(reviews) = %d, want 2", got)
	}
	if reviews[0].TurnID != secondTurnID {
		t.Fatalf("reviews[0].TurnID = %q, want %q", reviews[0].TurnID, secondTurnID)
	}
	if reviews[1].TurnID != firstTurnID {
		t.Fatalf("reviews[1].TurnID = %q, want %q", reviews[1].TurnID, firstTurnID)
	}
	if reviews[0].Status != op.ThreadReviewTurnPending || reviews[1].Status != op.ThreadReviewTurnPending {
		t.Fatalf("review statuses = %q, %q, want pending/pending", reviews[0].Status, reviews[1].Status)
	}
}

func TestResolveThreadReviewOnlyTouchesRequestedTurn(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "workspace/proj")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "test.md"),
		Title:    "test",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}

	firstPath := filepath.Join(cwd, "first.txt")
	if err := os.WriteFile(firstPath, []byte("before first\n"), 0o644); err != nil {
		t.Fatalf("write first baseline: %v", err)
	}
	firstTurnID := op.GenerateTurnID()
	firstPrepared, err := prepareReviewMutation(*meta, firstTurnID, cwd, "first.txt")
	if err != nil {
		t.Fatalf("prepareReviewMutation first: %v", err)
	}
	if err := os.WriteFile(firstPath, []byte("after first\n"), 0o644); err != nil {
		t.Fatalf("write first final: %v", err)
	}
	if _, err := commitReviewMutation(*meta, firstPrepared, "after first\n"); err != nil {
		t.Fatalf("commitReviewMutation first: %v", err)
	}

	secondPath := filepath.Join(cwd, "second.txt")
	if err := os.WriteFile(secondPath, []byte("before second\n"), 0o644); err != nil {
		t.Fatalf("write second baseline: %v", err)
	}
	secondTurnID := op.GenerateTurnID()
	secondPrepared, err := prepareReviewMutation(*meta, secondTurnID, cwd, "second.txt")
	if err != nil {
		t.Fatalf("prepareReviewMutation second: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("after second\n"), 0o644); err != nil {
		t.Fatalf("write second final: %v", err)
	}
	if _, err := commitReviewMutation(*meta, secondPrepared, "after second\n"); err != nil {
		t.Fatalf("commitReviewMutation second: %v", err)
	}

	review, err := resolveThreadReview(op.ThreadReviewResolveParams{
		ThreadID: result.ThreadID,
		TurnID:   firstTurnID,
		Decision: op.ThreadReviewDecisionApproveAll,
	})
	if err != nil {
		t.Fatalf("resolveThreadReview approveAll: %v", err)
	}
	if review.TurnID != firstTurnID {
		t.Fatalf("review.TurnID = %q, want %q", review.TurnID, firstTurnID)
	}
	if review.Status != op.ThreadReviewTurnResolved {
		t.Fatalf("review.Status = %q, want resolved", review.Status)
	}

	reviews, err := listThreadReviewStates(op.ThreadReviewListParams{
		ThreadID: result.ThreadID,
		ChatPath: result.ChatPath,
	})
	if err != nil {
		t.Fatalf("listThreadReviewStates: %v", err)
	}
	if got := len(reviews); got != 2 {
		t.Fatalf("len(reviews) = %d, want 2", got)
	}

	byTurnID := make(map[string]op.ThreadReviewState, len(reviews))
	for _, item := range reviews {
		byTurnID[item.TurnID] = item
	}
	firstReview, ok := byTurnID[firstTurnID]
	if !ok {
		t.Fatalf("missing review for turn %q", firstTurnID)
	}
	secondReview, ok := byTurnID[secondTurnID]
	if !ok {
		t.Fatalf("missing review for turn %q", secondTurnID)
	}
	if firstReview.Status != op.ThreadReviewTurnResolved {
		t.Fatalf("first review status = %q, want resolved", firstReview.Status)
	}
	if firstReview.ApprovedCount != 1 {
		t.Fatalf("first review approvedCount = %d, want 1", firstReview.ApprovedCount)
	}
	if secondReview.Status != op.ThreadReviewTurnPending {
		t.Fatalf("second review status = %q, want pending", secondReview.Status)
	}
	if secondReview.Unresolved != 1 {
		t.Fatalf("second review unresolved = %d, want 1", secondReview.Unresolved)
	}
}

func TestNewAgentLoopAllowsPendingReview(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	agentID, _, agentFilePath := createTestAgent(t, baseDir, "workspace/proj")
	cache.Flush()
	cache.Set("test:auto", cache.PrefixDefault, &op.ModelConfig{
		Key:      "test:auto",
		ID:       "auto",
		Name:     "Auto",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  "https://example.com/v1",
		APIKey:   "test-key",
	}, cache.NoExpiration)

	node := &op.OpNode{
		ID:   agentID,
		Kind: string(op.NodeKindAgent),
		Cwd:  cwd,
		URI:  op.PathToURI(agentFilePath),
		Meta: &op.AgentMeta{},
	}

	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "test.md"),
		Title:    "test",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}

	filePath := filepath.Join(cwd, "main.go")
	if err := os.WriteFile(filePath, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write baseline file: %v", err)
	}
	pendingTurnID := op.GenerateTurnID()
	prepared, err := prepareReviewMutation(*meta, pendingTurnID, cwd, "main.go")
	if err != nil {
		t.Fatalf("prepareReviewMutation: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("write final file: %v", err)
	}
	if _, err := commitReviewMutation(*meta, prepared, "after\n"); err != nil {
		t.Fatalf("commitReviewMutation: %v", err)
	}

	loop, err := NewAgentLoop(context.Background(), node, op.Meta{
		"chatPath": result.ChatPath,
		"threadID": result.ThreadID,
		"agentID":  agentID,
		"modelKey": "test:auto",
	}, &op.TextContent{Text: "continue"})
	if err != nil {
		t.Fatalf("NewAgentLoop: %v", err)
	}
	if loop == nil {
		t.Fatalf("NewAgentLoop returned nil loop")
	}
	if loop.ThreadID != result.ThreadID {
		t.Fatalf("loop.ThreadID = %q, want %q", loop.ThreadID, result.ThreadID)
	}
	loop.Cancel()
}

func TestNewAgentLoopBackfillsMetaCWDFromSession(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	cache.Flush()
	t.Cleanup(cache.Flush)
	agentID, _, agentFilePath := createTestAgent(t, baseDir, "workspace/proj")
	cache.Set("test:auto", cache.PrefixDefault, &op.ModelConfig{
		Key:      "test:auto",
		ID:       "auto",
		Name:     "Auto",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  "https://example.com/v1",
		APIKey:   "test-key",
	}, cache.NoExpiration)

	node := &op.OpNode{
		ID:   agentID,
		Kind: string(op.NodeKindAgent),
		Cwd:  filepath.Join(baseDir, "wrong-agent-dir"),
		URI:  op.PathToURI(agentFilePath),
		Meta: &op.AgentMeta{},
	}

	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "cwd-backfill.md"),
		Title:    "cwd-backfill",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	loop, err := NewAgentLoop(context.Background(), node, op.Meta{
		"chatPath": result.ChatPath,
		"threadID": result.ThreadID,
		"agentID":  agentID,
		"modelKey": "test:auto",
	}, &op.TextContent{Text: "continue"})
	if err != nil {
		t.Fatalf("NewAgentLoop: %v", err)
	}
	if loop == nil {
		t.Fatalf("NewAgentLoop returned nil loop")
	}
	if got := metaString(loop.Agent.Meta, "cwd"); got != cwd {
		t.Fatalf("loop.Agent.Meta[cwd] = %q, want %q", got, cwd)
	}
	if got := loop.Workdir; got != cwd {
		t.Fatalf("loop.Workdir = %q, want %q", got, cwd)
	}
	loop.Cancel()
}

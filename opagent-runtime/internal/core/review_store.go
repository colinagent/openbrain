package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	currentReviewVersion = 2
	maxReviewDiffMatrix  = 1_200_000
)

type reviewFileManifest struct {
	Path               string                     `json:"path"`
	Status             op.ThreadReviewFileStatus  `json:"status"`
	Diff               string                     `json:"diff"`
	BaselineExists     bool                       `json:"baselineExists"`
	FirstChangedLine   int                        `json:"firstChangedLine,omitempty"`
	FirstChangedColumn int                        `json:"firstChangedColumn,omitempty"`
	LineCount          int                        `json:"lineCount,omitempty"`
	ChangedRanges      []op.ThreadReviewLineRange `json:"changedRanges,omitempty"`
	Hunks              []op.ThreadReviewHunk      `json:"hunks,omitempty"`
	BaselineHash       string                     `json:"baselineHash,omitempty"`
	FinalHash          string                     `json:"finalHash,omitempty"`
	BaselineBlob       string                     `json:"baselineBlob,omitempty"`
	FinalBlob          string                     `json:"finalBlob,omitempty"`
	ApprovedHash       string                     `json:"approvedHash,omitempty"`
	ResolvedAt         string                     `json:"resolvedAt,omitempty"`
}

type reviewTurnManifest struct {
	Version   int                       `json:"version"`
	ThreadID  string                    `json:"threadID"`
	TurnID    string                    `json:"turnID"`
	ChatPath  string                    `json:"chatPath"`
	AgentID   string                    `json:"agentID"`
	CreatedAt string                    `json:"createdAt"`
	Status    op.ThreadReviewTurnStatus `json:"status"`
	Files     []reviewFileManifest      `json:"files"`
}

type preparedReviewMutation struct {
	ThreadID       string
	TurnID         string
	ResolvedPath   string
	BaselineExists bool
	BaselineText   string
}

type reviewFileMergeInfo struct {
	MergeState      op.ThreadReviewMergeState
	HasUserEdits    bool
	CanUndo         bool
	ConflictMessage string
	CurrentHash     string
	CurrentExists   bool
	ReverseText     string
	ReverseOK       bool
}

type reviewReverseApplyResult struct {
	Content         string
	OK              bool
	ConflictMessage string
}

func listThreadReviewStates(params op.ThreadReviewListParams) ([]op.ThreadReviewState, error) {
	if strings.TrimSpace(params.ThreadID) == "" {
		return nil, nil
	}
	record, err := defaultThreadStore.loadRecord(op.ThreadMetaQuery{
		ThreadID: params.ThreadID,
	})
	if err != nil {
		if isThreadNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	manifests, err := listReviewManifests(record)
	if err != nil || len(manifests) == 0 {
		return nil, err
	}
	states := make([]op.ThreadReviewState, 0, len(manifests))
	normalizedChatPath := normalizeThreadPath(record.header.ChatPath)
	for _, manifest := range manifests {
		if manifest == nil {
			continue
		}
		state, err := manifestToReviewState(manifest, reviewTurnDir(record, manifest.TurnID))
		if err != nil {
			return nil, err
		}
		state.ChatPath = normalizedChatPath
		states = append(states, state)
	}
	return states, nil
}

func prepareReviewMutation(meta op.ThreadMeta, turnID, workdir, rawPath string) (*preparedReviewMutation, error) {
	resolvedPath, err := resolveReviewPath(rawPath, workdir)
	if err != nil {
		return nil, err
	}
	record, err := defaultThreadStore.loadRecord(op.ThreadMetaQuery{
		ThreadID: meta.ThreadID,
		AgentID:  meta.AgentID,
	})
	if err != nil {
		return nil, err
	}
	manifest, err := loadReviewManifest(record, turnID)
	if err != nil {
		return nil, err
	}
	if manifest != nil {
		for _, file := range manifest.Files {
			if filepath.Clean(file.Path) == resolvedPath {
				return &preparedReviewMutation{
					ThreadID:     meta.ThreadID,
					TurnID:       turnID,
					ResolvedPath: resolvedPath,
				}, nil
			}
		}
	}
	content, exists, err := readFileMaybe(resolvedPath)
	if err != nil {
		return nil, err
	}
	return &preparedReviewMutation{
		ThreadID:       meta.ThreadID,
		TurnID:         turnID,
		ResolvedPath:   resolvedPath,
		BaselineExists: exists,
		BaselineText:   content,
	}, nil
}

func commitReviewMutation(meta op.ThreadMeta, prepared *preparedReviewMutation, finalText string) (*op.ThreadReviewState, error) {
	if prepared == nil {
		return nil, fmt.Errorf("prepared review mutation is required")
	}
	record, err := defaultThreadStore.loadRecord(op.ThreadMetaQuery{
		ThreadID: meta.ThreadID,
		AgentID:  meta.AgentID,
	})
	if err != nil {
		return nil, err
	}

	lock := defaultThreadStore.mutexForThread(strings.TrimSpace(record.header.ID))
	lock.Lock()
	defer lock.Unlock()

	manifest, err := loadReviewManifest(record, prepared.TurnID)
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		manifest = &reviewTurnManifest{
			Version:   currentReviewVersion,
			ThreadID:  strings.TrimSpace(record.header.ID),
			TurnID:    strings.TrimSpace(prepared.TurnID),
			ChatPath:  normalizeThreadPath(meta.ChatPath),
			AgentID:   normalizeThreadValue(meta.AgentID),
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Status:    op.ThreadReviewTurnPending,
			Files:     make([]reviewFileManifest, 0, 4),
		}
	}

	fileIndex := -1
	for index := range manifest.Files {
		if filepath.Clean(manifest.Files[index].Path) == prepared.ResolvedPath {
			fileIndex = index
			break
		}
	}

	turnDir := reviewTurnDir(record, prepared.TurnID)
	if err := os.MkdirAll(turnDir, 0o755); err != nil {
		return nil, err
	}

	if fileIndex == -1 {
		baselineBlob, baselineHash, err := writeReviewSnapshot(turnDir, prepared.ResolvedPath, "baseline", prepared.BaselineText)
		if err != nil {
			return nil, err
		}
		manifest.Files = append(manifest.Files, reviewFileManifest{
			Path:           prepared.ResolvedPath,
			Status:         op.ThreadReviewFilePending,
			BaselineExists: prepared.BaselineExists,
			BaselineHash:   baselineHash,
			BaselineBlob:   baselineBlob,
		})
		fileIndex = len(manifest.Files) - 1
	}

	file := &manifest.Files[fileIndex]
	finalBlob, finalHash, err := writeReviewSnapshot(turnDir, prepared.ResolvedPath, "final", finalText)
	if err != nil {
		return nil, err
	}
	baselineText, err := readReviewBlob(turnDir, file.BaselineBlob)
	if err != nil {
		return nil, err
	}
	file.FinalBlob = finalBlob
	file.FinalHash = finalHash
	file.Status = op.ThreadReviewFilePending
	file.Diff, file.FirstChangedLine = generateReviewDiff(baselineText, finalText)
	file.Hunks = generateReviewHunks(baselineText, finalText)
	file.ChangedRanges = reviewChangedRangesFromHunks(file.Hunks)
	if len(file.Hunks) > 0 {
		file.FirstChangedLine = file.Hunks[0].NewStartLine
	} else if len(file.ChangedRanges) > 0 {
		file.FirstChangedLine = file.ChangedRanges[0].StartLine
	}
	if file.FirstChangedLine > 0 {
		file.FirstChangedColumn = 1
	} else {
		file.FirstChangedColumn = 0
	}
	file.LineCount = countReviewLines(finalText)
	manifest.Status = deriveReviewTurnStatus(manifest)

	if err := writeReviewManifest(record, manifest); err != nil {
		return nil, err
	}
	state, err := manifestToReviewState(manifest, turnDir)
	if err != nil {
		return nil, err
	}
	state.ChatPath = normalizeThreadPath(record.header.ChatPath)
	return &state, nil
}

func resolveThreadReview(params op.ThreadReviewResolveParams) (*op.ThreadReviewState, error) {
	record, manifest, turnDir, err := loadReviewMutationTarget(params.ThreadID, params.TurnID)
	if err != nil {
		return nil, err
	}
	lock := defaultThreadStore.mutexForThread(strings.TrimSpace(record.header.ID))
	lock.Lock()
	defer lock.Unlock()

	targets, err := resolveDecisionTargets(manifest, params)
	if err != nil {
		return nil, err
	}
	if err := applyDecisionTargets(turnDir, targets, params.Decision); err != nil {
		return nil, err
	}

	manifest.Status = deriveReviewTurnStatus(manifest)
	if err := writeReviewManifest(record, manifest); err != nil {
		return nil, err
	}
	state, err := manifestToReviewState(manifest, turnDir)
	if err != nil {
		return nil, err
	}
	state.ChatPath = normalizeThreadPath(record.header.ChatPath)
	return &state, nil
}

func rollbackThreadReview(params op.ThreadReviewRollbackParams) (*op.ThreadReviewState, error) {
	record, manifest, turnDir, err := loadReviewMutationTarget(params.ThreadID, params.TurnID)
	if err != nil {
		return nil, err
	}
	lock := defaultThreadStore.mutexForThread(strings.TrimSpace(record.header.ID))
	lock.Lock()
	defer lock.Unlock()

	targets, err := resolveRollbackTargets(manifest, params)
	if err != nil {
		return nil, err
	}
	if err := applyRollbackTargets(turnDir, targets); err != nil {
		return nil, err
	}

	manifest.Status = deriveReviewTurnStatus(manifest)
	if err := writeReviewManifest(record, manifest); err != nil {
		return nil, err
	}
	state, err := manifestToReviewState(manifest, turnDir)
	if err != nil {
		return nil, err
	}
	state.ChatPath = normalizeThreadPath(record.header.ChatPath)
	return &state, nil
}

func loadReviewMutationTarget(threadID, turnID string) (*threadRecord, *reviewTurnManifest, string, error) {
	record, err := defaultThreadStore.loadRecord(op.ThreadMetaQuery{
		ThreadID: threadID,
	})
	if err != nil {
		return nil, nil, "", err
	}
	manifest, err := loadReviewManifest(record, turnID)
	if err != nil {
		return nil, nil, "", err
	}
	if manifest == nil {
		return nil, nil, "", fmt.Errorf("review turn not found: %s", turnID)
	}
	turnDir := reviewTurnDir(record, turnID)
	return record, manifest, turnDir, nil
}

func resolveDecisionTargets(manifest *reviewTurnManifest, params op.ThreadReviewResolveParams) ([]*reviewFileManifest, error) {
	switch params.Decision {
	case op.ThreadReviewDecisionApprove, op.ThreadReviewDecisionReject:
		if strings.TrimSpace(params.Path) == "" {
			return nil, fmt.Errorf("path is required for %s", params.Decision)
		}
		for index := range manifest.Files {
			if filepath.Clean(manifest.Files[index].Path) == filepath.Clean(params.Path) {
				return []*reviewFileManifest{&manifest.Files[index]}, nil
			}
		}
		return nil, fmt.Errorf("review file not found: %s", params.Path)
	case op.ThreadReviewDecisionApproveAll, op.ThreadReviewDecisionRejectAll:
		targets := make([]*reviewFileManifest, 0, len(manifest.Files))
		for index := range manifest.Files {
			if manifest.Files[index].Status != op.ThreadReviewFilePending {
				continue
			}
			targets = append(targets, &manifest.Files[index])
		}
		return targets, nil
	default:
		return nil, fmt.Errorf("unsupported review decision: %s", params.Decision)
	}
}

func applyDecisionTargets(turnDir string, targets []*reviewFileManifest, decision op.ThreadReviewDecision) error {
	for _, target := range targets {
		if target == nil {
			continue
		}
		if err := applyDecisionTarget(turnDir, target, decision); err != nil {
			return err
		}
	}
	return nil
}

func applyDecisionTarget(turnDir string, target *reviewFileManifest, decision op.ThreadReviewDecision) error {
	if target.Status != op.ThreadReviewFilePending {
		return nil
	}
	mergeInfo, err := classifyReviewFileManifest(turnDir, target)
	if err != nil {
		return err
	}
	switch decision {
	case op.ThreadReviewDecisionApprove, op.ThreadReviewDecisionApproveAll:
		if !mergeInfo.CurrentExists && target.BaselineExists {
			return nil
		}
		if mergeInfo.MergeState == op.ThreadReviewMergeUserUndone || (!mergeInfo.CurrentExists && !target.BaselineExists) {
			target.Status = op.ThreadReviewFileRejected
		} else {
			target.Status = op.ThreadReviewFileApproved
			target.ApprovedHash = mergeInfo.CurrentHash
		}
		target.ResolvedAt = time.Now().UTC().Format(time.RFC3339Nano)
	case op.ThreadReviewDecisionReject, op.ThreadReviewDecisionRejectAll:
		if mergeInfo.MergeState == op.ThreadReviewMergeUserUndone || (!mergeInfo.CurrentExists && !target.BaselineExists) {
			if !target.BaselineExists && mergeInfo.CurrentExists {
				if err := os.Remove(target.Path); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			target.Status = op.ThreadReviewFileRejected
			target.ResolvedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return nil
		}
		if !mergeInfo.CanUndo || !mergeInfo.ReverseOK {
			return nil
		}
		if target.BaselineExists {
			if err := writeFileAtomic(target.Path, mergeInfo.ReverseText); err != nil {
				return err
			}
		} else {
			if err := os.Remove(target.Path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		target.Status = op.ThreadReviewFileRejected
		target.ResolvedAt = time.Now().UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Errorf("unsupported review decision: %s", decision)
	}
	return nil
}

func resolveRollbackTargets(manifest *reviewTurnManifest, params op.ThreadReviewRollbackParams) ([]*reviewFileManifest, error) {
	switch params.Scope {
	case op.ThreadReviewRollbackFile:
		if strings.TrimSpace(params.Path) == "" {
			return nil, fmt.Errorf("path is required for file rollback")
		}
		for index := range manifest.Files {
			if filepath.Clean(manifest.Files[index].Path) == filepath.Clean(params.Path) {
				if manifest.Files[index].Status != op.ThreadReviewFileApproved {
					return nil, fmt.Errorf("file is not approved: %s", params.Path)
				}
				return []*reviewFileManifest{&manifest.Files[index]}, nil
			}
		}
		return nil, fmt.Errorf("review file not found: %s", params.Path)
	case op.ThreadReviewRollbackTurn:
		targets := make([]*reviewFileManifest, 0, len(manifest.Files))
		for index := range manifest.Files {
			if manifest.Files[index].Status == op.ThreadReviewFileApproved {
				targets = append(targets, &manifest.Files[index])
			}
		}
		return targets, nil
	default:
		return nil, fmt.Errorf("unsupported rollback scope: %s", params.Scope)
	}
}

func applyRollbackTargets(turnDir string, targets []*reviewFileManifest) error {
	for _, target := range targets {
		if target == nil {
			continue
		}
		mergeInfo, err := classifyReviewFileManifest(turnDir, target)
		if err != nil {
			return err
		}
		if mergeInfo.MergeState == op.ThreadReviewMergeUserUndone || (!mergeInfo.CurrentExists && !target.BaselineExists) {
			if !target.BaselineExists && mergeInfo.CurrentExists {
				if err := os.Remove(target.Path); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			target.Status = op.ThreadReviewFileRolledBack
			target.ResolvedAt = time.Now().UTC().Format(time.RFC3339Nano)
			continue
		}
		if !mergeInfo.CanUndo || !mergeInfo.ReverseOK {
			continue
		}
		if target.BaselineExists {
			if err := writeFileAtomic(target.Path, mergeInfo.ReverseText); err != nil {
				return err
			}
		} else if err := os.Remove(target.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		target.Status = op.ThreadReviewFileRolledBack
		target.ResolvedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return nil
}

func classifyReviewFileManifest(turnDir string, file *reviewFileManifest) (reviewFileMergeInfo, error) {
	if file == nil {
		return reviewFileMergeInfo{}, nil
	}
	baselineText, err := readReviewBlob(turnDir, file.BaselineBlob)
	if err != nil {
		return reviewFileMergeInfo{}, err
	}
	finalText, err := readReviewBlob(turnDir, file.FinalBlob)
	if err != nil {
		return reviewFileMergeInfo{}, err
	}
	currentText, currentExists, err := readFileMaybe(file.Path)
	if err != nil {
		return reviewFileMergeInfo{}, err
	}
	return classifyReviewFileState(baselineText, finalText, currentText, currentExists, file.BaselineExists, file.Status, file.Hunks), nil
}

func classifyReviewFileState(baseline, final, current string, exists bool, baselineExists bool, status op.ThreadReviewFileStatus, hunks []op.ThreadReviewHunk) reviewFileMergeInfo {
	currentHash := hashReviewContent(current)
	info := reviewFileMergeInfo{
		MergeState:    op.ThreadReviewMergeClean,
		CurrentHash:   currentHash,
		CurrentExists: exists,
	}
	canUndo := status == op.ThreadReviewFilePending || status == op.ThreadReviewFileApproved
	if status == op.ThreadReviewFileRejected || status == op.ThreadReviewFileRolledBack {
		if !exists {
			if baselineExists {
				info.MergeState = op.ThreadReviewMergeMissing
				info.HasUserEdits = true
				info.ConflictMessage = "The file is missing."
			}
			return info
		}
		if baselineExists && currentHash == hashReviewContent(baseline) {
			return info
		}
		info.MergeState = op.ThreadReviewMergeUserEdited
		info.HasUserEdits = true
		return info
	}

	if !exists {
		if baselineExists {
			info.MergeState = op.ThreadReviewMergeMissing
			info.HasUserEdits = true
			info.ConflictMessage = "The file is missing."
			return info
		}
		info.MergeState = op.ThreadReviewMergeUserUndone
		info.HasUserEdits = true
		info.CanUndo = canUndo
		info.ReverseOK = true
		info.ReverseText = baseline
		return info
	}

	finalHash := hashReviewContent(final)
	baselineHash := hashReviewContent(baseline)
	if currentHash == finalHash {
		info.CanUndo = canUndo
		info.ReverseOK = canUndo
		info.ReverseText = baseline
		return info
	}
	info.HasUserEdits = true
	if currentHash == baselineHash {
		info.MergeState = op.ThreadReviewMergeUserUndone
		info.CanUndo = canUndo
		info.ReverseOK = canUndo
		info.ReverseText = baseline
		return info
	}
	if !baselineExists {
		info.MergeState = op.ThreadReviewMergeConflicted
		info.ConflictMessage = "This change was edited after the agent wrote it."
		return info
	}

	reverse := tryReverseAgentHunks(baseline, final, current, hunks)
	if reverse.OK {
		info.MergeState = op.ThreadReviewMergeUserEdited
		info.CanUndo = canUndo
		info.ReverseOK = canUndo
		info.ReverseText = reverse.Content
		return info
	}
	info.MergeState = op.ThreadReviewMergeConflicted
	info.ConflictMessage = reverse.ConflictMessage
	if strings.TrimSpace(info.ConflictMessage) == "" {
		info.ConflictMessage = "This change was edited after the agent wrote it."
	}
	return info
}

func tryReverseAgentHunks(baseline, final, current string, hunks []op.ThreadReviewHunk) reviewReverseApplyResult {
	if current == final {
		return reviewReverseApplyResult{Content: baseline, OK: true}
	}
	if current == baseline {
		return reviewReverseApplyResult{Content: baseline, OK: true}
	}
	if len(hunks) == 0 {
		return reviewReverseApplyResult{ConflictMessage: "This change was edited after the agent wrote it."}
	}

	finalLines := strings.Split(final, "\n")
	currentLines := strings.Split(current, "\n")
	mapping, ok := computeReviewLineMapping(finalLines, currentLines)
	if !ok {
		return reviewReverseApplyResult{ConflictMessage: "This change is too large to safely rebase."}
	}

	type replacement struct {
		start int
		end   int
		lines []string
	}
	replacements := make([]replacement, 0, len(hunks))
	for _, hunk := range hunks {
		start, end, ok := mapReviewHunkFinalRangeToCurrent(hunk, mapping, finalLines, currentLines)
		if !ok {
			return reviewReverseApplyResult{ConflictMessage: "This change was edited after the agent wrote it."}
		}
		if hunk.NewLineCount > 0 {
			if end-start != len(hunk.AddedLines) {
				return reviewReverseApplyResult{ConflictMessage: "This change was edited after the agent wrote it."}
			}
			for offset, line := range hunk.AddedLines {
				if start+offset >= len(currentLines) || currentLines[start+offset] != line {
					return reviewReverseApplyResult{ConflictMessage: "This change was edited after the agent wrote it."}
				}
			}
		}
		replacements = append(replacements, replacement{
			start: start,
			end:   end,
			lines: append([]string(nil), hunk.RemovedLines...),
		})
	}

	sort.Slice(replacements, func(i, j int) bool {
		if replacements[i].start == replacements[j].start {
			return replacements[i].end > replacements[j].end
		}
		return replacements[i].start > replacements[j].start
	})
	nextStart := len(currentLines) + 1
	for _, item := range replacements {
		if item.start < 0 || item.end < item.start || item.end > len(currentLines) || item.end > nextStart {
			return reviewReverseApplyResult{ConflictMessage: "This change was edited after the agent wrote it."}
		}
		nextStart = item.start
	}
	for _, item := range replacements {
		next := make([]string, 0, len(currentLines)-(item.end-item.start)+len(item.lines))
		next = append(next, currentLines[:item.start]...)
		next = append(next, item.lines...)
		next = append(next, currentLines[item.end:]...)
		currentLines = next
	}
	return reviewReverseApplyResult{Content: strings.Join(currentLines, "\n"), OK: true}
}

func computeReviewLineMapping(fromLines, toLines []string) ([]int, bool) {
	if len(fromLines) > 0 && len(toLines) > maxReviewDiffMatrix/len(fromLines) {
		return nil, false
	}
	dp := make([][]int, len(fromLines)+1)
	for i := range dp {
		dp[i] = make([]int, len(toLines)+1)
	}
	for i := len(fromLines) - 1; i >= 0; i-- {
		for j := len(toLines) - 1; j >= 0; j-- {
			if fromLines[i] == toLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	mapping := make([]int, len(fromLines))
	for i := range mapping {
		mapping[i] = -1
	}
	for i, j := 0, 0; i < len(fromLines) && j < len(toLines); {
		if fromLines[i] == toLines[j] {
			mapping[i] = j
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return mapping, true
}

func mapReviewHunkFinalRangeToCurrent(hunk op.ThreadReviewHunk, mapping []int, finalLines, currentLines []string) (int, int, bool) {
	finalStart := hunk.NewStartLine - 1
	if finalStart < 0 || finalStart > len(finalLines) || hunk.NewLineCount < 0 || finalStart+hunk.NewLineCount > len(finalLines) {
		return 0, 0, false
	}
	if hunk.NewLineCount == 0 {
		boundary, ok := mapReviewFinalBoundaryToCurrent(finalStart, mapping, finalLines, currentLines)
		return boundary, boundary, ok
	}
	start := -1
	for offset := 0; offset < hunk.NewLineCount; offset++ {
		mapped := mapping[finalStart+offset]
		if mapped < 0 {
			return 0, 0, false
		}
		if start < 0 {
			start = mapped
		} else if mapped != start+offset {
			return 0, 0, false
		}
		if finalLines[finalStart+offset] != currentLines[mapped] {
			return 0, 0, false
		}
	}
	return start, start + hunk.NewLineCount, true
}

func mapReviewFinalBoundaryToCurrent(boundary int, mapping []int, finalLines, currentLines []string) (int, bool) {
	if boundary < 0 || boundary > len(finalLines) {
		return 0, false
	}
	prevMapped := -1
	if boundary > 0 && boundary-1 < len(mapping) {
		prevMapped = mapping[boundary-1]
	}
	nextMapped := -1
	if boundary < len(mapping) {
		nextMapped = mapping[boundary]
	}
	switch {
	case prevMapped >= 0 && nextMapped >= 0:
		if prevMapped+1 != nextMapped {
			return 0, false
		}
		return nextMapped, true
	case prevMapped >= 0:
		return prevMapped + 1, true
	case nextMapped >= 0:
		return nextMapped, true
	case boundary == 0 && isReviewEmptyLines(currentLines):
		return 0, true
	default:
		return 0, false
	}
}

func isReviewEmptyLines(lines []string) bool {
	return len(lines) == 0 || (len(lines) == 1 && lines[0] == "")
}

func listReviewManifests(record *threadRecord) ([]*reviewTurnManifest, error) {
	root := reviewRootDir(record)
	entries, err := os.ReadDir(filepath.Join(root, "turns"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	manifests := make([]*reviewTurnManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := readReviewManifest(filepath.Join(root, "turns", entry.Name(), "manifest.json"))
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].CreatedAt > manifests[j].CreatedAt
	})
	return manifests, nil
}

func loadReviewManifest(record *threadRecord, turnID string) (*reviewTurnManifest, error) {
	path := reviewManifestPath(record, turnID)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return readReviewManifest(path)
}

func writeReviewManifest(record *threadRecord, manifest *reviewTurnManifest) error {
	manifest.Status = deriveReviewTurnStatus(manifest)
	path := reviewManifestPath(record, manifest.TurnID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readReviewManifest(path string) (*reviewTurnManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest reviewTurnManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	manifest.Status = deriveReviewTurnStatus(&manifest)
	return &manifest, nil
}

func reviewRootDir(record *threadRecord) string {
	base := strings.TrimSuffix(record.filePath, ".jsonl")
	return base + ".review"
}

func reviewTurnDir(record *threadRecord, turnID string) string {
	return filepath.Join(reviewRootDir(record), "turns", strings.TrimSpace(turnID))
}

func reviewManifestPath(record *threadRecord, turnID string) string {
	return filepath.Join(reviewTurnDir(record, turnID), "manifest.json")
}

func writeReviewSnapshot(turnDir, filePath, kind, content string) (string, string, error) {
	name := reviewBlobName(filePath, kind)
	path := filepath.Join(turnDir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", "", err
	}
	return name, hashReviewContent(content), nil
}

func readReviewBlob(turnDir, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", nil
	}
	raw, err := os.ReadFile(filepath.Join(turnDir, name))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func reviewBlobName(filePath, kind string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(filePath) + ":" + kind))
	return fmt.Sprintf("%s-%s.txt", kind, hex.EncodeToString(sum[:8]))
}

func hashReviewContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func countReviewLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func manifestToReviewState(manifest *reviewTurnManifest, turnDir string) (op.ThreadReviewState, error) {
	files := make([]op.ThreadReviewFile, 0, len(manifest.Files))
	unresolved := 0
	approved := 0
	rejected := 0
	rolledBack := 0
	conflicted := 0
	for _, file := range manifest.Files {
		mergeInfo, err := classifyReviewFileManifest(turnDir, &file)
		if err != nil {
			return op.ThreadReviewState{}, err
		}
		files = append(files, op.ThreadReviewFile{
			Path:               file.Path,
			Status:             file.Status,
			MergeState:         mergeInfo.MergeState,
			HasUserEdits:       mergeInfo.HasUserEdits,
			CanUndo:            mergeInfo.CanUndo,
			ConflictMessage:    mergeInfo.ConflictMessage,
			Diff:               file.Diff,
			BaselineExists:     file.BaselineExists,
			FirstChangedLine:   file.FirstChangedLine,
			FirstChangedColumn: file.FirstChangedColumn,
			LineCount:          file.LineCount,
			ChangedRanges:      append([]op.ThreadReviewLineRange(nil), file.ChangedRanges...),
			Hunks:              append([]op.ThreadReviewHunk(nil), file.Hunks...),
		})
		if mergeInfo.MergeState == op.ThreadReviewMergeConflicted {
			conflicted++
		}
		switch file.Status {
		case op.ThreadReviewFilePending:
			unresolved++
		case op.ThreadReviewFileApproved:
			approved++
		case op.ThreadReviewFileRejected:
			rejected++
		case op.ThreadReviewFileRolledBack:
			rolledBack++
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return op.ThreadReviewState{
		ThreadID:        manifest.ThreadID,
		TurnID:          manifest.TurnID,
		ChatPath:        manifest.ChatPath,
		Status:          manifest.Status,
		CreatedAt:       manifest.CreatedAt,
		CanReview:       unresolved > 0,
		CanRollback:     unresolved == 0 && approved > 0,
		Unresolved:      unresolved,
		ApprovedCount:   approved,
		RejectedCount:   rejected,
		RolledBackCount: rolledBack,
		ConflictCount:   conflicted,
		Files:           files,
	}, nil
}

func deriveReviewTurnStatus(manifest *reviewTurnManifest) op.ThreadReviewTurnStatus {
	hasPending := false
	hasApproved := false
	hasRolledBack := false
	for _, file := range manifest.Files {
		switch file.Status {
		case op.ThreadReviewFilePending:
			hasPending = true
		case op.ThreadReviewFileApproved:
			hasApproved = true
		case op.ThreadReviewFileRolledBack:
			hasRolledBack = true
		}
	}
	switch {
	case hasPending:
		return op.ThreadReviewTurnPending
	case hasApproved:
		return op.ThreadReviewTurnResolved
	case hasRolledBack:
		return op.ThreadReviewTurnRolledBack
	default:
		return op.ThreadReviewTurnResolved
	}
}

func generateReviewChangedRanges(oldContent, newContent string) []op.ThreadReviewLineRange {
	return reviewChangedRangesFromHunks(generateReviewHunks(oldContent, newContent))
}

func reviewChangedRangesFromHunks(hunks []op.ThreadReviewHunk) []op.ThreadReviewLineRange {
	if len(hunks) == 0 {
		return nil
	}
	ranges := make([]op.ThreadReviewLineRange, 0, len(hunks))
	for _, hunk := range hunks {
		if hunk.NewLineCount <= 0 {
			continue
		}
		ranges = append(ranges, op.ThreadReviewLineRange{
			StartLine: hunk.NewStartLine,
			EndLine:   hunk.NewStartLine + hunk.NewLineCount - 1,
		})
	}
	return ranges
}

func generateReviewHunks(oldContent, newContent string) []op.ThreadReviewHunk {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	if len(newLines) == 0 {
		return nil
	}

	const maxReviewDiffMatrix = 1_200_000
	if len(oldLines)*len(newLines) > maxReviewDiffMatrix {
		return generateReviewCoarseHunk(oldLines, newLines)
	}

	dp := make([][]int, len(oldLines)+1)
	for i := range dp {
		dp[i] = make([]int, len(newLines)+1)
	}

	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	hunks := make([]op.ThreadReviewHunk, 0, 4)
	var current *op.ThreadReviewHunk
	flush := func() {
		if current == nil {
			return
		}
		current.OldLineCount = len(current.RemovedLines)
		current.NewLineCount = len(current.AddedLines)
		hunks = append(hunks, *current)
		current = nil
	}
	ensureHunk := func(i, j int) *op.ThreadReviewHunk {
		if current == nil {
			current = &op.ThreadReviewHunk{
				OldStartLine: i + 1,
				NewStartLine: j + 1,
			}
		}
		return current
	}

	for i, j := 0, 0; i < len(oldLines) || j < len(newLines); {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			flush()
			i++
			j++
			continue
		}
		if j >= len(newLines) || (i < len(oldLines) && dp[i+1][j] >= dp[i][j+1]) {
			hunk := ensureHunk(i, j)
			hunk.RemovedLines = append(hunk.RemovedLines, oldLines[i])
			i++
		} else {
			hunk := ensureHunk(i, j)
			hunk.AddedLines = append(hunk.AddedLines, newLines[j])
			j++
		}
	}
	flush()
	return hunks
}

func generateReviewCoarseHunk(oldLines, newLines []string) []op.ThreadReviewHunk {
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix {
		oldLine := oldLines[len(oldLines)-1-suffix]
		newLine := newLines[len(newLines)-1-suffix]
		if oldLine != newLine {
			break
		}
		suffix++
	}

	oldEnd := len(oldLines) - suffix
	newEnd := len(newLines) - suffix
	removed := append([]string(nil), oldLines[prefix:oldEnd]...)
	added := append([]string(nil), newLines[prefix:newEnd]...)
	if len(removed) == 0 && len(added) == 0 {
		return nil
	}
	return []op.ThreadReviewHunk{{
		OldStartLine: prefix + 1,
		OldLineCount: len(removed),
		NewStartLine: prefix + 1,
		NewLineCount: len(added),
		RemovedLines: removed,
		AddedLines:   added,
	}}
}

func generateReviewDiff(oldContent, newContent string) (string, int) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix {
		oldLine := oldLines[len(oldLines)-1-suffix]
		newLine := newLines[len(newLines)-1-suffix]
		if oldLine != newLine {
			break
		}
		suffix++
	}

	oldEnd := len(oldLines) - suffix
	newEnd := len(newLines) - suffix
	removed := oldLines[prefix:oldEnd]
	added := newLines[prefix:newEnd]

	if len(removed) == 0 && len(added) == 0 {
		return "", 0
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "@@ -%d,%d +%d,%d @@\n", prefix+1, len(removed), prefix+1, len(added))
	for _, line := range removed {
		builder.WriteString("-")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	for _, line := range added {
		builder.WriteString("+")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	return strings.TrimSuffix(builder.String(), "\n"), prefix + 1
}

func readFileMaybe(path string) (string, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(raw), true, nil
}

func writeFileAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".review-tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func resolveReviewPath(filePath string, cwd string) (string, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(filePath, "@"))
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Clean(home), nil
	}
	if strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Clean(filepath.Join(home, trimmed[2:])), nil
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed), nil
	}
	if strings.TrimSpace(cwd) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		cwd = wd
	}
	return filepath.Clean(filepath.Join(cwd, trimmed)), nil
}

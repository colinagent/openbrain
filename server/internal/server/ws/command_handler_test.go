package ws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCommandExecTargetCreatesTempLogPath(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace root: %v", err)
	}

	h := &Handler{cmd: newCommandManager()}
	resolvedWorkspaceRoot, targetPath, created, err := h.resolveCommandExecTarget(
		workspaceRoot,
		"",
		"echo hello world",
	)
	if err != nil {
		t.Fatalf("resolveCommandExecTarget(): %v", err)
	}
	if !created {
		t.Fatalf("created = false, want true")
	}
	if resolvedWorkspaceRoot != workspaceRoot {
		t.Fatalf("workspaceRoot = %q, want %q", resolvedWorkspaceRoot, workspaceRoot)
	}
	wantDir := filepath.Join(workspaceRoot, "temp")
	if filepath.Dir(targetPath) != wantDir {
		t.Fatalf("target dir = %q, want %q", filepath.Dir(targetPath), wantDir)
	}
	if filepath.Base(targetPath) != "echo-hello-world.md" {
		t.Fatalf("target file = %q, want %q", filepath.Base(targetPath), "echo-hello-world.md")
	}
	if _, err := os.Stat(wantDir); err != nil {
		t.Fatalf("temp dir missing: %v", err)
	}
}

func TestResolveCommandExecTargetRejectsTargetOutsideTemp(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace root: %v", err)
	}

	h := &Handler{cmd: newCommandManager()}
	_, _, _, err := h.resolveCommandExecTarget(
		workspaceRoot,
		filepath.Join(workspaceRoot, "notes", "bad.md"),
		"pwd",
	)
	if err == nil || !strings.Contains(err.Error(), "targetPath must be inside") {
		t.Fatalf("resolveCommandExecTarget() error = %v, want targetPath validation error", err)
	}
}

func TestResolveCommandExecTargetAllowsWorkspaceOutsideDefaultContainer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "external", "repo")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace root: %v", err)
	}

	h := &Handler{cmd: newCommandManager()}
	resolvedWorkspaceRoot, targetPath, created, err := h.resolveCommandExecTarget(
		workspaceRoot,
		"",
		"whoami",
	)
	if err != nil {
		t.Fatalf("resolveCommandExecTarget(): %v", err)
	}
	if !created {
		t.Fatalf("created = false, want true")
	}
	if resolvedWorkspaceRoot != workspaceRoot {
		t.Fatalf("workspaceRoot = %q, want %q", resolvedWorkspaceRoot, workspaceRoot)
	}
	if filepath.Dir(targetPath) != filepath.Join(workspaceRoot, "temp") {
		t.Fatalf("target dir = %q, want %q", filepath.Dir(targetPath), filepath.Join(workspaceRoot, "temp"))
	}
}

func TestBuildCommandMarkdownBlockTruncatesLargeOutput(t *testing.T) {
	t.Parallel()

	output := strings.Repeat("a", commandMaxMarkdownBytes+4096) + "```"
	block := buildCommandMarkdownBlock(
		"printf x",
		output,
		commandStateFinished,
		intPtr(0),
		"/tmp/demo.md",
		"/tmp/demo.full.log",
		true,
		"",
		"cmd-1",
	)

	if len(block) > commandMaxMarkdownBytes {
		t.Fatalf("block size = %d, want <= %d", len(block), commandMaxMarkdownBytes)
	}
	if !strings.Contains(block, "# output truncated in markdown") {
		t.Fatalf("expected truncation marker, got %q", block)
	}
	lastLineStart := strings.LastIndex(block, "\n")
	if lastLineStart < 0 {
		t.Fatalf("missing closing fence")
	}
	lastLine := block[lastLineStart+1:]
	if len(lastLine) < 3 || strings.Trim(lastLine, "`") != "" {
		t.Fatalf("closing fence malformed: %q", lastLine)
	}
}

func TestAppendCommandMarkdownAppendsBlocks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "temp", "demo.md")
	if err := appendCommandMarkdown(path, "```sh\n$ first\n# exit_code: 0\n```"); err != nil {
		t.Fatalf("append first block: %v", err)
	}
	if err := appendCommandMarkdown(path, "```sh\n$ second\n# exit_code: 0\n```"); err != nil {
		t.Fatalf("append second block: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "$ first") || !strings.Contains(text, "$ second") {
		t.Fatalf("expected both command blocks, got %q", text)
	}
	if !strings.Contains(text, "```\n\n```sh\n$ second") {
		t.Fatalf("expected blank-line-separated append, got %q", text)
	}
}

func intPtr(v int) *int {
	return &v
}

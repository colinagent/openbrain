package remotecontrol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
	"github.com/colinagent/openbrain/server/internal/rgsearch"
)

type remoteFileTestFixture struct {
	workspace   string
	workspaceID string
	service     *remoteFileService
	principal   Principal
}

func newRemoteFileTestFixture(t *testing.T) remoteFileTestFixture {
	t.Helper()
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	runtimeView := &fakeRuntimeView{
		system: &op.SystemConfigResult{
			SystemConfig:     op.SystemConfig{HostID: "host-file-test", BaseDir: t.TempDir()},
			DefaultWorkspace: workspace,
		},
	}
	workspaceAccess, err := defaultWorkspace(context.Background(), runtimeView)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := NewPrincipal("user-a", "environment-a", "client-a", protocol.CapabilityFileRead)
	if err != nil {
		t.Fatal(err)
	}
	return remoteFileTestFixture{
		workspace: workspace, workspaceID: workspaceAccess.ID, principal: principal,
		service: &remoteFileService{runtime: runtimeView, now: time.Now, handles: make(map[string]previewHandle)},
	}
}

func TestNormalizeRemoteRelativePathRejectsTraversalAndSensitivePaths(t *testing.T) {
	denied := map[string]protocol.ErrorCode{
		"/etc/passwd":       protocol.ErrorPathOutsideWorkspace,
		"C:/Users/a.txt":    protocol.ErrorPathOutsideWorkspace,
		`..\secret`:         protocol.ErrorPathOutsideWorkspace,
		"../secret":         protocol.ErrorPathOutsideWorkspace,
		"a/../secret":       protocol.ErrorPathOutsideWorkspace,
		"%2e%2e/secret":     protocol.ErrorPathOutsideWorkspace,
		"a/%2F/secret":      protocol.ErrorPathOutsideWorkspace,
		"a//secret":         protocol.ErrorPathOutsideWorkspace,
		".env":              protocol.ErrorSensitivePathDenied,
		"config/.ENV.local": protocol.ErrorSensitivePathDenied,
		".git/config":       protocol.ErrorSensitivePathDenied,
		"nested/.agent/log": protocol.ErrorSensitivePathDenied,
		"private.pem":       protocol.ErrorSensitivePathDenied,
		"ID_ED25519":        protocol.ErrorSensitivePathDenied,
		"credentials.json":  protocol.ErrorSensitivePathDenied,
	}
	for value, want := range denied {
		t.Run(value, func(t *testing.T) {
			_, remoteErr := normalizeRemoteRelativePath(value)
			if remoteErr == nil || remoteErr.Code != want {
				t.Fatalf("normalizeRemoteRelativePath(%q) error = %+v, want %q", value, remoteErr, want)
			}
		})
	}
	for _, value := range []string{"", "README.md", "src/main.go", ".gitignore", "docs/credentials-guide.md"} {
		if normalized, remoteErr := normalizeRemoteRelativePath(value); remoteErr != nil || normalized != value {
			t.Fatalf("normalizeRemoteRelativePath(%q) = %q, %+v", value, normalized, remoteErr)
		}
	}
}

func TestRemoteFileServiceContainsSymlinksAndHidesSensitiveEntries(t *testing.T) {
	fixture := newRemoteFileTestFixture(t)
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, "README.md"), "public")
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, ".env.local"), "secret")
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, "server.pem"), "secret")
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, ".git", "config"), "secret")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeRemoteTestFile(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(fixture.workspace, "escape.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	for _, relative := range []string{".env.local", "server.pem", ".git/config"} {
		if _, remoteErr := fixture.service.resolve(context.Background(), fixture.workspaceID, relative); remoteErr == nil || remoteErr.Code != protocol.ErrorSensitivePathDenied {
			t.Fatalf("resolve(%q) error = %+v", relative, remoteErr)
		}
	}
	if _, remoteErr := fixture.service.resolve(context.Background(), fixture.workspaceID, "escape.txt"); remoteErr == nil || remoteErr.Code != protocol.ErrorPathOutsideWorkspace {
		t.Fatalf("escaped symlink error = %+v", remoteErr)
	}

	payload, remoteErr := fixture.service.list(context.Background(), fixture.principal, mustRemoteJSON(t, fileListInput{
		WorkspaceID: fixture.workspaceID,
	}))
	if remoteErr != nil {
		t.Fatal(remoteErr)
	}
	text := string(payload)
	if !strings.Contains(text, "README.md") {
		t.Fatalf("listing omitted public file: %s", text)
	}
	for _, forbidden := range []string{".env", "server.pem", ".git", "escape.txt", "outside"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("listing exposed %q: %s", forbidden, text)
		}
	}
}

func TestRemoteFileListPaginatesWithPathBoundCursor(t *testing.T) {
	fixture := newRemoteFileTestFixture(t)
	for _, name := range []string{"c.txt", "a.txt", "b.txt"} {
		writeRemoteTestFile(t, filepath.Join(fixture.workspace, name), name)
	}
	firstRaw, remoteErr := fixture.service.list(context.Background(), fixture.principal, mustRemoteJSON(t, fileListInput{
		WorkspaceID: fixture.workspaceID, Limit: 2,
	}))
	if remoteErr != nil {
		t.Fatal(remoteErr)
	}
	var first struct {
		Entries    []remoteFileEntry `json:"entries"`
		NextCursor string            `json:"nextCursor"`
	}
	if err := json.Unmarshal(firstRaw, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 2 || first.Entries[0].Name != "a.txt" || first.Entries[1].Name != "b.txt" || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	secondRaw, remoteErr := fixture.service.list(context.Background(), fixture.principal, mustRemoteJSON(t, fileListInput{
		WorkspaceID: fixture.workspaceID, Cursor: first.NextCursor, Limit: 2,
	}))
	if remoteErr != nil {
		t.Fatal(remoteErr)
	}
	var second struct {
		Entries    []remoteFileEntry `json:"entries"`
		NextCursor string            `json:"nextCursor"`
	}
	if err := json.Unmarshal(secondRaw, &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 1 || second.Entries[0].Name != "c.txt" || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}

	tampered, err := encodeFileCursor(fileCursor{Path: "other", Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, remoteErr := fixture.service.list(context.Background(), fixture.principal, mustRemoteJSON(t, fileListInput{
		WorkspaceID: fixture.workspaceID, Cursor: tampered,
	})); remoteErr == nil || remoteErr.Code != protocol.ErrorInvalidEnvelope {
		t.Fatalf("tampered cursor error = %+v", remoteErr)
	}
}

func TestRemotePreviewIsBoundExpiresAndInvalidatesOnChange(t *testing.T) {
	fixture := newRemoteFileTestFixture(t)
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	fixture.service.now = func() time.Time { return now }
	filePath := filepath.Join(fixture.workspace, "notes.md")
	writeRemoteTestFile(t, filePath, "# Notes\nhello")

	openedRaw, remoteErr := fixture.service.previewOpen(context.Background(), fixture.principal, mustRemoteJSON(t, filePreviewOpenInput{
		WorkspaceID: fixture.workspaceID, Path: "notes.md",
	}))
	if remoteErr != nil {
		t.Fatal(remoteErr)
	}
	var opened struct {
		HandleID   string `json:"handleID"`
		InlineText string `json:"inlineText"`
	}
	if err := json.Unmarshal(openedRaw, &opened); err != nil {
		t.Fatal(err)
	}
	if opened.HandleID == "" || opened.InlineText != "# Notes\nhello" {
		t.Fatalf("opened preview = %+v", opened)
	}

	chunkRaw, remoteErr := fixture.service.previewChunk(context.Background(), fixture.principal, mustRemoteJSON(t, filePreviewChunkInput{
		WorkspaceID: fixture.workspaceID, HandleID: opened.HandleID, Offset: 0, Limit: 5,
	}))
	if remoteErr != nil {
		t.Fatal(remoteErr)
	}
	var chunk struct {
		DataBase64 string `json:"dataBase64"`
		NextOffset int64  `json:"nextOffset"`
		EOF        bool   `json:"eof"`
	}
	if err := json.Unmarshal(chunkRaw, &chunk); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if err != nil || string(decoded) != "# Not" || chunk.NextOffset != 5 || chunk.EOF {
		t.Fatalf("chunk = %+v, decoded = %q, err = %v", chunk, decoded, err)
	}

	otherPrincipal, err := NewPrincipal("user-a", "environment-a", "client-b", protocol.CapabilityFileRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, remoteErr := fixture.service.previewChunk(context.Background(), otherPrincipal, mustRemoteJSON(t, filePreviewChunkInput{
		WorkspaceID: fixture.workspaceID, HandleID: opened.HandleID,
	})); remoteErr == nil || remoteErr.Code != protocol.ErrorPreviewExpired {
		t.Fatalf("cross-client preview error = %+v", remoteErr)
	}

	now = now.Add(previewHandleTTL)
	if _, remoteErr := fixture.service.previewChunk(context.Background(), fixture.principal, mustRemoteJSON(t, filePreviewChunkInput{
		WorkspaceID: fixture.workspaceID, HandleID: opened.HandleID,
	})); remoteErr == nil || remoteErr.Code != protocol.ErrorPreviewExpired {
		t.Fatalf("expired preview error = %+v", remoteErr)
	}

	now = now.Add(time.Second)
	openedRaw, remoteErr = fixture.service.previewOpen(context.Background(), fixture.principal, mustRemoteJSON(t, filePreviewOpenInput{
		WorkspaceID: fixture.workspaceID, Path: "notes.md",
	}))
	if remoteErr != nil {
		t.Fatal(remoteErr)
	}
	if err := json.Unmarshal(openedRaw, &opened); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	writeRemoteTestFile(t, filePath, "changed")
	if _, remoteErr := fixture.service.previewChunk(context.Background(), fixture.principal, mustRemoteJSON(t, filePreviewChunkInput{
		WorkspaceID: fixture.workspaceID, HandleID: opened.HandleID,
	})); remoteErr == nil || remoteErr.Code != protocol.ErrorPreviewExpired {
		t.Fatalf("changed preview error = %+v", remoteErr)
	}
}

func TestRemotePreviewRejectsOversizedFile(t *testing.T) {
	fixture := newRemoteFileTestFixture(t)
	file, err := os.Create(filepath.Join(fixture.workspace, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxQuickLookBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, remoteErr := fixture.service.previewOpen(context.Background(), fixture.principal, mustRemoteJSON(t, filePreviewOpenInput{
		WorkspaceID: fixture.workspaceID, Path: "large.bin",
	})); remoteErr == nil || remoteErr.Code != protocol.ErrorFileTooLarge {
		t.Fatalf("oversized preview error = %+v", remoteErr)
	}
}

func TestRemotePreviewRejectsNonRegularFile(t *testing.T) {
	fixture := newRemoteFileTestFixture(t)
	socketPath := filepath.Join(fixture.workspace, "preview.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	if _, remoteErr := fixture.service.previewOpen(context.Background(), fixture.principal, mustRemoteJSON(t, filePreviewOpenInput{
		WorkspaceID: fixture.workspaceID, Path: "preview.sock",
	})); remoteErr == nil || remoteErr.Code != protocol.ErrorOperationDenied {
		t.Fatalf("non-regular preview error = %+v", remoteErr)
	}
}

func TestRemoteFileSearchUsesLiteralQueryAndExcludesSecrets(t *testing.T) {
	if _, err := rgsearch.ResolveBinary("rg"); err != nil {
		t.Skip("ripgrep is unavailable")
	}
	fixture := newRemoteFileTestFixture(t)
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, "public.txt"), "needle [literal]")
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, "credentials.json"), "needle [literal]")
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, ".git", "config"), "needle [literal]")
	payload, remoteErr := fixture.service.search(context.Background(), fixture.principal, mustRemoteJSON(t, fileSearchInput{
		WorkspaceID: fixture.workspaceID, Query: "needle [literal]", Limit: 500,
	}))
	if remoteErr != nil {
		t.Fatal(remoteErr)
	}
	text := string(payload)
	if !strings.Contains(text, "public.txt") {
		t.Fatalf("search omitted public match: %s", text)
	}
	if strings.Contains(text, "credentials") || strings.Contains(text, ".git") {
		t.Fatalf("search exposed sensitive match: %s", text)
	}
}

func TestRemoteFileSearchReturnsStableCancellation(t *testing.T) {
	if _, err := rgsearch.ResolveBinary("rg"); err != nil {
		t.Skip("ripgrep is unavailable")
	}
	fixture := newRemoteFileTestFixture(t)
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, "public.txt"), "needle")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, remoteErr := fixture.service.search(ctx, fixture.principal, mustRemoteJSON(t, fileSearchInput{
		WorkspaceID: fixture.workspaceID, Query: "needle",
	}))
	if remoteErr == nil || remoteErr.Code != protocol.ErrorOperationDenied || remoteErr.Message != "file search was canceled" {
		t.Fatalf("canceled search error = %+v", remoteErr)
	}
}

func TestRemoteFileHandlersRequireCapabilityAndReplayPreviewOpen(t *testing.T) {
	fixture := newRemoteFileTestFixture(t)
	writeRemoteTestFile(t, filepath.Join(fixture.workspace, "public.txt"), "preview")
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	if err := RegisterFileHandlers(dispatcher, fixture.service.runtime); err != nil {
		t.Fatal(err)
	}
	request := protocol.Envelope{
		ProtocolVersion: protocol.CurrentProtocolVersion,
		Type:            protocol.EnvelopeTypeRequest,
		ClientID:        fixture.principal.ClientID,
		StreamID:        "files",
		SeqID:           1,
		RequestID:       "preview-request-a",
		Operation:       protocol.OperationFilePreviewOpen,
		Payload: mustRemoteJSON(t, filePreviewOpenInput{
			WorkspaceID: fixture.workspaceID, Path: "public.txt",
		}),
	}
	withoutFileRead, err := NewPrincipal("user-a", "environment-a", "client-a", protocol.CapabilityEnvironmentRead)
	if err != nil {
		t.Fatal(err)
	}
	if response := dispatcher.Dispatch(context.Background(), withoutFileRead, request); response.Error == nil || response.Error.Code != protocol.ErrorCapabilityUnavailable {
		t.Fatalf("missing-capability response = %+v", response.Error)
	}
	first := dispatcher.Dispatch(context.Background(), fixture.principal, request)
	second := dispatcher.Dispatch(context.Background(), fixture.principal, request)
	if first.Error != nil || second.Error != nil {
		t.Fatalf("preview responses = %+v, %+v", first.Error, second.Error)
	}
	var firstPreview, secondPreview struct {
		HandleID string `json:"handleID"`
	}
	if err := json.Unmarshal(first.Payload, &firstPreview); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Payload, &secondPreview); err != nil {
		t.Fatal(err)
	}
	if firstPreview.HandleID == "" || firstPreview.HandleID != secondPreview.HandleID {
		t.Fatalf("replayed preview handles = %q, %q", firstPreview.HandleID, secondPreview.HandleID)
	}
}

func writeRemoteTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustRemoteJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

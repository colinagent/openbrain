package treeimport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubGrantResolver struct {
	roots []string
	err   error
}

func (s stubGrantResolver) ResolveGrant(_ string, _ string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.roots, nil
}

func setupTreeImportRouter(t *testing.T) (*gin.Engine, *Service, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	baseDir := t.TempDir()
	workspaceDir := filepath.Join(baseDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	service := NewService(baseDir, stubGrantResolver{roots: []string{workspaceDir}})
	handler := NewHandler(service)
	router := gin.New()
	router.POST("/v1/tree-import/sessions", handler.CreateSession)
	router.PUT("/v1/tree-import/sessions/:sessionId/files/*relativePath", handler.UploadFile)
	router.POST("/v1/tree-import/sessions/:sessionId/commit", handler.CommitSession)
	router.DELETE("/v1/tree-import/sessions/:sessionId", handler.CancelSession)
	return router, service, workspaceDir
}

func int64Ptr(value int64) *int64 {
	return &value
}

func TestCreateSessionRejectsInvalidManifest(t *testing.T) {
	router, _, workspaceDir := setupTreeImportRouter(t)
	body := CreateTreeImportSessionRequest{
		TargetDir:  workspaceDir,
		GrantToken: "grant",
		Entries: []TreeImportEntry{
			{Kind: EntryKindFile, RelativePath: "../bad.txt", Size: int64Ptr(1)},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateSessionRejectsUnauthorizedTarget(t *testing.T) {
	router, _, _ := setupTreeImportRouter(t)
	body := CreateTreeImportSessionRequest{
		TargetDir:  "/tmp/outside",
		GrantToken: "grant",
		Entries: []TreeImportEntry{
			{Kind: EntryKindFile, RelativePath: "a.txt", Size: int64Ptr(1)},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestImportSingleFileLifecycle(t *testing.T) {
	router, _, workspaceDir := setupTreeImportRouter(t)
	body := CreateTreeImportSessionRequest{
		TargetDir:  workspaceDir,
		GrantToken: "grant",
		Entries: []TreeImportEntry{
			{Kind: EntryKindFile, RelativePath: "demo.txt", Size: int64Ptr(5)},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create session: %d %s", rr.Code, rr.Body.String())
	}

	var created CreateTreeImportSessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if len(created.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", created.Conflicts)
	}

	uploadReq := httptest.NewRequest(http.MethodPut, created.UploadBaseURL+"/demo.txt", bytes.NewReader([]byte("hello")))
	uploadRR := httptest.NewRecorder()
	router.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", uploadRR.Code, uploadRR.Body.String())
	}

	commitRaw, _ := json.Marshal(CommitTreeImportRequest{Overwrite: false})
	commitReq := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions/"+created.SessionID+"/commit", bytes.NewReader(commitRaw))
	commitReq.Header.Set("Content-Type", "application/json")
	commitRR := httptest.NewRecorder()
	router.ServeHTTP(commitRR, commitReq)
	if commitRR.Code != http.StatusOK {
		t.Fatalf("commit: %d %s", commitRR.Code, commitRR.Body.String())
	}

	targetPath := filepath.Join(workspaceDir, "demo.txt")
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected content %q", string(content))
	}
}

func TestImportNestedDirectoryAndEmptyDirectory(t *testing.T) {
	router, _, workspaceDir := setupTreeImportRouter(t)
	body := CreateTreeImportSessionRequest{
		TargetDir:  workspaceDir,
		GrantToken: "grant",
		Entries: []TreeImportEntry{
			{Kind: EntryKindDir, RelativePath: "folder"},
			{Kind: EntryKindDir, RelativePath: "folder/empty"},
			{Kind: EntryKindFile, RelativePath: "folder/nested.txt", Size: int64Ptr(6)},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create session: %d %s", rr.Code, rr.Body.String())
	}

	var created CreateTreeImportSessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPut, created.UploadBaseURL+"/folder/nested.txt", bytes.NewReader([]byte("nested")))
	uploadRR := httptest.NewRecorder()
	router.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", uploadRR.Code, uploadRR.Body.String())
	}

	commitRaw, _ := json.Marshal(CommitTreeImportRequest{Overwrite: false})
	commitReq := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions/"+created.SessionID+"/commit", bytes.NewReader(commitRaw))
	commitReq.Header.Set("Content-Type", "application/json")
	commitRR := httptest.NewRecorder()
	router.ServeHTTP(commitRR, commitReq)
	if commitRR.Code != http.StatusOK {
		t.Fatalf("commit: %d %s", commitRR.Code, commitRR.Body.String())
	}

	if info, err := os.Stat(filepath.Join(workspaceDir, "folder", "empty")); err != nil || !info.IsDir() {
		t.Fatalf("expected empty directory, err=%v info=%v", err, info)
	}
}

func TestCommitRejectsConflictsWithoutOverwrite(t *testing.T) {
	router, _, workspaceDir := setupTreeImportRouter(t)
	if err := os.WriteFile(filepath.Join(workspaceDir, "demo.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	body := CreateTreeImportSessionRequest{
		TargetDir:  workspaceDir,
		GrantToken: "grant",
		Entries: []TreeImportEntry{
			{Kind: EntryKindFile, RelativePath: "demo.txt", Size: int64Ptr(5)},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create session: %d %s", rr.Code, rr.Body.String())
	}

	var created CreateTreeImportSessionResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	if len(created.Conflicts) != 1 || created.Conflicts[0] != "demo.txt" {
		t.Fatalf("unexpected conflicts: %v", created.Conflicts)
	}

	uploadReq := httptest.NewRequest(http.MethodPut, created.UploadBaseURL+"/demo.txt", bytes.NewReader([]byte("hello")))
	uploadRR := httptest.NewRecorder()
	router.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", uploadRR.Code, uploadRR.Body.String())
	}

	commitRaw, _ := json.Marshal(CommitTreeImportRequest{Overwrite: false})
	commitReq := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions/"+created.SessionID+"/commit", bytes.NewReader(commitRaw))
	commitReq.Header.Set("Content-Type", "application/json")
	commitRR := httptest.NewRecorder()
	router.ServeHTTP(commitRR, commitReq)
	if commitRR.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", commitRR.Code, commitRR.Body.String())
	}
}

func TestCommitOverwritesFilesAndMergesDirectories(t *testing.T) {
	router, _, workspaceDir := setupTreeImportRouter(t)
	if err := os.MkdirAll(filepath.Join(workspaceDir, "folder"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "folder", "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "folder", "overwrite.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	body := CreateTreeImportSessionRequest{
		TargetDir:  workspaceDir,
		GrantToken: "grant",
		Entries: []TreeImportEntry{
			{Kind: EntryKindDir, RelativePath: "folder"},
			{Kind: EntryKindFile, RelativePath: "folder/overwrite.txt", Size: int64Ptr(3)},
			{Kind: EntryKindFile, RelativePath: "folder/new.txt", Size: int64Ptr(3)},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create session: %d %s", rr.Code, rr.Body.String())
	}

	var created CreateTreeImportSessionResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &created)

	for _, upload := range []struct {
		path    string
		content string
	}{
		{path: "folder/overwrite.txt", content: "new"},
		{path: "folder/new.txt", content: "add"},
	} {
		uploadReq := httptest.NewRequest(http.MethodPut, created.UploadBaseURL+"/"+upload.path, bytes.NewReader([]byte(upload.content)))
		uploadRR := httptest.NewRecorder()
		router.ServeHTTP(uploadRR, uploadReq)
		if uploadRR.Code != http.StatusOK {
			t.Fatalf("upload %s: %d %s", upload.path, uploadRR.Code, uploadRR.Body.String())
		}
	}

	commitRaw, _ := json.Marshal(CommitTreeImportRequest{Overwrite: true})
	commitReq := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions/"+created.SessionID+"/commit", bytes.NewReader(commitRaw))
	commitReq.Header.Set("Content-Type", "application/json")
	commitRR := httptest.NewRecorder()
	router.ServeHTTP(commitRR, commitReq)
	if commitRR.Code != http.StatusOK {
		t.Fatalf("commit: %d %s", commitRR.Code, commitRR.Body.String())
	}

	if content, err := os.ReadFile(filepath.Join(workspaceDir, "folder", "overwrite.txt")); err != nil || string(content) != "new" {
		t.Fatalf("unexpected overwrite file: %q err=%v", string(content), err)
	}
	if content, err := os.ReadFile(filepath.Join(workspaceDir, "folder", "keep.txt")); err != nil || string(content) != "keep" {
		t.Fatalf("unexpected keep file: %q err=%v", string(content), err)
	}
}

func TestCancelDeletesSession(t *testing.T) {
	router, _, workspaceDir := setupTreeImportRouter(t)
	body := CreateTreeImportSessionRequest{
		TargetDir:  workspaceDir,
		GrantToken: "grant",
		Entries: []TreeImportEntry{
			{Kind: EntryKindFile, RelativePath: "demo.txt", Size: int64Ptr(5)},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create session: %d %s", rr.Code, rr.Body.String())
	}

	var created CreateTreeImportSessionResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &created)

	cancelReq := httptest.NewRequest(http.MethodDelete, "/v1/tree-import/sessions/"+created.SessionID, nil)
	cancelRR := httptest.NewRecorder()
	router.ServeHTTP(cancelRR, cancelReq)
	if cancelRR.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", cancelRR.Code, cancelRR.Body.String())
	}

	commitRaw, _ := json.Marshal(CommitTreeImportRequest{Overwrite: false})
	commitReq := httptest.NewRequest(http.MethodPost, "/v1/tree-import/sessions/"+created.SessionID+"/commit", bytes.NewReader(commitRaw))
	commitReq.Header.Set("Content-Type", "application/json")
	commitRR := httptest.NewRecorder()
	router.ServeHTTP(commitRR, commitReq)
	if commitRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", commitRR.Code, commitRR.Body.String())
	}
}

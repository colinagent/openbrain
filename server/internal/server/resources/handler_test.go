package resources

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/gin-gonic/gin"
)

type stubMetaResolver struct {
	meta *op.ThreadMeta
	err  error
}

func (s stubMetaResolver) GetThreadMeta(_ context.Context, query op.ThreadMetaQuery) (*op.ThreadMeta, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.meta == nil {
		return nil, os.ErrNotExist
	}
	if query.ChatPath != "" && query.ChatPath != s.meta.ChatPath {
		return nil, os.ErrNotExist
	}
	return s.meta, nil
}

func setupResourceRouter(t *testing.T) (*gin.Engine, *Service, string, string, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	baseDir := t.TempDir()
	chatPath := filepath.Join(baseDir, "workspace", "demo", ".agent", "chat", "hello.md")
	if err := os.MkdirAll(filepath.Dir(chatPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(chatPath, []byte("seed"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	service := NewService(baseDir, stubMetaResolver{meta: &op.ThreadMeta{
		ThreadID: "thread-test",
		ChatPath: chatPath,
	}})
	h := NewHandler(service)
	r := gin.New()
	r.POST("/v1/resources/grants", h.CreateGrant)
	r.POST("/v1/resources/inspect", h.Inspect)
	r.POST("/v1/resources/handle", h.CreateHandle)
	r.GET("/v1/resources/content/:handleId", h.GetContent)
	r.GET("/v1/resources/content/:handleId/*resourcePath", h.GetPackageContent)
	r.POST("/v1/resources/import-sessions", h.CreateImportSession)
	r.PUT("/v1/resources/import-sessions/:sessionId/content", h.UploadImportSession)
	authority := "local"
	return r, service, chatPath, "opfs://" + authority + filepath.ToSlash(chatPath), filepath.Dir(filepath.Dir(filepath.Dir(chatPath))), authority
}

func createGrantToken(t *testing.T, router *gin.Engine, authority string, roots []string) string {
	t.Helper()
	body := CreateGrantRequest{Authority: authority, Roots: roots}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/grants", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create grant: %d %s", rr.Code, rr.Body.String())
	}
	var res CreateGrantResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode grant: %v", err)
	}
	return res.GrantToken
}

func createHandleResponse(t *testing.T, router *gin.Engine, target Target, intent string, authority string, roots []string) HandleResponse {
	t.Helper()
	body := InspectRequest{
		Target:     target,
		Intent:     intent,
		GrantToken: createGrantToken(t, router, authority, roots),
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/handle", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create handle: %d %s", rr.Code, rr.Body.String())
	}
	var res HandleResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	return res
}

func TestImportMarkdownImageAndRenderHandle(t *testing.T) {
	router, _, _, chatURI, rootDir, authority := setupResourceRouter(t)
	content := []byte("png")
	reqBody := CreateImportSessionRequest{
		Purpose:           ImportPurposeMarkdownImage,
		TargetDocumentURI: chatURI,
		FileName:          "hello.png",
		MIMEType:          "image/png",
		Size:              int64(len(content)),
		SHA256:            hex.EncodeToString(sha(content)),
		GrantToken:        createGrantToken(t, router, authority, []string{rootDir}),
	}
	raw, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/import-sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create import session: %d %s", rr.Code, rr.Body.String())
	}
	var created CreateImportSessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if !strings.HasPrefix(created.ExpectedDocumentRef, "./assets/") {
		t.Fatalf("unexpected document ref %q", created.ExpectedDocumentRef)
	}
	uploadReq := httptest.NewRequest(http.MethodPut, created.UploadURL, bytes.NewReader(content))
	uploadReq.Header.Set("Content-Type", "image/png")
	uploadRR := httptest.NewRecorder()
	router.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("upload session: %d %s", uploadRR.Code, uploadRR.Body.String())
	}
	var result ImportResult
	if err := json.Unmarshal(uploadRR.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if result.RenderHandle == nil || !strings.Contains(result.RenderHandle.URL, "/v1/resources/content/") {
		t.Fatalf("missing render handle: %+v", result)
	}
	contentReq := httptest.NewRequest(http.MethodGet, result.RenderHandle.URL, nil)
	contentReq.Header.Set("Range", "bytes=0-1")
	contentRR := httptest.NewRecorder()
	router.ServeHTTP(contentRR, contentReq)
	if contentRR.Code != http.StatusPartialContent {
		t.Fatalf("range content: %d %s", contentRR.Code, contentRR.Body.String())
	}
	if got := contentRR.Body.String(); got != "pn" {
		t.Fatalf("unexpected content %q", got)
	}
}

func TestInspectMissingFile(t *testing.T) {
	router, _, _, _, rootDir, authority := setupResourceRouter(t)
	body := InspectRequest{Target: Target{Kind: "file", URI: "opfs://" + authority + "/tmp/missing.png"}, Intent: "render", GrantToken: createGrantToken(t, router, authority, []string{rootDir})}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/inspect", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		var meta Meta
		_ = json.Unmarshal(rr.Body.Bytes(), &meta)
		if meta.Exists {
			t.Fatalf("expected missing file meta, got %+v", meta)
		}
		return
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d %s", rr.Code, rr.Body.String())
	}
}

func TestImportRejectsBadSHA(t *testing.T) {
	router, _, _, chatURI, rootDir, authority := setupResourceRouter(t)
	content := []byte("png")
	reqBody := CreateImportSessionRequest{
		Purpose:           ImportPurposeMarkdownImage,
		TargetDocumentURI: chatURI,
		FileName:          "hello.png",
		MIMEType:          "image/png",
		Size:              int64(len(content)),
		SHA256:            strings.Repeat("0", 64),
		GrantToken:        createGrantToken(t, router, authority, []string{rootDir}),
	}
	raw, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/import-sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create import session: %d %s", rr.Code, rr.Body.String())
	}
	var created CreateImportSessionResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	uploadReq := httptest.NewRequest(http.MethodPut, created.UploadURL, bytes.NewReader(content))
	uploadRR := httptest.NewRecorder()
	router.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d %s", uploadRR.Code, uploadRR.Body.String())
	}
}

func sha(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func TestInspectExistingFile(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	assets := filepath.Join(filepath.Dir(chatPath), "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(assets, "a.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := InspectRequest{Target: Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(imagePath)}, Intent: "render", GrantToken: createGrantToken(t, router, authority, []string{rootDir})}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/inspect", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inspect existing file: %d %s", rr.Code, rr.Body.String())
	}
}

func TestInspectPDFIsRenderable(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	assets := filepath.Join(filepath.Dir(chatPath), "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(assets, "doc.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := InspectRequest{Target: Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(pdfPath)}, Intent: "render", GrantToken: createGrantToken(t, router, authority, []string{rootDir})}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/inspect", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inspect pdf: %d %s", rr.Code, rr.Body.String())
	}
	var meta Meta
	if err := json.Unmarshal(rr.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if meta.MIMEType != "application/pdf" {
		t.Fatalf("MIMEType = %q, want application/pdf", meta.MIMEType)
	}
	if !meta.Renderable {
		t.Fatal("PDF should be renderable")
	}
}

func TestPDFRenderHandleIsInline(t *testing.T) {
	router, service, chatPath, _, rootDir, authority := setupResourceRouter(t)
	assets := filepath.Join(filepath.Dir(chatPath), "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(assets, "inline.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handle := createHandleResponse(t, router, Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(pdfPath)}, "render", authority, []string{rootDir})
	if handle.MIMEType != "application/pdf" {
		t.Fatalf("MIMEType = %q, want application/pdf", handle.MIMEType)
	}
	record, ok := service.GetHandle(handle.HandleID)
	if !ok {
		t.Fatal("expected PDF handle record")
	}
	if !record.Inline {
		t.Fatal("PDF render handle should be inline")
	}
}

func TestInspectZippedEPUBIsRenderable(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	assets := filepath.Join(filepath.Dir(chatPath), "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	epubPath := filepath.Join(assets, "book.epub")
	if err := os.WriteFile(epubPath, []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := InspectRequest{Target: Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(epubPath)}, Intent: "render", GrantToken: createGrantToken(t, router, authority, []string{rootDir})}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/inspect", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inspect epub: %d %s", rr.Code, rr.Body.String())
	}
	var meta Meta
	if err := json.Unmarshal(rr.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if meta.MIMEType != epubMIME {
		t.Fatalf("MIMEType = %q, want %s", meta.MIMEType, epubMIME)
	}
	if meta.EntryType != "file" {
		t.Fatalf("EntryType = %q, want file", meta.EntryType)
	}
	if !meta.Renderable {
		t.Fatal("EPUB should be renderable")
	}
}

func TestInspectUnpackedEPUBDirectoryIsRenderable(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	bookDir := filepath.Join(filepath.Dir(chatPath), "Fan.epub")
	if err := writeUnpackedEPUBFixture(bookDir); err != nil {
		t.Fatal(err)
	}
	body := InspectRequest{Target: Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(bookDir)}, Intent: "render", GrantToken: createGrantToken(t, router, authority, []string{rootDir})}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/inspect", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inspect unpacked epub: %d %s", rr.Code, rr.Body.String())
	}
	var meta Meta
	if err := json.Unmarshal(rr.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if meta.EntryType != "directory" {
		t.Fatalf("EntryType = %q, want directory", meta.EntryType)
	}
	if meta.EpubPackagePath != "content.opf" {
		t.Fatalf("EpubPackagePath = %q, want content.opf", meta.EpubPackagePath)
	}
	if !meta.Renderable {
		t.Fatal("unpacked EPUB should be renderable")
	}
	if meta.Downloadable {
		t.Fatal("unpacked EPUB directory should not be directly downloadable")
	}
}

func TestUnpackedEPUBHandleServesPackageResources(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	bookDir := filepath.Join(filepath.Dir(chatPath), "Fan.epub")
	if err := writeUnpackedEPUBFixture(bookDir); err != nil {
		t.Fatal(err)
	}
	handle := createHandleResponse(t, router, Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(bookDir)}, "render", authority, []string{rootDir})
	if handle.EntryType != "directory" {
		t.Fatalf("EntryType = %q, want directory", handle.EntryType)
	}
	if !strings.HasSuffix(handle.URL, "/") {
		t.Fatalf("directory handle URL should end with /, got %q", handle.URL)
	}
	contentReq := httptest.NewRequest(http.MethodGet, handle.URL+"chapters/chapter.html", nil)
	contentRR := httptest.NewRecorder()
	router.ServeHTTP(contentRR, contentReq)
	if contentRR.Code != http.StatusOK {
		t.Fatalf("package content: %d %s", contentRR.Code, contentRR.Body.String())
	}
	if got := contentRR.Body.String(); got != "<html><body>chapter</body></html>" {
		t.Fatalf("unexpected content %q", got)
	}
	if got := contentRR.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q, want html", got)
	}
}

func TestUnpackedEPUBHandleRejectsTraversal(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	bookDir := filepath.Join(filepath.Dir(chatPath), "Fan.epub")
	if err := writeUnpackedEPUBFixture(bookDir); err != nil {
		t.Fatal(err)
	}
	handle := createHandleResponse(t, router, Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(bookDir)}, "render", authority, []string{rootDir})
	contentReq := httptest.NewRequest(http.MethodGet, handle.URL+"../hello.md", nil)
	contentRR := httptest.NewRecorder()
	router.ServeHTTP(contentRR, contentReq)
	if contentRR.Code != http.StatusBadRequest {
		t.Fatalf("expected traversal rejection, got %d %s", contentRR.Code, contentRR.Body.String())
	}
}

func TestInvalidEPUBDirectoryIsNotRenderable(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	bookDir := filepath.Join(filepath.Dir(chatPath), "Broken.epub")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := InspectRequest{Target: Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(bookDir)}, Intent: "render", GrantToken: createGrantToken(t, router, authority, []string{rootDir})}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/inspect", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid package error, got %d %s", rr.Code, rr.Body.String())
	}
}

func writeUnpackedEPUBFixture(bookDir string) error {
	if err := os.MkdirAll(filepath.Join(bookDir, "META-INF"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(bookDir, "chapters"), 0o755); err != nil {
		return err
	}
	container := `<?xml version="1.0"?><container><rootfiles><rootfile full-path="content.opf"/></rootfiles></container>`
	if err := os.WriteFile(filepath.Join(bookDir, "META-INF", "container.xml"), []byte(container), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bookDir, "content.opf"), []byte(`<?xml version="1.0"?><package></package>`), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bookDir, "chapters", "chapter.html"), []byte("<html><body>chapter</body></html>"), 0o644)
}

func TestRenderHandleReusesStableHandleWithinSameRoots(t *testing.T) {
	router, service, chatPath, _, rootDir, authority := setupResourceRouter(t)
	assets := filepath.Join(filepath.Dir(chatPath), "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(assets, "stable.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(imagePath)}

	first := createHandleResponse(t, router, target, "render", authority, []string{rootDir})
	second := createHandleResponse(t, router, target, "render", authority, []string{rootDir})
	if first.HandleID != second.HandleID {
		t.Fatalf("expected stable handle reuse, got %q and %q", first.HandleID, second.HandleID)
	}
	if first.ExpiresAt != "" || second.ExpiresAt != "" {
		t.Fatalf("expected non-expiring render handle, got %q and %q", first.ExpiresAt, second.ExpiresAt)
	}

	record, ok := service.GetHandle(first.HandleID)
	if !ok {
		t.Fatalf("expected render handle record")
	}
	if !record.ExpiresAt.IsZero() {
		t.Fatalf("expected zero expiry for render handle, got %v", record.ExpiresAt)
	}

	contentReq := httptest.NewRequest(http.MethodGet, first.URL+"?v=123", nil)
	contentReq.Header.Set("Range", "bytes=0-1")
	contentRR := httptest.NewRecorder()
	router.ServeHTTP(contentRR, contentReq)
	if contentRR.Code != http.StatusPartialContent {
		t.Fatalf("render content: %d %s", contentRR.Code, contentRR.Body.String())
	}
	if got := contentRR.Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Fatalf("unexpected cache-control %q", got)
	}
	if got := contentRR.Header().Get("Content-Disposition"); got != "inline" {
		t.Fatalf("unexpected disposition %q", got)
	}
}

func TestRenderHandleDiffersAcrossRoots(t *testing.T) {
	router, _, chatPath, _, rootDir, authority := setupResourceRouter(t)
	assets := filepath.Join(filepath.Dir(chatPath), "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(assets, "stable.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(imagePath)}

	first := createHandleResponse(t, router, target, "render", authority, []string{rootDir})
	second := createHandleResponse(t, router, target, "render", authority, []string{assets})
	if first.HandleID == second.HandleID {
		t.Fatalf("expected different stable handles for different roots")
	}
}

func TestDownloadHandleStillExpires(t *testing.T) {
	router, service, chatPath, _, rootDir, authority := setupResourceRouter(t)
	assets := filepath.Join(filepath.Dir(chatPath), "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(assets, "download.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := Target{Kind: "file", URI: "opfs://" + authority + filepath.ToSlash(imagePath)}

	handle := createHandleResponse(t, router, target, "download", authority, []string{rootDir})
	if handle.ExpiresAt == "" {
		t.Fatalf("expected expiring download handle")
	}

	service.handleMu.Lock()
	record := service.handles[handle.HandleID]
	record.ExpiresAt = time.Now().Add(-time.Minute)
	service.handleMu.Unlock()

	if _, ok := service.GetHandle(handle.HandleID); ok {
		t.Fatalf("expected expired download handle to be rejected")
	}
}

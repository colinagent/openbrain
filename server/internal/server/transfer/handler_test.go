package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := NewService(t.TempDir())
	handler := NewHandler(service)
	router := gin.New()
	router.POST("/v1/transfers", handler.Create)
	router.PUT("/v1/transfers/:id/content", handler.PutContent)
	router.GET("/v1/transfers/:id/content", handler.GetContent)
	router.GET("/v1/transfers/:id/meta", handler.GetMeta)
	return router, service
}

func TestBinaryTransferLifecycle(t *testing.T) {
	router, _ := setupRouter(t)
	body := CreateRequest{
		Purpose:  PurposeBinary,
		FileName: "hello world.txt",
		MIMEType: "text/plain",
		Size:     3,
		SHA256:   hex.EncodeToString(sha256Sum([]byte("abc"))),
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create transfer: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created CreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.TransferID == "" || created.RelativePath != "" || !strings.Contains(created.DownloadURL, created.TransferID) {
		t.Fatalf("unexpected create response: %+v", created)
	}

	uploadReq := httptest.NewRequest(http.MethodPut, "/v1/transfers/"+created.TransferID+"/content", bytes.NewReader([]byte("abc")))
	uploadReq.Header.Set("Content-Type", "text/plain")
	uploadRR := httptest.NewRecorder()
	router.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("upload transfer: status=%d body=%s", uploadRR.Code, uploadRR.Body.String())
	}

	metaReq := httptest.NewRequest(http.MethodGet, "/v1/transfers/"+created.TransferID+"/meta", nil)
	metaRR := httptest.NewRecorder()
	router.ServeHTTP(metaRR, metaReq)
	if metaRR.Code != http.StatusOK {
		t.Fatalf("meta transfer: status=%d body=%s", metaRR.Code, metaRR.Body.String())
	}
	var meta Record
	if err := json.Unmarshal(metaRR.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if meta.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %+v", meta)
	}
	if _, err := os.Stat(meta.StoragePath); err != nil {
		t.Fatalf("stored file missing: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, created.DownloadURL, nil)
	getReq.Header.Set("Range", "bytes=0-1")
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusPartialContent {
		t.Fatalf("expected range download 206, got %d body=%s", getRR.Code, getRR.Body.String())
	}
	if got := getRR.Body.String(); got != "ab" {
		t.Fatalf("unexpected range body %q", got)
	}
}

func TestBinaryTransferRejectsBadSHA(t *testing.T) {
	router, _ := setupRouter(t)
	body := CreateRequest{
		Purpose:  PurposeBinary,
		FileName: "bad.txt",
		MIMEType: "text/plain",
		Size:     3,
		SHA256:   strings.Repeat("0", 64),
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create transfer: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created CreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPut, "/v1/transfers/"+created.TransferID+"/content", bytes.NewReader([]byte("abc")))
	uploadRR := httptest.NewRecorder()
	router.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", uploadRR.Code, uploadRR.Body.String())
	}
	metaReq := httptest.NewRequest(http.MethodGet, "/v1/transfers/"+created.TransferID+"/meta", nil)
	metaRR := httptest.NewRecorder()
	router.ServeHTTP(metaRR, metaReq)
	var meta Record
	if err := json.Unmarshal(metaRR.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if meta.Status != StatusFailed {
		t.Fatalf("expected failed status, got %+v", meta)
	}
	if _, err := os.Stat(meta.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("expected no stored file, stat err=%v", err)
	}
}

func TestTransferRejectsUnsupportedPurpose(t *testing.T) {
	router, _ := setupRouter(t)
	body := CreateRequest{
		Purpose:  Purpose("chat-image"),
		FileName: "bad.png",
		MIMEType: "image/png",
		Size:     3,
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestBinaryTransferRejectsDownloadBeforeCompletion(t *testing.T) {
	router, _ := setupRouter(t)
	body := CreateRequest{
		Purpose:  PurposeBinary,
		FileName: "pending.txt",
		MIMEType: "text/plain",
		Size:     3,
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create transfer: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created CreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	getReq := httptest.NewRequest(http.MethodGet, created.DownloadURL, nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", getRR.Code, getRR.Body.String())
	}
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

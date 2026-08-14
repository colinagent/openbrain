package guardian

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
)

func TestReviewSendsOnlyRedactedStructuredMetadata(t *testing.T) {
	var received reviewRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer session-secret" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&received); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(writer).Encode(reviewResponse{
			RequestID: received.RequestID, Decision: op.GuardianReviewRequireUser,
			Risk: op.GuardianRiskHigh, Reason: "needs a person", ReviewID: "review-1", PolicyVersion: "guardian-v1",
		})
	}))
	defer server.Close()

	baseDir := t.TempDir()
	writeAuth(t, baseDir, server.URL)
	workspace := filepath.Join(baseDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(baseDir, "private", "token.txt")
	result, err := NewClient(baseDir).Review(context.Background(), op.ApprovalRequest{
		RequestID: "request-1", ToolName: "shell", ProfileID: op.PermissionProfileApprove,
		CWD: workspace, CommandSummary: "curl -H 'Authorization: Bearer abc.secret' https://api.example.com",
		Reason: "password=hunter2 then inspect the requested file", RiskTags: []string{"external_path"},
		PolicyVersion: 1, PolicyDigest: "sha256:abc", AgentID: "agent-main", ParentAgentID: "agent-parent",
		ExistingThreadGrantCount: 2,
		Resources: op.ApprovalResourceScope{
			ReadPaths: []string{filepath.Join(workspace, "docs", "plan.md")}, WritePaths: []string{outside},
			NetworkDomains: []string{"https://API.Example.com/path"}, SensitiveRights: []string{"keychain:read"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != op.GuardianReviewRequireUser || result.ReviewID != "review-1" {
		t.Fatalf("result = %#v", result)
	}
	raw, _ := json.Marshal(received)
	text := string(raw)
	for _, forbidden := range []string{"abc.secret", "hunter2", outside} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("request leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "workspace:docs/plan.md") || !strings.Contains(text, "outside_workspace") || !strings.Contains(text, "api.example.com") {
		t.Fatalf("sanitized request = %s", text)
	}
	if received.ExistingThreadGrantCount != 2 {
		t.Fatalf("existing thread grant count = %d", received.ExistingThreadGrantCount)
	}
}

func TestGuardianEndpointDoesNotDuplicateV1(t *testing.T) {
	if got := guardianEndpoint("https://api.example.com/v1/"); got != "https://api.example.com/v1/security-reviews" {
		t.Fatalf("guardianEndpoint = %q", got)
	}
}

func TestReviewRejectsMalformedOrMismatchedResponses(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field": `{"requestID":"request-1","decision":"approve_once","risk":"low","reason":"ok","extra":true}`,
		"mismatch":      `{"requestID":"other","decision":"approve_once","risk":"low","reason":"ok"}`,
		"invalid enum":  `{"requestID":"request-1","decision":"always","risk":"low","reason":"ok"}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(body))
			}))
			defer server.Close()
			baseDir := t.TempDir()
			writeAuth(t, baseDir, server.URL)
			_, err := NewClient(baseDir).Review(context.Background(), op.ApprovalRequest{RequestID: "request-1", ToolName: "read", ProfileID: op.PermissionProfileApprove})
			if err == nil {
				t.Fatal("malformed response was accepted")
			}
		})
	}
}

func TestReviewRequiresLogin(t *testing.T) {
	_, err := NewClient(t.TempDir()).Review(context.Background(), op.ApprovalRequest{RequestID: "request-1"})
	if err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("Review error = %v", err)
	}
}

func TestReviewRejectsSymlinkAuthConfig(t *testing.T) {
	baseDir := t.TempDir()
	authPath := filepath.Join(baseDir, "configs", "user", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(baseDir, "auth-target.json")
	if err := os.WriteFile(target, []byte(`{"version":2,"aiGateway":"https://api.example.com","token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, authPath); err != nil {
		t.Fatal(err)
	}
	_, err := NewClient(baseDir).Review(context.Background(), op.ApprovalRequest{RequestID: "request-1"})
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Review error = %v", err)
	}
}

func TestReviewRejectsRedirects(t *testing.T) {
	redirectTargetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectTargetCalled = true
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	baseDir := t.TempDir()
	writeAuth(t, baseDir, redirector.URL)
	_, err := NewClient(baseDir).Review(context.Background(), op.ApprovalRequest{RequestID: "request-1", ToolName: "read", ProfileID: op.PermissionProfileApprove})
	if err == nil || redirectTargetCalled {
		t.Fatalf("redirect error = %v, target called = %v", err, redirectTargetCalled)
	}
}

func writeAuth(t *testing.T, baseDir, gateway string) {
	t.Helper()
	path := filepath.Join(baseDir, "configs", "user", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"version": 2, "aiGateway": gateway, "token": "session-secret"})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

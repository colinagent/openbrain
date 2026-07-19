package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestRuntimeEvidenceAnswerUsesExplicitUserModel(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/chat/completions" || req.Header.Get("Authorization") != "Bearer runtime-secret" {
			t.Errorf("provider request = %s %#v", req.URL.Path, req.Header)
		}
		raw, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(raw), "Verified excerpt") || !strings.Contains(string(raw), "Question?") {
			t.Errorf("provider payload = %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Verified answer [1]"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":4,"total_tokens":24}}`)
	}))
	defer provider.Close()
	cache.Set("user-provider:model", cache.PrefixDefault, &op.ModelConfig{
		Key: "user-provider:model", ID: "model", Name: "User Model", Provider: "user-provider",
		API: "openai-completions", BaseURL: provider.URL, APIKey: "runtime-secret", Enabled: true,
	}, cache.NoExpiration)

	result, err := executeRuntimeEvidenceAnswer(context.Background(), op.RuntimeEvidenceAnswerRequest{
		RequestID: "turn-runtime-test", ModelKey: "user-provider:model", Question: "Question?",
		Evidence: []op.RuntimeEvidenceItem{{CitationID: "pbcit_test", Title: "Evidence", Excerpt: "Verified excerpt"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "Verified answer [1]" || result.BillingResponsibility != "external_provider" || result.ModelKey != "user-provider:model" {
		t.Fatalf("runtime evidence result = %+v", result)
	}
}

func TestRuntimeEvidenceAnswerRejectsManagedCloudModel(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	cache.Set("cloud:model", cache.PrefixDefault, &op.ModelConfig{
		Key: "cloud:model", ID: "model", Name: "Cloud", Provider: "cloud", Source: "gateway",
		API: "openai-completions", BaseURL: "https://example.invalid", APIKey: "managed", Enabled: true,
	}, cache.NoExpiration)
	_, err := executeRuntimeEvidenceAnswer(context.Background(), op.RuntimeEvidenceAnswerRequest{
		RequestID: "turn-runtime-test", ModelKey: "cloud:model", Question: "Question?",
		Evidence: []op.RuntimeEvidenceItem{{CitationID: "pbcit_test", Title: "Evidence", Excerpt: "Verified excerpt"}},
	})
	if err == nil || !strings.Contains(err.Error(), "user-configured provider") {
		t.Fatalf("managed model error = %v", err)
	}
}

package provider

import (
	"encoding/json"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestToUsage_SplitsCachedAndCacheWritePromptTokens(t *testing.T) {
	raw := `{
	  "prompt_tokens": 20,
	  "completion_tokens": 5,
	  "total_tokens": 25,
	  "prompt_tokens_details": {
	    "cached_tokens": 9,
	    "cache_write_tokens": 4
	  }
	}`
	var usage openai.CompletionUsage
	if err := json.Unmarshal([]byte(raw), &usage); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	converted := toUsage(usage)
	if converted.InputTokens != 11 {
		t.Fatalf("InputTokens = %d, want 11", converted.InputTokens)
	}
	if converted.CacheReadTokens != 5 {
		t.Fatalf("CacheReadTokens = %d, want 5", converted.CacheReadTokens)
	}
	if converted.CacheWriteTokens != 4 {
		t.Fatalf("CacheWriteTokens = %d, want 4", converted.CacheWriteTokens)
	}
	if converted.TotalTokens != 25 {
		t.Fatalf("TotalTokens = %d, want 25", converted.TotalTokens)
	}
}

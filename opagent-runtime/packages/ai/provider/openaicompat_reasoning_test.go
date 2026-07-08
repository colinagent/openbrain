package provider

import (
	"testing"
)

func TestExtractReasoningDeltaFromJSON(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		path   string
		wantS  string
		wantOK bool
	}{
		{
			name:   "missing field",
			raw:    `{"delta":{}}`,
			path:   "delta.reasoning_content",
			wantS:  "",
			wantOK: false,
		},
		{
			name:   "empty raw",
			raw:    "",
			path:   "delta.reasoning_content",
			wantS:  "",
			wantOK: false,
		},
		{
			name:   "string delta with leading space preserved",
			raw:    `{"delta":{"reasoning_content":" are"}}`,
			path:   "delta.reasoning_content",
			wantS:  " are",
			wantOK: true,
		},
		{
			name:   "string delta with newline and indent preserved",
			raw:    `{"delta":{"reasoning_content":"First.\n  Then this."}}`,
			path:   "delta.reasoning_content",
			wantS:  "First.\n  Then this.",
			wantOK: true,
		},
		{
			name:   "string delta no whitespace",
			raw:    `{"delta":{"reasoning_content":"We"}}`,
			path:   "delta.reasoning_content",
			wantS:  "We",
			wantOK: true,
		},
		{
			name:   "array delta with leading spaces in items",
			raw:    `{"delta":{"reasoning_content":[{"text":"We"},{"text":" are"},{"text":" being"}]}}`,
			path:   "delta.reasoning_content",
			wantS:  "We are being",
			wantOK: true,
		},
		{
			name:   "array delta string items preserved",
			raw:    `{"delta":{"reasoning_content":["Hello", " world"]}}`,
			path:   "delta.reasoning_content",
			wantS:  "Hello world",
			wantOK: true,
		},
		{
			name:   "empty string delta still present",
			raw:    `{"delta":{"reasoning_content":""}}`,
			path:   "delta.reasoning_content",
			wantS:  "",
			wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotS, gotOK := extractReasoningDeltaFromJSON(tt.raw, tt.path)
			if gotOK != tt.wantOK {
				t.Errorf("extractReasoningDeltaFromJSON() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotS != tt.wantS {
				t.Errorf("extractReasoningDeltaFromJSON() = %q, want %q", gotS, tt.wantS)
			}
		})
	}
}

func TestExtractReasoningWithReplayField(t *testing.T) {
	field, reasoning := extractReasoningWithReplayField(`{"message":{"reasoning":"plan first"}}`, "message.")
	if field != "reasoning" {
		t.Fatalf("field = %q, want reasoning", field)
	}
	if reasoning != "plan first" {
		t.Fatalf("reasoning = %q, want plan first", reasoning)
	}

	field, delta, ok := extractReasoningDelta(`{"delta":{"reasoning_text":" step two"}}`, "delta.")
	if !ok {
		t.Fatal("expected reasoning delta")
	}
	if field != "reasoning_text" {
		t.Fatalf("delta field = %q, want reasoning_text", field)
	}
	if delta != " step two" {
		t.Fatalf("delta = %q, want %q", delta, " step two")
	}
}

func TestExtractReasoningSignatureAndStringDeltaFromJSON(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		path      string
		wantValue string
		wantOK    bool
	}{
		{
			name:      "message reasoning signature",
			raw:       `{"message":{"reasoning_signature":"sig_message"}}`,
			path:      "message.reasoning_signature",
			wantValue: "sig_message",
			wantOK:    true,
		},
		{
			name:      "delta reasoning signature",
			raw:       `{"delta":{"reasoning_signature":"sig_delta"}}`,
			path:      "delta.reasoning_signature",
			wantValue: "sig_delta",
			wantOK:    true,
		},
		{
			name:      "missing reasoning signature",
			raw:       `{"delta":{}}`,
			path:      "delta.reasoning_signature",
			wantValue: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDelta, gotOK := extractStringDeltaFromJSON(tt.raw, tt.path)
			if gotOK != tt.wantOK {
				t.Fatalf("extractStringDeltaFromJSON() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotDelta != tt.wantValue {
				t.Fatalf("extractStringDeltaFromJSON() = %q, want %q", gotDelta, tt.wantValue)
			}

			gotValue := extractStringFromJSON(tt.raw, tt.path)
			if gotValue != tt.wantValue {
				t.Fatalf("extractStringFromJSON() = %q, want %q", gotValue, tt.wantValue)
			}
		})
	}

	if got := extractReasoningSignature(`{"reasoning_signature":"sig_root"}`); got != "sig_root" {
		t.Fatalf("extractReasoningSignature() = %q, want %q", got, "sig_root")
	}
}

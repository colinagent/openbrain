package ai

import (
	"strings"
	"testing"
)

func TestParseResponsesFailureErrorJSON_PreservesGatewayStringErrorDetail(t *testing.T) {
	err := ParseResponsesFailureErrorJSON(`{"type":"response.failed","error":"response failed without details","request_id":"req_x"}`)
	retryErr, ok := AsRetryError(err)
	if !ok || retryErr == nil {
		t.Fatalf("expected retry error, got %T %v", err, err)
	}
	if !retryErr.Retryable {
		t.Fatalf("Retryable = false, want true: %#v", retryErr)
	}
	if !strings.Contains(retryErr.Error(), "response failed without details") {
		t.Fatalf("error = %q, want preserved detail", retryErr.Error())
	}
	if !strings.Contains(retryErr.Error(), "req_x") {
		t.Fatalf("error = %q, want request id", retryErr.Error())
	}
}

func TestParseResponsesStreamEventJSON_ParsesCompletedResponse(t *testing.T) {
	event, err := ParseResponsesStreamEventJSON([]byte(`{"type":"response.completed","response":{"id":"resp_123","model":"gpt-5.4","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`))
	if err != nil {
		t.Fatalf("ParseResponsesStreamEventJSON(): %v", err)
	}
	if event.Type != "response.completed" {
		t.Fatalf("event.Type = %q, want response.completed", event.Type)
	}
	if event.Response == nil {
		t.Fatal("event.Response is nil")
	}
	if event.Response.ID != "resp_123" || event.Response.Usage.TotalTokens != 3 {
		t.Fatalf("event.Response = %#v", event.Response)
	}
	if len(event.Response.Output) != 1 || event.Response.Output[0].Role != "assistant" {
		t.Fatalf("event.Response.Output = %#v", event.Response.Output)
	}
}

func TestParseResponsesStreamEventJSON_PreservesWhitespaceOnlyDelta(t *testing.T) {
	event, err := ParseResponsesStreamEventJSON([]byte(`{"type":"response.output_text.delta","delta":" "}`))
	if err != nil {
		t.Fatalf("ParseResponsesStreamEventJSON(): %v", err)
	}
	if event.Type != "response.output_text.delta" {
		t.Fatalf("event.Type = %q, want response.output_text.delta", event.Type)
	}
	if event.Delta != " " {
		t.Fatalf("event.Delta = %q, want single space preserved", event.Delta)
	}
}

func TestParseResponsesStreamEventJSON_PreservesNewlineDelta(t *testing.T) {
	event, err := ParseResponsesStreamEventJSON([]byte(`{"type":"response.output_text.delta","delta":"\n"}`))
	if err != nil {
		t.Fatalf("ParseResponsesStreamEventJSON(): %v", err)
	}
	if event.Delta != "\n" {
		t.Fatalf("event.Delta = %q, want newline preserved", event.Delta)
	}
}

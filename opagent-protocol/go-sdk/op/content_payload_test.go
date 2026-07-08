package op

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJsonContentMarshalsPayload(t *testing.T) {
	content := &JsonContent{Raw: json.RawMessage(`{"k":"v"}`)}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal json content: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"payload":{"k":"v"}`) {
		t.Fatalf("expected payload field, got %s", got)
	}
	if strings.Contains(got, `"message"`) {
		t.Fatalf("legacy message field should not be present, got %s", got)
	}
}

func TestGeneralContentRejectsLegacyJsonMessageField(t *testing.T) {
	raw := `{"meta":{"opcode":"thread/submit"},"content":{"type":"json","message":{"foo":"bar"}}}`
	var req GeneralContent
	err := json.Unmarshal([]byte(raw), &req)
	if err == nil {
		t.Fatal("expected error for legacy content.message")
	}
	if !strings.Contains(err.Error(), `"message" is no longer supported`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGeneralContentRejectsContentMetaField(t *testing.T) {
	raw := `{"meta":{"opcode":"thread/submit"},"content":{"type":"text","text":"hi","_meta":{"k":"v"}}}`
	var req GeneralContent
	err := json.Unmarshal([]byte(raw), &req)
	if err == nil {
		t.Fatal("expected error for content._meta")
	}
	if !strings.Contains(err.Error(), `content._meta is no longer supported`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGeneralContentRequiresJsonPayloadField(t *testing.T) {
	raw := `{"meta":{"opcode":"tool/call"},"content":{"type":"json"}}`
	var req GeneralContent
	err := json.Unmarshal([]byte(raw), &req)
	if err == nil {
		t.Fatal("expected error when json content payload is missing")
	}
	if !strings.Contains(err.Error(), `"payload" is required`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

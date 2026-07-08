package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeResponsesAPIRequestPayload_NormalizesServiceTier(t *testing.T) {
	req, _, err := DecodeResponsesAPIRequestPayload(ResponsesAPIRequestPayload{
		Model:       "gpt-5.4",
		ServiceTier: "priority",
		Input:       json.RawMessage(`[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]`),
	})
	if err != nil {
		t.Fatalf("DecodeResponsesAPIRequestPayload(): %v", err)
	}
	if got := req.ServiceTier; got != ServiceTierPriority {
		t.Fatalf("ServiceTier = %q, want %q", got, ServiceTierPriority)
	}
}

func TestMarshalResponsesAPIRequestJSON_WritesPriorityServiceTier(t *testing.T) {
	raw, err := MarshalResponsesAPIRequestJSON(&ResponsesRequest{
		Model:       "gpt-5.4",
		ServiceTier: ServiceTierPriority,
	})
	if err != nil {
		t.Fatalf("MarshalResponsesAPIRequestJSON(): %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if got := strings.TrimSpace(body["service_tier"].(string)); got != "priority" {
		t.Fatalf("service_tier = %q, want priority", got)
	}
}

func TestMarshalResponsesInputItemsJSON_ToolImageOutputUsesStringImageURL(t *testing.T) {
	rawItems, err := MarshalResponsesInputItemsJSON([]ResponseItem{{
		Type:   "function_call_output",
		CallID: "call_img",
		OutputContent: []ResponseContentPart{
			{Type: "input_text", Text: "Read image file [image/png]"},
			{Type: "input_image", ImageURL: "data:image/png;base64,AAA", Detail: "auto"},
		},
	}})
	if err != nil {
		t.Fatalf("MarshalResponsesInputItemsJSON(): %v", err)
	}
	if len(rawItems) != 1 {
		t.Fatalf("len(rawItems) = %d, want 1", len(rawItems))
	}

	var item struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Output []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL any    `json:"image_url"`
			Detail   string `json:"detail"`
		} `json:"output"`
	}
	if err := json.Unmarshal(rawItems[0], &item); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if item.Type != "function_call_output" || item.CallID != "call_img" {
		t.Fatalf("item envelope = %+v", item)
	}
	if len(item.Output) != 2 {
		t.Fatalf("len(output) = %d, want 2", len(item.Output))
	}
	image := item.Output[1]
	if image.Type != "input_image" {
		t.Fatalf("image type = %q, want input_image", image.Type)
	}
	if image.ImageURL != "data:image/png;base64,AAA" {
		t.Fatalf("image_url = %#v, want string data URL", image.ImageURL)
	}
	if image.Detail != "auto" {
		t.Fatalf("detail = %q, want auto", image.Detail)
	}
}

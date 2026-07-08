package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CanonicalAPIRequestPayload struct {
	Model              string              `json:"model"`
	Context            ConversationContext `json:"context"`
	Config             GenerationConfig    `json:"config"`
	PreviousResponseID string              `json:"previousResponseID,omitempty"`
	RequestID          string              `json:"requestID,omitempty"`
}

type CanonicalWebsocketCreatePayload struct {
	Type string `json:"type"`
	CanonicalAPIRequestPayload
}

func DecodeCanonicalAPIRequestJSON(raw []byte) (string, *ProviderRequest, error) {
	var payload CanonicalAPIRequestPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", nil, fmt.Errorf("invalid JSON payload")
	}
	return DecodeCanonicalAPIRequestPayload(payload)
}

func DecodeCanonicalAPIRequestPayload(payload CanonicalAPIRequestPayload) (string, *ProviderRequest, error) {
	modelID := strings.TrimSpace(payload.Model)
	if modelID == "" {
		return "", nil, fmt.Errorf("model is required")
	}
	return modelID, &ProviderRequest{
		Context:            payload.Context,
		Config:             payload.Config,
		PreviousResponseID: strings.TrimSpace(payload.PreviousResponseID),
		RequestID:          strings.TrimSpace(payload.RequestID),
	}, nil
}

func MarshalCanonicalAPIRequestJSON(modelID string, req *ProviderRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("model is required")
	}
	return json.Marshal(CanonicalAPIRequestPayload{
		Model:              modelID,
		Context:            req.Context,
		Config:             req.Config,
		PreviousResponseID: strings.TrimSpace(req.PreviousResponseID),
		RequestID:          strings.TrimSpace(req.RequestID),
	})
}

func DecodeCanonicalWebsocketCreateJSON(raw []byte) (string, *ProviderRequest, error) {
	var payload CanonicalWebsocketCreatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", nil, fmt.Errorf("invalid JSON payload")
	}
	if strings.TrimSpace(payload.Type) != "canonical.create" {
		return "", nil, fmt.Errorf("unsupported websocket event")
	}
	return DecodeCanonicalAPIRequestPayload(payload.CanonicalAPIRequestPayload)
}

func MarshalCanonicalWebsocketCreateJSON(modelID string, req *ProviderRequest) ([]byte, error) {
	body, err := MarshalCanonicalAPIRequestJSON(modelID, req)
	if err != nil {
		return nil, err
	}
	payload := make(map[string]json.RawMessage)
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["type"] = json.RawMessage(`"canonical.create"`)
	return json.Marshal(payload)
}

func ParseCanonicalAPIResponseJSON(raw []byte) (*ProviderResponse, error) {
	var payload struct {
		Message    ConversationMessage `json:"message"`
		Usage      Usage               `json:"usage"`
		StopReason StopReason          `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return &ProviderResponse{
		Message:    payload.Message,
		Usage:      payload.Usage,
		StopReason: payload.StopReason,
	}, nil
}

func RenderCanonicalAPIResponseJSON(resp *ProviderResponse) json.RawMessage {
	if resp == nil {
		resp = &ProviderResponse{}
	}
	data, _ := json.Marshal(struct {
		Message    ConversationMessage `json:"message"`
		Usage      Usage               `json:"usage"`
		StopReason StopReason          `json:"stopReason,omitempty"`
	}{
		Message:    resp.Message,
		Usage:      resp.Usage,
		StopReason: resp.StopReason,
	})
	return data
}

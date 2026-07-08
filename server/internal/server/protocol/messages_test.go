package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuthConfigJSONRoundTripPreservesAIGateway(t *testing.T) {
	raw := []byte(`{
		"version": 1,
		"baseUrl": "https://www.openbrain.io",
		"gateway": "https://api.op-agent.com",
		"aiGateway": "https://api.op-agent.com",
		"token": "tok",
		"uid": "user-test1",
		"email": "u1@example.com",
		"activeOrgID": "acme",
		"activeOrgName": "Acme",
		"updatedAt": 123
	}`)

	var cfg AuthConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if cfg.AIGateway != "https://api.op-agent.com" {
		t.Fatalf("AIGateway = %q, want https://api.op-agent.com", cfg.AIGateway)
	}
	if cfg.ActiveOrgID != "acme" {
		t.Fatalf("ActiveOrgID = %q, want acme", cfg.ActiveOrgID)
	}

	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("encoded json is invalid: %s", string(encoded))
	}
	if !strings.Contains(string(encoded), `"aiGateway":"https://api.op-agent.com"`) {
		t.Fatalf("encoded json missing aiGateway: %s", string(encoded))
	}
	if !strings.Contains(string(encoded), `"activeOrgID":"acme"`) {
		t.Fatalf("encoded json missing activeOrgID: %s", string(encoded))
	}
}

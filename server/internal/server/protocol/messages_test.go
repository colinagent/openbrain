package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuthConfigJSONRoundTripPreservesAIGateway(t *testing.T) {
	raw := []byte(`{
		"version": 2,
		"baseUrl": "https://www.openbrain.io",
		"gateway": "https://api.op-agent.com",
		"aiGateway": "https://api.op-agent.com",
		"token": "tok",
		"uid": "user-test1",
		"email": "u1@example.com",
		"deploymentID": "dep-test",
		"orgID": "org-acme",
		"orgName": "Acme",
		"identityID": "idn-test",
		"connectionID": "conn-test",
		"authMethod": "email",
		"authTime": "2026-07-23T00:00:00Z",
		"expiresAt": "2026-07-24T00:00:00Z",
		"updatedAt": 123
	}`)

	var cfg AuthConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if cfg.AIGateway != "https://api.op-agent.com" {
		t.Fatalf("AIGateway = %q, want https://api.op-agent.com", cfg.AIGateway)
	}
	if cfg.OrgID != "org-acme" {
		t.Fatalf("OrgID = %q, want org-acme", cfg.OrgID)
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
	if !strings.Contains(string(encoded), `"orgID":"org-acme"`) {
		t.Fatalf("encoded json missing orgID: %s", string(encoded))
	}
}

package remotecontrol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

type fakeRuntimeView struct {
	config *op.Config
	system *op.SystemConfigResult
	nodes  []*op.OpNode
}

func (f *fakeRuntimeView) GetConfigContext(context.Context) (*op.Config, error) {
	return f.config, nil
}
func (f *fakeRuntimeView) GetSystemConfig(context.Context) (*op.SystemConfigResult, error) {
	return f.system, nil
}
func (f *fakeRuntimeView) ListNodes(context.Context) ([]*op.OpNode, error) { return f.nodes, nil }

type staticEnvironmentView struct{ snapshot EnvironmentSnapshot }

func (v staticEnvironmentView) EnvironmentSnapshot() EnvironmentSnapshot { return v.snapshot }

func TestMinimumHandlersReturnSanitizedRuntimeViews(t *testing.T) {
	runtimeView := &fakeRuntimeView{
		config: &op.Config{User: &op.UserConfig{
			DefaultModelKey: "model-key",
			Auth:            &op.AuthConfig{Token: "account-secret", UID: "user-alice"},
			Models:          []op.ModelConfig{{Key: "model-key", ID: "model-id", Name: "Model", Provider: "provider", APIKey: "model-secret", BaseURL: "https://private.example", Headers: map[string]string{"Authorization": "secret"}, Enabled: true}},
		}},
		system: &op.SystemConfigResult{SystemConfig: op.SystemConfig{HostID: "host-a", BaseDir: "/Users/alice/.openbrain"}, DefaultWorkspace: "/Users/alice/.openbrain/conversations"},
		nodes:  []*op.OpNode{{ID: "agent-a", Kind: "agent", URI: "file:///Users/alice/.openbrain/agents/a/AGENT.md", Cwd: "/Users/alice/private", Meta: op.AgentMeta{Name: "Coder", Description: "Codes"}}},
	}
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	environment := staticEnvironmentView{snapshot: EnvironmentSnapshot{EnvironmentID: "environment-a", Name: "Alice Mac", RegionID: "us", State: "online", ServerVersion: "test"}}
	if err := RegisterMinimumHandlers(dispatcher, runtimeView, environment); err != nil {
		t.Fatal(err)
	}
	principal, err := NewPrincipal("user-alice", "environment-a", "client-a",
		protocol.CapabilityEnvironmentRead, protocol.CapabilityWorkspaceList,
		protocol.CapabilityAgentList, protocol.CapabilityModelList)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []protocol.Operation{
		protocol.OperationConnectionHandshake, protocol.OperationEnvironmentStatus,
		protocol.OperationWorkspaceList, protocol.OperationAgentList, protocol.OperationModelList,
	} {
		response := dispatcher.Dispatch(context.Background(), principal, protocol.Envelope{
			ProtocolVersion: protocol.CurrentProtocolVersion, Type: protocol.EnvelopeTypeRequest,
			ClientID: "client-a", StreamID: "stream-a", SeqID: 1, RequestID: "request-" + string(operation),
			Operation: operation, Payload: json.RawMessage(`{}`),
		})
		if response.Error != nil {
			t.Fatalf("%s failed: %+v", operation, response.Error)
		}
		payload := string(response.Payload)
		for _, forbidden := range []string{"account-secret", "model-secret", "private.example", "/Users/alice", "file://"} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("%s leaked %q in %s", operation, forbidden, payload)
			}
		}
	}
}

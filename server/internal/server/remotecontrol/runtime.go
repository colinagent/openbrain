package remotecontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

type RuntimeView interface {
	GetConfigContext(context.Context) (*op.Config, error)
	GetSystemConfig(context.Context) (*op.SystemConfigResult, error)
	ListNodes(context.Context) ([]*op.OpNode, error)
}

type EnvironmentView interface {
	EnvironmentSnapshot() EnvironmentSnapshot
}

type EnvironmentSnapshot struct {
	EnvironmentID string `json:"environmentID"`
	Name          string `json:"name"`
	RegionID      string `json:"regionID"`
	State         string `json:"state"`
	ServerVersion string `json:"serverVersion"`
	Platform      string `json:"platform"`
}

type workspaceAccess struct {
	ID   string
	Name string
	Path string
}

func RegisterMinimumHandlers(dispatcher *Dispatcher, runtimeView RuntimeView, environment EnvironmentView) error {
	registrations := []struct {
		operation  protocol.Operation
		capability protocol.Capability
		handler    Handler
	}{
		{protocol.OperationConnectionHandshake, protocol.CapabilityEnvironmentRead, handshakeHandler(environment, dispatcher)},
		{protocol.OperationEnvironmentStatus, protocol.CapabilityEnvironmentRead, environmentStatusHandler(environment)},
		{protocol.OperationWorkspaceList, protocol.CapabilityWorkspaceList, workspaceListHandler(runtimeView)},
		{protocol.OperationAgentList, protocol.CapabilityAgentList, agentListHandler(runtimeView)},
		{protocol.OperationModelList, protocol.CapabilityModelList, modelListHandler(runtimeView)},
	}
	for _, registration := range registrations {
		if err := dispatcher.Register(registration.operation, registration.capability, registration.handler); err != nil {
			return err
		}
	}
	return nil
}

func handshakeHandler(environment EnvironmentView, dispatcher *Dispatcher) Handler {
	return func(_ context.Context, _ Principal, _ json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
		snapshot := environment.EnvironmentSnapshot()
		return marshalRemote(protocol.HandshakeResponse{
			ProtocolVersion: protocol.CurrentProtocolVersion,
			EnvironmentID:   snapshot.EnvironmentID,
			InstanceID:      snapshot.EnvironmentID,
			ServerVersion:   snapshot.ServerVersion,
			Capabilities:    dispatcher.Capabilities(),
			Limits:          protocol.DefaultTransportLimits(),
		})
	}
}

func environmentStatusHandler(environment EnvironmentView) Handler {
	return func(_ context.Context, _ Principal, _ json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
		return marshalRemote(environment.EnvironmentSnapshot())
	}
}

func workspaceListHandler(runtimeView RuntimeView) Handler {
	return func(ctx context.Context, _ Principal, _ json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
		workspace, err := defaultWorkspace(ctx, runtimeView)
		if err != nil {
			return nil, internalRemoteError()
		}
		return marshalRemote(map[string]any{"workspaces": []map[string]any{{
			"workspaceID": workspace.ID,
			"name":        workspace.Name,
			"kind":        "conversations",
		}}})
	}
}

func defaultWorkspace(ctx context.Context, runtimeView RuntimeView) (workspaceAccess, error) {
	cfg, err := runtimeView.GetSystemConfig(ctx)
	if err != nil {
		return workspaceAccess{}, err
	}
	path := strings.TrimSpace(cfg.DefaultWorkspace)
	if path == "" {
		return workspaceAccess{}, errors.New("runtime default workspace is unavailable")
	}
	name := strings.TrimSpace(filepath.Base(path))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "Conversations"
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(cfg.HostID) + "\x00default-workspace"))
	return workspaceAccess{
		ID:   "workspace-" + hex.EncodeToString(digest[:12]),
		Name: name,
		Path: filepath.Clean(path),
	}, nil
}

func agentListHandler(runtimeView RuntimeView) Handler {
	return func(ctx context.Context, _ Principal, _ json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
		nodes, err := runtimeView.ListNodes(ctx)
		if err != nil {
			return nil, internalRemoteError()
		}
		agents := make([]map[string]any, 0)
		for _, node := range nodes {
			if !nodeSupportsThreadSubmit(node) {
				continue
			}
			name, description := node.ID, ""
			if meta, ok := node.Meta.(op.AgentMeta); ok {
				name, description = strings.TrimSpace(meta.Name), strings.TrimSpace(meta.Description)
			} else if raw, err := json.Marshal(node.Meta); err == nil {
				var meta op.AgentMeta
				if json.Unmarshal(raw, &meta) == nil {
					name, description = strings.TrimSpace(meta.Name), strings.TrimSpace(meta.Description)
				}
			}
			if name == "" {
				name = node.ID
			}
			agents = append(agents, map[string]any{"agentID": node.ID, "name": name, "description": description})
		}
		sort.Slice(agents, func(i, j int) bool { return agents[i]["name"].(string) < agents[j]["name"].(string) })
		return marshalRemote(map[string]any{"agents": agents})
	}
}

func nodeSupportsThreadSubmit(node *op.OpNode) bool {
	if node == nil || node.Kind != string(op.NodeKindAgent) {
		return false
	}
	for _, opcode := range node.OpCodes {
		if opcode == op.OpThreadSubmit {
			return true
		}
	}
	return false
}

func modelListHandler(runtimeView RuntimeView) Handler {
	return func(ctx context.Context, _ Principal, _ json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
		cfg, err := runtimeView.GetConfigContext(ctx)
		if err != nil || cfg.User == nil {
			return nil, internalRemoteError()
		}
		models := make([]map[string]any, 0, len(cfg.User.Models))
		for _, model := range cfg.User.Models {
			if !model.Enabled {
				continue
			}
			models = append(models, map[string]any{
				"key": model.Key, "modelID": model.ID, "name": model.Name,
				"provider": model.Provider, "reasoning": model.Reasoning,
				"reasoningControl": model.ReasoningControl, "reasoningLevels": model.ReasoningLevels,
			})
		}
		return marshalRemote(map[string]any{"models": models, "defaultModelKey": cfg.User.DefaultModelKey})
	}
}

func marshalRemote(value any) (json.RawMessage, *protocol.RemoteError) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, internalRemoteError()
	}
	return raw, nil
}

func internalRemoteError() *protocol.RemoteError {
	return &protocol.RemoteError{Code: protocol.ErrorInternal, Message: "remote operation failed", Retryable: true}
}

func platformName() string { return runtime.GOOS }

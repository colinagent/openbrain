package remotecontrol

import (
	"context"
	"encoding/json"
	"testing"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

func TestDefaultConfigDeniesRegisteredOperation(t *testing.T) {
	dispatcher := NewDispatcher(DefaultConfig())
	registerStatusHandler(t, dispatcher)
	principal := testPrincipal(t, protocol.CapabilityEnvironmentRead)

	response := dispatcher.Dispatch(context.Background(), principal, testRequest(protocol.OperationEnvironmentStatus))
	assertErrorCode(t, response, protocol.ErrorOperationDenied)
}

func TestKillSwitchOverridesEnabledFlag(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: true})
	registerStatusHandler(t, dispatcher)
	principal := testPrincipal(t, protocol.CapabilityEnvironmentRead)

	response := dispatcher.Dispatch(context.Background(), principal, testRequest(protocol.OperationEnvironmentStatus))
	assertErrorCode(t, response, protocol.ErrorOperationDenied)
}

func TestUnknownAndUnregisteredOperationsAreDenied(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	principal := testPrincipal(t, protocol.CapabilityEnvironmentRead)

	unknown := testRequest(protocol.Operation("http.proxy"))
	assertErrorCode(t, dispatcher.Dispatch(context.Background(), principal, unknown), protocol.ErrorOperationDenied)

	unregistered := testRequest(protocol.OperationEnvironmentStatus)
	assertErrorCode(t, dispatcher.Dispatch(context.Background(), principal, unregistered), protocol.ErrorOperationDenied)
}

func TestDispatcherRequiresMatchingPrincipalAndCapability(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	registerStatusHandler(t, dispatcher)

	missingCapability := testPrincipal(t, protocol.CapabilityWorkspaceList)
	assertErrorCode(t, dispatcher.Dispatch(context.Background(), missingCapability, testRequest(protocol.OperationEnvironmentStatus)), protocol.ErrorCapabilityUnavailable)

	wrongClient := testPrincipal(t, protocol.CapabilityEnvironmentRead)
	request := testRequest(protocol.OperationEnvironmentStatus)
	request.ClientID = "client-other"
	assertErrorCode(t, dispatcher.Dispatch(context.Background(), wrongClient, request), protocol.ErrorNotAuthenticated)
}

func TestDispatcherCallsExplicitlyRegisteredHandler(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	registerStatusHandler(t, dispatcher)
	principal := testPrincipal(t, protocol.CapabilityEnvironmentRead)

	response := dispatcher.Dispatch(context.Background(), principal, testRequest(protocol.OperationEnvironmentStatus))
	if response.Error != nil {
		t.Fatalf("Dispatch error = %+v", response.Error)
	}
	if got := string(response.Payload); got != `{"online":true}` {
		t.Fatalf("payload = %s", got)
	}
	if response.Type != protocol.EnvelopeTypeResponse || response.SeqID != 1 || response.RequestID != "request-1" {
		t.Fatalf("unexpected response envelope: %+v", response)
	}
}

func TestRegisterRejectsUnknownDuplicateAndNilHandlers(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	handler := func(context.Context, Principal, json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
		return nil, nil
	}
	if err := dispatcher.Register(protocol.Operation("http.proxy"), protocol.CapabilityEnvironmentRead, handler); err == nil {
		t.Fatal("unknown operation registration succeeded")
	}
	if err := dispatcher.Register(protocol.OperationEnvironmentStatus, protocol.CapabilityEnvironmentRead, nil); err == nil {
		t.Fatal("nil handler registration succeeded")
	}
	if err := dispatcher.Register(protocol.OperationEnvironmentStatus, protocol.CapabilityEnvironmentRead, handler); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := dispatcher.Register(protocol.OperationEnvironmentStatus, protocol.CapabilityEnvironmentRead, handler); err == nil {
		t.Fatal("duplicate registration succeeded")
	}
}

func registerStatusHandler(t *testing.T, dispatcher *Dispatcher) {
	t.Helper()
	err := dispatcher.Register(
		protocol.OperationEnvironmentStatus,
		protocol.CapabilityEnvironmentRead,
		func(context.Context, Principal, json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
			return json.RawMessage(`{"online":true}`), nil
		},
	)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func testPrincipal(t *testing.T, capabilities ...protocol.Capability) Principal {
	t.Helper()
	principal, err := NewPrincipal("user-1", "environment-1", "client-1", capabilities...)
	if err != nil {
		t.Fatalf("NewPrincipal: %v", err)
	}
	return principal
}

func testRequest(operation protocol.Operation) protocol.Envelope {
	return protocol.Envelope{
		ProtocolVersion: protocol.CurrentProtocolVersion,
		Type:            protocol.EnvelopeTypeRequest,
		ClientID:        "client-1",
		StreamID:        "stream-1",
		SeqID:           1,
		RequestID:       "request-1",
		Operation:       operation,
		Payload:         json.RawMessage(`{}`),
	}
}

func assertErrorCode(t *testing.T, response protocol.Envelope, want protocol.ErrorCode) {
	t.Helper()
	if response.Error == nil {
		t.Fatalf("response error = nil, want %q", want)
	}
	if response.Error.Code != want {
		t.Fatalf("response error code = %q, want %q", response.Error.Code, want)
	}
	if len(response.Payload) != 0 {
		t.Fatalf("error response contains payload: %s", response.Payload)
	}
}

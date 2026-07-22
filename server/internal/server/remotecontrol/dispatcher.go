package remotecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

type Handler func(context.Context, Principal, json.RawMessage) (json.RawMessage, *protocol.RemoteError)

type registeredHandler struct {
	requiredCapability protocol.Capability
	handler            Handler
}

func (d *Dispatcher) Capabilities() []protocol.Capability {
	d.mu.RLock()
	defer d.mu.RUnlock()
	unique := make(map[protocol.Capability]struct{}, len(d.handlers))
	for _, registered := range d.handlers {
		unique[registered.requiredCapability] = struct{}{}
	}
	capabilities := make([]protocol.Capability, 0, len(unique))
	for capability := range unique {
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	return capabilities
}

// Dispatcher is the only entry point from remote transport frames into host
// behavior. An operation is inaccessible until a handler is explicitly
// registered with its required capability.
type Dispatcher struct {
	config   Config
	mu       sync.RWMutex
	handlers map[protocol.Operation]registeredHandler
	replays  *replayCache
}

func NewDispatcher(config Config) *Dispatcher {
	defaults := DefaultConfig()
	if config.ReplayWindow <= 0 {
		config.ReplayWindow = defaults.ReplayWindow
	}
	if config.MaxReplayEntries <= 0 {
		config.MaxReplayEntries = defaults.MaxReplayEntries
	}
	if config.MaxReplayResultBytes <= 0 {
		config.MaxReplayResultBytes = defaults.MaxReplayResultBytes
	}
	return &Dispatcher{
		config:   config,
		handlers: make(map[protocol.Operation]registeredHandler),
		replays:  newReplayCache(config.ReplayWindow, config.MaxReplayEntries, config.MaxReplayResultBytes),
	}
}

func (d *Dispatcher) Register(operation protocol.Operation, requiredCapability protocol.Capability, handler Handler) error {
	if !protocol.IsKnownOperation(operation) {
		return fmt.Errorf("register unknown remote-control operation %q", operation)
	}
	if requiredCapability == "" {
		return fmt.Errorf("register remote-control operation %q without a capability", operation)
	}
	if handler == nil {
		return fmt.Errorf("register remote-control operation %q with a nil handler", operation)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.handlers[operation]; exists {
		return fmt.Errorf("remote-control operation %q is already registered", operation)
	}
	d.handlers[operation] = registeredHandler{
		requiredCapability: requiredCapability,
		handler:            handler,
	}
	return nil
}

func (d *Dispatcher) Dispatch(ctx context.Context, principal Principal, request protocol.Envelope) protocol.Envelope {
	response := protocol.Envelope{
		ProtocolVersion: protocol.CurrentProtocolVersion,
		Type:            protocol.EnvelopeTypeResponse,
		ClientID:        request.ClientID,
		StreamID:        request.StreamID,
		SeqID:           request.SeqID,
		RequestID:       request.RequestID,
	}

	if err := request.Validate(); err != nil {
		response.Error = remoteError(protocol.ErrorCodeOf(err), err.Error())
		return response
	}
	if request.Type != protocol.EnvelopeTypeRequest {
		response.Error = remoteError(protocol.ErrorInvalidEnvelope, "dispatcher accepts request envelopes only")
		return response
	}
	if !protocol.IsKnownOperation(request.Operation) {
		response.Error = remoteError(protocol.ErrorOperationDenied, "remote operation is not allowed")
		return response
	}
	if !d.config.AllowsRemoteControl() {
		response.Error = remoteError(protocol.ErrorOperationDenied, "remote control is unavailable")
		return response
	}
	if principal.ClientID == "" || principal.EnvironmentID == "" || principal.UID == "" || principal.ClientID != request.ClientID {
		response.Error = remoteError(protocol.ErrorNotAuthenticated, "remote principal does not match the request")
		return response
	}

	d.mu.RLock()
	registered, ok := d.handlers[request.Operation]
	d.mu.RUnlock()
	if !ok {
		response.Error = remoteError(protocol.ErrorOperationDenied, "remote operation is not allowed")
		return response
	}
	if !principal.HasCapability(registered.requiredCapability) {
		response.Error = remoteError(protocol.ErrorCapabilityUnavailable, "remote client lacks the required capability")
		return response
	}

	if protocol.RequiresIdempotency(request.Operation) {
		result := d.replays.Do(ctx, principal, request, func() replayResult {
			payload, remoteErr := registered.handler(ctx, principal, request.Payload)
			return replayResult{payload: payload, remoteError: remoteErr}
		})
		response.Payload = result.payload
		response.Error = result.remoteError
	} else {
		response.Payload, response.Error = registered.handler(ctx, principal, request.Payload)
	}
	if response.Error != nil {
		response.Payload = nil
	}
	return response
}

func remoteError(code protocol.ErrorCode, message string) *protocol.RemoteError {
	return &protocol.RemoteError{Code: code, Message: message}
}

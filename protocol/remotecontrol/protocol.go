// Package remotecontrol defines the versioned wire contract used by OpenBrain
// remote-control clients, relays, and host connectors. It is intentionally
// separate from the OpAgent runtime protocol: relay envelopes are transport
// frames, not runtime opcodes.
package remotecontrol

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	ProtocolVersionV1          = 1
	CurrentProtocolVersion     = ProtocolVersionV1
	MaxIdentifierBytes         = 128
	MaxCursorBytes             = 1024
	MaxFrameBytes              = 256 * 1024
	MaxMessageBytes            = 8 * 1024 * 1024
	MaxChunkCount              = 64
	MaxInFlightRequests        = 32
	HeartbeatIntervalSeconds   = 25
	PongTimeoutSeconds         = 60
	RequestReplayWindowSeconds = 5 * 60
)

type EnvelopeType string

const (
	EnvelopeTypeRequest  EnvelopeType = "request"
	EnvelopeTypeResponse EnvelopeType = "response"
	EnvelopeTypeEvent    EnvelopeType = "event"
	EnvelopeTypeAck      EnvelopeType = "ack"
	EnvelopeTypePing     EnvelopeType = "ping"
	EnvelopeTypePong     EnvelopeType = "pong"
	EnvelopeTypeClose    EnvelopeType = "close"
	EnvelopeTypeChunk    EnvelopeType = "chunk"
)

type Capability string

const (
	CapabilityEnvironmentRead Capability = "environment.read"
	CapabilityWorkspaceList   Capability = "workspace.list"
	CapabilityAgentList       Capability = "agent.list"
	CapabilityModelList       Capability = "model.list"
	CapabilityThreadRead      Capability = "thread.read"
	CapabilityThreadExecute   Capability = "thread.execute"
	CapabilityMessageReply    Capability = "message.reply"
	CapabilityFileRead        Capability = "file.read"
)

type Operation string

const (
	OperationConnectionHandshake Operation = "connection.handshake"
	OperationEnvironmentStatus   Operation = "environment.status"
	OperationWorkspaceList       Operation = "workspace.list"
	OperationAgentList           Operation = "agent.list"
	OperationModelList           Operation = "model.list"
	OperationThreadList          Operation = "thread.list"
	OperationThreadCreate        Operation = "thread.create"
	OperationThreadSnapshot      Operation = "thread.snapshot"
	OperationThreadSubmit        Operation = "thread.submit"
	OperationThreadInterrupt     Operation = "thread.interrupt"
	OperationThreadContinue      Operation = "thread.continue"
	OperationThreadSteer         Operation = "thread.steer"
	OperationThreadFollowUp      Operation = "thread.followUp"
	OperationThreadQueueRemove   Operation = "thread.queue.remove"
	OperationThreadQueuePromote  Operation = "thread.queue.promote"
	OperationMessageReply        Operation = "message.reply"
	OperationMessageMarkRead     Operation = "message.markRead"
	OperationFileList            Operation = "file.list"
	OperationFileStat            Operation = "file.stat"
	OperationFileSearch          Operation = "file.search"
	OperationFilePreviewOpen     Operation = "file.preview.open"
	OperationFilePreviewChunk    Operation = "file.preview.chunk"
)

var knownOperations = map[Operation]struct{}{
	OperationConnectionHandshake: {},
	OperationEnvironmentStatus:   {},
	OperationWorkspaceList:       {},
	OperationAgentList:           {},
	OperationModelList:           {},
	OperationThreadList:          {},
	OperationThreadCreate:        {},
	OperationThreadSnapshot:      {},
	OperationThreadSubmit:        {},
	OperationThreadInterrupt:     {},
	OperationThreadContinue:      {},
	OperationThreadSteer:         {},
	OperationThreadFollowUp:      {},
	OperationThreadQueueRemove:   {},
	OperationThreadQueuePromote:  {},
	OperationMessageReply:        {},
	OperationMessageMarkRead:     {},
	OperationFileList:            {},
	OperationFileStat:            {},
	OperationFileSearch:          {},
	OperationFilePreviewOpen:     {},
	OperationFilePreviewChunk:    {},
}

func IsKnownOperation(operation Operation) bool {
	_, ok := knownOperations[operation]
	return ok
}

func RequiresIdempotency(operation Operation) bool {
	switch operation {
	case OperationThreadCreate,
		OperationThreadSubmit,
		OperationThreadInterrupt,
		OperationThreadContinue,
		OperationThreadSteer,
		OperationThreadFollowUp,
		OperationThreadQueueRemove,
		OperationThreadQueuePromote,
		OperationMessageReply,
		OperationMessageMarkRead,
		OperationFilePreviewOpen:
		return true
	default:
		return false
	}
}

type ErrorCode string

const (
	ErrorNotAuthenticated      ErrorCode = "not_authenticated"
	ErrorEnvironmentOffline    ErrorCode = "environment_offline"
	ErrorClientRevoked         ErrorCode = "client_revoked"
	ErrorPairingExpired        ErrorCode = "pairing_expired"
	ErrorPairingAlreadyClaimed ErrorCode = "pairing_already_claimed"
	ErrorProtocolIncompatible  ErrorCode = "protocol_incompatible"
	ErrorCapabilityUnavailable ErrorCode = "capability_unavailable"
	ErrorOperationDenied       ErrorCode = "operation_denied"
	ErrorInvalidEnvelope       ErrorCode = "invalid_envelope"
	ErrorWorkspaceNotFound     ErrorCode = "workspace_not_found"
	ErrorPathOutsideWorkspace  ErrorCode = "path_outside_workspace"
	ErrorSensitivePathDenied   ErrorCode = "sensitive_path_denied"
	ErrorFileNotFound          ErrorCode = "file_not_found"
	ErrorFileTooLarge          ErrorCode = "file_too_large"
	ErrorPreviewExpired        ErrorCode = "preview_expired"
	ErrorThreadNotFound        ErrorCode = "thread_not_found"
	ErrorThreadBusy            ErrorCode = "thread_busy"
	ErrorRequestConflict       ErrorCode = "request_conflict"
	ErrorResyncRequired        ErrorCode = "resync_required"
	ErrorRateLimited           ErrorCode = "rate_limited"
	ErrorInternal              ErrorCode = "internal_error"
)

type Envelope struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Type            EnvelopeType    `json:"type"`
	ClientID        string          `json:"clientID,omitempty"`
	StreamID        string          `json:"streamID,omitempty"`
	SeqID           uint64          `json:"seqID,omitempty"`
	RequestID       string          `json:"requestID,omitempty"`
	Cursor          string          `json:"cursor,omitempty"`
	Operation       Operation       `json:"operation,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Error           *RemoteError    `json:"error,omitempty"`
	Chunk           *Chunk          `json:"chunk,omitempty"`
}

type RemoteError struct {
	Code      ErrorCode       `json:"code"`
	Message   string          `json:"message"`
	Retryable bool            `json:"retryable,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
}

type Chunk struct {
	SegmentID          uint32 `json:"segmentID"`
	SegmentCount       uint32 `json:"segmentCount"`
	MessageSizeBytes   uint64 `json:"messageSizeBytes"`
	MessageChunkBase64 string `json:"messageChunkBase64"`
}

type HandshakeRequest struct {
	ClientName               string `json:"clientName"`
	ClientVersion            string `json:"clientVersion"`
	RequestedProtocolVersion int    `json:"requestedProtocolVersion"`
	LastCursor               string `json:"lastCursor,omitempty"`
}

type HandshakeResponse struct {
	ProtocolVersion int             `json:"protocolVersion"`
	EnvironmentID   string          `json:"environmentID"`
	InstanceID      string          `json:"instanceID"`
	ServerVersion   string          `json:"serverVersion"`
	Capabilities    []Capability    `json:"capabilities"`
	Limits          TransportLimits `json:"limits"`
}

type TransportLimits struct {
	MaxFrameBytes              int `json:"maxFrameBytes"`
	MaxMessageBytes            int `json:"maxMessageBytes"`
	MaxChunkCount              int `json:"maxChunkCount"`
	MaxInFlightRequests        int `json:"maxInFlightRequests"`
	HeartbeatIntervalSeconds   int `json:"heartbeatIntervalSeconds"`
	PongTimeoutSeconds         int `json:"pongTimeoutSeconds"`
	RequestReplayWindowSeconds int `json:"requestReplayWindowSeconds"`
}

func DefaultTransportLimits() TransportLimits {
	return TransportLimits{
		MaxFrameBytes:              MaxFrameBytes,
		MaxMessageBytes:            MaxMessageBytes,
		MaxChunkCount:              MaxChunkCount,
		MaxInFlightRequests:        MaxInFlightRequests,
		HeartbeatIntervalSeconds:   HeartbeatIntervalSeconds,
		PongTimeoutSeconds:         PongTimeoutSeconds,
		RequestReplayWindowSeconds: RequestReplayWindowSeconds,
	}
}

type ResyncRequiredDetails struct {
	StreamID          string    `json:"streamID"`
	SnapshotOperation Operation `json:"snapshotOperation"`
	LatestRevision    string    `json:"latestRevision,omitempty"`
}

type ValidationError struct {
	Code    ErrorCode
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func DecodeEnvelope(data []byte) (Envelope, error) {
	if len(data) > MaxFrameBytes {
		return Envelope{}, invalidEnvelope("remote-control frame exceeds the size limit")
	}
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, invalidEnvelope("remote-control frame is invalid JSON")
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func EncodeEnvelope(envelope Envelope) ([]byte, error) {
	data, err := EncodeMessage(envelope)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFrameBytes {
		return nil, invalidEnvelope("remote-control frame exceeds the size limit")
	}
	return data, nil
}

// EncodeMessage encodes a complete logical envelope before optional transport
// chunking. A single relay frame must still use EncodeEnvelope.
func EncodeMessage(envelope Envelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, invalidEnvelope("remote-control envelope cannot be encoded")
	}
	if len(data) > MaxMessageBytes {
		return nil, invalidEnvelope("remote-control message exceeds the size limit")
	}
	return data, nil
}

func (e Envelope) Validate() error {
	if e.ProtocolVersion != CurrentProtocolVersion {
		return &ValidationError{
			Code:    ErrorProtocolIncompatible,
			Message: fmt.Sprintf("remote-control protocol version %d is unsupported", e.ProtocolVersion),
		}
	}
	if !isKnownEnvelopeType(e.Type) {
		return invalidEnvelope("remote-control envelope type is unsupported")
	}
	if e.ClientID == "" {
		return invalidEnvelope("remote-control clientID is required")
	}
	if err := validateIdentifier("clientID", e.ClientID); err != nil {
		return err
	}
	if e.StreamID != "" {
		if err := validateIdentifier("streamID", e.StreamID); err != nil {
			return err
		}
	}
	if e.RequestID != "" {
		if err := validateIdentifier("requestID", e.RequestID); err != nil {
			return err
		}
	}
	if len(e.Cursor) > MaxCursorBytes {
		return invalidEnvelope("remote-control cursor exceeds the size limit")
	}
	if len(e.Payload) > MaxMessageBytes {
		return invalidEnvelope("remote-control payload exceeds the message size limit")
	}
	if len(e.Payload) > 0 && !json.Valid(e.Payload) {
		return invalidEnvelope("remote-control payload must be valid JSON")
	}

	switch e.Type {
	case EnvelopeTypeRequest:
		if e.StreamID == "" || e.SeqID == 0 || e.RequestID == "" || e.Operation == "" {
			return invalidEnvelope("request envelope requires streamID, seqID, requestID, and operation")
		}
		if !IsKnownOperation(e.Operation) {
			return &ValidationError{Code: ErrorOperationDenied, Message: "remote operation is not allowed"}
		}
	case EnvelopeTypeResponse:
		if e.StreamID == "" || e.SeqID == 0 || e.RequestID == "" {
			return invalidEnvelope("response envelope requires streamID, seqID, and requestID")
		}
		if len(e.Payload) > 0 && e.Error != nil {
			return invalidEnvelope("response envelope cannot contain both payload and error")
		}
	case EnvelopeTypeEvent:
		if e.StreamID == "" || e.SeqID == 0 || e.Operation == "" {
			return invalidEnvelope("event envelope requires streamID, seqID, and operation")
		}
		if !IsKnownOperation(e.Operation) {
			return &ValidationError{Code: ErrorOperationDenied, Message: "remote operation is not allowed"}
		}
	case EnvelopeTypeAck:
		if e.StreamID == "" || e.SeqID == 0 {
			return invalidEnvelope("ack envelope requires streamID and seqID")
		}
	case EnvelopeTypeClose:
		if e.StreamID == "" {
			return invalidEnvelope("close envelope requires streamID")
		}
	case EnvelopeTypeChunk:
		if e.StreamID == "" || e.SeqID == 0 || e.Chunk == nil {
			return invalidEnvelope("chunk envelope requires streamID, seqID, and chunk")
		}
		if err := e.Chunk.Validate(); err != nil {
			return err
		}
	case EnvelopeTypePing, EnvelopeTypePong:
		// Connection-level heartbeats do not require a logical stream.
	}

	return nil
}

func (c Chunk) Validate() error {
	if c.SegmentCount == 0 {
		return invalidEnvelope("chunk segmentCount must be positive")
	}
	if c.SegmentCount > MaxChunkCount {
		return invalidEnvelope("chunk segmentCount exceeds the limit")
	}
	if c.SegmentID >= c.SegmentCount {
		return invalidEnvelope("chunk segmentID must be less than segmentCount")
	}
	if c.MessageSizeBytes == 0 || c.MessageSizeBytes > MaxMessageBytes || c.MessageChunkBase64 == "" {
		return invalidEnvelope("chunk requires messageSizeBytes and messageChunkBase64")
	}
	if len(c.MessageChunkBase64) > MaxFrameBytes {
		return invalidEnvelope("encoded chunk exceeds the frame size limit")
	}
	decoded, err := base64.StdEncoding.DecodeString(c.MessageChunkBase64)
	if err != nil || len(decoded) == 0 {
		return invalidEnvelope("chunk payload is not valid base64")
	}
	return nil
}

func ErrorCodeOf(err error) ErrorCode {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Code
	}
	return ErrorInternal
}

func isKnownEnvelopeType(envelopeType EnvelopeType) bool {
	switch envelopeType {
	case EnvelopeTypeRequest,
		EnvelopeTypeResponse,
		EnvelopeTypeEvent,
		EnvelopeTypeAck,
		EnvelopeTypePing,
		EnvelopeTypePong,
		EnvelopeTypeClose,
		EnvelopeTypeChunk:
		return true
	default:
		return false
	}
}

func invalidEnvelope(message string) error {
	return &ValidationError{Code: ErrorInvalidEnvelope, Message: message}
}

func validateIdentifier(name, value string) error {
	if len(value) > MaxIdentifierBytes {
		return invalidEnvelope(fmt.Sprintf("remote-control %s exceeds the size limit", name))
	}
	return nil
}

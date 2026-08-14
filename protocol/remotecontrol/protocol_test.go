package remotecontrol

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestV1FixturesDecodeAndValidate(t *testing.T) {
	for _, name := range []string{
		"handshake-request.json",
		"handshake-response.json",
		"thread-submit-request.json",
		"chunk-event.json",
		"error-response.json",
		"resync-required.json",
		"unknown-fields.json",
	} {
		t.Run(name, func(t *testing.T) {
			raw := readFixture(t, name)
			envelope, err := DecodeEnvelope(raw)
			if err != nil {
				t.Fatalf("DecodeEnvelope(%s): %v", name, err)
			}
			encoded, err := EncodeEnvelope(envelope)
			if err != nil {
				t.Fatalf("EncodeEnvelope(%s): %v", name, err)
			}
			if _, err := DecodeEnvelope(encoded); err != nil {
				t.Fatalf("round-trip DecodeEnvelope(%s): %v", name, err)
			}
		})
	}
}

func TestHandshakeFixturesUseSharedPayloadTypes(t *testing.T) {
	requestEnvelope := decodeFixture(t, "handshake-request.json")
	var request HandshakeRequest
	if err := json.Unmarshal(requestEnvelope.Payload, &request); err != nil {
		t.Fatalf("decode request payload: %v", err)
	}
	wantRequest := HandshakeRequest{
		ClientName:               "OpenBrain iOS",
		ClientVersion:            "0.1.0",
		RequestedProtocolVersion: ProtocolVersionV1,
		LastCursor:               "cursor-41",
	}
	if !reflect.DeepEqual(request, wantRequest) {
		t.Fatalf("request = %#v, want %#v", request, wantRequest)
	}

	responseEnvelope := decodeFixture(t, "handshake-response.json")
	var response HandshakeResponse
	if err := json.Unmarshal(responseEnvelope.Payload, &response); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
	wantResponse := HandshakeResponse{
		ProtocolVersion: ProtocolVersionV1,
		EnvironmentID:   "env-01JZ8M4M7Y6W8D2N9Z8Q1P4R5S",
		InstanceID:      "runtime-01JZ8M4S2N0Y8F6Q3T7V9X1K2C",
		ServerVersion:   "0.1.0",
		Capabilities: []Capability{
			CapabilityEnvironmentRead,
			CapabilityWorkspaceList,
			CapabilityThreadRead,
			CapabilityThreadExecute,
			CapabilityMessageReply,
			CapabilityFileRead,
		},
		Limits: DefaultTransportLimits(),
	}
	if !reflect.DeepEqual(response, wantResponse) {
		t.Fatalf("response = %#v, want %#v", response, wantResponse)
	}
}

func TestUnknownFieldsAreIgnored(t *testing.T) {
	envelope := decodeFixture(t, "unknown-fields.json")
	if envelope.Operation != OperationEnvironmentStatus {
		t.Fatalf("operation = %q, want %q", envelope.Operation, OperationEnvironmentStatus)
	}
	var payload struct {
		IncludeCapabilities bool `json:"includeCapabilities"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !payload.IncludeCapabilities {
		t.Fatal("includeCapabilities = false, want true")
	}
}

func TestEnvelopeRejectsIncompatibleProtocol(t *testing.T) {
	envelope := decodeFixture(t, "handshake-request.json")
	envelope.ProtocolVersion = CurrentProtocolVersion + 1
	err := envelope.Validate()
	if got := ErrorCodeOf(err); got != ErrorProtocolIncompatible {
		t.Fatalf("ErrorCodeOf(%v) = %q, want %q", err, got, ErrorProtocolIncompatible)
	}
}

func TestEnvelopeRejectsInvalidChunk(t *testing.T) {
	envelope := decodeFixture(t, "chunk-event.json")
	envelope.Chunk.SegmentID = envelope.Chunk.SegmentCount
	err := envelope.Validate()
	if got := ErrorCodeOf(err); got != ErrorInvalidEnvelope {
		t.Fatalf("ErrorCodeOf(%v) = %q, want %q", err, got, ErrorInvalidEnvelope)
	}
}

func TestKnownOperationAllowlist(t *testing.T) {
	if !IsKnownOperation(OperationThreadSubmit) {
		t.Fatalf("%q should be a known operation", OperationThreadSubmit)
	}
	if IsKnownOperation(Operation("command.run")) {
		t.Fatal("command.run unexpectedly accepted as a known operation")
	}
	for _, operation := range []Operation{"file.write", "file.delete", "file.rename", "file.move"} {
		if IsKnownOperation(operation) {
			t.Fatalf("remote file mutation %q unexpectedly accepted", operation)
		}
	}
}

func TestUnknownOperationIsRejected(t *testing.T) {
	envelope := decodeFixture(t, "thread-submit-request.json")
	envelope.Operation = Operation("http.proxy")
	if got := ErrorCodeOf(envelope.Validate()); got != ErrorOperationDenied {
		t.Fatalf("unknown operation error = %q, want %q", got, ErrorOperationDenied)
	}
}

func TestMutatingOperationClassification(t *testing.T) {
	if !RequiresIdempotency(OperationThreadSubmit) {
		t.Fatal("thread.submit should require idempotency")
	}
	if RequiresIdempotency(OperationThreadSnapshot) {
		t.Fatal("thread.snapshot should not require idempotency")
	}
	if !RequiresIdempotency(OperationThreadQueueRemove) || !RequiresIdempotency(OperationThreadQueuePromote) {
		t.Fatal("thread queue mutations should require idempotency")
	}
	if !RequiresIdempotency(OperationFilePreviewOpen) {
		t.Fatal("file.preview.open should replay the same short-lived handle")
	}
}

func TestEncodeMessageAllowsChunkableLogicalEnvelope(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"value": string(bytes.Repeat([]byte("a"), MaxFrameBytes))})
	if err != nil {
		t.Fatal(err)
	}
	envelope := Envelope{
		ProtocolVersion: CurrentProtocolVersion,
		Type:            EnvelopeTypeResponse, ClientID: "client-a", StreamID: "stream-a",
		SeqID: 1, RequestID: "request-a", Payload: payload,
	}
	message, err := EncodeMessage(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(message) <= MaxFrameBytes {
		t.Fatalf("logical message size = %d, want over frame limit", len(message))
	}
	if _, err := EncodeEnvelope(envelope); ErrorCodeOf(err) != ErrorInvalidEnvelope {
		t.Fatalf("EncodeEnvelope error = %v, want frame limit", err)
	}
}

func TestResyncFixtureUsesSharedDetailsType(t *testing.T) {
	envelope := decodeFixture(t, "resync-required.json")
	if envelope.Error == nil || envelope.Error.Code != ErrorResyncRequired {
		t.Fatalf("error = %+v, want %q", envelope.Error, ErrorResyncRequired)
	}
	var details ResyncRequiredDetails
	if err := json.Unmarshal(envelope.Error.Details, &details); err != nil {
		t.Fatalf("decode resync details: %v", err)
	}
	if details.SnapshotOperation != OperationThreadSnapshot || details.StreamID == "" {
		t.Fatalf("details = %+v", details)
	}
}

func TestEnvelopeLimits(t *testing.T) {
	t.Run("frame", func(t *testing.T) {
		_, err := DecodeEnvelope(bytes.Repeat([]byte(" "), MaxFrameBytes+1))
		if got := ErrorCodeOf(err); got != ErrorInvalidEnvelope {
			t.Fatalf("error = %q, want %q", got, ErrorInvalidEnvelope)
		}
	})
	t.Run("identifier", func(t *testing.T) {
		envelope := decodeFixture(t, "thread-submit-request.json")
		envelope.RequestID = string(make([]byte, MaxIdentifierBytes+1))
		if got := ErrorCodeOf(envelope.Validate()); got != ErrorInvalidEnvelope {
			t.Fatalf("error = %q, want %q", got, ErrorInvalidEnvelope)
		}
	})
	t.Run("chunk-count", func(t *testing.T) {
		envelope := decodeFixture(t, "chunk-event.json")
		envelope.Chunk.SegmentCount = MaxChunkCount + 1
		if got := ErrorCodeOf(envelope.Validate()); got != ErrorInvalidEnvelope {
			t.Fatalf("error = %q, want %q", got, ErrorInvalidEnvelope)
		}
	})
	t.Run("message-size", func(t *testing.T) {
		envelope := decodeFixture(t, "chunk-event.json")
		envelope.Chunk.MessageSizeBytes = MaxMessageBytes + 1
		if got := ErrorCodeOf(envelope.Validate()); got != ErrorInvalidEnvelope {
			t.Fatalf("error = %q, want %q", got, ErrorInvalidEnvelope)
		}
	})
	t.Run("base64", func(t *testing.T) {
		envelope := decodeFixture(t, "chunk-event.json")
		envelope.Chunk.MessageChunkBase64 = "***"
		if got := ErrorCodeOf(envelope.Validate()); got != ErrorInvalidEnvelope {
			t.Fatalf("error = %q, want %q", got, ErrorInvalidEnvelope)
		}
	})
}

func decodeFixture(t *testing.T, name string) Envelope {
	t.Helper()
	envelope, err := DecodeEnvelope(readFixture(t, name))
	if err != nil {
		t.Fatalf("DecodeEnvelope(%s): %v", name, err)
	}
	return envelope
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", name, err)
	}
	return raw
}

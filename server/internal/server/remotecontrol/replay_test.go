package remotecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

func TestMutatingRequestReplaysStoredResult(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	var calls atomic.Int32
	registerSubmitHandler(t, dispatcher, func() {
		calls.Add(1)
	})
	principal := testPrincipal(t, protocol.CapabilityThreadExecute)
	request := testRequest(protocol.OperationThreadSubmit)

	first := dispatcher.Dispatch(context.Background(), principal, request)
	second := dispatcher.Dispatch(context.Background(), principal, request)
	if first.Error != nil || second.Error != nil {
		t.Fatalf("responses contain errors: first=%+v second=%+v", first.Error, second.Error)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestRequestIDConflictRejectsDifferentContent(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	registerSubmitHandler(t, dispatcher, func() {})
	principal := testPrincipal(t, protocol.CapabilityThreadExecute)
	request := testRequest(protocol.OperationThreadSubmit)
	dispatcher.Dispatch(context.Background(), principal, request)

	request.Payload = json.RawMessage(`{"text":"different"}`)
	assertErrorCode(t, dispatcher.Dispatch(context.Background(), principal, request), protocol.ErrorRequestConflict)
}

func TestConcurrentRetryExecutesOnce(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	if err := dispatcher.Register(
		protocol.OperationThreadSubmit,
		protocol.CapabilityThreadExecute,
		func(context.Context, Principal, json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
			calls.Add(1)
			close(started)
			<-release
			return json.RawMessage(`{"accepted":true}`), nil
		},
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
	principal := testPrincipal(t, protocol.CapabilityThreadExecute)
	request := testRequest(protocol.OperationThreadSubmit)
	firstDone := make(chan protocol.Envelope, 1)
	go func() {
		firstDone <- dispatcher.Dispatch(context.Background(), principal, request)
	}()
	<-started
	secondDone := make(chan protocol.Envelope, 1)
	go func() {
		secondDone <- dispatcher.Dispatch(context.Background(), principal, request)
	}()
	close(release)

	for _, responseChannel := range []<-chan protocol.Envelope{firstDone, secondDone} {
		select {
		case response := <-responseChannel:
			if response.Error != nil {
				t.Fatalf("Dispatch error = %+v", response.Error)
			}
		case <-time.After(time.Second):
			t.Fatal("Dispatch timed out")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestOversizedMutatingResultIsReplacedAndReplayed(t *testing.T) {
	dispatcher := NewDispatcher(Config{
		Enabled:              true,
		KillSwitch:           false,
		MaxReplayResultBytes: 32,
	})
	var calls atomic.Int32
	if err := dispatcher.Register(
		protocol.OperationThreadSubmit,
		protocol.CapabilityThreadExecute,
		func(context.Context, Principal, json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
			calls.Add(1)
			return bytes.Repeat([]byte("x"), 33), nil
		},
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
	principal := testPrincipal(t, protocol.CapabilityThreadExecute)
	request := testRequest(protocol.OperationThreadSubmit)
	for range 2 {
		assertErrorCode(t, dispatcher.Dispatch(context.Background(), principal, request), protocol.ErrorInternal)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func registerSubmitHandler(t *testing.T, dispatcher *Dispatcher, called func()) {
	t.Helper()
	if err := dispatcher.Register(
		protocol.OperationThreadSubmit,
		protocol.CapabilityThreadExecute,
		func(context.Context, Principal, json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
			called()
			return json.RawMessage(`{"accepted":true}`), nil
		},
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

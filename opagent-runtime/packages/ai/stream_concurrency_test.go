package ai

import (
	"errors"
	"sync"
	"testing"
)

func TestProviderEventStreamConcurrentCloseIsIdempotent(t *testing.T) {
	stream := NewProviderEventStream(1)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream.Close()
		}()
	}
	wg.Wait()
	if stream.Next() {
		t.Fatal("Next() = true after close, want false")
	}
	if stream.Emit(ProviderEvent{Type: EventCanonicalStart}) {
		t.Fatal("Emit() = true after close, want false")
	}
}

func TestProviderEventStreamFinishDrainsFailureBeforeClosing(t *testing.T) {
	stream := NewProviderEventStream(1)
	wantErr := errors.New("stream failed")
	stream.Finish(wantErr)
	if !stream.Next() || stream.Event().Type != EventCanonicalError {
		t.Fatalf("Event() = %#v, want canonical error", stream.Event())
	}
	if stream.Next() {
		t.Fatal("Next() = true after terminal error, want false")
	}
	if !errors.Is(stream.Err(), wantErr) {
		t.Fatalf("Err() = %v, want %v", stream.Err(), wantErr)
	}
}

func TestResponsesEventStreamConcurrentCloseIsIdempotent(t *testing.T) {
	stream := NewResponsesEventStream(1)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream.Close()
		}()
	}
	wg.Wait()
	if stream.Next() {
		t.Fatal("Next() = true after close, want false")
	}
	if stream.Emit(ResponsesStreamEvent{Type: "response.created"}) {
		t.Fatal("Emit() = true after close, want false")
	}
}

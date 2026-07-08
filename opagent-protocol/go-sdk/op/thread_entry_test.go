package op

import (
	"encoding/json"
	"testing"
)

func TestThreadEntryPreservesRawWireObject(t *testing.T) {
	raw := []byte(`{"type":"canonical_message","id":"entry-1","parentId":"entry-0","timestamp":"2026-06-24T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"extra":{"kept":true}}`)

	entry, err := DecodeThreadEntry(raw)
	if err != nil {
		t.Fatalf("DecodeThreadEntry: %v", err)
	}
	if entry.Type != ThreadEntryTypeCanonicalMessage {
		t.Fatalf("Type = %q, want %q", entry.Type, ThreadEntryTypeCanonicalMessage)
	}
	if entry.ID != "entry-1" {
		t.Fatalf("ID = %q, want entry-1", entry.ID)
	}
	if entry.ParentID == nil || *entry.ParentID != "entry-0" {
		t.Fatalf("ParentID = %+v, want entry-0", entry.ParentID)
	}
	if entry.Timestamp != "2026-06-24T00:00:00Z" {
		t.Fatalf("Timestamp = %q, want timestamp", entry.Timestamp)
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var encodedObject map[string]any
	if err := json.Unmarshal(encoded, &encodedObject); err != nil {
		t.Fatalf("encoded object: %v", err)
	}
	if _, ok := encodedObject["raw"]; ok {
		t.Fatalf("encoded entry unexpectedly wrapped raw: %s", encoded)
	}
	if encodedObject["extra"] == nil {
		t.Fatalf("encoded entry dropped unknown fields: %s", encoded)
	}
}

func TestThreadEntryWindowQueryWireShape(t *testing.T) {
	raw, err := json.Marshal(ThreadMetaQuery{
		ThreadID: "thread-1",
		EntryWindow: &ThreadEntryWindowQuery{
			Mode:     ThreadEntryWindowModeBefore,
			AnchorID: "entry-5",
			Limit:    200,
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("Unmarshal wire: %v", err)
	}
	window, ok := wire["entryWindow"].(map[string]any)
	if !ok {
		t.Fatalf("entryWindow missing from wire: %s", raw)
	}
	if window["mode"] != ThreadEntryWindowModeBefore {
		t.Fatalf("mode = %v, want %q", window["mode"], ThreadEntryWindowModeBefore)
	}
	if window["anchorId"] != "entry-5" {
		t.Fatalf("anchorId = %v, want entry-5", window["anchorId"])
	}
	if window["limit"] != float64(200) {
		t.Fatalf("limit = %v, want 200", window["limit"])
	}

	var decoded ThreadMetaQuery
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal query: %v", err)
	}
	if decoded.EntryWindow == nil || decoded.EntryWindow.Mode != ThreadEntryWindowModeBefore || decoded.EntryWindow.AnchorID != "entry-5" || decoded.EntryWindow.Limit != 200 {
		t.Fatalf("decoded.EntryWindow = %+v", decoded.EntryWindow)
	}
}

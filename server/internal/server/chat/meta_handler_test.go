package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/gin-gonic/gin"
)

func TestMetaHandlerMissingSessionReturnsNotFoundThroughProtocolError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		nil,
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req.Params.OpCode != op.OpThreadMetaGet {
				t.Fatalf("received node opcode %s, want %s", req.Params.OpCode, op.OpThreadMetaGet)
			}
			return nil, os.ErrNotExist
		},
	)
	defer cleanup()

	req := httptest.NewRequest("GET", "/v1/thread/meta?threadID=thread-missing", nil)
	rr := httptest.NewRecorder()
	router := gin.New()
	router.GET("/v1/thread/meta", NewMetaHandler(service).Get)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "thread not found" {
		t.Fatalf("unexpected error body: %+v", body)
	}
}

func TestMetaHandlerSnapshotReturnsEntriesAndRevision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		nil,
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req.Params.OpCode != op.OpThreadSnapshotGet {
				t.Fatalf("received node opcode %s, want %s", req.Params.OpCode, op.OpThreadSnapshotGet)
			}
			if req.Params.Meta["modelKey"] != "test:model" {
				t.Fatalf("modelKey meta = %v, want test:model", req.Params.Meta["modelKey"])
			}
			var query op.ThreadMetaQuery
			if err := decodeJSONContent(req.Params.Content, &query); err != nil {
				t.Fatalf("decode query: %v", err)
			}
			if query.ThreadID != "thread-test" {
				t.Fatalf("query.ThreadID = %q, want thread-test", query.ThreadID)
			}
			if query.EntryWindow == nil || query.EntryWindow.Mode != op.ThreadEntryWindowModeBefore || query.EntryWindow.AnchorID != "entry-10" || query.EntryWindow.Limit != 200 {
				t.Fatalf("query.EntryWindow = %+v, want before entry-10 limit 200", query.EntryWindow)
			}
			raw, err := json.Marshal(ai.ThreadSnapshot{
				Meta: op.ThreadMeta{
					ThreadID: "thread-test",
					AgentID:  "agent-id",
					CWD:      "/tmp/workspace",
					Title:    "Snapshot",
				},
				Entries: []op.ThreadEntry{{
					Type:      op.ThreadEntryTypeCanonicalMessage,
					ID:        "entry-1",
					Timestamp: "2026-06-24T00:00:00Z",
					Raw:       json.RawMessage(`{"type":"canonical_message","id":"entry-1","timestamp":"2026-06-24T00:00:00Z","message":{"role":"user"}}`),
				}},
				EntryWindow: op.ThreadEntryWindow{
					Mode:      op.ThreadEntryWindowModeBefore,
					AnchorID:  "entry-10",
					Limit:     200,
					Start:     0,
					End:       1,
					Total:     10,
					HasBefore: false,
					HasAfter:  true,
				},
				Revision: "entry-1",
			})
			if err != nil {
				t.Fatalf("marshal snapshot: %v", err)
			}
			return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
		},
	)
	defer cleanup()

	req := httptest.NewRequest("GET", "/v1/thread/snapshot?threadID=thread-test&modelKey=test:model&entryWindow=before&entryAnchorId=entry-10&entryLimit=200", nil)
	rr := httptest.NewRecorder()
	router := gin.New()
	router.GET("/v1/thread/snapshot", NewMetaHandler(service).Snapshot)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["messages"]; ok {
		t.Fatalf("snapshot response contains removed messages field: %s", rr.Body.String())
	}
	var revision string
	if err := json.Unmarshal(body["revision"], &revision); err != nil {
		t.Fatalf("decode revision: %v", err)
	}
	if revision != "entry-1" {
		t.Fatalf("revision = %q, want entry-1", revision)
	}
	var entries []map[string]any
	if err := json.Unmarshal(body["entries"], &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 1 || entries[0]["id"] != "entry-1" || entries[0]["type"] != op.ThreadEntryTypeCanonicalMessage {
		t.Fatalf("entries = %+v, want canonical entry", entries)
	}
}

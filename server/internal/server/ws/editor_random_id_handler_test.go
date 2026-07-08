package ws

import (
	"testing"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/rs/xid"
)

func TestHandleEditorRandomIDReturnsXID(t *testing.T) {
	handler := &Handler{}

	result, rpcErr := handler.handleEditorRandomID()
	if rpcErr != nil {
		t.Fatalf("handleEditorRandomID() rpcErr = %+v", rpcErr)
	}

	randomID, ok := result.(protocol.EditorRandomIDResult)
	if !ok {
		t.Fatalf("result type = %T, want protocol.EditorRandomIDResult", result)
	}
	if randomID.ID == "" {
		t.Fatal("random id is empty")
	}
	if _, err := xid.FromString(randomID.ID); err != nil {
		t.Fatalf("random id %q is not a valid xid: %v", randomID.ID, err)
	}
}

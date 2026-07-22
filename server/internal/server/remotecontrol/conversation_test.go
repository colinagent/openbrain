package remotecontrol

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	remoteprotocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/colinagent/openbrain/server/internal/server/chat"
)

type fakeConversationRuntime struct {
	config *op.Config
	system *op.SystemConfigResult
	nodes  []*op.OpNode
	active []op.ThreadRuntimeInfo
}

func (f *fakeConversationRuntime) GetConfigContext(context.Context) (*op.Config, error) {
	return f.config, nil
}

func (f *fakeConversationRuntime) GetSystemConfig(context.Context) (*op.SystemConfigResult, error) {
	return f.system, nil
}

func (f *fakeConversationRuntime) ListNodes(context.Context) ([]*op.OpNode, error) {
	return f.nodes, nil
}

func (f *fakeConversationRuntime) ListActiveThreads(context.Context) ([]op.ThreadRuntimeInfo, error) {
	return append([]op.ThreadRuntimeInfo(nil), f.active...), nil
}

func (f *fakeConversationRuntime) CallAgent(context.Context, op.OpCode, op.Meta, op.Content) (*op.OpAgentResult, error) {
	return nil, nil
}

func TestConversationHandlersEnforceWorkspaceAuthorityAndSanitizeSnapshot(t *testing.T) {
	baseDir := t.TempDir()
	workspacePath := filepath.Join(baseDir, "workspace")
	otherPath := filepath.Join(baseDir, "other")
	if err := os.MkdirAll(filepath.Join(baseDir, "threads"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeThreadHeader(t, baseDir, op.ThreadHeader{
		Type: "thread", Version: 1, ID: "thread-owned", AgentID: "agent-coder",
		CWD: workspacePath, Title: "Owned thread",
	})
	writeThreadHeader(t, baseDir, op.ThreadHeader{
		Type: "thread", Version: 1, ID: "thread-other", AgentID: "agent-coder",
		CWD: otherPath, Title: "Other thread",
	})

	runtimeView := &fakeConversationRuntime{
		config: &op.Config{User: &op.UserConfig{
			Models: []op.ModelConfig{{Key: "model-a", ID: "model-a", Name: "Model A", Enabled: true}},
		}},
		system: &op.SystemConfigResult{
			SystemConfig:     op.SystemConfig{HostID: "host-a", BaseDir: baseDir},
			DefaultWorkspace: workspacePath,
		},
		nodes: []*op.OpNode{{
			ID: "agent-coder", Kind: string(op.NodeKindAgent), OpCodes: []op.OpCode{op.OpThreadSubmit},
		}},
		active: []op.ThreadRuntimeInfo{{ThreadID: "thread-owned"}},
	}

	var createdCWD string
	chatService, cleanup := newRemoteConversationChatService(t, func(_ context.Context, request *op.OpNodeRequest) (*op.OpNodeResult, error) {
		switch request.Params.OpCode {
		case op.OpThreadMetaGet:
			var query op.ThreadMetaQuery
			decodeTestJSONContent(t, request.Params.Content, &query)
			cwd := workspacePath
			if query.ThreadID == "thread-other" {
				cwd = otherPath
			}
			return testNodeJSONResult(t, op.ThreadMeta{
				ThreadID: query.ThreadID, FileID: "file-a", AgentID: "agent-coder",
				CWD: cwd, Path: filepath.Join(cwd, ".agent/chat/private.md"),
				ChatPath: filepath.Join(cwd, ".agent/chat/private.md"), Title: "Private",
			})
		case op.OpThreadSnapshotGet:
			var query op.ThreadMetaQuery
			decodeTestJSONContent(t, request.Params.Content, &query)
			if query.EntryWindow == nil || query.EntryWindow.Limit != maxRemoteEntryLimit {
				t.Fatalf("snapshot entry limit = %+v, want %d", query.EntryWindow, maxRemoteEntryLimit)
			}
			entryRaw := json.RawMessage(`{"type":"thread_meta_update","id":"entry-a","timestamp":"2026-07-18T00:00:00Z","cwd":"` + workspacePath + `","chatPath":"` + filepath.Join(workspacePath, "private.md") + `"}`)
			return testNodeJSONResult(t, ai.ThreadSnapshot{
				Meta: op.ThreadMeta{
					ThreadID: "thread-owned", FileID: "file-a", AgentID: "agent-coder",
					CWD: workspacePath, Path: filepath.Join(workspacePath, "private.md"),
					ChatPath:       filepath.Join(workspacePath, "private.md"),
					ThreadFilePath: filepath.Join(baseDir, "threads/thread-owned.jsonl"), Title: "Private",
				},
				Entries:     []op.ThreadEntry{{Type: "thread_meta_update", ID: "entry-a", Raw: entryRaw}},
				EntryWindow: op.ThreadEntryWindow{Mode: "tail", Limit: maxRemoteEntryLimit, Start: 0, End: 1, Total: 1},
				Revision:    "entry-a",
				QueuedMessages: op.ThreadQueueSnapshot{FollowUp: []op.ThreadQueueItem{{
					ID: "queue-a", CWD: workspacePath, Message: op.Message{Role: op.RoleUser, Content: "queued"},
				}}},
				MessageRecords: []op.MessageRecord{{
					ID: "message-a", ChannelID: "channel-a", ThreadID: "thread-owned", AgentID: "agent-coder",
					Sender: op.MessageSenderAgent, Kind: op.MessageKindMessage, Status: op.MessageStatusOpen,
					Body: "hello", Meta: op.Meta{"cwd": workspacePath, "path": filepath.Join(workspacePath, "private.md")},
				}},
			})
		case op.OpThreadCreate:
			var params op.ThreadCreateParams
			decodeTestJSONContent(t, request.Params.Content, &params)
			createdCWD = params.CWD
			return testNodeJSONResult(t, op.ThreadCreateResult{
				ThreadID: "thread-created", FileID: "file-created", Title: params.Title, CWD: params.CWD,
			})
		default:
			t.Fatalf("unexpected node opcode %s", request.Params.OpCode)
			return nil, nil
		}
	})
	defer cleanup()

	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	if err := RegisterConversationHandlers(dispatcher, runtimeView, chatService); err != nil {
		t.Fatal(err)
	}
	principal := testPrincipal(t,
		remoteprotocol.CapabilityThreadRead,
		remoteprotocol.CapabilityThreadExecute,
		remoteprotocol.CapabilityMessageReply,
	)
	workspace, err := defaultWorkspace(context.Background(), runtimeView)
	if err != nil {
		t.Fatal(err)
	}

	list := dispatchConversation(t, dispatcher, principal, remoteprotocol.OperationThreadList, map[string]any{
		"workspaceID": workspace.ID, "limit": 50,
	})
	if strings.Contains(string(list.Payload), "thread-other") || !strings.Contains(string(list.Payload), "thread-owned") {
		t.Fatalf("thread list crossed workspace boundary: %s", list.Payload)
	}
	if !strings.Contains(string(list.Payload), `"running":true`) {
		t.Fatalf("thread list did not include runtime state: %s", list.Payload)
	}

	snapshot := dispatchConversation(t, dispatcher, principal, remoteprotocol.OperationThreadSnapshot, map[string]any{
		"workspaceID": workspace.ID, "threadID": "thread-owned",
		"window": map[string]any{"mode": "tail", "limit": 9999},
	})
	if snapshot.Error != nil {
		t.Fatalf("snapshot failed: %+v", snapshot.Error)
	}
	if strings.Contains(string(snapshot.Payload), baseDir) || strings.Contains(string(snapshot.Payload), workspacePath) {
		t.Fatalf("snapshot leaked a host path: %s", snapshot.Payload)
	}

	crossWorkspace := dispatchConversation(t, dispatcher, principal, remoteprotocol.OperationThreadSnapshot, map[string]any{
		"workspaceID": workspace.ID, "threadID": "thread-other",
	})
	if crossWorkspace.Error == nil || crossWorkspace.Error.Code != remoteprotocol.ErrorThreadNotFound {
		t.Fatalf("cross-workspace snapshot error = %+v", crossWorkspace.Error)
	}

	unknownPath := dispatchConversation(t, dispatcher, principal, remoteprotocol.OperationThreadCreate, map[string]any{
		"workspaceID": workspace.ID, "agentID": "agent-coder", "title": "Created", "cwd": otherPath,
	})
	if unknownPath.Error == nil || unknownPath.Error.Code != remoteprotocol.ErrorInvalidEnvelope {
		t.Fatalf("client-supplied cwd was accepted: %+v", unknownPath)
	}

	created := dispatchConversation(t, dispatcher, principal, remoteprotocol.OperationThreadCreate, map[string]any{
		"workspaceID": workspace.ID, "agentID": "agent-coder", "title": "Created",
	})
	if created.Error != nil {
		t.Fatalf("create failed: %+v", created.Error)
	}
	if createdCWD != workspacePath {
		t.Fatalf("created cwd = %q, want host workspace %q", createdCWD, workspacePath)
	}
}

func TestRemoteEnvelopeChunkingPreservesLogicalResponse(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"value": strings.Repeat("a", remoteprotocol.MaxFrameBytes+1024)})
	if err != nil {
		t.Fatal(err)
	}
	envelope := remoteprotocol.Envelope{
		ProtocolVersion: remoteprotocol.CurrentProtocolVersion,
		Type:            remoteprotocol.EnvelopeTypeResponse,
		ClientID:        "client-a",
		StreamID:        "conversation",
		SeqID:           1,
		RequestID:       "request-a",
		Payload:         payload,
	}
	frames, err := encodeRemoteEnvelopeFrames(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) < 2 {
		t.Fatalf("frame count = %d, want chunked response", len(frames))
	}
	var reassembled []byte
	for index, frame := range frames {
		if len(frame) > remoteprotocol.MaxFrameBytes {
			t.Fatalf("chunk %d exceeds frame limit", index)
		}
		chunkEnvelope, err := remoteprotocol.DecodeEnvelope(frame)
		if err != nil {
			t.Fatal(err)
		}
		if chunkEnvelope.Type != remoteprotocol.EnvelopeTypeChunk || int(chunkEnvelope.Chunk.SegmentID) != index {
			t.Fatalf("unexpected chunk metadata: %+v", chunkEnvelope)
		}
		decoded, err := base64.StdEncoding.DecodeString(chunkEnvelope.Chunk.MessageChunkBase64)
		if err != nil {
			t.Fatal(err)
		}
		reassembled = append(reassembled, decoded...)
	}
	var decoded remoteprotocol.Envelope
	if err := json.Unmarshal(reassembled, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if string(decoded.Payload) != string(payload) {
		t.Fatal("reassembled payload changed")
	}
}

func TestValidateMessageAnswersRejectsSpoofedOptionsAndNormalizesLabels(t *testing.T) {
	questions := []op.MessageQuestion{{
		ID: "question-a", Question: "Choose", Options: []op.MessageQuestionOption{{ID: "yes", Label: "Yes"}},
	}}
	if _, ok := validateMessageAnswers(questions, []op.MessageAnswer{{QuestionID: "question-a", OptionID: "no"}}); ok {
		t.Fatal("unknown option was accepted")
	}
	answers, ok := validateMessageAnswers(questions, []op.MessageAnswer{{QuestionID: "question-a", OptionID: "yes", Label: "spoofed"}})
	if !ok || len(answers) != 1 || answers[0].Label != "Yes" {
		t.Fatalf("normalized answers = %+v, ok=%v", answers, ok)
	}
}

func TestNormalizeMessageReplyAnswersAllowsPublishedActionWithoutAnswers(t *testing.T) {
	questions := []op.MessageQuestion{{
		ID: "question-a", Question: "Choose", Options: []op.MessageQuestionOption{{ID: "yes", Label: "Yes"}},
	}}
	answers, ok := normalizeMessageReplyAnswers(questions, nil, "approve")
	if !ok || len(answers) != 0 {
		t.Fatalf("action-only answers = %+v, ok=%v", answers, ok)
	}
	if _, ok := normalizeMessageReplyAnswers(questions, nil, ""); ok {
		t.Fatal("missing action and question answers was accepted")
	}
}

func newRemoteConversationChatService(
	t *testing.T,
	nodeHandler func(context.Context, *op.OpNodeRequest) (*op.OpNodeResult, error),
) (*chat.Service, func()) {
	t.Helper()
	server := op.NewServer(&op.Implementation{Name: "server", Version: "test"}, nil)
	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := op.NewClient(&op.Implementation{Name: "runtime", Version: "test"}, &op.ClientOptions{
		OpNodeHandler: nodeHandler,
	})
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := chat.NewService(nil)
	service.SetHostSession(serverSession)
	return service, func() { _ = clientSession.Close() }
}

func testNodeJSONResult(t *testing.T, value any) (*op.OpNodeResult, error) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
}

func decodeTestJSONContent(t *testing.T, content op.Content, out any) {
	t.Helper()
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		t.Fatalf("content type = %T, want JSON", content)
	}
	if err := json.Unmarshal(jsonContent.Raw, out); err != nil {
		t.Fatal(err)
	}
}

func writeThreadHeader(t *testing.T, baseDir string, header op.ThreadHeader) {
	t.Helper()
	raw, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(baseDir, "threads", header.ID+".jsonl")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func dispatchConversation(
	t *testing.T,
	dispatcher *Dispatcher,
	principal Principal,
	operation remoteprotocol.Operation,
	payload any,
) remoteprotocol.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(append([]byte(operation), raw...))
	return dispatcher.Dispatch(context.Background(), principal, remoteprotocol.Envelope{
		ProtocolVersion: remoteprotocol.CurrentProtocolVersion,
		Type:            remoteprotocol.EnvelopeTypeRequest,
		ClientID:        principal.ClientID,
		StreamID:        "conversation",
		SeqID:           1,
		RequestID:       "request-" + string(operation) + "-" + string(hexDigest(digest[:8])),
		Operation:       operation,
		Payload:         raw,
	})
}

func hexDigest(value []byte) []byte {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(value)*2)
	for index, item := range value {
		out[index*2] = alphabet[item>>4]
		out[index*2+1] = alphabet[item&0x0f]
	}
	return out
}

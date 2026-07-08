package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/builtintools"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type scriptedProvider struct {
	responses []*ai.ProviderResponse
	seen      [][]op.Message
}

type eventedProvider struct {
	streams [][]ai.ProviderEvent
	seen    [][]op.Message
}

type completeOnlyProvider struct {
	response *ai.ProviderResponse
	seen     [][]op.Message
}

func opMessagesFromCanonicalForLoopTool(messages []ai.ConversationMessage) ([]op.Message, error) {
	out := make([]op.Message, 0, len(messages))
	for _, msg := range messages {
		converted, err := ai.OpMessageFromCanonical(msg)
		if err != nil {
			return nil, err
		}
		if converted.Role == "" {
			continue
		}
		out = append(out, converted)
	}
	return out, nil
}

func testProviderResponse(content, reasoning string, toolCalls []op.MessageToolCall, stopReason ai.StopReason) *ai.ProviderResponse {
	return ai.ProviderResponseFromOpMessage(op.Message{
		Role:             op.RoleAssistant,
		Content:          content,
		ReasoningContent: reasoning,
		ToolCalls:        toolCalls,
	}, ai.Usage{}, stopReason)
}

func (p *scriptedProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func (p *eventedProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func (p *completeOnlyProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func testToolCallEvent(call op.MessageToolCall) ai.ProviderEvent {
	return ai.ProviderEvent{
		Type: ai.EventCanonicalToolCallEnd,
		Block: &ai.StreamContentBlock{
			Type: ai.BlockToolCall,
			ToolCall: &ai.StreamToolCall{
				ID:           call.ID,
				Name:         call.Name,
				Arguments:    ai.CloneToolArguments(call.Arguments),
				RawArguments: ai.MarshalToolArgumentsJSON(call.Arguments),
				Complete:     true,
			},
		},
	}
}

func emitTestProviderResponseReplay(stream *ai.ProviderEventStream, resp *ai.ProviderResponse) bool {
	if resp == nil {
		return stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone})
	}
	partial := ai.StreamConversationMessageFromCanonical(resp.Message)
	if partial == nil {
		partial = &ai.StreamConversationMessage{Role: ai.RoleCanonicalAssistant}
	}
	if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalStart, Partial: partial}) {
		return false
	}
	for index := range partial.Content {
		block := &partial.Content[index]
		switch block.Type {
		case ai.BlockText:
			if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalTextStart, ContentIndex: index, Block: block, Partial: partial}) {
				return false
			}
			if strings.TrimSpace(block.Text) != "" {
				if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalTextDelta, ContentIndex: index, Delta: block.Text, Block: block, Partial: partial}) {
					return false
				}
			}
			if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalTextEnd, ContentIndex: index, Content: block.Text, Block: block, Partial: partial}) {
				return false
			}
		case ai.BlockThinking:
			if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalThinkingStart, ContentIndex: index, Block: block, Partial: partial}) {
				return false
			}
			if strings.TrimSpace(block.Text) != "" {
				if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalThinkingDelta, ContentIndex: index, Delta: block.Text, Block: block, Partial: partial}) {
					return false
				}
			}
			if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalThinkingEnd, ContentIndex: index, Content: block.Text, Block: block, Partial: partial}) {
				return false
			}
		case ai.BlockToolCall:
			if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalToolCallStart, ContentIndex: index, Block: block, Partial: partial}) {
				return false
			}
			if block.ToolCall != nil && strings.TrimSpace(block.ToolCall.RawArguments) != "" {
				if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalToolCallDelta, ContentIndex: index, Delta: block.ToolCall.RawArguments, Block: block, Partial: partial}) {
					return false
				}
			}
			if !stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalToolCallEnd, ContentIndex: index, Block: block, Partial: partial}) {
				return false
			}
		}
	}
	return stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: resp})
}

func (p *scriptedProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, fmt.Errorf("unexpected CompleteCanonical call")
}

func (p *scriptedProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("req is nil")
	}
	index := len(p.seen)
	if index >= len(p.responses) {
		return nil, fmt.Errorf("unexpected StreamCanonical call %d", index)
	}
	msgs, err := opMessagesFromCanonicalForLoopTool(req.Context.Messages)
	if err != nil {
		return nil, err
	}
	p.seen = append(p.seen, msgs)
	stream := ai.NewProviderEventStream(1)
	resp := p.responses[index]
	go func() {
		_ = emitTestProviderResponseReplay(stream, resp)
		stream.Close()
	}()
	return stream, nil
}

func (p *eventedProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, fmt.Errorf("unexpected CompleteCanonical call")
}

func (p *eventedProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("req is nil")
	}
	index := len(p.seen)
	if index >= len(p.streams) {
		return nil, fmt.Errorf("unexpected StreamCanonical call %d", index)
	}
	msgs, err := opMessagesFromCanonicalForLoopTool(req.Context.Messages)
	if err != nil {
		return nil, err
	}
	p.seen = append(p.seen, msgs)
	events := p.streams[index]
	stream := ai.NewProviderEventStream(len(events))
	go func() {
		for _, event := range events {
			if !stream.Emit(event) {
				return
			}
		}
		stream.Close()
	}()
	return stream, nil
}

func (p *completeOnlyProvider) CompleteCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("req is nil")
	}
	msgs, err := opMessagesFromCanonicalForLoopTool(req.Context.Messages)
	if err != nil {
		return nil, err
	}
	p.seen = append(p.seen, msgs)
	return p.response, nil
}

func (p *completeOnlyProvider) StreamCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	return nil, ai.ErrStreamingNotSupported
}

type testToolInput struct {
	Path string `json:"path"`
}

func drainNotifyChan() {
	for {
		select {
		case <-notifyChan:
		default:
			return
		}
	}
}

func waitNotifyMessage(t *testing.T) *op.InfoNotificationParams {
	t.Helper()
	select {
	case msg := <-notifyChan:
		if msg == nil {
			t.Fatal("received nil notify message")
		}
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notify message")
		return nil
	}
}

func waitNotifyMessages(t *testing.T, count int) []*op.InfoNotificationParams {
	t.Helper()
	out := make([]*op.InfoNotificationParams, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, waitNotifyMessage(t))
	}
	return out
}

func waitNotifyMessagesExcludingType(t *testing.T, count int, excludedType string) []*op.InfoNotificationParams {
	t.Helper()
	out := make([]*op.InfoNotificationParams, 0, count)
	for len(out) < count {
		msg := waitNotifyMessage(t)
		if got, _ := msg.Meta["type"].(string); got == excludedType {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func waitNotifyMessagesExcludingTypes(t *testing.T, count int, excludedTypes ...string) []*op.InfoNotificationParams {
	t.Helper()
	excluded := make(map[string]struct{}, len(excludedTypes))
	for _, typ := range excludedTypes {
		excluded[typ] = struct{}{}
	}
	out := make([]*op.InfoNotificationParams, 0, count)
	for len(out) < count {
		msg := waitNotifyMessage(t)
		if got, _ := msg.Meta["type"].(string); got != "" {
			if _, skip := excluded[got]; skip {
				continue
			}
		}
		out = append(out, msg)
	}
	return out
}

func waitNotifyMessagesOfType(t *testing.T, count int, expectedType string) []*op.InfoNotificationParams {
	t.Helper()
	out := make([]*op.InfoNotificationParams, 0, count)
	for len(out) < count {
		msg := waitNotifyMessage(t)
		if got, _ := msg.Meta["type"].(string); got != expectedType {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func drainNotifyMessagesAfter(t *testing.T, wait time.Duration) []*op.InfoNotificationParams {
	t.Helper()
	deadline := time.After(wait)
	out := make([]*op.InfoNotificationParams, 0)
	for {
		select {
		case msg := <-notifyChan:
			if msg != nil {
				out = append(out, msg)
			}
		case <-deadline:
			return out
		}
	}
}

func notifyText(t *testing.T, content op.Content) string {
	t.Helper()
	text, ok := content.(*op.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", content)
	}
	return text.Text
}

func notifyMetaInt64(t *testing.T, meta op.Meta, key string) int64 {
	t.Helper()
	raw, ok := meta[key]
	if !ok {
		t.Fatalf("expected meta[%q] to exist", key)
	}
	value, err := strconv.ParseInt(fmt.Sprint(raw), 10, 64)
	if err != nil {
		t.Fatalf("parse meta[%q]=%v: %v", key, raw, err)
	}
	return value
}

func notifyMetaString(t *testing.T, meta op.Meta, key string) string {
	t.Helper()
	raw, ok := meta[key]
	if !ok {
		t.Fatalf("expected meta[%q] to exist", key)
	}
	return fmt.Sprint(raw)
}

func notifyJSONMessage(t *testing.T, content op.Content) op.Message {
	t.Helper()
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		t.Fatalf("expected json content, got %T", content)
	}
	var msg op.Message
	if err := jsonContent.Unmarshal(&msg); err != nil {
		t.Fatalf("unmarshal json content: %v", err)
	}
	return msg
}

func notifyJSONPayload[T any](t *testing.T, content op.Content) T {
	t.Helper()
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		t.Fatalf("expected json content, got %T", content)
	}
	var payload T
	if err := jsonContent.Unmarshal(&payload); err != nil {
		t.Fatalf("unmarshal json content: %v", err)
	}
	return payload
}

func connectTestToolServer(t *testing.T, serverID string, handler func(testToolInput) string) *op.ClientSession {
	t.Helper()

	server := op.NewServer(&op.Implementation{Name: "tool-server", Version: "v0.0.1"}, nil)
	for _, name := range []string{"read_file", "read", "shell"} {
		toolName := name
		op.AddTool(server, &op.Tool{
			Name:        toolName,
			Description: "Test tool " + toolName,
		}, func(_ context.Context, _ *op.CallToolRequest, input testToolInput) (*op.CallToolResult, any, error) {
			return &op.CallToolResult{
				Content: []op.Content{&op.TextContent{Text: handler(input)}},
			}, nil, nil
		})
	}

	t1, t2 := op.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), t1, nil); err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})

	node := op.OpNode{ID: serverID, Kind: string(op.NodeKindTools)}
	cache.SetValue(serverID, cache.PrefixNode, node, cache.NoExpiration)
	SetConn(&Connection{NodeID: serverID, Session: session, Ctx: context.Background(), Daemon: true})
	return session
}

func startStreamableHTTPTestToolServer(t *testing.T, handler op.ToolHandler) (*httptest.Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()

	var toolCalls atomic.Int32
	var initializes atomic.Int32
	server := op.NewServer(&op.Implementation{Name: "tool-server", Version: "v0.0.1"}, nil)
	server.AddTool(&op.Tool{
		Name:        "read",
		Description: "Test read tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, req *op.CallToolRequest) (*op.CallToolResult, error) {
		toolCalls.Add(1)
		return handler(ctx, req)
	})

	mcpHandler := op.NewStreamableHTTPHandler(func(*http.Request) *op.Server {
		return server
	}, &op.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
			var req struct {
				Method string `json:"method"`
			}
			if err := json.Unmarshal(body, &req); err == nil && req.Method == "initialize" {
				initializes.Add(1)
			}
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	return httpServer, &toolCalls, &initializes
}

func TestCallToolRecoversClosedCachedDaemonSessionOnceForConcurrentCalls(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	httpServer, toolCalls, initializes := startStreamableHTTPTestToolServer(t, func(_ context.Context, _ *op.CallToolRequest) (*op.CallToolResult, error) {
		return &op.CallToolResult{
			Content: []op.Content{&op.TextContent{Text: "recovered"}},
		}, nil
	})
	node := op.OpNode{
		ID:   "tools-stale-streamable",
		Kind: string(op.NodeKindTools),
		Run:  op.Run{URL: httpServer.URL, Daemon: true},
	}
	cache.SetValue(node.ID, cache.PrefixNode, node, cache.NoExpiration)
	staleConn, err := CreateConnection(context.Background(), &node)
	if err != nil {
		t.Fatalf("CreateConnection(): %v", err)
	}
	if err := staleConn.Session.Close(); err != nil {
		t.Fatalf("close stale session: %v", err)
	}

	const callers = 8
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			text, _, err := callTool(&Loop{Ctx: context.Background(), Meta: op.Meta{}}, ToolCall{
				ID: fmt.Sprintf("call-%d", index),
				Info: toolInfo{
					Name:   "tools-stale-streamable__read",
					Params: map[string]any{"path": fmt.Sprintf("file-%d.md", index)},
				},
			}, node.ID, "read")
			if err != nil {
				errs <- err
				return
			}
			if text != "recovered" {
				errs <- fmt.Errorf("text = %q, want recovered", text)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := toolCalls.Load(); got != callers {
		t.Fatalf("tool calls = %d, want %d", got, callers)
	}
	if got := initializes.Load(); got != 2 {
		t.Fatalf("initialize calls = %d, want initial stale connection plus one recovery", got)
	}
	if cached := GetConn(node.ID); cached == nil || cached == staleConn || cached.Session == nil {
		t.Fatalf("cached connection was not replaced after recovery: %#v", cached)
	}
}

func TestCallToolDoesNotRetryOrdinaryToolError(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	httpServer, toolCalls, _ := startStreamableHTTPTestToolServer(t, func(_ context.Context, _ *op.CallToolRequest) (*op.CallToolResult, error) {
		return nil, fmt.Errorf("ordinary tool failure")
	})
	node := op.OpNode{
		ID:   "tools-error-streamable",
		Kind: string(op.NodeKindTools),
		Run:  op.Run{URL: httpServer.URL, Daemon: true},
	}
	cache.SetValue(node.ID, cache.PrefixNode, node, cache.NoExpiration)

	_, _, err := callTool(&Loop{Ctx: context.Background(), Meta: op.Meta{}}, ToolCall{
		ID: "call-1",
		Info: toolInfo{
			Name:   "tools-error-streamable__read",
			Params: map[string]any{"path": "file.md"},
		},
	}, node.ID, "read")
	if err == nil {
		t.Fatal("callTool succeeded, want ordinary tool failure")
	}
	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("tool calls = %d, want 1", got)
	}
}

func TestToolResultMessageFromCallResultPreservesImageParts(t *testing.T) {
	msg := toolResultMessageFromCallResult("read", "call-1", "Read image file [image/png]", &op.CallToolResult{
		Content: []op.Content{
			&op.TextContent{Text: "Read image file [image/png]"},
			&op.ImageContent{Data: []byte{1, 2, 3}, MIMEType: "image/png"},
		},
	})
	if msg.Content != "Read image file [image/png]" {
		t.Fatalf("content = %q", msg.Content)
	}
	if len(msg.ContentParts) != 2 {
		t.Fatalf("content parts = %d, want 2", len(msg.ContentParts))
	}
	if msg.ContentParts[1].ImageURL == nil || !strings.HasPrefix(msg.ContentParts[1].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("image part = %+v", msg.ContentParts[1])
	}

	canonical := ai.CanonicalMessagesFromOp([]op.Message{msg})
	if len(canonical) != 1 || len(canonical[0].Content) != 1 || canonical[0].Content[0].ToolResult == nil {
		t.Fatalf("canonical = %+v", canonical)
	}
	output := canonical[0].Content[0].ToolResult.OutputContent
	if len(output) != 2 || output[1].Type != ai.BlockImage {
		t.Fatalf("canonical output content = %+v", output)
	}
}

func TestAssembleTools_UsesMatchingSystemToolAndIncludesToolServerTools(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	cache.SetValue("browser-server", cache.PrefixNode, op.OpNode{
		ID:   "browser-server",
		Kind: string(op.NodeKindTools),
		Meta: &op.ToolsMeta{Tools: []*op.ToolSpec{{ServerID: "browser-server", Name: "browser_click"}}},
	}, cache.NoExpiration)

	tools := assembleTools(context.Background(), &op.AgentMeta{
		SysTools:    []string{"shell", "read"},
		SysToolMode: op.SystoolModeAllowlist,
		ToolServers: []string{"browser-server"},
	})
	if len(tools) != 3 {
		t.Fatalf("assembleTools() len = %d, want 3", len(tools))
	}
	if got := tools["shell"]; got == nil || got.Name != "shell" {
		t.Fatalf("shell tool mismatch: %+v", got)
	}
	if got := tools["read"]; got == nil || got.Name != "read" {
		t.Fatalf("read tool mismatch: %+v", got)
	}
	if got := tools["browser_click"]; got == nil || got.Name != "browser_click" {
		t.Fatalf("tool server tool mismatch: %+v", got)
	}
}

func TestAssembleTools_DefaultsToAllSystoolTools(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	tools := assembleTools(context.Background(), &op.AgentMeta{})
	for _, name := range op.SystoolNames {
		if !builtintools.IsOSToolName(name) {
			continue
		}
		if got := tools[name]; got == nil || got.ServerID != systoolServerID {
			t.Fatalf("tool %s = %+v, want built-in systool spec", name, got)
		}
	}
}

func TestAssembleTools_DisablesSystoolTools(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	tools := assembleTools(context.Background(), &op.AgentMeta{SysToolMode: op.SystoolModeDisabled})
	if len(tools) != 0 {
		t.Fatalf("assembleTools() = %+v, want no tools", tools)
	}
}

func TestAssembleTools_WarnsWhenConfiguredToolServerIsMissing(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)

	tools := assembleTools(context.Background(), &op.AgentMeta{
		SysToolMode: op.SystoolModeDisabled,
		ToolServers: []string{"tools-gbrain-cloud"},
	})
	if len(tools) != 0 {
		t.Fatalf("assembleTools() len = %d, want 0", len(tools))
	}
	if got := logs.String(); !strings.Contains(got, "configured tool server missing from node cache") || !strings.Contains(got, "tools-gbrain-cloud") {
		t.Fatalf("log output = %q, want missing tool-server warning", got)
	}
}

func TestSanitizeToolArgumentsForSchemaDropsOptionalEmptyStrings(t *testing.T) {
	args := map[string]any{
		"query":     "who am I",
		"source_id": "",
		"lang":      "   ",
		"limit":     3,
		"flag":      false,
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":     map[string]any{"type": "string"},
			"source_id": map[string]any{"type": "string"},
			"lang":      map[string]any{"type": []any{"string", "null"}},
			"limit":     map[string]any{"type": "number"},
			"flag":      map[string]any{"type": "boolean"},
		},
		"required": []any{"query"},
	}

	got, ok := sanitizeToolArgumentsForSchema(args, schema).(map[string]any)
	if !ok {
		t.Fatalf("sanitized arguments type = %T, want map[string]any", got)
	}
	if _, ok := got["source_id"]; ok {
		t.Fatalf("source_id was not removed: %#v", got)
	}
	if _, ok := got["lang"]; ok {
		t.Fatalf("lang was not removed: %#v", got)
	}
	if got["query"] != "who am I" || got["limit"] != 3 || got["flag"] != false {
		t.Fatalf("unexpected sanitized arguments: %#v", got)
	}
	if _, ok := args["source_id"]; !ok {
		t.Fatalf("original arguments were mutated: %#v", args)
	}
}

func TestSanitizeToolArgumentsForSchemaKeepsRequiredEmptyString(t *testing.T) {
	args := map[string]any{"query": ""}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
		"required": []any{"query"},
	}

	got, ok := sanitizeToolArgumentsForSchema(args, schema).(map[string]any)
	if !ok {
		t.Fatalf("sanitized arguments type = %T, want map[string]any", got)
	}
	if _, ok := got["query"]; !ok {
		t.Fatalf("required query was removed: %#v", got)
	}
}

func TestExecuteToolCalls_ReturnsToolResultAndSteeringSkipsRemaining(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)
	connectTestToolServer(t, "sys-server", func(input testToolInput) string {
		return "read: " + input.Path
	})

	steeringChecks := 0
	loop := &AgentLoop{
		Ctx: context.Background(),
		Agent: &Agent{ToolSpecs: map[string]*op.ToolSpec{
			"read_file": {ServerID: "sys-server", Name: "read_file"},
		}},
		getSteeringMessages: func(context.Context) ([]PendingLoopMessage, error) {
			steeringChecks++
			if steeringChecks == 1 {
				return []PendingLoopMessage{pendingLoopMessageFromMessage(op.NewUserMessage("interrupt"))}, nil
			}
			return nil, nil
		},
	}

	results, steering, err := loop.executeToolCalls(Loop{Ctx: context.Background(), Meta: op.Meta{}}, op.NewAssistantToolCalls([]op.MessageToolCall{
		{ID: "call-1", Name: "read_file", Arguments: map[string]any{"path": "/etc/passwd"}, Type: "function"},
		{ID: "call-2", Name: "read_file", Arguments: map[string]any{"path": "/etc/hosts"}, Type: "function"},
	}))
	if err != nil {
		t.Fatalf("executeToolCalls(): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Role != op.RoleTool || results[0].ToolCallID != "call-1" || results[0].Content != "read: /etc/passwd" {
		t.Fatalf("unexpected first tool result: %+v", results[0])
	}
	if results[1].Role != op.RoleTool || results[1].ToolCallID != "call-2" || results[1].Content != "Skipped due to queued user message." {
		t.Fatalf("unexpected skipped tool result: %+v", results[1])
	}
	if len(steering) != 1 || steering[0].Message.Role != op.RoleUser || steering[0].Message.Content != "interrupt" {
		t.Fatalf("unexpected steering: %+v", steering)
	}

	notifications := waitNotifyMessages(t, 2)
	if got, _ := notifications[0].Meta["type"].(string); got != "tool_result_step" {
		t.Fatalf("first notification type = %q, want tool_result_step", got)
	}
	if got := notifyJSONMessage(t, notifications[0].Content); got.Content != "read: /etc/passwd" || got.ToolCallID != "call-1" {
		t.Fatalf("first notification message = %+v", got)
	}
	if got, _ := notifications[1].Meta["type"].(string); got != "tool_result_step" {
		t.Fatalf("second notification type = %q, want tool_result_step", got)
	}
	if got := notifyJSONMessage(t, notifications[1].Content); got.Content != "Skipped due to queued user message." || got.ToolCallID != "call-2" {
		t.Fatalf("second notification message = %+v", got)
	}
}

func TestExecuteToolCalls_NormalizesToolNames(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)
	connectTestToolServer(t, "sys-server", func(input testToolInput) string {
		return "read: " + input.Path
	})

	loop := &AgentLoop{
		Ctx: context.Background(),
		Agent: &Agent{ToolSpecs: map[string]*op.ToolSpec{
			"shell": {ServerID: "sys-server", Name: "shell"},
			"read":  {ServerID: "sys-server", Name: "read"},
		}},
	}

	readResults, _, err := loop.executeToolCalls(Loop{Ctx: context.Background(), Meta: op.Meta{}}, op.NewAssistantToolCalls([]op.MessageToolCall{{
		ID: "call-read", Name: "Read", Arguments: map[string]any{"path": "/etc/passwd"}, Type: "function",
	}}))
	if err != nil {
		t.Fatalf("executeToolCalls(Read): %v", err)
	}
	if len(readResults) != 1 || readResults[0].Content != "read: /etc/passwd" || readResults[0].Name != "read" {
		t.Fatalf("unexpected Read result: %+v", readResults)
	}

	shellResults, _, err := loop.executeToolCalls(Loop{Ctx: context.Background(), Meta: op.Meta{}}, op.NewAssistantToolCalls([]op.MessageToolCall{{
		ID: "call-shell", Name: "shell", Arguments: map[string]any{"path": "/etc/group"}, Type: "function",
	}}))
	if err != nil {
		t.Fatalf("executeToolCalls(shell): %v", err)
	}
	if len(shellResults) != 1 || shellResults[0].Content != "read: /etc/group" || shellResults[0].Name != "shell" {
		t.Fatalf("unexpected shell result: %+v", shellResults)
	}
}

func TestExecuteToolCalls_EmitsErrorToolResultNotification(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	loop := &AgentLoop{
		Ctx:   context.Background(),
		Agent: &Agent{ToolSpecs: map[string]*op.ToolSpec{}},
	}

	results, steering, err := loop.executeToolCalls(Loop{Ctx: context.Background(), Meta: op.Meta{"threadID": "thread-1"}}, op.NewAssistantToolCalls([]op.MessageToolCall{{
		ID:        "missing-tool",
		Name:      "read_file",
		Arguments: map[string]any{"path": "/tmp/demo"},
		Type:      "function",
	}}))
	if err != nil {
		t.Fatalf("executeToolCalls(): %v", err)
	}
	if len(steering) != 0 {
		t.Fatalf("unexpected steering: %+v", steering)
	}
	if len(results) != 1 || results[0].Role != op.RoleTool {
		t.Fatalf("unexpected results: %+v", results)
	}

	notification := waitNotifyMessage(t)
	if got, _ := notification.Meta["type"].(string); got != "tool_result_step" {
		t.Fatalf("notification type = %q, want tool_result_step", got)
	}
	if got := notifyJSONMessage(t, notification.Content); got.Content != "tool not found: read_file" || got.ToolCallID != "missing-tool" {
		t.Fatalf("notification message = %+v", got)
	}
}

func TestExecuteToolCalls_AgentTaskRejectsUnmountedSubagent(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	loop := &AgentLoop{
		Ctx: context.Background(),
		Agent: &Agent{
			AgentID: "agent-parent",
			ToolSpecs: map[string]*op.ToolSpec{
				agentTaskToolName: {ServerID: systoolServerID, Name: agentTaskToolName},
			},
		},
	}

	results, steering, err := loop.executeToolCalls(Loop{Ctx: context.Background(), Meta: op.Meta{"threadID": "thread-parent"}}, op.NewAssistantToolCalls([]op.MessageToolCall{{
		ID:        "call-agent-task",
		Name:      agentTaskToolName,
		Arguments: map[string]any{"subagent_id": "agent-gbrain", "task": "look this up"},
		Type:      "function",
	}}))
	if err != nil {
		t.Fatalf("executeToolCalls(): %v", err)
	}
	if len(steering) != 0 {
		t.Fatalf("unexpected steering: %+v", steering)
	}
	if len(results) != 1 || results[0].Role != op.RoleTool {
		t.Fatalf("unexpected results: %+v", results)
	}
	if !strings.Contains(results[0].Content, "not mounted") {
		t.Fatalf("agent_task result = %q, want not mounted error", results[0].Content)
	}

	notification := waitNotifyMessage(t)
	if got := notifyJSONMessage(t, notification.Content); got.ToolCallID != "call-agent-task" || !strings.Contains(got.Content, "not mounted") {
		t.Fatalf("notification message = %+v", got)
	}
}

func TestResolveMountedSubagentAcceptsAtPrefixedID(t *testing.T) {
	loop := &AgentLoop{
		Agent: &Agent{
			AvailableSubagents: []op.OpNode{{ID: "agent-gbrain"}},
		},
	}

	target, err := loop.resolveMountedSubagent("@agent-gbrain")
	if err != nil {
		t.Fatalf("resolveMountedSubagent(): %v", err)
	}
	if target.ID != "agent-gbrain" {
		t.Fatalf("target.ID = %q, want agent-gbrain", target.ID)
	}
}

func TestRunContinuation_ToolResultTailProducesAssistant(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &scriptedProvider{
		responses: []*ai.ProviderResponse{
			testProviderResponse("continued after tool result", "", nil, ai.StopReasonStop),
		},
	}

	loop := &AgentLoop{
		Ctx:      context.Background(),
		Agent:    &Agent{ToolSpecs: map[string]*op.ToolSpec{}},
		Model:    &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		ThreadID: "thread-tool-result-tail",
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{
			op.NewUserMessage("continue"),
			{
				Role:    op.RoleAssistant,
				Content: "",
				ToolCalls: []op.MessageToolCall{{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: map[string]any{"path": "/etc/passwd"},
					Type:      "function",
				}},
			},
			op.NewToolResultMessage("read_file", "call-1", "file body"),
		}),
		Meta: op.Meta{},
	}

	result, err := loop.runContinuation(nil)
	if err != nil {
		t.Fatalf("runContinuation(): %v", err)
	}
	if result == nil || result.Content == nil {
		t.Fatal("result = nil, want assistant continuation")
	}
	raw, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal continuation content: %v", err)
	}
	if !strings.Contains(string(raw), "continued after tool result") {
		t.Fatalf("continuation content = %s, want continued after tool result", string(raw))
	}
	if len(provider.seen) != 1 {
		t.Fatalf("provider call count = %d, want 1", len(provider.seen))
	}
	if len(provider.seen[0]) != 3 || provider.seen[0][2].Role != op.RoleTool {
		t.Fatalf("provider replay = %+v, want user/assistant/tool tail", provider.seen[0])
	}
}

func TestRunLoop_ContinuesAfterToolResult(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)
	connectTestToolServer(t, "sys-server", func(input testToolInput) string {
		return "read: " + input.Path
	})

	provider := &scriptedProvider{
		responses: []*ai.ProviderResponse{
			testProviderResponse("", "", []op.MessageToolCall{{
				ID:        "call-1",
				Name:      "read_file",
				Arguments: map[string]any{"path": "/etc/passwd"},
				Type:      "function",
			}}, ai.StopReasonToolUse),
			testProviderResponse("done", "", nil, ai.StopReasonStop),
		},
	}

	loop := &AgentLoop{
		Ctx: context.Background(),
		Agent: &Agent{ToolSpecs: map[string]*op.ToolSpec{
			"read_file": {ServerID: "sys-server", Name: "read_file"},
		}},
		Model:            &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{op.NewUserMessage("cat /etc/passwd")}),
		Meta:             op.Meta{},
	}

	messages, err := loop.runLoop(nil)
	if err != nil {
		t.Fatalf("runLoop(): %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}
	if messages[0].Role != op.RoleAssistant || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("unexpected assistant tool call message: %+v", messages[0])
	}
	if messages[1].Role != op.RoleTool || messages[1].ToolCallID != "call-1" || messages[1].Content != "read: /etc/passwd" {
		t.Fatalf("unexpected tool result message: %+v", messages[1])
	}
	if messages[2].Role != op.RoleAssistant || messages[2].Content != "done" {
		t.Fatalf("unexpected final assistant message: %+v", messages[2])
	}
	if len(provider.seen) != 2 {
		t.Fatalf("provider call count = %d, want 2", len(provider.seen))
	}
	secondCall := provider.seen[1]
	if len(secondCall) != 3 {
		t.Fatalf("second provider call len = %d, want 3", len(secondCall))
	}
	if secondCall[0].Role != op.RoleUser || secondCall[1].Role != op.RoleAssistant || secondCall[2].Role != op.RoleTool {
		t.Fatalf("unexpected second provider roles: %+v", secondCall)
	}
	if secondCall[2].ToolCallID != "call-1" || secondCall[2].Content != "read: /etc/passwd" {
		t.Fatalf("unexpected second provider tool result: %+v", secondCall[2])
	}
}

func TestRunLoop_EmitsActivityNotificationsForStreamedToolUse(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)
	connectTestToolServer(t, "sys-server", func(input testToolInput) string {
		return "read: " + input.Path
	})

	toolCall := op.MessageToolCall{
		ID:        "call-1",
		Name:      "read_file",
		Arguments: map[string]any{"path": "/etc/passwd"},
		Type:      "function",
	}
	toolEvent := testToolCallEvent(toolCall)
	toolStart := toolEvent
	toolStart.Type = ai.EventCanonicalToolCallStart
	toolDelta := toolEvent
	toolDelta.Type = ai.EventCanonicalToolCallDelta
	toolDelta.Delta = ai.MarshalToolArgumentsJSON(toolCall.Arguments)
	provider := &eventedProvider{
		streams: [][]ai.ProviderEvent{
			{
				{Type: ai.EventCanonicalThinkingStart},
				{Type: ai.EventCanonicalThinkingDelta, Delta: "thinking step"},
				toolStart,
				toolDelta,
				testToolCallEvent(toolCall),
				{Type: ai.EventCanonicalThinkingEnd, Content: "thinking step"},
				{Type: ai.EventCanonicalDone, Response: testProviderResponse("", "thinking step", []op.MessageToolCall{toolCall}, ai.StopReasonToolUse)},
			},
			{{Type: ai.EventCanonicalDone, Response: testProviderResponse("done", "", nil, ai.StopReasonStop)}},
		},
	}

	loop := &AgentLoop{
		Ctx: context.Background(),
		Agent: &Agent{ToolSpecs: map[string]*op.ToolSpec{
			"read_file": {ServerID: "sys-server", Name: "read_file"},
		}},
		Model:            &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{op.NewUserMessage("cat /etc/passwd")}),
		Meta:             op.Meta{"threadID": "thread-1"},
	}

	messages, err := loop.runLoop(nil)
	if err != nil {
		t.Fatalf("runLoop(): %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}

	notifications := waitNotifyMessagesExcludingTypes(t, 8, "tokenUsage", "start", "done")
	wantTypes := []string{
		"thinking_start",
		"thinking_delta",
		"toolcall_start",
		"toolcall_delta",
		"toolcall_end",
		"thinking_end",
		"assistant_step",
		"tool_result_step",
	}
	for i, want := range wantTypes {
		if got, _ := notifications[i].Meta["type"].(string); got != want {
			t.Fatalf("notification[%d] type = %q, want %q", i, got, want)
		}
	}
	if got := notifyText(t, notifications[1].Content); got != "thinking step" {
		t.Fatalf("thinking delta content = %q, want thinking step", got)
	}
	if got, _ := notifications[2].Meta["name"].(string); got != "read_file" {
		t.Fatalf("toolcall_start name = %q, want read_file", got)
	}
	type toolCallProgress struct {
		Delta    string `json:"delta,omitempty"`
		ToolCall struct {
			ID           string         `json:"id,omitempty"`
			Name         string         `json:"name,omitempty"`
			RawArguments string         `json:"rawArguments,omitempty"`
			Arguments    map[string]any `json:"arguments,omitempty"`
			Complete     bool           `json:"complete,omitempty"`
		} `json:"toolCall"`
	}
	startProgress := notifyJSONPayload[toolCallProgress](t, notifications[2].Content)
	if startProgress.ToolCall.ID != "call-1" || startProgress.ToolCall.Name != "read_file" {
		t.Fatalf("toolcall_start payload = %+v", startProgress)
	}
	deltaProgress := notifyJSONPayload[toolCallProgress](t, notifications[3].Content)
	if deltaProgress.Delta != `{"path":"/etc/passwd"}` || deltaProgress.ToolCall.RawArguments != `{"path":"/etc/passwd"}` {
		t.Fatalf("toolcall_delta payload = %+v", deltaProgress)
	}
	endProgress := notifyJSONPayload[toolCallProgress](t, notifications[4].Content)
	if !endProgress.ToolCall.Complete || endProgress.ToolCall.RawArguments != `{"path":"/etc/passwd"}` {
		t.Fatalf("toolcall_end payload = %+v", endProgress)
	}
	if got := notifyJSONMessage(t, notifications[7].Content); got.Content != "read: /etc/passwd" || got.ToolCallID != "call-1" {
		t.Fatalf("tool result notification message = %+v", got)
	}
}

func TestStreamAssistantResponse_UsesCompleteForUserImageInput(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)
	provider := &completeOnlyProvider{response: testProviderResponse("image ok", "", nil, ai.StopReasonStop)}

	loop := &AgentLoop{
		Ctx:   context.Background(),
		Agent: &Agent{},
		Model: &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{{
			Role: op.RoleUser,
			ContentParts: []op.ContentPart{{
				Type:     "image_url",
				ImageURL: &op.ImageURL{URL: "data:image/png;base64,AAA", Detail: "auto"},
			}},
		}}),
		Meta: op.Meta{},
	}

	msg, err := loop.streamAssistantResponse()
	if err != nil {
		t.Fatalf("streamAssistantResponse(): %v", err)
	}
	if msg.Role != op.RoleAssistant || msg.Content != "image ok" {
		t.Fatalf("unexpected assistant message: %+v", msg)
	}
	if len(provider.seen) != 1 {
		t.Fatalf("Complete call count = %d, want 1", len(provider.seen))
	}

	notifications := waitNotifyMessagesExcludingTypes(t, 3, "tokenUsage", "start", "done")
	wantTypes := []string{"text_start", "text_delta", "text_end"}
	for i, want := range wantTypes {
		if got, _ := notifications[i].Meta["type"].(string); got != want {
			t.Fatalf("notification[%d] type = %q, want %q", i, got, want)
		}
	}
	if got := notifyText(t, notifications[1].Content); got != "image ok" {
		t.Fatalf("text delta content = %q, want image ok", got)
	}
}

func TestStreamAssistantResponse_CompleteEmitsReasoningAndToolCallNotifications(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &completeOnlyProvider{
		response: testProviderResponse("", "thinking first", []op.MessageToolCall{{
			ID:        "call-1",
			Name:      "shell",
			Arguments: map[string]any{"path": "/etc/hosts"},
			Type:      "function",
		}}, ai.StopReasonToolUse),
	}

	loop := &AgentLoop{
		Ctx:   context.Background(),
		Agent: &Agent{},
		Model: &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{{
			Role: op.RoleUser,
			ContentParts: []op.ContentPart{{
				Type:     "image_url",
				ImageURL: &op.ImageURL{URL: "data:image/png;base64,AAA", Detail: "auto"},
			}},
		}}),
		Meta: op.Meta{"threadID": "thread-1"},
	}

	msg, err := loop.streamAssistantResponse()
	if err != nil {
		t.Fatalf("streamAssistantResponse(): %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("assistant tool calls = %d, want 1", len(msg.ToolCalls))
	}

	notifications := waitNotifyMessagesExcludingTypes(t, 6, "tokenUsage", "start", "done")
	wantTypes := []string{
		"thinking_start",
		"thinking_delta",
		"thinking_end",
		"toolcall_start",
		"toolcall_delta",
		"toolcall_end",
	}
	for i, want := range wantTypes {
		if got, _ := notifications[i].Meta["type"].(string); got != want {
			t.Fatalf("notification[%d] type = %q, want %q", i, got, want)
		}
	}
	if got := notifyText(t, notifications[1].Content); got != "thinking first" {
		t.Fatalf("thinking delta content = %q, want thinking first", got)
	}
	if got, _ := notifications[3].Meta["name"].(string); got != "shell" {
		t.Fatalf("toolcall_start name = %q, want shell", got)
	}
}

func TestStreamAssistantResponse_PlanTurnSuppressesVisibleAssistantNotifications(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &scriptedProvider{responses: []*ai.ProviderResponse{{
		Message: ai.ProviderResponseFromOpMessage(op.Message{Role: op.RoleAssistant, Content: "plan answer"}, ai.Usage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14}, ai.StopReasonStop).Message,
		Usage:   ai.Usage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14},
	}}}

	loop := &AgentLoop{
		Ctx:      context.Background(),
		Agent:    &Agent{},
		PlanTurn: true,
		Model:    &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{{
			Role:    op.RoleUser,
			Content: "question",
		}}),
		Meta: op.Meta{"threadID": "thread-1", "planTurn": true},
	}

	msg, err := loop.streamAssistantResponse()
	if err != nil {
		t.Fatalf("streamAssistantResponse(): %v", err)
	}
	if msg.Content != "plan answer" {
		t.Fatalf("assistant content = %q, want plan answer", msg.Content)
	}

	notifications := drainNotifyMessagesAfter(t, 100*time.Millisecond)
	if len(notifications) == 0 {
		t.Fatal("expected canonical notifications for plan turn")
	}
	if got, _ := notifications[0].Meta["type"].(string); got != "start" {
		t.Fatalf("first notification type = %q, want start", got)
	}
}

func TestStreamAssistantResponse_TokenUsageReportsCurrentLoopOnly(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &scriptedProvider{responses: []*ai.ProviderResponse{testProviderResponse("done", "", nil, ai.StopReasonStop)}}
	provider.responses[0].Usage = ai.Usage{InputTokens: 12, OutputTokens: 8, TotalTokens: 20}
	provider.responses[0].Message.Usage = ai.MessageUsageFromUsage(provider.responses[0].Usage)

	loop := &AgentLoop{
		Ctx:   context.Background(),
		Agent: &Agent{},
		Model: &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{
			{
				Role:    op.RoleAssistant,
				Content: "previous answer",
				Usage: &op.MessageUsage{
					InputTokens:  30,
					OutputTokens: 10,
					TotalTokens:  40,
				},
			},
			{
				Role:    op.RoleUser,
				Content: "next question",
			},
		}),
		Meta: op.Meta{"threadID": "thread-1"},
	}

	if _, err := loop.streamAssistantResponse(); err != nil {
		t.Fatalf("streamAssistantResponse(): %v", err)
	}

	notifications := waitNotifyMessagesOfType(t, 1, "tokenUsage")
	if got := notifyMetaInt64(t, notifications[0].Meta, "loopInputTokens"); got != 12 {
		t.Fatalf("loop input tokens = %d, want 12", got)
	}
	if got := notifyMetaInt64(t, notifications[0].Meta, "loopOutputTokens"); got != 8 {
		t.Fatalf("loop output tokens = %d, want 8", got)
	}
	if got := notifyMetaInt64(t, notifications[0].Meta, "loopTotalTokens"); got != 20 {
		t.Fatalf("loop total tokens = %d, want 20", got)
	}
	if got := notifyMetaInt64(t, notifications[0].Meta, "contextTokens"); got != 20 {
		t.Fatalf("context tokens = %d, want 20", got)
	}
	if got := notifyMetaString(t, notifications[0].Meta, "contextKnown"); got != "true" {
		t.Fatalf("context known = %q, want true", got)
	}
}

func TestStreamAssistantResponse_TokenUsageStaysScopedToEachLoop(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &scriptedProvider{responses: []*ai.ProviderResponse{
		testProviderResponse("", "", []op.MessageToolCall{{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: map[string]any{"path": "foo.md"},
			Type:      "function",
		}}, ai.StopReasonToolUse),
		testProviderResponse("final answer", "", nil, ai.StopReasonStop),
	}}
	provider.responses[0].Usage = ai.Usage{InputTokens: 15, OutputTokens: 5, TotalTokens: 20}
	provider.responses[0].Message.Usage = ai.MessageUsageFromUsage(provider.responses[0].Usage)
	provider.responses[1].Usage = ai.Usage{InputTokens: 11, OutputTokens: 9, TotalTokens: 20}
	provider.responses[1].Message.Usage = ai.MessageUsageFromUsage(provider.responses[1].Usage)

	loop := &AgentLoop{
		Ctx:   context.Background(),
		Agent: &Agent{},
		Model: &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{{
			Role:    op.RoleUser,
			Content: "question",
		}}),
		Meta: op.Meta{"threadID": "thread-1"},
	}

	first, err := loop.streamAssistantResponse()
	if err != nil {
		t.Fatalf("first streamAssistantResponse(): %v", err)
	}
	firstNotifications := waitNotifyMessagesExcludingTypes(t, 3, "tokenUsage", "start", "done")
	if got, _ := firstNotifications[0].Meta["type"].(string); got != "toolcall_start" {
		t.Fatalf("first notification type = %q, want toolcall_start", got)
	}
	firstUsageNotifications := waitNotifyMessagesOfType(t, 1, "tokenUsage")
	if got := notifyMetaInt64(t, firstUsageNotifications[0].Meta, "loopTotalTokens"); got != 20 {
		t.Fatalf("first loop total tokens = %d, want 20", got)
	}

	loop.appendCanonicalStateMessages(ai.CanonicalMessagesFromOp([]op.Message{first})...)
	loop.appendStateMessages(op.NewToolResultMessage("read_file", "call-1", "file body"))

	second, err := loop.streamAssistantResponse()
	if err != nil {
		t.Fatalf("second streamAssistantResponse(): %v", err)
	}
	if second.Usage == nil || second.Usage.TotalTokens != 20 {
		t.Fatalf("second assistant usage = %+v, want total 20", second.Usage)
	}

	secondNotifications := waitNotifyMessagesExcludingTypes(t, 3, "tokenUsage", "start", "done")
	if got, _ := secondNotifications[0].Meta["type"].(string); got != "text_start" {
		t.Fatalf("second assistant notification type = %q, want text_start", got)
	}
	secondUsageNotifications := waitNotifyMessagesOfType(t, 1, "tokenUsage")
	if got := notifyMetaInt64(t, secondUsageNotifications[0].Meta, "loopInputTokens"); got != 11 {
		t.Fatalf("second loop input tokens = %d, want 11", got)
	}
	if got := notifyMetaInt64(t, secondUsageNotifications[0].Meta, "loopOutputTokens"); got != 9 {
		t.Fatalf("second loop output tokens = %d, want 9", got)
	}
	if got := notifyMetaInt64(t, secondUsageNotifications[0].Meta, "loopTotalTokens"); got != 20 {
		t.Fatalf("second loop total tokens = %d, want 20", got)
	}
	if got := notifyMetaInt64(t, secondUsageNotifications[0].Meta, "contextTokens"); got != 20 {
		t.Fatalf("second context tokens = %d, want 20", got)
	}
}

func TestMergeObservedAssistantContent_SkipsIncompleteToolCalls(t *testing.T) {
	final := ai.FinalizeStreamConversationMessage(&ai.StreamConversationMessage{
		Role: ai.RoleCanonicalAssistant,
		Content: []ai.StreamContentBlock{
			{
				Type: ai.BlockText,
				Text: "final",
			},
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call-1",
					Name:         "write",
					RawArguments: `{"path":"/tmp/out.md"`,
					Complete:     false,
				},
			},
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call-2",
					Name:         "read",
					RawArguments: `{"path":"/tmp/in.md"}`,
					Complete:     true,
				},
			},
		},
	})

	if len(final.Content) != 2 {
		t.Fatalf("content blocks = %d, want 2", len(final.Content))
	}
	if final.Content[1].Type != ai.BlockToolCall || final.Content[1].ToolCall == nil || final.Content[1].ToolCall.ID != "call-2" {
		t.Fatalf("unexpected finalized tool call: %#v", final.Content[1])
	}
}

func TestCanonicalToolCallsSummary_UsesFinalCanonicalToolCallsOnly(t *testing.T) {
	summary := canonicalToolCallsSummary(ai.FinalizeStreamConversationMessage(&ai.StreamConversationMessage{
		Role: ai.RoleCanonicalAssistant,
		Content: []ai.StreamContentBlock{
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call-1",
					Name:         "write",
					RawArguments: `{"path":"/tmp/out.md"`,
					Complete:     false,
				},
			},
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call-2",
					Name:         "read",
					RawArguments: `{"path":"/tmp/in.md"}`,
					Complete:     true,
				},
			},
		},
	}))

	if len(summary) != 1 {
		t.Fatalf("summary len = %d, want 1", len(summary))
	}
	if summary[0] != `read({"path":"/tmp/in.md"})` {
		t.Fatalf("summary[0] = %q", summary[0])
	}
}

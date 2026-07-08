package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type GatewayResponsesWSProvider struct {
	httpFallback ai.ResponsesProvider
	cfg          *op.ModelConfig
	wsURL        string
	headers      http.Header
	dialer       *websocket.Dialer

	mu         sync.Mutex
	sessions   map[string]*gatewayResponsesWSSessionEntry
	sessionTTL time.Duration
}

const gatewayResponsesWSSessionCacheTTL = 5 * time.Minute

var gatewayWebsocketReadTimeout = 5 * time.Minute

type gatewayResponsesWSSessionEntry struct {
	conn      *websocket.Conn
	busy      bool
	idleTimer *time.Timer
}

type gatewayResponsesWSLease struct {
	conn         *websocket.Conn
	reusedCached bool
	release      func(keep bool)
}

func (l *gatewayResponsesWSLease) Release(keep bool) {
	if l == nil || l.release == nil {
		return
	}
	l.release(keep)
}

func NewGatewayResponsesWSProviderWithOptions(cfg *op.ModelConfig, httpClient *http.Client, headers map[string]string) (*GatewayResponsesWSProvider, error) {
	httpProvider, err := NewResponsesProviderWithTransport(cfg, httpClient, headers)
	if err != nil {
		return nil, err
	}
	wsURL, err := gatewayResponsesWebsocketURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &GatewayResponsesWSProvider{
		httpFallback: httpProvider,
		cfg:          cfg,
		wsURL:        wsURL,
		headers:      gatewayResponsesWSHeaders(cfg, headers),
		dialer:       newGatewayResponsesWSDialer(httpClient),
		sessions:     make(map[string]*gatewayResponsesWSSessionEntry),
		sessionTTL:   gatewayResponsesWSSessionCacheTTL,
	}, nil
}

func (p *GatewayResponsesWSProvider) Capabilities() ai.ProviderCapabilities {
	caps := ai.DefaultCapabilitiesForAPI("openai-responses")
	caps.SupportsWebsocketStream = true
	return caps
}

func (p *GatewayResponsesWSProvider) CompleteCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	prepared := prepareCanonicalReplayForProvider(req, p.cfg, "openai-responses")
	result, err := p.CompleteResponses(ctx, canonicalRequestToResponses(prepared))
	if err != nil {
		return nil, err
	}
	return ai.ProviderResponseFromResponsesResult(result), nil
}

func (p *GatewayResponsesWSProvider) StreamCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	return streamCanonicalFromResponsesProvider(ctx, req, p.cfg, p)
}

func (p *GatewayResponsesWSProvider) CompleteResponses(ctx context.Context, req *ai.ResponsesRequest) (*ai.ResponsesResult, error) {
	stream, err := p.StreamResponses(ctx, req)
	if err != nil {
		return nil, err
	}
	var final *ai.ResponsesResult
	for stream.Next() {
		event := stream.Event()
		if event.Response != nil && strings.TrimSpace(event.Type) == "response.completed" {
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if final == nil {
		return nil, fmt.Errorf("empty model response")
	}
	return final, nil
}

func (p *GatewayResponsesWSProvider) StreamResponses(ctx context.Context, req *ai.ResponsesRequest) (*ai.ResponsesEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("responses request is nil")
	}
	normalizedReq := normalizeResponsesRequestForProvider(req)
	lease, firstEvent, err := p.openWebsocketStream(ctx, normalizedReq)
	if err != nil {
		if !shouldFallbackGatewayResponsesToHTTP(err) {
			return nil, err
		}
		slog.Warn("gateway responses websocket fallback to http",
			"model", strings.TrimSpace(normalizedReq.Model),
			"requestID", strings.TrimSpace(normalizedReq.RequestID),
			"promptCacheKey", strings.TrimSpace(normalizedReq.PromptCacheKey),
			"error", err,
		)
		if p.httpFallback == nil {
			return nil, err
		}
		return p.httpFallback.StreamResponses(ctx, normalizedReq)
	}
	out := ai.NewResponsesEventStream(128)
	go p.forwardWebsocketStream(ctx, out, lease, firstEvent)
	return out, nil
}

func (p *GatewayResponsesWSProvider) forwardWebsocketStream(ctx context.Context, out *ai.ResponsesEventStream, lease *gatewayResponsesWSLease, firstEvent ai.ResponsesStreamEvent) {
	if lease == nil || lease.conn == nil {
		out.Finish(fmt.Errorf("gateway responses websocket lease is nil"))
		return
	}
	keepConnection := true
	defer func() {
		lease.Release(keepConnection)
	}()
	stopCancel := closeWebsocketOnContextDone(ctx, lease.conn)
	defer stopCancel()

	if !out.Emit(firstEvent) {
		keepConnection = false
		return
	}
	if strings.TrimSpace(firstEvent.Type) == "response.completed" {
		out.Close()
		return
	}
	for {
		event, err := readGatewayResponsesWSEvent(lease.conn)
		if err != nil {
			keepConnection = false
			out.Finish(err)
			return
		}
		if isBlankGatewayResponsesFailure(event) {
			continue
		}
		if strings.TrimSpace(event.Type) == "response.failed" {
			keepConnection = false
			if event.Error != nil {
				out.Finish(event.Error)
				return
			}
			out.Close()
			return
		}
		if !out.Emit(event) {
			keepConnection = false
			return
		}
		if strings.TrimSpace(event.Type) == "response.completed" {
			out.Close()
			return
		}
	}
}

func (p *GatewayResponsesWSProvider) openWebsocketStream(ctx context.Context, req *ai.ResponsesRequest) (*gatewayResponsesWSLease, ai.ResponsesStreamEvent, error) {
	return p.openWebsocketStreamWithRetry(ctx, req, true)
}

func (p *GatewayResponsesWSProvider) openWebsocketStreamWithRetry(ctx context.Context, req *ai.ResponsesRequest, allowCachedRetry bool) (*gatewayResponsesWSLease, ai.ResponsesStreamEvent, error) {
	lease, err := p.acquireWebsocket(ctx, req, true)
	if err != nil {
		return nil, ai.ResponsesStreamEvent{}, markGatewayResponsesWSFallback(err)
	}
	stopCancel := closeWebsocketOnContextDone(ctx, lease.conn)
	defer stopCancel()
	payload, err := marshalResponsesWebsocketCreate(req, nil)
	if err != nil {
		lease.Release(false)
		return nil, ai.ResponsesStreamEvent{}, err
	}
	if err := lease.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		lease.Release(false)
		if allowCachedRetry && lease.reusedCached {
			slog.Warn("gateway responses websocket cached session failed; redialing fresh",
				append(gatewayResponsesWSLogFields(req), "stage", "write", "error", err)...,
			)
			return p.openWebsocketStreamWithRetry(ctx, req, false)
		}
		return nil, ai.ResponsesStreamEvent{}, markGatewayResponsesWSFallback(fmt.Errorf("write websocket request: %w", err))
	}
	for {
		event, err := readGatewayResponsesWSEvent(lease.conn)
		if err != nil {
			lease.Release(false)
			if allowCachedRetry && lease.reusedCached {
				slog.Warn("gateway responses websocket cached session failed; redialing fresh",
					append(gatewayResponsesWSLogFields(req), "stage", "read", "error", err)...,
				)
				return p.openWebsocketStreamWithRetry(ctx, req, false)
			}
			return nil, ai.ResponsesStreamEvent{}, markGatewayResponsesWSFallback(err)
		}
		if isBlankGatewayResponsesFailure(event) {
			continue
		}
		if strings.TrimSpace(event.Type) == "response.failed" {
			lease.Release(false)
			if event.Error != nil {
				return nil, ai.ResponsesStreamEvent{}, event.Error
			}
			return nil, ai.ResponsesStreamEvent{}, fmt.Errorf("response failed")
		}
		return lease, event, nil
	}
}

func (p *GatewayResponsesWSProvider) requestHeaders(requestID string) http.Header {
	headers := cloneGatewayResponsesWSHeaders(p.headers)
	if strings.TrimSpace(requestID) != "" {
		headers.Set("X-Request-ID", strings.TrimSpace(requestID))
	}
	return headers
}

func (p *GatewayResponsesWSProvider) acquireWebsocket(ctx context.Context, req *ai.ResponsesRequest, allowCachedReuse bool) (*gatewayResponsesWSLease, error) {
	sessionKey := strings.TrimSpace(req.PromptCacheKey)
	if sessionKey == "" {
		return p.newEphemeralWebsocketLease(ctx, req)
	}
	if !gatewayResponsesWSCanReuseCachedSession(req) {
		return p.newEphemeralWebsocketLease(ctx, req)
	}

	p.mu.Lock()
	if p.sessions == nil {
		p.sessions = make(map[string]*gatewayResponsesWSSessionEntry)
	}
	entry := p.sessions[sessionKey]
	if entry != nil && entry.idleTimer != nil {
		entry.idleTimer.Stop()
		entry.idleTimer = nil
	}
	if allowCachedReuse && entry != nil && !entry.busy {
		entry.busy = true
		p.mu.Unlock()
		slog.Info("gateway responses websocket session cache hit",
			append(gatewayResponsesWSLogFields(req), "reusedCached", true)...,
		)
		return &gatewayResponsesWSLease{
			conn:         entry.conn,
			reusedCached: true,
			release: func(keep bool) {
				p.releaseSessionLease(sessionKey, entry, keep)
			},
		}, nil
	}
	busy := entry != nil && entry.busy
	p.mu.Unlock()

	if entry != nil && !busy && !allowCachedReuse {
		p.releaseSessionLease(sessionKey, entry, false)
	}
	if busy {
		return p.newEphemeralWebsocketLease(ctx, req)
	}

	conn, err := p.dialWebsocket(ctx, req)
	if err != nil {
		return nil, err
	}
	entry = &gatewayResponsesWSSessionEntry{conn: conn, busy: true}
	p.mu.Lock()
	if p.sessions == nil {
		p.sessions = make(map[string]*gatewayResponsesWSSessionEntry)
	}
	p.sessions[sessionKey] = entry
	p.mu.Unlock()
	return &gatewayResponsesWSLease{
		conn: conn,
		release: func(keep bool) {
			p.releaseSessionLease(sessionKey, entry, keep)
		},
	}, nil
}

func gatewayResponsesWSCanReuseCachedSession(req *ai.ResponsesRequest) bool {
	if req == nil {
		return false
	}
	// Reusing the same websocket session is only safe for simple prompt-cache-key
	// continuation where the next request is effectively another user prompt.
	// Once the request includes assistant/tool replay items, the server-side
	// session state can diverge from the local canonical history after tool turns.
	// In that case, force a fresh websocket so the create payload starts from a
	// clean stream context even if prompt_cache_key stays the same.
	for _, item := range req.Input {
		switch strings.TrimSpace(item.Type) {
		case "function_call", "function_call_output", "reasoning", "compaction":
			return false
		}
		role := strings.ToLower(strings.TrimSpace(item.Role))
		if role == "assistant" {
			return false
		}
	}
	return true
}

func (p *GatewayResponsesWSProvider) newEphemeralWebsocketLease(ctx context.Context, req *ai.ResponsesRequest) (*gatewayResponsesWSLease, error) {
	conn, err := p.dialWebsocket(ctx, req)
	if err != nil {
		return nil, err
	}
	return &gatewayResponsesWSLease{
		conn: conn,
		release: func(bool) {
			_ = conn.Close()
		},
	}, nil
}

func (p *GatewayResponsesWSProvider) dialWebsocket(ctx context.Context, req *ai.ResponsesRequest) (*websocket.Conn, error) {
	conn, resp, err := p.dialer.DialContext(ctx, p.wsURL, p.requestHeaders(req.RequestID))
	if err != nil {
		return nil, gatewayResponsesWSError("dial websocket", err, resp)
	}
	return conn, nil
}

func (p *GatewayResponsesWSProvider) releaseSessionLease(sessionKey string, entry *gatewayResponsesWSSessionEntry, keep bool) {
	if entry == nil || entry.conn == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		_ = entry.conn.Close()
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions == nil || p.sessions[sessionKey] != entry {
		if entry.idleTimer != nil {
			entry.idleTimer.Stop()
		}
		_ = entry.conn.Close()
		return
	}
	if !keep {
		if entry.idleTimer != nil {
			entry.idleTimer.Stop()
			entry.idleTimer = nil
		}
		delete(p.sessions, sessionKey)
		_ = entry.conn.Close()
		return
	}
	entry.busy = false
	if entry.idleTimer != nil {
		entry.idleTimer.Stop()
	}
	ttl := p.sessionTTL
	if ttl <= 0 {
		ttl = gatewayResponsesWSSessionCacheTTL
	}
	entry.idleTimer = time.AfterFunc(ttl, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		current := p.sessions[sessionKey]
		if current != entry || entry.busy {
			return
		}
		delete(p.sessions, sessionKey)
		_ = entry.conn.Close()
	})
}

func gatewayResponsesWSLogFields(req *ai.ResponsesRequest) []any {
	if req == nil {
		return nil
	}
	return []any{
		"model", strings.TrimSpace(req.Model),
		"requestID", strings.TrimSpace(req.RequestID),
		"promptCacheKey", strings.TrimSpace(req.PromptCacheKey),
	}
}

func newGatewayResponsesWSDialer(httpClient *http.Client) *websocket.Dialer {
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}
	if httpClient == nil {
		return dialer
	}
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok || transport == nil {
		return dialer
	}
	if transport.Proxy != nil {
		dialer.Proxy = transport.Proxy
	}
	if transport.DialContext != nil {
		dialer.NetDialContext = transport.DialContext
	}
	if transport.TLSClientConfig != nil {
		dialer.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	if dialer.TLSClientConfig == nil {
		dialer.TLSClientConfig = &tls.Config{}
	}
	// WebSocket handshakes must negotiate HTTP/1.1. Reusing an http.Transport
	// TLS config can carry over h2 preferences, which breaks gorilla/websocket
	// with malformed HTTP response errors on wss endpoints.
	dialer.TLSClientConfig.NextProtos = []string{"http/1.1"}
	return dialer
}

func gatewayResponsesWSHeaders(cfg *op.ModelConfig, headers map[string]string) http.Header {
	out := make(http.Header, len(headers)+2)
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out.Set(key, value)
	}
	if cfg != nil && strings.TrimSpace(cfg.APIKey) != "" {
		out.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	}
	out.Set("Content-Type", "application/json")
	return out
}

func cloneGatewayResponsesWSHeaders(headers http.Header) http.Header {
	out := make(http.Header, len(headers))
	for key, values := range headers {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func gatewayResponsesWebsocketURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse responses websocket base url: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
	default:
		return "", fmt.Errorf("unsupported websocket scheme: %s", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/responses"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func marshalResponsesWebsocketCreate(req *ai.ResponsesRequest, generate *bool) ([]byte, error) {
	return ai.MarshalResponsesWebsocketCreateJSON(req, generate)
}

func readGatewayResponsesWSEvent(conn *websocket.Conn) (ai.ResponsesStreamEvent, error) {
	for {
		setGatewayWebsocketReadDeadline(conn)
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return ai.ResponsesStreamEvent{}, ai.NormalizeRetryError(fmt.Errorf("read websocket event: %w", err))
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		return parseGatewayResponsesWSEvent(payload)
	}
}

func parseGatewayResponsesWSEvent(raw []byte) (ai.ResponsesStreamEvent, error) {
	return ai.ParseResponsesStreamEventJSON(raw)
}

func setGatewayWebsocketReadDeadline(conn *websocket.Conn) {
	if conn == nil || gatewayWebsocketReadTimeout <= 0 {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(gatewayWebsocketReadTimeout))
}

func isBlankGatewayResponsesFailure(event ai.ResponsesStreamEvent) bool {
	if strings.TrimSpace(event.Type) != "response.failed" {
		return false
	}
	return event.Error == nil || strings.TrimSpace(event.Error.Error()) == ""
}

func closeWebsocketOnContextDone(ctx context.Context, conn *websocket.Conn) func() {
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	return func() {
		close(stop)
	}
}

type gatewayResponsesWSFallbackError struct {
	err error
}

func (e *gatewayResponsesWSFallbackError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *gatewayResponsesWSFallbackError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func markGatewayResponsesWSFallback(err error) error {
	if err == nil {
		return nil
	}
	var tagged *gatewayResponsesWSFallbackError
	if errors.As(err, &tagged) {
		return err
	}
	return &gatewayResponsesWSFallbackError{err: err}
}

func shouldFallbackGatewayResponsesToHTTP(err error) bool {
	if err == nil {
		return false
	}
	var tagged *gatewayResponsesWSFallbackError
	return errors.As(err, &tagged)
}

func gatewayResponsesWSError(action string, err error, resp *http.Response) error {
	if err == nil {
		return nil
	}
	action = strings.TrimSpace(action)
	wrapped := ai.NormalizeRetryError(fmt.Errorf("%s: %w", firstNonEmptyString(action, "responses websocket"), err))
	if resp == nil {
		return wrapped
	}
	retryAfterMs := ai.ParseRetryAfterHeaders(resp.Header)
	return ai.WrapRetryError(
		fmt.Errorf("%s: %w (status=%d)", firstNonEmptyString(action, "responses websocket"), err, resp.StatusCode),
		resp.StatusCode,
		"",
		wrapped.Error(),
		retryAfterMs,
	)
}

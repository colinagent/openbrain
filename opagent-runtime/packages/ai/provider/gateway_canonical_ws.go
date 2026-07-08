package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type GatewayCanonicalWSProvider struct {
	cfg     *op.ModelConfig
	wsURL   string
	headers http.Header
	dialer  *websocket.Dialer
}

func NewGatewayCanonicalWSProviderWithOptions(cfg *op.ModelConfig, httpClient *http.Client, headers map[string]string) (*GatewayCanonicalWSProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("model config is nil")
	}
	wsURL, err := gatewayCanonicalWebsocketURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &GatewayCanonicalWSProvider{
		cfg:     cfg,
		wsURL:   wsURL,
		headers: gatewayResponsesWSHeaders(cfg, headers),
		dialer:  newGatewayResponsesWSDialer(httpClient),
	}, nil
}

func (p *GatewayCanonicalWSProvider) Capabilities() ai.ProviderCapabilities {
	api := "openai-completions"
	if p.cfg != nil && strings.TrimSpace(p.cfg.API) != "" {
		api = strings.TrimSpace(p.cfg.API)
	}
	caps := ai.DefaultCapabilitiesForAPI(api)
	caps.SupportsWebsocketStream = true
	return caps
}

func (p *GatewayCanonicalWSProvider) CompleteCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	stream, err := p.StreamCanonical(ctx, req)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	var final *ai.ProviderResponse
	for stream.Next() {
		event := stream.Event()
		if event.Type == ai.EventCanonicalDone {
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if final == nil {
		return nil, fmt.Errorf("empty model response")
	}
	if !ai.HasSemanticCanonicalResponse(final) {
		return nil, fmt.Errorf("empty semantic model response")
	}
	return final, nil
}

func (p *GatewayCanonicalWSProvider) StreamCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	conn, resp, err := p.dialer.DialContext(ctx, p.wsURL, p.requestHeaders(req.RequestID))
	if err != nil {
		return nil, gatewayResponsesWSError("dial canonical websocket", err, resp)
	}
	payload, err := ai.MarshalCanonicalWebsocketCreateJSON(p.modelID(req), req)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		_ = conn.Close()
		return nil, ai.NormalizeRetryError(fmt.Errorf("write canonical websocket request: %w", err))
	}
	out := ai.NewProviderEventStream(128)
	go p.forwardWebsocketStream(ctx, out, conn)
	return out, nil
}

func (p *GatewayCanonicalWSProvider) forwardWebsocketStream(ctx context.Context, out *ai.ProviderEventStream, conn *websocket.Conn) {
	defer conn.Close()
	stopCancel := closeWebsocketOnContextDone(ctx, conn)
	defer stopCancel()
	for {
		event, err := readGatewayCanonicalWSEvent(conn)
		if err != nil {
			out.Finish(err)
			return
		}
		if event.Error != nil {
			out.Finish(event.Error)
			return
		}
		if !out.Emit(event) {
			return
		}
		if event.Type == ai.EventCanonicalDone {
			out.Close()
			return
		}
	}
}

func (p *GatewayCanonicalWSProvider) requestHeaders(requestID string) http.Header {
	headers := cloneGatewayResponsesWSHeaders(p.headers)
	if strings.TrimSpace(requestID) != "" {
		headers.Set("X-Request-ID", strings.TrimSpace(requestID))
	}
	return headers
}

func (p *GatewayCanonicalWSProvider) modelID(req *ai.ProviderRequest) string {
	if p.cfg != nil && strings.TrimSpace(p.cfg.ID) != "" {
		return strings.TrimSpace(p.cfg.ID)
	}
	if req != nil && strings.TrimSpace(req.Config.Model) != "" {
		return strings.TrimSpace(req.Config.Model)
	}
	if p.cfg != nil {
		return strings.TrimSpace(firstNonEmptyString(p.cfg.Name))
	}
	return ""
}

func gatewayCanonicalWebsocketURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse canonical websocket base url: %w", err)
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
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/internal/canonical"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func readGatewayCanonicalWSEvent(conn *websocket.Conn) (ai.ProviderEvent, error) {
	for {
		setGatewayWebsocketReadDeadline(conn)
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return ai.ProviderEvent{}, ai.NormalizeRetryError(fmt.Errorf("read canonical websocket event: %w", err))
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		return ai.ParseCanonicalStreamEventJSON(payload)
	}
}

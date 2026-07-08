package provider

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type ResponsesProvider struct {
	client     openai.Client
	cfg        *op.ModelConfig
	httpClient *http.Client
	headers    map[string]string
}

func NewResponsesProvider(cfg *op.ModelConfig) (*ResponsesProvider, error) {
	return NewResponsesProviderWithTransport(cfg, nil, nil)
}

func NewResponsesProviderWithOptions(cfg *op.ModelConfig, opts ...option.RequestOption) (*ResponsesProvider, error) {
	return NewResponsesProviderWithTransport(cfg, nil, nil, opts...)
}

func NewResponsesProviderWithTransport(cfg *op.ModelConfig, httpClient *http.Client, headers map[string]string, opts ...option.RequestOption) (*ResponsesProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("model config is nil")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("model config: apiKey is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("model config: baseURL is required")
	}
	normalizedHeaders := cloneStringMap(headers)
	clientOpts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
	}
	if httpClient != nil {
		clientOpts = append(clientOpts, option.WithHTTPClient(httpClient))
	}
	for key, value := range normalizedHeaders {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		clientOpts = append(clientOpts, option.WithHeader(key, value))
	}
	clientOpts = append(clientOpts, opts...)
	client := openai.NewClient(clientOpts...)
	return &ResponsesProvider{
		client:     client,
		cfg:        cfg,
		httpClient: httpClient,
		headers:    normalizedHeaders,
	}, nil
}

func normalizeResponsesRequestForProvider(req *ai.ResponsesRequest) *ai.ResponsesRequest {
	out := cloneResponsesRequestForProvider(req)
	out.Input = normalizeResponsesReplayInput(out.Input)
	return out
}

func cloneResponsesRequestForProvider(req *ai.ResponsesRequest) *ai.ResponsesRequest {
	if req == nil {
		return &ai.ResponsesRequest{}
	}
	out := *req
	out.Include = append([]string(nil), req.Include...)
	if len(req.Input) > 0 {
		out.Input = make([]ai.ResponseItem, len(req.Input))
		copy(out.Input, req.Input)
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]ai.ResponseTool, len(req.Tools))
		copy(out.Tools, req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		out.ToolChoice = append([]byte(nil), req.ToolChoice...)
	}
	if req.Text != nil {
		text := *req.Text
		if len(req.Text.FormatRaw) > 0 {
			text.FormatRaw = append([]byte(nil), req.Text.FormatRaw...)
		}
		out.Text = &text
	}
	if req.Reasoning != nil {
		reasoning := *req.Reasoning
		out.Reasoning = &reasoning
	}
	return &out
}

func (p *ResponsesProvider) normalizeRequestForProvider(req *ai.ResponsesRequest) *ai.ResponsesRequest {
	return normalizeResponsesRequestForProvider(req)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

package runtime

import (
	"context"
	"net/http"
	"time"

	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type APIProvider = ai.CanonicalProvider

type ProviderEndpoint struct {
	ID       string
	Provider string
	BaseURL  string
	APIKey   string
	Headers  map[string]string
}

type RouteCandidate struct {
	ProviderRef   string
	UpstreamModel string
	Priority      int
	Weight        int
}

type LogicalModel struct {
	ID              string
	Name            string
	Provider        string
	API             string
	Reasoning       bool
	ReasoningLevels []string
	ContextWindow   int64
	MaxOutputTokens int64
	Routes          []RouteCandidate
}

type RetryPolicy struct {
	MaxAttempts      int
	InitialBackoff   time.Duration
	MaxBackoff       time.Duration
	JitterRatio      float64
	TotalBudget      time.Duration
	FirstByteTimeout time.Duration
	Cooldown         time.Duration
}

type StickyScope string

const (
	StickyScopeNone StickyScope = ""
	// StickyScopeThread uses the durable thread identity as the routing key.
	// This is the default for thread chat flows in opagent.
	StickyScopeThread StickyScope = "thread"
	// StickyScopeSession uses an optional transport/provider session hint.
	// It is not the persisted thread identity and should only be used when a
	// caller explicitly wants session-scoped stickiness outside normal thread chat.
	StickyScopeSession StickyScope = "session"
)

type StickyPolicy struct {
	Scope StickyScope
	TTL   time.Duration
}

type PoolPolicy struct {
	Isolation           string
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
}

type Config struct {
	Providers    []ProviderEndpoint
	Models       []LogicalModel
	RetryPolicy  RetryPolicy
	StickyPolicy StickyPolicy
	PoolPolicy   PoolPolicy
}

type ResolveOptions struct {
	// ThreadID is the durable thread/chat identity.
	ThreadID string
	// SessionID is an optional transport/provider session hint, not transcript truth.
	SessionID string
}

type ResolvedRouteCandidate struct {
	Candidate RouteCandidate
	Endpoint  ProviderEndpoint
}

type ResolvedModel struct {
	Model      LogicalModel
	Candidates []ResolvedRouteCandidate
}

type Runtime interface {
	ResolveModel(modelID string, opts ResolveOptions) (*ResolvedModel, error)
	CompleteCanonical(ctx context.Context, req *CanonicalRequest) (*ai.ProviderResponse, error)
	StreamCanonical(ctx context.Context, req *CanonicalRequest) (*ai.ProviderEventStream, error)
	CompleteResponses(ctx context.Context, req *ResponsesRequest) (*ai.ResponsesResult, error)
	StreamResponses(ctx context.Context, req *ResponsesRequest) (*ai.ResponsesEventStream, error)
	ListModels() []LogicalModel
}

type ResponsesRequest struct {
	ModelID string
	// ThreadID is the durable thread/chat identity.
	ThreadID string
	// SessionID is an optional transport/provider session hint, not transcript truth.
	SessionID string
	Params    *ai.ResponsesRequest
}

type CanonicalRequest struct {
	ModelID string
	// ThreadID is the durable thread/chat identity.
	ThreadID string
	// SessionID is an optional transport/provider session hint, not transcript truth.
	SessionID string
	Params    *ai.ProviderRequest
}

type ProviderFactory func(ctx context.Context, endpoint ProviderEndpoint, model LogicalModel, candidate RouteCandidate, httpClient *http.Client) (APIProvider, error)

package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai/provider"
)

var errEmptyStream = errors.New("runtime: empty stream")

type stickyEntry struct {
	providerRef string
	expiresAt   time.Time
}

type router struct {
	mu            sync.RWMutex
	providers     map[string]ProviderEndpoint
	models        map[string]LogicalModel
	retry         RetryPolicy
	sticky        StickyPolicy
	pool          PoolPolicy
	stickyMap     map[string]stickyEntry
	cooldowns     map[string]time.Time
	httpClients   map[string]*http.Client
	providerCache map[string]ai.CanonicalProvider
	factory       ProviderFactory
	rng           *rand.Rand
}

var singleModelProviderCache = struct {
	mu        sync.Mutex
	providers map[string]ai.CanonicalProvider
}{
	providers: make(map[string]ai.CanonicalProvider),
}

func New(config Config) (Runtime, error) {
	return NewWithFactory(config, defaultProviderFactory)
}

func NewWithFactory(config Config, factory ProviderFactory) (Runtime, error) {
	if factory == nil {
		return nil, fmt.Errorf("provider factory is required")
	}
	r := &router{
		providers:     make(map[string]ProviderEndpoint),
		models:        make(map[string]LogicalModel),
		retry:         normalizeRetryPolicy(config.RetryPolicy),
		sticky:        normalizeStickyPolicy(config.StickyPolicy),
		pool:          normalizePoolPolicy(config.PoolPolicy),
		stickyMap:     make(map[string]stickyEntry),
		cooldowns:     make(map[string]time.Time),
		httpClients:   make(map[string]*http.Client),
		providerCache: make(map[string]ai.CanonicalProvider),
		factory:       factory,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, endpoint := range config.Providers {
		id := strings.TrimSpace(endpoint.ID)
		if id == "" {
			return nil, fmt.Errorf("provider endpoint id is required")
		}
		endpoint.ID = id
		endpoint.Provider = strings.TrimSpace(endpoint.Provider)
		endpoint.BaseURL = strings.TrimSpace(endpoint.BaseURL)
		endpoint.APIKey = strings.TrimSpace(endpoint.APIKey)
		r.providers[id] = cloneEndpoint(endpoint)
	}
	for _, model := range config.Models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			return nil, fmt.Errorf("logical model id is required")
		}
		model.ID = id
		model.Name = strings.TrimSpace(model.Name)
		model.Provider = strings.TrimSpace(model.Provider)
		model.API = strings.TrimSpace(model.API)
		model.ReasoningLevels = append([]string(nil), model.ReasoningLevels...)
		model.Routes = append([]RouteCandidate(nil), model.Routes...)
		r.models[id] = model
	}
	return r, nil
}

func NewSingleModelProvider(cfg *op.ModelConfig) (ai.CanonicalProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("model config is nil")
	}
	modelID := strings.TrimSpace(cfg.ID)
	if modelID == "" {
		return nil, fmt.Errorf("model config: model id is required")
	}
	cacheKey := singleModelProviderCacheKey(cfg)
	singleModelProviderCache.mu.Lock()
	if cached := singleModelProviderCache.providers[cacheKey]; cached != nil {
		singleModelProviderCache.mu.Unlock()
		return cached, nil
	}
	singleModelProviderCache.mu.Unlock()

	if strings.TrimSpace(cfg.Provider) == "opagent-ai-gateway" {
		prov, err := newSingleModelGatewayProvider(cfg)
		if err != nil {
			return nil, err
		}
		singleModelProviderCache.mu.Lock()
		defer singleModelProviderCache.mu.Unlock()
		if cached := singleModelProviderCache.providers[cacheKey]; cached != nil {
			return cached, nil
		}
		singleModelProviderCache.providers[cacheKey] = prov
		return prov, nil
	}

	endpointID := modelID + ":endpoint"
	rt, err := New(Config{
		Providers: []ProviderEndpoint{{
			ID:       endpointID,
			Provider: strings.TrimSpace(cfg.Provider),
			BaseURL:  strings.TrimSpace(cfg.BaseURL),
			APIKey:   strings.TrimSpace(cfg.APIKey),
			Headers:  cloneStringMap(cfg.Headers),
		}},
		Models: []LogicalModel{{
			ID:              modelID,
			Name:            strings.TrimSpace(cfg.Name),
			Provider:        strings.TrimSpace(cfg.Provider),
			API:             strings.TrimSpace(cfg.API),
			Reasoning:       cfg.Reasoning,
			ReasoningLevels: append([]string(nil), cfg.ReasoningLevels...),
			ContextWindow:   cfg.ContextWindow,
			MaxOutputTokens: cfg.MaxOutputTokens,
			Routes: []RouteCandidate{{
				ProviderRef:   endpointID,
				UpstreamModel: strings.TrimSpace(cfg.Name),
				Priority:      0,
				Weight:        1,
			}},
		}},
		// Single direct providers should not add another runtime-level retry/failover layer
		// on top of provider/client retries.
		RetryPolicy: RetryPolicy{MaxAttempts: 1, Cooldown: -1},
	})
	if err != nil {
		return nil, err
	}
	prov := &singleModelProvider{
		runtime:      rt,
		modelID:      modelID,
		capabilities: ai.DefaultCapabilitiesForAPI(strings.TrimSpace(cfg.API)),
	}
	singleModelProviderCache.mu.Lock()
	defer singleModelProviderCache.mu.Unlock()
	if cached := singleModelProviderCache.providers[cacheKey]; cached != nil {
		return cached, nil
	}
	singleModelProviderCache.providers[cacheKey] = prov
	return prov, nil
}

func newSingleModelGatewayProvider(cfg *op.ModelConfig) (ai.CanonicalProvider, error) {
	api := strings.TrimSpace(cfg.API)
	if api == "" {
		api = "openai-completions"
	}
	switch api {
	case "openai-responses":
		return provider.NewGatewayResponsesWSProviderWithOptions(cfg, nil, cfg.Headers)
	case "openai-completions", "anthropic-messages", "gemini-native":
		return provider.NewGatewayCanonicalWSProviderWithOptions(cfg, nil, cfg.Headers)
	default:
		return nil, fmt.Errorf("unsupported api: %s", api)
	}
}

type singleModelProvider struct {
	runtime      Runtime
	modelID      string
	capabilities ai.ProviderCapabilities
}

func (p *singleModelProvider) Capabilities() ai.ProviderCapabilities {
	return p.capabilities
}

func (p *singleModelProvider) CompleteCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return p.runtime.CompleteCanonical(ctx, &CanonicalRequest{ModelID: p.modelID, Params: cloneCanonicalRequest(req)})
}

func (p *singleModelProvider) StreamCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	return p.runtime.StreamCanonical(ctx, &CanonicalRequest{ModelID: p.modelID, Params: cloneCanonicalRequest(req)})
}

func (p *singleModelProvider) CompleteResponses(ctx context.Context, req *ai.ResponsesRequest) (*ai.ResponsesResult, error) {
	return p.runtime.CompleteResponses(ctx, &ResponsesRequest{ModelID: p.modelID, Params: cloneResponsesRequest(req)})
}

func (p *singleModelProvider) StreamResponses(ctx context.Context, req *ai.ResponsesRequest) (*ai.ResponsesEventStream, error) {
	return p.runtime.StreamResponses(ctx, &ResponsesRequest{ModelID: p.modelID, Params: cloneResponsesRequest(req)})
}

func (r *router) ListModels() []LogicalModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	models := make([]LogicalModel, 0, len(r.models))
	for _, model := range r.models {
		models = append(models, cloneModel(model))
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models
}

func (r *router) ResolveModel(modelID string, opts ResolveOptions) (*ResolvedModel, error) {
	r.mu.RLock()
	model, ok := r.models[strings.TrimSpace(modelID)]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}
	candidates := r.resolveCandidates(model, opts)
	if len(candidates) == 0 {
		slog.Error("model resolution yielded no candidates",
			"modelID", strings.TrimSpace(modelID),
			"routeProviderRefs", routeProviderRefs(model.Routes),
			"knownProviders", r.providerIDs(),
			"routeCooldowns", r.routeCooldowns(model.Routes),
			"retryCooldown", r.retry.Cooldown,
		)
		return nil, fmt.Errorf("model %s has no available providers", modelID)
	}
	return &ResolvedModel{
		Model:      cloneModel(model),
		Candidates: candidates,
	}, nil
}

func (r *router) CompleteCanonical(ctx context.Context, req *CanonicalRequest) (*ai.ProviderResponse, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("canonical request params are required")
	}
	resolved, err := r.ResolveModel(req.ModelID, ResolveOptions{ThreadID: req.ThreadID, SessionID: req.SessionID})
	if err != nil {
		return nil, err
	}
	startedAt := time.Now()
	var lastErr error
	for _, candidate := range resolved.Candidates {
		if !withinBudget(startedAt, r.retry.TotalBudget) {
			break
		}
		resp, err := r.completeCanonicalCandidate(ctx, resolved.Model, candidate, req.Params, req.ThreadID, req.SessionID)
		if err == nil {
			resp = annotateCanonicalResponse(resp, candidate.Endpoint.ID)
			r.recordSuccess(candidate.Endpoint.ID, req.ThreadID, req.SessionID)
			return resp, nil
		}
		lastErr = err
		if !isRetryableError(err) {
			return nil, err
		}
		r.recordFailure(candidate.Endpoint.ID)
		if err := sleepContext(ctx, r.retryDelay(candidate.Candidate, startedAt, err)); err != nil {
			return nil, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("canonical request budget exhausted for model %s", req.ModelID)
	}
	return nil, lastErr
}

func (r *router) StreamCanonical(ctx context.Context, req *CanonicalRequest) (*ai.ProviderEventStream, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("canonical request params are required")
	}
	resolved, err := r.ResolveModel(req.ModelID, ResolveOptions{ThreadID: req.ThreadID, SessionID: req.SessionID})
	if err != nil {
		return nil, err
	}
	out := ai.NewProviderEventStream(128)
	go r.runCanonicalStream(ctx, out, resolved, req)
	return out, nil
}

func (r *router) CompleteResponses(ctx context.Context, req *ResponsesRequest) (*ai.ResponsesResult, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("responses request params are required")
	}
	resolved, err := r.ResolveModel(req.ModelID, ResolveOptions{ThreadID: req.ThreadID, SessionID: req.SessionID})
	if err != nil {
		return nil, err
	}
	startedAt := time.Now()
	var lastErr error
	for _, candidate := range resolved.Candidates {
		if !withinBudget(startedAt, r.retry.TotalBudget) {
			break
		}
		resp, err := r.completeResponsesCandidate(ctx, resolved.Model, candidate, req.Params, req.ThreadID, req.SessionID)
		if err == nil {
			resp = annotateResponsesResult(resp, candidate.Endpoint.ID)
			r.recordSuccess(candidate.Endpoint.ID, req.ThreadID, req.SessionID)
			return resp, nil
		}
		lastErr = err
		if !isRetryableError(err) {
			return nil, err
		}
		r.recordFailure(candidate.Endpoint.ID)
		delay := r.retryDelay(candidate.Candidate, startedAt, lastErr)
		if delay > 0 {
			if err := sleepContext(ctx, delay); err != nil {
				return nil, err
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("responses request budget exhausted for model %s", req.ModelID)
	}
	return nil, lastErr
}

func (r *router) StreamResponses(ctx context.Context, req *ResponsesRequest) (*ai.ResponsesEventStream, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("responses request params are required")
	}
	resolved, err := r.ResolveModel(req.ModelID, ResolveOptions{ThreadID: req.ThreadID, SessionID: req.SessionID})
	if err != nil {
		return nil, err
	}
	out := ai.NewResponsesEventStream(128)
	go r.runResponsesStream(ctx, out, resolved, req)
	return out, nil
}

func (r *router) runResponsesStream(ctx context.Context, out *ai.ResponsesEventStream, resolved *ResolvedModel, req *ResponsesRequest) {
	startedAt := time.Now()
	var lastErr error
	for _, candidate := range resolved.Candidates {
		if !withinBudget(startedAt, r.retry.TotalBudget) {
			break
		}
		stream, err := r.streamResponsesCandidate(ctx, resolved.Model, candidate, req.Params, req.ThreadID, req.SessionID)
		if err != nil {
			lastErr = err
			if !isRetryableError(err) {
				out.Finish(err)
				return
			}
			r.recordFailure(candidate.Endpoint.ID)
			if err := sleepContext(ctx, r.retryDelay(candidate.Candidate, startedAt, err)); err != nil {
				out.Finish(err)
				return
			}
			continue
		}
		if err := r.forwardResponsesStream(ctx, out, stream, candidate, req); err != nil {
			lastErr = err
			if !isRetryableError(err) {
				out.Finish(err)
				return
			}
			r.recordFailure(candidate.Endpoint.ID)
			if err := sleepContext(ctx, r.retryDelay(candidate.Candidate, startedAt, err)); err != nil {
				out.Finish(err)
				return
			}
			continue
		}
		return
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("responses request budget exhausted for model %s", req.ModelID)
	}
	out.Finish(lastErr)
}

func (r *router) runCanonicalStream(ctx context.Context, out *ai.ProviderEventStream, resolved *ResolvedModel, req *CanonicalRequest) {
	startedAt := time.Now()
	var lastErr error
	for _, candidate := range resolved.Candidates {
		if !withinBudget(startedAt, r.retry.TotalBudget) {
			break
		}
		stream, err := r.streamCanonicalCandidate(ctx, resolved.Model, candidate, req.Params, req.ThreadID, req.SessionID)
		if err != nil {
			lastErr = err
			if !isRetryableError(err) {
				out.Finish(err)
				return
			}
			r.recordFailure(candidate.Endpoint.ID)
			if err := sleepContext(ctx, r.retryDelay(candidate.Candidate, startedAt, err)); err != nil {
				out.Finish(err)
				return
			}
			continue
		}
		if err := r.forwardCanonicalStream(ctx, out, stream, candidate, req); err != nil {
			lastErr = err
			if !isRetryableError(err) {
				out.Finish(err)
				return
			}
			r.recordFailure(candidate.Endpoint.ID)
			if err := sleepContext(ctx, r.retryDelay(candidate.Candidate, startedAt, err)); err != nil {
				out.Finish(err)
				return
			}
			continue
		}
		return
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("canonical request budget exhausted for model %s", req.ModelID)
	}
	out.Finish(lastErr)
}

func (r *router) forwardResponsesStream(ctx context.Context, out *ai.ResponsesEventStream, stream *ai.ResponsesEventStream, candidate ResolvedRouteCandidate, req *ResponsesRequest) error {
	firstEvent, err := readFirstResponsesEvent(ctx, stream, r.retry.FirstByteTimeout)
	if err != nil {
		return err
	}
	if firstEvent.Error != nil {
		return firstEvent.Error
	}
	if firstEvent.Type == "response.completed" && firstEvent.Response == nil {
		return errEmptyStream
	}
	firstEvent = annotateResponsesEvent(firstEvent, candidate.Endpoint.ID)
	if !out.Emit(firstEvent) {
		return context.Canceled
	}
	recordedSuccess := false
	if isSubstantiveResponsesEvent(firstEvent) || firstEvent.Type == "response.completed" {
		r.recordSuccess(candidate.Endpoint.ID, req.ThreadID, req.SessionID)
		recordedSuccess = true
	}
	for stream.Next() {
		event := annotateResponsesEvent(stream.Event(), candidate.Endpoint.ID)
		if event.Error != nil && !recordedSuccess {
			continue
		}
		if !recordedSuccess && (isSubstantiveResponsesEvent(event) || event.Type == "response.completed") {
			r.recordSuccess(candidate.Endpoint.ID, req.ThreadID, req.SessionID)
			recordedSuccess = true
		}
		if !out.Emit(event) {
			return context.Canceled
		}
	}
	if err := stream.Err(); err != nil {
		if !recordedSuccess {
			return err
		}
		out.Finish(err)
		return nil
	}
	if !recordedSuccess {
		r.recordSuccess(candidate.Endpoint.ID, req.ThreadID, req.SessionID)
	}
	out.Close()
	return nil
}

func (r *router) forwardCanonicalStream(ctx context.Context, out *ai.ProviderEventStream, stream *ai.ProviderEventStream, candidate ResolvedRouteCandidate, req *CanonicalRequest) error {
	firstEvent, err := readFirstCanonicalEvent(ctx, stream, r.retry.FirstByteTimeout)
	if err != nil {
		return err
	}
	if firstEvent.Error != nil {
		return firstEvent.Error
	}
	firstEvent = annotateCanonicalEvent(firstEvent, candidate.Endpoint.ID)
	if !out.Emit(firstEvent) {
		return context.Canceled
	}
	recordedSuccess := false
	if isSubstantiveCanonicalEvent(firstEvent) || isSuccessfulCanonicalDoneEvent(firstEvent) {
		r.recordSuccess(candidate.Endpoint.ID, req.ThreadID, req.SessionID)
		recordedSuccess = true
	}
	for stream.Next() {
		event := annotateCanonicalEvent(stream.Event(), candidate.Endpoint.ID)
		if event.Error != nil && !recordedSuccess {
			continue
		}
		if !recordedSuccess && (isSubstantiveCanonicalEvent(event) || isSuccessfulCanonicalDoneEvent(event)) {
			r.recordSuccess(candidate.Endpoint.ID, req.ThreadID, req.SessionID)
			recordedSuccess = true
		}
		if !out.Emit(event) {
			return context.Canceled
		}
	}
	if err := stream.Err(); err != nil {
		if !recordedSuccess {
			return err
		}
		out.Finish(err)
		return nil
	}
	out.Close()
	return nil
}

func isSubstantiveResponsesEvent(event ai.ResponsesStreamEvent) bool {
	switch strings.TrimSpace(event.Type) {
	case "response.output_item.added",
		"response.output_item.done",
		"response.output_text.delta",
		"response.refusal.delta",
		"response.reasoning_text.delta",
		"response.reasoning_summary_text.delta",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done":
		return true
	default:
		return false
	}
}

func isSubstantiveCanonicalEvent(event ai.ProviderEvent) bool {
	switch event.Type {
	case ai.EventCanonicalTextStart,
		ai.EventCanonicalTextDelta,
		ai.EventCanonicalTextEnd,
		ai.EventCanonicalThinkingStart,
		ai.EventCanonicalThinkingDelta,
		ai.EventCanonicalThinkingEnd,
		ai.EventCanonicalToolCallStart,
		ai.EventCanonicalToolCallDelta,
		ai.EventCanonicalToolCallEnd:
		return true
	default:
		return false
	}
}

func isSuccessfulCanonicalDoneEvent(event ai.ProviderEvent) bool {
	return event.Type == ai.EventCanonicalDone && ai.HasSemanticCanonicalResponse(event.Response)
}

func (r *router) completeResponsesCandidate(ctx context.Context, model LogicalModel, candidate ResolvedRouteCandidate, params *ai.ResponsesRequest, threadID string, sessionID string) (*ai.ResponsesResult, error) {
	prov, err := r.providerForCandidate(ctx, model, candidate)
	if err != nil {
		return nil, err
	}
	responsesProv, ok := prov.(ai.ResponsesProvider)
	if !ok {
		return nil, fmt.Errorf("provider for %s does not support responses api", candidate.Endpoint.ID)
	}
	next := cloneResponsesRequest(params)
	next.Model = candidateModelNameForResponses(model, candidate, params)
	if strings.TrimSpace(next.PromptCacheKey) == "" {
		next.PromptCacheKey = defaultPromptCacheKey(threadID, sessionID)
	}
	return responsesProv.CompleteResponses(ctx, next)
}

func (r *router) streamResponsesCandidate(ctx context.Context, model LogicalModel, candidate ResolvedRouteCandidate, params *ai.ResponsesRequest, threadID string, sessionID string) (*ai.ResponsesEventStream, error) {
	prov, err := r.providerForCandidate(ctx, model, candidate)
	if err != nil {
		return nil, err
	}
	responsesProv, ok := prov.(ai.ResponsesProvider)
	if !ok {
		return nil, fmt.Errorf("provider for %s does not support responses api", candidate.Endpoint.ID)
	}
	next := cloneResponsesRequest(params)
	next.Model = candidateModelNameForResponses(model, candidate, params)
	if strings.TrimSpace(next.PromptCacheKey) == "" {
		next.PromptCacheKey = defaultPromptCacheKey(threadID, sessionID)
	}
	return responsesProv.StreamResponses(ctx, next)
}

func (r *router) completeCanonicalCandidate(ctx context.Context, model LogicalModel, candidate ResolvedRouteCandidate, params *ai.ProviderRequest, threadID string, sessionID string) (*ai.ProviderResponse, error) {
	prov, err := r.providerForCandidate(ctx, model, candidate)
	if err != nil {
		return nil, err
	}
	canonical, err := canonicalProviderForEndpoint(prov, candidate.Endpoint, model)
	if err != nil {
		return nil, err
	}
	next := cloneCanonicalRequest(params)
	target := canonicalReplayTarget(model, candidate, next)
	if next.Config.Model == "" {
		next.Config.Model = target.Model
	}
	next = ai.PrepareCanonicalReplayForTarget(next, target)
	if strings.TrimSpace(next.Config.PromptCacheKey) == "" {
		next.Config.PromptCacheKey = defaultPromptCacheKey(threadID, sessionID)
	}
	return canonical.CompleteCanonical(ctx, next)
}

func (r *router) streamCanonicalCandidate(ctx context.Context, model LogicalModel, candidate ResolvedRouteCandidate, params *ai.ProviderRequest, threadID string, sessionID string) (*ai.ProviderEventStream, error) {
	prov, err := r.providerForCandidate(ctx, model, candidate)
	if err != nil {
		return nil, err
	}
	canonical, err := canonicalProviderForEndpoint(prov, candidate.Endpoint, model)
	if err != nil {
		return nil, err
	}
	next := cloneCanonicalRequest(params)
	target := canonicalReplayTarget(model, candidate, next)
	if next.Config.Model == "" {
		next.Config.Model = target.Model
	}
	next = ai.PrepareCanonicalReplayForTarget(next, target)
	if strings.TrimSpace(next.Config.PromptCacheKey) == "" {
		next.Config.PromptCacheKey = defaultPromptCacheKey(threadID, sessionID)
	}
	return canonical.StreamCanonical(ctx, next)
}

// defaultPromptCacheKey follows opagent thread semantics: threadID is the
// durable conversation identity; sessionID is only a transport/provider hint.
func defaultPromptCacheKey(threadID string, sessionID string) string {
	if key := strings.TrimSpace(threadID); key != "" {
		return key
	}
	return strings.TrimSpace(sessionID)
}

func logicalModelAPI(model LogicalModel) string {
	api := strings.TrimSpace(model.API)
	if api == "" {
		return "openai-completions"
	}
	return api
}

func canonicalProviderForEndpoint(prov ai.CanonicalProvider, endpoint ProviderEndpoint, model LogicalModel) (ai.CanonicalProvider, error) {
	if prov == nil {
		return nil, fmt.Errorf("provider for %s is nil", endpoint.ID)
	}
	return prov, nil
}

func (r *router) providerForCandidate(ctx context.Context, model LogicalModel, candidate ResolvedRouteCandidate) (ai.CanonicalProvider, error) {
	cacheKey := providerCacheKey(model, candidate)
	r.mu.RLock()
	cached := r.providerCache[cacheKey]
	r.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}
	prov, err := r.factory(ctx, candidate.Endpoint, model, candidate.Candidate, r.httpClient(candidate.Endpoint))
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached = r.providerCache[cacheKey]; cached != nil {
		return cached, nil
	}
	r.providerCache[cacheKey] = prov
	return prov, nil
}

func (r *router) httpClient(endpoint ProviderEndpoint) *http.Client {
	endpointID := strings.TrimSpace(endpoint.ID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if client, ok := r.httpClients[endpointID]; ok {
		return client
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        r.pool.MaxIdleConns,
		MaxIdleConnsPerHost: r.pool.MaxIdleConnsPerHost,
		MaxConnsPerHost:     r.pool.MaxConnsPerHost,
		IdleConnTimeout:     r.pool.IdleConnTimeout,
	}
	client := &http.Client{Transport: transport}
	r.httpClients[endpointID] = client
	return client
}

func (r *router) resolveCandidates(model LogicalModel, opts ResolveOptions) []ResolvedRouteCandidate {
	now := time.Now()
	r.cleanupState(now)

	allCandidates := make([]ResolvedRouteCandidate, 0, len(model.Routes))
	scored := make([]ResolvedRouteCandidate, 0, len(model.Routes))
	stickyRef := r.stickyProviderRef(opts)
	for _, route := range model.Routes {
		endpoint, ok := r.providers[strings.TrimSpace(route.ProviderRef)]
		if !ok {
			continue
		}
		candidate := ResolvedRouteCandidate{Candidate: route, Endpoint: cloneEndpoint(endpoint)}
		allCandidates = append(allCandidates, candidate)
		if until := r.cooldownUntil(endpoint.ID); until.After(now) {
			continue
		}
		scored = append(scored, candidate)
	}
	if len(scored) == 0 {
		scored = append(scored, allCandidates...)
	}
	out := make([]ResolvedRouteCandidate, 0, len(scored))
	if stickyRef != "" {
		for i, candidate := range scored {
			if candidate.Endpoint.ID != stickyRef {
				continue
			}
			out = append(out, candidate)
			scored = append(scored[:i], scored[i+1:]...)
			break
		}
	}
	buckets := make(map[int][]ResolvedRouteCandidate)
	priorities := make([]int, 0, len(scored))
	seenPriorities := make(map[int]struct{}, len(scored))
	for _, candidate := range scored {
		priority := normalizePriority(candidate.Candidate.Priority)
		buckets[priority] = append(buckets[priority], candidate)
		if _, ok := seenPriorities[priority]; ok {
			continue
		}
		seenPriorities[priority] = struct{}{}
		priorities = append(priorities, priority)
	}
	sort.Ints(priorities)
	for _, priority := range priorities {
		out = append(out, r.weightedShuffleCandidates(buckets[priority])...)
		if r.retry.MaxAttempts > 0 && len(out) >= r.retry.MaxAttempts {
			return out[:r.retry.MaxAttempts]
		}
	}
	return out
}

func (r *router) weightedShuffleCandidates(candidates []ResolvedRouteCandidate) []ResolvedRouteCandidate {
	if len(candidates) < 2 {
		return append([]ResolvedRouteCandidate(nil), candidates...)
	}
	remaining := append([]ResolvedRouteCandidate(nil), candidates...)
	out := make([]ResolvedRouteCandidate, 0, len(candidates))
	for len(remaining) > 0 {
		totalWeight := 0
		for _, candidate := range remaining {
			totalWeight += normalizeWeight(candidate.Candidate.Weight)
		}
		if totalWeight <= 0 {
			sort.SliceStable(remaining, func(i, j int) bool {
				wi := normalizeWeight(remaining[i].Candidate.Weight)
				wj := normalizeWeight(remaining[j].Candidate.Weight)
				if wi != wj {
					return wi > wj
				}
				return remaining[i].Endpoint.ID < remaining[j].Endpoint.ID
			})
			out = append(out, remaining...)
			return out
		}
		target := r.rng.Intn(totalWeight)
		index := 0
		for i, candidate := range remaining {
			target -= normalizeWeight(candidate.Candidate.Weight)
			if target < 0 {
				index = i
				break
			}
		}
		out = append(out, remaining[index])
		remaining = append(remaining[:index], remaining[index+1:]...)
	}
	return out
}

func (r *router) stickyProviderRef(opts ResolveOptions) string {
	key := stickyKey(r.sticky.Scope, opts.ThreadID, opts.SessionID)
	if key == "" {
		return ""
	}
	r.mu.RLock()
	entry, ok := r.stickyMap[key]
	r.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return ""
	}
	return entry.providerRef
}

func (r *router) recordSuccess(providerRef, threadID, sessionID string) {
	key := stickyKey(r.sticky.Scope, threadID, sessionID)
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stickyMap[key] = stickyEntry{
		providerRef: providerRef,
		expiresAt:   time.Now().Add(r.sticky.TTL),
	}
	delete(r.cooldowns, providerRef)
}

func (r *router) recordFailure(providerRef string) {
	if r.retry.Cooldown <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cooldowns[providerRef] = time.Now().Add(r.retry.Cooldown)
}

func (r *router) cooldownUntil(providerRef string) time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cooldowns[providerRef]
}

func (r *router) cleanupState(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, entry := range r.stickyMap {
		if now.After(entry.expiresAt) {
			delete(r.stickyMap, key)
		}
	}
	for key, until := range r.cooldowns {
		if now.After(until) {
			delete(r.cooldowns, key)
		}
	}
}

func routeProviderRefs(routes []RouteCandidate) []string {
	if len(routes) == 0 {
		return nil
	}
	refs := make([]string, 0, len(routes))
	for _, route := range routes {
		if ref := strings.TrimSpace(route.ProviderRef); ref != "" {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}

func (r *router) providerIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	sort.Strings(ids)
	return ids
}

func (r *router) routeCooldowns(routes []RouteCandidate) map[string]string {
	if len(routes) == 0 {
		return nil
	}
	now := time.Now()
	out := make(map[string]string, len(routes))
	for _, route := range routes {
		ref := strings.TrimSpace(route.ProviderRef)
		if ref == "" {
			continue
		}
		until := r.cooldownUntil(ref)
		if until.IsZero() {
			out[ref] = ""
			continue
		}
		out[ref] = until.Sub(now).String()
	}
	return out
}

func (r *router) retryDelay(candidate RouteCandidate, startedAt time.Time, err error) time.Duration {
	if !withinBudget(startedAt, r.retry.TotalBudget) {
		return 0
	}
	if retryErr, ok := ai.AsRetryError(err); ok && retryErr != nil && retryErr.RetryAfterMs > 0 {
		delay := time.Duration(retryErr.RetryAfterMs) * time.Millisecond
		if r.retry.MaxBackoff > 0 && delay > r.retry.MaxBackoff {
			delay = r.retry.MaxBackoff
		}
		if delay < 0 {
			delay = 0
		}
		return delay
	}
	attempt := normalizePriority(candidate.Priority)
	delay := float64(r.retry.InitialBackoff)
	if delay <= 0 {
		return 0
	}
	if attempt > 0 {
		delay = delay * math.Pow(2, float64(attempt))
	}
	if r.retry.MaxBackoff > 0 && time.Duration(delay) > r.retry.MaxBackoff {
		delay = float64(r.retry.MaxBackoff)
	}
	if r.retry.JitterRatio > 0 {
		jitter := delay * r.retry.JitterRatio
		delay = delay - jitter + (r.rng.Float64() * 2 * jitter)
	}
	if delay < 0 {
		delay = 0
	}
	return time.Duration(delay)
}

func defaultProviderFactory(ctx context.Context, endpoint ProviderEndpoint, model LogicalModel, candidate RouteCandidate, httpClient *http.Client) (APIProvider, error) {
	api := logicalModelAPI(model)
	cfg := &op.ModelConfig{
		ID:              model.ID,
		Name:            candidateModelName(model, ResolvedRouteCandidate{Candidate: candidate, Endpoint: endpoint}),
		Provider:        endpoint.Provider,
		API:             api,
		APIKey:          endpoint.APIKey,
		BaseURL:         endpoint.BaseURL,
		ContextWindow:   model.ContextWindow,
		MaxOutputTokens: model.MaxOutputTokens,
		Reasoning:       model.Reasoning,
		ReasoningLevels: append([]string(nil), model.ReasoningLevels...),
		Enabled:         true,
	}
	isGateway := strings.TrimSpace(endpoint.Provider) == "opagent-ai-gateway"
	switch api {
	case "openai-responses":
		if isGateway {
			return provider.NewGatewayResponsesWSProviderWithOptions(cfg, httpClient, endpoint.Headers)
		}
		return provider.NewResponsesProviderWithTransport(cfg, httpClient, endpoint.Headers)
	case "openai-completions", "anthropic-messages", "gemini-native":
		if isGateway {
			return provider.NewGatewayCanonicalWSProviderWithOptions(cfg, httpClient, endpoint.Headers)
		}
		if api == "openai-completions" {
			return provider.NewProviderWithOptions(cfg, provider.OpenAIRequestOptions(httpClient, endpoint.Headers)...)
		}
		if api == "anthropic-messages" {
			return provider.NewAnthropicProviderWithOptions(cfg, provider.AnthropicRequestOptions(httpClient, endpoint.Headers)...)
		}
		return provider.NewGeminiProviderWithHeaders(ctx, cfg, toHTTPHeader(endpoint.Headers))
	default:
		return nil, fmt.Errorf("unsupported api: %s", api)
	}
}

func candidateModelName(model LogicalModel, candidate ResolvedRouteCandidate) string {
	if strings.TrimSpace(candidate.Candidate.UpstreamModel) != "" {
		return strings.TrimSpace(candidate.Candidate.UpstreamModel)
	}
	return strings.TrimSpace(model.Name)
}

func candidateModelNameForResponses(model LogicalModel, candidate ResolvedRouteCandidate, params *ai.ResponsesRequest) string {
	if strings.TrimSpace(candidate.Candidate.UpstreamModel) != "" {
		return strings.TrimSpace(candidate.Candidate.UpstreamModel)
	}
	if params != nil && strings.TrimSpace(params.Model) != "" {
		return strings.TrimSpace(params.Model)
	}
	return strings.TrimSpace(model.Name)
}

func candidateModelNameForCanonical(model LogicalModel, candidate ResolvedRouteCandidate, params *ai.ProviderRequest) string {
	if strings.TrimSpace(candidate.Candidate.UpstreamModel) != "" {
		return strings.TrimSpace(candidate.Candidate.UpstreamModel)
	}
	if params != nil && strings.TrimSpace(params.Config.Model) != "" {
		return strings.TrimSpace(params.Config.Model)
	}
	return strings.TrimSpace(model.Name)
}

func canonicalReplayTarget(model LogicalModel, candidate ResolvedRouteCandidate, params *ai.ProviderRequest) ai.ReplayTarget {
	return ai.ReplayTarget{
		ProviderRef: strings.TrimSpace(candidate.Endpoint.ID),
		Provider:    strings.TrimSpace(candidate.Endpoint.Provider),
		API:         logicalModelAPI(model),
		Model:       candidateModelNameForCanonical(model, candidate, params),
	}
}

func annotateCanonicalResponse(resp *ai.ProviderResponse, providerRef string) *ai.ProviderResponse {
	if resp == nil {
		return nil
	}
	if resp.Message.ProviderState == nil {
		resp.Message.ProviderState = &ai.ProviderState{}
	}
	resp.Message.ProviderState.ProviderRef = strings.TrimSpace(providerRef)
	return resp
}

func annotateCanonicalEvent(event ai.ProviderEvent, providerRef string) ai.ProviderEvent {
	if event.Partial != nil {
		if event.Partial.ProviderState == nil {
			event.Partial.ProviderState = &ai.ProviderState{}
		}
		event.Partial.ProviderState.ProviderRef = strings.TrimSpace(providerRef)
	}
	if event.Response != nil {
		event.Response = annotateCanonicalResponse(event.Response, providerRef)
	}
	return event
}

func annotateResponsesResult(resp *ai.ResponsesResult, providerRef string) *ai.ResponsesResult {
	if resp == nil {
		return nil
	}
	resp.ProviderRef = strings.TrimSpace(providerRef)
	return resp
}

func annotateResponsesEvent(event ai.ResponsesStreamEvent, providerRef string) ai.ResponsesStreamEvent {
	if event.Response != nil {
		event.Response = annotateResponsesResult(event.Response, providerRef)
	}
	return event
}

func cloneResponsesRequest(req *ai.ResponsesRequest) *ai.ResponsesRequest {
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

func cloneCanonicalRequest(req *ai.ProviderRequest) *ai.ProviderRequest {
	if req == nil {
		return &ai.ProviderRequest{}
	}
	out := *req
	out.Context.Messages = append([]ai.ConversationMessage(nil), req.Context.Messages...)
	out.Context.Tools = append([]ai.ToolDefinition(nil), req.Context.Tools...)
	out.Config.Include = append([]string(nil), req.Config.Include...)
	if len(req.Config.ToolChoice) > 0 {
		out.Config.ToolChoice = append([]byte(nil), req.Config.ToolChoice...)
	}
	return &out
}

func providerCacheKey(model LogicalModel, candidate ResolvedRouteCandidate) string {
	return strings.Join([]string{
		strings.TrimSpace(candidate.Endpoint.ID),
		strings.TrimSpace(model.ID),
		strings.TrimSpace(candidate.Candidate.UpstreamModel),
		stableHeaderCacheKey(candidate.Endpoint.Headers),
	}, "|")
}

func singleModelProviderCacheKey(cfg *op.ModelConfig) string {
	if cfg == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(cfg.Key),
		strings.TrimSpace(cfg.ID),
		strings.TrimSpace(cfg.Name),
		strings.TrimSpace(cfg.Provider),
		strings.TrimSpace(cfg.API),
		strings.TrimSpace(cfg.BaseURL),
		strings.TrimSpace(cfg.APIKey),
		stableHeaderCacheKey(cfg.Headers),
		fmt.Sprintf("%t", cfg.Reasoning),
		fmt.Sprintf("%d", cfg.ContextWindow),
		fmt.Sprintf("%d", cfg.MaxOutputTokens),
		strings.Join(cfg.ReasoningLevels, ","),
	}, "|")
}

func stableHeaderCacheKey(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, strings.TrimSpace(key))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		value := strings.TrimSpace(headers[key])
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "\n")
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneEndpoint(endpoint ProviderEndpoint) ProviderEndpoint {
	out := endpoint
	if endpoint.Headers != nil {
		out.Headers = make(map[string]string, len(endpoint.Headers))
		for key, value := range endpoint.Headers {
			out.Headers[key] = value
		}
	}
	return out
}

func cloneModel(model LogicalModel) LogicalModel {
	out := model
	out.ReasoningLevels = append([]string(nil), model.ReasoningLevels...)
	out.Routes = append([]RouteCandidate(nil), model.Routes...)
	return out
}

func normalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 3
	}
	if policy.InitialBackoff <= 0 {
		policy.InitialBackoff = 150 * time.Millisecond
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = 2 * time.Second
	}
	if policy.TotalBudget <= 0 {
		policy.TotalBudget = 5 * time.Second
	}
	if policy.FirstByteTimeout <= 0 {
		policy.FirstByteTimeout = 12 * time.Second
	}
	if policy.Cooldown == 0 {
		policy.Cooldown = 30 * time.Second
	} else if policy.Cooldown < 0 {
		policy.Cooldown = 0
	}
	if policy.JitterRatio < 0 {
		policy.JitterRatio = 0
	}
	if policy.JitterRatio > 1 {
		policy.JitterRatio = 1
	}
	return policy
}

func normalizeStickyPolicy(policy StickyPolicy) StickyPolicy {
	if policy.Scope == "" {
		policy.Scope = StickyScopeThread
	}
	if policy.TTL <= 0 {
		policy.TTL = 30 * time.Minute
	}
	return policy
}

func normalizePoolPolicy(policy PoolPolicy) PoolPolicy {
	if policy.Isolation == "" {
		policy.Isolation = "provider_endpoint"
	}
	if policy.MaxIdleConns <= 0 {
		policy.MaxIdleConns = 256
	}
	if policy.MaxIdleConnsPerHost <= 0 {
		policy.MaxIdleConnsPerHost = 64
	}
	if policy.MaxConnsPerHost <= 0 {
		policy.MaxConnsPerHost = 128
	}
	if policy.IdleConnTimeout <= 0 {
		policy.IdleConnTimeout = 90 * time.Second
	}
	return policy
}

func stickyKey(scope StickyScope, threadID, sessionID string) string {
	switch scope {
	case StickyScopeThread:
		threadID = strings.TrimSpace(threadID)
		if threadID == "" {
			return ""
		}
		return "thread:" + threadID
	case StickyScopeSession:
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return ""
		}
		return "session:" + sessionID
	default:
		return ""
	}
}

func normalizePriority(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func normalizeWeight(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}

func toHTTPHeader(headers map[string]string) http.Header {
	out := http.Header{}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out.Set(key, value)
	}
	return out
}

func withinBudget(startedAt time.Time, budget time.Duration) bool {
	if budget <= 0 {
		return true
	}
	return time.Since(startedAt) < budget
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readFirstResponsesEvent(ctx context.Context, stream *ai.ResponsesEventStream, timeout time.Duration) (ai.ResponsesStreamEvent, error) {
	if stream == nil {
		return ai.ResponsesStreamEvent{}, fmt.Errorf("responses stream is nil")
	}
	type result struct {
		event ai.ResponsesStreamEvent
		ok    bool
	}
	ch := make(chan result, 1)
	go func() {
		ok := stream.Next()
		if !ok {
			ch <- result{ok: false}
			return
		}
		ch <- result{event: stream.Event(), ok: true}
	}()
	if timeout <= 0 {
		res := <-ch
		if !res.ok {
			if err := stream.Err(); err != nil {
				return ai.ResponsesStreamEvent{}, err
			}
			return ai.ResponsesStreamEvent{}, errEmptyStream
		}
		return res.event, nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ai.ResponsesStreamEvent{}, ctx.Err()
	case <-timer.C:
		return ai.ResponsesStreamEvent{}, fmt.Errorf("first byte timeout")
	case res := <-ch:
		if !res.ok {
			if err := stream.Err(); err != nil {
				return ai.ResponsesStreamEvent{}, err
			}
			return ai.ResponsesStreamEvent{}, errEmptyStream
		}
		return res.event, nil
	}
}

func readFirstCanonicalEvent(ctx context.Context, stream *ai.ProviderEventStream, timeout time.Duration) (ai.ProviderEvent, error) {
	if stream == nil {
		return ai.ProviderEvent{}, fmt.Errorf("canonical stream is nil")
	}
	type result struct {
		event ai.ProviderEvent
		ok    bool
	}
	ch := make(chan result, 1)
	go func() {
		ok := stream.Next()
		if !ok {
			ch <- result{ok: false}
			return
		}
		ch <- result{event: stream.Event(), ok: true}
	}()
	if timeout <= 0 {
		res := <-ch
		if !res.ok {
			if err := stream.Err(); err != nil {
				return ai.ProviderEvent{}, err
			}
			return ai.ProviderEvent{}, errEmptyStream
		}
		return res.event, nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ai.ProviderEvent{}, ctx.Err()
	case <-timer.C:
		return ai.ProviderEvent{}, fmt.Errorf("first byte timeout")
	case res := <-ch:
		if !res.ok {
			if err := stream.Err(); err != nil {
				return ai.ProviderEvent{}, err
			}
			return ai.ProviderEvent{}, errEmptyStream
		}
		return res.event, nil
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if retryErr := ai.NormalizeRetryError(err); retryErr != nil {
		return retryErr.Retryable
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

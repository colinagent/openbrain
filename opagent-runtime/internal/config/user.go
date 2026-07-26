package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

const defaultModelContextWindow int64 = 128000
const localModelsSchemaVersion = 6
const openbrainProviderKey = "openbrain"

type localModelsJSON struct {
	Version         int                           `json:"version"`
	DefaultModelKey string                        `json:"defaultModelKey"`
	Strategies      *op.ModelStrategies           `json:"strategies,omitempty"`
	Providers       map[string]localProviderEntry `json:"providers"`
	Models          []localModelEntry             `json:"models,omitempty"`
}

type localProviderEntry struct {
	Label   string                    `json:"label"`
	Managed bool                      `json:"managed,omitempty"`
	API     string                    `json:"api"`
	BaseURL string                    `json:"baseUrl"`
	APIKey  string                    `json:"apiKey"`
	Models  []localProviderModelEntry `json:"models"`
}

type localProviderModelEntry struct {
	Key              string   `json:"key,omitempty"`
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Enabled          *bool    `json:"enabled"`
	API              string   `json:"api,omitempty"`
	BaseURL          string   `json:"baseUrl,omitempty"`
	APIKey           string   `json:"apiKey,omitempty"`
	ContextWindow    int64    `json:"contextWindow,omitempty"`
	MaxOutputTokens  int64    `json:"maxOutputTokens,omitempty"`
	Reasoning        *bool    `json:"reasoning,omitempty"`
	ReasoningControl string   `json:"reasoningControl,omitempty"`
	ReasoningLevels  []string `json:"reasoningLevels,omitempty"`
	ServiceTiers     []string `json:"serviceTiers,omitempty"`
}

type localModelEntry struct {
	Key              string   `json:"key"`
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Enabled          *bool    `json:"enabled"`
	Provider         string   `json:"provider"`
	ProviderLabel    string   `json:"providerLabel"`
	Managed          bool     `json:"managed"`
	API              string   `json:"api"`
	Source           string   `json:"source"`
	BaseURL          string   `json:"baseUrl"`
	APIKey           string   `json:"apiKey"`
	ContextWindow    int64    `json:"contextWindow"`
	MaxOutputTokens  int64    `json:"maxOutputTokens"`
	Reasoning        *bool    `json:"reasoning"`
	ReasoningControl string   `json:"reasoningControl"`
	ReasoningLevels  []string `json:"reasoningLevels"`
	ServiceTiers     []string `json:"serviceTiers"`
}

func normalizeReasoningControl(value string, reasoning bool, levels []string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "level":
		return "level"
	case "toggle":
		return "toggle"
	}
	if len(levels) > 0 {
		return "level"
	}
	if reasoning {
		return "toggle"
	}
	return ""
}

func normalizeLocalModelKey(value string) string {
	return strings.TrimSpace(value)
}

func normalizeLocalProviderKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeServiceTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "priority":
		return "priority"
	case "flex":
		return "flex"
	default:
		return ""
	}
}

func normalizeServiceTierList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		tier := NormalizeServiceTier(raw)
		if tier == "" {
			continue
		}
		if _, ok := seen[tier]; ok {
			continue
		}
		seen[tier] = struct{}{}
		out = append(out, tier)
	}
	return out
}

func isValidLocalProviderKey(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			if i == 0 && (r == '.' || r == '_' || r == '-') {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func isManagedOpenBrainProviderKey(value string) bool {
	return normalizeLocalProviderKey(value) == openbrainProviderKey
}

func defaultLocalProviderKeyForAPI(api string) string {
	switch strings.TrimSpace(api) {
	case "anthropic-messages":
		return "anthropic"
	case "gemini-native":
		return "google"
	default:
		return "openai"
	}
}

func buildLocalModelKey(provider, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	provider = normalizeLocalProviderKey(provider)
	if provider == "" {
		return ""
	}
	return provider + ":" + id
}

func managedLocalProviderKeys(raw localModelsJSON) map[string]struct{} {
	keys := make(map[string]struct{}, len(raw.Providers))
	for rawProviderKey := range raw.Providers {
		providerKey := normalizeLocalProviderKey(rawProviderKey)
		if providerKey == "" {
			continue
		}
		if isManagedOpenBrainProviderKey(providerKey) {
			keys[providerKey] = struct{}{}
		}
	}
	return keys
}

func normalizeManagedLocalModelKey(value string, _ map[string]struct{}) string {
	key := normalizeLocalModelKey(value)
	separator := strings.Index(key, ":")
	if separator <= 0 || separator >= len(key)-1 {
		return key
	}
	providerKey := normalizeLocalProviderKey(key[:separator])
	if providerKey == "" {
		return key
	}
	modelID := strings.TrimSpace(key[separator+1:])
	return buildLocalModelKey(providerKey, modelID)
}

func flattenLocalProviders(raw localModelsJSON) ([]localModelEntry, error) {
	if len(raw.Providers) == 0 {
		return nil, fmt.Errorf("models.json providers must not be empty")
	}
	providerKeys := make([]string, 0, len(raw.Providers))
	for key := range raw.Providers {
		providerKeys = append(providerKeys, key)
	}
	sort.Strings(providerKeys)

	flat := make([]localModelEntry, 0)
	for _, rawProviderKey := range providerKeys {
		providerKey := normalizeLocalProviderKey(rawProviderKey)
		if providerKey == "" {
			return nil, fmt.Errorf("models.json provider key is required")
		}
		if providerKey != rawProviderKey {
			return nil, fmt.Errorf("models.json provider key %s must be normalized as %s", rawProviderKey, providerKey)
		}
		if !isValidLocalProviderKey(providerKey) {
			return nil, fmt.Errorf("models.json provider %s key is invalid", providerKey)
		}
		provider := raw.Providers[rawProviderKey]
		if len(provider.Models) == 0 {
			return nil, fmt.Errorf("models.json provider %s models must not be empty", providerKey)
		}
		providerAPI := strings.TrimSpace(provider.API)
		providerBaseURL := strings.TrimSpace(provider.BaseURL)
		providerAPIKey := strings.TrimSpace(provider.APIKey)
		if provider.Managed && providerKey != openbrainProviderKey {
			return nil, fmt.Errorf("models.json managed provider must use reserved key %s", openbrainProviderKey)
		}
		managedProvider := isManagedOpenBrainProviderKey(providerKey)
		for _, item := range provider.Models {
			rawID := strings.TrimSpace(item.ID)
			if rawID == "" {
				return nil, fmt.Errorf("models.json provider %s model id is required", providerKey)
			}
			id := rawID
			expectedKey := buildLocalModelKey(providerKey, id)
			key := normalizeLocalModelKey(item.Key)
			if key == "" {
				key = expectedKey
			}
			if key != expectedKey {
				return nil, fmt.Errorf("models.json model %s key must be %s", id, expectedKey)
			}
			api := strings.TrimSpace(item.API)
			if api == "" {
				api = providerAPI
			}
			if api == "" {
				api = "openai-completions"
			}
			baseURL := strings.TrimSpace(item.BaseURL)
			if baseURL == "" {
				baseURL = providerBaseURL
			}
			apiKey := strings.TrimSpace(item.APIKey)
			if apiKey == "" {
				apiKey = providerAPIKey
			}
			if !managedProvider {
				if baseURL == "" {
					return nil, fmt.Errorf("models.json provider %s model %s baseUrl is required", providerKey, id)
				}
				if apiKey == "" {
					return nil, fmt.Errorf("models.json provider %s model %s apiKey is required", providerKey, id)
				}
			}
			flat = append(flat, localModelEntry{
				Key:              expectedKey,
				ID:               id,
				Label:            item.Label,
				Enabled:          item.Enabled,
				Provider:         providerKey,
				ProviderLabel:    strings.TrimSpace(provider.Label),
				Managed:          managedProvider,
				API:              api,
				BaseURL:          baseURL,
				APIKey:           apiKey,
				ContextWindow:    item.ContextWindow,
				MaxOutputTokens:  item.MaxOutputTokens,
				Reasoning:        item.Reasoning,
				ReasoningControl: item.ReasoningControl,
				ReasoningLevels:  item.ReasoningLevels,
				ServiceTiers:     normalizeServiceTierList(item.ServiceTiers),
			})
		}
	}
	return flat, nil
}

func validateLocalModelsConfig(raw localModelsJSON) error {
	if raw.Version != localModelsSchemaVersion {
		return fmt.Errorf("models.json version must be %d", localModelsSchemaVersion)
	}
	defaultModelKey := normalizeLocalModelKey(raw.DefaultModelKey)
	if defaultModelKey == "" {
		return fmt.Errorf("models.json defaultModelKey is required")
	}
	flatModels, err := flattenLocalProviders(raw)
	if err != nil {
		return err
	}
	normalizedDefaultModelKey := normalizeManagedLocalModelKey(defaultModelKey, managedLocalProviderKeys(raw))

	seenKeys := make(map[string]struct{}, len(flatModels))
	hasDefault := false
	for _, item := range flatModels {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("models.json model id is required")
		}
		provider := normalizeLocalProviderKey(item.Provider)
		if provider == "" {
			return fmt.Errorf("models.json model %s provider is required", id)
		}
		if !isValidLocalProviderKey(provider) {
			return fmt.Errorf("models.json model %s provider key is invalid", id)
		}
		key := normalizeLocalModelKey(item.Key)
		if key == "" {
			return fmt.Errorf("models.json model %s key is required", id)
		}
		expectedKey := buildLocalModelKey(provider, id)
		if key != expectedKey {
			return fmt.Errorf("models.json model %s key must be %s", id, expectedKey)
		}
		if _, exists := seenKeys[key]; exists {
			return fmt.Errorf("models.json duplicate model key %s", key)
		}
		seenKeys[key] = struct{}{}
		if key == normalizedDefaultModelKey {
			hasDefault = true
		}
	}
	if !hasDefault {
		return fmt.Errorf("models.json defaultModelKey %s not found", normalizedDefaultModelKey)
	}
	if strategies := normalizeLocalModelStrategies(raw.Strategies, managedLocalProviderKeys(raw)); strategies != nil && strategies.Auto != nil {
		if defaultChatModelKey := strings.TrimSpace(strategies.Auto.DefaultChatModelID); defaultChatModelKey != "" {
			foundEnabled := false
			for _, item := range flatModels {
				if normalizeLocalModelKey(item.Key) != defaultChatModelKey {
					continue
				}
				if item.Enabled == nil || *item.Enabled {
					foundEnabled = true
				}
				break
			}
			if !foundEnabled {
				return fmt.Errorf("models.json Default Chat Model %s must reference an enabled model", defaultChatModelKey)
			}
		}
	}
	return nil
}

func LoadLocalUserProfile() (*op.UserProfile, error) {
	cfg := GetSystem()
	if cfg == nil || strings.TrimSpace(cfg.BaseDir) == "" {
		emptyProfile := &op.UserProfile{}
		setUserConfig(&op.UserConfig{Profile: emptyProfile})
		return emptyProfile, nil
	}

	userCfg, err := loadLocalUserConfig(cfg.BaseDir)
	if err != nil {
		return nil, err
	}

	profile := &op.UserProfile{}
	profilePath := filepath.Join(cfg.BaseDir, "configs", "user", "profile.json")
	if loadedProfile, ok, err := readOptionalJSON[op.UserProfile](profilePath); err != nil {
		return nil, err
	} else if ok {
		profile = &loadedProfile
	}
	if strings.TrimSpace(profile.UID) == "" && userCfg.Auth != nil {
		profile.UID = strings.TrimSpace(userCfg.Auth.UID)
	}
	userCfg.Profile = profile
	setUserConfig(userCfg)
	return profile, nil
}

func ReloadLocalUserConfig() (*op.UserProfile, error) {
	return LoadLocalUserProfile()
}

func loadLocalUserConfig(baseDir string) (*op.UserConfig, error) {
	userCfg := &op.UserConfig{}
	env := op.EnvLocal
	currentHostID := ""
	if systemCfg := GetSystem(); systemCfg != nil {
		if strings.TrimSpace(systemCfg.Env) != "" {
			env = strings.TrimSpace(systemCfg.Env)
		}
		currentHostID = strings.TrimSpace(systemCfg.HostID)
	}

	authPath := filepath.Join(baseDir, "configs", "user", "auth.json")
	if auth, ok, err := readOptionalJSON[op.AuthConfig](authPath); err != nil {
		return nil, err
	} else if ok {
		if auth.Version != 2 || strings.TrimSpace(auth.Token) == "" ||
			strings.TrimSpace(auth.UID) == "" || strings.TrimSpace(auth.DeploymentID) == "" ||
			strings.TrimSpace(auth.OrgID) == "" || strings.TrimSpace(auth.IdentityID) == "" ||
			strings.TrimSpace(auth.ConnectionID) == "" || strings.TrimSpace(auth.AuthMethod) == "" ||
			strings.TrimSpace(auth.AuthTime) == "" || strings.TrimSpace(auth.ExpiresAt) == "" {
			return nil, fmt.Errorf("tenant-bound auth config version 2 is required")
		}
		userCfg.Auth = &auth
	}

	modelsPath := filepath.Join(baseDir, "configs", "user", "models.json")
	modelsSource, ok, err := readOptionalJSON[localModelsJSON](modelsPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Still try to load nodes even when models.json is absent.
		nodesPath := filepath.Join(baseDir, "configs", "user", "nodes.json")
		if rawNodes, ok, err := readOptionalJSON[map[string]op.OpNode](nodesPath); err != nil {
			slog.Warn("failed to load local user nodes config", "path", nodesPath, "error", err)
		} else if ok {
			userCfg.Nodes = normalizeNodesMap(rawNodes, env, currentHostID)
		}
		return userCfg, nil
	}
	if err := validateLocalModelsConfig(modelsSource); err != nil {
		return nil, err
	}
	flatModels, err := flattenLocalProviders(modelsSource)
	if err != nil {
		return nil, err
	}
	managedProviderKeys := managedLocalProviderKeys(modelsSource)
	userCfg.DefaultModelKey = normalizeManagedLocalModelKey(modelsSource.DefaultModelKey, managedProviderKeys)
	userCfg.Strategies = normalizeLocalModelStrategies(modelsSource.Strategies, managedProviderKeys)

	gatewayBaseURL := ""
	gatewayToken := ""
	if userCfg.Auth != nil {
		gatewayBaseURL = resolveAIGatewayBaseURL(userCfg.Auth)
		gatewayToken = strings.TrimSpace(userCfg.Auth.Token)
	}

	models := make([]op.ModelConfig, 0, len(flatModels))
	for _, item := range flatModels {
		model, ok, err := toModelConfig(item, gatewayBaseURL, gatewayToken)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		models = append(models, *model)
	}
	userCfg.Models = models

	nodesPath := filepath.Join(baseDir, "configs", "user", "nodes.json")
	if rawNodes, ok, err := readOptionalJSON[map[string]op.OpNode](nodesPath); err != nil {
		slog.Warn("failed to load local user nodes config", "path", nodesPath, "error", err)
	} else if ok {
		userCfg.Nodes = normalizeNodesMap(rawNodes, env, currentHostID)
	}
	return userCfg, nil
}

func normalizeLocalModelStrategies(strategies *op.ModelStrategies, managedProviderKeys map[string]struct{}) *op.ModelStrategies {
	if strategies == nil || strategies.Auto == nil {
		return nil
	}
	auto := strategies.Auto
	nextAuto := &op.ModelAutoStrategy{
		DefaultChatModelID:                   normalizeManagedLocalModelKey(auto.DefaultChatModelID, managedProviderKeys),
		DefaultChatThinkingLevel:             strings.TrimSpace(auto.DefaultChatThinkingLevel),
		DefaultInlineCompletionModelID:       normalizeManagedLocalModelKey(auto.DefaultInlineCompletionModelID, managedProviderKeys),
		DefaultInlineCompletionThinkingLevel: strings.TrimSpace(auto.DefaultInlineCompletionThinkingLevel),
	}
	if nextAuto.DefaultChatModelID == "" &&
		nextAuto.DefaultChatThinkingLevel == "" &&
		nextAuto.DefaultInlineCompletionModelID == "" &&
		nextAuto.DefaultInlineCompletionThinkingLevel == "" {
		return nil
	}
	return &op.ModelStrategies{Auto: nextAuto}
}

func normalizeNodesMap(raw map[string]op.OpNode, env string, currentHostID string) map[string]op.OpNode {
	if len(raw) == 0 {
		return nil
	}

	sortedKeys := make([]string, 0, len(raw))
	for k := range raw {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	out := make(map[string]op.OpNode, len(raw))
	for _, mapKey := range sortedKeys {
		node := raw[mapKey]
		nodeID := strings.TrimSpace(node.ID)
		if nodeID == "" {
			nodeID = strings.TrimSpace(mapKey)
		}
		if nodeID == "" {
			continue
		}
		node.ID = nodeID
		if strings.TrimSpace(node.HostID) == "" {
			node.HostID = strings.TrimSpace(currentHostID)
		}
		if strings.TrimSpace(node.Kind) == "" {
			if kind, ok := op.NodeKindFromID(nodeID); ok {
				node.Kind = string(kind)
			}
		}
		if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.Kind) == "" {
			slog.Warn("skip node with invalid id", "nodeID", nodeID)
			continue
		}
		out[nodeID] = node
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeReasoningLevels(levels []string) []string {
	if len(levels) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(levels))
	out := make([]string, 0, len(levels))
	for _, raw := range levels {
		level := strings.ToLower(strings.TrimSpace(raw))
		if level == "" {
			continue
		}
		if _, exists := seen[level]; exists {
			continue
		}
		seen[level] = struct{}{}
		out = append(out, level)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toModelConfig(item localModelEntry, gatewayBaseURL, gatewayToken string) (*op.ModelConfig, bool, error) {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		return nil, false, fmt.Errorf("models.json model id is required")
	}
	if item.Enabled != nil && !*item.Enabled {
		return nil, false, nil
	}
	key := normalizeLocalModelKey(item.Key)
	contextWindow := item.ContextWindow
	if contextWindow <= 0 {
		contextWindow = defaultModelContextWindow
	}
	providerKey := normalizeLocalProviderKey(item.Provider)
	if providerKey == "" {
		providerKey = defaultLocalProviderKeyForAPI(item.API)
	}
	api := strings.TrimSpace(item.API)
	reasoning := item.Reasoning != nil && *item.Reasoning
	reasoningLevels := normalizeReasoningLevels(item.ReasoningLevels)
	reasoningControl := normalizeReasoningControl(item.ReasoningControl, reasoning, reasoningLevels)
	serviceTiers := normalizeServiceTierList(item.ServiceTiers)
	expectedKey := buildLocalModelKey(providerKey, id)
	if key == "" {
		return nil, false, fmt.Errorf("models.json model %s key is required", id)
	}
	if key != expectedKey {
		return nil, false, fmt.Errorf("models.json model %s key must be %s", id, expectedKey)
	}
	if item.Managed || isManagedOpenBrainProviderKey(providerKey) {
		if gatewayBaseURL == "" || gatewayToken == "" {
			return nil, false, nil
		}
		if api == "" {
			api = "openai-completions"
		}
		return &op.ModelConfig{
			Key:              key,
			ID:               id,
			Name:             id,
			Provider:         "opagent-ai-gateway",
			API:              api,
			Source:           providerKey,
			APIKey:           gatewayToken,
			BaseURL:          gatewayAPIBaseURL(gatewayBaseURL),
			Headers:          nil,
			ContextWindow:    contextWindow,
			MaxOutputTokens:  item.MaxOutputTokens,
			Reasoning:        reasoning,
			ReasoningControl: reasoningControl,
			ReasoningLevels:  reasoningLevels,
			ServiceTiers:     serviceTiers,
			Enabled:          true,
		}, true, nil
	}
	baseURL := strings.TrimSpace(item.BaseURL)
	apiKey := strings.TrimSpace(item.APIKey)
	if baseURL == "" || apiKey == "" || providerKey == "" {
		return nil, false, fmt.Errorf("models.json provider model %s requires provider/baseUrl/apiKey", key)
	}
	if api == "" {
		api = "openai-completions"
	}
	return &op.ModelConfig{
		Key: key,
		ID:  id,
		// Provider requests must use the configured upstream model ID.
		Name:             id,
		Provider:         providerKey,
		API:              api,
		Source:           "provider",
		APIKey:           apiKey,
		BaseURL:          baseURL,
		ContextWindow:    contextWindow,
		MaxOutputTokens:  item.MaxOutputTokens,
		Reasoning:        reasoning,
		ReasoningControl: reasoningControl,
		ReasoningLevels:  reasoningLevels,
		ServiceTiers:     serviceTiers,
		Enabled:          true,
	}, true, nil
}

func resolveAIGatewayBaseURL(auth *op.AuthConfig) string {
	if auth == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(auth.AIGateway), "/")
}

func gatewayAPIBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return baseURL + "/v1"
}

func readOptionalJSON[T any](path string) (T, bool, error) {
	var out T
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, false, nil
		}
		return out, false, err
	}
	if err := common.UnmarshalJSONC(raw, &out); err != nil {
		return out, false, err
	}
	return out, true, nil
}

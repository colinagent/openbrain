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
const localModelsSchemaVersion = 5
const legacyLocalModelsSchemaVersion = 2
const legacyProviderKeyModelsSchemaVersion = 3
const previousProviderModelsSchemaVersion = 4
const opagentProviderKey = "opagent"

var upstreamModelIDNamespaces = map[string]struct{}{
	"anthropic":  {},
	"deepseek":   {},
	"google":     {},
	"meta-llama": {},
	"mistral":    {},
	"mistralai":  {},
	"moonshotai": {},
	"openai":     {},
	"qwen":       {},
	"x-ai":       {},
	"z-ai":       {},
}

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
	provider := strings.ToLower(strings.TrimSpace(value))
	if provider == "opagent-ai-gateway" {
		return opagentProviderKey
	}
	return provider
}

func normalizePublicLocalModelID(value string) string {
	modelID := strings.TrimSpace(value)
	slash := strings.Index(modelID, "/")
	if slash <= 0 || slash >= len(modelID)-1 {
		return modelID
	}
	namespace := strings.ToLower(strings.TrimSpace(modelID[:slash]))
	if _, ok := upstreamModelIDNamespaces[namespace]; !ok {
		return modelID
	}
	return strings.TrimSpace(modelID[slash+1:])
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

func isManagedOpagentProviderKey(value string) bool {
	provider := normalizeLocalProviderKey(value)
	if provider == opagentProviderKey {
		return true
	}
	if !strings.HasPrefix(provider, "org-") {
		return false
	}
	if len(provider) < len("org-a") || len(provider) > len("org-")+63 {
		return false
	}
	for i, r := range provider {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			if i == len("org-") && r == '-' {
				return false
			}
			continue
		}
		return false
	}
	last := provider[len(provider)-1]
	return last >= 'a' && last <= 'z' || last >= '0' && last <= '9'
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
	for rawProviderKey, provider := range raw.Providers {
		providerKey := normalizeLocalProviderKey(rawProviderKey)
		if providerKey == "" {
			continue
		}
		if provider.Managed || isManagedOpagentProviderKey(providerKey) {
			keys[providerKey] = struct{}{}
		}
	}
	return keys
}

func normalizeManagedLocalModelKey(value string, managedProviderKeys map[string]struct{}) string {
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
	if _, ok := managedProviderKeys[providerKey]; ok {
		modelID = normalizePublicLocalModelID(modelID)
	}
	return buildLocalModelKey(providerKey, modelID)
}

func migrateLocalModelsConfig(raw localModelsJSON) (localModelsJSON, error) {
	if raw.Version == previousProviderModelsSchemaVersion {
		raw.Version = localModelsSchemaVersion
		return raw, nil
	}
	if raw.Version != legacyLocalModelsSchemaVersion && raw.Version != legacyProviderKeyModelsSchemaVersion {
		return raw, nil
	}

	type migratedProviderModel struct {
		entry   localProviderModelEntry
		baseURL string
		apiKey  string
	}
	type migratedProvider struct {
		label  string
		models []migratedProviderModel
	}

	providers := make(map[string]migratedProvider)
	keyMap := make(map[string]string, len(raw.Models))
	for _, item := range raw.Models {
		rawID := strings.TrimSpace(item.ID)
		if rawID == "" {
			continue
		}
		legacyKey := normalizeLocalModelKey(item.Key)
		if legacyKey == "" && strings.TrimSpace(item.Source) != "" {
			legacyKey = strings.ToLower(strings.TrimSpace(item.Source)) + ":" + rawID
		}
		providerKey := opagentProviderKey
		if raw.Version == legacyLocalModelsSchemaVersion {
			if strings.ToLower(strings.TrimSpace(item.Source)) != "gateway" {
				providerKey = normalizeLocalProviderKey(item.Provider)
				if providerKey == "" {
					providerKey = defaultLocalProviderKeyForAPI(item.API)
				}
			}
		} else {
			providerKey = normalizeLocalProviderKey(item.Provider)
			if providerKey == "" {
				providerKey = defaultLocalProviderKeyForAPI(item.API)
			}
		}
		id := rawID
		if isManagedOpagentProviderKey(providerKey) {
			id = normalizePublicLocalModelID(rawID)
		}
		nextKey := buildLocalModelKey(providerKey, id)
		if legacyKey != "" && nextKey != "" {
			keyMap[legacyKey] = nextKey
		}

		provider := providers[providerKey]
		if provider.label == "" {
			provider.label = strings.TrimSpace(item.ProviderLabel)
		}
		provider.models = append(provider.models, migratedProviderModel{
			entry: localProviderModelEntry{
				Key:              nextKey,
				ID:               id,
				Label:            item.Label,
				Enabled:          item.Enabled,
				API:              strings.TrimSpace(item.API),
				ContextWindow:    item.ContextWindow,
				MaxOutputTokens:  item.MaxOutputTokens,
				Reasoning:        item.Reasoning,
				ReasoningControl: item.ReasoningControl,
				ReasoningLevels:  item.ReasoningLevels,
				ServiceTiers:     normalizeServiceTierList(item.ServiceTiers),
			},
			baseURL: strings.TrimSpace(item.BaseURL),
			apiKey:  strings.TrimSpace(item.APIKey),
		})
		providers[providerKey] = provider
	}

	migratedProviders := make(map[string]localProviderEntry, len(providers))
	for providerKey, provider := range providers {
		next := localProviderEntry{Label: provider.label}
		if isManagedOpagentProviderKey(providerKey) {
			next.Managed = true
			next.Models = make([]localProviderModelEntry, 0, len(provider.models))
			for _, model := range provider.models {
				next.Models = append(next.Models, model.entry)
			}
			migratedProviders[providerKey] = next
			continue
		}

		baseURLCounts := make(map[string]int)
		apiKeyCounts := make(map[string]int)
		for _, model := range provider.models {
			if model.baseURL != "" {
				baseURLCounts[model.baseURL]++
			}
			if model.apiKey != "" {
				apiKeyCounts[model.apiKey]++
			}
		}
		if len(baseURLCounts) == 1 {
			for value := range baseURLCounts {
				next.BaseURL = value
			}
		}
		if len(apiKeyCounts) == 1 {
			for value := range apiKeyCounts {
				next.APIKey = value
			}
		}
		next.Models = make([]localProviderModelEntry, 0, len(provider.models))
		for _, model := range provider.models {
			entry := model.entry
			if model.baseURL != "" && model.baseURL != next.BaseURL {
				entry.BaseURL = model.baseURL
			}
			if model.apiKey != "" && model.apiKey != next.APIKey {
				entry.APIKey = model.apiKey
			}
			next.Models = append(next.Models, entry)
		}
		migratedProviders[providerKey] = next
	}

	migratedDefaultModelKey := keyMap[normalizeLocalModelKey(raw.DefaultModelKey)]
	if migratedDefaultModelKey == "" {
		migratedDefaultModelKey = normalizeLocalModelKey(raw.DefaultModelKey)
	}
	return localModelsJSON{
		Version:         localModelsSchemaVersion,
		DefaultModelKey: migratedDefaultModelKey,
		Providers:       migratedProviders,
	}, nil
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
		managedProvider := provider.Managed || isManagedOpagentProviderKey(providerKey)
		for _, item := range provider.Models {
			rawID := strings.TrimSpace(item.ID)
			if rawID == "" {
				return nil, fmt.Errorf("models.json provider %s model id is required", providerKey)
			}
			id := rawID
			if managedProvider {
				id = normalizePublicLocalModelID(rawID)
			}
			expectedKey := buildLocalModelKey(providerKey, id)
			key := normalizeLocalModelKey(item.Key)
			if key == "" {
				key = expectedKey
			}
			legacyExpectedKey := ""
			if rawID != id {
				legacyExpectedKey = buildLocalModelKey(providerKey, rawID)
			}
			if key != expectedKey && key != legacyExpectedKey {
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
	modelsSource, err = migrateLocalModelsConfig(modelsSource)
	if err != nil {
		return nil, err
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
	if item.Managed || isManagedOpagentProviderKey(providerKey) {
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
			Headers:          gatewayHeadersForProvider(providerKey),
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

func gatewayHeadersForProvider(providerKey string) map[string]string {
	providerKey = normalizeLocalProviderKey(providerKey)
	if providerKey == "" || providerKey == opagentProviderKey {
		return nil
	}
	return map[string]string{"X-Org-ID": providerKey}
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

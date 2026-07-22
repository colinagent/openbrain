package config

import (
	"strings"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	DefaultBaseDir = "~/.openbrain"
	// System config supports JSONC: ~/.openbrain/configs/config.json
	// DefaultConfigDir      = "~/.openbrain/configs/config.json"
	// DefaultAgentsDir      = "~/.openbrain/agents"
	// DefaultLogDir         = "~/.openbrain/logs/opagent-runtime"
	DefaultDebug                        = false
	DefaultEnv                          = op.EnvLocal
	DefaultCloudOSBaseURL               = "http://127.0.0.1:8080"
	DefaultMessageStorage               = "markdown"
	DefaultRuntimeUpdateManifestURL     = "https://download.op-agent.com/runtime/latest/manifest.json"
	DefaultRuntimeUpdateCheckInterval   = "10m"
	DefaultRuntimeUpdateCheckTimeout    = "8s"
	DefaultRuntimeUpdateIdleGracePeriod = "15s"
	DefaultRuntimeUpdateDownloadDir     = "run/runtime-update"
	DefaultMaxThreads                   = 10000
)

var (
	// command line flags
	CmdBaseDir        = ""
	CmdDebug          = false
	CmdEnv            = op.EnvLocal
	CmdCloudOSBaseURL = ""
)

// type SystemConfig = op.SystemConfig
// type UserConfig = op.UserConfig
// type AuthConfig = op.AuthConfig
// type MongoDBConfig = op.MongoDBConfig
// type MemoryConfig = op.MemoryConfig
// type ObjectStoreConfig = op.ObjectStoreConfig
// type FSObjectStoreConfig = op.FSObjectStoreConfig
// type S3ObjectStoreConfig = op.S3ObjectStoreConfig
// type MongoObjectStoreConfig = op.MongoObjectStoreConfig
// type CloudOSConfig = op.CloudOSConfig
// type ModelConfig = op.ModelConfig
// type CompactionConfig = op.CompactionConfig
// type HeartbeatConfig = op.HeartbeatConfig

var (
	mu           sync.RWMutex
	configs      = &op.Config{}
	secretValues = map[string]string{}
)

// GetConfig returns the global config instance
// func GetSystem() *SystemConfig {
// 	return GetSystem()
// }

func GetConfig() *op.Config {
	mu.RLock()
	defer mu.RUnlock()
	return configs
}

func GetSystem() *op.SystemConfig {
	mu.RLock()
	defer mu.RUnlock()
	return configs.System
}

func SetSystem(c *op.SystemConfig) {
	mu.Lock()
	defer mu.Unlock()
	configs.System = c
}

func GetUserProfile() *op.UserProfile {
	mu.RLock()
	defer mu.RUnlock()
	if configs.User == nil || configs.User.Profile == nil {
		return nil
	}
	out := *configs.User.Profile
	return &out
}

func SetUserProfile(c *op.UserProfile) {
	mu.Lock()
	defer mu.Unlock()
	if configs.User == nil {
		configs.User = &op.UserConfig{}
	}
	if c == nil {
		configs.User.Profile = nil
		return
	}
	out := *c
	configs.User.Profile = &out
}

func GetUserConfig() *op.UserConfig {
	mu.RLock()
	defer mu.RUnlock()
	if configs.User == nil {
		return nil
	}
	out := &op.UserConfig{}
	out.DefaultModelKey = strings.TrimSpace(configs.User.DefaultModelKey)
	out.Strategies = cloneModelStrategies(configs.User.Strategies)
	if configs.User.Profile != nil {
		profile := *configs.User.Profile
		out.Profile = &profile
	}
	if configs.User.Auth != nil {
		auth := *configs.User.Auth
		out.Auth = &auth
	}
	if len(configs.User.Models) > 0 {
		out.Models = cloneModelConfigs(configs.User.Models)
	}
	if len(configs.User.Nodes) > 0 {
		out.Nodes = make(map[string]op.OpNode, len(configs.User.Nodes))
		for key, node := range configs.User.Nodes {
			out.Nodes[key] = node
		}
	}
	return out
}

func setUserConfig(c *op.UserConfig) {
	mu.Lock()
	defer mu.Unlock()
	if c == nil {
		configs.User = nil
		return
	}
	out := &op.UserConfig{}
	out.DefaultModelKey = strings.TrimSpace(c.DefaultModelKey)
	out.Strategies = cloneModelStrategies(c.Strategies)
	if c.Profile != nil {
		profile := *c.Profile
		out.Profile = &profile
	}
	if c.Auth != nil {
		auth := *c.Auth
		out.Auth = &auth
	}
	if len(c.Models) > 0 {
		out.Models = cloneModelConfigs(c.Models)
	}
	if len(c.Nodes) > 0 {
		out.Nodes = make(map[string]op.OpNode, len(c.Nodes))
		for key, node := range c.Nodes {
			out.Nodes[key] = node
		}
	}
	configs.User = out
}

func SetSecrets(values map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	secretValues = cloneSecretValues(values)
}

func GetSecrets() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	return cloneSecretValues(secretValues)
}

func cloneSecretValues(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

func cloneModelConfigs(models []op.ModelConfig) []op.ModelConfig {
	out := make([]op.ModelConfig, len(models))
	for i := range models {
		out[i] = models[i]
		out[i].ReasoningControl = models[i].ReasoningControl
		if len(models[i].Headers) > 0 {
			out[i].Headers = cloneSecretValues(models[i].Headers)
		}
		if len(models[i].ReasoningLevels) > 0 {
			out[i].ReasoningLevels = append([]string(nil), models[i].ReasoningLevels...)
		}
		if len(models[i].ServiceTiers) > 0 {
			out[i].ServiceTiers = append([]string(nil), models[i].ServiceTiers...)
		}
	}
	return out
}

func cloneModelStrategies(strategies *op.ModelStrategies) *op.ModelStrategies {
	if strategies == nil {
		return nil
	}
	out := &op.ModelStrategies{}
	if strategies.Auto != nil {
		auto := *strategies.Auto
		auto.DefaultChatModelID = strings.TrimSpace(auto.DefaultChatModelID)
		auto.DefaultChatThinkingLevel = strings.TrimSpace(auto.DefaultChatThinkingLevel)
		auto.DefaultInlineCompletionModelID = strings.TrimSpace(auto.DefaultInlineCompletionModelID)
		auto.DefaultInlineCompletionThinkingLevel = strings.TrimSpace(auto.DefaultInlineCompletionThinkingLevel)
		out.Auto = &auto
	}
	if out.Auto == nil {
		return nil
	}
	return out
}

// // SetConfig sets the global config instance (called by run.InitConfig)
// func Set(c *Config) {
// 	SetSystem(c)
// }

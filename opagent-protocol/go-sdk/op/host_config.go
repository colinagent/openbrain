package op

type Config struct {
	System      *SystemConfig     `json:"system,omitempty" mapstructure:"system"`
	User        *UserConfig       `json:"user,omitempty" mapstructure:"user"`
	MongoDB     MongoDBConfig     `json:"mongodb,omitempty" mapstructure:"mongodb"`
	Memory      MemoryConfig      `json:"memory,omitempty" mapstructure:"memory"`
	ObjectStore ObjectStoreConfig `json:"objectStore,omitempty" mapstructure:"objectStore"`
	Compaction  CompactionConfig  `json:"compaction,omitempty" mapstructure:"compaction"`
}

const (
	EnvLocal = "local"
	EnvCloud = "cloud"

	LocalUser = "local"
)

// SystemConfig is process-level startup configuration.
type SystemConfig struct {
	HostID        string              `json:"hostID,omitempty" mapstructure:"hostID"`
	HostName      string              `json:"hostName,omitempty" mapstructure:"hostName"`
	Ips           []string            `json:"ips,omitempty" mapstructure:"ips"`
	ConfigFile    string              `json:"configFile,omitempty" mapstructure:"configFile"`
	Heartbeat     HeartbeatConfig     `json:"heartbeat,omitempty" mapstructure:"heartbeat"`
	RuntimeUpdate RuntimeUpdateConfig `json:"runtimeUpdate,omitempty" mapstructure:"runtimeUpdate"`
	ThreadStorage ThreadStorageConfig `json:"threadStorage,omitempty" mapstructure:"threadStorage"`
	Debug         bool                `json:"debug,omitempty" mapstructure:"debug"`
	BaseDir       string              `json:"baseDir,omitempty" mapstructure:"baseDir"`
	Systool       map[string]ToolSpec `json:"systool,omitempty" mapstructure:"systool"`
	Env           string              `json:"env,omitempty" mapstructure:"env"`
	CloudOS       CloudOSConfig       `json:"cloudos,omitempty" mapstructure:"cloudos"`
	ModelIDs      []string            `json:"models,omitempty" mapstructure:"models"` // model ids
}

// SystemConfigResult is the read-only runtime view returned by config/system/get.
// DefaultWorkspace is derived by the runtime from BaseDir; it is not a startup
// configuration field.
type SystemConfigResult struct {
	SystemConfig
	DefaultWorkspace string `json:"defaultWorkspace"`
}

type UserConfig struct {
	DefaultModelKey string            `json:"defaultModelKey,omitempty" mapstructure:"defaultModelKey"`
	Strategies      *ModelStrategies  `json:"strategies,omitempty" mapstructure:"strategies"`
	Profile         *UserProfile      `json:"profile,omitempty" mapstructure:"profile"`
	Auth            *AuthConfig       `json:"auth,omitempty" mapstructure:"auth"`
	Models          []ModelConfig     `json:"models,omitempty" mapstructure:"models"`
	Nodes           map[string]OpNode `json:"nodes,omitempty" mapstructure:"nodes"` // map(nodeID)OpNode
}

type ModelStrategies struct {
	Auto *ModelAutoStrategy `json:"auto,omitempty" mapstructure:"auto"`
}

type ModelAutoStrategy struct {
	DefaultChatModelID                   string `json:"defaultChatModelID,omitempty" mapstructure:"defaultChatModelID"`
	DefaultChatThinkingLevel             string `json:"defaultChatThinkingLevel,omitempty" mapstructure:"defaultChatThinkingLevel"`
	DefaultInlineCompletionModelID       string `json:"defaultInlineCompletionModelID,omitempty" mapstructure:"defaultInlineCompletionModelID"`
	DefaultInlineCompletionThinkingLevel string `json:"defaultInlineCompletionThinkingLevel,omitempty" mapstructure:"defaultInlineCompletionThinkingLevel"`
}

type AuthConfig struct {
	BaseURL       string `json:"baseURL,omitempty" mapstructure:"baseURL"`
	Gateway       string `json:"gateway,omitempty" mapstructure:"gateway"`
	AIGateway     string `json:"aiGateway,omitempty" mapstructure:"aiGateway"`
	Token         string `json:"token,omitempty" mapstructure:"token"`
	UID           string `json:"uid,omitempty" mapstructure:"uid"`
	Email         string `json:"email,omitempty" mapstructure:"email"`
	ActiveOrgID   string `json:"activeOrgID,omitempty" mapstructure:"activeOrgID"`
	ActiveOrgName string `json:"activeOrgName,omitempty" mapstructure:"activeOrgName"`
	UpdatedAt     int64  `json:"updatedAt,omitempty" mapstructure:"updatedAt"`
}

// UserConfig is per-user runtime configuration.
type UserProfile struct {
	UID         string `json:"uid,omitempty" mapstructure:"uid"`
	UserName    string `json:"username,omitempty" mapstructure:"userName"`
	Email       string `json:"email,omitempty" mapstructure:"email"`
	Avatar      string `json:"avatar,omitempty" mapstructure:"avatar"`
	LocalAvatar string `json:"localAvatar,omitempty" mapstructure:"localAvatar"`
	Provider    string `json:"provider,omitempty" mapstructure:"provider"`
	Address     string `json:"address,omitempty" mapstructure:"address"`
	UpdatedAt   int64  `json:"updatedAt,omitempty" mapstructure:"updatedAt"`
}

// HeartbeatConfig holds heartbeat reporter controls.
type HeartbeatConfig struct {
	Enabled  *bool  `json:"enabled,omitempty" mapstructure:"enabled"`
	Interval string `json:"interval,omitempty" mapstructure:"interval"`
}

type RuntimeUpdateConfig struct {
	Enabled         *bool  `json:"enabled,omitempty" mapstructure:"enabled"`
	ManifestURL     string `json:"manifestURL,omitempty" mapstructure:"manifestURL"`
	CheckInterval   string `json:"checkInterval,omitempty" mapstructure:"checkInterval"`
	CheckTimeout    string `json:"checkTimeout,omitempty" mapstructure:"checkTimeout"`
	IdleGracePeriod string `json:"idleGracePeriod,omitempty" mapstructure:"idleGracePeriod"`
	DownloadDir     string `json:"downloadDir,omitempty" mapstructure:"downloadDir"`
}

type RuntimeUpdateState struct {
	CurrentVersion string `json:"currentVersion,omitempty"`
	TargetVersion  string `json:"targetVersion,omitempty"`
	StagedVersion  string `json:"stagedVersion,omitempty"`
	Phase          string `json:"phase,omitempty"`
	Downloaded     bool   `json:"downloaded,omitempty"`
	Applying       bool   `json:"applying,omitempty"`
	LastCheckedAt  string `json:"lastCheckedAt,omitempty"`
	LastError      string `json:"lastError,omitempty"`
}

type ThreadStorageConfig struct {
	MaxThreads int `json:"maxThreads,omitempty" mapstructure:"maxThreads"`
}

type MongoDBConfig struct {
	URI      string `json:"uri,omitempty" mapstructure:"uri"`
	Database string `json:"database,omitempty" mapstructure:"database"`
}

type MemoryConfig struct {
	Storage    string `json:"storage" mapstructure:"storage"`
	Cache      string `json:"cache,omitempty" mapstructure:"cache"`
	SQLitePath string `json:"sqlitePath,omitempty" mapstructure:"sqlitePath"`
}

type ObjectStoreConfig struct {
	// Type: fs | s3 | mongodb
	Type    string                 `json:"type,omitempty" mapstructure:"type"`
	FS      FSObjectStoreConfig    `json:"fs,omitempty" mapstructure:"fs"`
	S3      S3ObjectStoreConfig    `json:"s3,omitempty" mapstructure:"s3"`
	MongoDB MongoObjectStoreConfig `json:"mongodb,omitempty" mapstructure:"mongodb"`
}

type FSObjectStoreConfig struct {
	// BaseDir: 存放对象的根目录
	BaseDir string `json:"baseDir,omitempty" mapstructure:"baseDir"`
}

type S3ObjectStoreConfig struct {
	Endpoint        string `json:"endpoint,omitempty" mapstructure:"endpoint"`
	Region          string `json:"region,omitempty" mapstructure:"region"`
	Bucket          string `json:"bucket,omitempty" mapstructure:"bucket"`
	Prefix          string `json:"prefix,omitempty" mapstructure:"prefix"`
	AccessKeyID     string `json:"accessKeyId,omitempty" mapstructure:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey,omitempty" mapstructure:"secretAccessKey"`
	SessionToken    string `json:"sessionToken,omitempty" mapstructure:"sessionToken"`
	ForcePathStyle  bool   `json:"forcePathStyle,omitempty" mapstructure:"forcePathStyle"`
}

type MongoObjectStoreConfig struct {
	// 可选：未配置时会回退使用顶层 mongodb 配置
	URI      string `json:"uri,omitempty" mapstructure:"uri"`
	Database string `json:"database,omitempty" mapstructure:"database"`
	// GridFSBucket: GridFS bucket 名（默认 images）
	GridFSBucket string `json:"gridfsBucket,omitempty" mapstructure:"gridfsBucket"`
}

// HostCloudOSConfig cloud file system access
type CloudOSConfig struct {
	BaseURL string `json:"baseURL,omitempty" mapstructure:"baseURL"`
}

// HostCompactionConfig controls history compaction behavior.
// Defaults aligned with pi-mono: reserveTokens=16384, keepRecentTokens=20000.
type CompactionConfig struct {
	// Enabled toggles automatic compaction. Default: true.
	Enabled *bool `json:"enabled,omitempty" mapstructure:"enabled"`
	// ModelID is the model used to generate compaction summaries.
	ModelID string `json:"modelID,omitempty" mapstructure:"modelID"`
	// ReserveTokens is the token headroom reserved for the LLM response.
	// Compaction triggers when contextTokens > contextWindow - ReserveTokens.
	// Default: 16384.
	ReserveTokens int64 `json:"reserveTokens,omitempty" mapstructure:"reserveTokens"`
	// KeepRecentTokens is the number of recent tokens to keep verbatim
	// (not summarized) during compaction. Default: 20000.
	KeepRecentTokens int64 `json:"keepRecentTokens,omitempty" mapstructure:"keepRecentTokens"`
}

// type HostConfigGetResponse struct {
// 	Config *HostConfig `json:"config"`
// }

type HostSecretGetResponse struct {
	SecretKeyID string            `json:"secretKeyId,omitempty"`
	Value       string            `json:"value,omitempty"`
	Secrets     map[string]string `json:"secrets,omitempty"`
}

// --------------- model --------------------
type ModelConfig struct {
	Key              string            `json:"key,omitempty"`
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Provider         string            `json:"provider"`
	API              string            `json:"api,omitempty"`
	APIKey           string            `json:"apiKey"`
	BaseURL          string            `json:"baseURL,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	ContextWindow    int64             `json:"contextWindow,omitempty"`
	MaxOutputTokens  int64             `json:"maxOutputTokens,omitempty"`
	Reasoning        bool              `json:"reasoning,omitempty"`
	ReasoningControl string            `json:"reasoningControl,omitempty"`
	ReasoningLevels  []string          `json:"reasoningLevels,omitempty"`
	ServiceTiers     []string          `json:"serviceTiers,omitempty"`
	Enabled          bool              `json:"enabled,omitempty"`
	Source           string            `json:"source,omitempty"` // gateway | custom
}

type TokenUsage struct {
	Prompt     int    `json:"prompt"`
	Completion int    `json:"completion"`
	Total      int    `json:"total"`
	Content    string `json:"content"`
}

package op

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/xid"
)

// ---------------------------------------------------------------------------
// OpNode — the universal in-memory representation for agents, skills, tools.
// ---------------------------------------------------------------------------

type NodeKind string

const (
	NodeKindAgent NodeKind = "agent"
	NodeKindSkill NodeKind = "skill"
	NodeKindTools NodeKind = "tools"
)

var SystoolNames = []string{
	"shell",
	"read",
	"write",
	"edit",
	"agent_task",
	"message_publish",
	"message_update",
	"message_read",
	"message_subscribe",
	"message_ack",
}

const (
	SystoolModeDefault   = "default"
	SystoolModeAllowlist = "allowlist"
	SystoolModeDisabled  = "disabled"
)

type OpNode struct {
	ID      string   `json:"id"`                                     // persistent node id, e.g. agent-cm1...
	HostID  string   `json:"hostID,omitempty" mapstructure:"hostID"` // host id
	UID     string   `json:"uid"`                                    // owner/tenant identifier
	OpCodes []OpCode `json:"opCodes,omitempty"`
	Kind    string   `json:"kind"` // agent | skill | tools
	URI     string   `json:"uri"`  // resource locator (file://, cloudos://, ...)
	Cwd     string   `json:"cwd"`  // current working directory
	Tags    []string `json:"tags,omitempty"`
	Run     Run      `json:"run,omitempty"`
	Meta    any      `json:"meta,omitempty"` // AgentMeta | SkillMeta | ToolsMeta
}

// ---------------------------------------------------------------------------
// Node identity format: uid:hostID:kind:uri
//
// Examples:
//   local:   local:host-a1b2:agent:file:///root/.openbrain/agents/li/.agent/AGENT.md
//   local:   local:host-a1b2:tools:file:///root/.openbrain/tools/system-tools/TOOL.md
//   cloud:   user123:host-c3d4:skill:file:///home/user/skills/search/SKILL.md
//
// The first three colons delimit uid, hostID and kind.
// Everything after the third colon is the URI (catch-all, may contain colons).
// ---------------------------------------------------------------------------

func BuildNodeIdentity(uid, hostID, kind, uri string, env string) string {
	if env == EnvLocal {
		uid = "local"
	}
	return strings.TrimSpace(uid) + ":" + strings.TrimSpace(hostID) + ":" + strings.TrimSpace(kind) + ":" + strings.TrimSpace(uri)
}

// ---------------------------------------------------------------------------
// Node ID — deterministic UUIDv5 with kind prefix.
// ---------------------------------------------------------------------------

var nodeIDNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("OpAgent"))

func NormalizeNodeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func NodeKindFromID(id string) (NodeKind, bool) {
	trimmed := strings.TrimSpace(id)
	for _, kind := range []NodeKind{NodeKindAgent, NodeKindSkill, NodeKindTools} {
		if strings.HasPrefix(trimmed, string(kind)+"-") {
			return kind, true
		}
	}
	return NodeKind(""), false
}

// ComputeNodeID returns a deterministic UUIDv5 suffix from identity.
func ComputeNodeID(identity string) string {
	return uuid.NewSHA1(nodeIDNamespace, []byte(strings.TrimSpace(identity))).String()
}

// BuildNodeID builds id in `kind-uuidv5` format from uid/hostID/kind/uri.
func BuildNodeID(uid, hostID string, kind NodeKind, uri string, env string) string {
	normalizedKind := NormalizeNodeKind(string(kind))
	switch NodeKind(normalizedKind) {
	case NodeKindAgent, NodeKindSkill, NodeKindTools:
	default:
		normalizedKind = strings.TrimSpace(normalizedKind)
	}
	identity := BuildNodeIdentity(strings.TrimSpace(uid), strings.TrimSpace(hostID), normalizedKind, strings.TrimSpace(uri), env)
	return normalizedKind + "-" + ComputeNodeID(identity)
}

func BuildNode(uid, hostID string, kind NodeKind, uri string, env string, tags []string, run Run, opCodes []OpCode, meta any) *OpNode {
	id := BuildNodeID(uid, hostID, kind, uri, env)
	return &OpNode{
		ID:      id,
		HostID:  strings.TrimSpace(hostID),
		UID:     uid,
		OpCodes: opCodes,
		Kind:    string(kind),
		URI:     uri,
		Tags:    tags,
		Run:     run,
		Meta:    meta,
	}
}

// ---------------------------------------------------------------------------
// URI helpers
// ---------------------------------------------------------------------------

// PathToURI converts a local path to a file:// URI.
// Optional isDir controls whether a trailing slash is appended.
func PathToURI(path string, isDir ...bool) string {
	// Normalize to forward slashes
	p := strings.ReplaceAll(path, "\\", "/")
	// Handle Windows drive letters: C:/path -> /C:/path
	if len(p) >= 2 && p[1] == ':' {
		p = "/" + p
	}
	withTrailingSlash := len(isDir) > 0 && isDir[0]
	if withTrailingSlash && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return "file://" + p
}

// URIToPath extracts the local path from a file:// URI.
// Returns empty string if URI is not a file:// URI.
func URIToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	p := strings.TrimPrefix(uri, "file://")
	// Handle Windows: /C:/path -> C:/path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	// Remove trailing slash for directory paths
	p = strings.TrimSuffix(p, "/")
	return p
}

// URIToDir extracts a local directory path from a file URI.
// For file paths that point to a file, its parent directory is returned.
func URIToDir(uri string) string {
	p := URIToPath(uri)
	if p == "" {
		return ""
	}
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasSuffix(p, "/") {
		return strings.TrimSuffix(p, "/")
	}
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return ""
	}
	return p[:idx]
}

// ---------------------------------------------------------------------------
// Meta types (no prompt — loaded on-demand from URI)
// ---------------------------------------------------------------------------

type AgentMeta struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Avatar      string   `json:"avatar,omitempty"`
	MaxToken    int64    `json:"maxToken,omitempty"`
	BindAgentID string   `json:"bindAgentID,omitempty"` // optional bind target agent node ID
	ToolServers []string `json:"toolServers,omitempty"` // tool server OpNode IDs
	SysTools    []string `json:"sysTools,omitempty"`    // allowlisted built-in systool names when SysToolMode=allowlist
	SysToolMode string   `json:"sysToolMode,omitempty"` // default | allowlist | disabled
	Skills      []string `json:"skills,omitempty"`      // skill OpNode IDs
	SubAgents   []string `json:"subAgents,omitempty"`   // agent OpNode IDs
	Model       string   `json:"model,omitempty"`       // local models.json modelKey
}

type SkillMeta struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

type ToolUse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema,omitempty"`
}

type ToolSpec struct {
	ServerID    string `json:"serverID"`
	Name        string `json:"name"`
	Sampling    bool   `json:"sampling,omitempty"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema,omitempty"`
}

type ToolsMeta struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Tools       []*ToolSpec `json:"tools,omitempty"`
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

type Run struct {
	Command []string          `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Header  map[string]string `json:"header,omitempty"`
	Daemon  bool              `json:"daemon,omitempty"`
}

func (r Run) HasEndpoint() bool {
	return len(r.Command) > 0 || strings.TrimSpace(r.URL) != ""
}

// Validate checks whether run config is internally consistent.
func (r Run) Validate() error {
	if len(r.Command) > 0 && strings.TrimSpace(r.URL) != "" {
		return errors.New("run: command and url are mutually exclusive")
	}
	if len(r.Header) > 0 && strings.TrimSpace(r.URL) == "" {
		return errors.New("run: header is only allowed with url")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Connection types
// ---------------------------------------------------------------------------

type TransportType string

const (
	Stdio          TransportType = "stdio"
	HttpStreamable TransportType = "http_streamable"
)

type OpNodeParams struct {
	OpCode  OpCode `json:"opCode"`
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content,omitempty"`
}

func (*OpNodeParams) isParams() {}
func (p *OpNodeParams) UnmarshalJSON(data []byte) error {
	type params OpNodeParams
	var wire struct {
		params
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.params.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*p = OpNodeParams(wire.params)
	return nil
}

// ---------------------------------------------------------------------------
// Agent call / OpAgent protocol types
// ---------------------------------------------------------------------------

type CallAgentHandler func(context.Context, *CallAgentRequest) (*CallAgentResult, error)

type AgentListChangedParams struct {
	Meta `json:"_meta,omitempty"`
}

func (x *AgentListChangedParams) isParams() {}

type OpAgentParams struct {
	OpCode  OpCode `json:"opCode"`
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content,omitempty"`
}

func (*OpAgentParams) isParams() {}

func (p *OpAgentParams) UnmarshalJSON(data []byte) error {
	type params OpAgentParams
	var wire struct {
		params
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.params.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*p = OpAgentParams(wire.params)
	return nil
}

type CallNodeParams struct {
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content,omitempty"`
}

func (*CallNodeParams) isParams() {}

func (p *CallNodeParams) UnmarshalJSON(data []byte) error {
	type params CallNodeParams
	var wire struct {
		params
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.params.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*p = CallNodeParams(wire.params)
	return nil
}

type CallAgentParams struct {
	AgentID string `json:"agentID"`
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content,omitempty"`
}

func (*CallAgentParams) isParams() {}

func (p *CallAgentParams) UnmarshalJSON(data []byte) error {
	type params CallAgentParams
	var wire struct {
		params
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.params.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*p = CallAgentParams(wire.params)
	return nil
}

type OpNodeResult struct {
	OpCode  OpCode `json:"opCode"`
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content"`
}

func (*OpNodeResult) isResult() {}

func (p *OpNodeResult) UnmarshalJSON(data []byte) error {
	type result OpAgentResult
	var wire struct {
		result
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.result.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*p = OpNodeResult(wire.result)
	return nil
}

type OpAgentResult struct {
	OpCode  OpCode `json:"opCode"`
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content"`
}

func (*OpAgentResult) isResult() {}
func (r *OpAgentResult) UnmarshalJSON(data []byte) error {
	type result OpAgentResult
	var wire struct {
		result
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.result.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*r = OpAgentResult(wire.result)
	return nil
}

type CallAgentResult struct {
	AgentID string `json:"agentID"`
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content"`
}

type CallNodeResult struct {
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content"`
}

func (*CallNodeResult) isResult() {}
func (r *CallNodeResult) UnmarshalJSON(data []byte) error {
	type result CallNodeResult
	var wire struct {
		result
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.result.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*r = CallNodeResult(wire.result)
	return nil
}

func (*CallAgentResult) isResult() {}
func (r *CallAgentResult) UnmarshalJSON(data []byte) error {
	type result CallAgentResult
	var wire struct {
		result
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Content != nil {
		var err error
		if wire.result.Content, err = contentFromWire(wire.Content, nil); err != nil {
			return err
		}
	}
	*r = CallAgentResult(wire.result)
	return nil
}

type serverAgent struct {
	agent   *AgentMeta
	handler CallAgentHandler
}

// ---------------------------------------------------------------------------
// User task / status (unchanged)
// ---------------------------------------------------------------------------

type UserTask struct {
	UID       string   `bson:"uid" json:"uid"`
	AppID     string   `bson:"appID,omitempty" json:"appID,omitempty"`
	TaskID    string   `bson:"taskID,omitempty" json:"taskID,omitempty"`
	TaskName  string   `bson:"taskName,omitempty" json:"taskName,omitempty"`
	ThreadIDs []string `bson:"threadIDs,omitempty" json:"threadIDs,omitempty"`
}

func GetUserTaskID() string {
	return fmt.Sprintf("ut-%s", xid.New().String())
}

type Status string

const (
	Status_Init       Status = "init"
	Status_Pending    Status = "pending"
	Status_Started    Status = "started"
	Status_InProgress Status = "in_progress"
	Status_Completed  Status = "completed"
	Status_Failed     Status = "failed"
	Status_Running    Status = "running"
	Status_Cancelled  Status = "cancelled"
	Status_Stopped    Status = "stopped"
)

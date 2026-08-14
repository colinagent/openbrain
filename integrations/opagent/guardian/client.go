package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
)

const (
	defaultTimeout       = 12 * time.Second
	maximumRequestBytes  = 32 * 1024
	maximumResponseBytes = 16 * 1024
)

var (
	bearerPattern = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/-]+=*`)
	secretPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)\s*[:=]\s*[^\s,;]+`)
	keyPattern    = regexp.MustCompile(`\b(?:sk|rk|pk)-[A-Za-z0-9_-]{12,}\b`)
	safeName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,191}$`)
)

type Client struct {
	baseDir string
	client  *http.Client
}

type authConfig struct {
	Version   int    `json:"version"`
	AIGateway string `json:"aiGateway"`
	Token     string `json:"token"`
}

type reviewRequest struct {
	RequestID                string           `json:"requestID"`
	ToolName                 string           `json:"toolName"`
	CommandSummary           string           `json:"commandSummary,omitempty"`
	IntentSummary            string           `json:"intentSummary,omitempty"`
	ProfileID                string           `json:"profileID"`
	PolicyVersion            int              `json:"policyVersion"`
	PolicyDigest             string           `json:"policyDigest"`
	RiskTags                 []string         `json:"riskTags,omitempty"`
	Resources                []reviewResource `json:"resources,omitempty"`
	ExistingThreadGrantCount int              `json:"existingThreadGrantCount,omitempty"`
	AgentID                  string           `json:"agentID,omitempty"`
	ParentAgentID            string           `json:"parentAgentID,omitempty"`
}

type reviewResource struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type reviewResponse struct {
	RequestID     string                    `json:"requestID"`
	Decision      op.GuardianReviewDecision `json:"decision"`
	Risk          op.GuardianRisk           `json:"risk"`
	Reason        string                    `json:"reason"`
	ReviewID      string                    `json:"reviewID"`
	PolicyVersion string                    `json:"policyVersion"`
}

func NewClient(baseDir string) *Client {
	return &Client{
		baseDir: strings.TrimSpace(baseDir),
		client: &http.Client{
			Timeout: defaultTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("Guardian redirects are disabled")
			},
		},
	}
}

func (client *Client) Review(ctx context.Context, request op.ApprovalRequest) (op.GuardianReviewResult, error) {
	if client == nil || strings.TrimSpace(client.baseDir) == "" {
		return op.GuardianReviewResult{}, errors.New("Guardian client is not configured")
	}
	auth, err := loadAuth(filepath.Join(client.baseDir, "configs", "user", "auth.json"))
	if err != nil {
		return op.GuardianReviewResult{}, err
	}
	payload := sanitizeRequest(request)
	raw, err := json.Marshal(payload)
	if err != nil {
		return op.GuardianReviewResult{}, err
	}
	if len(raw) > maximumRequestBytes {
		return op.GuardianReviewResult{}, errors.New("Guardian review metadata exceeds the size limit")
	}
	endpoint := guardianEndpoint(auth.AIGateway)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return op.GuardianReviewResult{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+auth.Token)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Request-ID", payload.RequestID)
	response, err := client.client.Do(httpRequest)
	if err != nil {
		return op.GuardianReviewResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return op.GuardianReviewResult{}, fmt.Errorf("Guardian service responded %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maximumResponseBytes+1))
	decoder.DisallowUnknownFields()
	var result reviewResponse
	if err := decoder.Decode(&result); err != nil {
		return op.GuardianReviewResult{}, errors.New("Guardian response is invalid")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return op.GuardianReviewResult{}, err
	}
	if strings.TrimSpace(result.RequestID) != payload.RequestID {
		return op.GuardianReviewResult{}, errors.New("Guardian response requestID mismatch")
	}
	if !validDecision(result.Decision) || !validRisk(result.Risk) || strings.TrimSpace(result.Reason) == "" {
		return op.GuardianReviewResult{}, errors.New("Guardian response contract is invalid")
	}
	return op.GuardianReviewResult{
		RequestID: payload.RequestID, Decision: result.Decision, Risk: result.Risk,
		Reason: truncate(result.Reason, 512), ReviewID: truncate(result.ReviewID, 192),
		PolicyVersion: truncate(result.PolicyVersion, 192),
	}, nil
}

func loadAuth(path string) (authConfig, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return authConfig{}, errors.New("OpenBrain login is required for Guardian")
		}
		return authConfig{}, err
	}
	if !info.Mode().IsRegular() {
		return authConfig{}, errors.New("OpenBrain auth config must be a regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return authConfig{}, errors.New("OpenBrain login is required for Guardian")
		}
		return authConfig{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var auth authConfig
	if err := decoder.Decode(&auth); err != nil {
		return authConfig{}, errors.New("OpenBrain auth config is invalid")
	}
	if auth.Version != 2 || strings.TrimSpace(auth.AIGateway) == "" || strings.TrimSpace(auth.Token) == "" {
		return authConfig{}, errors.New("OpenBrain login with an AI Gateway is required for Guardian")
	}
	parsed, err := url.Parse(strings.TrimSpace(auth.AIGateway))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return authConfig{}, errors.New("OpenBrain AI Gateway URL is invalid")
	}
	if parsed.Scheme == "http" {
		address := net.ParseIP(parsed.Hostname())
		if parsed.Hostname() != "localhost" && (address == nil || !address.IsLoopback()) {
			return authConfig{}, errors.New("OpenBrain AI Gateway URL must use HTTPS")
		}
	}
	auth.AIGateway = strings.TrimRight(strings.TrimSpace(auth.AIGateway), "/")
	auth.Token = strings.TrimSpace(auth.Token)
	return auth, nil
}

func guardianEndpoint(gateway string) string {
	base := strings.TrimRight(strings.TrimSpace(gateway), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/security-reviews"
	}
	return base + "/v1/security-reviews"
}

func sanitizeRequest(request op.ApprovalRequest) reviewRequest {
	existingThreadGrantCount := request.ExistingThreadGrantCount
	if existingThreadGrantCount < 0 {
		existingThreadGrantCount = 0
	}
	if existingThreadGrantCount > 1024 {
		existingThreadGrantCount = 1024
	}
	resources := make([]reviewResource, 0,
		len(request.Resources.ReadPaths)+len(request.Resources.WritePaths)+len(request.Resources.NetworkDomains)+len(request.Resources.SensitiveRights))
	for _, path := range request.Resources.ReadPaths {
		resources = append(resources, reviewResource{Kind: "read_path", Value: classifyPath(request.CWD, path)})
	}
	for _, path := range request.Resources.WritePaths {
		resources = append(resources, reviewResource{Kind: "write_path", Value: classifyPath(request.CWD, path)})
	}
	for _, domain := range request.Resources.NetworkDomains {
		if value := sanitizeDomain(domain); value != "" {
			resources = append(resources, reviewResource{Kind: "network_domain", Value: value})
		}
	}
	for _, right := range request.Resources.SensitiveRights {
		if value := sanitizeName(right); value != "" {
			resources = append(resources, reviewResource{Kind: "sensitive_right", Value: value})
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Kind != resources[j].Kind {
			return resources[i].Kind < resources[j].Kind
		}
		return resources[i].Value < resources[j].Value
	})
	return reviewRequest{
		RequestID:                truncate(sanitizeName(request.RequestID), 192),
		ToolName:                 truncate(sanitizeName(request.ToolName), 192),
		CommandSummary:           redact(request.CommandSummary, 1024),
		IntentSummary:            redact(request.Reason, 512),
		ProfileID:                truncate(sanitizeName(string(request.ProfileID)), 192),
		PolicyVersion:            request.PolicyVersion,
		PolicyDigest:             truncate(sanitizeName(request.PolicyDigest), 192),
		RiskTags:                 sanitizeNames(request.RiskTags, 32),
		Resources:                resources,
		ExistingThreadGrantCount: existingThreadGrantCount,
		AgentID:                  truncate(sanitizeName(request.AgentID), 192),
		ParentAgentID:            truncate(sanitizeName(request.ParentAgentID), 192),
	}
}

func classifyPath(workspace, raw string) string {
	path := filepath.Clean(strings.TrimSpace(raw))
	root := filepath.Clean(strings.TrimSpace(workspace))
	if path == "." || path == "" {
		return "path:unknown"
	}
	if root != "." && root != "" {
		if relative, err := filepath.Rel(root, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "workspace:" + filepath.ToSlash(relative)
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		relative, err := filepath.Rel(home, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			first := strings.ToLower(strings.Split(filepath.ToSlash(relative), "/")[0])
			switch first {
			case ".ssh":
				return "sensitive:ssh"
			case ".aws", ".azure", ".config", ".gnupg", "library":
				return "sensitive:user_credentials_or_config"
			default:
				return "outside_workspace:user_home"
			}
		}
	}
	if filepath.IsAbs(path) {
		return "outside_workspace:host_absolute"
	}
	return "outside_workspace:relative"
}

func sanitizeDomain(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		value = parsed.Hostname()
	} else if host, _, err := strings.Cut(value, ":"); err && host != "" {
		value = host
	}
	value = strings.Trim(value, ".")
	if value == "" || strings.ContainsAny(value, " /\\@") {
		return ""
	}
	return truncate(value, 253)
}

func sanitizeNames(values []string, maximum int) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := sanitizeName(raw)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, truncate(value, 192))
		if len(result) == maximum {
			break
		}
	}
	sort.Strings(result)
	return result
}

func sanitizeName(raw string) string {
	value := strings.TrimSpace(raw)
	if safeName.MatchString(value) {
		return value
	}
	return ""
}

func redact(raw string, maximum int) string {
	value := bearerPattern.ReplaceAllString(raw, "Bearer [REDACTED]")
	value = secretPattern.ReplaceAllString(value, "$1=[REDACTED]")
	value = keyPattern.ReplaceAllString(value, "[REDACTED_KEY]")
	return truncate(strings.TrimSpace(value), maximum)
}

func truncate(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if maximum <= 0 || len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}

func validDecision(value op.GuardianReviewDecision) bool {
	return value == op.GuardianReviewApproveOnce || value == op.GuardianReviewRequireUser || value == op.GuardianReviewDeny
}

func validRisk(value op.GuardianRisk) bool {
	return value == op.GuardianRiskLow || value == op.GuardianRiskMedium || value == op.GuardianRiskHigh || value == op.GuardianRiskCritical
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("Guardian response contains trailing data")
	}
	return nil
}

package gbrain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCommandTimeout = 20 * time.Second
	defaultQueryLimit     = 8
	maxQueryLimit         = 20
)

var (
	ErrGBrainUnavailable = errors.New("gbrain unavailable")
	ErrGBrainTimeout     = errors.New("gbrain timeout")
)

type CommandResult struct {
	Stdout string
	Stderr string
}

type CommandRunner func(ctx context.Context, args []string) (CommandResult, error)

type Service struct {
	baseDir string
	runner  CommandRunner
}

func NewService(baseDir string) *Service {
	service := &Service{baseDir: strings.TrimSpace(baseDir)}
	service.runner = service.runCommand
	return service
}

func NewServiceWithRunner(baseDir string, runner CommandRunner) *Service {
	service := NewService(baseDir)
	if runner != nil {
		service.runner = runner
	}
	return service
}

func (s *Service) ListSources(ctx context.Context) ListSourcesResponse {
	result, err := s.runner(ctx, []string{"sources", "list", "--json"})
	if err != nil {
		return listUnavailableResponse(err)
	}
	sources, err := parseSources(result.Stdout)
	if err != nil {
		return ListSourcesResponse{
			Success: false,
			Code:    "gbrain_error",
			Error:   err.Error(),
			Sources: []Source{},
		}
	}
	return ListSourcesResponse{Success: true, Sources: sources}
}

func (s *Service) Query(ctx context.Context, req QueryRequest) QueryResponse {
	normalizedQuery := strings.TrimSpace(req.Query)
	if normalizedQuery == "" {
		return QueryResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "query is required",
			Results: []QueryResult{},
		}
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "brain"
	}
	if scope != "brain" && scope != "workspace" {
		return QueryResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "scope must be brain or workspace",
			Results: []QueryResult{},
		}
	}
	brainID := strings.TrimSpace(req.BrainID)
	if brainID != "" && brainID != "personal" {
		return QueryResponse{Success: true, Results: []QueryResult{}}
	}

	sourceID := "__all__"
	if scope == "workspace" {
		sourceID = strings.TrimSpace(req.WorkspaceID)
		if sourceID == "" {
			return QueryResponse{
				Success: false,
				Code:    "invalid_request",
				Error:   "workspaceID is required for workspace scope",
				Results: []QueryResult{},
			}
		}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}

	payload := map[string]interface{}{
		"query":     normalizedQuery,
		"limit":     limit,
		"source_id": sourceID,
	}
	rawPayload, _ := json.Marshal(payload)
	result, err := s.runner(ctx, []string{"call", "query", string(rawPayload)})
	if err != nil {
		return queryUnavailableResponse(err)
	}
	rawResults, err := parseSearchResults(result.Stdout)
	if err != nil {
		return QueryResponse{
			Success: false,
			Code:    "gbrain_error",
			Error:   err.Error(),
			Results: []QueryResult{},
		}
	}

	sourceNames := map[string]string{}
	sourcePaths := map[string]string{}
	if sourcesResponse := s.ListSources(ctx); sourcesResponse.Success {
		for _, source := range sourcesResponse.Sources {
			sourceNames[source.SourceID] = source.Name
			sourcePaths[source.SourceID] = source.Path
		}
	}

	results := make([]QueryResult, 0, len(rawResults))
	for _, raw := range rawResults {
		mapped := mapSearchResult(raw, sourceID, sourceNames, sourcePaths)
		if strings.TrimSpace(mapped.Text) == "" {
			continue
		}
		results = append(results, mapped)
	}
	return QueryResponse{Success: true, Results: results}
}

func (s *Service) Status(ctx context.Context) StatusResponse {
	result, err := s.runner(ctx, []string{"call", "get_stats", "{}"})
	if err != nil {
		return statusUnavailableResponse(err)
	}
	status, err := parseStatus(result.Stdout)
	if err != nil {
		return StatusResponse{
			Success: false,
			Code:    "gbrain_error",
			Error:   err.Error(),
		}
	}
	return StatusResponse{Success: true, Status: status}
}

func (s *Service) runCommand(ctx context.Context, args []string) (CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	binary := s.resolveBinary()
	cmd := exec.CommandContext(ctx, binary, args...)
	if s.homeDir() != "" {
		cmd.Dir = s.homeDir()
	}
	cmd.Env = s.commandEnv()
	stdout, err := cmd.Output()
	stderrBytes := []byte(nil)
	if exitErr := (&exec.ExitError{}); errors.As(err, &exitErr) {
		stderrBytes = exitErr.Stderr
	}
	result := CommandResult{Stdout: string(stdout), Stderr: string(stderrBytes)}
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("%w: gbrain command timed out", ErrGBrainTimeout)
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return result, fmt.Errorf("%w: GBrain CLI is not available", ErrGBrainUnavailable)
		}
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if message == "" {
			message = err.Error()
		}
		return result, fmt.Errorf("%w: %s", ErrGBrainUnavailable, message)
	}
	return result, nil
}

func (s *Service) resolveBinary() string {
	if explicit := strings.TrimSpace(os.Getenv("GBRAIN_CLI_PATH")); explicit != "" {
		return explicit
	}
	if s.baseDir != "" {
		bundled := filepath.Join(s.baseDir, "bin", gbrainExecutableName())
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}
	return gbrainExecutableName()
}

func (s *Service) commandEnv() []string {
	env := os.Environ()
	if s.baseDir != "" {
		homeDir := filepath.Dir(s.baseDir)
		env = upsertEnv(env, "OPENBRAIN_BASE_DIR", s.baseDir)
		env = upsertEnv(env, "HOME", homeDir)
		env = upsertEnv(env, "PATH", prependPath([]string{
			filepath.Join(s.baseDir, "bin"),
			filepath.Join(homeDir, ".bun", "bin"),
			"/opt/homebrew/bin",
			"/usr/local/bin",
		}, os.Getenv("PATH")))
	}
	return env
}

func (s *Service) homeDir() string {
	if s.baseDir == "" {
		return ""
	}
	return filepath.Dir(s.baseDir)
}

func gbrainExecutableName() string {
	if runtime.GOOS == "windows" {
		return "gbrain.exe"
	}
	return "gbrain"
}

func prependPath(first []string, existing string) string {
	parts := []string{}
	parts = append(parts, first...)
	for _, part := range strings.Split(existing, string(os.PathListSeparator)) {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
	}
	seen := map[string]bool{}
	unique := parts[:0]
	for _, part := range parts {
		if seen[part] {
			continue
		}
		seen[part] = true
		unique = append(unique, part)
	}
	return strings.Join(unique, string(os.PathListSeparator))
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

type rawSourcesPayload struct {
	Sources []rawSource `json:"sources"`
}

type rawSource struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	LocalPath  string          `json:"local_path"`
	RemoteURL  *string         `json:"remote_url"`
	Federated  bool            `json:"federated"`
	PageCount  json.RawMessage `json:"page_count"`
	LastSyncAt *string         `json:"last_sync_at"`
}

func parseSources(stdout string) ([]Source, error) {
	body := strings.TrimSpace(stdout)
	if body == "" {
		return []Source{}, nil
	}
	var payload rawSourcesPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		var arrayPayload []rawSource
		if arrayErr := json.Unmarshal([]byte(body), &arrayPayload); arrayErr != nil {
			return nil, fmt.Errorf("GBrain sources list returned non-JSON output")
		}
		payload.Sources = arrayPayload
	}
	sources := make([]Source, 0, len(payload.Sources))
	for _, raw := range payload.Sources {
		sourceID := strings.TrimSpace(raw.ID)
		if sourceID == "" {
			continue
		}
		sourcePath := strings.TrimSpace(raw.LocalPath)
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			name = sourceID
		}
		pageCount := parseOptionalInt(raw.PageCount)
		updatedAt := ""
		if raw.LastSyncAt != nil {
			updatedAt = strings.TrimSpace(*raw.LastSyncAt)
		}
		remoteURL := raw.RemoteURL
		if remoteURL == nil {
			empty := ""
			remoteURL = &empty
		}
		sources = append(sources, Source{
			SourceID:    sourceID,
			Name:        name,
			Path:        sourcePath,
			WorkspaceID: sourceID,
			OrgID:       "local",
			BrainID:     "personal",
			UpdatedAt:   updatedAt,
			PageCount:   pageCount,
			Federated:   raw.Federated,
			RemoteURL:   remoteURL,
			Openable:    sourcePath != "",
		})
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return strings.ToLower(sources[i].Name) < strings.ToLower(sources[j].Name)
	})
	return sources, nil
}

func parseOptionalInt(raw json.RawMessage) *int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return &number
	}
	var floatNumber float64
	if err := json.Unmarshal(raw, &floatNumber); err == nil {
		number = int(floatNumber)
		return &number
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, err := strconv.Atoi(strings.TrimSpace(text))
		if err == nil {
			return &parsed
		}
	}
	return nil
}

type toolResultEnvelope struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

type rawSearchResult struct {
	Slug        string          `json:"slug"`
	PageID      json.RawMessage `json:"page_id"`
	Title       string          `json:"title"`
	Type        string          `json:"type"`
	ChunkText   string          `json:"chunk_text"`
	ChunkSource string          `json:"chunk_source"`
	ChunkID     json.RawMessage `json:"chunk_id"`
	ChunkIndex  json.RawMessage `json:"chunk_index"`
	Score       json.RawMessage `json:"score"`
	SourceID    string          `json:"source_id"`
}

func parseSearchResults(stdout string) ([]rawSearchResult, error) {
	body := strings.TrimSpace(stdout)
	if body == "" {
		return []rawSearchResult{}, nil
	}
	var results []rawSearchResult
	if err := json.Unmarshal([]byte(body), &results); err == nil {
		return results, nil
	}
	var envelope toolResultEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return nil, fmt.Errorf("GBrain query returned non-JSON output")
	}
	if envelope.IsError {
		message := ""
		if len(envelope.Content) > 0 {
			message = strings.TrimSpace(envelope.Content[0].Text)
		}
		if message == "" {
			message = "GBrain query failed"
		}
		return nil, errors.New(message)
	}
	for _, content := range envelope.Content {
		if content.Type != "text" && content.Type != "" {
			continue
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(content.Text)), &results); err == nil {
			return results, nil
		}
	}
	return nil, fmt.Errorf("GBrain query response did not include search results")
}

func parseStatus(stdout string) (map[string]interface{}, error) {
	body := strings.TrimSpace(stdout)
	if body == "" {
		return map[string]interface{}{}, nil
	}
	var envelope toolResultEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err == nil && len(envelope.Content) > 0 {
		if envelope.IsError {
			message := "GBrain status failed"
			if strings.TrimSpace(envelope.Content[0].Text) != "" {
				message = strings.TrimSpace(envelope.Content[0].Text)
			}
			return nil, errors.New(message)
		}
		for _, content := range envelope.Content {
			if content.Type != "text" && content.Type != "" {
				continue
			}
			var status map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimSpace(content.Text)), &status); err == nil {
				return status, nil
			}
		}
		return nil, fmt.Errorf("GBrain status response did not include status JSON")
	}
	var status map[string]interface{}
	if err := json.Unmarshal([]byte(body), &status); err == nil {
		return status, nil
	}
	return nil, fmt.Errorf("GBrain status returned non-JSON output")
}

func mapSearchResult(raw rawSearchResult, requestedSourceID string, sourceNames map[string]string, sourcePaths map[string]string) QueryResult {
	sourceID := strings.TrimSpace(raw.SourceID)
	if sourceID == "" && requestedSourceID != "__all__" {
		sourceID = requestedSourceID
	}
	if sourceID == "" || sourceID == "__all__" {
		sourceID = "default"
	}
	workspaceName := sourceNames[sourceID]
	if workspaceName == "" {
		workspaceName = sourceID
	}
	slug := strings.Trim(strings.TrimSpace(raw.Slug), "/")
	title := strings.TrimSpace(raw.Title)
	if title == "" {
		title = slug
	}
	relativePath := strings.TrimSpace(raw.ChunkSource)
	if relativePath == "" {
		relativePath = slug
	}
	if relativePath != "" && !strings.Contains(filepath.Base(relativePath), ".") {
		relativePath += ".md"
	}
	queryPath := ""
	if sourcePath := strings.TrimSpace(sourcePaths[sourceID]); sourcePath != "" && relativePath != "" {
		queryPath = filepath.Join(sourcePath, filepath.FromSlash(relativePath))
	}
	chunkID := rawMessageString(raw.ChunkID)
	if chunkID == "" {
		chunkID = strings.Join([]string{sourceID, slug, rawMessageString(raw.ChunkIndex)}, ":")
	}
	return QueryResult{
		ChunkID:       chunkID,
		WorkspaceID:   sourceID,
		WorkspaceName: workspaceName,
		Path:          queryPath,
		RelativePath:  relativePath,
		Title:         title,
		Text:          strings.TrimSpace(raw.ChunkText),
		Score:         rawMessageFloat(raw.Score),
	}
}

func rawMessageString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return strings.TrimSpace(number.String())
	}
	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		return strconv.FormatInt(integer, 10)
	}
	return strings.Trim(string(raw), `"`)
}

func rawMessageFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(text), 64)
		return parsed
	}
	return 0
}

func listUnavailableResponse(err error) ListSourcesResponse {
	code := errorCode(err)
	return ListSourcesResponse{Success: false, Code: code, Error: cleanError(err), Sources: []Source{}}
}

func queryUnavailableResponse(err error) QueryResponse {
	code := errorCode(err)
	return QueryResponse{Success: false, Code: code, Error: cleanError(err), Results: []QueryResult{}}
}

func statusUnavailableResponse(err error) StatusResponse {
	code := errorCode(err)
	return StatusResponse{Success: false, Code: code, Error: cleanError(err)}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrGBrainTimeout):
		return "gbrain_timeout"
	case errors.Is(err, ErrGBrainUnavailable):
		return "gbrain_unavailable"
	default:
		return "gbrain_error"
	}
}

func cleanError(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "GBrain is unavailable."
	}
	return message
}

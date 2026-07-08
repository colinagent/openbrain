package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

const (
	commandStateStarted             = "started"
	commandStateFinished            = "finished"
	commandStateFailed              = "failed"
	commandStateCancelled           = "cancelled"
	commandStateOutputLimitExceeded = "output_limit_exceeded"
	commandStateTimeout             = "timeout"

	commandDefaultTimeout   = 60 * time.Second
	commandMaxMarkdownBytes = 2 * 1024 * 1024
	commandMaxCapturedBytes = 8 * 1024 * 1024
	commandTruncatedHead    = 128 * 1024
	commandTruncatedTail    = 384 * 1024
	commandChunkSize        = 4 * 1024
	commandSlugMaxLen       = 64
)

var commandANSIRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type commandManager struct {
	mu     sync.Mutex
	seq    uint64
	byID   map[string]*runningCommand
	byPath map[string]*runningCommand
}

type runningCommand struct {
	id         string
	targetPath string
	client     *Client
	cancel     context.CancelFunc

	mu         sync.Mutex
	stopReason string
}

type commandRunResult struct {
	state     string
	output    string
	exitCode  *int
	writeLog  bool
	logPath   string
	markdown  string
	notifyErr string
}

func newCommandManager() *commandManager {
	return &commandManager{
		byID:   make(map[string]*runningCommand),
		byPath: make(map[string]*runningCommand),
	}
}

func (m *commandManager) nextID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	return fmt.Sprintf("cmd-%d-%d", time.Now().UnixMilli(), m.seq)
}

func (m *commandManager) reserve(id, targetPath string, client *Client, cancel context.CancelFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.byPath[targetPath]; existing != nil {
		return fmt.Errorf("a command is already running for %s", targetPath)
	}
	run := &runningCommand{
		id:         id,
		targetPath: targetPath,
		client:     client,
		cancel:     cancel,
	}
	m.byID[id] = run
	m.byPath[targetPath] = run
	return nil
}

func (m *commandManager) release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.byID[id]
	if run == nil {
		return
	}
	delete(m.byID, id)
	delete(m.byPath, run.targetPath)
}

func (m *commandManager) hasTargetPath(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byPath[path] != nil
}

func (m *commandManager) stop(commandID string) error {
	m.mu.Lock()
	run := m.byID[commandID]
	m.mu.Unlock()
	if run == nil {
		return fmt.Errorf("command not found: %s", commandID)
	}
	run.setStopReason(commandStateCancelled)
	run.cancel()
	return nil
}

func (r *runningCommand) setStopReason(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopReason != "" {
		return
	}
	r.stopReason = reason
}

func (r *runningCommand) getStopReason() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopReason
}

func (h *Handler) handleCommandExec(client *Client, params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CommandExecParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}

	command := strings.TrimSpace(p.Command)
	if command == "" {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "command is required",
		}
	}

	workspaceRoot, targetPath, created, err := h.resolveCommandExecTarget(p.WorkspaceRoot, p.TargetPath, command)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	commandID := h.cmd.nextID()
	runCtx, cancel := context.WithTimeout(context.Background(), commandDefaultTimeout)
	if err := h.cmd.reserve(commandID, targetPath, client, cancel); err != nil {
		cancel()
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	go h.runCommandAsync(runCtx, commandID, workspaceRoot, targetPath, command, client)

	return &protocol.CommandExecResult{
		CommandID:     commandID,
		FilePath:      targetPath,
		WorkspaceRoot: workspaceRoot,
		Created:       created,
	}, nil
}

func (h *Handler) handleCommandStop(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CommandStopParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	commandID := strings.TrimSpace(p.CommandID)
	if commandID == "" {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "commandID is required",
		}
	}
	if err := h.cmd.stop(commandID); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}
	return &protocol.CommandStopResult{OK: true}, nil
}

func (h *Handler) runCommandAsync(
	runCtx context.Context,
	commandID string,
	workspaceRoot string,
	targetPath string,
	command string,
	client *Client,
) {
	defer h.cmd.release(commandID)
	h.notifyCommandState(client, &protocol.CommandStateEvent{
		CommandID: commandID,
		FilePath:  targetPath,
		State:     commandStateStarted,
	})

	result := h.executeCommand(runCtx, commandID, workspaceRoot, targetPath, command)
	if err := appendCommandMarkdown(targetPath, result.markdown); err != nil {
		result.state = commandStateFailed
		result.notifyErr = "failed to write command log: " + err.Error()
	}
	h.notifyCommandState(client, &protocol.CommandStateEvent{
		CommandID: commandID,
		FilePath:  targetPath,
		State:     result.state,
		ExitCode:  result.exitCode,
		Error:     result.notifyErr,
	})
}

func (h *Handler) executeCommand(
	runCtx context.Context,
	commandID string,
	workspaceRoot string,
	targetPath string,
	command string,
) commandRunResult {
	run := func() *runningCommand {
		h.cmd.mu.Lock()
		defer h.cmd.mu.Unlock()
		return h.cmd.byID[commandID]
	}()

	rawOutput, exitCode, execErr := runCommandProcess(runCtx, command, workspaceRoot, run)
	cleanedOutput := cleanCommandOutput(rawOutput)
	state := commandStateFinished
	notifyErr := ""
	if run != nil {
		switch run.getStopReason() {
		case commandStateCancelled:
			state = commandStateCancelled
		case commandStateOutputLimitExceeded:
			state = commandStateOutputLimitExceeded
		}
	}
	if state == commandStateFinished && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		state = commandStateTimeout
	}
	if state == commandStateFinished && execErr != nil {
		state = commandStateFailed
		notifyErr = execErr.Error()
	}
	if state == commandStateFinished && runCtx.Err() == context.Canceled {
		state = commandStateCancelled
	}

	if state == commandStateFinished && exitCode != nil && *exitCode != 0 {
		state = commandStateFailed
	}

	writeLog := len(cleanedOutput) > commandMaxMarkdownBytes || state == commandStateOutputLimitExceeded
	logPath := strings.TrimSuffix(targetPath, ".md") + ".full.log"
	if writeLog {
		if err := appendCommandFullLog(logPath, commandID, command, cleanedOutput); err != nil {
			writeLog = false
			notifyErr = joinCommandErrors(notifyErr, "failed to write full output log: "+err.Error())
		}
	}

	markdown := buildCommandMarkdownBlock(command, cleanedOutput, state, exitCode, targetPath, logPath, writeLog, notifyErr, commandID)
	if len(markdown) > commandMaxMarkdownBytes {
		markdown = forceTrimMarkdown(markdown, commandMaxMarkdownBytes)
	}

	return commandRunResult{
		state:     state,
		output:    cleanedOutput,
		exitCode:  exitCode,
		writeLog:  writeLog,
		logPath:   logPath,
		markdown:  markdown,
		notifyErr: notifyErr,
	}
}

func runCommandProcess(
	runCtx context.Context,
	command string,
	workspaceRoot string,
	run *runningCommand,
) (string, *int, error) {
	invocation := resolveCommandShellInvocation(command)
	cmd := exec.CommandContext(runCtx, invocation.Executable, invocation.Args...)
	cmd.Dir = workspaceRoot
	prepareCommandProcess(cmd)
	cmd.Env = common.WithPrependedPath(nil, common.OpagentBinDir())

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("start command: %w", err)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-runCtx.Done():
			killCommandProcessTree(cmd)
		case <-done:
		}
	}()

	collector := &commandOutputCollector{}
	readErrCh := make(chan error, 2)
	chunkCh := make(chan []byte, 64)
	var readers sync.WaitGroup
	readers.Add(2)

	go readCommandPipe(stdoutPipe, chunkCh, readErrCh, &readers)
	go readCommandPipe(stderrPipe, chunkCh, readErrCh, &readers)
	go func() {
		readers.Wait()
		close(chunkCh)
		close(readErrCh)
	}()

	overflowTriggered := false
	for chunk := range chunkCh {
		if collector.Append(chunk) && !overflowTriggered {
			overflowTriggered = true
			if run != nil {
				run.setStopReason(commandStateOutputLimitExceeded)
			}
			if run != nil {
				run.cancel()
			}
		}
	}

	waitErr := cmd.Wait()
	close(done)

	for readErr := range readErrCh {
		if readErr != nil && !errors.Is(readErr, os.ErrClosed) && !strings.Contains(readErr.Error(), "file already closed") {
			return collector.String(), nil, readErr
		}
	}

	if overflowTriggered {
		return collector.String(), nil, nil
	}

	if runCtx.Err() != nil {
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			return collector.String(), nil, nil
		case errors.Is(runCtx.Err(), context.Canceled):
			return collector.String(), nil, nil
		}
	}

	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			code := exitErr.ExitCode()
			return collector.String(), &code, nil
		}
		return collector.String(), nil, waitErr
	}

	code := 0
	return collector.String(), &code, nil
}

func readCommandPipe(
	reader io.ReadCloser,
	chunkCh chan<- []byte,
	errCh chan<- error,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	defer func() {
		_ = reader.Close()
	}()

	buf := make([]byte, commandChunkSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			chunkCh <- chunk
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			errCh <- err
			return
		}
	}
}

type commandOutputCollector struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	totalSize int
	overflow  bool
}

func (c *commandOutputCollector) Append(chunk []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.overflow {
		return true
	}
	remaining := commandMaxCapturedBytes - c.totalSize
	if remaining <= 0 {
		c.overflow = true
		return true
	}
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
		c.overflow = true
	}
	c.totalSize += len(chunk)
	_, _ = c.buffer.Write(chunk)
	return c.overflow
}

func (c *commandOutputCollector) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffer.String()
}

func appendCommandMarkdown(path string, block string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	prefix, err := commandMarkdownAppendPrefix(path)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	if _, err := f.WriteString(prefix + block); err != nil {
		return err
	}
	return nil
}

func commandMarkdownAppendPrefix(path string) (string, error) {
	info, err := os.Stat(path)
	if err == nil && info.Size() > 0 {
		return "\n\n", nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return buildCommandLogFrontmatter(path), nil
}

func buildCommandLogFrontmatter(path string) string {
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.TrimSpace(title) == "" {
		title = "Command"
	}
	return fmt.Sprintf("---\nindex: command-log\ntitle: %s\n---\n\n", quoteCommandFrontmatterString(title))
}

func quoteCommandFrontmatterString(value string) string {
	encoded, err := json.Marshal(strings.TrimSpace(value))
	if err != nil {
		return "\"Command\""
	}
	return string(encoded)
}

func appendCommandFullLog(path, commandID, command, output string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	prefix := ""
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		prefix = "\n\n"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString("=== ")
	b.WriteString(commandID)
	b.WriteString(" ===\n")
	b.WriteString("$ ")
	b.WriteString(command)
	if output != "" {
		b.WriteByte('\n')
		b.WriteString(output)
		if !strings.HasSuffix(output, "\n") {
			b.WriteByte('\n')
		}
	} else {
		b.WriteString("\n(no output)\n")
	}
	_, err = f.WriteString(b.String())
	return err
}

func buildCommandMarkdownBlock(
	command string,
	output string,
	state string,
	exitCode *int,
	targetPath string,
	logPath string,
	hasLog bool,
	notifyErr string,
	commandID string,
) string {
	renderBody := func(renderedOutput string, truncated bool) string {
		var body strings.Builder
		body.WriteString("$ ")
		body.WriteString(command)
		if renderedOutput != "" {
			body.WriteByte('\n')
			body.WriteString(renderedOutput)
		}
		if truncated {
			body.WriteByte('\n')
			body.WriteString("# output truncated in markdown")
		}
		if hasLog {
			body.WriteByte('\n')
			body.WriteString("# full_output_log: ")
			body.WriteString(filepath.Base(logPath))
			body.WriteString(" (commandID: ")
			body.WriteString(commandID)
			body.WriteByte(')')
		}
		switch state {
		case commandStateCancelled:
			body.WriteByte('\n')
			body.WriteString("# cancelled")
		case commandStateOutputLimitExceeded:
			body.WriteByte('\n')
			body.WriteString("# output_limit_exceeded")
		case commandStateTimeout:
			body.WriteByte('\n')
			body.WriteString("# timeout")
		case commandStateFailed:
			if exitCode != nil {
				body.WriteByte('\n')
				body.WriteString("# exit_code: ")
				body.WriteString(fmt.Sprintf("%d", *exitCode))
			} else if notifyErr != "" {
				body.WriteByte('\n')
				body.WriteString("# failed: ")
				body.WriteString(notifyErr)
			} else {
				body.WriteByte('\n')
				body.WriteString("# failed")
			}
		default:
			if exitCode != nil {
				body.WriteByte('\n')
				body.WriteString("# exit_code: ")
				body.WriteString(fmt.Sprintf("%d", *exitCode))
			}
		}
		return body.String()
	}

	trimmedOutput, truncated := truncateCommandMarkdownOutput(output, false)
	block := formatCommandFencedBlock(renderBody(trimmedOutput, truncated), commandMarkdownFenceInfo())
	if len(block) <= commandMaxMarkdownBytes {
		return block
	}
	trimmedOutput, truncated = truncateCommandMarkdownOutput(output, true)
	block = formatCommandFencedBlock(renderBody(trimmedOutput, truncated), commandMarkdownFenceInfo())
	if len(block) <= commandMaxMarkdownBytes {
		return block
	}
	return formatCommandFencedBlock(renderBody(forceTrimMarkdown(trimmedOutput, commandTruncatedHead), true), commandMarkdownFenceInfo())
}

func truncateCommandMarkdownOutput(output string, force bool) (string, bool) {
	if !force && len(output) <= commandMaxMarkdownBytes {
		return output, false
	}
	head := trimUTF8Prefix(output, commandTruncatedHead)
	tail := trimUTF8Suffix(output, commandTruncatedTail)
	var b strings.Builder
	b.Grow(len(head) + len(tail) + 128)
	b.WriteString(head)
	if head != "" && !strings.HasSuffix(head, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("[... output truncated ...]")
	if tail != "" {
		if !strings.HasPrefix(tail, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(tail)
	}
	return b.String(), true
}

func trimUTF8Prefix(input string, maxBytes int) string {
	if maxBytes <= 0 || len(input) <= maxBytes {
		return input
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(input[:end]) {
		end--
	}
	return input[:end]
}

func trimUTF8Suffix(input string, maxBytes int) string {
	if maxBytes <= 0 || len(input) <= maxBytes {
		return input
	}
	start := len(input) - maxBytes
	for start < len(input) && !utf8.ValidString(input[start:]) {
		start++
	}
	return input[start:]
}

func forceTrimMarkdown(input string, maxBytes int) string {
	if len(input) <= maxBytes {
		return input
	}
	return trimUTF8Prefix(input, maxBytes)
}

func cleanCommandOutput(input string) string {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return commandANSIRegex.ReplaceAllString(normalized, "")
}

func formatCommandFencedBlock(body, infoString string) string {
	width := maxCommandInt(3, maxCommandBacktickRun(body)+1)
	fence := strings.Repeat("`", width)
	open := fence + strings.TrimSpace(infoString)

	var b strings.Builder
	b.Grow(len(body) + len(open) + len(fence) + 2)
	b.WriteString(open)
	b.WriteByte('\n')
	b.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(fence)
	return b.String()
}

func maxCommandBacktickRun(text string) int {
	maxRun := 0
	current := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '`' {
			current++
			if current > maxRun {
				maxRun = current
			}
			continue
		}
		current = 0
	}
	return maxRun
}

func maxCommandInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (h *Handler) notifyCommandState(client *Client, event *protocol.CommandStateEvent) {
	if client == nil || event == nil {
		return
	}
	data, err := json.Marshal(protocol.NewNotification("command/state", event))
	if err != nil {
		return
	}
	if !client.Send(data) {
		go client.server.unregisterClient(client)
	}
}

func (h *Handler) resolveCommandExecTarget(
	workspaceRoot string,
	targetPath string,
	command string,
) (string, string, bool, error) {
	resolvedWorkspaceRoot, err := normalizeCommandPath(workspaceRoot)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid workspaceRoot: %w", err)
	}
	info, err := os.Stat(resolvedWorkspaceRoot)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid workspaceRoot: %w", err)
	}
	if !info.IsDir() {
		return "", "", false, fmt.Errorf("workspaceRoot is not a directory: %s", resolvedWorkspaceRoot)
	}

	tempDir := filepath.Join(resolvedWorkspaceRoot, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", "", false, err
	}

	trimmedTarget := strings.TrimSpace(targetPath)
	if trimmedTarget == "" {
		candidate, err := h.findAvailableCommandLogPath(tempDir, command)
		if err != nil {
			return "", "", false, err
		}
		return resolvedWorkspaceRoot, candidate, true, nil
	}

	resolvedTargetPath, err := normalizeCommandPath(trimmedTarget)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid targetPath: %w", err)
	}
	if filepath.Ext(resolvedTargetPath) != ".md" {
		return "", "", false, fmt.Errorf("targetPath must be a .md file")
	}
	if !isPathInsideRoot(resolvedTargetPath, tempDir) {
		return "", "", false, fmt.Errorf("targetPath must be inside %s", tempDir)
	}
	return resolvedWorkspaceRoot, resolvedTargetPath, false, nil
}

func (h *Handler) findAvailableCommandLogPath(tempDir string, command string) (string, error) {
	slug := slugifyCommand(command)
	for idx := 1; idx < 10_000; idx++ {
		name := slug
		if idx > 1 {
			name = fmt.Sprintf("%s-%d", slug, idx)
		}
		candidate := filepath.Join(tempDir, name+".md")
		if h.cmd.hasTargetPath(candidate) {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("failed to allocate temp command log path")
}

func slugifyCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "command"
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	lastDash := false
	for _, r := range strings.ToLower(trimmed) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if lastDash {
			continue
		}
		b.WriteByte('-')
		lastDash = true
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "command"
	}
	if len(slug) > commandSlugMaxLen {
		slug = slug[:commandSlugMaxLen]
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		return "command"
	}
	return slug
}

func normalizeCommandPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("path must be absolute")
	}
	return cleaned, nil
}

func isPathInsideRoot(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func joinCommandErrors(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	switch {
	case existing == "":
		return next
	case next == "":
		return existing
	default:
		return existing + "; " + next
	}
}
